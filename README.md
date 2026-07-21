# RewindBPF

**RewindBPF is a reversible safety runtime for AI agents.** It lets an agent
work inside a disposable filesystem transaction, blocks configured sensitive
reads, records what happened, and gives an operator an explicit **rollback** or
conflict-checked **commit**.

The reference implementation runs in a disposable Ubuntu VM. A safe macOS
native transaction path and a fail-closed Windows contract are included; they
are not advertised as Linux-equivalent enforcement.

## The problem

An agent can delete a source tree, overwrite a configuration file, or read a
secret before a human can intervene. Command deny-lists are brittle, and a
full pre-run copy is expensive.

## The idea

Rewind prepares the write boundary **before** the agent starts:

```text
real workspace (lower, unchanged)
             ↓
agent sees a merged view  →  writes land in a disposable upper layer
             ↓
        review → rollback  |  explicit, conflict-checked commit
```

- **Copy-on-write filesystem:** OverlayFS/FUSE keeps the original lower layer
  untouched while the agent works.
- **Read policy:** Landlock enforces user-defined deny patterns such as
  `**/*.env`, `**/*.pem`, or a project-specific PII path.
- **Evidence:** eBPF tracepoints and lifecycle records produce an ordered,
  hash-chained event stream. Incomplete evidence fails closed at verification.
- **Process and network scope:** cgroup-v2 and explicit proxy/deny/namespace
  backends constrain the Linux reference run.
- **Operator control:** review the diff, then discard the upper layer or apply
  only a conflict-checked candidate.

Rewind is a **CLI + local supervisor + Control Plane UI**, not an MCP server or
an agent SDK plugin. The agent command remains the operator's command; Rewind
supplies the boundary around it.

## What is ready

| Surface | Status | Honest boundary |
|---|---|---|
| Linux reference runtime | Ready in disposable Ubuntu VM | Privileged OverlayFS/eBPF tests are VM-only |
| Rollback and recovery | Ready | Reverses the protected filesystem transaction, not external side effects |
| Sensitive-read policy | Ready on Linux | Pattern/PII enforcement is policy-scoped |
| Evidence and supervisor | Ready | Authenticated local control plane; no distributed control plane claim |
| macOS native path | Safe staged lifecycle | EndpointSecurity, network, and resource helper gates remain |
| Windows native path | Fail-closed contract and cross-build | Signed minifilter/VHDX acceptance remains |
| Control Plane UI and public site | Ready | Connected UI requires a local supervisor; fixture mode is non-mutating |

See the canonical status ledger in
[`docs/FEATURE_BACKLOG.md`](docs/FEATURE_BACKLOG.md) and the platform matrix in
[`docs/PLATFORM_STATUS.md`](docs/PLATFORM_STATUS.md).

## Quick start

### Safe macOS local experience

Use a disposable workspace. This starts the local supervisor, opens the UI, and
launches a protected interactive shell:

```bash
go run ./cmd/rewind dashboard start --workspace "$PWD"
```

When the shell exits, the dashboard stays open so you can inspect the diff and
choose **Rollback** or **Commit**. Independent terminals are not retroactively
monitored. The safe native smoke tests use temporary fixtures only:

```bash
make mac-safe-smoke
make mac-native-smoke
make mac-crash-smoke
```

### Linux jury path

Run this inside the disposable Ubuntu UTM VM, never on a personal host:

```bash
cd /home/vagrant/RewindBPF
REWIND_VM_CONFIRM=VM_ONLY make final-vm
```

For the short deterministic presentation:

```bash
REWIND_DEMO_CONFIRM=VM_ONLY make jury-demo-vm
```

Expected marker:

```text
JURY_DEMO_VM_PASS
```

The complete rehearsal and recording script are in
[`docs/HACKATHON_TEST_AND_DEMO_PLAN.md`](docs/HACKATHON_TEST_AND_DEMO_PLAN.md).

### Safe repository checks

```bash
go test ./...
go vet ./...
make ui-smoke
make site-smoke
make benchmark-verify
make public-audit
```

`make hackathon-preflight` runs the complete non-privileged host checklist and
creates a local evidence bundle. It does not mount filesystems, load eBPF,
change firewall state, or touch a real workspace.

## Example policy

```yaml
read:
  mode: enforce
  pii:
    mode: audit
  deny:
    - "**/*.env"
    - "**/*.pem"
    - "**/*.key"
    - "/home/*/.ssh/**"
  allow:
    - "/workspace/.env.example"

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

The complete non-secret example is
[`policies/example.yaml`](policies/example.yaml). Patterns are user-defined;
`.env` is only one example.

## CLI lifecycle

Inside the Linux VM, the core flow is:

```bash
rewind run \
  --workspace ./project \
  --runtime-root ./runtime \
  --policy ./policy.yaml \
  --record ./runtime/record.json \
  --overlay-backend fuse \
  --on-success review \
  -- agent-command

