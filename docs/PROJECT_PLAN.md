# RewindBPF — Project Decisions, Architecture, and MVP Plan

## 1. Project summary

RewindBPF is an **AI Agent Safety Runtime** that limits an autonomous agent’s ability to corrupt filesystem integrity, read sensitive files, access unauthorized resources, or exhaust system resources.

Product thesis:

> Start every agent run inside an isolated filesystem transaction. eBPF observes behavior, OverlayFS contains changes, policy controls sensitive access, and a failed transaction can be discarded in one operation.

Engineering motto:

> Keep the hot path cheap; make expensive work lazy and copy-on-write.

## 2. Problem and solution

When an AI agent has terminal access, its destructive behavior cannot be fully predicted:

- It can delete or overwrite files and directories.
- It can read secrets such as `.env` files, SSH keys, or PII directories.
- It can attempt privilege escalation, `mount`, or `ptrace` operations.
- It can make unauthorized network connections and exfiltrate data.
- It can exhaust CPU, memory, process IDs, or disk space.

Traditional pre-operation `cp` backups add I/O and latency to every operation. RewindBPF creates the OverlayFS layer before the agent starts and routes changes to a copy-on-write upper layer instead.

## 3. Critical architecture correction

We do not create a snapshot after eBPF observes a destructive event. By the time a userspace daemon reacts to `unlink` or `write`, the operation may already have started.

Correct flow:

```text
Before the agent starts:
  lowerdir = original, preferably read-only layer
  upperdir = temporary change layer
  workdir  = OverlayFS work directory
  merged   = workspace visible to the agent

While the agent runs:
  reads come from the lower/upper union
  writes and deletes go to upperdir
  eBPF observes events and produces policy signals

Rollback:
  stop the agent
  unmount merged
  discard upperdir
  expose lowerdir again
```

OverlayFS represents deletes with whiteouts and writes with copy-up without modifying the lower layer. Discarding the upper layer is the core rollback mechanism. OverlayFS is not a backup system; it is a pre-created temporary working layer.

## 4. Product definition and operating modes

The core product is a Linux runtime application, not an AI agent.

```text
rewind          CLI
rewindd         userspace daemon
ebpf/*.bpf.c    kernel sensor/policy programs
OverlayFS       filesystem transaction
Landlock/LSM    access control
namespace/VM   isolation
```

Target usage:

```bash
rewind run --workspace ./project --policy policy.yaml -- agent-command
rewind status
rewind events run_42
rewind rollback run_42
rewind commit run_42
```

Filesystem scopes:

- `workspace`: protect only the project/workspace; this is the first development mode.
- `system`: protect the normal filesystem inside a disposable Linux VM.

“Protect the entire host filesystem” is not removed from the MVP, but its safe and reproducible demonstration is performed inside a disposable VM. A live host’s `/proc`, `/sys`, device state, kernel state, open file descriptors, and network state are not fully reversible through OverlayFS.

## 5. Security layers

### 5.1 Filesystem integrity

OverlayFS should make these changes reversible:

- `write`, `pwrite`, `truncate`, `ftruncate`, `fallocate`
- `unlink`, `rmdir`, `rename`
- New files, directories, symlinks, or links
- Metadata changes, according to the MVP scope

eBPF observes target process/cgroup events such as `execve`, `openat/openat2`, `write`, `unlinkat`, `renameat2`, and `truncate/ftruncate`. A single `sys_enter_write` hook does not cover every write path; the observation boundary must be documented.

### 5.2 Read confidentiality

`.env` is only an example. Users define glob-based policies:

```yaml
read:
  mode: enforce # off | audit | enforce
  deny:
    - "**/.env"
    - "**/*.pem"
    - "**/*.key"
    - "/home/*/.ssh/**"
    - "/data/pii/**"
  allow:
    - "/workspace/.env.example"
```

Modes:

