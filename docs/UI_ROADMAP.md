# Rewind Control Plane UI Roadmap

Status: Fixture control plane delivered; authenticated loopback supervisor bridge and local config writes available
Audience: hackathon jurors, security-minded engineers, and developers running autonomous agents on Linux

## 1. Product decision

RewindBPF needs more than a read-only dashboard. The UI is a control plane for a reversible filesystem transaction:

```text
Observe → Configure → Package → Assign → Run → Review → Rollback
```

The UI must make three questions answerable immediately:

1. What is the agent doing right now?
2. What changed in the filesystem and what was denied?
3. Can the operator safely rewind this run?

The UI is not the security boundary. eBPF, Landlock, OverlayFS/FUSE, cgroup-v2, lifecycle recovery, and evidence persistence remain in the Linux runtime and supervisor. The UI sends validated intents to a local supervisor and never performs privileged filesystem or kernel operations itself.

## 1.1 P0 user-outcome surface

The UI follows the same four P0 promises as the runtime:

| Promise | Required UI behavior |
|---|---|
| Immutable project | Show `DISCARD BY DEFAULT`, lower-layer integrity, upper-layer bytes, and an explicit review hold. |
| Invisible secrets | Show the matched pattern and decision without rendering secret contents; expose degraded/refused backend state. |
| Explicit acceptance | Enable commit only for succeeded review runs, show the conflict gate, and keep export/discard available. |
| Fail-closed trust | Surface dropped events, truncation, stale descendants, mount failures, and recovery progress as actionable states. |

## 2. Users and jobs to be done

### Hackathon juror

- Understand the lower/upper/merged invariant in under one minute.
- Watch an agent delete or overwrite files in a live timeline.
- See that the lower layer remains intact.
- Trigger rollback and see a verified restoration.
- Distinguish measured evidence from roadmap promises.

### Security engineer

- Inspect policy decisions, denied sensitive reads, descendant processes, and resource limits.
- Verify event completeness, dropped records, truncation, hash-chain validity, and record matching.
- Review configuration revisions and administrative actions.
- Confirm that unsupported enforcement backends fail closed.

### Developer/operator

- Assign a safe policy package to a workspace.
- Preview the effective policy before starting an agent.
- Review created, modified, deleted, and renamed files.
- Export a review bundle or recover a stale run without typing long commands.

## 3. Information architecture

The operational application is separate from the public jury-facing project site. The public site explains the project; the Control Plane operates it.

Primary navigation:

- **Overview** — active runs, health, recent actions, and the next recommended action.
- **Runs** — searchable run history and lifecycle state.
- **Policies** — policy package catalog, validation, simulation, and version history.
- **Workspaces** — workspace-to-policy assignments and defaults.
- **Global Config** — defaults, retention, telemetry, backend, and resource settings.
- **Evidence** — event integrity, journal segments, diffs, and exports.
- **Benchmarks** — B0/B2/B4 evidence and storage/latency measurements.
- **Audit Log** — configuration, assignment, rollback, recover, and export actions.

## 4. Phase 1: live transaction console

Phase 1 is the first implementation slice. It is fixture-driven and safe to run on a development host; no kernel, mount, or destructive operation is performed.

### 4.1 App shell

- Editorial technical-lab visual language: warm paper, ink navy, safety orange, sage verification.
- Persistent left rail on desktop; compact top bar and bottom action area on narrow screens.
- Environment banner clearly identifies fixture mode versus connected local supervisor mode.
- Keyboard-visible focus states, skip link, semantic landmarks, and reduced-motion behavior.

### 4.2 Overview

The overview should prioritize state, not a wall of cards:

- active run status and elapsed time;
- lower-layer integrity;
- evidence completeness;
- upper-layer bytes and process count;
- recent high-risk events;
- recommended next action.

### 4.3 Run detail

The run detail screen is the jury climax and the daily operator surface.

Header fields:

- run ID and lifecycle state (`running`, `succeeded`, `failed`, `rolled_back`, `recovery_required`);
- policy package and version;
- overlay backend;
- cgroup/resource status;
- evidence status;
- start and end time.

Panels:

- **Timeline** — grouped event stream with timestamps, operation, risk, path, decision, and lifecycle markers.
- **Filesystem diff** — created, modified, deleted, renamed, and policy-denied paths without secret contents.
- **Process tree** — cgroup descendants and resource budget usage.
- **Evidence** — count, bytes, dropped, truncated, chain validity, record match, and rotated journal count.
- **Action rail** — rollback, recover, export, and disabled commit state.

### 4.4 Safe actions

- `Rollback` shows an impact summary before execution: upper bytes discarded, lower layer preserved, and evidence retained.
- `Recover` is available only for stale or recovery-required runs and explains descendant draining.
- `Export` is review-only and can be performed without changing the workspace.
- `Commit` remains visibly unavailable until conflict-checked merge semantics exist.
- Every action has loading, success, error, disabled, hover, active, and keyboard-focus states.

