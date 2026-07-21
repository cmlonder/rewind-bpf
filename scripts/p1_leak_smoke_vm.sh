#!/usr/bin/env bash
set -euo pipefail

# Long-running P1 lifecycle smoke. This script is intentionally VM-only: it
# creates real cgroups, FUSE mounts, and (for the namespace case) firewall
# state, then proves that every owned resource is gone after rollback.
if [[ "$(uname -s)" != "Linux" || "${REWIND_VM_CONFIRM:-}" != "VM_ONLY" ]]; then
  echo "set REWIND_VM_CONFIRM=VM_ONLY inside the disposable Ubuntu VM" >&2
  exit 2
fi

ROOT="$(mktemp -d /home/vagrant/rewind-p1-leak.XXXXXX)"
BIN="${REWIND_BIN:-$(pwd)/bin/rewind}"
OBJECT="${REWIND_SENSOR_OBJECT:-$(pwd)/ebpf/rewind_trace.bpf.o}"
BASELINE="$(mktemp)"
cleanup() {
  sudo rm -rf -- "$ROOT" || true
  rm -f -- "$BASELINE"
}
trap cleanup EXIT

sudo find /sys/fs/cgroup/rewind -mindepth 1 -maxdepth 1 -type d -name 'run-*' -print 2>/dev/null | sort > "$BASELINE" || true
mkdir -p "$ROOT/workspace"
cat > "$ROOT/policy.yaml" <<'YAML'
read:
  mode: off
write:
  mode: rollback
  scope: workspace
network:
  mode: enforce
YAML

for iteration in 1 2 3; do
  runtime="$ROOT/runtime-$iteration"
  record="$runtime/record.json"
  mkdir -p "$ROOT/workspace/src"
  printf 'p1-marker-%s\n' "$iteration" > "$ROOT/workspace/src/marker.txt"

  sudo env PATH="$PATH" "$BIN" run \
    --workspace "$ROOT/workspace" \
    --runtime-root "$runtime" \
    --policy "$ROOT/policy.yaml" \
    --record "$record" \
    --sensor-object "$OBJECT" \
    --runtime-roots /bin,/usr/bin,/lib,/usr/lib,/etc \
    --network-backend deny \
    --on-success review -- /bin/sh -c \
    '(sleep 2) & child=$!; printf "child=%s\\n" "$child" > child.status; wait "$child"; printf "iteration=%s\\n" "$child" > complete.status'

  sudo "$BIN" rollback --record "$record"
  ! mountpoint -q "$runtime/merged"
  test -d "$runtime/upper" && test -z "$(find "$runtime/upper" -mindepth 1 -print -quit)"
  test -d "$runtime/work" && test -z "$(find "$runtime/work" -mindepth 1 -print -quit)"
  test ! -e "$runtime/cgroup"
  ! pgrep -af "$ROOT" >/dev/null 2>&1

  current="$(mktemp)"
  sudo find /sys/fs/cgroup/rewind -mindepth 1 -maxdepth 1 -type d -name 'run-*' -print 2>/dev/null | sort > "$current" || true
  if ! diff -u "$BASELINE" "$current"; then
    echo "P1 leak smoke: cgroup scope leaked after iteration $iteration" >&2
    rm -f -- "$current"
    exit 1
  fi
  rm -f -- "$current"
done

if ip link show 2>/dev/null | grep -Eq '\brewind-(host|agent)\b'; then
  echo "P1 leak smoke: owned veth leaked" >&2
  exit 1
fi
if sudo ipset list -name 2>/dev/null | grep -qx 'REWIND_ALLOWLIST4'; then
  echo "P1 leak smoke: owned ipset leaked" >&2
  exit 1
fi
if sudo iptables -S 2>/dev/null | grep -q 'REWIND_ALLOWLIST'; then
  echo "P1 leak smoke: owned iptables chain leaked" >&2
  exit 1
fi

echo "P1_LEAK_SMOKE_PASS iterations=3"
