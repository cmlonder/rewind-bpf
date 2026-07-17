# eBPF component

This directory will contain kernel-space programs. The first implementation will use C + libbpf/CO-RE.

Initial observation points:

- process `execve`
- `openat/openat2`
- `unlinkat`
- `renameat2`
- `write`, `pwrite`, `truncate`, `ftruncate`

Programs must filter target agent PIDs/cgroups and emit small event records through a ring buffer. Expensive user policy matching must stay out of the kernel hot path.
