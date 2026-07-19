# Competitor benchmark matrix

This file is the evidence ledger for external comparisons. It intentionally separates product capabilities from measurements. A blank cell means “not measured,” never zero.

## Provenance contract

Every external number must include:

- tool and version;
- kernel, architecture, filesystem, and backend;
- workload command and policy configuration;
- repetition count and warm/cold order;
- raw artifact or public source URL; and
- provenance: `measured`, `published`, or `not-comparable`.

The primary RewindBPF dataset remains B0/B2/B4 in [`RESULTS.md`](RESULTS.md). External systems are not ranked when their safety boundary or workload is different.

## Feature and benchmark scope

| System | Relationship | Feature overlap | Fair benchmark dimensions | Explicitly out of scope |
|---|---|---|---|---|
| [nono](https://nono.sh/os-sandbox) | Direct product competitor | Landlock/Seatbelt isolation, profiles, undo, audit, child inheritance | Startup, denied-read latency, mixed I/O, undo latency, storage growth, audit bytes, fork/exec coverage | Universal “faster” claim across different backends |
| [Cilium Tetragon](https://tetragon.io/docs/getting-started/enforcement/) | Kernel telemetry/enforcement adjacent | eBPF process/file/network observation and enforcement | Event rate, CPU/memory, drop behavior, policy decision latency, process coverage | Filesystem rollback and COW cost |
| [KubeArmor](https://docs.kubearmor.io/kubearmor/quick-links/kubearmor_overview/runtime_enforcer) | Workload policy adjacent | Runtime allow/deny policy and telemetry | Rule install, deny latency, event rate, CPU/memory, enforcement coverage | Local agent transaction UX and rollback |
| [AgentFS](https://www.agentfs.ai/) | Filesystem/history alternative | Agent-scoped filesystem, snapshots, history, export | Write throughput, snapshot/restore/query latency, logical/physical bytes, export size | Native syscall and OverlayFS overhead unless directly measured |
| [DeltaBox](https://arxiv.org/abs/2605.22781) | Research reference | Layered filesystem/process checkpoint and rollback | Published paper values with exact configuration; artifact reproduction if available | Treating research numbers as a product shootout |

## Results ledger

| System | Tool/version | Workload | Metric | Value | Provenance | Source/artifact | Notes |
|---|---|---|---|---:|---|---|---|
| RewindBPF B0 | fio 3.36 | 128 MiB randrw, 70/30, 4 KiB, iodepth 1 | read IOPS | 10,334.2 | measured | [`RESULTS.md`](RESULTS.md) | Native ext4 VM baseline |
| RewindBPF B2 | fio 3.36 | Same | read IOPS | 9,143.8 | measured | [`RESULTS.md`](RESULTS.md) | FUSE OverlayFS only |
| RewindBPF B4 | fio 3.36 | Same | read IOPS | 9,181.7 | measured | [`RESULTS.md`](RESULTS.md) | Protected run, FUSE + eBPF + helper |
| nono | — | — | — | — | not-comparable | [nono docs](https://nono.sh/) | Add only after a reproducible VM run |
| Tetragon | — | — | — | — | not-comparable | [Tetragon docs](https://tetragon.io/docs/getting-started/enforcement/) | Requires its supported deployment |
| KubeArmor | — | — | — | — | not-comparable | [KubeArmor docs](https://docs.kubearmor.io/kubearmor/quick-links/kubearmor_overview/runtime_enforcer) | Requires a matching workload environment |
| AgentFS | — | — | — | — | not-comparable | [AgentFS](https://www.agentfs.ai/) | Storage model differs |
| DeltaBox | — | — | — | — | published/not-comparable | [paper](https://arxiv.org/abs/2605.22781) | Use paper values only with configuration |

## Presentation rule

The jury slide should show RewindBPF’s measured B0/B2/B4 bars and a separate competitor capability/provenance table. It should not combine unlike measurements into one leaderboard. The honest claim is: **the complete safety transaction has a measured, explainable cost, and every external comparison states exactly what was and was not measured.**
