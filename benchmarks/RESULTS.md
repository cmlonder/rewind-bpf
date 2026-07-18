# Benchmark results

**Status:** Living result ledger

These values are the measurements captured in the disposable Ubuntu 24.04 ARM64 VM. The machine was running kernel `6.8.0-49-generic`, fio `3.36`, and buffered I/O (`direct=0`). Raw fio JSON files remain in the VM's temporary benchmark directories; this checked-in CSV is the durable summary used for charts and presentation material.

## Current results

| Variant | Read IOPS | Write IOPS | Read BW | Write BW | Read p50 | Write p50 | Upper bytes | Notes |
|---|---:|---:|---:|---:|---:|---:|---:|---|
| B0 native ext4 | 10,334.2 | 4,421.0 | 41,336.6 KiB/s | 17,683.4 KiB/s | 79.2 µs | 3.344 µs | 134,217,728 | Five repetitions |
| B2 FUSE-only | 9,143.8 | 3,915.4 | 36,574.8 KiB/s | 15,661.4 KiB/s | 66.2 µs | 68.096 µs | 134,217,728 | Five repetitions |
| B4 Rewind protected | 9,181.7 | 3,932.6 | 36,726.2 KiB/s | 15,729.8 KiB/s | 66.7 µs | 68.710 µs | 134,253,346 | Five repetitions in one run |

B4 throughput was approximately 11.1% below B0 and 0.4% above B2. The current result is warm/page-cache exploratory data, not the final cold-cache claim.

## Telemetry result

The direct-fio PID validation generated 16,620 events in a 2,467,528-byte JSONL log: 16,403 `write`, 216 `openat`, and one `unlinkat`. The current PID-only sensor does not follow shell-launched child processes; cgroup/descendant scoping is a follow-up hardening item.

The static footprint was 5,670,919 bytes for the `rewind` binary and 21,352 bytes for the compiled eBPF object. The run record was 746 bytes. The telemetry stream averaged approximately 148.5 bytes per event for this JSONL format.

## Storage interpretation

For the full-file 128 MiB fio workload, native and FUSE upper storage were both 134,217,728 bytes, so measured copy-up amplification was approximately 1.0x. The B4 upper layer was 134,253,346 bytes because it also contained five small fio JSON outputs and metadata.

The CSV is intentionally separate from `benchmarks/results/`, which remains ignored for large raw artifacts. A future benchmark run should append a new dated row or replace this ledger only after preserving the previous commit.
