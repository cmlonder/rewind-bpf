# eBPF component

Bu dizin kernel-space programlarını barındırır. Planlanan ilk program C + libbpf/CO-RE ile yazılacaktır.

İlk gözlem noktaları:

- process `execve`
- `openat/openat2`
- `unlinkat`
- `renameat2`
- `write`, `pwrite`, `truncate`, `ftruncate`

Programlar yalnızca hedef agent PID/cgroup’larını filtrelemeli ve ring buffer üzerinden küçük event kayıtları göndermelidir. Path string’i üzerinde pahalı kullanıcı politikası eşleştirmesi kernel hot path’ine taşınmamalıdır.
