#!/usr/bin/env bash
set -euo pipefail

# This is intentionally a Linux/VM-only acceptance gate. It creates synthetic
# data under a temporary directory and never accepts a real project path.
if [[ "$(uname -s)" != "Linux" ]]; then
  echo "rewind acceptance: run only inside the disposable Ubuntu VM" >&2
  exit 2
fi
if [[ "${REWIND_VM_CONFIRM:-}" != "VM_ONLY" ]]; then
  echo "set REWIND_VM_CONFIRM=VM_ONLY to run the destructive synthetic matrix" >&2
  exit 2
fi

BIN="${REWIND_BIN:-/tmp/rewind}"
OBJECT="${REWIND_SENSOR_OBJECT:-$PWD/ebpf/rewind_trace.bpf.o}"
if [[ ! -x "$BIN" ]]; then
  echo "rewind acceptance: binary not found or not executable: $BIN" >&2
  exit 2
fi

RUNTIME_ROOTS="/bin,/usr/bin,/lib,/usr/lib,/etc"
if [[ -e /lib64 ]]; then
  RUNTIME_ROOTS+="/lib64"
fi
if [[ ! -r "$OBJECT" ]]; then
  echo "rewind acceptance: sensor object not found: $OBJECT" >&2
  exit 2
fi

ROOT="$(mktemp -d /home/vagrant/rewind-accept.XXXXXX)"
SERVER_PID=""
cleanup() {
  if [[ -n "$SERVER_PID" ]]; then
    kill "$SERVER_PID" 2>/dev/null || true
  fi
  sudo rm -rf "$ROOT"
}
trap cleanup EXIT

make_policy() {
  local path="$1" read_mode="$2" network_mode="$3"
  printf '%s\n' \
    'read:' \
    "  mode: $read_mode" \
    '  deny:' \
    '    - "**/*.env"' \
    '' \
    'write:' \
    '  mode: rollback' \
    '  scope: workspace' \
    '' \
    'network:' \
    "  mode: $network_mode" > "$path"
}

run_args() {
  local workspace="$1" runtime="$2" policy="$3" record="$4"
  printf '%s\n' \
    --workspace "$workspace" \
    --runtime-root "$runtime" \
    --policy "$policy" \
    --record "$record" \
    --sensor-object "$OBJECT" \
    --runtime-roots "$RUNTIME_ROOTS" \
    --overlay-backend fuse
}

echo "acceptance root: $ROOT"

# 1. Read denial + recursive deletion + discard-by-default rollback.
mkdir -p "$ROOT/core/workspace/src"
printf 'original-source\n' > "$ROOT/core/workspace/src/marker.txt"
printf 'SYNTHETIC_ONLY=true\n' > "$ROOT/core/workspace/synthetic.env"
make_policy "$ROOT/core/policy.yaml" enforce audit
sudo env PATH="$PATH" "$BIN" run $(run_args "$ROOT/core/workspace" "$ROOT/core/runtime" "$ROOT/core/policy.yaml" "$ROOT/core/runtime/record.json") -- \
  /bin/sh -c 'cat synthetic.env >/dev/null 2>read.err || true; rm -rf src; printf "created-by-agent\\n" > generated.txt'
test -f "$ROOT/core/workspace/src/marker.txt"
test ! -e "$ROOT/core/workspace/generated.txt"
sudo "$BIN" verify --record "$ROOT/core/runtime/record.json"
sudo "$BIN" bundle create --record "$ROOT/core/runtime/record.json" --output "$ROOT/core/evidence.tar.gz"
sudo "$BIN" bundle verify --input "$ROOT/core/evidence.tar.gz"
echo "core rollback/read denial: PASS"

# 2. Explicit review and conflict-checked commit.
mkdir -p "$ROOT/commit/workspace"
printf 'before\n' > "$ROOT/commit/workspace/accepted.txt"
make_policy "$ROOT/commit/policy.yaml" off audit
sudo env PATH="$PATH" "$BIN" run $(run_args "$ROOT/commit/workspace" "$ROOT/commit/runtime" "$ROOT/commit/policy.yaml" "$ROOT/commit/runtime/record.json") --on-success review -- \
  /bin/sh -c 'printf "accepted\\n" > accepted.txt'
