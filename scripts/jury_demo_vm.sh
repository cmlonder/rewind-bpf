#!/usr/bin/env bash
set -euo pipefail

if [[ "$(uname -s)" != "Linux" || "${REWIND_DEMO_CONFIRM:-}" != "VM_ONLY" ]]; then
  echo "set REWIND_DEMO_CONFIRM=VM_ONLY inside the disposable Linux VM" >&2
  exit 2
fi

BIN="${REWIND_BIN:-$(pwd)/bin/rewind}"
OBJECT="${REWIND_SENSOR_OBJECT:-$(pwd)/ebpf/rewind_trace.bpf.o}"
ROOT="${REWIND_DEMO_ROOT:-$(mktemp -d /tmp/rewind-jury-demo.XXXXXX)}"
mkdir -p "$ROOT/workspace/src"
cleanup() { if [[ -z "${REWIND_DEMO_ROOT:-}" ]]; then sudo rm -rf -- "$ROOT"; fi; }
trap cleanup EXIT
printf 'original-source\n' > "$ROOT/workspace/src/marker.txt"
printf 'SYNTHETIC_ONLY=true\n' > "$ROOT/workspace/synthetic.env"
printf '%s\n' 'read:' '  mode: enforce' '  deny:' '    - "**/*.env"' '' 'write:' '  mode: rollback' '  scope: workspace' '' 'network:' '  mode: audit' > "$ROOT/policy.yaml"

sudo "$BIN" run --workspace "$ROOT/workspace" --runtime-root "$ROOT/runtime" --policy "$ROOT/policy.yaml" --record "$ROOT/runtime/record.json" --sensor-object "$OBJECT" --runtime-roots /bin,/usr/bin,/lib,/usr/lib,/etc --overlay-backend fuse --on-success review -- /bin/sh -c 'cat synthetic.env >/tmp/rewind-secret-read 2>/tmp/rewind-secret-read.err || true; rm -rf src; printf "created-by-agent\n" > generated.txt'
echo '--- merged view after agent ---'
test ! -e "$ROOT/runtime/merged/src" && echo 'deleted src is isolated in upper layer'
cat "$ROOT/runtime/merged/generated.txt"
grep -q 'Permission denied' /tmp/rewind-secret-read.err && echo 'sensitive read denied'
sudo "$BIN" rollback --record "$ROOT/runtime/record.json"
test -f "$ROOT/workspace/src/marker.txt"
test ! -e "$ROOT/workspace/generated.txt"
echo 'JURY_DEMO_VM_PASS'
