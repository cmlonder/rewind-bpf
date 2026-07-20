#!/usr/bin/env bash
set -euo pipefail

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "mac safe smoke: this script must run on macOS" >&2
  exit 2
fi

ROOT="$(mktemp -d "${TMPDIR:-/tmp}/rewind-mac-safe.XXXXXX")"
trap 'rm -rf "$ROOT"' EXIT
mkdir -p "$ROOT/workspace"
printf 'contact alice@example.com\n' > "$ROOT/workspace/notes.txt"
printf '%s\n' 'read:' '  mode: off' '' 'write:' '  mode: rollback' '  scope: workspace' '' 'network:' '  mode: off' > "$ROOT/policy.yaml"

GOTOOLCHAIN=local go test ./internal/agent ./internal/pii ./internal/platform ./internal/registry ./internal/session ./internal/runplan
GOTOOLCHAIN=local go run ./cmd/rewind platform contract --platform darwin --workspace "$ROOT/workspace" | tee "$ROOT/native-contract.json"
GOTOOLCHAIN=local go run ./cmd/rewind platform contract --platform windows --workspace "$ROOT/workspace" | tee "$ROOT/windows-contract.json"
GOTOOLCHAIN=local go run ./cmd/rewind pii scan --path "$ROOT/workspace" --output "$ROOT/pii.json"
test -s "$ROOT/pii.json"
if GOTOOLCHAIN=local go run ./cmd/rewind run --workspace "$ROOT/workspace" --runtime-root "$ROOT/runtime" --policy "$ROOT/policy.yaml" --record "$ROOT/runtime/record.json" -- /bin/true 2>"$ROOT/linux-run.err"; then
  echo "mac safe smoke: unexpected Linux protected-run success" >&2
  exit 1
fi
grep -Eq 'Linux-only|OverlayFS|unsupported|cgroup|runtime' "$ROOT/linux-run.err" || { cat "$ROOT/linux-run.err" >&2; exit 1; }
echo "MAC_SAFE_SMOKE_PASS"
