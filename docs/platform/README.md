# Native platform adapter contract

RewindBPF has one product invariant across operating systems: the agent must
not receive direct write access to the real project, and a missing safety
primitive must fail closed rather than silently degrade.

## macOS (native filesystem backend / helper gate)

The native adapter combines a Seatbelt process/write boundary with an APFS
clone-backed disposable workspace. `platform.PlanForWorkspace` performs a
read-only prerequisite probe for APFS, `sandbox-exec`, and `diskutil`. The
native CLI now runs the staged filesystem lifecycle on macOS: clone, launch,
diff, rollback, and conflict-checked commit. Sensitive paths matched by
`read.mode: enforce` (including PII findings) are moved into the disposable
runtime while the agent runs and restored afterwards, so the agent never sees
their contents.

The implementation boundary is deliberately separate from the Linux
OverlayFS/Landlock/eBPF path. A native helper may share policy and evidence
schemas, but it must provide its own capability proof and rollback tests.

`rewind platform contract --platform darwin --workspace PATH` emits the shared
contract used by the UI and release checklist. `rewind platform plan` remains a
read-only prerequisite report. `platform.SeatbeltProfile` is a reviewable
profile generator. The darwin build also exposes
`SeatbeltCommandWithOptions`, which scopes cwd, environment, and runtime read
roots for a staged workspace without widening write access.

The darwin build also contains a `SeatbeltCommand` wrapper that creates a
disposable profile file and launches a command through `sandbox-exec`, with an
explicit cleanup callback. The native CLI supplies the APFS clone transaction
and rollback lifecycle; EndpointSecurity coverage remains a separate signed
helper gate.

### Native run workflow

Keep the record outside the disposable runtime because rollback removes the
runtime directory. The event sidecar is written beside the record as
`<record>.events.jsonl` for the same reason:

```bash
rewind native run \
  --workspace ./project \
  --runtime-root /tmp/rewind-runtime \
  --policy ./policy.yaml \
  --record ./run.json \
  --on-success review -- claude

rewind native diff --record ./run.json
rewind native rollback --record ./run.json
# or, after reviewing the diff:
rewind native commit --record ./run.json --confirm
```

The macOS path currently refuses `network.mode: enforce`,
`write.scope: system`, and `resources.*` limits. It does not yet emit
EndpointSecurity kernel/file telemetry, enforce network egress, or constrain
process resources; those require a signed privileged helper. The run record
therefore describes lifecycle/policy decisions, not Linux-level syscall
evidence. Unsupported promises fail closed.

See the focused [macOS backend guide](macos.md) for the command workflow and
the exact limitation boundary.

## Windows (native code-complete / manual gate)

Windows will use a native process/filesystem policy adapter and a disposable
workspace. WSL2 is a compatibility path for Linux development only; it must
never be presented as protection for the Windows host filesystem.

`rewind platform contract --platform windows --workspace PATH` emits the
Job Object/restricted-token/VHDX boundary and its manual gates. The portable
contract does not pretend to enforce a Windows host from Linux or WSL2.

The windows build contains a Job Object helper with
`JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE`, configurable cwd/env/stdio options, and a
`Wait` lifecycle method that closes the job after the command exits. The helper
is cross-compiled and intentionally not advertised as filesystem protection
until the signed minifilter and disposable VHDX tests exist.

## Read-only status and helper trust

```bash
rewind platform status
rewind platform status --helper-manifest /secure/rewind-native-helper.json
```

The status matrix separates `code_complete`, `helper_verified`,
`enforcement_ready`, and `manual_gate_required`. A helper manifest binds a
target platform and exact SHA-256 bytes; an optional detached Ed25519 release
signature can pin the publisher key. Verification never launches the helper or
changes host state.

## Test rule

Native backend tests require a disposable VM/volume and explicit platform
fixtures. The development Mac is not a test target for destructive or
privileged operations.

`make mac-safe-smoke` is the approved host-side contract check. It tests contracts,
PII scanning, registry/session/run-plan packages, and the expected refusal of
the Linux protected-run path without mounting, loading eBPF, changing cgroups,
or touching a real workspace.

The first safe command is:

```bash
rewind platform plan --workspace /path/to/disposable-apfs-fixture
```

This only reports prerequisites. It is not an enforcement or rollback test.

`make mac-native-smoke` is the approved synthetic native lifecycle check. It
uses only a temporary APFS fixture and verifies review/rollback, sensitive-read
denial, explicit commit, and destination-conflict refusal. It never targets a
real project directory.
