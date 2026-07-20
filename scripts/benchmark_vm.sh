#!/usr/bin/env bash
set -euo pipefail

if [[ "$(uname -s)" != "Linux" ]]; then
  echo "rewind benchmark: run only inside the disposable Linux VM" >&2
  exit 2
fi
if [[ "${REWIND_BENCH_CONFIRM:-}" != "VM_ONLY" ]]; then
  echo "set REWIND_BENCH_CONFIRM=VM_ONLY to run filesystem benchmarks" >&2
  exit 2
fi

ROOT="${REWIND_BENCH_ROOT:-$(mktemp -d /tmp/rewind-benchmark.XXXXXX)}"
mkdir -p "$ROOT"
echo "benchmark root: $ROOT"
echo "Use the B0/B2/B4 commands in benchmarks/README.md and save raw JSON under this root."
echo "This wrapper intentionally does not mount or delete anything automatically."
