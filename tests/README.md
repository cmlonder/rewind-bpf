# Tests

Integration tests must run inside a Linux VM. Destructive tests against a filesystem are allowed only inside a disposable VM or an explicitly created test image.

Before every rollback test, create a hash/metadata manifest for the lower layer and compare it after rollback.

Stage 1 unit tests run on the development host and cover synthetic fixture creation, manifest verification, recursive glob matching, policy modes, and run ID generation. They do not load eBPF or mount filesystems.

The disposable-VM acceptance gate covers the product boundary that cannot run
on a development host:

```bash
REWIND_VM_CONFIRM=VM_ONLY make acceptance-vm
```

It creates only synthetic data under a temporary VM directory and checks read
denial, recursive deletion rollback, explicit commit, destination-drift
refusal, proxy allow/deny behavior, and incomplete-evidence refusal. The script
requires a built `/tmp/rewind` binary and compiled `ebpf/rewind_trace.bpf.o`.
