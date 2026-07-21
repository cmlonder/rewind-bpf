# RewindBPF Product Strategy

> **Delivery status:** the Linux transaction/product-core slice and P1 acceptance gates are complete in the disposable Ubuntu VM. Native helper and distributed productisation tracks remain explicitly staged; see [`FEATURE_BACKLOG.md`](FEATURE_BACKLOG.md) for the canonical matrix.

**Status:** adopted after the nono comparison review  
**Decision:** focus on destructive-change safety and sensitive-data safety before pursuing broad sandbox parity

## 1. The strategic correction

RewindBPF must not become a less mature copy of nono. Nono is the stronger general-purpose developer sandbox today: it combines cross-platform isolation, durable undo, policy profiles, network controls, credentials, and session UX.

Our product wedge is narrower and more consequential:

> **Let an agent work aggressively without giving it direct access to the real project or real credentials. Accept only the reviewed result.**

This turns the product from a generic sandbox into a **high-assurance agent transaction runtime**.

## 2. The four user promises

### 2.1 Immutable project

The agent starts in a disposable write layer. Deletes, renames, overwrites, generated files, and dependency changes are visible to the agent but do not touch the protected workspace before explicit acceptance.

This is a filesystem boundary, not a source-code rule. Images, video, audio,
PDFs, archives, binaries, fonts, model files, generated assets, symlinks,
directories, and their metadata are covered when they live inside the protected
workspace. The boundary follows paths, so it also includes files Git has never
tracked.

Default outcome: **discard**, not “remember to rollback.”

### 2.2 Invisible secrets

Sensitive files are not merely blocked after an attempted read; the preferred experience is that they are absent from the agent’s view. User-defined patterns cover `.env`, SSH keys, cloud credentials, certificates, PII directories, and organization-specific paths.

When an agent genuinely needs external access, the opt-in credential broker supplies a short-lived, scoped capability without placing the raw secret in the workspace, argv, or process environment. Provider-specific KMS/IAM wiring remains deployment configuration.

### 2.3 Explicit acceptance

The agent never writes directly into the real workspace. The operator reviews a diff and chooses one of three outcomes:

1. discard the transaction;
2. export a patch/review bundle;
3. apply through a conflict-checked branch or patch workflow.

`commit` is enabled only after the lower-layer manifest and destination state
are checked safely; same-path drift and incomplete evidence refuse the apply.

### 2.4 Fail-closed trust

The runtime must not report a run as safely complete when policy installation, process cleanup, mount cleanup, or evidence capture is incomplete. Dropped events, truncated evidence, unsupported enforcement, and stale descendants are visible failure states.

eBPF, cgroups, Landlock, OverlayFS, hash chains, and recovery are implementation mechanisms for these promises. They are not the headline product features.

### 2.5 RewindBPF and Git solve different problems

Git protects repository history after a developer tracks and commits a change.
RewindBPF protects a live agent run before that history exists. It stages the
whole protected workspace, so an untracked image, generated bundle, binary, or
deleted directory can be reviewed and discarded as part of the same run.

Git is still the system of record for accepted project history. RewindBPF is the
transaction boundary around the risky work that may or may not become a Git
commit. Neither system can undo database writes, cloud/API calls, device state,
or other effects outside the protected filesystem.

## 3. What we deliberately do not chase first

The following are valuable, but are not the initial wedge:

- a large snapshot history browser;
- durable deduplicated checkpoint storage;
- detachable/ghost sessions;
- a remote policy registry;
- generic container/VM replacement;
- broad agent-specific integrations.

We will add them only after the four promises above work on the target platforms. Otherwise we create parity theatre without a stronger safety outcome.

## 4. Product scorecard

The roadmap is measured by user-visible guarantees:

| Guarantee | Demo target | Product target |
|---|---:|---:|
| Direct writes to the protected workspace before acceptance | 0 | 0, verified by manifest/integrity checks |
| Sensitive-read decisions visible in the run record | 100% of tested policy events | 100% of supported backend events |
| Raw credentials present in agent workspace/environment | 0 | 0; brokered access only |
| Runs left with stale mounts or descendants | 0 in tested VM faults | 0 in supported backends |
| Runs falsely marked complete after evidence loss | 0 | 0, fail closed |
| Acceptance after destination drift | blocked | conflict report required |

## 5. Competitive position

The product positioning is:

> **nono is a developer-friendly sandbox and durable undo system. RewindBPF is a high-assurance transaction runtime for agent runs: changes are isolated before execution, secrets are policy-hidden, acceptance is explicit, and incomplete evidence is treated as failure.**

We should not claim that nono lacks kernel enforcement, audit, or portability. We should show the different primary boundary:

| User question | RewindBPF answer |
|---|---|
| “Can the agent delete my project?” | It can delete only its disposable write layer; the real workspace remains untouched. |
| “Can it read my `.env` or SSH key?” | A user-defined read policy denies or hides the path; future brokered access avoids exposing raw credentials. |
| “How do I keep a good result?” | Review first, then export or conflict-checked apply. |
| “How do I know the run is trustworthy?” | The run is incomplete when telemetry, policy, process cleanup, or recovery guarantees are incomplete. |

