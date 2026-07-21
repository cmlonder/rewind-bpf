#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT="${REWIND_RELEASE_OUT:-$ROOT/dist/rewind-release-preflight}"
cd "$ROOT"

rm -rf -- "$OUT"
mkdir -p "$OUT"

GOFLAGS="${GOFLAGS:-}"
GO_BUILD_FLAGS=(-trimpath -ldflags=-s\ -w)
GOTOOLCHAIN=local GOOS=linux GOARCH=amd64 go build "${GO_BUILD_FLAGS[@]}" -o "$OUT/rewind-linux-amd64" ./cmd/rewind
GOTOOLCHAIN=local GOOS=linux GOARCH=arm64 go build "${GO_BUILD_FLAGS[@]}" -o "$OUT/rewind-linux-arm64" ./cmd/rewind
GOTOOLCHAIN=local GOOS=darwin GOARCH=arm64 go build "${GO_BUILD_FLAGS[@]}" -o "$OUT/rewind-darwin-arm64" ./cmd/rewind
GOTOOLCHAIN=local GOOS=windows GOARCH=amd64 go build "${GO_BUILD_FLAGS[@]}" -o "$OUT/rewind-windows-amd64.exe" ./cmd/rewind

cp -- policies/example.yaml "$OUT/policy.example.yaml"
cp -- README.md docs/platform/README.md docs/platform/macos_manual_e2e.md "$OUT/"

if [[ -f ebpf/rewind_trace.bpf.o ]]; then
  cp -- ebpf/rewind_trace.bpf.o "$OUT/rewind_trace.bpf.o"
  ebpf_status="included"
else
  ebpf_status="vm-only-missing-build-in-disposable-ubuntu-vm"
fi

if command -v shasum >/dev/null 2>&1; then
  (cd "$OUT" && shasum -a 256 $(find . -maxdepth 1 -type f ! -name SHA256SUMS -exec basename {} \; | sort)) > "$OUT/SHA256SUMS"
else
  (cd "$OUT" && sha256sum $(find . -maxdepth 1 -type f ! -name SHA256SUMS -exec basename {} \; | sort)) > "$OUT/SHA256SUMS"
fi

cat > "$OUT/release-metadata.txt" <<EOF
schema_version=1
created_at_utc=$(date -u '+%Y-%m-%dT%H:%M:%SZ')
git_revision=$(git rev-parse HEAD 2>/dev/null || printf unknown)
platforms=linux/amd64,linux/arm64,darwin/arm64,windows/amd64
ebpf_object_status=$ebpf_status
signing_status=unsigned-checksum-only
manual_gates=EndpointSecurity entitlement,Windows signed minifilter,VHDX acceptance
EOF

printf 'RELEASE_PREFLIGHT=PASS output=%s ebpf=%s\n' "$OUT" "$ebpf_status"
