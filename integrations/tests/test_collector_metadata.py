#!/usr/bin/env python3
"""Mechanically checkable content rules for collector metadata.yaml.

The rules themselves live in `.agents/skills/project-collector-metadata/`. Only what a script can decide without
judgment is checked here: claims the page makes by omission, and Markdown that breaks the Learn build.
"""

import re
import sys
import unittest
from pathlib import Path

INTEGRATIONS_DIR = Path(__file__).resolve().parents[1]
REPO_ROOT = INTEGRATIONS_DIR.parent
sys.path.insert(0, str(INTEGRATIONS_DIR))

import _common  # noqa: E402

GO_D_COLLECTORS = REPO_ROOT / 'src' / 'go' / 'plugin' / 'go.d' / 'collector'
SD_RULES_DIR = REPO_ROOT / 'src' / 'go' / 'plugin' / 'go.d' / 'config' / 'go.d' / 'sd'

# Service-discovery rule ids that create no collector job (filters and pass-through rules).
SD_NON_COLLECTOR_RULE_IDS = frozenset({'skip', 'passthrough'})

# Collectors named by a service-discovery rule that ship no metadata.yaml, so they have no integration page at all.
# Remove an entry when the collector gains its metadata; the test fails if a listed collector no longer needs it.
SD_COLLECTORS_WITHOUT_METADATA = frozenset({'bind'})

# Collectors covered by a service-discovery rule whose `auto_detection` field is still empty. Their pages render the
# placeholder "This integration doesn't support auto-detection.", which is false for them. Filling the field is fleet
# work tracked separately; remove a module here when its field is written. The test fails if a listed module no longer
# needs the exemption, so the list can only shrink.
SD_AUTO_DETECTION_BACKLOG = frozenset({
    'cockroachdb', 'coredns', 'couchbase', 'couchdb', 'dnsdist', 'dnsmasq', 'fluentd', 'hdfs', 'k8s_kubelet',
    'k8s_kubeproxy', 'logstash', 'mongodb', 'ntpd', 'openvpn', 'pgbouncer', 'pika', 'powerdns', 'powerdns_recursor',
    'proxysql', 'rabbitmq', 'supervisord', 'tengine', 'traefik', 'unbound', 'upsd', 'vernemq',
})

# Fields whose text is rendered as Markdown prose on the page (not as code, a link, an identifier, or a table cell
# escaped by the generator). Only these are checked for MDX-unsafe constructs.
PROSE_KEYS = frozenset({
    'metrics_description', 'method_description', 'description', 'detailed_description', 'info',
    'error', 'when', 'cause', 'fix',
})

# HTML tags Learn's ingest escapes or renders; everything else in angle brackets outside code is treated as JSX.
ALLOWED_HTML_TAGS = frozenset({'details', 'summary', 'br', 'code', 'b', 'strong', 'em', 'i', 'sub', 'sup', 'a', 'img',
                               'p', 'ul', 'li', 'kbd'})

ADMONITION_FENCE = re.compile(r'^\s*:::')
CODE_FENCE = re.compile(r'^\s*```')
TABLE_ROW = re.compile(r'^\s*\|.*\|\s*$')
TABLE_SEPARATOR = re.compile(r'^\s*\|[\s:|-]+\|\s*$')
ANGLE_TAG = re.compile(r'</?([A-Za-z][A-Za-z0-9_.-]*)>')
ANGLE_DIGIT = re.compile(r'<\d')
INLINE_CODE = re.compile(r'`[^`\n]*`')
TEMPLATE_MODULE = re.compile(r'^\s*module:\s*(\S+)', re.M)


def sd_covered_modules():
    """Module name -> set of sd rule files naming it. The rule id is the module unless the job template sets one."""
    covered = {}
    for path in sorted(SD_RULES_DIR.glob('*.conf')):
        conf = _common.load_yaml(path)
        for rule in (conf or {}).get('services') or []:
            if rule['id'] in SD_NON_COLLECTOR_RULE_IDS:
                continue
            match = TEMPLATE_MODULE.search(rule.get('config_template') or '')
            module = match.group(1) if match else rule['id']
            covered.setdefault(module, set()).add(path.name)
    return covered


def auto_detection_text(metadata):
    parts = []
    for module in metadata.get('modules') or []:
        behavior = (module.get('overview') or {}).get('default_behavior') or {}
        parts.append(((behavior.get('auto_detection') or {}).get('description') or '').strip())
    return ' '.join(parts).strip()


