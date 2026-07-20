# Phase 2 benchmark protocol

These scenarios extend the B0/B2/B4 filesystem comparison without mixing
startup, cold-cache, and steady-state costs.

| Scenario | Primary metrics | Interpretation |
| --- | --- | --- |
| PII pre-read | scan wall time, files/second, findings, deny decision latency, JSONL bytes | Measures the bounded scanner and Landlock compilation separately from file I/O. Values must include detector set and corpus size. |
| PII post-run | new files scanned, findings, hashed finding bytes, rollback time | Verifies that generated files are covered without exposing raw values. |
| Namespace plan | plan generation time, command count, allow/deny decision latency | The current implementation emits a reviewable veth/NAT plan; it does not claim privileged egress until a disposable VM broker executes it. |
| Remote retention | upload/download wall time, retry count, digest verification, bytes transferred | Use an S3-compatible local gateway or authenticated test server; never benchmark a public bucket with secrets. |
| Checkpoint graph | node transition latency, descendant-first rollback order, dependency refusal rate | Use branching graphs and assert that ambiguous or active descendants fail closed. |
| Adapter lifecycle | prepare/start/exit hook latency and identity propagation | Run each adapter with a deterministic shell fixture; report adapter version and command contract. |

Every result must record kernel, architecture, backend, corpus/workload size,
cache state, repetitions, and whether the number is measured, published, or
not comparable. Missing competitor data stays blank.
