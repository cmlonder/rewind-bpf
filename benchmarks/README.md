# Benchmarks

The benchmark design and B0–B5 comparison groups are defined in [docs/PROJECT_PLAN.md](../docs/PROJECT_PLAN.md).

Planned tools:

- `hyperfine`: macro-level timing
- `fio`: large-file I/O
- `fs_mark`: small-file and metadata workloads
- `perf stat`: CPU and kernel counters
- Go workload runner: deterministic agent scenarios

Results are stored as JSON/CSV under `benchmarks/results/`; that directory is ignored by Git.

The checked-in summary ledger is `results_summary.csv`. Generate a dependency-free SVG chart for the README or presentation with:

```bash
python3 benchmarks/plot_results.py
```

This writes `benchmarks/results_chart.svg`. The script uses only the Python standard library; no plotting package is required.

## Competitor comparison policy

The feature matrix in [`docs/PHASE2_PLAN.md`](../docs/PHASE2_PLAN.md) separates direct product competitors from adjacent kernel systems and research references. Benchmark claims must preserve that distinction:

| System | Comparison class | Reproducible benchmark dimensions | Do not claim |
|---|---|---|---|
| [nono](https://nono.sh/os-sandbox) | Closest direct product competitor | Protected-command startup, policy-denied read latency, steady-state file I/O, undo/rollback time, snapshot/storage growth, audit bytes, and child-process coverage in the same disposable VM | That a different backend or workload proves universal superiority |
| [Cilium Tetragon](https://tetragon.io/docs/getting-started/enforcement/) | Kernel telemetry/enforcement adjacent | Event throughput, CPU/memory overhead, drop behavior, policy decision latency, and process-tree coverage when its supported deployment is reproducible | Filesystem rollback or transaction costs it does not provide |
| [KubeArmor](https://docs.kubearmor.io/kubearmor/quick-links/kubearmor_overview/runtime_enforcer) | Workload policy/enforcement adjacent | Rule-install time, deny latency, event rate, CPU/memory overhead, and enforcement coverage in a matching Kubernetes/VM fixture | Native OverlayFS rollback or local-agent UX parity |
| [AgentFS](https://www.agentfs.ai/) | Filesystem/history alternative | Write throughput, snapshot/restore latency, history query latency, logical bytes, physical bytes, and export size using its documented storage mode | Kernel syscall overhead or native ext4 semantics unless measured directly |
| [DeltaBox](https://arxiv.org/abs/2605.22781) | Research reference | Only numbers from the paper, clearly labeled with workload/kernel/configuration; a local reproduction only if the authors’ artifact is available | Treating published research results as an apples-to-apples product run |

The final presentation will show RewindBPF B0/B2/B4 measurements as the reproducible primary dataset, then a separate competitor table with tool/version, kernel, workload, and whether the number is **measured**, **published**, or **not comparable**. We will not fill missing competitor cells with estimates. This is important because nono, Tetragon, KubeArmor, AgentFS, and DeltaBox optimize different boundaries; a single “benchmark score” would be misleading.
