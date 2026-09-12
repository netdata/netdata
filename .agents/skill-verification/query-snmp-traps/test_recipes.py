#!/usr/bin/env python3
"""Run the documented jq reductions using synthetic, offline Function responses."""

import json
import os
import pathlib
import re
import shlex
import subprocess
import tempfile
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[3]
GUIDES = ROOT / 'docs/netdata-ai/skills/query-snmp-traps/how-tos'


def programs(filename):
    text = (GUIDES / filename).read_text()
    result = []
    for block in re.findall(r'```bash\n(.*?)```', text, re.S):
        tokens = shlex.split(block)
        for index, token in enumerate(tokens):
            if token != 'jq':
                continue
            for end in range(index + 1, len(tokens)):
                expression = tokens[end]
                if '.columns as $c' in expression or '.facets[]?' in expression:
                    options = tokens[index + 1:end]
                    # These are local reductions only; never execute request/setup blocks.
                    args = ['jq'] + [flag for flag in ('-e', '-s') if flag in options]
                    if '--arg' in options:
                        args += ['--arg', 'field', 'TRAP_VAR_IFINDEX']
                    result.append((args, expression))
                    break
                if tokens[end] == 'jq':
                    break
    return result


def run(program, response, requested=None):
    args, expression = program
    args = args + ['--argjson', 'requested', json.dumps(requested), expression]
    return subprocess.run(args, input=response, text=True, capture_output=True, check=False)


def fixture(partial=False, empty=False):
    fields = ['TRAP_SEVERITY', 'TRAP_SUPPRESSED_COUNT', 'TRAP_SUPPRESSED_FINGERPRINTS',
              'TRAP_REPORT_PERIOD_SEC', 'TRAP_VAR_IFINDEX', 'TRAP_JSON']
    return json.dumps({
        'status': 200, 'partial': partial,
        'columns': {field: {'index': index} for index, field in enumerate(fields)},
        'data': [] if empty else [['crit', '9', '2', '10', '29', '{"example":true}']],
        'facets': [
            {'id': name, 'options': [] if empty else [{'id': value, 'count': 7}]}
            for name, value in [('TRAP_SOURCE_IP', '192.0.2.1'), ('_HOSTNAME', 'example.com')]
        ],
    })


class RecipeTests(unittest.TestCase):
    def test_function_envelopes(self):
        recipes = {
            'inspect-dedup-summary-entries.md': 1,
            'recent-security-traps-from-device.md': 1,
            'search-varbind-value-in-trap-json.md': 2,
            'top-trap-senders-last-hour.md': 2,
        }
        for filename, count in recipes.items():
            extracted = programs(filename)
            self.assertEqual(len(extracted), count, filename)
            for index, program in enumerate(extracted):
                for response in ['', '{', '{"status":503,"errorMessage":"SYNTHETIC_PRIVATE_DETAIL"}',
                                 '{"status":200}', '{"status":200,"columns":{},"data":null}']:
                    with self.subTest(recipe=filename, reduction=index, invalid=response):
                        completed = run(program, response)
                        self.assertNotEqual(completed.returncode, 0)
                        self.assertEqual(completed.stdout, '')
                        self.assertNotIn('SYNTHETIC_PRIVATE_DETAIL', completed.stderr)
                for empty in (False, True):
                    baseline = run(program, fixture(empty=empty))
                    partial = run(program, fixture(partial=True, empty=empty))
                    with self.subTest(recipe=filename, reduction=index, empty=empty):
                        self.assertEqual(baseline.returncode, 0, baseline.stderr)
                        self.assertEqual(partial.returncode, baseline.returncode, partial.stderr)
                        self.assertEqual(partial.stdout, baseline.stdout)
                        result = json.loads(baseline.stdout)
                        if filename == 'inspect-dedup-summary-entries.md':
                            self.assertEqual(result['entries'], 0 if empty else 1)
                            self.assertEqual(result['suppressed_total'], 0 if empty else 9)
                        else:
                            self.assertEqual(len(result), 0 if empty else 1)
                        if not empty and filename == 'recent-security-traps-from-device.md':
                            self.assertEqual(result, [{'severity': 'crit', 'returned_rows': 1}])
                        if not empty and filename == 'search-varbind-value-in-trap-json.md':
                            if index == 0:
                                self.assertEqual(result[0]['varbind_value'], '29')
                            else:
                                self.assertEqual(result[0]['varbinds'], {'example': True})
                        if not empty and filename == 'top-trap-senders-last-hour.md':
                            self.assertEqual(result[0]['count'], 7)

    def test_optional_severity_selection(self):
        extracted = programs('filter-by-severity-across-fleet.md')
        self.assertEqual(len(extracted), 1)
        response = json.dumps({'status': 200, 'data': [], 'facets': [{
            'id': 'TRAP_SEVERITY', 'options': [{'id': 'crit', 'count': 7}, {'id': 'info', 'count': 3}]
        }]})
        for selected in (None, [], ['crit']):
            with self.subTest(selected=selected):
                completed = run(extracted[0], response, selected)
                self.assertEqual(completed.returncode, 0, completed.stderr)
                expected = [{'severity': 'crit', 'count': 7}]
                if not selected:
                    expected.append({'severity': 'info', 'count': 3})
                self.assertEqual(json.loads(completed.stdout), expected)

    def test_top_sender_partial_warning(self):
        text = (GUIDES / 'top-trap-senders-last-hour.md').read_text()
        warning = re.search(r"   if jq -e '\.partial == true'.*?   fi", text, re.S).group()
        with tempfile.TemporaryDirectory() as directory:
            path = pathlib.Path(directory) / 'top-senders.json'
            for partial in (False, True):
                path.write_text(fixture(partial=partial))
                completed = subprocess.run(['bash', '-c', warning], env={'TRAP_QUERY_DIR': directory,
                                            'PATH': os.environ['PATH']},
                                           capture_output=True, text=True, check=False)
                self.assertEqual('WARN: partial query response' in completed.stderr, partial)


if __name__ == '__main__':
    unittest.main()