## 6. Execution order

### Track A — Linux correctness and demo proof

Complete the current Linux transaction: automatic discard semantics, sensitive-read enforcement, process-tree cleanup, evidence completeness, conflict-safe export, and benchmark controls.

### Track B — Confidentiality plane

Add network enforcement and a credential broker. The current product slice has a loopback HTTP/CONNECT proxy backend for proxy-aware clients, narrow seccomp denial for raw/packet sockets in enforce mode, and a refusal-safe credential broker contract. The broker must never pass raw credentials through argv, the agent workspace, or an unrestricted environment variable. Start with one provider and short-lived, scoped tokens; expand only after leakage tests pass. Non-proxy-aware clients and broader egress remain unsupported until a namespace/eBPF backend is available.

### Track C — Explicit acceptance

Implement patch/branch export and conflict-checked apply. Acceptance must compare the starting manifest with the current destination and refuse to overwrite drift.

### Track D — Native platform adapters

The cross-platform implementation is capability-driven and fail-closed. The
shared policy, manifest, evidence, acceptance schemas, platform status matrix,
and signed-helper trust gate are portable; the transaction backend is not
assumed portable. macOS now has an APFS clone + Seatbelt staged filesystem
lifecycle behind the same transaction contract; EndpointSecurity telemetry,
network/resource enforcement, and its signed helper gate remain. Windows has a
code-complete Job Object lifecycle contract behind the signed minifilter/VHDX
helper gate.
WSL2 is explicitly a compatibility path, not Windows-host protection.

Preserve one user-facing contract while using native primitives per platform:

- Linux: OverlayFS/FUSE + Landlock/eBPF + cgroup-v2.
- macOS: Seatbelt + APFS clone/disposable workspace backend; EndpointSecurity
  telemetry and network/resource helper enforcement remain gated.
- Windows: native process/filesystem policy adapter plus a disposable workspace backend; WSL2 remains a compatibility path, not host protection.

Native adapters are not allowed to silently downgrade the four promises. Each
run must display its capability matrix and refuse unsupported enforce modes.
`rewind platform status` makes the distinction explicit between code-complete,
helper-verified, enforcement-ready, and manual-gate states.

### Track E — Durable history and supervisor UX

The current slice includes bounded history, signed policy provenance, a token-authenticated supervisor API with status/rollback/recover/commit actions, follow-mode event streaming, redacted action audit, reconnectable local/SQLite/remote lease contracts, and a browser adapter. Distributed deployment and organization-scale registry/provenance distribution remain post-hackathon.

## 7. Platform roadmap

### Phase P0 — Hackathon

Linux VM only. Ship the destructive-delete → sensitive-read denial → rollback proof, evidence view, benchmark caveats, and fixture Control Plane UI.

### Phase P1 — Linux product core

Ship `rewindd`, persistent policy/workspace state, network enforcement, credential broker MVP, conflict-checked accept, and release packaging.

### Phase P2 — macOS native

The APFS clone + Seatbelt transaction, sensitive-read hiding, review/diff/
rollback/commit lifecycle, and destination conflict checks are implemented and
covered by `make mac-native-smoke`. The remaining work is the signed
EndpointSecurity helper, network/resource enforcement, crash/power-loss
acceptance on disposable storage, and native evidence parity. Do not claim
Linux-equivalent kernel telemetry; report the native capability matrix.

### Phase P3 — Windows native

Implement a Windows backend behind the same interfaces. Validate the same four promises using Windows-native process/filesystem controls and a disposable workspace. WSL2 may remain a supported Linux-compatibility mode, but it must never be described as protection for the Windows host filesystem.

### Phase P4 — Cross-platform history and integrations

Add durable checkpoint history, retention, detachable sessions, remote policy packages, and thin adapters for agent platforms after the safety contract is stable on all target operating systems.

## 8. Architecture consequence

The runtime should expose platform-neutral contracts rather than leaking Linux concepts into the UI:

```text
TransactionBackend
  prepare(workspace, policy) -> transaction
  view(transaction) -> agent-visible workspace
  diff(transaction) -> reviewable changes
  discard(transaction)
  accept(transaction, destination, conflict_policy)
  recover(transaction)

PolicyBackend
  install(policy)
  evaluate(read/write/execute/network intent)
  capabilities()

EvidenceBackend
  stream(events)
  completeness()
  verify(record)
```

The Control Plane UI consumes these contracts. It must show “Linux FUSE backend,” “macOS Seatbelt/APFS backend,” or “Windows native backend” explicitly instead of presenting one misleading universal guarantee.

## 9. Decision rules

1. If a feature does not improve immutable project, invisible secrets, explicit acceptance, or fail-closed trust, it is not a P0.
2. If a platform cannot provide a promise, show degraded mode or refuse the run; never silently weaken enforcement.
3. If a competitor already owns a broad workflow, differentiate on the safety invariant and its evidence rather than copying every surface.
4. Benchmark only equivalent boundaries and label published/non-comparable values.
5. Never run native host destructive tests on a personal machine while the platform backend is experimental.
