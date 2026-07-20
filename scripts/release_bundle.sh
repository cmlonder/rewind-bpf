#!/usr/bin/env bash
set -euo pipefail

BIN_DIR="${1:-bin}"
OBJECT="${REWIND_EBPF_OBJECT:-ebpf/rewind_trace.bpf.o}"
POLICY="${REWIND_RELEASE_POLICY:-policies/example.yaml}"
BUNDLE_DIR="$BIN_DIR/rewind-release"

[[ -d "$BIN_DIR" ]] || { echo "release bundle: missing $BIN_DIR" >&2; exit 2; }
[[ -f "$OBJECT" ]] || { echo "release bundle: eBPF object not found at $OBJECT (build it in the Linux VM)" >&2; exit 2; }
[[ -f "$POLICY" ]] || { echo "release bundle: policy example not found at $POLICY" >&2; exit 2; }

rm -rf -- "$BUNDLE_DIR"
mkdir -p "$BUNDLE_DIR"
find "$BIN_DIR" -maxdepth 1 -type f \
  \( -name 'rewind-linux-*' -o -name 'rewind-darwin-*' -o -name 'rewind-windows-*' \) \
  -exec cp -- {} "$BUNDLE_DIR/" \;
cp -- "$OBJECT" "$BUNDLE_DIR/rewind_trace.bpf.o"
cp -- "$POLICY" "$BUNDLE_DIR/policy.example.yaml"

if command -v sha256sum >/dev/null 2>&1; then
  (cd "$BUNDLE_DIR" && find . -maxdepth 1 -type f ! -name SHA256SUMS -print0 | sort -z | xargs -0 sha256sum) > "$BUNDLE_DIR/SHA256SUMS"
else
  (cd "$BUNDLE_DIR" && find . -maxdepth 1 -type f ! -name SHA256SUMS -print0 | sort -z | xargs -0 shasum -a 256) > "$BUNDLE_DIR/SHA256SUMS"
fi
printf 'RELEASE_BUNDLE=PASS path=%s\n' "$BUNDLE_DIR"
