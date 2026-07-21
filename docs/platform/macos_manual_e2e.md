# macOS Native Manual E2E Runbook

This runbook reproduces the macOS native staged lifecycle without touching a
real repository. Every test uses a temporary fixture under `/Users/Shared` and
stores the record outside the disposable runtime directory.

## Safety boundary

- Do not replace `ROOT/workspace` with a real project path.
- Do not reuse a runtime directory from a previous run. The CLI intentionally
  refuses to overwrite a non-empty runtime root.
- The test exercises APFS clone staging and Seatbelt process isolation. It does
  not install an EndpointSecurity entitlement, change network settings, or
  run against personal files.
- The runtime directory is disposable; the JSON record and its event sidecar
  are durable evidence and remain after rollback/discard.

## Prerequisites

```bash
cd /path/to/RewindBPF

GOTOOLCHAIN=local go test ./...
GOTOOLCHAIN=local go build -o /tmp/rewind-darwin ./cmd/rewind

/tmp/rewind-darwin platform plan --workspace /Users/Shared
```

The platform plan should report `filesystem: apfs`, `ready: true`, and
`enforcement_ready: false`. The last value is expected until a signed
EndpointSecurity helper is installed.

## Create a disposable fixture

```bash
export BIN=/tmp/rewind-darwin
export ROOT="$(mktemp -d /Users/Shared/rewind-manual.XXXXXX)"

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
```

## Review and rollback

Run a synthetic destructive command in review mode:

```bash
"$BIN" native run \
  --workspace "$ROOT/workspace" \
  --runtime-root "$ROOT/review-runtime" \
  --policy "$ROOT/policy.yaml" \
  --record "$ROOT/review.json" \
  --on-success review \
  -- /bin/sh -c \
  'rm -rf src; printf "created-by-agent\n" > generated.txt'
```

The merged view should show the deletion and new file, while the source
workspace remains intact:

```bash
test ! -e "$ROOT/review-runtime/view/src/marker.txt"
cat "$ROOT/review-runtime/view/generated.txt"
cat "$ROOT/workspace/src/marker.txt"

"$BIN" native diff --record "$ROOT/review.json"
"$BIN" native events --record "$ROOT/review.json"
```

Expected evidence includes a created `generated.txt`, deleted `src`, a denied
`.env` read-policy event, and an `exit` event with decision `review`.

Discard the candidate:

```bash
"$BIN" native rollback --record "$ROOT/review.json"

test -f "$ROOT/workspace/src/marker.txt"
test ! -e "$ROOT/workspace/generated.txt"
test ! -e "$ROOT/review-runtime"
grep -qx 'original-source' "$ROOT/workspace/src/marker.txt"
```

The event sidecar remains available at `$ROOT/review.json.events.jsonl` and
contains a final `rollback` event.

## Sensitive-read denial

Use a fresh runtime and record:

```bash
PII_RUNTIME="$ROOT/pii-runtime"
PII_RECORD="$ROOT/pii.json"

if "$BIN" native run \
  --workspace "$ROOT/workspace" \
  --runtime-root "$PII_RUNTIME" \
  --policy "$ROOT/policy.yaml" \
  --record "$PII_RECORD" \
  --on-success discard \
  -- /bin/sh -c 'cat .env > leaked.txt'
then
  echo "sensitive read unexpectedly succeeded"
  exit 1
else
  echo "sensitive read denied as expected"
fi

test ! -e "$ROOT/workspace/leaked.txt"
test -f "$ROOT/workspace/.env"
grep '"operation":"read_policy"' "$PII_RECORD.events.jsonl"
```

The child exits non-zero, no leaked file is created, and the sidecar records a
`read_policy` denial for the staged `.env` path.

## Conflict-checked commit

First accept a reviewed candidate:

```bash
COMMIT_RUNTIME="$ROOT/commit-runtime"
COMMIT_RECORD="$ROOT/commit.json"

"$BIN" native run \
  --workspace "$ROOT/workspace" \
  --runtime-root "$COMMIT_RUNTIME" \
  --policy "$ROOT/policy.yaml" \
  --record "$COMMIT_RECORD" \
  --on-success review \
  -- /bin/sh -c \
  'printf "accepted\n" > src/marker.txt; printf "persisted\n" > generated.txt'

"$BIN" native diff --record "$COMMIT_RECORD"
"$BIN" native commit --record "$COMMIT_RECORD" --confirm

grep -qx 'accepted' "$ROOT/workspace/src/marker.txt"
grep -qx 'persisted' "$ROOT/workspace/generated.txt"
test ! -e "$COMMIT_RUNTIME"
```

Then prove destination drift is refused:

```bash
CONFLICT_RUNTIME="$ROOT/conflict-runtime"
CONFLICT_RECORD="$ROOT/conflict.json"

"$BIN" native run \
  --workspace "$ROOT/workspace" \
  --runtime-root "$CONFLICT_RUNTIME" \
  --policy "$ROOT/policy.yaml" \
  --record "$CONFLICT_RECORD" \
  --on-success review \
  -- /bin/sh -c 'printf "candidate\n" > src/marker.txt'

printf 'destination-drift\n' > "$ROOT/workspace/src/marker.txt"

if "$BIN" native commit --record "$CONFLICT_RECORD" --confirm; then
  echo "conflict commit unexpectedly succeeded"
  exit 1
else
  echo "conflict refused as expected"
fi

grep -qx 'destination-drift' "$ROOT/workspace/src/marker.txt"
"$BIN" native rollback --record "$CONFLICT_RECORD"
test ! -e "$CONFLICT_RUNTIME"
```

The commit must refuse with a conflict for `src/marker.txt`; the external
destination value must remain unchanged. The record sidecar should include a
`commit_attempt` followed by `rollback`.

## Automated repeat

The same synthetic lifecycle is covered by the repository smoke targets:

```bash
make mac-safe-smoke
make mac-native-smoke
make mac-crash-smoke
make ui-smoke
```

Expected terminal markers are `MAC_SAFE_SMOKE_PASS`, `MAC_NATIVE_SMOKE_PASS`,
`MACOS_CRASH_SMOKE_PASS`, and `UI_SMOKE_PASS`.

## Current boundary

The tested macOS path provides APFS clone staging, Seatbelt launch, policy
denied sensitive-read hiding, review/diff/rollback/commit, durable evidence,
and destination conflict checks. EndpointSecurity telemetry, network and
resource enforcement, signed helper installation, and crash/power-loss
acceptance on disposable APFS storage remain explicit manual gates.
