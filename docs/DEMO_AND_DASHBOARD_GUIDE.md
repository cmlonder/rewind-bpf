# RewindBPF demo and dashboard guide

This is the single-page runbook for the local macOS demonstration. It combines
the actions we have tested, the words to use during the recording, and a short
map of every Control Plane menu.

The demo uses a disposable workspace under `/Users/Shared`. It does not use a
real repository, real credentials, or the host project. The macOS path proves
the local supervisor, APFS clone-backed staging, Seatbelt read denial, live
diffs, rollback, and explicit commit. The Linux VM remains the reference path
for eBPF, OverlayFS, Landlock, cgroup, and privileged enforcement.

## The idea in one sentence

Rewind gives an agent a temporary work view. The original workspace stays in a
lower layer until a person explicitly accepts the reviewed result.

## What we have tested

- The protected shell can delete `src/` in its staged view.
- The original `src/marker.txt` stays intact in a separate terminal.
- A policy can deny reads matching `**/*.env` without hard-coding `.env` into the product.
- The dashboard shows the live policy event and the staged filesystem diff.
- Rollback discards the temporary layer and leaves the lower workspace unchanged.
- Commit applies only a reviewed candidate and removes the disposable runtime.
- The commit path is conflict-checked; destination drift is refused.
- The run detail keeps the timeline, diff, event count, dropped-event count, and hash-chain status.

## The clean recording flow

Use a fresh disposable fixture for the recording. Keep a second normal terminal
open. The protected shell and the normal terminal intentionally show different
views of the same transaction.

### 1. Create the fixture

```bash
ROOT="$(mktemp -d /Users/Shared/rewind-video.XXXXXX)"
mkdir -p "$ROOT/workspace/src"
printf 'original-source\n' > "$ROOT/workspace/src/marker.txt"
printf 'synthetic-secret=do-not-read\n' > "$ROOT/workspace/.env"
printf 'ROOT=%s\n' "$ROOT"
```

In the second terminal:

```bash
export ROOT=/Users/Shared/rewind-video.REPLACE_ME
cat "$ROOT/workspace/src/marker.txt"
```

Say:

> “This is a disposable workspace for the demonstration. The source marker is
> the original file, and `.env` is synthetic sensitive data.”

> “I am keeping a second terminal outside the protected shell so we can compare
> the real workspace with the agent’s staged view.”

Replace `rewind-video.REPLACE_ME` with the actual directory printed by the
first terminal.

### 2. Start the local Control Plane

From the repository root:

```bash
cd /Users/cemalonder/Dev/RewindBPF
export BIN=/tmp/rewind-darwin
GOTOOLCHAIN=local go build -o "$BIN" ./cmd/rewind

"$BIN" dashboard start \
  --workspace "$ROOT/workspace" \
  --state-dir "$ROOT/state" \
  --ui-dir "$PWD/ui"
```

Do not pass `--no-open`; the command opens the dashboard automatically. It
also creates the default local policy and starts the protected shell. The
operator does not need to create an OverlayFS mount or write a policy file for
this demo.

Say:

> “This single command starts the local supervisor, the dashboard, and a
> protected shell around the disposable workspace.”

> “The lower layer is the original workspace. The process will work in a
> temporary staged view.”

On the initial Overview screen, point out:

- `Runtime healthy` and `Live local supervisor`.
- `LOWER LAYER — INTACT`.
- `UPPER LAYER — 0 bytes` before the agent acts.
- `READ ENFORCE`, `WRITE ROLLBACK`, and the selected network mode.
- The `.env` `READ POLICY` event as a deny decision, not a filesystem delete.

The `bash: no job control in this shell` message is expected for this isolated
shell. It is not a product failure and does not need to be explained in the
video.

### 3. Show the destructive action

In the protected shell:

```bash
rm -rf src
ls -la
```

The protected shell no longer sees `src`. The dashboard should show staged
deletions and a non-zero upper layer. The second normal terminal should still
show:

```bash
cat "$ROOT/workspace/src/marker.txt"
```