## 5. Phase 2: policy and configuration control plane

The UI must manage the safety contract for future runs, not mutate an active run behind the operator’s back.

### 5.1 Global configuration

Editable defaults:

- overlay backend (`fuse` or compatible kernel backend);
- default read mode (`off`, `audit`, `enforce`);
- default write mode and scope;
- telemetry total cap (`REWIND_EVENT_MAX_BYTES`);
- telemetry rotation size (`REWIND_EVENT_ROTATE_BYTES`);
- evidence retention;
- runtime roots;
- default cgroup PID, memory, and CPU budgets;
- network default (`off` or `audit` until an enforceable backend exists);
- recovery and drain timeouts;
- feature flags and capability requirements.

Rules:

- Config changes apply only to new runs.
- Each revision stores author, timestamp, before/after diff, validation result, and reason.
- Invalid or unsupported combinations fail closed.
- A running run retains an immutable config snapshot.

### 5.2 Policy packages

Policies become versioned, reviewable packages rather than loose YAML files. Initial built-in examples:

- `strict-agent`;
- `developer-safe`;
- `ci-build`;
- `secrets-protected`;
- `readonly-investigation`;
- `network-restricted`;
- `hackathon-demo`.

Package operations:

- create, duplicate, edit, validate, explain, simulate, compare versions, assign, export/import, archive, and restore a previous revision;
- never auto-apply a learned policy;
- never show secret contents;
- surface backend requirements and unsupported capabilities before assignment.

Suggested package shape:

```yaml
name: strict-agent
version: 1.3.0
description: High-safety profile for autonomous coding agents

read:
  mode: enforce
  deny:
    - "**/*.env"
    - "**/*.pem"
    - "**/*.key"
    - "/home/*/.ssh/**"

write:
  mode: rollback
  scope: workspace

network:
  mode: audit

resources:
  pids_max: "256"
  memory_max: "536870912"
  cpu_max: "50000 100000"
```

### 5.3 Effective policy

The UI resolves and explains precedence:

```text
Global defaults
   ↓
Profile/package
   ↓
Workspace assignment
   ↓
Session override
   ↓
Effective policy snapshot
```

The user can ask, for example, `/workspace/.env` and see `DENY`, the matching glob, the deny-before-allow precedence, and the active enforcement backend.

### 5.4 Workspaces

Workspace records contain:

- workspace path;
- assigned policy package and revision;
- allowed runtime roots;
- agent command template;
- resource and retention defaults;
- network profile;
- auto-rollback behavior.

## 6. Policy simulation and review

Before enabling `enforce`, a package can run against a synthetic fixture or historical event log:

```text
.env                  DENY
.ssh/id_rsa           DENY
src/main.go           ALLOW
build/output.bin      ALLOW
/tmp/cache            AUDIT
network: api.github   AUDIT
```

The simulation answers what would have been blocked, which allow rule is missing, and where an agent would fail. Learned suggestions remain audit-mode and require explicit human acceptance.

## 7. Evidence and benchmark surfaces

### Evidence

Show evidence as a verification statement, not a decorative green badge:

```text
The evidence stream is complete and matches the persisted run record.
51 events · 17.8 KiB · 0 dropped · 0 truncated
51 journal segments · chain valid
```

### Benchmarks

The benchmark lab visualizes:

- B0 native ext4;
- B1 eBPF-only;
- B2 FUSE-only;
- B3 FUSE + eBPF;
- B4 protected path;
- B5 protected path + policy.

Charts are decision-support visuals: IOPS bars, p50/p95/p99 latency plots, logical-changed versus physical-upper bytes, telemetry growth, rollback duration, and cleanup duration. External competitor numbers remain marked as measured, published, or not comparable.

## 8. Supervisor and API boundary

The UI uses an adapter so fixture data and a real supervisor share the same view model:

```text
ui/data/fixture-adapter.js
ui/data/api-adapter.js
        ↓
Control Plane view model
        ↓
components
```

Planned local API:

```text
GET  /api/runs
GET  /api/runs/:id
GET  /api/runs/:id/events
GET  /api/runs/:id/diff
GET  /api/runs/:id/evidence
GET  /api/policies
GET  /api/policies/:id/effective
GET  /api/config
GET  /api/audit
POST /api/runs/:id/rollback
POST /api/runs/:id/recover
POST /api/runs/:id/export
POST /api/policies/:id/simulate
POST /api/config/revisions
```

Live events use SSE first, with polling as a fallback. The supervisor owns Unix-socket access, capability checks, action authorization, and audit logging. The browser is never given root access.

## 9. Safety, privacy, and failure UX

- Bind the connected UI to localhost or a Unix socket; do not expose it by default.
- Use same-origin/CSP protections and an action token for state-changing requests.
- Never render secret file contents, raw credentials, or environment values.
- Treat path metadata as sensitive and redact where the policy requires it.
- Require an impact summary for rollback/recover and a conflict report before commit.
- Keep UI failure independent from runtime safety; a crashed browser must not stop recovery.
- Explain unsupported capabilities and unavailable actions in plain technical language.

