#!/usr/bin/env bash
set -euo pipefail

# This script is intentionally Linux/Ubuntu-only. It installs development
# dependencies inside the disposable VM; it never touches a host workspace.
if [[ "$(uname -s)" != "Linux" ]]; then
  echo "rewind bootstrap: run this inside the disposable Ubuntu VM" >&2
  exit 2
fi
if [[ ! -r /etc/os-release ]] || ! grep -q 'ID=ubuntu' /etc/os-release; then
  echo "rewind bootstrap: Ubuntu is required" >&2
  exit 2
fi

sudo apt-get update
sudo apt-get install -y clang llvm libbpf-dev bpftool bpftrace fuse-overlayfs golang-go jq fio linux-headers-$(uname -r)

echo "bootstrap complete"
echo "next: make build && (cd ebpf && make vmlinux compile)"
