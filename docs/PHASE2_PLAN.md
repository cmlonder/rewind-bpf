# RewindBPF Phase 2 Plan

**Status:** Proposed for the six-day post-MVP sprint
**Owner:** RewindBPF team
**Last updated:** 2026-07-19
**Decision horizon:** Hackathon demo in six days, followed by a 90-day productisation track

## 1. Executive decision

The MVP is ready to demonstrate in the disposable Ubuntu VM. Phase 2 should not try to add every Linux security primitive at once. The highest-value work is to make the current transaction boundary correct under failure, make policy scope complete for the whole process tree, and make the evidence reproducible.

The strategic correction after the nono comparison is recorded in [`docs/PRODUCT_STRATEGY.md`](PRODUCT_STRATEGY.md). RewindBPF will not become a less mature general-purpose sandbox. Its wedge is the user-visible combination of immutable project writes, invisible secrets, explicit acceptance, and fail-closed trust.

The Phase 2 product promise is therefore:

> **Let an agent work aggressively without giving it direct access to the real project or real credentials; accept only the reviewed result.**

The implementation promise underneath is: run an unmodified AI agent inside a pre-created, reversible Linux transaction; enforce least-privilege reads before execution; observe the complete process tree; and prove rollback or commit with verifiable evidence.

This is deliberately narrower than “protect the whole operating system” or “zero overhead.” OverlayFS protects filesystem changes inside the selected boundary. Landlock protects the selected read/write hierarchy. eBPF supplies low-cost telemetry and optional enforcement where the kernel supports it. Network, kernel state, devices, external services, and already-open descriptors remain explicit safety boundaries.

## 2. What the MVP proved, and what it did not

### Verified MVP evidence

- A FUSE OverlayFS-backed protected run completed in the disposable Ubuntu 24.04 ARM64 VM.
- A synthetic secret read was denied by Landlock; deleting `src/` affected only the merged view.
- Rollback restored the original lower layer and removed generated files.
- The eBPF sensor emitted `openat`, `write`, `unlinkat`, `renameat2`, `truncate`, and `execve` telemetry.
- Descendant tracking was verified with a shell-launched `dd` process: 46 events across two PIDs.
- Warm B4 throughput was approximately 11.1% below native B0 and approximately 0.4% above FUSE-only B2. This indicates that the measured steady-state cost is dominated by the FUSE backend, not a proven “zero-cost” Rewind layer.
- Cold B4 was materially slower because each run included mount setup, helper startup, and first copy-up. It is a lifecycle measurement, not a hot-path overhead result.

### Known MVP gaps

1. The record and event log are now restored to the invoking user after `sudo`; the privileged FUSE mount still requires `sudo` for unmount/rollback.
2. Cgroup-v2 is now the primary process identity and drain boundary; PID descendant tracking remains in the eBPF sensor for event correlation and compatibility.
3. The event stream is JSONL and can grow much faster than the run record; kernel-side reserve failures are now counted and make evidence incomplete. A bounded `REWIND_EVENT_MAX_BYTES` cap marks userspace truncation explicitly, while `REWIND_EVENT_ROTATE_BYTES` rotates the stream into ordered files without resetting the hash chain. The read-only evidence verifier checks the combined digest and chain; backpressure remains future work.
4. Rollback is strong for the mounted filesystem transaction, but crash recovery and open-file-descriptor semantics need explicit tests.
5. There is no conflict-aware `commit` path. “Discard upper” is safe; the review-only `export` path is implemented, while merging arbitrary agent changes into a live workspace is not yet safe.
6. Kernel OverlayFS and FUSE OverlayFS have different capabilities and performance. Backend selection is explicit, but the capability report and compatibility matrix are not yet productised.
7. Network and capability policy are represented in the policy model but are not equivalent to filesystem rollback.

### Phase 2 implementation progress

The first P0 slice is now implemented and verified in the disposable VM:

- `mounted` lifecycle state plus prepared run journal before mount/agent start.
- Idempotent `rollback` and explicit `recover` for stale records.
- Per-run cgroup-v2 creation, helper admission, descendant inheritance, and cleanup.
- Read-only `capabilities` probe persisted in the run plan.
- Invoker-owned record and event log after privileged execution.
- Helper start gate that releases the agent only after sensor attachment.
- Event count, byte count, SHA-256 digest, kernel-side dropped-event count, sequence numbers, a userspace hash chain, and complete/truncated JSONL evidence flag.
- Read-only `diff --record` manifest comparison for a live merged view.

