#!/usr/bin/env python3
"""Append a reproducible fio/storage/telemetry row to the benchmark ledger."""

from __future__ import annotations

import argparse
import csv
import glob
import json
import os
from pathlib import Path


FIELDS = ["variant", "read_iops", "write_iops", "read_bw_kib_s", "write_bw_kib_s", "read_p50_us", "read_p95_us", "read_p99_us", "write_p50_us", "write_p95_us", "write_p99_us", "upper_bytes", "telemetry_bytes", "event_count", "wall_seconds", "notes"]


def percentile(job: dict, direction: str, key: str) -> float:
    value = job.get(direction, {}).get("clat_ns", {}).get("percentile", {}).get(key)
    return float(value) / 1000 if value is not None else 0.0


def average(files: list[str]) -> dict[str, float]:
    rows = [json.loads(Path(path).read_text())["jobs"][0] for path in files]
    def mean(values: list[float]) -> float: return sum(values) / len(values) if values else 0.0
    result: dict[str, float] = {}
    for direction in ("read", "write"):
        result[f"{direction}_iops"] = mean([float(row.get(direction, {}).get("iops", 0)) for row in rows])
        result[f"{direction}_bw_kib_s"] = mean([float(row.get(direction, {}).get("bw", 0)) for row in rows])
        for label, key in (("p50", "50.000000"), ("p95", "95.000000"), ("p99", "99.000000")):
            result[f"{direction}_{label}_us"] = mean([percentile(row, direction, key) for row in rows])
    return result


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--variant", required=True)
    parser.add_argument("--fio-glob", help="glob of fio JSON files")
    parser.add_argument("--output", type=Path, default=Path("benchmarks/results_summary.csv"))
    parser.add_argument("--upper-bytes", type=int, default=0)
    parser.add_argument("--telemetry-bytes", type=int, default=0)
    parser.add_argument("--event-count", type=int, default=0)
    parser.add_argument("--wall-seconds", type=float, default=0)
    parser.add_argument("--notes", default="")
    args = parser.parse_args()
    row = {field: "" for field in FIELDS}; row["variant"] = args.variant
    files = sorted(glob.glob(args.fio_glob)) if args.fio_glob else []
    if files: row.update({key: f"{value:.3f}" for key, value in average(files).items()})
    row.update({"upper_bytes": args.upper_bytes, "telemetry_bytes": args.telemetry_bytes, "event_count": args.event_count, "wall_seconds": args.wall_seconds, "notes": args.notes})
    existing = list(csv.DictReader(args.output.open(newline=""))) if args.output.exists() else []
    existing = [item for item in existing if item.get("variant") != args.variant]
    args.output.parent.mkdir(parents=True, exist_ok=True)
    with args.output.open("w", newline="") as handle:
        writer = csv.DictWriter(handle, fieldnames=FIELDS); writer.writeheader(); writer.writerows(existing + [row])
    print(f"wrote {args.variant} to {args.output} ({len(files)} fio files)")


if __name__ == "__main__":
    main()
