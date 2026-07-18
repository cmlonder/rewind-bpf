#include "vmlinux.h"

#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>

#include "event.h"

char LICENSE[] SEC("license") = "GPL";

struct {
	__uint(type, BPF_MAP_TYPE_RINGBUF);
	__uint(max_entries, 1 << 24);
} rewind_events SEC(".maps");

// The userspace loader must set this before starting an agent run. A value of
// zero is useful for a controlled telemetry smoke test, but production runs
// should always scope events to the agent PID or cgroup.
const volatile __u32 target_pid;

static __always_inline bool target_matches(void)
{
	__u32 pid = (__u32)(bpf_get_current_pid_tgid() >> 32);

	return target_pid == 0 || target_pid == pid;
}

static __always_inline int emit_event(__u32 operation, __u32 risk,
					      const char *user_path)
{
	struct rewind_event *event;

	if (!target_matches())
		return 0;

	event = bpf_ringbuf_reserve(&rewind_events, sizeof(*event), 0);
	if (!event)
		return 0;

	event->pid = (__u32)(bpf_get_current_pid_tgid() >> 32);
	event->operation = operation;
	event->timestamp_ns = bpf_ktime_get_ns();
	event->decision = REWIND_DECISION_ALLOW;
	event->risk = risk;
	event->path[0] = '\0';
	if (user_path)
		bpf_probe_read_user_str(event->path, sizeof(event->path), user_path);

	bpf_ringbuf_submit(event, 0);
	return 0;
}

SEC("tracepoint/syscalls/sys_enter_execve")
int trace_execve(struct trace_event_raw_sys_enter *ctx)
{
	return emit_event(REWIND_OP_EXECVE, REWIND_RISK_LOW,
			  (const char *)ctx->args[0]);
}

SEC("tracepoint/syscalls/sys_enter_openat")
int trace_openat(struct trace_event_raw_sys_enter *ctx)
{
	return emit_event(REWIND_OP_OPENAT, REWIND_RISK_MEDIUM,
			  (const char *)ctx->args[1]);
}

SEC("tracepoint/syscalls/sys_enter_write")
int trace_write(struct trace_event_raw_sys_enter *ctx)
{
	return emit_event(REWIND_OP_WRITE, REWIND_RISK_HIGH, 0);
}

SEC("tracepoint/syscalls/sys_enter_pwrite64")
int trace_pwrite64(struct trace_event_raw_sys_enter *ctx)
{
	return emit_event(REWIND_OP_WRITE, REWIND_RISK_HIGH, 0);
}

SEC("tracepoint/syscalls/sys_enter_unlinkat")
int trace_unlinkat(struct trace_event_raw_sys_enter *ctx)
{
	return emit_event(REWIND_OP_UNLINKAT, REWIND_RISK_HIGH,
			  (const char *)ctx->args[1]);
}

SEC("tracepoint/syscalls/sys_enter_renameat2")
int trace_renameat2(struct trace_event_raw_sys_enter *ctx)
{
	return emit_event(REWIND_OP_RENAMEAT2, REWIND_RISK_HIGH,
			  (const char *)ctx->args[1]);
}

SEC("tracepoint/syscalls/sys_enter_truncate")
int trace_truncate(struct trace_event_raw_sys_enter *ctx)
{
	return emit_event(REWIND_OP_TRUNCATE, REWIND_RISK_HIGH,
			  (const char *)ctx->args[0]);
}