with the original content.

Say:

> “The agent removed `src`, but only inside the staged view.”

> “The real workspace still contains the original source file. The agent has
> not crossed the filesystem boundary.”

Do not exit the protected shell until the dashboard has visibly updated.

### 4. Review and roll back

Exit the protected shell:

```bash
exit
```

The run becomes `succeeded` and the dashboard changes to `Review transaction`.
Open the run detail page and show:

- `FILESYSTEM DIFF` with `src` and `src/marker.txt`.
- `LOWER LAYER — INTACT`.
- The timeline entries for prepare, read-policy denial, exec, and exit.
- `EVIDENCE HEALTH`, including `0 dropped` and a valid hash chain.

Choose **Rollback run**, type the one-time token shown in the confirmation
dialog, and confirm.

Say:

> “Rewind has isolated the damage as candidate changes.”

> “I am discarding the temporary upper layer. The original workspace remains
> untouched.”

In the second terminal, verify:

```bash
test -f "$ROOT/workspace/src/marker.txt" && echo "original source restored"
cat "$ROOT/workspace/src/marker.txt"
```

The diff remains visible as historical evidence after rollback. That does not
mean the temporary runtime still exists.

### 5. Show an explicit commit

Rollback is terminal for that run. Start a new dashboard transaction with a
new state directory, using the restored workspace:

```bash
"$BIN" dashboard start \
  --workspace "$ROOT/workspace" \
  --state-dir "$ROOT/commit-state" \
  --ui-dir "/Users/cemalonder/Dev/RewindBPF/ui"
```

In the new protected shell, make a harmless candidate change:

```bash
printf 'accepted-by-operator\n' > generated.txt
exit
```

In the dashboard run detail page, inspect the created file, choose **Accept
changes**, type the one-time token, and confirm. Verify from the normal
terminal:

```bash
cat "$ROOT/workspace/generated.txt"
```

Say:

> “Commit is explicit. Only after review does a candidate change reach the
> destination workspace.”

> “The commit is conflict-checked before it applies anything.”

For the video, do not commit the `rm -rf src` candidate. Use a harmless created
file so the acceptance path is clear.

### 6. Show sensitive-read denial

In another fresh transaction, run:

```bash
cat .env > leaked.txt
```

The command should fail, the dashboard should show `READ POLICY` with
`deny`, and `leaked.txt` should not exist in the workspace.

Say:

> “Sensitive reads are denied before the file content can enter the agent’s
> staged workspace.”

Do not show the contents of `.env`, even though the fixture is synthetic.

## Three-minute narration order

1. **0:00–0:20 — Problem:** “An agent can remove a project directory or read a secret before a human notices.”
2. **0:20–0:45 — Boundary:** Show the lower layer, staged view, policy state, and empty upper layer.
3. **0:45–1:20 — Destructive action:** Run `rm -rf src`; show the staged deletion and the untouched file in the second terminal.
4. **1:20–1:55 — Rollback:** Review the diff, show evidence health, and roll back.
5. **1:55–2:20 — Commit:** Create `generated.txt`, review it, and accept it explicitly.
6. **2:20–2:45 — Sensitive read:** Show `.env` denial and no leaked file.
7. **2:45–3:00 — Close:** Show the run detail timeline and state the platform boundary honestly.

Closing line:

> “Rewind does not try to predict every bad command. It gives the agent a
> disposable filesystem view and gives the operator the final decision.”

## Control Plane menu map

### Overview

The operational summary: current transaction, lower/upper state, evidence
health, recent high-risk events, active policy, and runtime health.

Say:

> “Overview tells me whether the supervisor is connected and whether a run
> needs my decision.”

### Runs

The searchable list of transactions. Use it to find a running, succeeded,
rolled-back, or committed run. Select a run to open its detail page.

Say:

> “Runs is the durable index of decisions, separate from disposable workspace
> layers.”

### Run detail

The main review screen: action rail, filesystem diff, lifecycle timeline,
process scope, evidence counters, and rollback/commit controls.

