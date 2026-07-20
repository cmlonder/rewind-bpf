#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 2 ]]; then
  echo "usage: mark_release_signed.sh METADATA_PATH SIGNATURE_FILE" >&2
  exit 2
fi

metadata="$1"
signature_file="$2"
if [[ ! -f "$metadata" ]]; then
  echo "release metadata: file not found: $metadata" >&2
  exit 1
fi

tmp="${metadata}.tmp.$$"
trap 'rm -f "$tmp"' EXIT
awk -v signature_file="$signature_file" '
  /^signing_status=/ { next }
  /^signing_note=/ {
    print
    print "signature_file=" signature_file
    print "signing_status=ed25519-detached"
    next
  }
  { print }
' "$metadata" > "$tmp"
chmod 0644 "$tmp"
mv "$tmp" "$metadata"
trap - EXIT
