# RewindBPF Feature Backlog

**Status:** canonical delivery ledger  
**Last updated:** 2026-07-20  
**Rule:** a feature is `shipped` only when its code path, unit tests, and (where privileged) disposable-VM evidence exist.

This ledger is the source of truth for the question “is the feature backlog finished?” The public site and other plans summarize this file; they must not turn a contract, scaffold, or fixture into a production capability.

## Delivery status

| Area | Status | What is actually shipped | Remaining acceptance gate |
|---|---|---|---|
| Reversible filesystem transaction | **Shipped** | FUSE OverlayFS transaction prepared before agent start; discard-by-default; rollback, recovery, diff, export, and conflict-checked commit | Broader filesystem compatibility matrix and long-running checkpoint research |
| Sensitive-read policy | **Shipped / Linux** | User-defined glob patterns with `off`/`audit`/`enforce`; Landlock planning and enforcement; policy explain/learn; secret contents are not logged | BPF-LSM acceleration where enabled; content-aware PII classification is post-product |
| Process and resource scope | **Shipped / Linux** | cgroup-v2 scope, descendant drain gate, PID/memory/CPU limits, fail-closed cleanup | Windows Job Object and macOS native process scope |
| eBPF evidence | **Shipped / Linux** | CO-RE trace sensor, start gate, sequence numbers, hash chain, dropped-event accounting, bounded cap, ordered rotation, standalone verifier | Kernel-side backpressure policy and remote signed evidence storage |
| Crash and stale-run recovery | **Shipped / Linux** | Parent death, open descriptors, stale FUSE mount, child drain, idempotent rollback/recover | Power-loss/startup matrix across filesystems |
| Network policy | **Partial** | Explicit loopback HTTP/HTTPS proxy backend; `audit` persists observations and `enforce` applies allow/deny decisions in the run evidence chain for proxy-aware clients; enforce runs also deny raw/packet socket creation with seccomp and record socket refusal events | Network namespace/cgroup egress enforcement, non-proxy-aware client coverage, and broader socket policy |
| Credential safety | **Partial / refusing boundary** | Capability-only references and a broker that refuses raw secret exposure | One real keychain/secret-manager provider with leakage tests and short-lived leases |
| Explicit acceptance | **Shipped / Linux** | Review-only JSON export, text-file unified patch export, and full-fidelity Git patch export plus manifest conflict checks; `rewind commit --confirm`; supervisor commit requires confirmation | Branch adapters and remote review workflow |
| Signed policy provenance | **Shipped / local trust** | Ed25519 keygen/sign/verify policy bundles, persisted envelope re-verification, signer key IDs, and optional supervisor public-key allow-list enforcement | Remote registry, revocation, and organization trust distribution |
| Local supervisor | **Shipped / Linux** | Permissioned Unix socket, token auth, health/capabilities/history, status/rollback/recover/commit, snapshot/follow events, redacted action audit | Detachable process sessions and operator takeover/reconnect |
| Control Plane UI | **Shipped / fixture + authenticated bridge** | Responsive operational views, local policy/workspace config store, loopback HTTP supervisor bridge, bearer-authenticated rollback/recover/commit and policy/workspace writes, authenticated SSE evidence follow with reconnect backoff, signed policy bundle import with verification and audit | Trusted remote registry, local action-token UX |
| Public jury site | **Shipped** | Static modular single-page narrative, honest competitor matrix, roadmap, and measured benchmark ledger | Publish/deploy automation and post-hackathon content updates |
| Linux release/bootstrap | **Shipped / checksummed** | VM bootstrap, release Make targets, cross-build checks, SHA256SUMS and release metadata with explicit unsigned status | External signing workflow and package repository |
| macOS native backend | **Not implemented** | Capability probe and fail-closed unavailable backend only | Seatbelt/EndpointSecurity + APFS disposable-volume implementation and destructive tests on disposable storage |
| Windows native backend | **Not implemented** | Cross-build and fail-closed unavailable backend only; WSL2 remains compatibility mode | Native process/filesystem policy + disposable workspace implementation and Windows VM tests |
| Agent integrations | **Not implemented** | Generic `-- <agent-command>` wrapper | Tested adapters for Codex CLI/OpenHands/Claude Code or a stable adapter SDK |
| Durable remote retention | **Partial / local only** | Bounded local history and keep-latest pruning | Object-store/remote evidence bundles, retention policy, encryption, and restore |
| Detachable/ghost sessions | **Not implemented** | Event follow and history are available after a run; no persistent session owner | Persistent run handles, reconnect, takeover, and session lease expiry |
| Content-aware PII protection | **Not implemented** | Path-pattern protection only | Classifier/redactor with deterministic policy and leakage benchmarks |
| Multi-agent/checkpoint graph | **Not implemented** | One transaction per run | Child transactions, dependency-aware merge, and multiple rewind points |

## What this means before tests

The **Linux demo and product-core feature set is complete enough to enter the verification phase**. The entire long-term backlog is not complete, and it cannot honestly be completed in the current test pass because the native macOS/Windows backends and remote integrations require separate disposable platform environments and product decisions.

Before running the full regression suite, only these implementation items are still in the current Linux scope:

1. Keep the documentation/site status synchronized with the shipped supervisor, commit path, proxy backend, and event rotation.
2. Add and maintain contract tests for every `partial` or `unavailable` capability so unsupported paths fail closed.
3. Run the disposable-VM acceptance matrix and benchmark verification; do not run privileged or destructive tests on the development Mac.

Everything else in the table is a deliberately staged post-demo/productisation item, not a hidden unfinished P0 feature.

## Verification evidence

On 2026-07-20, the disposable Ubuntu 24.04 ARM64 VM passed
`REWIND_VM_CONFIRM=VM_ONLY make acceptance-vm` after rebuilding the Go binary
and eBPF object. The gate covered:

- Landlock sensitive-read denial and recursive delete rollback;
- review plus explicit conflict-checked commit;
- destination drift refusal with no partial apply;
- proxy allow/deny for a local HTTP endpoint and `example.invalid`; and
- bounded-event evidence marked incomplete and rejected by verification.

The network case also persisted one `allow` and one `deny` `network_connect`
event in the ordered hash-chained evidence stream. The same acceptance matrix
verified that an enforce-mode agent receives `EPERM` when creating an IPv4 raw
socket, while the ordinary proxy path remains usable.

The separate supervisor smoke also passed: the mode-`0600` Unix socket returned
`401` without a bearer token, authenticated status and explicit commit succeeded,
and the redacted action audit contained the commit record.

Host-side `go test ./...`, `go vet ./...`, UI syntax checks, and shell syntax
checks also pass. Privileged OverlayFS/eBPF tests remain VM-only by design.

## Recommended next phases

### P0 — Verification gate (current)

Run unit, static, UI, and disposable-VM integration tests; verify rollback, read denial, process drain, evidence integrity, commit conflicts, supervisor auth, and proxy enforcement.

### P1 — Linux productisation

Finish broader network namespace enforcement for non-proxy-aware clients, one real credential provider, patch/branch acceptance adapters, signed release artifacts, and connected Control Plane mutation through a local action-token bridge.

### P2 — Native macOS

Implement and test Seatbelt/EndpointSecurity plus APFS clone/snapshot or disposable workspace. Report a native capability matrix and refuse any unsupported promise.

### P3 — Native Windows

Implement and test Windows-native process/filesystem policy and a disposable workspace. Keep WSL2 explicitly separate from Windows-host protection.

### P4 — Scale and ecosystem

Add detachable sessions, remote retention/registry, agent adapters, multi-agent transaction graphs, content-aware PII controls, and checkpoint research.
