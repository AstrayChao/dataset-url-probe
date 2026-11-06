#!/usr/bin/env python3
"""从《数据集URL域名统计.xlsx》提取域名清单，生成探活程序使用的 CSV。

用法:
    pip install openpyxl
    python tools/extract_domains.py data/source/数据集URL域名统计.xlsx data/domains.csv

输出列: domain,dataset_count,url_occurrences,distinct_urls,usage_records
"""

from __future__ import annotations

import csv
import sys
from pathlib import Path

import openpyxl


def find_header(rows: list[tuple]) -> int:
    """定位表头行（第一列为“域名”）。"""
    for idx, row in enumerate(rows):
        if row and str(row[0]).strip() in {"域名", "domain", "Domain"}:
            return idx
    raise SystemExit("未找到表头行（第一列为“域名”）")


def main() -> None:
    if len(sys.argv) != 3:
        raise SystemExit(__doc__)

    src = Path(sys.argv[1])
    dst = Path(sys.argv[2])

    wb = openpyxl.load_workbook(src, read_only=True, data_only=True)
    ws = wb.worksheets[0]
    rows = list(ws.iter_rows(values_only=True))
    header_idx = find_header(rows)
    header = [str(c).strip() if c is not None else "" for c in rows[header_idx]]
    print(f"header: {header}")

    col = {name: i for i, name in enumerate(header)}
    domain_i = col.get("域名", 0)
    count_i = col.get("数据集数", 1)
    url_i = col.get("URL 出现次数", 2)
    distinct_i = col.get("不同 URL 数", 3)
    usage_i = col.get("使用记录数", 4)

    seen: set[str] = set()
    out_rows: list[tuple[str, int, int, int, int]] = []
    for row in rows[header_idx + 1 :]:
        if not row:
            continue
        domain = str(row[domain_i] or "").strip()
        if not domain or domain in seen:
            continue
        seen.add(domain)
        out_rows.append(
            (
                domain,
                int(row[count_i] or 0),
                int(row[url_i] or 0),
                int(row[distinct_i] or 0),
                int(row[usage_i] or 0),
            )
        )

    dst.parent.mkdir(parents=True, exist_ok=True)
    with dst.open("w", encoding="utf-8", newline="") as fh:
        writer = csv.writer(fh)
        writer.writerow(["domain", "dataset_count", "url_occurrences", "distinct_urls", "usage_records"])
        writer.writerows(out_rows)

    print(f"wrote {len(out_rows)} domains -> {dst}")


if __name__ == "__main__":
    main()
