# Hackathon Test, Demo, and Recording Plan

**Status:** ready for rehearsal
**Scope:** safe Mac validation, disposable Ubuntu VM acceptance, and jury evidence
**Safety rule:** never point a destructive or privileged command at the personal Mac, a real repository, a real `.env`, or a real credential.

## 1. Readiness snapshot

| Surface | Status | Evidence |
|---|---|---|
| Linux P0/P1 runtime | Complete | UTM `final-vm` gate, acceptance matrix, P1 leak smoke |
| macOS staged lifecycle | Complete for safe fixture | `mac-native-smoke`, `mac-crash-smoke`, manual APFS/Seatbelt runbook |
| Windows | Code-complete contract; helper gate remains | Cross-build and PowerShell preflight only |
| UI | Ready for fixture demo and authenticated supervisor walkthrough | `make ui-smoke`, responsive Control Plane fixture |
| Public site | Ready to publish | `make site-smoke`, GitHub Pages workflow |
| Release/evidence | Complete | Cross-platform binaries, eBPF object, policy, metadata, SHA-256 bundle |

## 2. Your pre-hackathon manual test plan (Mac)

Perform these steps on the development Mac only with disposable fixtures under `/Users/Shared`. Do not use the project repository as the workspace.

### 2.1 Safe automated checks

From the repository root:

```bash
go test ./...
go vet ./...
make ui-smoke
make site-smoke
make benchmark-verify
make mac-safe-smoke
make mac-native-smoke
make mac-crash-smoke
make hackathon-preflight
```

Expected markers:

```text
UI_SMOKE_PASS
SITE_SMOKE_PASS
BENCHMARK_LEDGER=PASS
MAC_SAFE_SMOKE_PASS
MAC_NATIVE_SMOKE_PASS
MACOS_CRASH_SMOKE_PASS
HACKATHON_PREFLIGHT=PASS
```

These scripts use temporary fixtures and do not modify a real workspace.

### 2.2 Manual macOS lifecycle rehearsal

Use the full command sequence in [`docs/platform/macos_manual_e2e.md`](platform/macos_manual_e2e.md). The short acceptance sequence is:

1. Build `/tmp/rewind-darwin` for the current Mac architecture.
2. Run `platform plan --workspace /Users/Shared` and confirm the output says `filesystem=apfs` and `ready=true`.
3. Create a fresh `/Users/Shared/rewind-manual.<suffix>` fixture containing `src/marker.txt` and a synthetic `.env`.
4. Run a review transaction that deletes `src/` and creates `generated.txt`.
5. Inspect `native diff` and `native events`; confirm the original workspace is unchanged and `.env` is denied.
6. Run `native rollback`; verify the marker is restored, generated output is absent, and the runtime directory is gone.
7. Run the commit transaction; inspect the diff, commit with `--confirm`, and verify the accepted file is present.
8. Run the destination-drift transaction; change the destination manually, confirm commit is refused, then rollback.
9. Run the PII denial transaction (`cat .env > leaked.txt`); confirm non-zero exit, no leaked file, and a redacted `read_policy` denial event.
10. Save only the command output and event metadata; never record actual secret contents.

### 2.3 Mac test evidence checklist

Save:

- `platform plan` output
- review `native diff` output
- `native events` output
- rollback verification output
- commit verification output
- conflict refusal output
- PII denial output
- `wc -c <record>.events.jsonl` and the event sidecar

Do not save or publish:

- real home-directory paths
- real secret values
- real repository contents
- personal usernames, tokens, or API keys

### 2.4 Safe UI/site validation

Open `site/index.html` locally and verify:

- the shipped P0/P1 status is visible;
- the competitor matrix distinguishes rollback, kernel enforcement, and filesystem COW;
- benchmark charts and storage/evidence caveats render;
- the UI System Boundaries screen is separate from Global Config;
- policy selection updates effective policy state;
- retention, session, remote restore, PII rescan, registry, and adapter actions show a visible result;
- every contextual `i` explanation opens in a centered, keyboard-safe modal;
- no fixture action claims to mutate the host when disconnected.

## 3. Disposable Ubuntu VM acceptance plan

This is the privileged Linux path and must run inside the UTM Ubuntu VM, never on the Mac host:

```bash
cd /home/vagrant/RewindBPF
REWIND_VM_CONFIRM=VM_ONLY make final-vm
```

The gate performs:

- bootstrap and capability checks;
- cross-platform release builds and eBPF CO-RE compilation;
- B0/B2/B4/B5 benchmark normalization and chart generation;
- rollback/read-denial, review/commit, conflict, proxy, strict-deny, namespace, and incomplete-evidence acceptance;
- three-iteration cgroup/network leak smoke;
- deterministic jury demo;
- release metadata and SHA-256 verification.

