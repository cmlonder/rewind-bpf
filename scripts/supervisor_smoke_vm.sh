#!/usr/bin/env bash
set -euo pipefail

if [[ "$(uname -s)" != "Linux" ]]; then
  echo "rewind supervisor smoke: run only inside the disposable Ubuntu VM" >&2
  exit 2
fi
if [[ "${REWIND_VM_CONFIRM:-}" != "VM_ONLY" ]]; then
  echo "set REWIND_VM_CONFIRM=VM_ONLY to run the synthetic supervisor smoke" >&2
  exit 2
fi

BIN="${REWIND_BIN:-/tmp/rewind}"
OBJECT="${REWIND_SENSOR_OBJECT:-$PWD/ebpf/rewind_trace.bpf.o}"
ROOT="$(mktemp -d /home/vagrant/rewind-supervisor.XXXXXX)"
SUPERVISOR_PID=""
cleanup() {
  if [[ -n "$SUPERVISOR_PID" ]]; then
    sudo kill "$SUPERVISOR_PID" 2>/dev/null || true
  fi
  sudo rm -rf "$ROOT"
}
trap cleanup EXIT

mkdir -p "$ROOT/workspace"
printf '%s\n' '#!/bin/sh' 'test "$REWIND_CREDENTIAL_REF" = github' 'printf "vm-secret-token\n"' > "$ROOT/credential-provider.sh"
chmod 700 "$ROOT/credential-provider.sh"
printf 'original\n' > "$ROOT/workspace/marker.txt"
printf '%s\n' \
  'read:' \
  '  mode: off' \
  '' \
  'write:' \
  '  mode: rollback' \
  '  scope: workspace' \
  '' \
  'network:' \
  '  mode: audit' > "$ROOT/policy.yaml"

RUNTIME_ROOTS="/bin,/usr/bin,/lib,/usr/lib,/etc"
if [[ -e /lib64 ]]; then
  RUNTIME_ROOTS+="/lib64"
fi

sudo env PATH="$PATH" "$BIN" run \
  --workspace "$ROOT/workspace" \
  --runtime-root "$ROOT/runtime" \
  --history "$ROOT/history.json" \
  --policy "$ROOT/policy.yaml" \
  --record "$ROOT/runtime/record.json" \
  --sensor-object "$OBJECT" \
  --runtime-roots "$RUNTIME_ROOTS" \
  --overlay-backend fuse \
  --on-success review -- \
  /bin/sh -c 'printf "accepted-by-supervisor\\n" > marker.txt' > "$ROOT/run.log"
RUN_ID="$(sed -n 's/.*run_id=\([^ ]*\).*/\1/p' "$ROOT/run.log")"
test -n "$RUN_ID"

SOCKET="$ROOT/supervisor.sock"
TOKEN_FILE="$ROOT/supervisor.token"
sudo "$BIN" supervisor --socket "$SOCKET" --history "$ROOT/history.json" --token-file "$TOKEN_FILE" --credential-provider-command "$ROOT/credential-provider.sh" > "$ROOT/supervisor.log" 2>&1 &
SUPERVISOR_PID=$!
for _ in $(seq 1 50); do
  [[ -S "$SOCKET" ]] && break
  sleep 0.1
done
test -S "$SOCKET"

test "$(sudo curl --unix-socket "$SOCKET" -sS -o /dev/null -w '%{http_code}' http://localhost/health)" = 200
TOKEN="$(sudo cat "$TOKEN_FILE")"
test "$(sudo curl --unix-socket "$SOCKET" -sS -o /dev/null -w '%{http_code}' http://localhost/v1/actions -X POST -H 'Content-Type: application/json' -d '{"action":"status","run_id":"'"$RUN_ID"'"}')" = 401
STATUS="$(sudo curl --unix-socket "$SOCKET" -sS http://localhost/v1/actions -X POST -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' -d '{"action":"status","run_id":"'"$RUN_ID"'"}')"
echo "$STATUS" | grep -q '"ok":true'
LEASE="$(sudo curl --unix-socket "$SOCKET" -sS http://localhost/v1/credential-leases -X POST -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' -d '{"ref":"github","scopes":["read:org"]}')"
echo "$LEASE" | jq -e '.id != null and .ref == "github" and .secret_exposed == false' >/dev/null
if echo "$LEASE" | grep -q 'vm-secret-token'; then
  echo "credential lease unexpectedly exposed provider output" >&2
  exit 1
fi
SESSION="$(sudo curl --unix-socket "$SOCKET" -sS http://localhost/v1/sessions -X POST -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' -d '{"action":"acquire","run_id":"'"$RUN_ID"'","owner":"browser-a","ttl_seconds":60}')"
echo "$SESSION" | jq -e '.run_id == "'"$RUN_ID"'" and .owner == "browser-a" and .expires_at != null' >/dev/null
TAKEOVER="$(sudo curl --unix-socket "$SOCKET" -sS http://localhost/v1/sessions -X POST -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' -d '{"action":"takeover","run_id":"'"$RUN_ID"'","owner":"browser-b","ttl_seconds":60}')"
echo "$TAKEOVER" | jq -e '.owner == "browser-b"' >/dev/null
PRUNE="$(sudo curl --unix-socket "$SOCKET" -sS http://localhost/v1/history/prune -X POST -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' -d '{"keep":1}')"
echo "$PRUNE" | grep -q '"ok":true'
COMMIT="$(sudo curl --unix-socket "$SOCKET" -sS http://localhost/v1/actions -X POST -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' -d '{"action":"commit","run_id":"'"$RUN_ID"'","confirmation":"COMMIT"}')"
echo "$COMMIT" | grep -q '"ok":true'
test "$(cat "$ROOT/workspace/marker.txt")" = accepted-by-supervisor
sudo curl --unix-socket "$SOCKET" -sS -H "Authorization: Bearer $TOKEN" 'http://localhost/v1/audit?limit=20' | grep -q '"action":"credential_lease"'
sudo curl --unix-socket "$SOCKET" -sS -H "Authorization: Bearer $TOKEN" 'http://localhost/v1/audit?limit=20' | grep -q '"action":"commit"'

echo "SUPERVISOR_SMOKE=PASS"