sudo "$BIN" commit --record "$ROOT/commit/runtime/record.json" --confirm
test "$(cat "$ROOT/commit/workspace/accepted.txt")" = accepted
echo "review/commit: PASS"

# 2b. Explicit clean-branch Git acceptance. The repository and candidate are
# disposable; the adapter must refuse a dirty or wrong checkout before apply.
mkdir -p "$ROOT/branch/workspace"
git -C "$ROOT/branch/workspace" init -b main >/dev/null
git -C "$ROOT/branch/workspace" config user.email test@example.invalid
git -C "$ROOT/branch/workspace" config user.name "Rewind Acceptance"
printf 'before\n' > "$ROOT/branch/workspace/accepted.txt"
git -C "$ROOT/branch/workspace" add --all
git -C "$ROOT/branch/workspace" commit -m initial >/dev/null
make_policy "$ROOT/branch/policy.yaml" off audit
sudo env PATH="$PATH" "$BIN" run $(run_args "$ROOT/branch/workspace" "$ROOT/branch/runtime" "$ROOT/branch/policy.yaml" "$ROOT/branch/runtime/record.json") --on-success review -- \
  /bin/sh -c 'printf "accepted\n" > accepted.txt; printf "generated\n" > generated.txt'
sudo "$BIN" branch apply --record "$ROOT/branch/runtime/record.json" --repo "$ROOT/branch/workspace" --branch main --confirm --commit --message "accept reviewed result" >/dev/null
test "$(cat "$ROOT/branch/workspace/accepted.txt")" = accepted
test "$(cat "$ROOT/branch/workspace/generated.txt")" = generated
test "$(git -C "$ROOT/branch/workspace" log -1 --pretty=%s)" = "accept reviewed result"
sudo "$BIN" rollback --record "$ROOT/branch/runtime/record.json"
echo "clean-branch acceptance: PASS"

mkdir -p "$ROOT/conflict/workspace"
printf 'base\n' > "$ROOT/conflict/workspace/conflict.txt"
make_policy "$ROOT/conflict/policy.yaml" off audit
sudo env PATH="$PATH" "$BIN" run $(run_args "$ROOT/conflict/workspace" "$ROOT/conflict/runtime" "$ROOT/conflict/policy.yaml" "$ROOT/conflict/runtime/record.json") --on-success review -- \
  /bin/sh -c 'printf "candidate\\n" > conflict.txt'
printf 'drifted\n' > "$ROOT/conflict/workspace/conflict.txt"
if sudo "$BIN" commit --record "$ROOT/conflict/runtime/record.json" --confirm; then
  echo "conflict commit unexpectedly succeeded" >&2
  exit 1
fi
sudo "$BIN" rollback --record "$ROOT/conflict/runtime/record.json"
echo "commit conflict refusal: PASS"

# 3. Proxy allow/deny for proxy-aware HTTP clients.
mkdir -p "$ROOT/network/workspace" "$ROOT/network/server"
printf 'proxy-fixture\n' > "$ROOT/network/server/index.html"
make_policy "$ROOT/network/policy.yaml" off enforce
printf '%s\n' '  allow_domains:' '    - 127.0.0.1' >> "$ROOT/network/policy.yaml"
python3 -m http.server 18080 --directory "$ROOT/network/server" > "$ROOT/network/http.log" 2>&1 &
SERVER_PID=$!
sudo env PATH="$PATH" "$BIN" run $(run_args "$ROOT/network/workspace" "$ROOT/network/runtime" "$ROOT/network/policy.yaml" "$ROOT/network/runtime/record.json") --network-backend proxy --on-success review -- \
  /bin/sh -c 'curl --noproxy "" -sS -o allowed.txt -w "%{http_code}\\n" http://127.0.0.1:18080/ > allowed.status; curl --noproxy "" -sS -o denied.txt -w "%{http_code}\\n" http://example.invalid/ > denied.status || true'