The VM smoke recorded 77 events (14,428 bytes) for a short synthetic command with `dropped=0`, and rollback preserved the lower-layer marker. A follow-up synthetic destructive run recorded 39 events (7,334 bytes), `dropped=0`, and rolled back successfully. A background `sleep` child was then detected by the cgroup drain gate; the run failed closed, rolled back, and left no child process or cgroup behind. Finally, a `SIGKILL` parent crash left a `running` record; `rewind recover` accepted the already-torn-down FUSE mount, killed/drained the scope, discarded upper/work, and restored the lower marker. An open-descriptor crash smoke wrote through fd 9 in the merged layer, was forcibly terminated, and recovered with the lower marker unchanged. A VM-only small-ring stress test intentionally dropped 37 events from 50,000 writes; the run record remained `dropped=37`, `complete=false` after rollback, and `rewind verify` exited 2. P0 now includes sequence/hash-chain evidence in addition to kernel drop accounting. The review-only `rewind export`, `policy learn`, optional cgroup resource-limit workflow, independent evidence verifier, and ordered JSONL rotation are now implemented; remaining P0 work is broader lifecycle fault coverage, while explicit backpressure remains P1.

## 3. Research and competitive findings

The market does not have a clean “nobody does kernel-level agent safety” gap. The defensible opportunity is the composition and proof boundary: RewindBPF combines a pre-run writable filesystem transaction, configurable read confidentiality, kernel telemetry, and a run-level rollback state machine in one Linux-first workflow.

