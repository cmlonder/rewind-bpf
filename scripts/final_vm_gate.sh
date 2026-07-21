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
STAMP="$(date -u '+%Y%m%dT%H%M%SZ')"
OUT="${REWIND_FINAL_OUT:-$ROOT/dist/final-vm-$STAMP}"
LOGS="$OUT/logs"
mkdir -p "$LOGS"

for command in go make clang bpftool fuse-overlayfs jq curl python3 sha256sum; do
  command -v "$command" >/dev/null 2>&1 || { echo "final VM gate: missing command $command" >&2; exit 2; }
done

run_logged() {
  local name="$1"
  shift
  echo "--- $name ---"
  "$@" 2>&1 | tee "$LOGS/$name.log"
}

run_logged bootstrap ./scripts/bootstrap_vm.sh
run_logged release make release
run_logged build make build
run_logged ebpf-build bash -c 'cd ebpf && make vmlinux compile'
run_logged release-bundle env REWIND_EBPF_OBJECT="$OBJECT" make release-bundle
run_logged release-check bash -c "cd '$ROOT/bin/rewind-release' && sha256sum -c SHA256SUMS"
test -s "$ROOT/bin/rewind-release/rewind_trace.bpf.o"
test -s "$ROOT/bin/rewind-release/policy.example.yaml"
run_logged benchmark make benchmark-verify
run_logged acceptance env REWIND_BIN="$BIN" REWIND_SENSOR_OBJECT="$OBJECT" REWIND_VM_CONFIRM=VM_ONLY ./scripts/acceptance_vm.sh
run_logged p1-leak env REWIND_BIN="$BIN" REWIND_SENSOR_OBJECT="$OBJECT" REWIND_VM_CONFIRM=VM_ONLY ./scripts/p1_leak_smoke_vm.sh
run_logged jury-demo env REWIND_BIN="$BIN" REWIND_SENSOR_OBJECT="$OBJECT" REWIND_DEMO_CONFIRM=VM_ONLY ./scripts/jury_demo_vm.sh

cp -- benchmarks/results_summary.csv benchmarks/results_normalized.csv benchmarks/results_chart.svg "$OUT/"
cp -- "$ROOT/bin/rewind-release/SHA256SUMS" "$OUT/release-SHA256SUMS"
cp -- "$ROOT/bin/release-metadata.txt" "$OUT/release-metadata.txt"
cat > "$OUT/manifest.txt" <<EOF
schema_version=1
created_at_utc=$STAMP
git_revision=$(git rev-parse HEAD 2>/dev/null || printf unknown)
acceptance=ACCEPTANCE_MATRIX=PASS
jury_demo=JURY_DEMO_VM_PASS
release_bundle=$ROOT/bin/rewind-release
logs=logs/
EOF
(cd "$OUT" && sha256sum $(find . -type f ! -name SHA256SUMS -print | sort)) > "$OUT/SHA256SUMS"
ARCHIVE="$OUT.tar.gz"
tar -czf "$ARCHIVE" -C "$(dirname "$OUT")" "$(basename "$OUT")"

echo "FINAL_VM_GATE=PASS"
echo "release bundle: $ROOT/bin/rewind-release"
echo "evidence directory: $OUT"
echo "evidence archive: $ARCHIVE"
