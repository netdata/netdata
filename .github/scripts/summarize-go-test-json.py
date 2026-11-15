#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later

"""Render completed Go package and top-level test timings from a raw JSON log."""

import argparse
import json
import math
from collections import Counter
from pathlib import Path


def markdown_cell(value):
    return (value.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;")
            .replace("|", "&#124;").replace("`", "&#96;").replace("\n", " ").replace("\r", " "))


def summarize(lines, label):
    packages = []
    tests = []
    counts = Counter()
    diagnostics = 0
    for line in lines:
        if not line.strip():
            continue
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            diagnostics += 1
            continue
        if not isinstance(event, dict):
            continue
        action = event.get("Action")
        package = event.get("Package")
        if action not in ("pass", "fail", "skip") or not isinstance(package, str) or not package:
            continue
        test = event.get("Test", "")
        if not test:
            counts[action] += 1
        elif not isinstance(test, str) or "/" in test:
            continue
        elapsed = event.get("Elapsed")
        if (not isinstance(elapsed, (int, float)) or isinstance(elapsed, bool)
                or not math.isfinite(elapsed) or elapsed < 0):
            continue
        if test:
            tests.append((elapsed, package + "/" + test, action))
        else:
            packages.append((elapsed, package, action))

    output = [
        f"## Go test timings: {markdown_cell(label)}",
        "",
        f"Package results: {counts['pass']} passed, {counts['fail']} failed, {counts['skip']} skipped.",
        "",
        "Package times overlap and must not be summed. Test times include subtests.",
        "Raw JSON and diagnostics are retained in `go-test.jsonl`.",
        "",
    ]
    if diagnostics:
        output.extend([f"Non-JSON diagnostic lines in the raw log: {diagnostics}.", ""])
    for heading, name, results in (("Slowest packages", "Package", packages),
                                   ("Slowest top-level tests", "Test", tests)):
        output.extend([f"### {heading}", ""])
        if not results:
            output.extend(["No completed timings were recorded.", ""])
            continue
        output.extend([f"| Seconds | Result | {name} |", "| ---: | --- | --- |"])
        for elapsed, identity, action in sorted(results, key=lambda row: (-row[0], row[1]))[:20]:
            output.append(f"| {elapsed:.3f} | {action} | {markdown_cell(identity)} |")
        output.append("")
    return "\n".join(output)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("log", type=Path)
    parser.add_argument("--label", required=True, help="Module or tagged suite shown in the report")
    args = parser.parse_args()
    with args.log.open(encoding="utf-8", errors="replace") as stream:
        print(summarize(stream, args.label))


if __name__ == "__main__":
    main()
