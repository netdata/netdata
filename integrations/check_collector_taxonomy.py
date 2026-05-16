#!/usr/bin/env python3

import argparse
import re
import subprocess
import sys
from pathlib import Path

from ruamel.yaml import YAML, YAMLError

from gen_taxonomy import FATAL, Finding, build_taxonomy, relpath
from _common import REPO_PATH

HUNK_RE = re.compile(r'^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@')


def run_git(*args):
    return subprocess.check_output(['git', '-C', str(REPO_PATH), *args], text=True)


def metadata_metrics_spans(path):
    text = path.read_text()
    lines = text.splitlines()
    yaml = YAML(typ='rt')
    try:
        data = yaml.load(text)
    except YAMLError:
        return []

    spans = []
    modules = data.get('modules', []) if isinstance(data, dict) else []
    for module_index, module in enumerate(modules):
        if not isinstance(module, dict) or 'metrics' not in module:
            continue
        try:
            start = module.lc.key('metrics')[0] + 1
        except (AttributeError, KeyError, TypeError):
            continue

        next_lines = []
        for key in module:
            if key == 'metrics':
                continue
            try:
                key_line = module.lc.key(key)[0] + 1
            except (AttributeError, KeyError, TypeError):
                continue
            if key_line > start:
                next_lines.append(key_line)

        if next_lines:
            end = min(next_lines) - 1
        else:
            next_module_line = None
            try:
                if module_index + 1 < len(modules):
                    next_module_line = modules.lc.item(module_index + 1)[0] + 1
            except (AttributeError, KeyError, TypeError):
                next_module_line = None
            end = (next_module_line - 1) if next_module_line else len(lines)

        spans.append((start, end))
    return spans


def range_intersects_spans(start, length, spans):
    if length == 0:
        changed_start = start
        changed_end = start
    else:
        changed_start = start
        changed_end = start + length - 1
    return any(changed_start <= span_end and changed_end >= span_start for span_start, span_end in spans)


def metadata_metrics_touched(diff_range, path):
    if not path.exists():
        return True

    diff = run_git('diff', '--unified=0', diff_range, '--', relpath(path))
    if not diff.strip():
        return False

    spans = metadata_metrics_spans(path)
    if not spans:
        return True

    for line in diff.splitlines():
        match = HUNK_RE.match(line)
        if not match:
            continue
        start = int(match.group(1))
        length = int(match.group(2) or '1')
        if range_intersects_spans(start, length, spans):
            return True
    return False


def touched_collectors(diff_range):
    output = run_git('diff', '--name-status', diff_range)
    touched = set()
    for line in output.splitlines():
        if not line.strip():
            continue
        fields = line.split('\t')
        status = fields[0]
        path = REPO_PATH / fields[-1]
        name = path.name

        if name == 'taxonomy.yaml':
            touched.add(path.parent)
        elif name == 'metadata.yaml':
            if status.startswith(('A', 'D')):
                touched.add(path.parent)
            elif metadata_metrics_touched(diff_range, path):
                touched.add(path.parent)
    return sorted(touched)


def check_touched_coverage(diff_range):
    findings = []
    for collector_dir in touched_collectors(diff_range):
        taxonomy_path = collector_dir / 'taxonomy.yaml'
        metadata_path = collector_dir / 'metadata.yaml'
        if not taxonomy_path.exists() and not metadata_path.exists():
            continue
        if not taxonomy_path.exists():
            findings.append(Finding(
                code='TAX030',
                severity=FATAL,
                path=taxonomy_path,
                message='Collector metrics or taxonomy changed, but taxonomy.yaml is missing.',
            ))
    return findings


def main():
    parser = argparse.ArgumentParser(description='Validate collector taxonomy coverage and taxonomy artifact generation.')
    parser.add_argument('--pr-diff', help='Git diff range for touched-collector coverage, for example origin/master...HEAD.')
    args = parser.parse_args()

    findings = []
    if args.pr_diff:
        findings.extend(check_touched_coverage(args.pr_diff))

    _, taxonomy_findings = build_taxonomy()
    findings.extend(taxonomy_findings)

    for finding in findings:
        print(finding.render(), file=sys.stderr)

    return 1 if any(finding.severity == FATAL for finding in findings) else 0


if __name__ == '__main__':
    sys.exit(main())
