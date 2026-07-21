# Platform delivery status

This is the short, operator-facing answer to “what is still missing?” The
machine-readable version is available without changing host state:

```bash
rewind platform status
```

## Complete in the repository

- Linux reference path: OverlayFS/FUSE, Landlock, cgroup-v2, eBPF evidence,
  rollback/recovery, policy, supervisor, and acceptance workflows.
- macOS native backend: APFS clone-backed staged workspace, Seatbelt scoped
  launcher, sensitive-read hiding, review/diff/rollback/commit lifecycle,
  destination conflict checks, APFS prerequisite probe, platform contract, and
  exact signed-helper checksum/signature verification. EndpointSecurity,
  network, and resource enforcement remain explicitly unavailable.
- Windows native Go contract: Job Object kill-on-close lifecycle, configurable
  cwd/environment/stdio launch options, read-only PowerShell/fsutil probe,
  platform contract, and exact signed-helper checksum/signature verification.
- Cross-build outputs: `darwin/arm64` and `windows/amd64` CLI builds plus
  target platform test binaries are produced in CI/local verification.
- UI/site language: native targets are shown as “code complete / manual gate”;
  they are never presented as Linux capabilities.

## Acceptance status

### Linux (disposable Ubuntu VM)

- **PASS (2026-07-21):** `REWIND_VM_CONFIRM=VM_ONLY make final-vm` completed from
  a disposable Ubuntu 24.04 ARM64 UTM VM. It covers crash/recovery and rollback
  fixtures, PII/read denial, review/commit and conflict refusal, proxy and
  namespace egress, final benchmarks, release checksums, and the jury demo.
- Keep all privileged OverlayFS/eBPF tests inside the disposable VM. The
  development Mac is not a target.

### macOS (disposable APFS volume or VM)

- The synthetic host gate is `make mac-native-smoke`; it validates Seatbelt,
  APFS clone isolation, sensitive-read denial, rollback, commit, and conflict
  refusal using only a temporary fixture.
- `make mac-crash-smoke` adds a synthetic `SIGKILL` acceptance: the failed
  child follows the rollback path, the lower marker remains intact, the
  candidate file is absent, and the event sidecar records the failed exit.
- `make evidence-bundle` packages the UI/macOS smoke logs, benchmark ledger,
  normalized chart, platform status, and SHA-256 manifest into `dist/`.
- Build/install a signed EndpointSecurity helper with the required entitlement
  for syscall/file telemetry and network/resource enforcement.
- On disposable storage, validate crash recovery and helper-backed evidence;
  the development Mac is still not a target for destructive or privileged
  operations.

### Windows (disposable VM + VHDX)

- Install the signed filesystem minifilter/service and restricted-token launch
  helper.
- Validate Job Object descendant cleanup, VHDX differencing-disk rollback,
  sensitive-read denial, crash recovery, and destination conflict refusal.

The remaining native-helper gates require the target operating systems,
privileged entitlements, and disposable storage. They cannot be validated on
the personal Mac or replaced by a WSL2 claim about Windows-host protection.

## Productisation after the gates

Remote/KMS retention, distributed session consensus, SDK-specific callbacks,
content classifiers, multi-agent orchestration, remote checkpoint graphs, and
CRIU/process-memory research remain post-gate product tracks. They are not
required to call the three native platform adapters code-complete.
