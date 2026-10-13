#!/usr/bin/env python3
"""Stock health alert metadata: the ``alerts`` catalogue export and the metadata ``alerts`` rows that mirror it.

The mirror rules live in `.agents/skills/collectors-metadata-yaml/alerts-and-meta.md`; only name, link and metric are
checked here. The loaders skip a metadata file that fails schema validation; `gen_integrations.py` fails on it first.
"""

import json
import re
import sys
import tempfile
import unittest
from pathlib import Path

INTEGRATIONS_DIR = Path(__file__).resolve().parents[1]
REPO_ROOT = INTEGRATIONS_DIR.parent
sys.path.insert(0, str(INTEGRATIONS_DIR))

from gen_docs_integrations import read_integrations_js  # noqa: E402
from gen_integrations import alert_owners, load_collectors, load_device, load_flows, render_integrations  # noqa: E402
from health_alerts import (  # noqa: E402
    HEALTH_CONFIG_DIR,
    build_alert_index,
    check_alert_rows,
    conf_link,
    load_stock_rules,
    parse_health_config,
)

RULE_LINE = re.compile(r'^\s*(alarm|template)\s*:\s*(\S+)', re.IGNORECASE)

# Alarm rows whose `metric` is the context of the chart the alarm's `on:` names, not that chart id. The Go IPMI
# collector keeps the C plugin's chart id `ipmi.events` under its own context. Remove a pair when it is no longer
# needed; the test fails if a listed pair stops being an exception.
METRIC_ELSEWHERE = frozenset({('ipmi_events', 'ipmi_go.sel')})


def stock_rule_lines():
    lines = []
    for path in sorted(HEALTH_CONFIG_DIR.glob('*.conf')):
        for line in path.read_text(encoding='utf-8', errors='replace').split('\n'):
            match = RULE_LINE.match(line)
            if match:
                lines.append((path.relative_to(REPO_ROOT).as_posix(), match.group(2)))
    return lines


def rows_of(items):
    return [
        (f"{Path(item['_src_path']).relative_to(REPO_ROOT)} ({item['meta']['module_name']})", row)
        for item in items
        for row in item.get('alerts') or []
    ]


class ParseHealthConfigTest(unittest.TestCase):
    def test_reads_rules_like_the_agent(self):
        text = '''
# template: commented_out
   on: ignored.before.any.rule

 template: first
       on: app.requests
    CLASS: Errors
     type: "Web Server"
component: 'nginx'
  summary: requests   are \\
           failing
     info: the "upstream" is down
    units:
  nothing to see here

    alarm: second
       on: system.cpu
     info: a line ending in a backslash \\
'''

        self.assertEqual(parse_health_config(text, 'src/health/health.d/x.conf'), [
            {
                'name': 'first',
                'template': True,
                'source': 'src/health/health.d/x.conf',
                'on': 'app.requests',
                'class': 'Errors',
                'type': 'Web Server',
                'component': 'nginx',
                'summary': 'requests are failing',
                'info': 'the  upstream  is down',
            },
            {
                'name': 'second',
                'template': False,
                'source': 'src/health/health.d/x.conf',
                'on': 'system.cpu',
                'info': 'a line ending in a backslash',
            },
        ])


class BuildAlertIndexTest(unittest.TestCase):
    def test_one_entry_per_name_with_every_target_and_its_owners(self):
        rules = [
            {'name': 'ram_in_use', 'template': False, 'source': 'a.conf', 'on': 'system.ram', 'class': 'Utilization',
             'summary': 'RAM'},
            {'name': 'disk', 'template': True, 'source': 'b.conf', 'on': 'disk.space'},
            {'name': 'ram_in_use', 'template': False, 'source': 'c.conf', 'on': 'mem.available', 'class': 'Other'},
            {'name': 'ram_in_use', 'template': False, 'source': 'c.conf', 'on': 'system.ram'},
        ]

        self.assertEqual(build_alert_index(rules, {'ram_in_use': {'proc-b', 'proc-a'}, 'gone': {'x'}}), {
            'disk': {'template': True, 'on': ['disk.space'], 'source': 'b.conf', 'integrations': []},
            'ram_in_use': {
                'template': False,
                'on': ['system.ram', 'mem.available'],
                'class': 'Utilization',
                'summary': 'RAM',
                'source': 'a.conf',
                'integrations': ['proc-a', 'proc-b'],
            },
        })

    def test_owners_are_the_integrations_whose_metadata_lists_the_alert(self):
        def item(module, *names):
            return {
                'meta': {'plugin_name': 'p', 'module_name': module, 'monitored_instance': {'name': module.upper()}},
                'alerts': [{'name': name} for name in names],
            }

        owners = alert_owners([item('m1', 'a', 'b'), item('m2', 'b')], [item('m3')])

        self.assertEqual({name: sorted(ids) for name, ids in owners.items()}, {
            'a': ['p-m1-M1'],
            'b': ['p-m1-M1', 'p-m2-M2'],
        })


