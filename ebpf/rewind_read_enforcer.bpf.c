#include "vmlinux.h"

#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>

#include "event.h"

char LICENSE[] SEC("license") = "GPL";

struct rewind_path_key {
	char path[REWIND_EVENT_PATH_LEN];
};

struct {
	__uint(type, BPF_MAP_TYPE_HASH);
	__uint(max_entries, 16384);
	__type(key, struct rewind_path_key);
	__type(value, __u32);
} rewind_read_rules SEC(".maps");

struct {
	__uint(type, BPF_MAP_TYPE_RINGBUF);
	__uint(max_entries, 1 << 24);
} rewind_events SEC(".maps");

// The userspace loader must set this before attaching the hook. A value of
// zero is rejected by the userspace loader and is not a production scope.
const volatile __u32 target_pid;

// Linux f_mode uses bit 0 for FMODE_READ. Keeping this local avoids importing
// userspace kernel headers into the CO-RE translation unit.
#define REWIND_FMODE_READ 0x1
#define REWIND_EACCES 13

static __always_inline bool target_matches(void)
{
	__u32 pid = (__u32)(bpf_get_current_pid_tgid() >> 32);

	return target_pid == 0 || target_pid == pid;
}

static __always_inline int emit_read_event(const struct rewind_path_key *key,
						   __u32 decision)
{
	struct rewind_event *event;

	event = bpf_ringbuf_reserve(&rewind_events, sizeof(*event), 0);
	if (!event)
		return 0;

	event->pid = (__u32)(bpf_get_current_pid_tgid() >> 32);
	event->operation = REWIND_OP_READ;
	event->timestamp_ns = bpf_ktime_get_ns();
	event->decision = decision;
	event->risk = REWIND_RISK_HIGH;
	__builtin_memcpy(event->path, key->path, sizeof(event->path));

	bpf_ringbuf_submit(event, 0);
	return 0;
}

SEC("lsm/file_open")
int BPF_PROG(rewind_file_open, struct file *file, int ret)
{
	struct rewind_path_key key = {};
	__u32 *decision;
	long path_len;

	if (ret != 0 || !target_matches() || !(file->f_mode & REWIND_FMODE_READ))
		return ret;

	path_len = bpf_d_path(&file->f_path, key.path, sizeof(key.path));
	if (path_len < 0)
		return ret;

	decision = bpf_map_lookup_elem(&rewind_read_rules, &key);
	if (!decision)
		return ret;

	if (*decision == REWIND_DECISION_DENY) {
		emit_read_event(&key, REWIND_DECISION_DENY);
		return -REWIND_EACCES;
	}

	if (*decision == REWIND_DECISION_AUDIT)
		emit_read_event(&key, REWIND_DECISION_AUDIT);

	return ret;
}
