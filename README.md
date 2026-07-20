# RewindBPF

RewindBPF is an **AI Agent Safety Runtime** for running autonomous agents inside reversible, policy-controlled filesystem transactions.

Its product focus is deliberately narrow: **let an agent work aggressively without giving it direct access to the real project or real credentials**. The current production proof is Linux-first; native macOS and Windows backends are planned behind the same transaction contract.

The product strategy is documented in [docs/PRODUCT_STRATEGY.md](docs/PRODUCT_STRATEGY.md).

## Current status

The MVP is complete for its explicitly documented disposable-VM boundary. The Linux product-core slice now includes cgroup-v2 scopes, capability reporting, prepared-run journaling, recovery, evidence digests and hash chains, diff/export, signed policy envelopes, a loopback proxy network backend, narrow raw/packet-socket denial in enforce mode, network/credential refusal contracts, conflict-checked `commit --confirm`, durable history, an authenticated supervisor transport with lifecycle actions and follow-mode events, release/bootstrap scripts, and the fixture Control Plane UI. Warm and cold B0/B2/B4 measurements, storage footprint, telemetry growth, and benchmark charts are recorded. Remaining productisation work is a namespace/non-proxy-aware network backend, a real credential provider, detachable sessions, and native macOS/Windows implementations; unsupported capabilities remain fail-closed.

Track the implementation and architecture in [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md). The architecture document is updated after every completed stage.

The six-day hardening sprint and the post-hackathon product roadmap are in [docs/PHASE2_PLAN.md](docs/PHASE2_PLAN.md). It includes the competitive analysis, P0/P1 work packages, exit criteria, correctness matrix, and research references.

Release builds are cross-compiled with `make release`; `make release-manifest` adds `bin/SHA256SUMS` and `bin/release-metadata.txt`. For a detached Ed25519 signature, generate a key outside the repository and run `REWIND_RELEASE_PRIVATE_KEY=/secure/path/release.key make release-sign`; this writes `bin/SHA256SUMS.sig` and records the signing status without copying the private key. Verify with `rewind release verify --input bin/SHA256SUMS --signature bin/SHA256SUMS.sig --public-key /secure/path/release.pub`. An embedded public key proves integrity, while a pinned key proves publisher identity; public registry trust, rotation, and revocation remain deployment responsibilities.

## Competitive landscape

RewindBPF is not the first project to use a kernel primitive, a sandbox, or a filesystem snapshot for agent safety. The credible distinction is the combination and the boundary: a Linux-first, agent-agnostic transaction that is prepared before the agent starts, keeps the original lower layer untouched, emits kernel telemetry, supports user-defined sensitive-read policies, and can discard the whole write layer in one rollback operation.

The comparison below was reviewed in July 2026 against the projects’ public documentation. “No” means that the capability is not the primary documented behavior; it does not mean the project cannot be composed with another tool.

