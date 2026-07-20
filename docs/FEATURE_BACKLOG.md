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
| Network policy | **Partial / fail-closed** | Explicit loopback HTTP/HTTPS proxy backend; `audit` persists observations and `enforce` applies allow/deny decisions in the run evidence chain for proxy-aware clients; enforce runs also deny raw/packet socket creation with seccomp; the explicit `deny` backend refuses IPv4/IPv6/packet sockets and connect attempts for non-proxy-aware clients | Allow-listed network namespace/cgroup egress enforcement and broader non-proxy-aware allow coverage |
| Credential safety | **Partial / broker MVP** | Capability-only references, default refusal, and an opt-in external command provider with short-lived one-shot leases, expiry, revoke, and no secret in lease JSON/argv/workspace | Native keychain/secret-manager adapters, scoped injection protocol, and leakage tests against real providers |
| Explicit acceptance | **Shipped / Linux** | Review-only JSON export, text-file unified patch export, full-fidelity Git patch export, manifest conflict checks, and clean-branch `rewind branch apply`; `rewind commit --confirm`; supervisor commit requires confirmation | Remote review workflow and richer provider adapters |
| Signed policy provenance | **Shipped / local trust** | Ed25519 keygen/sign/verify policy bundles, persisted envelope re-verification, signer key IDs, and optional supervisor public-key allow-list enforcement | Remote registry, revocation, and organization trust distribution |
| Local supervisor | **Shipped / Linux** | Permissioned Unix socket, token auth, health/capabilities/history, status/rollback/recover/commit, snapshot/follow events, redacted action audit | Detachable process sessions and operator takeover/reconnect |
| Control Plane UI | **Shipped / fixture + authenticated bridge** | Responsive operational views, local policy/workspace config store, loopback HTTP supervisor bridge, bearer-authenticated rollback/recover/commit and policy/workspace writes, authenticated SSE evidence follow with reconnect backoff, signed policy bundle import with verification and audit | Trusted remote registry, local action-token UX |
| Public jury site | **Shipped** | Static modular single-page narrative, honest competitor matrix, roadmap, and measured benchmark ledger | Publish/deploy automation and post-hackathon content updates |
| Linux release/bootstrap | **Shipped / signed locally** | VM bootstrap, release Make targets, cross-build checks, SHA256SUMS, release metadata, and detached Ed25519 signature/verification with optional pinned public key | Public registry trust, key rotation/revocation, and package repository |
| macOS native backend | **Prepared / manual gate** | Read-only APFS/Seatbelt/diskutil prerequisite plan, platform CLI, and fail-closed capability report; no destructive operation is enabled | Native Seatbelt/EndpointSecurity + APFS disposable-volume implementation and destructive tests on disposable storage |
| Windows native backend | **Not implemented** | Cross-build and fail-closed unavailable backend only; WSL2 remains compatibility mode | Native process/filesystem policy + disposable workspace implementation and Windows VM tests |
| Agent integrations | **Not implemented** | Generic `-- <agent-command>` wrapper | Tested adapters for Codex CLI/OpenHands/Claude Code or a stable adapter SDK |
| Durable remote retention | **Partial / signed hand-off** | Bounded local history and keep-latest pruning plus checksum-indexed `rewind bundle create`/`verify` archives, detached Ed25519 signatures, and explicit HTTPS `rewind bundle publish` with optional pinned-key verification | Object-store durability, retention policy, encryption, key rotation, and restore |
| Detachable/ghost sessions | **Not implemented** | Event follow and history are available after a run; no persistent session owner | Persistent run handles, reconnect, takeover, and session lease expiry |
| Content-aware PII protection | **Not implemented** | Path-pattern protection only | Classifier/redactor with deterministic policy and leakage benchmarks |
| Multi-agent/checkpoint graph | **Not implemented** | One transaction per run | Child transactions, dependency-aware merge, and multiple rewind points |

## What this means before tests

The **Linux demo and product-core feature set is complete enough to enter the verification phase**. The entire long-term backlog is not complete, and it cannot honestly be completed in the current test pass because the native macOS/Windows backends and remote integrations require separate disposable platform environments and product decisions.

Before running the full regression suite, only these implementation items are still in the current Linux scope:

1. Keep the documentation/site status synchronized with the shipped supervisor, commit path, proxy backend, event rotation, branch acceptance, and evidence bundles.
2. Add and maintain contract tests for every `partial` or `unavailable` capability so unsupported paths fail closed.
3. Run the disposable-VM acceptance matrix and `make benchmark-verify`; do not run privileged or destructive tests on the development Mac.

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
socket, while the ordinary proxy path remains usable. A second enforce-mode
case verified the new `--network-backend deny` path: IPv4 socket creation was
refused while Unix-domain socket creation remained available.

The separate supervisor smoke also passed: the mode-`0600` Unix socket returned
`401` without a bearer token, authenticated status and explicit commit succeeded,
and the redacted action audit contained the commit record.

Host-side `go test ./...`, `go vet ./...`, UI syntax checks, and shell syntax
checks also pass. Privileged OverlayFS/eBPF tests remain VM-only by design.

## Recommended next phases

### P0 — Verification gate (current)

Run unit, static, UI, and disposable-VM integration tests; verify rollback, read denial, process drain, evidence integrity, commit conflicts, supervisor auth, and proxy enforcement.

### P1 — Linux productisation

The local fail-closed network boundary, signed evidence hand-off, release signing,
authenticated supervisor, and connected Control Plane mutation are shipped. The
remaining P1 gates are an allow-listed network namespace/cgroup backend for
non-proxy-aware clients, one real credential provider, and remote review/object
storage with retention, encryption, and trust rotation.

### P2 — Native macOS

The read-only native prerequisite plan and capability CLI are prepared. The
remaining gate is manual: validate Seatbelt/EndpointSecurity plus APFS
clone/snapshot or disposable workspace on disposable storage, then wire the
enforcing process adapter. Report a native capability matrix and refuse any
unsupported promise.

### P3 — Native Windows

Implement and test Windows-native process/filesystem policy and a disposable workspace. Keep WSL2 explicitly separate from Windows-host protection.

### P4 — Scale and ecosystem

Add detachable sessions, remote retention/registry, agent adapters, multi-agent transaction graphs, content-aware PII controls, and checkpoint research.
