#!/usr/bin/env bash
set -euo pipefail

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "mac native smoke: this script must run on macOS" >&2
  exit 2
fi

ROOT="$(mktemp -d /Users/Shared/rewind-mac-native.XXXXXX)"
trap 'rm -rf "$ROOT"' EXIT
BIN="$ROOT/rewind"
HISTORY="$ROOT/history.json"
GOTOOLCHAIN=local go build -o "$BIN" ./cmd/rewind

mkdir -p "$ROOT/workspace/src"
printf 'original-source\n' > "$ROOT/workspace/src/marker.txt"
printf 'synthetic-secret\n' > "$ROOT/workspace/.env"
cat > "$ROOT/policy.yaml" <<'YAML'
read:
  mode: enforce
  deny:
    - "**/*.env"
write:
  mode: rollback
  scope: workspace
network:
  mode: off
YAML

"$BIN" native run --workspace "$ROOT/workspace" --runtime-root "$ROOT/review-runtime" \
  --policy "$ROOT/policy.yaml" --record "$ROOT/review.json" --history "$HISTORY" --on-success review -- \
  /bin/sh -c 'rm -rf src; printf "created-by-agent\n" > generated.txt'
test ! -e "$ROOT/review-runtime/view/src/marker.txt"
test -f "$ROOT/review-runtime/view/generated.txt"
test -f "$ROOT/workspace/src/marker.txt"
"$BIN" native diff --record "$ROOT/review.json" | grep -q '"kind":"deleted"'
"$BIN" native rollback --record "$ROOT/review.json"
"$BIN" native events --record "$ROOT/review.json" | grep -q '"operation":"exit"'
test -f "$ROOT/workspace/src/marker.txt"
test ! -e "$ROOT/review-runtime"

if "$BIN" native run --workspace "$ROOT/workspace" --runtime-root "$ROOT/pii-runtime" \
  --policy "$ROOT/policy.yaml" --record "$ROOT/pii.json" --history "$HISTORY" --on-success discard -- \
  /bin/sh -c 'cat .env > leaked.txt'; then
  echo "mac native smoke: sensitive read unexpectedly succeeded" >&2
  exit 1
fi
test ! -e "$ROOT/workspace/leaked.txt"
test -f "$ROOT/workspace/.env"
test -s "$ROOT/pii.json.events.jsonl"

if "$BIN" native run --workspace "$ROOT/workspace" --runtime-root "$ROOT/absolute-runtime" \
  --policy "$ROOT/policy.yaml" --record "$ROOT/absolute.json" --history "$HISTORY" --on-success discard -- \
  /bin/sh -c "cat '$ROOT/workspace/.env' > absolute-leaked.txt"; then
  echo "mac native smoke: source workspace was readable through an absolute path" >&2
  exit 1
fi
test ! -e "$ROOT/workspace/absolute-leaked.txt"

"$BIN" native run --workspace "$ROOT/workspace" --runtime-root "$ROOT/commit-runtime" \
  --policy "$ROOT/policy.yaml" --record "$ROOT/commit.json" --history "$HISTORY" --on-success review -- \
  /bin/sh -c 'printf "accepted\n" > src/marker.txt; printf "persisted\n" > generated.txt'
"$BIN" native commit --record "$ROOT/commit.json" --confirm
grep -qx 'accepted' "$ROOT/workspace/src/marker.txt"
grep -qx 'persisted' "$ROOT/workspace/generated.txt"

"$BIN" native run --workspace "$ROOT/workspace" --runtime-root "$ROOT/conflict-runtime" \
  --policy "$ROOT/policy.yaml" --record "$ROOT/conflict.json" --history "$HISTORY" --on-success review -- \
  /bin/sh -c 'printf "candidate\n" > src/marker.txt'
printf 'destination-drift\n' > "$ROOT/workspace/src/marker.txt"
if "$BIN" native commit --record "$ROOT/conflict.json" --confirm; then
  echo "mac native smoke: conflict commit unexpectedly succeeded" >&2
  exit 1
fi
grep -qx 'destination-drift' "$ROOT/workspace/src/marker.txt"
"$BIN" native rollback --record "$ROOT/conflict.json"
grep -q 'apfs-clone-seatbelt' "$HISTORY"

echo "MAC_NATIVE_SMOKE_PASS"
