# RewindBPF

RewindBPF is an **AI Agent Safety Runtime** for running autonomous agents inside reversible, policy-controlled Linux filesystem transactions.

It protects the agent operator from destructive changes and unauthorized sensitive-file access without requiring changes to the agent itself.

## Current status

Stage 1 is complete: safe synthetic fixtures, SHA-256 manifests, run IDs, glob policy parsing, and CLI smoke checks are available. OverlayFS, eBPF, namespace, and policy enforcement are being implemented incrementally in a disposable Linux environment.

Track the implementation and architecture in [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md). The architecture document is updated after every completed stage.

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
