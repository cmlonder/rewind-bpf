# Benchmarks

The benchmark design and B0–B5 comparison groups are defined in [docs/PROJECT_PLAN.md](../docs/PROJECT_PLAN.md).

Planned tools:

- `hyperfine`: macro-level timing
- `fio`: large-file I/O
- `fs_mark`: small-file and metadata workloads
- `perf stat`: CPU and kernel counters
- Go workload runner: deterministic agent scenarios

Results are stored as JSON/CSV under `benchmarks/results/`; that directory is ignored by Git.
