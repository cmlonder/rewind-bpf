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
| Network policy | **Partial / fail-closed** | Explicit loopback HTTP/HTTPS proxy backend; `audit` persists observations and `enforce` applies allow/deny decisions in the run evidence chain for proxy-aware clients; enforce runs deny raw/packet sockets; explicit `deny` backend refuses non-proxy-aware egress; namespace backend resolves domains, moves a veth peer into the child namespace at the start gate, installs NAT/IPSet/iptables rules, supports atomic DNS/IPSet refresh, and cleans them on every lifecycle path; `rewind network plan` remains reviewable and injectable VM tests cover the command sequence | Periodic refresh scheduling and long-running leak evidence remain operational hardening; privileged VM acceptance is passing |
| Credential safety | **Partial / broker MVP** | Capability-only references, default refusal, opt-in command and native macOS Keychain/Linux Secret Service providers with short-lived one-shot leases, expiry/revoke, no secret in lease JSON/argv/workspace, and an authenticated supervisor `POST /v1/credential-leases` metadata endpoint | Scoped injection protocol and leakage tests against real providers |
| Explicit acceptance | **Shipped / Linux** | Review-only JSON export, text-file unified patch export, full-fidelity Git patch export, manifest conflict checks, and clean-branch `rewind branch apply`; `rewind commit --confirm`; supervisor commit requires confirmation | Remote review workflow and richer provider adapters |
| Signed policy provenance | **Shipped / local trust** | Ed25519 keygen/sign/verify policy bundles, persisted envelope re-verification, signer key IDs, optional supervisor public-key allow-list enforcement, a fail-closed HTTPS registry client with retry, size bound, and pinned-key verification, plus an atomic file registry with list and marker-based revocation (`410 Gone`) | Organization trust distribution, KMS-backed registry durability, and revocation federation |
| Local supervisor | **Shipped / Linux** | Permissioned Unix socket, token auth, health/capabilities/history, status/rollback/recover/commit, snapshot/follow events, redacted action audit, history pruning, and expiring acquire/heartbeat/takeover/release session leases; `--session-backend sqlite` selects the WAL-backed store | Runtime-enforced multi-operator policy and distributed session deployment |
| Control Plane UI | **Shipped / fixture + authenticated bridge** | Responsive operational views, local policy/workspace config store, loopback HTTP supervisor bridge, bearer-authenticated rollback/recover/commit and policy/workspace writes, authenticated SSE evidence follow with reconnect backoff, signed policy bundle import, credential lease metadata flow, retention pruning, detachable session controls, one-time in-memory action-token challenges, and trusted registry metadata/verification UX | Connected registry endpoint and server-side action-intent policy |
| Public jury site | **Shipped** | Static modular single-page narrative, honest competitor matrix, roadmap, measured normalized benchmark ledger, storage/evidence/lifecycle callouts, and explicit local/S3 publish automation | External hosting credentials and post-hackathon content updates |
| Linux release/bootstrap | **Shipped / signed locally** | VM bootstrap, release Make targets, cross-build checks, SHA256SUMS, release metadata, and detached Ed25519 signature/verification with optional pinned public key | Public registry trust, key rotation/revocation, and package repository |
| macOS native backend | **Prepared / manual gate** | Read-only APFS/Seatbelt/diskutil prerequisite plan, platform CLI, fail-closed capability report, Seatbelt profile/command wrapper, and native contract; no destructive operation is enabled | EndpointSecurity telemetry, APFS disposable-volume rollback, signed helper, and destructive tests on disposable storage |
| Windows native backend | **Prepared / manual gate** | Cross-build, fail-closed capability report, read-only PowerShell/fsutil prerequisite plan, Job Object/restricted-token contract, and kill-on-close Job Object helper; WSL2 remains compatibility mode | Signed filesystem minifilter, restricted-token launch integration, disposable VHDX workspace, and Windows VM tests |
| Agent integrations | **Partial / lifecycle contract** | `--agent-adapter` validates and persists identity; adapter registry records executable aliases and `rewind/v1` prepare/start/exit hook contract; run IDs are injected into the child environment; `rewind agent list|contract` exposes the registry | SDK-specific launch semantics, callbacks, and provider tests |
| Durable remote retention | **Partial / encrypted signed hand-off** | Bounded local history and keep-latest pruning, AES-256-GCM encrypted evidence envelopes, checksum-indexed archives, detached Ed25519 signatures, multi-key trust rotation, explicit HTTPS publish, and digest-pinned fetch/restore | Object-store durability, KMS-backed key lifecycle, and real-provider restore automation |
| Detachable/ghost sessions | **Partial / local + SQLite + remote protocol** | Expiring authenticated acquire/heartbeat/takeover/release owner leases, reconnectable event follow, redacted session audit, atomic cross-process lock file, bearer-authenticated remote lease client with bounded retries, and a WAL-backed SQLite store with expiry/ownership tests | Distributed deployment/consensus and operational migration tooling |
| Content-aware PII protection | **Partial / bounded lifecycle coverage** | Deterministic bounded scanner detects common PII/token patterns; audit findings contain hashes only; `read.pii.mode: enforce` turns findings into exact Landlock denies before agent start; protected runs rescan newly-created files after exit; event paths and proxy hosts are redacted before evidence persistence; configurable regex rules, streaming limits, and no-leakage tests | Configurable classifiers beyond regex, VM leakage benchmark runs, and richer event-level finding metadata |
| Multi-agent/checkpoint graph | **Partial / live lifecycle foundation** | Durable dependency graph with parent validation, deterministic state transitions, pending-child merge guard, descendant-first rollback, CLI transitions, and protected-run lifecycle wiring | Full multi-agent orchestration, durable remote graph store, and process-memory checkpoints |

