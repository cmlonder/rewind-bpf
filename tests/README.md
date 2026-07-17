# Tests

Integration tests must run inside a Linux VM. Destructive tests against a filesystem are allowed only inside a disposable VM or an explicitly created test image.

Before every rollback test, create a hash/metadata manifest for the lower layer and compare it after rollback.

Stage 1 unit tests run on the development host and cover synthetic fixture creation, manifest verification, recursive glob matching, policy modes, and run ID generation. They do not load eBPF or mount filesystems.
