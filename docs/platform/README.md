# Native platform adapter contract

RewindBPF has one product invariant across operating systems: the agent must
not receive direct write access to the real project, and a missing safety
primitive must fail closed rather than silently degrade.

## macOS (P2)

The planned adapter combines a Seatbelt profile for process/filesystem policy,
EndpointSecurity for high-fidelity process and file telemetry, and an APFS
disposable workspace/snapshot boundary. The current `internal/platform`
capability report intentionally marks this backend unavailable. No macOS
command is allowed to claim project isolation until a native helper has been
tested on a disposable APFS volume.

The implementation boundary is deliberately separate from the Linux
OverlayFS/Landlock/eBPF path. A native helper may share policy and evidence
schemas, but it must provide its own capability proof and rollback tests.

## Windows (P3 preview)

Windows will use a native process/filesystem policy adapter and a disposable
workspace. WSL2 is a compatibility path for Linux development only; it must
never be presented as protection for the Windows host filesystem.

## Test rule

Native backend tests require a disposable VM/volume and explicit platform
fixtures. The development Mac is not a test target for destructive or
privileged operations.
