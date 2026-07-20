#!/usr/bin/env bash
set -euo pipefail

OUT_DIR="${1:-bin}"
if [[ ! -d "$OUT_DIR" ]]; then
  echo "release manifest: directory not found: $OUT_DIR" >&2
  exit 2
fi

if command -v sha256sum >/dev/null 2>&1; then
  HASH_CMD=(sha256sum)
elif command -v shasum >/dev/null 2>&1; then
  HASH_CMD=(shasum -a 256)
else
  echo "release manifest: sha256sum or shasum is required" >&2
  exit 2
fi

artifacts=()
artifact_names=()
while IFS= read -r artifact; do
	artifacts+=("$artifact")
	artifact_names+=("${artifact##*/}")
done < <(find "$OUT_DIR" -maxdepth 1 -type f \
  \( -name 'rewind-linux-*' -o -name 'rewind-darwin-*' -o -name 'rewind-windows-*' \) \
  -print | sort)
if [[ "${#artifacts[@]}" -eq 0 ]]; then
  echo "release manifest: no release binaries found in $OUT_DIR" >&2
  exit 2
fi

checksum_path="$OUT_DIR/SHA256SUMS"
metadata_path="$OUT_DIR/release-metadata.txt"
(cd "$OUT_DIR" && "${HASH_CMD[@]}" "${artifact_names[@]}") > "$checksum_path"

{
  printf 'schema_version=1\n'
  printf 'git_revision=%s\n' "$(git rev-parse HEAD 2>/dev/null || printf unknown)"
  printf 'created_at_utc=%s\n' "$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
  printf 'artifacts=%s\n' "${#artifacts[@]}"
  printf 'checksum_file=SHA256SUMS\n'
  printf 'signing_status=unsigned-checksum-only\n'
  printf 'signing_note=SHA-256 detects accidental or post-build changes; use release-sign with a protected Ed25519 key before public distribution.\n'
} > "$metadata_path"

printf 'wrote %s and %s\n' "$checksum_path" "$metadata_path"
