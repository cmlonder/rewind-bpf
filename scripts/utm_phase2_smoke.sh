#!/usr/bin/env bash
set -euo pipefail

ROOT="$(mktemp -d /home/vagrant/rewind-phase2.XXXXXX)"
mkdir -p "$ROOT/workspace"
printf 'original-source\n' > "$ROOT/workspace/marker.txt"
printf '%s\n' \
  'read:' '  mode: audit' '  pii:' '    mode: audit' '' \
  'write:' '  mode: rollback' '  scope: workspace' '' \
  'network:' '  mode: audit' > "$ROOT/policy.yaml"

/tmp/rewind run \
  --workspace "$ROOT/workspace" \
  --runtime-root "$ROOT/runtime" \
  --policy "$ROOT/policy.yaml" \
  --record "$ROOT/runtime/record.json" \
  --checkpoint-graph "$ROOT/graph.json" \
  --checkpoint-id root \
  --on-success review \
  --overlay-backend fuse \
  -- /bin/sh -c 'prefix="ghp_"; suffix="synthetic_1234567890abcdef"; printf "github token %s%s\n" "$prefix" "$suffix" > generated.env; rm -f marker.txt'

/tmp/rewind status --record "$ROOT/runtime/record.json"
/tmp/rewind checkpoint graph inspect --path "$ROOT/graph.json"
jq '.plan.pii_findings' "$ROOT/runtime/record.json"
test ! -e "$ROOT/runtime/merged/marker.txt"
test -f "$ROOT/runtime/merged/generated.env"
/tmp/rewind rollback --record "$ROOT/runtime/record.json"
test -f "$ROOT/workspace/marker.txt"
printf 'CHECKPOINT_PII_VM_PASS\n'

printf '%s\n' \
  'read:' '  mode: enforce' '' \
  'write:' '  mode: rollback' '  scope: workspace' '' \
  'network:' '  mode: enforce' '  allow_domains:' '    - example.com' > "$ROOT/ns-policy.yaml"
if /tmp/rewind run --workspace "$ROOT/workspace" --runtime-root "$ROOT/ns-runtime" \
  --policy "$ROOT/ns-policy.yaml" --record "$ROOT/ns-runtime/record.json" \
  --network-backend namespace --overlay-backend fuse -- /bin/true; then
  echo 'NS_UNEXPECTED_SUCCESS'
  exit 1
else
  echo 'NAMESPACE_ALLOWLIST_FAIL_CLOSED_PASS'
fi