test "$(cat "$ROOT/network/runtime/merged/allowed.status")" = 200
test "$(cat "$ROOT/network/runtime/merged/denied.status")" = 403
sudo "$BIN" rollback --record "$ROOT/network/runtime/record.json"
kill "$SERVER_PID" 2>/dev/null || true
SERVER_PID=""
sudo tail -10 "$ROOT/network/runtime/events.jsonl"
sudo grep -q '"operation":"network_connect"' "$ROOT/network/runtime/events.jsonl"
test "$(sudo jq -s '[.[] | select(.operation == "network_connect")] | length' "$ROOT/network/runtime/events.jsonl")" -eq 2
sudo jq -s -e 'any(.[]; .operation == "network_connect" and .decision == "allow")' "$ROOT/network/runtime/events.jsonl" >/dev/null
sudo jq -s -e 'any(.[]; .operation == "network_connect" and .decision == "deny")' "$ROOT/network/runtime/events.jsonl" >/dev/null
echo "proxy network allow/deny: PASS"

# 4. Enforced proxy runs also deny raw/packet socket creation while keeping
# ordinary proxy-aware TCP clients available.
mkdir -p "$ROOT/raw-network/workspace"
make_policy "$ROOT/raw-network/policy.yaml" off enforce
sudo env PATH="$PATH" "$BIN" run $(run_args "$ROOT/raw-network/workspace" "$ROOT/raw-network/runtime" "$ROOT/raw-network/policy.yaml" "$ROOT/raw-network/runtime/record.json") --network-backend proxy --on-success review -- \
  /bin/sh -c '/usr/bin/python3 -c "import socket; socket.socket(socket.AF_INET, socket.SOCK_RAW, socket.IPPROTO_RAW)" && printf "raw-allowed\\n" > raw.status || printf "raw-denied\\n" > raw.status'
test "$(cat "$ROOT/raw-network/runtime/merged/raw.status")" = raw-denied
sudo "$BIN" rollback --record "$ROOT/raw-network/runtime/record.json"
sudo jq -s -e 'any(.[]; .operation == "socket" and .decision == "deny")' "$ROOT/raw-network/runtime/events.jsonl" >/dev/null
echo "raw socket refusal: PASS"

# Audit mode must preserve the real syscall outcome and must not label a raw
# socket as denied when no seccomp defense was requested.
mkdir -p "$ROOT/raw-network-audit/workspace"
make_policy "$ROOT/raw-network-audit/policy.yaml" off audit
sudo env PATH="$PATH" "$BIN" run $(run_args "$ROOT/raw-network-audit/workspace" "$ROOT/raw-network-audit/runtime" "$ROOT/raw-network-audit/policy.yaml" "$ROOT/raw-network-audit/runtime/record.json") --network-backend proxy --on-success review -- \
  /bin/sh -c '/usr/bin/python3 -c "import socket; socket.socket(socket.AF_INET, socket.SOCK_RAW, socket.IPPROTO_RAW)" && printf "raw-allowed\\n" > raw.status || printf "raw-denied\\n" > raw.status'
test -s "$ROOT/raw-network-audit/runtime/merged/raw.status"
sudo jq -s -e 'any(.[]; .operation == "socket" and .decision == "allow")' "$ROOT/raw-network-audit/runtime/events.jsonl" >/dev/null
sudo "$BIN" rollback --record "$ROOT/raw-network-audit/runtime/record.json"
echo "raw socket audit semantics: PASS"

# 5. Bounded evidence must fail verification rather than look complete.
mkdir -p "$ROOT/evidence/workspace"
make_policy "$ROOT/evidence/policy.yaml" off audit
REWIND_EVENT_MAX_BYTES=512 sudo env PATH="$PATH" REWIND_EVENT_MAX_BYTES=512 "$BIN" run $(run_args "$ROOT/evidence/workspace" "$ROOT/evidence/runtime" "$ROOT/evidence/policy.yaml" "$ROOT/evidence/runtime/record.json") -- \
  /bin/sh -c 'for i in $(seq 1 1000); do printf "%s\\n" "$i" > evidence.txt; done' || true
if sudo "$BIN" verify --record "$ROOT/evidence/runtime/record.json"; then
  echo "truncated evidence unexpectedly verified" >&2
  exit 1
fi
echo "incomplete evidence refusal: PASS"

echo "ACCEPTANCE_MATRIX=PASS"
