#!/usr/bin/env python3
"""Render the checked-in benchmark summary as a dependency-free SVG chart."""

from __future__ import annotations

import argparse
import csv
import html
from pathlib import Path


COLORS = {"B0-native-ext4": "#2563eb", "B2-fuse-only": "#f59e0b", "B4-rewind-protected": "#16a34a"}
LABELS = {"B0-native-ext4": "B0 native", "B2-fuse-only": "B2 FUSE", "B4-rewind-protected": "B4 Rewind"}


def load_rows(path: Path) -> list[dict[str, str]]:
    with path.open(newline="") as handle:
        return [row for row in csv.DictReader(handle) if row["variant"] in LABELS]


def number(row: dict[str, str], key: str) -> float | None:
    value = row.get(key, "").strip()
    return float(value) if value else None


def panel(rows: list[dict[str, str]], x: int, y: int, width: int, height: int, title: str, keys: tuple[str, str], unit: str) -> list[str]:
    values = [number(row, key) for row in rows for key in keys]
    maximum = max((value for value in values if value is not None), default=1.0)
    maximum *= 1.15
    baseline = y + height - 42
    lines = [f'<text x="{x}" y="{y + 20}" class="title">{html.escape(title)}</text>']
    lines.append(f'<line x1="{x + 48}" y1="{y + 32}" x2="{x + width}" y2="{y + 32}" class="axis"/>')
    lines.append(f'<line x1="{x + 48}" y1="{y + 32}" x2="{x + 48}" y2="{baseline}" class="axis"/>')
    lines.append(f'<text x="{x + 2}" y="{baseline + 4}" class="tick">0</text>')
    lines.append(f'<text x="{x + 2}" y="{y + 38}" class="tick">{maximum:.0f}</text>')
    group_width = (width - 64) / max(len(rows), 1)
    bar_width = min(34, group_width / 3)
    for index, row in enumerate(rows):
        group_x = x + 58 + index * group_width
        for offset, key in enumerate(keys):
            value = number(row, key)
            if value is None:
                continue
            bar_height = (value / maximum) * (height - 76)
            bar_x = group_x + offset * (bar_width + 3)
            bar_y = baseline - bar_height
            lines.append(f'<rect x="{bar_x:.1f}" y="{bar_y:.1f}" width="{bar_width:.1f}" height="{bar_height:.1f}" fill="{COLORS[row["variant"]]}"/>')
            lines.append(f'<text x="{bar_x + bar_width / 2:.1f}" y="{bar_y - 4:.1f}" text-anchor="middle" class="value">{value:.1f}</text>')
        lines.append(f'<text x="{group_x + group_width / 2:.1f}" y="{baseline + 18}" text-anchor="middle" class="label">{LABELS[row["variant"]]}</text>')
    lines.append(f'<text x="{x + width - 4}" y="{baseline + 34}" text-anchor="end" class="unit">{html.escape(unit)}</text>')
    return lines


def render(rows: list[dict[str, str]]) -> str:
    lines = [
        '<svg xmlns="http://www.w3.org/2000/svg" width="1200" height="720" viewBox="0 0 1200 720">',
        '<style>text{font-family:Inter,Arial,sans-serif;fill:#172033}.title{font-size:17px;font-weight:700}.axis{stroke:#94a3b8;stroke-width:1}.tick,.unit{font-size:10px;fill:#64748b}.label{font-size:11px}.value{font-size:9px}.legend{font-size:12px}</style>',
        '<rect width="1200" height="720" fill="#ffffff"/>',
        '<text x="48" y="34" font-size="24" font-weight="700">RewindBPF benchmark summary</text>',
        '<text x="48" y="56" font-size="12" fill="#64748b">Five-run buffered fio measurements · normalized storage, evidence, and lifecycle ledger</text>',
    ]
    lines += panel(rows, 48, 78, 520, 250, "Throughput", ("read_bw_kib_s", "write_bw_kib_s"), "KiB/s")
    lines += panel(rows, 610, 78, 520, 250, "p50 completion latency", ("read_p50_us", "write_p50_us"), "microseconds")
    lines += panel(rows, 48, 380, 520, 250, "IOPS", ("read_iops", "write_iops"), "operations/s")
    lines += panel(rows, 610, 380, 520, 250, "Storage amplification", ("storage_amplification_x",), "x workload bytes")
    legend_x = 850
    for index, variant in enumerate(LABELS):
        x = legend_x + index * 100
        lines.append(f'<rect x="{x}" y="646" width="12" height="12" fill="{COLORS[variant]}"/>')
        lines.append(f'<text x="{x + 17}" y="657" class="legend">{LABELS[variant]}</text>')
    lines.append('<text x="48" y="690" font-size="10" fill="#64748b">B5 telemetry: 16,620 events · 148.47 B/event · B4 protected wrapper: 64.34 s lifecycle wall time</text>')
    lines.append('</svg>')
    return "\n".join(lines) + "\n"


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--input", type=Path, default=Path(__file__).with_name("results_normalized.csv"))
    parser.add_argument("--output", type=Path, default=Path(__file__).with_name("results_chart.svg"))
    args = parser.parse_args()
    args.output.write_text(render(load_rows(args.input)), encoding="utf-8")
    print(f"wrote {args.output}")


if __name__ == "__main__":
    main()
