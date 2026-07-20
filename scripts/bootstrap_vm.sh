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

# Ubuntu ARM images may expose bpftool through a versioned linux-tools
# package, while the running generic kernel headers are not present in the
# ports archive. Reuse an installed bpftool/BTF when available and do not make
# an unrelated header package a hard prerequisite for CO-RE compilation.
packages=(clang llvm libbpf-dev bpftrace fuse-overlayfs golang-go jq fio ipset iptables iproute2 util-linux)
if ! command -v bpftool >/dev/null 2>&1; then
  packages+=(linux-tools-common)
fi
sudo apt-get install -y "${packages[@]}"
if ! command -v bpftool >/dev/null 2>&1; then
  echo "bootstrap: bpftool is required but was not provided by this Ubuntu image" >&2
  exit 2
fi
if [[ ! -d "/usr/src/linux-headers-$(uname -r)" ]]; then
  echo "bootstrap: running kernel headers unavailable; using /sys/kernel/btf/vmlinux for CO-RE" >&2
fi

echo "bootstrap complete"
echo "next: make build && (cd ebpf && make vmlinux compile)"
