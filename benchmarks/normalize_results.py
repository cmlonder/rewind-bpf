#!/usr/bin/env python3
"""Derive comparable storage, evidence, and lifecycle metrics from the ledger."""

from __future__ import annotations

import argparse
import csv
from pathlib import Path


WORKLOAD_BYTES = 128 * 1024 * 1024
DERIVED_FIELDS = [
    "storage_amplification_x",
    "event_bytes_per_event",
    "lifecycle_seconds",
    "read_gap_vs_b0_pct",
    "write_gap_vs_b0_pct",
]


def number(row: dict[str, str], key: str) -> float | None:
    raw = (row.get(key) or "").strip()
    return float(raw) if raw else None


def derive(rows: list[dict[str, str]]) -> list[dict[str, str]]:
    baseline = next((row for row in rows if row.get("variant") == "B0-native-ext4"), None)
    base_read = number(baseline or {}, "read_iops")
    base_write = number(baseline or {}, "write_iops")
    output: list[dict[str, str]] = []
    for source in rows:
        row = dict(source)
        upper = number(row, "upper_bytes")
        telemetry = number(row, "telemetry_bytes")
        events = number(row, "event_count")
        read = number(row, "read_iops")
        write = number(row, "write_iops")
        row["storage_amplification_x"] = f"{upper / WORKLOAD_BYTES:.4f}" if upper else ""
        row["event_bytes_per_event"] = f"{telemetry / events:.2f}" if telemetry and events else ""
        row["lifecycle_seconds"] = row.get("wall_seconds", "") or ""
        row["read_gap_vs_b0_pct"] = f"{(1 - read / base_read) * 100:.2f}" if read and base_read else ""
        row["write_gap_vs_b0_pct"] = f"{(1 - write / base_write) * 100:.2f}" if write and base_write else ""
        output.append(row)
    return output


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--input", type=Path, default=Path(__file__).with_name("results_summary.csv"))
    parser.add_argument("--output", type=Path, default=Path(__file__).with_name("results_normalized.csv"))
    args = parser.parse_args()
    with args.input.open(newline="") as handle:
        rows = list(csv.DictReader(handle))
    fields = list(rows[0].keys()) + DERIVED_FIELDS if rows else []
    normalized = derive(rows)
    with args.output.open("w", newline="") as handle:
        writer = csv.DictWriter(handle, fieldnames=fields, lineterminator="\n")
        writer.writeheader()
        writer.writerows(normalized)
    print(f"BENCHMARK_NORMALIZED=PASS rows={len(normalized)} output={args.output}")


if __name__ == "__main__":
    main()
