#!/usr/bin/env python3

import argparse
import subprocess
import sys

from ruamel.yaml import YAML, YAMLError

from gen_taxonomy import FATAL, Finding, build_taxonomy, dynamic_declarations, module_contexts, relpath
from _common import REPO_PATH, get_collector_metadata_entries


def run_git(*args):
    return subprocess.check_output(['git', '-C', str(REPO_PATH), *args], text=True)


def diff_range_base(diff_range):
    if '...' in diff_range:
        left, right = diff_range.split('...', 1)
        return run_git('merge-base', left, right).strip()
    if '..' in diff_range:
        left, _right = diff_range.split('..', 1)
        return left
    return diff_range


def module_key(module):
    meta = module.get('meta', {}) if isinstance(module, dict) else {}
    return (meta.get('plugin_name'), meta.get('module_name'))


def module_taxonomy_signature(module):
    prefixes, plugins = dynamic_declarations(module)
    return (
        tuple(sorted(module_contexts(module))),
        tuple(sorted(prefixes)),
        tuple(sorted(plugins)),
    )


def metadata_taxonomy_signatures(text):
    yaml = YAML(typ='safe')
    try:
        data = yaml.load(text)
    except YAMLError:
        return None

    modules = data.get('modules', []) if isinstance(data, dict) else []
    return {
        module_key(module): module_taxonomy_signature(module)
        for module in modules
        if isinstance(module, dict)
    }


def metadata_metrics_touched(diff_range, path):
    if not path.exists():
        return True

    diff = run_git('diff', '--unified=0', diff_range, '--', relpath(path))
    if not diff.strip():
        return False

    try:
        old_text = run_git('show', f'{diff_range_base(diff_range)}:{relpath(path)}')
    except subprocess.CalledProcessError:
        return True

    old_signatures = metadata_taxonomy_signatures(old_text)
    new_signatures = metadata_taxonomy_signatures(path.read_text())

    # Unparseable YAML on either side is a structural change we cannot rule
    # out safely -- treat as touched rather than silently passing it through.
    if old_signatures is None or new_signatures is None:
        return True

    return old_signatures != new_signatures


def collector_metadata_paths():
    return {path.resolve() for _, path in get_collector_metadata_entries()}


def touched_collectors(diff_range):
    output = run_git('diff', '--name-status', diff_range)
    collector_metadata = collector_metadata_paths()
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
            if path.resolve() not in collector_metadata:
                continue
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
    parser = argparse.ArgumentParser(
        description='Validate collector taxonomy coverage and taxonomy artifact generation.',
    )
    parser.add_argument(
        '--pr-diff',
        help='Git diff range for touched-collector coverage, for example origin/master...HEAD.',
    )
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
