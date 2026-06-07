#!/usr/bin/env python3
"""
Render a Markdown review document for the sample-gate stage of SOW-0034.

Reads every enriched/<OID>.json file written by classify.py and emits a
single Markdown file with one section per trap: input description,
varbind names, and the LLM-produced classification.

Designed to be skimmed -- the operator just wants to spot wrong
categories, hallucinated placeholders, and marketing tone.
"""

from __future__ import annotations

import argparse
import json
import os
import sys
from collections import Counter
from typing import Any, Dict, List, Optional


def truncate(s: Optional[str], n: int) -> str:
    if not s:
        return ""
    s = " ".join(s.split())
    return s if len(s) <= n else s[: n - 3] + "..."


def main() -> int:
    p = argparse.ArgumentParser()
    p.add_argument("--in-dir", default="output/enriched")
    p.add_argument("--out", default="output/sample-review.md")
    args = p.parse_args()

    records: List[Dict[str, Any]] = []
    for fn in sorted(os.listdir(args.in_dir)):
        if not fn.endswith(".json"):
            continue
        path = os.path.join(args.in_dir, fn)
        record = None
        try:
            with open(path) as f:
                record = json.load(f)
        except Exception as exc:
            print(f"Skipping malformed sample record {path}: {exc}", file=sys.stderr)
        if record is None:
            continue
        records.append(record)

    # Aggregate stats first.
    cat_counter = Counter(r.get("category") or "unknown" for r in records)
    sev_counter = Counter(r.get("severity") or "info" for r in records)
    src_counter = Counter(r.get("enrichment_source") or "?" for r in records)
    desc_lens = [len(r.get("description") or "") for r in records]

    # Group by category so reviewers can scan one bucket at a time.
    by_cat: Dict[str, List[Dict[str, Any]]] = {}
    for r in records:
        by_cat.setdefault(r.get("category") or "unknown", []).append(r)

    lines: List[str] = []
    lines.append("# Sample review - SOW-0034 LLM enrichment (200-trap gate)")
    lines.append("")
    lines.append(f"Total records reviewed: **{len(records)}**")
    lines.append("")
    lines.append("## Category distribution")
    lines.append("")
    for cat, n in sorted(cat_counter.items(), key=lambda kv: -kv[1]):
        lines.append(f"- `{cat}` -- {n}")
    lines.append("")
    lines.append("## Severity distribution")
    lines.append("")
    for sev, n in sorted(sev_counter.items(), key=lambda kv: -kv[1]):
        lines.append(f"- `{sev}` -- {n}")
    lines.append("")
    lines.append("## Enrichment source")
    lines.append("")
    for src, n in sorted(src_counter.items(), key=lambda kv: -kv[1]):
        lines.append(f"- `{src}` -- {n}")
    lines.append("")
    if desc_lens:
        avg = sum(desc_lens) / len(desc_lens)
        lines.append(f"Description length: min={min(desc_lens)}, max={max(desc_lens)}, mean={avg:.1f}")
        lines.append("")

    # Per-category sections.
    lines.append("## Records grouped by category")
    lines.append("")
    for cat in sorted(by_cat.keys()):
        recs = by_cat[cat]
        lines.append(f"### Category: `{cat}` ({len(recs)} records)")
        lines.append("")
        for r in sorted(recs, key=lambda x: (x.get("mib") or "", x.get("name") or "")):
            oid = r.get("oid")
            name = r.get("name") or "?"
            mib = r.get("mib") or "?"
            sev = r.get("severity")
            src = r.get("enrichment_source")
            input_desc = truncate(r.get("trap_description"), 220)
            vb_names = ", ".join(v.get("name") or "?" for v in (r.get("varbinds") or []))
            llm_desc = r.get("description") or ""
            lines.append(f"#### `{name}` ({oid}) -- `{mib}`")
            lines.append("")
            lines.append(f"- severity: **{sev}**")
            lines.append(f"- source: `{src}`")
            if vb_names:
                lines.append(f"- varbinds: {vb_names}")
            else:
                lines.append("- varbinds: (none)")
            if input_desc:
                lines.append("")
                lines.append("**Input description:**")
                lines.append("")
                lines.append(f"> {input_desc}")
            lines.append("")
            lines.append("**LLM description template:**")
            lines.append("")
            lines.append(f"    {llm_desc}")
            lines.append("")
        lines.append("")

    with open(args.out, "w") as f:
        f.write("\n".join(lines))
    print(f"Wrote {args.out} with {len(records)} records.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