def prose_fields(node, path=''):
    """Yield (path, text) for every prose field of a metadata document."""
    if isinstance(node, dict):
        for key, value in node.items():
            child = f'{path}.{key}'
            if key in PROSE_KEYS and isinstance(value, str):
                yield child, value
            else:
                yield from prose_fields(value, child)
    elif isinstance(node, list):
        for index, value in enumerate(node):
            yield from prose_fields(value, f'{path}[{index}]')


def without_code(text):
    """Drop fenced blocks and inline code spans; what remains is what MDX parses as markup."""
    kept = []
    in_fence = False
    for line in text.splitlines():
        if CODE_FENCE.match(line):
            in_fence = not in_fence
            continue
        if not in_fence:
            kept.append(INLINE_CODE.sub('', line))
    return '\n'.join(kept)


def markdown_problems(text):
    lines = text.splitlines()
    problems = []
    if sum(1 for line in lines if ADMONITION_FENCE.match(line)) % 2:
        problems.append('unbalanced ::: admonition fence')
    if sum(1 for line in lines if CODE_FENCE.match(line)) % 2:
        problems.append('unbalanced ``` code fence')
    index = 0
    while index < len(lines):
        if not TABLE_ROW.match(lines[index]):
            index += 1
            continue
        block = []
        while index < len(lines) and TABLE_ROW.match(lines[index]):
            block.append(lines[index])
            index += 1
        if len(block) < 2 or not TABLE_SEPARATOR.match(block[1]):
            problems.append('table without a header separator row')
        elif len({row.count('|') for row in block}) > 1:
            problems.append('table rows with different cell counts')
    plain = without_code(text)
    for match in ANGLE_TAG.finditer(plain):
        if match.group(1).lower() not in ALLOWED_HTML_TAGS:
            problems.append(f'angle-bracket placeholder outside code: {match.group(0)}')
    for match in ANGLE_DIGIT.finditer(plain):
        problems.append(f'"<" directly before a digit outside code: {plain[match.start():match.end() + 8]!r}')
    return problems


class ServiceDiscoveryClaimsTest(unittest.TestCase):
    """A collector named by a service-discovery rule is auto-detected; its page must not say otherwise."""

    def test_sd_covered_collectors_document_auto_detection(self):
        covered = sd_covered_modules()
        self.assertTrue(covered, 'no service-discovery rules found')

        missing_metadata = set()
        empty = set()
        for module in covered:
            metadata_path = GO_D_COLLECTORS / module / 'metadata.yaml'
            if not metadata_path.is_file():
                missing_metadata.add(module)
                continue
            metadata = _common.load_yaml(metadata_path)
            self.assertTrue(metadata, f'{metadata_path} failed to load')
            if not auto_detection_text(metadata):
                empty.add(module)

        self.assertEqual(
            missing_metadata - SD_COLLECTORS_WITHOUT_METADATA, set(),
            'collectors named by a service-discovery rule without a metadata.yaml (no integration page)')
        self.assertEqual(
            SD_COLLECTORS_WITHOUT_METADATA - missing_metadata, set(),
            'SD_COLLECTORS_WITHOUT_METADATA lists a collector that now has metadata.yaml; remove it')

        self.assertEqual(
            empty - SD_AUTO_DETECTION_BACKLOG, set(),
            'service-discovery covers these collectors but their auto_detection is empty, so the page claims they are '
            'not auto-detected; describe the discoverer and the rule file (see '
            '.agents/skills/project-collector-metadata/overview.md, section 6)')
        self.assertEqual(
            SD_AUTO_DETECTION_BACKLOG - empty, set(),
            'SD_AUTO_DETECTION_BACKLOG lists a collector whose auto_detection is now filled; remove it')


class MarkdownSafetyTest(unittest.TestCase):
    """Prose fields travel through the generator into MDX; these constructs break the Learn build silently here."""

    def test_prose_fields_are_mdx_safe(self):
        failures = []
        for _repo, path in _common.get_collector_metadata_entries():
            metadata = _common.load_yaml(path)
            if not metadata:
                continue
            for field_path, text in prose_fields(metadata):
                for problem in markdown_problems(text):
                    failures.append(f'{path.relative_to(REPO_ROOT)}{field_path}: {problem}')
        self.assertEqual(failures, [], '\n'.join(failures))


if __name__ == '__main__':
    unittest.main()
