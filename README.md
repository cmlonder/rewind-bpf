# RewindBPF

RewindBPF is an **AI Agent Safety Runtime** for running autonomous agents inside reversible, policy-controlled Linux filesystem transactions.

It protects the agent operator from destructive changes and unauthorized sensitive-file access without requiring changes to the agent itself.

## Current status

The MVP is complete for its explicitly documented disposable-VM boundary. Phase 2 P0 hardening is now in progress: cgroup-v2 run scopes, capability reporting, atomic prepared-run journaling, idempotent recovery, invoker-owned metadata, event evidence digests, a sensor start gate, dropped-event accounting, parent-crash recovery, and a read-only merged-view diff command are implemented and VM-smoke-tested. Stage 6 protected-run integration and Stage 7 benchmark controls remain validated: safe synthetic fixtures, SHA-256 manifests, run IDs, glob policy parsing, a protected-run state machine, a shared eBPF event contract, a userspace ring-buffer decoder/reader, scoped telemetry with descendant-PID tracking, a manifest-to-kernel read-rule compiler, a Landlock allowlist planner, process-level read denial, OverlayFS mount/rollback, a fail-closed coordinator, a policy-aware helper, and the user-facing `rewind run/status/events/rollback` flow are available. Warm and cold B0/B2/B4 measurements, storage footprint, telemetry growth, and benchmark charts are recorded. Remaining Phase 2 work is broader crash-edge coverage, bounded log rotation, network/credential policy planes, conflict-safe export, and release rehearsal.

Track the implementation and architecture in [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md). The architecture document is updated after every completed stage.

The six-day hardening sprint and the post-hackathon product roadmap are in [docs/PHASE2_PLAN.md](docs/PHASE2_PLAN.md). It includes the competitive analysis, P0/P1 work packages, exit criteria, correctness matrix, and research references.

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
- Our MVP focuses on a narrower integration: **OverlayFS as the write transaction, eBPF as the low-overhead sensor/enforcement path, and a userspace run controller as the rollback authority**.
- Unlike command deny-lists, the rollback boundary does not depend on recognizing every spelling of `rm`, `mv`, a shell script, or a library call. The agent can perform normal writes inside the merged view; the lower layer remains unchanged until an explicit commit/export.
- Unlike a post-hoc backup, the expensive copy is avoided on the hot path. Copy-on-write occurs only for blocks/files that the agent actually changes. This is a benchmark hypothesis, not an unmeasured guarantee.
- Unlike a generic container sandbox, the project makes the file transaction and recovery invariant explicit and testable. The MVP still uses a disposable VM for privileged Linux integration and does not claim to reverse kernel, device, network, or external-service side effects.

The benchmark plan deliberately compares these tradeoffs rather than relying on a “near-zero overhead” slogan: native ext4, eBPF-only, OverlayFS-only, OverlayFS + eBPF, and the full daemon path are measured separately. See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for the test matrix and safety boundary.

Captured VM measurements are summarized in [benchmarks/RESULTS.md](benchmarks/RESULTS.md) and [benchmarks/results_summary.csv](benchmarks/results_summary.csv). The CSV is the input for the final Python charts; raw VM artifacts remain outside Git unless explicitly archived.

## Safety warning

Do not run kernel, mount, or destructive tests directly on a personal host. RewindBPF integration tests must run in a disposable Ubuntu VM or an explicitly created test image. Do not bind-mount a real home directory, real project, `.env`, SSH keys, or personal data into a destructive test.

The approved integration-test boundary is a disposable Ubuntu VM where RewindBPF runs directly. The recommended layout is:

```text
macOS host → disposable Ubuntu VM → RewindBPF directly
```

## User workflow

The primary workflow runs inside the disposable Linux VM:

```bash
sudo rewind run --workspace ./project --runtime-root ./runtime \
  --policy ./policy.yaml --record ./runtime/record.json -- agent-command
sudo rewind status --record ./runtime/record.json
sudo rewind events --record ./runtime/record.json
sudo rewind diff --record ./runtime/record.json
sudo rewind rollback --record ./runtime/record.json
sudo rewind recover --record ./runtime/record.json
```

The agent will see a merged workspace backed by an OverlayFS lower/upper pair. Rollback discards the temporary upper layer. Read policies can be disabled, audited, or enforced with user-defined glob patterns.

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
```

The `run`, `status`, `events`, and `rollback` commands are now wired for the disposable Linux VM. `commit` remains intentionally disabled until diff/export semantics are implemented and verified.

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
  -- /home/vagrant/demo-agent
```

The command must run inside the disposable Ubuntu VM. It checks capabilities, creates one cgroup-v2 scope, creates a `fuse-overlayfs` mount, gates agent `exec` until telemetry is attached, starts the agent through the policy-aware helper, and leaves a successful run mounted until `rewind rollback --record ...` is called. The record includes event count, byte count, SHA-256 digest, and a kernel-side dropped-event count; any dropped event marks evidence incomplete. The FUSE backend is the default because this VM's 6.8 kernel does not expose OverlayFS copy-up checks to an unprivileged agent reliably. Use `--overlay-backend kernel` only after a separate VM capability check. Do not run this on the personal Mac or against a real home directory.

When the run is launched with `sudo`, inspect and roll it back with `sudo` as well because the current MVP writes the `0600` run record and telemetry log as root:

```bash
sudo ./bin/rewind status --record /home/vagrant/rewind-runs/run-1/record.json
./bin/rewind events --record /home/vagrant/rewind-runs/run-1/record.json
./bin/rewind diff --record /home/vagrant/rewind-runs/run-1/record.json
sudo ./bin/rewind rollback --record /home/vagrant/rewind-runs/run-1/record.json
sudo ./bin/rewind recover --record /home/vagrant/rewind-runs/run-1/record.json
```

The runtime changes record and event-log ownership back to the invoking `SUDO_UID`/`SUDO_GID`, so status, events, and diff are readable without `sudo`. A FUSE mount created by a privileged parent still requires `sudo` for unmount/rollback; `recover` is the explicit stale-run cleanup path.

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
ebpf/             planned C/libbpf kernel programs
internal/runplan/ pre-execution protected-run composition
internal/protectedrun/ run lifecycle ordering and fail-closed cleanup
policies/         safe example policies
benchmarks/       benchmark design and future results
docs/PHASE2_PLAN.md Phase 2 hardening and productisation roadmap
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
