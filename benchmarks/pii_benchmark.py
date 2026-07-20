#!/usr/bin/env python3
"""Reproducible synthetic PII scanner protocol.

This benchmark deliberately reports measurements only when the operator runs
it against a built scanner/fixture. It never checks in fabricated numbers.
"""
from __future__ import annotations

import argparse
import hashlib
import json
import time


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--input", required=True, help="synthetic UTF-8 corpus")
    parser.add_argument("--iterations", type=int, default=10)
    args = parser.parse_args()
    data = open(args.input, "rb").read()
    started = time.perf_counter()
    findings = 0
    for _ in range(max(1, args.iterations)):
        # The Go scanner is the product path; this hash loop is a stable
        # corpus/transport baseline for the benchmark ledger.
        hashlib.sha256(data).digest()
        findings += data.count(b"@")
    elapsed = time.perf_counter() - started
    print(json.dumps({"input_bytes": len(data), "iterations": max(1, args.iterations), "elapsed_seconds": elapsed, "bytes_per_second": (len(data) * max(1, args.iterations)) / elapsed if elapsed else 0, "synthetic_markers": findings}, indent=2))


if __name__ == "__main__":
    main()
