//go:build linux

// Package seccomp contains narrowly scoped syscall filters used as a defense
// in depth layer. It does not replace the network policy backend: it only
// refuses raw/packet socket creation while leaving normal TCP/UDP sockets
// available to proxy-aware clients.
package seccomp

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	seccompDataNR   = 0
	seccompDataArgs = 16
	sockTypeMask    = 0xf
	seccompRetAllow = unix.SECCOMP_RET_ALLOW
	seccompRetErrno = unix.SECCOMP_RET_ERRNO
)

// InstallDenyRawSockets installs a no-new-privileges seccomp filter that
// denies AF_PACKET, AF_INET6 raw, and AF_INET raw socket creation. Stream and
// datagram sockets remain available for the explicit proxy backend.
func InstallDenyRawSockets() error {
	filters := []unix.SockFilter{
		{Code: unix.BPF_LD | unix.BPF_W | unix.BPF_ABS, K: seccompDataNR},
		{Code: unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K, Jt: 0, Jf: 7, K: uint32(unix.SYS_SOCKET)},
		{Code: unix.BPF_LD | unix.BPF_W | unix.BPF_ABS, K: seccompDataArgs},
		{Code: unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K, Jt: 6, Jf: 0, K: unix.AF_PACKET},
		{Code: unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K, Jt: 5, Jf: 0, K: unix.AF_INET6},
		{Code: unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K, Jt: 0, Jf: 3, K: unix.AF_INET},
		{Code: unix.BPF_LD | unix.BPF_W | unix.BPF_ABS, K: seccompDataArgs + 4},
		{Code: unix.BPF_ALU | unix.BPF_AND | unix.BPF_K, K: sockTypeMask},
		{Code: unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K, Jt: 1, Jf: 0, K: unix.SOCK_RAW},
		{Code: unix.BPF_RET | unix.BPF_K, K: seccompRetAllow},
		{Code: unix.BPF_RET | unix.BPF_K, K: seccompRetErrno | uint32(unix.EPERM)},
	}
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return fmt.Errorf("seccomp no-new-privileges: %w", err)
	}
	program := unix.SockFprog{Len: uint16(len(filters)), Filter: &filters[0]}
	if err := unix.Prctl(unix.PR_SET_SECCOMP, unix.SECCOMP_MODE_FILTER, uintptr(unsafe.Pointer(&program)), 0, 0); err != nil {
		return fmt.Errorf("seccomp raw-socket filter: %w", err)
	}
	return nil
}