- `off`: read protection is disabled.
- `audit`: access is logged but not blocked.
- `enforce`: access is denied and an event is emitted.

Users write glob patterns; a policy compiler turns them into filesystem hierarchy and access rules. Avoid running expensive regex matching on every read syscall. Use Landlock and/or BPF LSM for enforcement. eBPF tracepoints are useful for audit and telemetry but should not be presented as the sole deny mechanism.

The first MVP provides path-based access control. Automatic content-based PII classification and redaction are future work.

### 5.3 Privilege, network, and resource policies

Future or limited-MVP policies:

| Risk | Suitable layer |
|---|---|
| `mount`, `ptrace`, setuid, BPF access | BPF LSM + seccomp |
| Unauthorized network connections | Network namespace + cgroup eBPF |
| Fork bombs and CPU/RAM/PID exhaustion | cgroups |
| Host paths or kernel interfaces | namespaces + Landlock |
| Process lineage and command ancestry | eBPF `execve` telemetry |

eBPF does not perform every security function by itself. Kernel hooks, namespaces, Landlock, seccomp, and cgroups are complementary layers.

## 6. Technical stack

- **Linux VM:** Ubuntu/Debian with OverlayFS and BPF/BTF support.
- **Filesystem:** ext4 for the first reproducible environment.
- **eBPF:** C + libbpf/CO-RE for portable kernel sensors and, where supported, BPF LSM hooks.
- **Read enforcement:** Landlock allowlists when active; optional BPF LSM backend for kernels with `bpf` enabled in the active LSM list.
- **Daemon:** Go for process management, mount namespaces, policy handling, ring buffers, JSON, and CLI orchestration.
- **Policy:** YAML/JSON input, glob patterns, and `off/audit/enforce` modes.
- **CLI:** no web UI in the first MVP; terminal timeline and commands are sufficient.
- **Benchmarks:** `hyperfine`, `fio`, `fs_mark`, `perf stat`, and a custom Go workload runner.
- **Verification:** hash/metadata manifests, JSON event logs, and CSV/JSON benchmark output.

Rust + Aya is a valid alternative; Go + C/libbpf is the lower-risk choice for a seven-day MVP.

## 7. Benchmark strategy

### 7.1 Comparison groups

| Group | Filesystem | eBPF | Daemon | Purpose |
|---|---|---:|---:|---|
| B0 | Native ext4 | No | No | Pure baseline |
| B1 | Native ext4 | Yes | No | eBPF-only cost |
| B2 | OverlayFS | No | No | OverlayFS cost |
| B3 | OverlayFS | Yes | No | eBPF + OverlayFS |
| B4 | OverlayFS | Yes | Yes | Real product path |
| B5 | OverlayFS | Yes | Yes + policy | Pause/kill enforcement cost |

The primary comparison is B0 ↔ B4. B1 and B2 isolate where overhead comes from.

### 7.2 Experiment controls

- Establish B0 before implementation changes.
- Use three warm-up runs and at least 15 measured repetitions.
- Report cold-cache and warm-cache results separately.
- Keep VM, kernel, CPU governor, disk, mount options, and dataset fixed.
- Minimize background services and keep workload seeds deterministic.
- Store the command, commit hash, kernel configuration, and dataset manifest for every result.

### 7.3 Workloads

- Read-heavy: `rg`, `find`, `git status`, small/large file reads, builds, and tests.
- Write-heavy: 10,000 small files, append, large-file overwrite, truncate, and rename.
- Metadata-heavy: create/unlink, recursive rename, chmod/chown, symlink, and hardlink.
- Mixed agent: create → modify → rename → delete → `rm -rf src/` → create new files.
- Concurrency: one, two, and four parallel agents.
- Policy: deny hit, audit hit, allow hit, and event flood.

### 7.4 Metrics