| System | Primary strength | RewindBPF overlap | Phase 2 implication |
|---|---|---|---|
| [nono](https://nono.sh/os-sandbox) | Landlock/Seatbelt allowlists, child inheritance, profiles, undo and audit for agents | Kernel filesystem isolation and agent-oriented undo | Treat Landlock policy UX, profile discovery, and cross-platform capability reporting as the benchmark for our policy surface; differentiate on explicit OverlayFS transaction semantics and Linux rollback evidence. |
| [Cilium Tetragon](https://tetragon.io/docs/getting-started/enforcement/) | eBPF observability and enforcement for process, file, and network events | eBPF event collection and sensitive-file enforcement | Reuse the idea of in-kernel filtering, but keep RewindBPF agent/run identity and rollback as the product boundary; add event-loss accounting and cgroup scope. |
| [KubeArmor](https://docs.kubearmor.io/kubearmor/quick-links/kubearmor_overview/runtime_enforcer) | AppArmor, SELinux, and BPF-LSM policy translation for workloads | Policy modes and kernel enforcement | Build a backend capability matrix rather than assuming BPF-LSM; Landlock is the portable unprivileged default, BPF-LSM is an optional accelerator/enforcer. |
| [AgentFS](https://www.agentfs.ai/) | SQLite-backed isolated, auditable, snapshot-able agent filesystem | COW isolation, history, rollback | Match queryable change history and exportability without replacing the native Linux filesystem; add a portable run manifest and diff index. |
| [OpenHands Docker sandbox](https://docs.openhands.dev/openhands/usage/sandboxes/docker) | Practical container runtime for agent execution | Agent-agnostic execution boundary | Position RewindBPF as a filesystem transaction that can sit inside a VM or container, not as a replacement for container/VM isolation. |
| [DeltaBox](https://arxiv.org/abs/2605.22781) | Research direction for millisecond agent checkpoint/rollback using layered filesystem/process state | Checkpoint and rewind objective | Borrow the layer-switching and checkpoint vocabulary, but publish measured limitations and avoid claiming research-level process checkpoint/restore until it is implemented. |

### Competitive conclusion

Phase 2 must stop treating “kernel-level” as a differentiator by itself. The differentiators to prove are:

- **Prepared transaction:** the write layer exists before the agent starts; eBPF is not a post-hoc backup trigger.
- **Two independent safety planes:** filesystem rollback for integrity, and Landlock/BPF-LSM policy for confidentiality/prevention.
- **Agent-agnostic process scope:** no SDK or agent rewrite; all descendants are covered.
- **Evidence-first lifecycle:** every run has a state, policy digest, backend, event-loss status, manifest, and rollback/commit result.
- **Honest portability:** capability detection chooses kernel OverlayFS, FUSE OverlayFS, Landlock, or a safe refusal instead of silently weakening the guarantee.

### Competitive benchmark strategy

The comparison is deliberately two-layered. RewindBPF owns the primary, reproducible B0/B2/B4 dataset in the disposable Ubuntu VM. Competitor numbers are added only when the same tool, version, kernel, workload, and policy boundary can be reproduced; otherwise the cell is labeled “published” or “not comparable.”

| Competitor | What we can measure fairly | What remains non-comparable |
|---|---|---|
| nono | Startup, policy-denied read latency, mixed file I/O, undo time, storage growth, audit bytes, and fork/exec coverage | Different sandbox/undo implementation and backend; no universal overhead ranking from one VM |
| Tetragon | Event throughput, CPU/memory, drop behavior, decision latency, and process-tree visibility | It is not a filesystem transaction or rollback engine |
| KubeArmor | Rule-install and deny latency, event throughput, CPU/memory, and enforcement coverage in its supported workload | Kubernetes policy deployment is a different product boundary from a local agent run |
| AgentFS | Write/snapshot/restore/query latency, logical vs physical bytes, and export size | SQLite/filesystem abstraction is not native ext4 + OverlayFS syscall behavior |
| DeltaBox | Published paper numbers with exact configuration, or an author artifact reproduction | Research checkpoint scope and workload cannot be treated as a product benchmark by default |

The benchmark ledger therefore reports measurement provenance (`measured`, `published`, or `not comparable`) beside every external value. The jury-facing claim is not “faster than every competitor”; it is that RewindBPF measures the cost of its explicit safety invariant—pre-created COW writes plus kernel telemetry—and makes the tradeoff auditable.

The jury-facing single-page site in `site/` now presents this same distinction: shipped capabilities first, planned work second, then a capability matrix and the measured B0/B2/B4 bars. The site is a static presentation layer; the Markdown architecture and benchmark ledgers remain canonical. The read-only `internal/evidence` verifier now backs `rewind verify`, `rewind evidence verify`, and the separately buildable `rewind-evidence` binary.

## 3.1 Nono parity track

Nono is the closest product benchmark, so its publicly documented feature set becomes a checklist rather than a vague comparison. The goal is feature parity where it materially improves agent safety, not a blind reimplementation of its architecture.

| Publicly documented nono capability | RewindBPF MVP state | Phase 2 decision | Longer-term position |
|---|---|---|---|
| Kernel isolation and inherited child restrictions | Landlock read policy plus PID descendant telemetry; FUSE transaction | **P0:** cgroup-v2 scope, capability report, policy preview, and explicit degraded mode | Keep Linux-first Landlock/BPF-LSM backends; consider other OS backends only after Linux correctness. |
| Profile-based policy and `learn` workflow | YAML policy, glob deny patterns, `off/audit/enforce` | **P0:** versioned policy schema, `policy learn`, explain/validate commands | Signed, composable profiles with toolchain/runtime groups. |
| Atomic undo and content-addressed snapshots | OverlayFS/FUSE upper-layer discard; SHA-256 start manifests | **P0:** diff index, rollback evidence, crash recovery; **P1:** deduplicated content store if storage measurements justify it | Multiple checkpoints and portable run bundles. |
| Cryptographic audit trail/Merkle commitment | JSONL telemetry and run record; no final Merkle root | **P0:** sequence numbers, drop counters, hash-chained batches, final root, read-only verifier | Signed remote evidence and standalone packaging. |
| Domain/network filtering | Policy field exists, no equivalent enforcement | **P1:** network namespace/proxy adapter with audit/enforce semantics | Credential-aware egress broker and per-agent network profiles. |
| Credential injection without exposing raw keys | Not implemented | **P1:** design only during six-day sprint; never pass secrets through argv or agent workspace | Keychain/secret-manager adapters and short-lived scoped tokens. |
| Runtime supervisor and dynamic permission approval | CLI owns one run; no long-lived supervisor | **P1:** Unix-socket supervisor design and approval protocol | Policy decision service with human/automation approval and time-bounded grants. |
| Signed provenance/registry for profiles and agent packs | Local files only | **P2:** document trust boundary and sign release artifacts | Sigstore-compatible profile/adapter registry. |
| Detachable/ghost sessions | Not implemented | **P2:** explicitly out of the six-day critical path | Persistent run handles with reconnect, retention, and operator takeover. |

The priority is intentional. Nono already demonstrates a broad product surface: kernel isolation, undo, audit, provenance, supervision, network filtering, credential injection, and detachable sessions ([feature overview](https://nono.sh/), [undo](https://nono.sh/undo), [audit trail](https://nono.sh/audit-trail), [profile learning](https://nono.sh/blog/nono-learn-policy-profile)). RewindBPF should first close the correctness and evidence gaps that would make our rollback claim unreliable, then add network/credential/supervisor features as separate policy planes. A six-day sprint that starts with a registry, durable history, or UI polish would create parity theatre without a stronger safety invariant. The complete product strategy, including native macOS and Windows tracks, lives in [`docs/PRODUCT_STRATEGY.md`](PRODUCT_STRATEGY.md).

### What RewindBPF should do better than nono

These are product hypotheses to validate, not current claims:

1. **Transaction-native writes:** make the lower/upper/merged filesystem boundary the primary object from which diffs, rollback, and future commit are derived, instead of treating undo as a post-session snapshot feature.
2. **Filesystem and policy timeline in one run:** correlate the eBPF event stream, Landlock decisions, process/cgroup identity, upper-layer diff, and final state in one evidence bundle.
3. **Conflict-safe export:** make `commit` refuse when the destination lower manifest changed, then export a reviewable patch/diff rather than overwriting the live workspace.
4. **Capability honesty:** report exactly which guarantees are active on this kernel (Landlock ABI, BPF-LSM, cgroup-v2, OverlayFS/FUSE, network backend) and fail closed when an enforce-mode guarantee cannot be provided.
5. **Agent-agnostic deployment:** keep the core runtime independent of Claude/Codex/OpenHands/etc.; integrations remain thin adapters.

## 4. Phase 2 goals and non-goals

### Goals

1. Make failure behavior deterministic: crash, `SIGKILL`, helper failure, mount failure, event loss, and interrupted rollback.
2. Replace PID-only identity with cgroup-v2 scope, retaining PID tracking as a fallback for the MVP VM.
3. Make policies auditable and composable: read, write, execute, network, resource, and mode/capability reporting.
4. Make rollback and future commit explicit transactions with manifests, diffs, and conflict checks.
5. Produce a reproducible benchmark harness with warm/cold, kernel/FUSE, telemetry on/off, and storage amplification dimensions.
6. Package the workflow so a reviewer can install one binary, run one safe synthetic demo, and understand exactly what is protected.

### Non-goals

- Reversing network requests, cloud mutations, kernel/device state, database transactions, or external side effects.
- A generic VM/container replacement.
- Automatic semantic judgement of whether an agent’s code change is “good.”
- Content inspection or PII classification of every file.
- Rootless system-wide protection on arbitrary host filesystems without a tested kernel/filesystem capability matrix.
- Claiming zero overhead; report measured overhead by backend and workload.

## 5. Six-day execution plan

Each day has a demonstrable exit criterion. All privileged or destructive commands remain VM-only and require a safety review before execution.

### Day 1 — Transaction correctness and crash recovery (P0)

**Build**

- Add an explicit run journal with atomic state transitions: `planned → mounted → running → succeeded|failed → rolled_back|committed`.
- Persist the policy digest, lower/upper/work/merged paths, backend, kernel capability report, helper PID/cgroup, and event sequence counters.
- Add startup recovery: detect stale `mounted/running` records, unmount safely, preserve evidence, and mark the run `aborted` rather than silently deleting data.
- Make rollback idempotent and verify that no merged mount remains.
- Write metadata with the invoking `SUDO_UID`/`SUDO_GID` where safe; otherwise print the exact privileged inspection command.

**Tests**

- Kill the helper at each lifecycle edge; kill the parent during agent execution; interrupt rollback twice.
- Restart the CLI and recover stale records.
- Verify lower-layer hashes before/after every failure injection.

**Exit criterion:** 100% of injected lifecycle failures leave either a mounted run with recoverable evidence or a cleanly unmounted, rolled-back run; never a false `succeeded` state.

### Day 2 — Process-tree and policy boundary hardening (P0)

**Build**

- Create one cgroup-v2 per run and place the helper/agent process tree in it before execution.
- Prefer cgroup identity in eBPF filtering; retain descendant-PID maps only as a compatibility fallback.
- Add a process-exit and cgroup-empty gate before unmount/rollback.
- Define policy precedence: deny beats allow, explicit paths beat globs, and unsupported rights fail closed in `enforce` mode.
- Add `execute` and `refer`/rename coverage to the policy capability report, even if the first backend cannot enforce every right.

**Tests**

- Shell → `dd` → background child → detached child; verify all events and cleanup.
- Attempt `setsid`, double-fork, `execve`, symlink traversal, hard links, rename across directories, and `/proc` path aliases.
- Run with an unsupported Landlock ABI and confirm the result is an explicit degraded mode or refusal.

**Exit criterion:** no tested child-process escape; every run reports its exact process-scope mechanism and any fallback.

### Day 3 — Policy and enforcement depth (P0/P1)

**Build**

- Stabilise a versioned YAML schema with `read`, `write`, `execute`, `network`, `resources`, `scope`, `mode`, and backend capabilities.
- Add `audit` mode that records denied-intent events without blocking; add `enforce` mode that denies before the operation.
- Keep Landlock as the default unprivileged filesystem enforcement backend.
- Add an optional BPF-LSM enforcement adapter for kernels with active `bpf` LSM; never silently select it when `/sys/kernel/security/lsm` does not contain `bpf`.
- Add a policy “learn” command that converts observed paths into a reviewable allowlist; never auto-allow secrets or broad parent directories. Implemented: output defaults to `audit`, refuses to overwrite an existing file, and filters secret-like, virtual, and broad paths. The read-only `policy explain` preview keeps deny-before-allow precedence.

**Tests**

- Synthetic `.env`, SSH key, token, symlink, hard link, mmap write, truncate, rename, and directory deletion cases.
- Compare `off`, `audit`, and `enforce` decisions and confirm that read denial does not rely on the agent’s cooperation.

**Exit criterion:** a reviewer can express a custom sensitive-file pattern, preview its effect, and demonstrate both audit and enforce behavior with a deterministic fixture.

### Day 4 — Telemetry integrity and usable evidence (P1)

**Build**

- Add a bounded ring-buffer/event pipeline with sequence numbers, dropped-event counters, backpressure policy, and rotation limits. Implemented slices: `REWIND_EVENT_MAX_BYTES` caps total retention, `REWIND_EVENT_ROTATE_BYTES` rolls the JSONL stream into ordered files while preserving the chain, and the reader continues draining the ring; capped streams persist `truncated=true` so verification fails closed.
- Store compact JSONL for streaming plus a queryable run index (SQLite or an append-only compact format) for summaries.
- Hash-chain event batches and include the final digest in the run record; document that this is tamper evidence, not a trusted remote log.
- Add `rewind diff` to summarize created, modified, deleted, renamed, and policy-denied paths without printing secret contents. The non-mutating `rewind export` review bundle is implemented; conflict-checked commit remains separate.
- Add `rewind capabilities`, `rewind inspect`, `rewind verify`, and machine-readable status output.

**Tests**

- Saturate the ring buffer and assert that a run cannot claim complete telemetry when events were dropped.
- Rotate logs, crash the writer, truncate the last line, and verify recovery behavior.
- Confirm that secret paths are redacted in summaries while exact policy decisions remain inspectable.

**Exit criterion:** every run has a bounded, verifiable evidence bundle and an explicit `events_complete`/`events_dropped` result.

### Day 5 — Backend, storage, and benchmark rigor (P1)

**Build**

- Add a capability probe that reports kernel OverlayFS, FUSE OverlayFS, xattrs, `d_type`, same-filesystem workdir, Landlock ABI, BPF-LSM, cgroup-v2, and seccomp support.
- Keep kernel OverlayFS and FUSE OverlayFS as separate measured backends; refuse incompatible combinations instead of guessing.
- Add workload classes: small-file tree, full-file overwrite, sparse overwrite, delete/rename storm, read-only workload, and mixed agent build workload.
- Add warm/cold order randomisation, at least five repetitions for warm and three for cold, and confidence intervals.
- Track upper bytes, work bytes, event bytes, record bytes, copy-up ratio, peak usage, rollback time, and cleanup time alongside IOPS/latency.

**Tests**

- Re-run B0/B2/B4/B5 with telemetry disabled/enabled and both backends where supported.
- Verify storage after rollback and after failed runs; no orphan upper/work trees.

**Exit criterion:** charts separate steady-state I/O overhead, lifecycle overhead, first-copy-up cost, telemetry cost, and storage amplification.

### Day 6 — Demo, release gate, and public evidence (P0)

**Build**

- Produce one deterministic three-act demo: destructive delete → secret-read denial → rollback proof.
- Add a failure-act: kill the agent midway and show automatic recovery/status.
- Freeze benchmark CSV/SVG, capability report, threat model, limitations, and competitor matrix.
- Add a minimal release artifact: versioned binary, eBPF object, example policy, VM quickstart, checksum, and a “do not run on your host” warning.
- Rehearse the exact command sequence in a clean disposable VM snapshot.

**Exit criterion:** a fresh VM operator can reproduce the demo from README without touching the host filesystem, and every claim on the slide deck maps to a checked-in artifact or cited external source.

## 6. Target Phase 2 architecture

```mermaid
flowchart LR
    CLI[rewind CLI] --> PLAN[Run planner + capability probe]
    PLAN --> TX[Transaction manager]
    TX --> MNT[Overlay backend\nKernel or FUSE]
    TX --> CG[Run cgroup-v2]
    CG --> H[Policy-aware helper]
    H --> AG[Unmodified agent + descendants]
    AG --> LL[Landlock default-deny\nread/write/execute policy]
    AG --> BPF[eBPF telemetry\noptional BPF-LSM enforcement]
    BPF --> EV[Bounded event pipeline\nsequence + drop counters]
    EV --> REC[Run evidence bundle\nrecord + diff + hash chain]
    REC --> DEC{Human/automation decision}
    DEC -->|rollback| DISC[Unmount + discard upper/work]
    DEC -->|commit| EXP[Conflict-checked diff/export]
```

### Boundary rules

1. **The transaction manager owns mounts and state;** the sensor never decides to create a snapshot after damage.
2. **The helper owns identity and policy installation;** the agent cannot run as root and cannot widen Landlock rules.
3. **The kernel owns enforcement;** userspace audit is advisory and must not be described as prevention.
4. **The evidence writer owns completeness;** dropped events are a first-class failure signal.
5. **Commit is a separate operation from rollback;** it must compare the lower-layer manifest captured at start with the live destination before exporting changes.

## 7. Correctness and security test matrix

| Area | Scenario | Expected invariant | Priority |
|---|---|---|---:|
| Lifecycle | mount fails | no agent starts; record is `failed`; no partial mount remains | P0 |
| Lifecycle | helper exits early | agent never runs; upper/work are cleaned or retained for diagnosis | P0 |
| Lifecycle | `SIGKILL` agent | run is `aborted` or `succeeded` only after explicit policy; rollback remains safe | P0 |
| Filesystem | unlink/rmdir | lower hash unchanged; whiteout is confined to upper | P0 |
| Filesystem | rename directory | no lower mutation; redirect/EXDEV behavior is recorded | P0 |
| Filesystem | mmap/truncate/writeback | write is visible in diff or the limitation is reported | P0 |
| Filesystem | open FD survives rollback | FD cannot mutate the lower layer after unmount; process is drained first | P0 |
| Confidentiality | glob-denied secret read | operation returns denial in enforce mode; path is redacted in user-facing summaries | P0 |
| Process scope | fork/exec/double-fork | all descendants are inside cgroup and telemetry scope | P0 |
| Policy | unsupported ABI/right | explicit degraded/refused status; no silent allow | P0 |
| Telemetry | ring-buffer overflow | dropped count is non-zero and evidence is incomplete | P1 |
| Storage | full overwrite/sparse overwrite | copy-up and peak disk usage are measured separately | P1 |
| Recovery | power-loss simulation / abrupt process death | startup recovery finds stale runs and preserves lower layer | P1 |
| Commit | destination changed concurrently | commit refuses with a manifest conflict; no partial export | P1 |

## 8. Post-hackathon product roadmap

### 0–30 days: reliable local runtime

- Finish P0 crash/recovery, cgroup scope, event completeness, and metadata ownership.
- Ship Linux x86_64 and ARM64 release artifacts with capability diagnostics.
- Add `diff`, `inspect`, policy learning, and a documented commit/export preview.
- Publish reproducible benchmark scripts and raw anonymised result bundles.
- Integrate one real agent adapter (OpenHands, Claude Code, Codex CLI, or a generic command wrapper) without coupling core packages to that agent.

### 31–60 days: team and CI workflow

- Add a long-running `rewindd` supervisor with a local Unix socket and authenticated run handles.
- Add CI mode: every agent task runs in a disposable workspace; output is a patch/artifact rather than an implicit host merge.
- Add remote/object-store evidence bundles, retention policies, and signed release metadata.
- Add network namespace/proxy policy as a separate plane; make credentials injectable without placing raw secrets in the agent filesystem.
- Evaluate seccomp filters for syscall-surface reduction. Use seccomp user notification only for narrow, reviewable operations; the kernel documentation warns about notification TOCTOU and blocking semantics, so it is not a default file-write interceptor. See the [kernel seccomp documentation](https://docs.kernel.org/userspace-api/seccomp_filter.html).

### 61–90 days: scale and research track

- Add a pluggable storage backend: native OverlayFS, FUSE OverlayFS, reflink filesystem, and a content-addressed diff store where measurements justify it.
- Add checkpoint markers for long-running agents and retain multiple rewind points; do not promise process-memory restore until a CRIU-based prototype passes its own compatibility matrix.
- Add multi-agent run trees with isolated child transactions and explicit merge dependencies.
- Add policy simulation and explainability: “which rule denied this syscall, and what would be allowed if the user changes it?”
- Commission an independent security review and publish a threat-model/limitations report.

## 9. Decisions that protect long-term value

- **Keep the core agent-agnostic.** Integrations belong in adapters, not in the transaction, policy, or telemetry packages.
- **Keep kernel primitives replaceable.** Landlock, BPF-LSM, cgroup-v2, seccomp, and OverlayFS have different availability and semantics; capability negotiation is part of correctness.
- **Make evidence portable.** A run should be inspectable without mounting the original filesystem or replaying the agent.
- **Never merge blindly.** Rollback can be a layer discard; commit needs a manifest comparison, conflict policy, and an explicit user/automation decision.
- **Measure the boring costs.** Mount latency, first copy-up, event bytes, cleanup time, and peak storage matter as much as IOPS.
- **Default safe, permit explicit weakening.** `enforce`/rollback and VM-only full-system scope are defaults for demos; audit/off and narrower scopes are explicit choices recorded in the run plan.

## 10. Definition of done for Phase 2

Phase 2 is complete when all of the following are true:

- A killed or crashed agent cannot mutate the lower workspace, and startup recovery is deterministic.
- The complete tested process tree is scoped by cgroup-v2 or the run is explicitly marked as PID-fallback/degraded.
- Sensitive-read policies are user-defined, previewable, enforceable, and versioned.
- Event loss, log rotation, and tampering are visible in the run evidence.
- Rollback is idempotent; commit is conflict-checked and opt-in.
- Warm/cold performance and storage results distinguish FUSE, kernel OverlayFS, telemetry, lifecycle, and copy-up costs.
- A clean disposable VM can reproduce the demo and all P0 tests from the English README.
- Documentation states exactly what is protected and what is outside the rollback boundary.

## 11. Research references

- [Linux OverlayFS documentation](https://docs.kernel.org/filesystems/overlayfs.html) — copy-up, whiteouts, workdir requirements, permissions, and durability.
- [Linux Landlock documentation](https://docs.kernel.org/userspace-api/landlock.html) — unprivileged policy, ABI evolution, and OverlayFS hierarchy semantics.
- [Linux cgroup-v2 documentation](https://docs.kernel.org/admin-guide/cgroup-v2.html) — process-tree identity and delegation model.
- [Linux seccomp user notification documentation](https://docs.kernel.org/userspace-api/seccomp_filter.html) — syscall reduction, notification semantics, and TOCTOU caveats.
- [CRIU checkpoint/restore documentation](https://criu.org/Checkpoint/Restore) — process-tree checkpointing constraints for the future research track.
- [DeltaBox paper](https://arxiv.org/abs/2605.22781) — layered filesystem/process checkpoint and rollback research direction.