class CheckAlertRowsTest(unittest.TestCase):
    RULES = [
        {'name': 'tmpl', 'template': True, 'source': 'src/health/health.d/t.conf', 'on': 'app.ctx'},
        {'name': 'chart_alarm', 'template': False, 'source': 'src/health/health.d/a.conf', 'on': 'ipmi.events'},
    ]

    def test_reports_unknown_names_wrong_links_and_wrong_metrics(self):
        alarm_link = conf_link('src/health/health.d/a.conf')
        template_link = conf_link('src/health/health.d/t.conf')
        rows = [
            ('ok template', {'name': 'tmpl', 'metric': 'app.ctx', 'link': template_link}),
            ('ok alarm', {'name': 'chart_alarm', 'metric': 'ipmi.events', 'link': alarm_link}),
            ('ok alarm documented elsewhere', {'name': 'chart_alarm', 'metric': 'ipmi_go.sel', 'link': alarm_link}),
            ('unknown', {'name': 'gone', 'metric': 'app.ctx', 'link': template_link}),
            ('wrong link', {'name': 'tmpl', 'metric': 'app.ctx', 'link': alarm_link}),
            ('wrong template metric', {'name': 'tmpl', 'metric': 'app.other', 'link': template_link}),
            ('wrong alarm metric', {'name': 'chart_alarm', 'metric': 'ipv4.events', 'link': alarm_link}),
        ]

        problems = check_alert_rows(rows, self.RULES, {('chart_alarm', 'ipmi_go.sel')})

        self.assertEqual([problem.split(':')[0] for problem in problems],
                         ['unknown', 'wrong link', 'wrong template metric', 'wrong alarm metric'])


class CatalogueExportTest(unittest.TestCase):
    def test_page_generators_still_read_categories_and_integrations(self):
        categories = [{'id': 'data-collection', 'name': 'Data Collection', 'children': []}]
        integrations = [{'id': 'p-m-M', 'meta': {}}]
        alerts = {'disk': {'template': True, 'on': ['disk.space'], 'source': 'b.conf', 'integrations': []}}

        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory) / 'integrations.js'
            render_integrations(categories, integrations, alerts, output)
            text = output.read_text()
            exported = json.loads(text.split('export const alerts = ', 1)[1])

            self.assertEqual((read_integrations_js(str(output)), exported), ((categories, integrations), alerts))


class StockHealthConfigTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.rules = load_stock_rules()

    def test_every_rule_line_becomes_one_rule_with_a_target(self):
        self.assertEqual([(rule['source'], rule['name']) for rule in self.rules], stock_rule_lines())
        self.assertEqual([rule['name'] for rule in self.rules if not rule.get('on')], [])

    def test_the_index_has_one_entry_per_stock_alert_name(self):
        index = build_alert_index(self.rules)

        self.assertEqual(sorted(index), sorted({name for _, name in stock_rule_lines()}))


class MetadataAlertRowsTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.rows = rows_of(load_collectors()) + rows_of(load_flows()) + rows_of(load_device())
        cls.rules = load_stock_rules()

    def test_every_metadata_alert_row_matches_a_stock_alert(self):
        problems = check_alert_rows(self.rows, self.rules, METRIC_ELSEWHERE)

        self.assertEqual(problems, [], '\n'.join(problems))

    def test_every_metric_exception_is_still_needed(self):
        needed = {pair for pair in METRIC_ELSEWHERE if check_alert_rows(
            [(where, row) for where, row in self.rows if (row.get('name'), row.get('metric')) == pair], self.rules)}

        self.assertEqual(needed, METRIC_ELSEWHERE)


if __name__ == '__main__':
    unittest.main()