Say:

> “This is where an operator reviews what changed and chooses rollback or an
> explicit commit.”

### Policies

Policy packages define read mode and patterns, PII mode, write behavior and
scope, network mode, and allowed domains. A policy can be selected, created, or
edited for future runs.

Say:

> “Policies are user-defined safety contracts. `.env` is only one example; a
> team can define any path pattern that matters to its workspace.”

### Workspaces

Workspace records map a path to a policy package and agent adapter. They are
the place to see which project roots are protected and which policy will apply
to future runs.

Say:

> “A workspace is the scope of protection. The product does not silently cover
> the entire host filesystem.”

### System boundaries

An explanatory boundary map: filesystem lower/merged/upper layers, process
scope, read policy, network/credential separation, and fixture versus connected
supervisor mode.

Say:

> “This page explains what Rewind protects, what it observes, and where its
> authority stops.”

### Global Config

Future-run defaults and control-plane settings: backend choice, read/write and
network defaults, evidence limits, retention, encryption, session behavior,
and PII defaults.

Say:

> “Global Config sets defaults for future runs; it does not rewrite the policy
> snapshot of an existing run.”

### History

The retention-oriented index of previous runs. It keeps bounded metadata and
links back to the run record without keeping the disposable upper layer alive.

Say:

> “History keeps the decision trail after a workspace layer has been discarded.”

### Evidence

Evidence and audit views: event journals, hash-chain verification, dropped or
truncated event status, manifests, export, and release evidence.

Say:

> “The evidence record lets us verify what was observed, not just what the UI
> currently renders.”

### Recovery / Safety Lab

The extended recovery surface: checkpoint graph, dependency-aware rollback,
PII findings, remote restore status, agent adapters, and platform hardening
gates. These are product capabilities beyond the short macOS demo.

Say:

> “The Safety Lab is where recovery, PII findings, adapters, and future remote
> restore controls are inspected together.”

### Trust & Actions

Human confirmation and trust controls: one-time action tokens, supervisor
connection, trusted registry metadata, signer keys, and policy distribution.

Say:

> “A browser session is not itself permission to mutate a run. Destructive
> actions use a short-lived, one-time confirmation token.”

### Benchmarks

The measured performance view: native baseline, staged filesystem modes,
protected lifecycle cost, storage amplification, event bytes, and latency.
Use measured numbers from `benchmarks/RESULTS.md`; do not claim zero overhead.

Say:

> “The benchmark view reports the cost we measured. Copy-on-write avoids a full
> pre-run copy, but it is not magic and we do not describe it as zero overhead.”

### Audit Log

Supervisor mutations and refused actions: policy changes, workspace changes,
session actions, registry checks, rollback, commit, and conflict refusal.

Say:

> “Audit Log records control-plane intent separately from the agent’s
> filesystem events.”

## Claims to keep precise

- Say **“copy-on-write avoids a full pre-run copy”**, not “zero overhead.”
- Say **“macOS native staged transaction”**, not “macOS eBPF enforcement.”
- Say **“Linux reference path”** for eBPF, OverlayFS, Landlock, cgroup, and
  privileged VM evidence.
- Say **“filesystem changes inside the protected workspace”**. Rewind does not
  undo databases, cloud APIs, network side effects, devices, or arbitrary kernel
  state.
- Say **“agent-agnostic command boundary”**. Rewind is a CLI, supervisor, and
  UI; it is not an MCP server or a required model plugin.

## Recording checklist

- [ ] Use a fresh `/Users/Shared` fixture.
- [ ] Keep the second normal terminal visible for lower-layer proof.
- [ ] Never show real credentials or a real repository path.
- [ ] Capture the initial Overview, live diff, Run detail, rollback result, and
      commit result.
- [ ] Capture one screenshot of `.env` denial without showing its contents.
- [ ] Keep the video under three minutes and narrate in English.
- [ ] Use the public site and README for architecture and benchmark context;
      use this guide as the operational demo script.
