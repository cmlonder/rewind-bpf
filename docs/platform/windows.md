# Windows adapter (P3)

The Windows target is a native policy/workspace backend, not a renamed Linux
VM. The adapter must own a process Job Object (for descendant containment), a
filesystem policy boundary, and a disposable project workspace. WSL2 remains
supported only as a Linux development compatibility path and must be labelled
as such in the UI.

The current repository provides the shared capability contract and keeps the
Go packages cross-compilable. It does not enable a Windows run: the capability
report remains unsupported until the native helper and destructive tests run on
a disposable Windows volume.