## 10. Motion and interaction system

- 500–700 ms staggered entrance for the initial control-room load.
- 180–260 ms timeline event entry and status changes.
- 220–300 ms page and drawer transitions.
- 400–500 ms rollback lifecycle transition: requested → unmounting → discarded → restored.
- Animate transform and opacity; use grid-row transitions for disclosure.
- Avoid bounce, elastic easing, continuous decorative glow, and motion that hides latency.
- Respect `prefers-reduced-motion` and preserve functional status feedback.
- Implement default, hover, focus-visible, active, disabled, loading, error, and success states for every action.

## 11. Current implementation status

The dependency-free `ui/` prototype now covers the Phase 1 control room and the fixture-safe portion of Phases 2–3:

- transaction search/filtering and run-detail navigation;
- rollback, recover, and export confirmation dialogs with explicit impact copy;
- policy package creation, selection, preview copy, and deterministic simulation;
- workspace creation/editing, policy assignment, and boundary-test feedback;
- revisioned global configuration controls with active-run isolation;
- evidence, benchmark, and audit surfaces suitable for a jury walkthrough;
- responsive navigation, focus-visible controls, reduced-motion support, and modal Escape handling.
- keyboard focus trapping/restoration, mobile navigation that preserves all destinations, notification feedback, empty-search states, and constrained form validation.

The remaining connected work is end-to-end reconnect/recovery behavior and
trusted policy distribution. P4 now has a bounded history contract, signed policy
provenance, a token-authenticated Unix-socket server, an optional loopback HTTP
bridge, snapshot and follow-mode event endpoints, local policy/workspace config
writes, a browser adapter, and a fixture-backed retention view. The socket,
HTTP bridge, and bearer token are protected by explicit loopback/mode `0600`
boundaries; authenticated status, rollback/recover, commit, policy creation,
and workspace assignment route through supervisor code with redacted JSONL
action audit. Local authentication/authorization beyond that token boundary is
explicitly a post-demo hardening item; the hackathon UI remains fixture-safe and
never receives root access.

The UI is being kept platform-neutral. It must show the active backend and capability matrix explicitly: Linux OverlayFS/FUSE + Landlock/eBPF, macOS Seatbelt/EndpointSecurity + APFS/disposable workspace, or Windows native process/filesystem policy + disposable workspace. A platform adapter that cannot provide one of the four product promises must render degraded/refused state rather than silently presenting Linux-level guarantees.

## 12. Implementation phases and exit criteria

### Phase 1 — Fixture-driven control room — delivered

Deliver Overview, Runs, Run Detail, timeline, filesystem diff, evidence panel, action rail, fixture adapter, responsive shell, and accessibility baseline.

Exit: a juror can follow a synthetic destructive run from start to rollback without reading the CLI.

### Phase 2 — Safe local actions — fixture slice delivered

Add rollback, recover, export, progress states, native dialog confirmations, and action audit fixtures.

Exit: every action has a visible safety explanation and deterministic success/error state.

### Phase 3 — Policy and configuration — fixture slice delivered

Add global config, policy package CRUD, workspace assignments, effective-policy resolver, policy simulation, revision history, and audit log.

Exit: an operator can create a package, simulate it, assign it to a workspace, and see the immutable snapshot used by a new run.

### Phase 4 — Supervisor integration — authenticated action/config boundary delivered

The current bridge exposes a mode-0600 Unix socket, an optional loopback HTTP
bridge, bearer-authenticated history/events, lifecycle actions, policy/workspace
config writes, follow-mode streams, and persistent redacted action audit. Add
recovery/reconnect behavior and signed policy upload UX. Add local authentication
beyond the socket/token boundary after the demo unless the connected deployment
requires it earlier.

Exit: UI state matches runtime state under refresh, reconnect, supervisor restart, and recovery scenarios.

### Phase 5 — Product surface

Add remote package registry, organization profiles, role-based approval, signed/remote evidence, Linux packaging, and conflict-checked commit.

## 13. What the UI must not become

- A generic card-grid SaaS dashboard.
- A terminal emulator with root privileges.
- A command deny-list editor pretending to be a transaction boundary.
- A green “safe” badge that hides dropped or truncated evidence.
- A commit button without conflict-safe merge semantics. The button is enabled only for a succeeded review run and requires an impact summary plus explicit confirmation.
- A second, divergent policy source that disagrees with the CLI/runtime.

## 14. First code slice

The first implementation keeps the current dependency-free ES module approach and adds a separate `ui/` application. It ships:

1. Overview with an active synthetic run.
2. Run detail with animated lifecycle timeline.
3. Filesystem diff and evidence verification panels.
4. Safe action rail with fixture-backed rollback/recover/export states.
5. Policy package preview and effective-policy panel as the first control-plane preview.

The static public site remains the project narrative. `ui/` becomes the operational product surface and will later connect to `rewindd` without changing the component view model.