rewind status   --record ./runtime/record.json
rewind diff     --record ./runtime/record.json
rewind events   --record ./runtime/record.json
rewind rollback --record ./runtime/record.json
# or, after review and a clean manifest comparison:
rewind commit   --record ./runtime/record.json --confirm
```

Successful runs discard the temporary upper layer by default. `review` keeps it
available until the operator chooses. A changed destination, unsupported path,
or incomplete evidence refuses commit rather than partially applying changes.

## UI and public site

The local Control Plane lives in [`ui/`](ui/). It provides run history,
timeline/events, filesystem diff, policy/workspace configuration, system
boundaries, evidence state, rollback/recover/commit actions, and trusted-policy
metadata. Fixture mode is deliberately non-mutating; connected mode talks only
to the authenticated local supervisor.

```bash
python3 -m http.server 4174 --directory ui
open http://127.0.0.1:4174
```

The jury-facing site lives in [`site/`](site/) and is dependency-free:

```bash
python3 -m http.server 4173 --directory site
open http://127.0.0.1:4173
```

GitHub Pages is configured through
[`.github/workflows/pages.yml`](.github/workflows/pages.yml); choose **GitHub
Actions** as the Pages source. The workflow publishes `site/`, not the repo
root.

## Benchmarks and evidence

The benchmark ledger compares native and protected controls (B0/B2/B4/B5),
including throughput, latency, storage amplification, lifecycle time, and
telemetry bytes. It is intentionally not presented as “zero overhead.”

```bash
python3 benchmarks/normalize_results.py
python3 benchmarks/plot_results.py
make benchmark-verify
```

Start with [`benchmarks/RESULTS.md`](benchmarks/RESULTS.md), then read the
protocol in [`benchmarks/PHASE2_PROTOCOL.md`](benchmarks/PHASE2_PROTOCOL.md).
Raw VM runtime data and generated release/evidence directories stay ignored.

## Competitive position

RewindBPF is not claiming to be the first kernel-security project or the broadest
developer sandbox. `nono` is stronger as a general-purpose sandbox; Tetragon
and KubeArmor are stronger adjacent enforcement/observability systems; AgentFS
and DeltaBox overlap with filesystem/history or checkpoint research.

Our narrower distinction is the pre-created write transaction plus explicit
acceptance: the agent can use normal filesystem operations, the lower layer
stays untouched, sensitive reads are policy-controlled, and rollback is a
discard operation rather than a best-effort reconstruction.

The full comparison is in
[`benchmarks/COMPETITOR_MATRIX.md`](benchmarks/COMPETITOR_MATRIX.md) and the
product strategy is in [`docs/PRODUCT_STRATEGY.md`](docs/PRODUCT_STRATEGY.md).

## Safety boundary

Do not run privileged or destructive tests on a personal Mac or real project.
Use the disposable Ubuntu VM for OverlayFS, eBPF, cgroup, Landlock, and network
namespace acceptance. macOS smoke tests use temporary `/Users/Shared` fixtures.
Never bind-mount a real home directory, `.env`, SSH key, credential, or customer
data.

Rewind does **not** undo database writes, cloud API calls, network side effects,
device changes, or arbitrary kernel effects. A successful process exit is not an
automatic safety approval; review remains explicit.

## Development

Requirements:

- Go 1.22+
- Linux VM for privileged integration
- `fuse-overlayfs`, Landlock or BPF LSM, and BPF/BTF support in that VM

Common commands:

```bash
make build
make test
make hackathon-preflight
make final-vm                 # disposable Ubuntu VM only
```

The repository layout and implementation invariants are documented in
[`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md). The roadmap and verification
status are in [`docs/PROJECT_PLAN.md`](docs/PROJECT_PLAN.md) and
[`docs/PHASE2_PLAN.md`](docs/PHASE2_PLAN.md).

## Codex and GPT-5.6

RewindBPF was built and iterated in Codex with GPT-5.6. Codex helped decompose
the runtime, implement the policy/rollback lifecycle, harden crash/evidence
paths, build the UI/site, and create the benchmark and VM gates. GPT-5.6 was a
build-time implementation and review partner; the shipped runtime remains
agent- and model-agnostic.

Primary Devpost `/feedback` Session ID:
`019f6f87-53d3-7c11-be4d-6d07217d62ea`

See [`docs/DEVPOST_SUBMISSION.md`](docs/DEVPOST_SUBMISSION.md) for the
submission copy, supported-platform test path, and video script.

## Public repository hygiene

Before pushing changes:

```bash
make public-audit
git diff --check
```

The audit and publication boundary are documented in
[`docs/PUBLIC_REPO_CHECKLIST.md`](docs/PUBLIC_REPO_CHECKLIST.md). Vulnerability
reporting guidance is in [`SECURITY.md`](SECURITY.md).
