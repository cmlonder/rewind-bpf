#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
STAMP="$(date -u '+%Y%m%dT%H%M%SZ')"
OUT="${REWIND_EVIDENCE_OUT:-$ROOT/dist/evidence-$STAMP}"
LOGS="$OUT/logs"
cd "$ROOT"

rm -rf -- "$OUT"
mkdir -p "$LOGS" "$OUT/benchmarks" "$OUT/docs"

run_logged() {
  local name="$1"
  shift
  "$@" >"$LOGS/$name.log" 2>&1
}

run_logged ui-smoke make ui-smoke
run_logged site-smoke make site-smoke
if [[ "$(uname -s)" == "Darwin" ]]; then
  run_logged mac-safe-smoke make mac-safe-smoke
  run_logged mac-native-smoke make mac-native-smoke
  run_logged mac-crash-smoke make mac-crash-smoke
else
  printf 'macOS smoke tests not run on %s\n' "$(uname -s)" > "$LOGS/macos-not-run.log"
fi

python3 benchmarks/normalize_results.py > "$LOGS/benchmark-normalize.log" 2>&1
python3 benchmarks/plot_results.py > "$LOGS/benchmark-plot.log" 2>&1
./scripts/benchmark_verify.sh benchmarks > "$LOGS/benchmark-verify.log" 2>&1

cp -- benchmarks/results_summary.csv benchmarks/results_normalized.csv benchmarks/results_chart.svg "$OUT/benchmarks/"
cp -- benchmarks/RESULTS.md benchmarks/README.md benchmarks/COMPETITOR_MATRIX.md "$OUT/benchmarks/"
cp -- docs/ARCHITECTURE.md docs/FEATURE_BACKLOG.md docs/PLATFORM_STATUS.md docs/platform/macos_manual_e2e.md "$OUT/docs/"
GOTOOLCHAIN=local go run ./cmd/rewind platform status > "$OUT/platform-status.json"

if command -v shasum >/dev/null 2>&1; then
  (cd "$OUT" && shasum -a 256 $(find . -type f ! -name SHA256SUMS -print | sort)) > "$OUT/SHA256SUMS"
else
  (cd "$OUT" && sha256sum $(find . -type f ! -name SHA256SUMS -print | sort)) > "$OUT/SHA256SUMS"
fi

cat > "$OUT/manifest.txt" <<EOF
schema_version=1
created_at_utc=$STAMP
git_revision=$(git rev-parse HEAD 2>/dev/null || printf unknown)
platform=$(uname -s)-$(uname -m)
benchmark_source=benchmarks/results_summary.csv
macos_native_e2e=see logs/mac-native-smoke.log
macos_crash_recovery=see logs/mac-crash-smoke.log
ui_validation=see logs/ui-smoke.log
checksum_file=SHA256SUMS
EOF

ARCHIVE="$OUT.tar.gz"
tar -czf "$ARCHIVE" -C "$(dirname "$OUT")" "$(basename "$OUT")"
printf 'FINAL_EVIDENCE_BUNDLE=PASS directory=%s archive=%s\n' "$OUT" "$ARCHIVE"