- Total duration and throughput.
- p50, p95, and p99 latency.
- CPU cycles, CPU utilization, context switches, and page faults.
- Read/write I/O bytes and peak RSS.
- eBPF event latency and dropped events.
- Copy-up time and upperdir size.
- Visible recovery time and full cleanup time.

```text
overhead (%) = ((variant_time - baseline_time) / baseline_time) × 100
space amplification = upperdir_bytes / logical_changed_bytes
```

Initial targets are hypotheses, not guarantees: low single-digit overhead for read-heavy work, acceptable overhead for mixed work, near-one-second visible rollback for the demo workspace, and zero event loss under normal workload.

## 8. Correctness and security tests

Create a lower-layer manifest before every test. Compare content, file structure, mode, UID/GID, symlink targets, xattrs, size, and timestamps after rollback.

Core scenarios:

1. Modify a file → original content returns.
2. Delete a file → file returns.
3. Recursive `rm -rf src/` → the complete tree returns.
4. Create a new file → it disappears after rollback.
5. Rename a directory → the old name returns.
6. Overwrite a large file → original content is preserved.
7. Kill the agent with `kill -9` → rollback still works.
8. Stop the daemon → the OverlayFS boundary continues protecting the current run; new runs fail closed.
9. Flood events → queue overflow and event loss are visible.
10. User policy → `off/audit/enforce` behavior is verified.
11. Denied `.env`, `.pem`, SSH, or PII path → read is blocked in enforce mode.
12. Symlink/path traversal → policy prevents access outside the allowed scope.
13. Unauthorized network → connection is audited or denied according to policy.
14. Open file descriptors, external writers, and host mounts are documented.
15. Successful run → commit/export preserves the expected diff.

Primary invariant:

> After rollback, the lower layer is unchanged and sentinel files outside the protected scope are unchanged.

## 9. Main demo flow

1. Start the agent inside an isolated VM/workspace transaction.
2. Show normal file events in the eBPF timeline.
3. Let the agent execute `rm -rf src/`.
4. Show that the lower layer remains intact and deletion records exist only in the upper layer.
5. Run `rewind rollback <run_id>`.
6. Show the project returning and prove recovery with the hash manifest.
7. Let the agent attempt to read `.env` or another user-defined sensitive pattern.
8. Show the access being audited or denied according to policy mode.

## 10. Scope and non-goals

### MVP scope

- Linux VM and reproducible setup.
- Workspace OverlayFS transaction.
- eBPF process/filesystem telemetry.
- Rollback.
- User-defined read patterns.
- General glob support rather than a hardcoded `.env` rule.
- System-scope experiment inside a disposable VM.
- B0–B5 benchmark matrix.

### Out of scope or later

- Automatic PII classification/redaction for all file contents.
- Absolute rollback of live-host kernel, device, or network state.
- Production-grade conflict-aware merge.
- Multi-filesystem and network-filesystem guarantees.
- Web dashboard and IDE integration.

## 11. Seven-day build plan

### Day 1 — Environment and baseline

- Ubuntu VM, kernel, and tools.
- OverlayFS/eBPF/Landlock capability checks.
- Deterministic dataset generator.
- B0 native filesystem baseline.

### Day 2 — OverlayFS sandbox

- lower/upper/work/merged lifecycle.
- Workspace mode.
- Agent process launch and namespace isolation.
- Basic rollback.

### Day 3 — eBPF telemetry

- `execve`, `openat`, `unlinkat`, `renameat2`, write/truncate events.
- Ring buffer → Go daemon.
- PID/cgroup filtering and JSON event logs.

### Day 4 — Read policy

- YAML policy parser.
- Glob → filesystem rule compiler.
- `off/audit/enforce` modes.
- Landlock/BPF LSM or supported-kernel enforcement.

### Day 5 — CLI, lifecycle, and fail-safe behavior

- `run`, `status`, `events`, `rollback`, `commit` commands.
- Crash and daemon-failure behavior.
- Workspace boundary and sentinel tests.

