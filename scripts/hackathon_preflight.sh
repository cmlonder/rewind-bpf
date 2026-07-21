#!/usr/bin/env bash
set -euo pipefail

# Safe host-side release checklist. It never mounts filesystems, loads eBPF,
# changes firewall state, or runs destructive agent commands. Privileged Linux
# acceptance remains the separate `make final-vm` command inside UTM.
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

run() {
  echo "--- $* ---"
  "$@"
}

run go test ./...
run go vet ./...
run bash -n scripts/*.sh
run make ui-smoke
run make site-smoke
run make benchmark-verify
run make release-preflight
run make evidence-bundle

test -s README.md
test -s docs/FEATURE_BACKLOG.md
test -s docs/PHASE2_PLAN.md
test -s site/index.html
find dist -maxdepth 1 -type f -name 'evidence-*.tar.gz' -size +0c -print -quit | grep -q .

echo "HACKATHON_PREFLIGHT=PASS"
echo "next: inside the disposable Ubuntu VM run REWIND_VM_CONFIRM=VM_ONLY make final-vm"
