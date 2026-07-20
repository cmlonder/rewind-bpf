#!/usr/bin/env bash
set -euo pipefail

# One-command final gate for the disposable Ubuntu VM. It installs only VM
# dependencies, rebuilds the binary and CO-RE object, creates the release
# bundle, runs the full acceptance matrix, and executes the deterministic jury
# scenario. It refuses to run on macOS, Windows, or an unconfirmed host.
if [[ "$(uname -s)" != "Linux" || "${REWIND_VM_CONFIRM:-}" != "VM_ONLY" ]]; then
  echo "set REWIND_VM_CONFIRM=VM_ONLY inside the disposable Ubuntu VM" >&2
  exit 2
fi
if [[ ! -r /etc/os-release ]] || ! grep -q 'ID=ubuntu' /etc/os-release; then
  echo "final VM gate: Ubuntu is required" >&2
  exit 2
fi

ROOT="$(pwd)"
BIN="$ROOT/bin/rewind"
OBJECT="$ROOT/ebpf/rewind_trace.bpf.o"

./scripts/bootstrap_vm.sh
make release
make build
(cd ebpf && make vmlinux compile)
REWIND_EBPF_OBJECT="$OBJECT" make release-bundle
(cd "$ROOT/bin/rewind-release" && sha256sum -c SHA256SUMS)
test -s "$ROOT/bin/rewind-release/rewind_trace.bpf.o"
test -s "$ROOT/bin/rewind-release/policy.example.yaml"
make benchmark-verify
REWIND_BIN="$BIN" REWIND_SENSOR_OBJECT="$OBJECT" REWIND_VM_CONFIRM=VM_ONLY ./scripts/acceptance_vm.sh
REWIND_BIN="$BIN" REWIND_SENSOR_OBJECT="$OBJECT" REWIND_DEMO_CONFIRM=VM_ONLY ./scripts/jury_demo_vm.sh

echo "FINAL_VM_GATE=PASS"
echo "release bundle: $ROOT/bin/rewind-release"
