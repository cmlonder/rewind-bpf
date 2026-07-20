#!/usr/bin/env bash
set -euo pipefail

ROOT="${1:-benchmarks}"
CSV="$ROOT/results_summary.csv"
CHART="$ROOT/results_chart.svg"
if [[ ! -s "$CSV" || ! -s "$CHART" ]]; then
  echo "benchmark verification: missing CSV or chart under $ROOT" >&2
  exit 1
fi

for variant in B0-native-ext4 B2-fuse-only B4-rewind-protected B5-telemetry-only; do
  if ! awk -F, -v wanted="$variant" 'NR > 1 && $1 == wanted { found = 1 } END { exit(found ? 0 : 1) }' "$CSV"; then
    echo "benchmark verification: missing variant $variant" >&2
    exit 1
  fi
done

header="$(head -n 1 "$CSV")"
for field in read_iops write_iops upper_bytes telemetry_bytes event_count; do
  case ",$header," in
    *,"$field",*) ;;
    *) echo "benchmark verification: missing column $field" >&2; exit 1 ;;
  esac
done

echo "BENCHMARK_LEDGER=PASS variants=B0,B2,B4,B5 chart=present"
