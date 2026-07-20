#!/usr/bin/env bash
set -euo pipefail

if [[ "$(uname -s)" != "Linux" ]]; then
  echo "rewind policy bundle smoke: run only inside the disposable Ubuntu VM" >&2
  exit 2
fi
if [[ "${REWIND_VM_CONFIRM:-}" != "VM_ONLY" ]]; then
  echo "set REWIND_VM_CONFIRM=VM_ONLY to run the synthetic policy bundle smoke" >&2
  exit 2
fi

BIN="${REWIND_BIN:-/tmp/rewind}"
ROOT="$(mktemp -d /home/vagrant/rewind-policy-bundle.XXXXXX)"
SUPERVISOR_PID=""
cleanup() {
  if [[ -n "$SUPERVISOR_PID" ]]; then
    kill "$SUPERVISOR_PID" 2>/dev/null || true
  fi
  rm -rf "$ROOT"
}
trap cleanup EXIT

mkdir -p "$ROOT/workspace"
printf '%s\n' \
  'read:' \
  '  mode: audit' \
  '' \
  'write:' \
  '  mode: rollback' \
  '  scope: workspace' \
  '' \
  'network:' \
  '  mode: off' > "$ROOT/policy.yaml"

"$BIN" policy keygen --private "$ROOT/private" --public "$ROOT/public" >/dev/null
"$BIN" policy sign "$ROOT/policy.yaml" --name signed-agent --version 1.0.0 --private-key "$ROOT/private" --output "$ROOT/bundle.json" >/dev/null

SOCKET="$ROOT/supervisor.sock"
TOKEN_FILE="$ROOT/supervisor.token"
HTTP_ADDR="127.0.0.1:18789"
"$BIN" supervisor --socket "$SOCKET" --history "$ROOT/history.json" --config "$ROOT/config.json" --token-file "$TOKEN_FILE" --trusted-policy-keys "$ROOT/public" --http-listen "$HTTP_ADDR" --cors-origin http://127.0.0.1:4173 > "$ROOT/supervisor.log" 2>&1 &
SUPERVISOR_PID=$!
for _ in $(seq 1 50); do
  curl -fsS "http://$HTTP_ADDR/health" >/dev/null 2>&1 && break
  sleep 0.1
done
curl -fsS "http://$HTTP_ADDR/health" >/dev/null
TOKEN="$(cat "$TOKEN_FILE")"

STATUS="$(curl -fsS -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' -X POST --data-binary @"$ROOT/bundle.json" "http://$HTTP_ADDR/v1/policy-bundles")"
echo "$STATUS" | grep -q '"ok":true'
test "$(jq -r '.policies[0].signed' "$ROOT/config.json")" = true
test "$(jq -r '.policies[0].signer_key_id' "$ROOT/config.json")" != null

jq '.signature = "tampered"' "$ROOT/bundle.json" > "$ROOT/tampered.json"
if curl -fsS -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' -X POST --data-binary @"$ROOT/tampered.json" "http://$HTTP_ADDR/v1/policy-bundles" >/dev/null; then
  echo "tampered policy bundle unexpectedly accepted" >&2
  exit 1
fi
grep -q 'policy_bundle_import' "$ROOT/history.json.actions.jsonl"
echo "POLICY_BUNDLE_SMOKE=PASS"
