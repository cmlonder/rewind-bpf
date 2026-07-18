# eBPF component

This directory contains the kernel-space telemetry program. The first implementation uses C + libbpf/CO-RE and emits compact records through a ring buffer. It is telemetry-only for now: deny decisions will be added through a separate BPF-LSM policy program after the event path is verified.

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
- `Makefile` — disposable Linux VM build commands; it generates `vmlinux.h` from the running kernel BTF.

## VM-only build

Run these commands only inside the disposable Ubuntu VM. They generate files under this directory and compile an object; they do not load a program or attach a hook:

```bash
cd /path/to/RewindBPF/ebpf
make vmlinux
make compile
```

Loading and attaching the object is a separate, privileged safety-gated step. Do not run it on the personal macOS host.
