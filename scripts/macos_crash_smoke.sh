#!/usr/bin/env bash
set -euo pipefail

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "macOS crash smoke: this script must run on macOS" >&2
  exit 2
fi

ROOT="$(mktemp -d /Users/Shared/rewind-mac-crash.XXXXXX)"
trap 'rm -rf -- "$ROOT"' EXIT
BIN="$ROOT/rewind"
GOTOOLCHAIN=local go build -o "$BIN" ./cmd/rewind

mkdir -p "$ROOT/workspace/src"
printf 'crash-safe-original\n' > "$ROOT/workspace/src/marker.txt"
printf '%s\n' 'read:' '  mode: off' '' 'write:' '  mode: rollback' '  scope: workspace' '' 'network:' '  mode: off' > "$ROOT/policy.yaml"

RECORD="$ROOT/crash.json"
RUNTIME="$ROOT/crash-runtime"
if "$BIN" native run \
  --workspace "$ROOT/workspace" \
  --runtime-root "$RUNTIME" \
  --policy "$ROOT/policy.yaml" \
  --record "$RECORD" \
  --on-success review \
  -- /bin/sh -c 'printf "candidate-before-crash\n" > crash.txt; kill -9 $$'
then
  echo "macOS crash smoke: killed agent unexpectedly returned success" >&2
  exit 1
else
  echo "agent termination converted to failed/rollback path"
fi

test -f "$ROOT/workspace/src/marker.txt"
test ! -e "$ROOT/workspace/crash.txt"
test ! -e "$RUNTIME"
test -s "$RECORD.events.jsonl"
grep -q '"operation":"exit"' "$RECORD.events.jsonl"
grep -q '"decision":"rollback"' "$RECORD.events.jsonl"

echo "MACOS_CRASH_SMOKE_PASS"
