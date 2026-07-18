#ifndef REWIND_EVENT_H
#define REWIND_EVENT_H

// vmlinux.h must be included before this header by the eBPF translation unit.
// It provides the kernel ABI types (__u32/__u64) without pulling userspace
// Linux headers into the BPF compilation and causing duplicate typedefs.

#define REWIND_EVENT_PATH_LEN 256

enum rewind_operation {
	REWIND_OP_EXECVE = 1,
	REWIND_OP_OPENAT,
	REWIND_OP_READ,
	REWIND_OP_WRITE,
	REWIND_OP_UNLINKAT,
	REWIND_OP_RENAMEAT2,
	REWIND_OP_TRUNCATE,
};

enum rewind_decision {
	REWIND_DECISION_ALLOW = 0,
	REWIND_DECISION_AUDIT,
	REWIND_DECISION_DENY,
};

enum rewind_risk {
	REWIND_RISK_LOW = 1,
	REWIND_RISK_MEDIUM,
	REWIND_RISK_HIGH,
};

// Keep this layout in sync with internal/event.Event's wire-code mapping.
struct rewind_event {
	__u32 pid;
	__u32 operation;
	__u64 timestamp_ns;
	__u32 decision;
	__u32 risk;
	char path[REWIND_EVENT_PATH_LEN];
};

#endif
