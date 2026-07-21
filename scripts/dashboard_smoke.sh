#!/bin/sh
set -eu

# Safe, disposable verification for the one-command local experience. It never
# points Rewind at the repository itself and is intentionally limited to macOS
# until the Linux VM launcher gets its own privileged acceptance wrapper.
if [ "$(uname -s)" != "Darwin" ]; then
  echo "DASHBOARD_SMOKE_SKIP: macOS native dashboard smoke is Darwin-only"
  exit 0
fi

ROOT="$(mktemp -d /Users/Shared/rewind-dashboard-smoke.XXXXXX)"
BIN="$ROOT/rewind"
cleanup() { rm -rf "$ROOT"; }
trap cleanup EXIT INT TERM

GOTOOLCHAIN=local go build -o "$BIN" ./cmd/rewind
mkdir -p "$ROOT/workspace/src"
printf '%s\n' 'original-source' > "$ROOT/workspace/src/marker.txt"
printf '%s\n' 'SYNTHETIC_ONLY=true' > "$ROOT/workspace/.env"

printf '%s\n' \
  'rm -rf src' \
  'printf "created-by-dashboard\\n" > generated.txt' \
  'exit' \
  | "$BIN" dashboard start \
      --workspace "$ROOT/workspace" \
      --state-dir "$ROOT/state" \
      --ui-dir "$PWD/ui" \
      --no-open \
      --exit-after-shell \
      --shell /bin/sh

test -f "$ROOT/workspace/src/marker.txt"
test ! -e "$ROOT/workspace/generated.txt"
grep -q '"backend": "apfs-clone-seatbelt"' "$ROOT/state/history.json"
grep -q '"state": "succeeded"' "$ROOT/state/history.json"
echo "DASHBOARD_SMOKE_PASS"
