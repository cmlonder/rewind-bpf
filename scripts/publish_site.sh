#!/usr/bin/env bash
set -euo pipefail

SITE_DIR="${1:-site}"
DESTINATION="${REWIND_SITE_DEST:-}"

if [[ ! -f "$SITE_DIR/index.html" ]]; then
  echo "site publish: expected $SITE_DIR/index.html" >&2
  exit 2
fi
if [[ -z "$DESTINATION" ]]; then
  echo "site publish: set REWIND_SITE_DEST to an explicit directory or s3:// URL" >&2
  exit 2
fi

if [[ "$DESTINATION" == s3://* ]]; then
  command -v aws >/dev/null 2>&1 || { echo "site publish: aws CLI is required for s3:// destinations" >&2; exit 2; }
  aws s3 sync "$SITE_DIR" "$DESTINATION" --delete --only-show-errors
else
  mkdir -p "$DESTINATION"
  if command -v rsync >/dev/null 2>&1; then
    rsync -a --delete "$SITE_DIR/" "$DESTINATION/"
  else
    find "$DESTINATION" -mindepth 1 -maxdepth 1 -exec rm -rf -- {} +
    cp -R "$SITE_DIR/." "$DESTINATION/"
  fi
fi

printf 'SITE_PUBLISH=PASS destination=%s\n' "$DESTINATION"
