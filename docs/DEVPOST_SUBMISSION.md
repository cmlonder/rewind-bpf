# Devpost Submission Pack

**Target event:** OpenAI Build Week 2026
**Recommended track:** Developer Tools
**Project:** RewindBPF — Reversible Safety Runtime for AI Agents
**Submission status:** engineering package ready; external submission steps remain operator-owned

This document is the copy-ready checklist for the Devpost entry. It intentionally separates repository evidence from actions that require the entrant's Devpost, YouTube, or Codex account.

## Product position

RewindBPF is a Linux-first, agent-agnostic safety runtime. It starts an agent inside a disposable filesystem transaction, applies user-defined read/write/network policy, records lifecycle evidence, and lets an operator review, commit, or discard the result. The product demo can run locally on macOS through the native transaction path and Control Plane UI. The Linux reference path adds OverlayFS, Landlock, cgroup, and eBPF enforcement inside a disposable Ubuntu 24.04 VM; Windows remains a fail-closed contract.

This is a developer tool, not an MCP server or an agent SDK plugin. The agent command remains the operator's command. RewindBPF supplies the execution boundary around it. A future MCP/SDK adapter can call the same CLI and supervisor contracts, but adding one is not required for this submission and would not improve the core jury demo at the deadline.

## Copy-ready project description

AI agents are good at changing files and bad at understanding the blast radius of a destructive command. RewindBPF gives an agent a reversible work session instead of direct access to the real project: copy-on-write keeps writes in a disposable layer, policy rules deny configured sensitive reads, and the supervisor records the run. The boundary is workspace-wide rather than code-only, so images, media, binaries, generated assets, and untracked files are candidates for the same review and rollback. The operator can inspect a manifest and timeline, reject the run in one rollback, or accept it through an explicit conflict-checked commit. The recorded Mac demo uses a synthetic project: the agent deletes `src/`, attempts to read a synthetic `.env`, creates output, and the original source is restored without touching the real workspace. The Ubuntu VM scenario demonstrates the additional Linux enforcement layers.

The central claim is deliberately narrower than “zero overhead”: copy-on-write avoids a full pre-run copy, while the benchmark ledger reports the measured B0/B2/B4/B5 throughput, latency, storage, telemetry, and lifecycle costs. The supported boundary is explicit: RewindBPF does not undo external databases, cloud APIs, devices, or arbitrary kernel side effects.

## How Codex and GPT-5.6 contributed

RewindBPF was built and iterated in Codex using GPT-5.6 as a primary implementation and review partner. Codex accelerated the work in four concrete places:

1. It decomposed the runtime into testable Go modules (policy, Landlock, OverlayFS/FUSE, eBPF loading, telemetry, lifecycle, evidence, supervisor, retention, and platform contracts) instead of one monolithic runner.
2. It helped implement and debug the protected-run lifecycle, including descendant cleanup, crash recovery, incomplete-evidence fail-closed behavior, destination-drift commit refusal, and the authenticated supervisor bridge.
3. It generated and iterated the Control Plane UI, public site, benchmark normalization/charts, release scripts, and deterministic VM acceptance gates.
4. It was used to challenge product claims against nono, Tetragon, KubeArmor, AgentFS, and DeltaBox, which led to the current narrower and evidence-backed positioning.

GPT-5.6 is used during development and verification, not as a hidden runtime dependency: RewindBPF remains model-agnostic and can protect a Codex, OpenHands, Claude, or arbitrary command through the same operator-owned launch boundary. This distinction is intentional; the safety invariant must not depend on a particular model vendor.

**Primary Codex `/feedback` Session ID:** `019f6f87-53d3-7c11-be4d-6d07217d62ea`

To obtain the required ID, run `/status` in the Codex thread where most of the core runtime was built. Do not invent or substitute a side-thread ID.

## Supported platforms and judge test path