| Solution | Main safety model | Pre-run filesystem COW | Kernel-level policy | Session rollback | User-defined sensitive-read policy | Agent-agnostic |
|---|---|---:|---:|---:|---:|---:|
| **RewindBPF (target MVP)** | OverlayFS transaction + eBPF telemetry/policy | **Yes** | **Yes** (eBPF/BPF-LSM where supported) | **Yes** (discard upper layer) | **Yes** (`off`/`audit`/`enforce` glob patterns) | **Yes** |
| [OpenAI Codex CLI](https://help.openai.com/en/articles/11096431) | Approval modes and a scoped, network-disabled sandbox | Partial | Not the product’s eBPF/LSM focus | No | Partial, through sandbox scope | No |
| [Claude Code sandboxing](https://www.anthropic.com/engineering/claude-code-sandboxing) | OS sandbox boundaries plus permission prompts | No | OS primitives (Linux `bubblewrap`, macOS Seatbelt), not an eBPF rollback layer | No | Yes, through filesystem/network allowlists | No |
| [Cursor Agent sandbox](https://cursor.com/blog/agent-sandboxing) | OS sandbox and approval/auto-review modes | No | Platform sandbox (for example macOS Seatbelt) | No | Partial, through sandbox settings | No |
| [OpenHands Docker Sandbox](https://docs.openhands.dev/openhands/usage/sandboxes/docker) | Containerized execution runtime | No | Container/kernel isolation, not an eBPF transaction | No | Partial, through mounts and runtime configuration | Yes |
| [Turso AgentFS](https://github.com/tursodatabase/agentfs) | SQLite-backed agent filesystem, history, and snapshots | Filesystem-layer COW/overlay options | No eBPF/BPF-LSM enforcement focus | **Yes** (database snapshot/restore) | Partial, through filesystem scope | Yes |
| [nono](https://nono.sh/os-sandbox) | Landlock (Linux) / Seatbelt (macOS) allowlists | No | **Yes**, kernel-enforced access control | **Yes**, content-addressed session snapshots | **Yes**, path/profile rules | Yes |

### Kernel-level adjacent systems

There are established kernel-security projects that overlap with parts of the design, but they solve a different primary problem:

| Project | What it proves | What it does not provide as the core product |
|---|---|---|
| [Cilium Tetragon](https://tetragon.io/docs/overview/) | eBPF-based runtime observability and in-kernel enforcement, including file and network policies | A pre-created OverlayFS transaction and filesystem rollback |
| [Falco](https://falco.org/docs/reference/rules/supported-events/) | eBPF/syscall telemetry, rules, alerts, and dropped-event visibility | Atomic agent-session rollback or a protected write layer |
| [KubeArmor](https://docs.kubearmor.io/kubearmor) | AppArmor/SELinux/BPF-LSM policy enforcement and telemetry | OverlayFS session rewind and agent-run lifecycle semantics |
| [DeltaBox](https://arxiv.org/abs/2605.22781) | Research prototype for OS-level agent checkpoint/rollback using change-based filesystem and process state | A production-ready, general-purpose CLI/runtime for local agent safety |

### Our defensible position

- We do **not** claim to be the first kernel-level agent safety project. `nono`, Tetragon, KubeArmor, and research systems such as DeltaBox make that claim untenable.
- Our MVP focuses on a narrower user outcome: **immutable project writes, invisible secrets, explicit acceptance, and fail-closed trust**. OverlayFS is the write transaction, eBPF is the low-overhead sensor/enforcement path, and a userspace run controller is the rollback authority.
- Unlike command deny-lists, the rollback boundary does not depend on recognizing every spelling of `rm`, `mv`, a shell script, or a library call. The agent can perform normal writes inside the merged view; the lower layer remains unchanged until an explicit commit/export.
- Unlike a post-hoc backup, the expensive copy is avoided on the hot path. Copy-on-write occurs only for blocks/files that the agent actually changes. This is a benchmark hypothesis, not an unmeasured guarantee.
- Unlike a generic container sandbox, the project makes the file transaction and recovery invariant explicit and testable. The MVP still uses a disposable VM for privileged Linux integration and does not claim to reverse kernel, device, network, or external-service side effects.

### Product boundary versus nono

nono is the stronger broad developer sandbox today. RewindBPF is intentionally not trying to copy its entire surface. We focus on the moment an operator needs a high-assurance answer to four questions:

1. Did the agent write to the real project before I accepted the result? **No.**
2. Could it read my configured sensitive paths? **The policy backend denies or hides them.**
3. Can I accept the result without overwriting destination drift? **Only through `rewind commit --confirm`, after the manifest conflict check passes.**
4. Can the runtime prove the run was complete? **Evidence loss makes the run incomplete.**

See [docs/PRODUCT_STRATEGY.md](docs/PRODUCT_STRATEGY.md) for the adopted native-platform and post-demo roadmap.

The benchmark plan deliberately compares these tradeoffs rather than relying on a “near-zero overhead” slogan: native ext4, eBPF-only, OverlayFS-only, OverlayFS + eBPF, and the full daemon path are measured separately. See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for the test matrix and safety boundary.

Captured VM measurements are summarized in [benchmarks/RESULTS.md](benchmarks/RESULTS.md) and [benchmarks/results_summary.csv](benchmarks/results_summary.csv). The CSV is the input for the final Python charts; raw VM artifacts remain outside Git unless explicitly archived.

## Project site

The jury-facing single-page site lives in [`site/`](site/). It is dependency-free and assembled from modular JavaScript sections so it can be published as a static site today and split into routes later. To preview it locally:

```bash
python3 -m http.server 4173 --directory site
open http://127.0.0.1:4173
```

The page covers the shipped safety surface, reversible transaction flow, Phase 2 roadmap, competitor capability matrix, and measured B0/B2/B4 evidence. The Markdown ledgers remain canonical.

On macOS, the safe prerequisite probe is read-only:

```bash
rewind platform plan --workspace /path/to/disposable-apfs-fixture
```

It reports APFS, `sandbox-exec`, and `diskutil` availability. It does not
clone, mount, launch, or delete anything. The macOS backend remains refused
until the disposable-volume manual gate is completed.

## Control Plane UI

The operational UI prototype lives in [`ui/`](ui/) and is tracked in [`docs/UI_ROADMAP.md`](docs/UI_ROADMAP.md). It is fixture-driven in Phase 1: no eBPF program, mount, process, workspace, or host file is touched. Preview it safely with:

```bash
python3 -m http.server 4174 --directory ui
open http://127.0.0.1:4174
```

The current fixture slice includes Overview, Runs, an animated Run Detail timeline, filesystem diff, evidence health, rollback/recover/export confirmation flows, searchable run filters with empty states, policy package creation and simulation, signed policy bundle import, workspace-to-policy assignments, revisioned global configuration controls, notifications, keyboard-safe dialogs, effective policy resolution, and benchmark/evidence surfaces. A local supervisor bridge exposes authenticated health, capability, history, snapshot/follow event streams with reconnect backoff, status, rollback/recover, explicit commit actions, validated policy/workspace writes, and signed bundle import; the browser adapter can invoke those actions only through the bearer-token bridge and never receives root privileges. Local authentication beyond the Unix-socket and bearer-token boundary is intentionally post-demo.

## Safety warning

Do not run kernel, mount, or destructive tests directly on a personal host. RewindBPF integration tests must run in a disposable Ubuntu VM or an explicitly created test image. Do not bind-mount a real home directory, real project, `.env`, SSH keys, or personal data into a destructive test.

The approved integration-test boundary is a disposable Ubuntu VM where RewindBPF runs directly. The recommended layout is:

```text
macOS host → disposable Ubuntu VM → RewindBPF directly
```

After building the binary and eBPF object inside that VM, run the repeatable
synthetic acceptance matrix with:

```bash
REWIND_VM_CONFIRM=VM_ONLY make acceptance-vm
```

This gate is VM-only and covers rollback/read denial, evidence bundle
create/verify, review/commit, clean-branch acceptance, destination-drift
refusal, proxy/raw-socket semantics, and incomplete-evidence refusal. Run
`make benchmark-verify` to validate the checked-in B0/B2/B4 ledger and chart.
The supervisor boundary can be checked separately with
`REWIND_VM_CONFIRM=VM_ONLY make supervisor-smoke-vm`.

## User workflow

The primary workflow runs inside the disposable Linux VM:

```bash
sudo rewind run --workspace ./project --runtime-root ./runtime \
  --policy ./policy.yaml --record ./runtime/record.json \
  --history ./runtime/history.json -- agent-command
sudo rewind status --record ./runtime/record.json
rewind inspect --record ./runtime/record.json
sudo rewind events --record ./runtime/record.json
rewind verify --record ./runtime/record.json
rewind evidence verify --record ./runtime/record.json
./bin/rewind-evidence --record ./runtime/record.json
sudo rewind diff --record ./runtime/record.json
rewind export --record ./runtime/record.json --output ./review-bundle.json
# Optional text-only review artifact for CI or a human patch review:
rewind export --record ./runtime/record.json --output ./review.patch --format patch
# Full-fidelity Git diff (requires git in the VM):
rewind export --record ./runtime/record.json --output ./review.git.patch --format git-patch
# Portable evidence archive (record + ordered event logs, no workspace files):
rewind bundle create --record ./runtime/record.json --output ./run-evidence.tar.gz
rewind bundle verify --input ./run-evidence.tar.gz
# Optional detached signature for remote review hand-off:
rewind bundle sign --input ./run-evidence.tar.gz --private-key /secure/review.key --output ./run-evidence.sig
rewind bundle verify --input ./run-evidence.tar.gz --signature ./run-evidence.sig --public-key /secure/review.pub
sudo rewind rollback --record ./runtime/record.json
sudo rewind recover --record ./runtime/record.json
sudo rewind commit --record ./runtime/record.json --confirm
# Optional Git branch adapter; branch must be clean and explicitly checked out:
sudo rewind branch apply --record ./runtime/record.json --repo ./project \
  --branch feature/agent-review --confirm --commit \
  --message "Accept reviewed agent result"
```

Successful runs discard the temporary upper/work layer by default. Add `--on-success review` when you explicitly need to inspect the merged view before choosing export or discard. The agent always sees a merged workspace backed by an OverlayFS lower/upper pair; the protected lower layer is never modified before acceptance. `export` writes a review-only JSON bundle containing before/after manifests and changes; `--format patch` renders regular text-file changes as a non-mutating unified diff, while `--format git-patch` uses Git’s read-only `--no-index --binary` mode for full-fidelity binary, directory, and mode changes. The JSON bundle remains canonical for machine inspection, and neither patch format merges into the workspace. Read policies can be disabled, audited, or enforced with user-defined glob patterns. Network policy supports an explicit loopback proxy backend for proxy-aware HTTP/HTTPS clients; `network.mode: audit` persists observations and `network.mode: enforce` applies allow/deny decisions. Enforce runs also deny raw/packet socket creation through seccomp; network namespaces and non-proxy-aware egress remain outside the guarantee. The default credential broker refuses raw secret exposure until a platform broker is configured. Candidate acceptance is conflict-checked against the immutable base before `rewind commit --confirm` applies regular-file and directory changes. The optional Git branch adapter requires a clean checkout of the named branch, runs Git patch preflight, refuses `.git` metadata changes, and only creates a commit when `--commit` and `--confirm` are both present.

Signed policy provenance is available without putting secrets in the package:

```sh
rewind policy keygen --private /path/to/policy-private.key --public /path/to/policy-public.key
rewind policy sign policy.yaml --name strict-agent --version 1.0.0 \
  --private-key /path/to/policy-private.key --output strict-agent.bundle.json
rewind policy verify strict-agent.bundle.json --public-key /path/to/policy-public.key
```

Signatures authenticate package contents; they do not bypass runtime capability checks or operator confirmation.

### Local supervisor control plane

The Linux VM can expose health, capability, and durable-history data over a
permissioned Unix socket. Action endpoints intentionally refuse until the
supervisor has an authenticated authorization layer:

```sh
sudo rewind supervisor --socket /tmp/rewind-supervisor.sock --history /tmp/rewind-history.json
```

For the browser Control Plane, expose an optional loopback-only HTTP bridge. It
requires an exact CORS origin and bearer token; non-loopback bind addresses are
refused:

```bash
sudo rewind supervisor \
  --socket /tmp/rewind-supervisor.sock \
  --history /tmp/rewind-history.json \
  --http-listen 127.0.0.1:8787 \
  --cors-origin http://127.0.0.1:4173 \
  --trusted-policy-keys /etc/rewind/trusted-signer.pub
```

The socket is intentionally mode `0600`; inspect it as the same privileged
user that owns the runtime (for example, `sudo curl --unix-socket
/tmp/rewind-supervisor.sock http://localhost/health`). A generated bearer token
is written to `/tmp/rewind-supervisor.sock.token` (mode `0600`) and is required
for action requests. Authenticated `status`, `rollback`/`recover`, and
explicit `commit` (`confirmation: "COMMIT"`) actions are routed through the
same lifecycle and conflict checks as the CLI. Each accepted or refused action
is appended to `/tmp/rewind-history.json.actions.jsonl` without tokens or file
contents. The Control Plane’s browser adapter can send those same intents and
persist validated local policy/workspace assignments or import a self-contained
Ed25519-signed policy bundle when the explicit HTTP bridge is enabled. The
bridge also exposes an authenticated signed-bundle inventory for export and
review. Pass
`--trusted-policy-keys` to require an organization signer allow-list; without
it, the supervisor still verifies the envelope’s embedded key and signature but
does not claim organization-level trust. Fixture mode remains the safe default
for the static demo.

Example policy:

```yaml
read:
  mode: enforce
  deny:
    - "**/.env"
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

# Optional cgroup-v2 guardrails; cpu_max is quota period.
resources:
  pids_max: "256"
  memory_max: "536870912"
  cpu_max: "50000 100000"
```

## Safe local commands

These commands do not perform kernel operations. They are safe to run on a development host because fixtures are synthetic and manifests operate on the directory explicitly supplied by the user:

```bash
make build
make test
./bin/rewind --help
./bin/rewind capabilities
./bin/rewind fixture create /tmp/rewind-fixture
./bin/rewind manifest create /tmp/rewind-fixture /tmp/rewind-manifest.json
./bin/rewind manifest verify /tmp/rewind-fixture /tmp/rewind-manifest.json
./bin/rewind policy check policies/example.yaml
./bin/rewind policy explain policies/example.yaml /workspace/.env
./bin/rewind policy learn --events /tmp/rewind-events.jsonl --output /tmp/rewind-policy-suggestion.yaml
```

The `run`, `status`, `events`, `verify`, `evidence verify`, `diff`, `export`, `rollback`, and explicit `commit --confirm` commands are wired for the disposable Linux VM. Commit compares the immutable base, current destination, and reviewed merged candidate; same-path drift, incomplete evidence, unsafe paths, and unsupported symlinks refuse the apply. `policy learn` produces an audit-mode, review-only allowlist suggestion and skips secret-like, virtual, and broad paths. Signed policy package commands (`policy keygen`, `policy sign`, and `policy verify`) provide provenance without bypassing runtime capability checks. `evidence verify` and the separately buildable `rewind-evidence` binary are read-only verification paths; neither loads eBPF or mounts filesystems.

VM-only run shape:

```bash
rewind run \
  --workspace /home/vagrant/demo-workspace \
  --runtime-root /home/vagrant/rewind-runs/run-1 \
  --policy /home/vagrant/demo-policy.yaml \
  --record /home/vagrant/rewind-runs/run-1/record.json \
  --sensor-object /home/vagrant/RewindBPF/ebpf/rewind_trace.bpf.o \
  --runtime-roots /bin,/usr/bin,/lib,/usr/lib \
  --overlay-backend fuse \
  --on-success review \
  -- /home/vagrant/demo-agent
```

The command must run inside the disposable Ubuntu VM. It checks capabilities, creates one cgroup-v2 scope, creates a `fuse-overlayfs` mount, gates agent `exec` until telemetry is attached, and starts the agent through the policy-aware helper. With `--on-success review`, the merged view stays available for inspection; without it, successful completion automatically discards upper/work. The record includes event count, byte count, SHA-256 digest, and a kernel-side dropped-event count; any dropped event marks evidence incomplete. The FUSE backend is the default because this VM's 6.8 kernel does not expose OverlayFS copy-up checks to an unprivileged agent reliably. Use `--overlay-backend kernel` only after a separate VM capability check. Do not run this on the personal Mac or against a real home directory.

For bounded telemetry retention, set `REWIND_EVENT_MAX_BYTES` to a positive total byte count inside the VM. The runtime continues draining kernel events after the cap, marks the run evidence `truncated=true`, and makes `verify` fail closed; it never presents a capped stream as complete. To rotate a long stream without truncation, set `REWIND_EVENT_ROTATE_BYTES`; the record stores the ordered `events.jsonl`, `events-000001.jsonl`, ... paths and the verifier hashes them as one chain. Explicit backpressure policy remains future work. If `resources` is present, the run writes the requested cgroup-v2 limits before the agent is released; a missing controller file fails closed. `network.mode: enforce` requires the explicit `--network-backend proxy` backend; audit mode can opt into the same backend to persist observations. The loopback proxy enforces domain policy for HTTP/HTTPS proxy-aware clients and injects proxy variables only into the agent process; enforce runs deny raw/packet socket creation, while non-proxy-aware clients and broader egress remain outside this guarantee.

When the run is launched with `sudo`, inspect and roll it back with `sudo` as well because the current MVP writes the `0600` run record and telemetry log as root:

```bash
sudo ./bin/rewind status --record /home/vagrant/rewind-runs/run-1/record.json
./bin/rewind events --record /home/vagrant/rewind-runs/run-1/record.json
./bin/rewind diff --record /home/vagrant/rewind-runs/run-1/record.json
sudo ./bin/rewind rollback --record /home/vagrant/rewind-runs/run-1/record.json
sudo ./bin/rewind recover --record /home/vagrant/rewind-runs/run-1/record.json
```

The runtime changes record and event-log ownership back to the invoking `SUDO_UID`/`SUDO_GID`, so status, events, diff, and `verify` are readable without `sudo`. A FUSE mount created by a privileged parent still requires `sudo` for unmount/rollback; `recover` is the explicit stale-run cleanup path. `verify` recomputes the JSONL digest and validates sequence/hash-chain links; it exits non-zero if the evidence was truncated, changed, or marked incomplete.

The parent may need `sudo` for OverlayFS/eBPF, but the helper drops the agent to the invoking user using `SUDO_UID`/`SUDO_GID`. Before mounting, only the temporary `upper/work` directories are chowned to that user; the original lower workspace is never chowned. A direct root agent is rejected.

The low-level telemetry smoke command is separate and privileged:

```bash
sudo ./bin/rewind sensor attach \
  --object /home/vagrant/RewindBPF/ebpf/rewind_trace.bpf.o \
  --run-id run_vm_smoke \
  --pid <agent-pid>
```

Run this only inside the disposable Ubuntu VM after the safety-gated attach step. It is not a replacement for the final `rewind run` workflow.

The Landlock read-enforcement smoke test is opt-in and does not require `sudo`, mounts, or real secrets. Run it only inside the disposable VM:

```bash
REWIND_LANDLOCK_INTEGRATION=1 GOTOOLCHAIN=local go test ./internal/landlock -run TestLandlockSyntheticReadEnforcement -count=1 -v
```

It creates synthetic files under a temporary VM directory, applies a read allowlist to a child process, and expects the protected synthetic file to fail with `EACCES`.

The OverlayFS manager also has an opt-in VM-only integration test. It writes one synthetic lower-layer marker, changes it through the merged mount, rolls back, and verifies that only the lower-layer original remains:

```bash
sudo env REWIND_OVERLAY_INTEGRATION=1 \
  GOTOOLCHAIN=local \
  go test ./internal/overlay \
  -run TestOverlaySyntheticMountRollback -count=1 -v
```

Run this only inside the disposable Ubuntu VM. It uses `t.TempDir()` under the VM’s temporary filesystem and does not touch the Mac host or a real project. If the VM lacks `CAP_SYS_ADMIN`, the test must be treated as an environment limitation, not as permission to broaden the test scope.

The first full protected-run smoke was verified in the disposable Ubuntu VM on 2026-07-18 using only a generated workspace. The agent was denied access to `synthetic.env`, deleted `src/`, created `generated.txt` in the merged view, emitted eBPF telemetry, and completed with `state=succeeded`. A subsequent rollback unmounted the FUSE view, discarded the generated file, preserved `original-source` in the lower workspace, and recorded `state=rolled_back`.

## Repository layout

```text
cmd/rewind/       CLI entry point
docs/             technical architecture and project plan
ebpf/             C/libbpf kernel sensor and optional enforcement programs
internal/runplan/ pre-execution protected-run composition
internal/protectedrun/ run lifecycle ordering and fail-closed cleanup
policies/         safe example policies
benchmarks/       reproducible benchmark scripts, ledgers, and charts
docs/PHASE2_PLAN.md Phase 2 hardening and productisation roadmap
docs/PRODUCT_STRATEGY.md product wedge, competitive position, and native-platform roadmap
tests/            integration-test safety notes
```

## Development prerequisites

- Go 1.22 or newer
- Linux VM for kernel integration
- OverlayFS and BPF/BTF support in the VM kernel
- `fuse-overlayfs` (the default protected-run backend; install with `sudo apt-get install -y fuse-overlayfs`)
- Landlock **or an active BPF LSM** for read enforcement (the current VM reports Landlock active)
The disposable VM setup and safety boundary are documented in [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md). The MVP runs directly inside that VM.

## Verification

```bash
go test ./...
go vet ./...
make build
git diff --check
```

For the full business context, security model, flow diagrams, benchmark matrix, safety gates, and implementation status, read [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) and [docs/PROJECT_PLAN.md](docs/PROJECT_PLAN.md).
