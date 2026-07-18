# RewindBPF

RewindBPF is an **AI Agent Safety Runtime** for running autonomous agents inside reversible, policy-controlled Linux filesystem transactions.

It protects the agent operator from destructive changes and unauthorized sensitive-file access without requiring changes to the agent itself.

## Current status

Stage 4 foundations are in place: safe synthetic fixtures, SHA-256 manifests, run IDs, glob policy parsing, a protected-run state machine, and a shared eBPF event contract are available. OverlayFS lifecycle code has passed unit tests and a controlled disposable-VM smoke test. eBPF loading, read enforcement, and the end-to-end daemon are still being implemented incrementally in the disposable Linux environment.

Track the implementation and architecture in [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md). The architecture document is updated after every completed stage.

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

## Safety warning

Do not run kernel, mount, or destructive tests directly on a personal host. RewindBPF integration tests must run in a disposable Ubuntu VM or an explicitly created test image. Do not bind-mount a real home directory, real project, `.env`, SSH keys, or personal data into a destructive test.

The approved integration-test boundary is a disposable Ubuntu VM where RewindBPF runs directly. The recommended layout is:

```text
macOS host → disposable Ubuntu VM → RewindBPF directly
```

## Planned user workflow

Once the runtime stages are implemented, the primary workflow will be:

```bash
rewind run --workspace ./project --policy ./policy.yaml -- agent-command
rewind status
rewind events <run_id>
rewind rollback <run_id>
rewind commit <run_id>
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
./bin/rewind fixture create /tmp/rewind-fixture
./bin/rewind manifest create /tmp/rewind-fixture /tmp/rewind-manifest.json
./bin/rewind manifest verify /tmp/rewind-fixture /tmp/rewind-manifest.json
./bin/rewind policy check policies/example.yaml
```

The runtime commands (`run`, `status`, `events`, `rollback`, and `commit`) currently expose the planned interface only; they will remain disabled until the Linux VM integration stages are completed.

## Repository layout

```text
cmd/rewind/       CLI entry point
docs/             technical architecture and project plan
ebpf/             planned C/libbpf kernel programs
policies/         safe example policies
benchmarks/       benchmark design and future results
tests/            integration-test safety notes
```

## Development prerequisites

- Go 1.22 or newer
- Linux VM for kernel integration
- OverlayFS, BPF/BTF, and Landlock support in the VM kernel
The disposable VM setup and safety boundary are documented in [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md). The MVP runs directly inside that VM.

## Verification

```bash
go test ./...
go vet ./...
make build
git diff --check
```

For the full business context, security model, flow diagrams, benchmark matrix, safety gates, and implementation status, read [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) and [docs/PROJECT_PLAN.md](docs/PROJECT_PLAN.md).