### Day 6 — Benchmarks and correctness

- B1–B5 measurements.
- Correctness test matrix.
- Hash manifests, event loss, and rollback timing.
- Graphs and CSV/JSON artifacts.

### Day 7 — Demo and presentation

- Deterministic destructive-agent demo.
- Secret-read policy demo.
- Recovery proof and latency display.
- Final review of technical claims, limitations, and benchmark results.

## 12. Open risks

- OverlayFS upper/work directories have filesystem, xattr, and `d_type` requirements.
- Overlaying a live host root filesystem is complex; system mode is constrained to a VM.
- eBPF syscall telemetry is not complete filesystem semantics; mmap and indirect write paths need separate treatment.
- A userspace decision after an event cannot undo a past operation; protection must be pre-established with mount and policy layers.
- Open file descriptors and namespace/capability escape are explicit threat-model items.
- Use “measured low hot-path overhead,” not “zero overhead.”

## 13. Decision summary

- Core product: Linux userspace runtime + eBPF + OverlayFS.
- eBPF does not create snapshots; it provides telemetry and, where appropriate, enforcement.
- A snapshot layer exists before every agent run.
- Rollback is the primary MVP operation; commit remains a simple, controlled diff/export path.
- Read protection is user-configurable with glob patterns and `off/audit/enforce` modes.
- Full filesystem scope is included in the MVP inside a disposable VM; no absolute live-host kernel/device rollback claim is made.
- Go + C/libbpf/CO-RE is the selected seven-day stack.
- Benchmarking starts with B0 baseline, then compares B1–B5 layers.

## 14. Safe execution roadmap

No mount, eBPF load, host bind mount, or destructive command should run on the personal host without an explicit safety review first.

### Stage 0 — Environment decision

Only read-only checks: host architecture, Go version, virtualization options, and Git status. Confirm the disposable Ubuntu VM path before kernel work.

### Stage 1 — Safe fixtures and policy contract

Create synthetic fixtures, manifests, run IDs, policy parsing, and CLI contracts. Never use real `.env`, SSH keys, or personal data.

### Stage 2 — Disposable Linux lab

Inside an Ubuntu VM, verify OverlayFS, BPF/BTF, Landlock, and ext4 capabilities. Do not perform destructive operations yet.

### Stage 3 — OverlayFS rollback

Run the first controlled destructive test only against synthetic fixtures inside the disposable VM, after reviewing the exact command and rollback path.

### Stage 4 onward

Add eBPF telemetry, read policies, fail-safe process isolation, VM system scope, benchmarks, and the deterministic demo in that order.

### Current continuation: end-to-end protected-run VM smoke

The manifest-to-kernel compiler, Landlock allowlist planner, fixed-size rule-map ABI, optional BPF-LSM `file_open` source, userspace loaders, Go OverlayFS manager, inert `internal/runplan` composer, fake-tested `internal/protectedrun` coordinator, policy-aware hidden helper, atomic run store, and `run/status/events/rollback` CLI paths are implemented. The disposable VM reports `landlock` active and `bpf` absent. Both opt-in VM tests passed: the Landlock child process denied the synthetic protected file with `EACCES`, and the OverlayFS manager preserved the lower marker after rollback. The next action is the first end-to-end `rewind run` smoke in the VM using only a generated workspace and static/synthetic agent command.

The helper refuses to launch an agent as root: when the parent requires `sudo`, it drops to `SUDO_UID`/`SUDO_GID` before Landlock and `exec`. This is a prerequisite for the end-to-end smoke rather than an optional hardening step.

```bash
REWIND_LANDLOCK_INTEGRATION=1 GOTOOLCHAIN=local go test ./internal/landlock -run TestLandlockSyntheticReadEnforcement -count=1 -v
```

The test must use only generated fixture files and an explicitly scoped child process; no real `.env`, SSH key, personal data, or host path is allowed.