The output directory contains `logs/acceptance.log`, `logs/p1-leak.log`, `logs/jury-demo.log`, normalized benchmark files, release metadata, and `SHA256SUMS`.

## 4. Jury presentation plan

### 4.1 Three-minute story

1. **Problem (20 seconds):** an AI agent can delete or overwrite a project and can read secrets unless the runtime creates a boundary before execution.
2. **Invariant (30 seconds):** the agent writes to an OverlayFS/FUSE upper layer; Landlock denies sensitive reads; cgroup-v2 scopes descendants; eBPF records evidence.
3. **Live act (90 seconds):** run the intentionally bad agent command, show `src/` disappearing only from the merged view, show the `.env` denial, then rollback and show the original source restored.
4. **Trust act (25 seconds):** show the event chain, dropped/truncated evidence state, and destination-drift commit refusal.
5. **Performance and differentiation (35 seconds):** show B0/B2/B4/B5, storage amplification, event bytes, and explain the measured hot-path tradeoff rather than presenting it as zero overhead.
6. **Close (20 seconds):** RewindBPF is a transaction runtime for agents, not an SDK plugin and not a replacement for a VM/container; it accepts only reviewed, verifiable results.

### 4.2 What to show on screen

- Left: UTM Ubuntu VM terminal running the safe demo.
- Right: the static Control Plane UI showing run timeline, policy decision, diff, evidence health, and rollback action.
- Keep the personal Mac desktop and real filesystem out of the recording.
- Use synthetic names: `src/marker.txt`, `synthetic.env`, `created-by-agent`.

### 4.3 Exact demo sequence

Use `scripts/jury_demo_vm.sh` for the deterministic terminal act. The expected visible output is:

```text
deleted src is isolated in upper layer
created-by-agent
sensitive read denied
JURY_DEMO_VM_PASS
```

Then show the corresponding record with:

```bash
rewind status --record <record.json>
rewind diff --record <record.json>
rewind events --record <record.json>
rewind rollback --record <record.json>
```

Never improvise a real `rm -rf` target during the presentation.

## 5. Video recording plan

### 5.1 Capture three clips

**Clip A — 90-second hero demo**

- title card: “RewindBPF — Ctrl+Z for AI agents”;
- bad command starts;
- `src/` is deleted in the merged view;
- `.env` read is denied;
- rollback restores the marker;
- final `JURY_DEMO_VM_PASS`.

**Clip B — 45-second technical proof**

- lower/merged/upper diagram in the site;
- eBPF event timeline and hash-chain evidence;
- cgroup/network boundary card;
- conflict refusal and explicit commit.

**Clip C — 30-second benchmark/release proof**

- B0/B2/B4/B5 chart;
- storage amplification and event-byte metrics;
- release bundle directory and checksum verification;
- `FINAL_VM_GATE=PASS`.

### 5.2 Recording checklist

- Start from a clean UTM VM snapshot.
- Disable desktop notifications and unrelated terminals.
- Use a large terminal font and a fixed 16:9 window.
- Keep the UI browser at `site/index.html` or the local Control Plane fixture.
- Narrate the safety invariant before the destructive command.
- Do not show passwords, real paths, real `.env` values, SSH keys, or tokens.
- Keep raw terminal output as a separate text artifact in the evidence bundle.
- Record one clean take and one backup take; do not edit away a failed safety result.

## 6. Submission bundle

Include:

- `README.md`
- `docs/ARCHITECTURE.md`
- `docs/FEATURE_BACKLOG.md`
- `docs/PHASE2_PLAN.md`
- `docs/platform/macos_manual_e2e.md`
- `benchmarks/results_summary.csv`
- `benchmarks/results_normalized.csv`
- `benchmarks/results_chart.svg`
- final VM evidence archive and its `SHA256SUMS`
- release bundle with platform binaries, eBPF object, policy example, and metadata
- hero demo video plus technical/benchmark clips
- static `site/` directory or deployed GitHub Pages URL

## 7. Claims we should make—and avoid

Make:

- “The write transaction is prepared before the agent starts.”
- “Sensitive reads are policy-controlled and denied before the agent sees content.”
- “Rollback and commit are explicit, conflict-checked operations.”
- “Event loss and incomplete evidence fail closed.”
- “The Linux reference path is reproducible in a disposable VM.”

Avoid:

- “Zero overhead.”
- “Undo for network, database, cloud, kernel, or device side effects.”
- “Windows/macOS provide Linux-equivalent kernel enforcement.”
- “eBPF alone blocks every dangerous operation.”
- “A successful process exit means the result is safe without review.”
