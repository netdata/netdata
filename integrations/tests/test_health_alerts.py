#!/usr/bin/env python3
"""Stock health alert metadata: the ``alerts`` catalogue export and the metadata ``alerts`` rows that mirror it.

The mirror rules live in `.agents/skills/collectors-metadata-yaml/alerts-and-meta.md`; only name, link and template
metric are checked here.
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
                'info': 'the "upstream" is down',
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

    def test_reports_unknown_names_wrong_links_and_wrong_template_metrics(self):
        rows = [
            ('ok template', {'name': 'tmpl', 'metric': 'app.ctx', 'link': conf_link('src/health/health.d/t.conf')}),
            ('ok alarm on another context', {'name': 'chart_alarm', 'metric': 'ipmi_go.sel',
                                             'link': conf_link('src/health/health.d/a.conf')}),
            ('unknown', {'name': 'gone', 'metric': 'app.ctx', 'link': conf_link('src/health/health.d/t.conf')}),
            ('wrong link', {'name': 'tmpl', 'metric': 'app.ctx', 'link': conf_link('src/health/health.d/a.conf')}),
            ('wrong metric', {'name': 'tmpl', 'metric': 'app.other', 'link': conf_link('src/health/health.d/t.conf')}),
        ]

        problems = check_alert_rows(rows, self.RULES)

        self.assertEqual([problem.split(':')[0] for problem in problems], ['unknown', 'wrong link', 'wrong metric'])


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
        expected = []
        for path in sorted(HEALTH_CONFIG_DIR.glob('*.conf')):
            for line in path.read_text(errors='replace').splitlines():
                match = RULE_LINE.match(line)
                if match:
                    expected.append((path.relative_to(REPO_ROOT).as_posix(), match.group(2)))

        self.assertEqual([(rule['source'], rule['name']) for rule in self.rules], expected)
        self.assertEqual([rule['name'] for rule in self.rules if not rule.get('on')], [])

    def test_the_index_covers_every_stock_alert_name(self):
        index = build_alert_index(self.rules)

        self.assertEqual(sorted(index), sorted({rule['name'] for rule in self.rules}))


class MetadataAlertRowsTest(unittest.TestCase):
    def test_every_metadata_alert_row_matches_a_stock_alert(self):
        rows = rows_of(load_collectors()) + rows_of(load_flows()) + rows_of(load_device())

        self.assertEqual(check_alert_rows(rows, load_stock_rules()), [])


if __name__ == '__main__':
    unittest.main()
