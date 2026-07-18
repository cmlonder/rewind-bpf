# eBPF component

This directory contains the kernel-space telemetry and optional BPF-LSM read-policy programs. The first implementation uses C + libbpf/CO-RE and emits compact records through a ring buffer. Tracepoints remain telemetry-only. The current VM has Landlock active, so the MVP’s primary read enforcement is the userspace Landlock allowlist; the BPF-LSM program is for kernels that explicitly enable `bpf` in the active LSM list.

Initial observation points:

- process `execve`
- `openat/openat2`
- `unlinkat`
- `renameat2`
- `write`, `pwrite`, `truncate`, `ftruncate`

Programs must filter target agent PIDs/cgroups and emit small event records through a ring buffer. Expensive user policy matching must stay out of the kernel hot path.

## Source layout

- `event.h` — stable numeric ring-buffer ABI shared with userspace.
- `rewind_trace.bpf.c` — tracepoint sensors for process execution, reads/opens, writes, deletes, renames, and truncation.
- `rewind_read_enforcer.bpf.c` — BPF-LSM `file_open` hook with a fixed-size exact-path rule map.
- `Makefile` — disposable Linux VM build commands; it generates `vmlinux.h` from the running kernel BTF.

The eBPF translation unit includes the generated `vmlinux.h` before `event.h`.
This keeps the kernel ABI types sourced from BTF and avoids mixing them with
userspace Linux headers, which can produce duplicate typedefs on ARM64.

## VM-only build

Run these commands only inside the disposable Ubuntu VM. They generate files under this directory and compile an object; they do not load a program or attach a hook:

```bash
cd /path/to/RewindBPF/ebpf
make vmlinux
make compile
make compile-read
```

The commands generate files under this directory and compile objects; they do not load a program or attach a hook. Loading and attaching an object is a separate, privileged safety-gated step. Do not run it on the personal macOS host. The optional read-enforcer requires an active BPF LSM (`bpf` in `/sys/kernel/security/lsm`); a kernel that merely supports the BPF program type is not sufficient.
