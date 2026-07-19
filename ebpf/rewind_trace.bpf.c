#include "vmlinux.h"

#include <bpf/bpf_core_read.h>
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>

#include "event.h"

char LICENSE[] SEC("license") = "GPL";

struct {
	__uint(type, BPF_MAP_TYPE_RINGBUF);
	__uint(max_entries, 1 << 24);
} rewind_events SEC(".maps");

/* A ring buffer reserve can fail under pressure. Keep the loss signal in a
 * per-CPU counter so the userspace journal can distinguish an empty stream
 * from an incomplete one without adding work to the hot path. */
struct {
	__uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
	__uint(max_entries, 1);
	__type(key, __u32);
	__type(value, __u64);
} rewind_dropped SEC(".maps");

// The userspace loader must set this before starting an agent run. A value of
// zero is useful for a controlled telemetry smoke test, but production runs
// should always scope events to the agent PID or cgroup.
const volatile __u32 target_pid;

struct {
	__uint(type, BPF_MAP_TYPE_HASH);
	__uint(max_entries, 4096);
	__type(key, __u32);
	__type(value, __u8);
} tracked_pids SEC(".maps");

static __always_inline bool target_matches(void)
{
	__u32 pid = (__u32)(bpf_get_current_pid_tgid() >> 32);
	__u8 one = 1;
	struct task_struct *task;
	struct task_struct *parent = 0;
	__u32 parent_pid = 0;

	if (target_pid == 0 || target_pid == pid)
		return true;
	if (bpf_map_lookup_elem(&tracked_pids, &pid))
		return true;

	/* The helper process may launch a shell, which then execs the agent.
	 * Track that child at exec time and retain the decision for its syscalls. */
	task = (struct task_struct *)bpf_get_current_task_btf();
	bpf_core_read(&parent, sizeof(parent), &task->real_parent);
	if (!parent)
		return false;
	bpf_core_read(&parent_pid, sizeof(parent_pid), &parent->tgid);
	if (parent_pid == target_pid || bpf_map_lookup_elem(&tracked_pids, &parent_pid)) {
		bpf_map_update_elem(&tracked_pids, &pid, &one, BPF_ANY);
		return true;
	}
	return false;
}

static __always_inline int emit_event(__u32 operation, __u32 risk,
					      const char *user_path)
{
	struct rewind_event *event;

	if (!target_matches())
		return 0;

	event = bpf_ringbuf_reserve(&rewind_events, sizeof(*event), 0);
	if (!event) {
		__u32 key = 0;
		__u64 *dropped = bpf_map_lookup_elem(&rewind_dropped, &key);
		if (dropped)
			(*dropped)++;
		return 0;
	}

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

SEC("tracepoint/sched/sched_process_exit")
int trace_process_exit(void *ctx)
{
	__u32 pid = (__u32)(bpf_get_current_pid_tgid() >> 32);

	bpf_map_delete_elem(&tracked_pids, &pid);
	return 0;
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