| Platform | Submission claim | Judge path |
|---|---|---|
| Ubuntu 24.04 ARM64 in UTM | Reference Linux enforcement and rollback path | `REWIND_VM_CONFIRM=VM_ONLY make final-vm` inside the disposable VM |
| macOS | Safe native APFS/Seatbelt staged transaction and UI bridge | `make mac-native-smoke`, `make mac-crash-smoke`, then `go run ./cmd/rewind dashboard start --workspace /Users/Shared/<fixture>` |
| Windows | Cross-built, fail-closed native contract; signed minifilter/VHDX gate remains | Inspect `docs/platform/windows.md` and `scripts/windows_acceptance.ps1`; do not claim Linux-equivalent enforcement |

The fastest judge experience is the local macOS dashboard flow using a disposable fixture. The Linux VM jury scenario is the enforcement reference. Both paths use synthetic data and do not require a real project, real secrets, or a host filesystem mount.

## Installation and test instructions

### Safe host checks

```bash
go test ./...
go vet ./...
make hackathon-preflight
```

### One-command local Control Plane

```bash
go run ./cmd/rewind dashboard start --workspace "$PWD"
```

This starts a loopback-only supervisor and UI, creates a safe default policy, opens the browser, and launches a protected shell. The UI receives a short-lived connection token through the local URL fragment; the token is removed from the address bar after connection. Use only a disposable workspace for destructive commands.

### Local macOS product demo

```bash
ROOT="$(mktemp -d /Users/Shared/rewind-demo.XXXXXX)"
mkdir -p "$ROOT/workspace/src"
printf 'original-source\n' > "$ROOT/workspace/src/marker.txt"
printf 'synthetic-secret=do-not-read\n' > "$ROOT/workspace/.env"
go run ./cmd/rewind dashboard start --workspace "$ROOT/workspace"
```

Run the destructive synthetic command in the protected shell, inspect the UI
timeline and diff, then roll back. Keep the screenshot and recording free of
real paths and secret values.

### Privileged Linux demo

Inside the disposable Ubuntu VM:

```bash
cd /home/vagrant/RewindBPF
REWIND_VM_CONFIRM=VM_ONLY make final-vm
```

For the short deterministic act, use `scripts/jury_demo_vm.sh`. Expected marker:

```text
JURY_DEMO_VM_PASS
```

## Three-minute video script

1. **0:00–0:20 — problem:** an autonomous agent can delete a project or read a secret before a human notices.
2. **0:20–0:45 — boundary:** show the lower/merged/upper layers, the sensitive-read policy, and the eBPF/evidence timeline.
3. **0:45–1:35 — live act:** run the synthetic bad command; `src/` disappears only in the merged view, `.env` is denied, and a generated file appears in the upper layer.
4. **1:35–2:05 — decision:** open the Control Plane diff and event timeline, then execute rollback. Show the original marker restored and the generated file gone.
5. **2:05–2:35 — trust/performance:** show conflict-checked commit, incomplete-evidence refusal, and B0/B2/B4/B5 benchmark/storage/telemetry cards. Say “measured overhead,” never “zero overhead.”
6. **2:35–3:00 — Codex/GPT-5.6:** explain that Codex and GPT-5.6 were used to build, test, and refine the runtime and UI, while the runtime remains agent- and model-agnostic.

Use English narration or provide an English translation. Do not show real `.env` contents, credentials, private paths, or a personal workspace. Upload the video publicly to YouTube and keep it under three minutes.

## Final external checklist

- [ ] Join the hackathon and select **Developer Tools**.
- [ ] Paste the project description above and edit it into the entrant's own voice.
- [ ] Add the public repository URL, or share a private repository with `testing@devpost.com` and `build-week-event@openai.com`.
- [ ] Verify the README, setup commands, supported-platform table, sample policy, and judge test path.
- [ ] Run `/status` in the primary Codex build thread and paste the `/feedback` Session ID.
- [ ] Record and upload the English, narrated, sub-three-minute YouTube demo.
- [ ] Invite teammates and verify they accepted before submission, if applicable.
- [ ] Save a Devpost draft early; submit before the deadline rather than waiting for the final minutes.

The Devpost Hackathons Plugin may help prepare the form, but it is optional and is not part of RewindBPF. The official rules and Devpost site remain the source of truth.