## What this means before tests

The **Linux demo and product-core feature set is complete enough to enter the verification phase**. The entire long-term backlog is not complete, and it cannot honestly be completed in the current test pass because the native macOS/Windows backends and remote integrations require separate disposable platform environments and product decisions.

Before running the full regression suite, only these implementation items are still in the current Linux scope:

1. Keep the documentation/site status synchronized with the shipped supervisor, commit path, proxy backend, event rotation, branch acceptance, evidence bundles, normalized benchmark ledger, and trust UX.
2. Add and maintain contract tests for every `partial` or `unavailable` capability so unsupported paths fail closed.
3. Run the disposable-VM acceptance matrix and `make benchmark-verify`; do not run privileged or destructive tests on the development Mac.

Everything else in the table is a deliberately staged post-demo/productisation item, not a hidden unfinished P0 feature.

## Verification evidence

On 2026-07-20, the disposable Ubuntu 24.04 ARM64 VM passed
`REWIND_VM_CONFIRM=VM_ONLY make final-vm` after bootstrapping packages,
rebuilding the Go binary
and eBPF object. The gate covered:

- Landlock sensitive-read denial and recursive delete rollback;
- review plus explicit conflict-checked commit;
- destination drift refusal with no partial apply;
- proxy allow/deny for a local HTTP endpoint and `example.invalid`; and
- strict deny/no-route namespace isolation plus real allow-listed namespace
  egress through a temporary veth/IPSet/NAT chain; and
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

The new content-aware gate also passed in the disposable Ubuntu 24.04 ARM64 VM
on 2026-07-20. A synthetic file containing `alice@example.com` was denied by
`read.pii.mode: enforce` before the agent could read it, while an ordinary
generated file remained writable in the merged layer; rollback preserved the
original lower file.

Host-side `go test ./...`, `go vet ./...`, UI syntax checks, and shell syntax
checks also pass. Privileged OverlayFS/eBPF tests remain VM-only by design.

## Recommended next phases

### P0 — Verification gate (current)

Run unit, static, UI, and disposable-VM integration tests; verify rollback, read denial, process drain, evidence integrity, commit conflicts, supervisor auth, and proxy enforcement.

### P1 — Linux productisation

The local fail-closed network boundary, signed evidence hand-off, release signing,
authenticated supervisor, connected Control Plane mutation, isolated Linux network
namespace backend, and opt-in command-provider credential lease endpoint are
shipped. The remaining P1 gates are operational namespace/cgroup egress refresh
and long-running leak tests, a real platform credential provider, and remote review/object storage with retention,
encryption, and trust rotation.

### P2 — Native macOS

The read-only native prerequisite plan and capability CLI are prepared. The
remaining gate is manual: validate Seatbelt/EndpointSecurity plus APFS
clone/snapshot or disposable workspace on disposable storage, then wire the
enforcing process adapter. Report a native capability matrix and refuse any
unsupported promise.

### P3 — Native Windows

Implement and test Windows-native process/filesystem policy and a disposable workspace. Keep WSL2 explicitly separate from Windows-host protection.

### P4 — Scale and ecosystem

Add distributed detachable sessions, remote retention/registry, SDK-specific agent adapters, multi-agent transaction graphs, runtime content-aware PII controls, and checkpoint research. Local leases, encrypted hand-off, adapter identity, and deterministic audit scanning are already shipped.
