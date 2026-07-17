# RewindBPF

RewindBPF is an **AI Agent Safety Runtime** designed to run autonomous agents inside reversible, policy-controlled filesystem transactions on Linux.

Core idea:

```text
Start the agent
    ↓
Prepare a mount namespace + OverlayFS
    ↓
Observe filesystem/process events with eBPF
    ↓
Enforce sensitive-read policies with Landlock/BPF LSM
    ↓
Commit on success, rollback on failure
```

This project is not an AI agent, Codex skill, or IDE extension. The core product is a Linux daemon, CLI, eBPF program, and OverlayFS-based sandbox. MCP, plugin, or IDE adapters can be added later.

## Status

This repository is the bootstrap scaffold. Runtime behavior is not implemented yet; the architecture decisions and seven-day MVP plan are documented in [docs/PROJECT_PLAN.md](docs/PROJECT_PLAN.md).

## Planned components

- `rewind`: user-facing CLI
- `rewindd`: sandbox, process, policy, and rollback manager
- `ebpf/`: C + libbpf/CO-RE kernel programs
- OverlayFS: copy-on-write filesystem transaction layer
- Landlock/BPF LSM: user-defined filesystem access policies
- VM/namespaces: agent isolation
- `benchmarks/`: baseline, overhead, and rollback measurements

## Development

Requirements: Go, a Linux VM, OverlayFS, and a kernel with eBPF/BTF support.

```bash
make build
make test
./bin/rewind --help
```

Kernel integration must be developed in an isolated Ubuntu VM rather than directly on a macOS host. See the project plan for scope, security boundaries, benchmark design, and test scenarios.
