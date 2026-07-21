# Windows adapter (P3)

The Windows target is a native policy/workspace backend, not a renamed Linux
VM. The adapter must own a process Job Object (for descendant containment), a
filesystem policy boundary, and a disposable project workspace. WSL2 remains
supported only as a Linux development compatibility path and must be labelled
as such in the UI.

The current repository provides the shared capability contract, a configurable
Job Object launcher with kill-on-close cleanup, a read-only prerequisite plan,
and exact signed-helper verification; the Go packages cross-compile to
`windows/amd64`. It does not enable a Windows host run until the signed native
helper and destructive tests run on a disposable Windows volume. WSL2 remains
Linux compatibility, never Windows-host protection.

On a Windows test host, run `scripts/windows_acceptance.ps1` for the safe
contract preflight. It creates only a temporary directory and verifies that
the portable Job Object/restricted-token/VHDX contract is complete while
keeping the signed-helper and disposable-VHDX gates visible.
