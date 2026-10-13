#!/usr/bin/env python3
"""Stock health alert metadata: the integration pages' alert rows and the ``alerts`` catalogue export, both derived
from ``src/health/health.d`` and the contexts integrations declare, and the stock alerts' classification vocabulary.

The ownership rule is documented in `.agents/skills/collectors-metadata-yaml/alerts-and-meta.md`. The loaders skip a
metadata file that fails schema validation; `gen_integrations.py` fails on it first.
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

from _common import load_collectors  # noqa: E402
from gen_docs_integrations import read_integrations_js  # noqa: E402
from gen_integrations import alert_subjects, attach_alerts, load_flows, render_integrations  # noqa: E402
from gen_npm_catalog import build_device_modules, load_profiles  # noqa: E402
from health_alerts import (  # noqa: E402
    ALERT_CLASSES,
    ALERT_TYPES,
    HEALTH_CONFIG_DIR,
    PLUGIN_PLATFORMS,
    agent_internal_alerts,
    alert_owners,
    alert_ownership,
    alert_subject,
    build_alert_index,
    check_classification,
    check_listed_names,
    conf_link,
    label_admits,
    label_patterns,
    load_stock_rules,
    parse_health_config,
    rule_admits,
)
from prometheus_profile_docs import project_prometheus_profile_coverage  # noqa: E402

RULE_LINE = re.compile(r'^\s*(alarm|template)\s*:\s*(\S+)', re.IGNORECASE)
HEALTH_REFERENCE = REPO_ROOT / 'src' / 'health' / 'REFERENCE.md'

# Stock alerts no integration owns yet: no integration documents the proc.plugin charts they target (the
# `/proc/sys/fs/file-nr` and `/run/reboot_required` modules have no metadata). Remove an entry when its alert gets an
# owner; the test fails while a listed alert has one.
UNOWNED = frozenset({
    'system_file_descriptors_utilization',
    'system_post_update_reboot_status',
})


def stock_rule_lines():
    lines = []
    for path in sorted(HEALTH_CONFIG_DIR.glob('*.conf')):
        for line in path.read_text(encoding='utf-8', errors='replace').split('\n'):
            match = RULE_LINE.match(line)
            if match:
                lines.append((path.relative_to(REPO_ROOT).as_posix(), match.group(2)))
    return lines


def reference_table_values(heading):
    """First-column values of the first table after a REFERENCE.md heading, without Markdown code quotes."""
    lines = HEALTH_REFERENCE.read_text(encoding='utf-8').split('\n')
    start = lines.index(heading)
    values = []
    for line in lines[start + 1:]:
        if line.startswith('#'):
            break
        if line.startswith('|'):
            values.append(line.split('|')[1].strip().strip('`'))
        elif values:
            break
    return values[2:]  # header row and separator


def corpus_subjects():
    """Every integration that can own an alert, as the generators see it.

    The NPM device entries come from the catalog generator rather than its committed output, which the post-merge
    regeneration refreshes; both integration workflows run the generator before this test.
    """
    collectors = load_collectors()
    project_prometheus_profile_coverage(collectors)
    return alert_subjects(collectors + load_flows() + build_device_modules(load_profiles()))


def rule(name, on, source='src/health/health.d/x.conf', template=True, **fields):
    return {'name': name, 'template': template, 'source': source, 'on': on, **fields}


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
host labels: _os=linux
       os: freebsd
   plugin: dem
   module: *
chart labels: no_label_given
chart labels: device = !wl* *
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
                'host_labels': '_os=freebsd _os=linux',
                'chart_labels': 'device = !wl* * _collect_plugin=dem',
            },
            {
                'name': 'second',
                'template': False,
                'source': 'src/health/health.d/x.conf',
                'on': 'system.cpu',
                'info': 'a line ending in a backslash',
            },
        ])


class LabelPatternsTest(unittest.TestCase):
    def test_splits_words_by_label_in_match_order(self):
        tests = {
            'implied label': {
                'text': '_os=linux freebsd',
                'want': {'_os': [(False, 'linux'), (False, 'freebsd')]},
            },
            'negation and catch-all': {
                'text': 'device=!wl* *',
                'want': {'device': [(True, 'wl*'), (False, '*')]},
            },
            'spaces around equals': {
                'text': 'mount_point = !/dev *',
                'want': {'mount_point': [(True, '/dev'), (False, '*')]},
            },
            'several labels': {
                'text': '_collect_plugin=dem _collect_module=journey',
                'want': {'_collect_plugin': [(False, 'dem')], '_collect_module': [(False, 'journey')]},
            },
        }

        for name, test in tests.items():
            with self.subTest(name):
                self.assertEqual(label_patterns(test['text']), test['want'])

    def test_the_first_matching_pattern_decides(self):
        tests = {
            'excluded by a negation': {'text': 'device=!wl* *', 'value': 'wlan0', 'want': False},
            'admitted by the catch-all': {'text': 'device=!wl* *', 'value': 'eth0', 'want': True},
            'admitted by a later word': {'text': '_os=linux freebsd', 'value': 'freebsd', 'want': True},
            'no pattern matches': {'text': '_os=linux freebsd', 'value': 'windows', 'want': False},
            'wildcard inside a word': {'text': 'sensor_name=*DIMM*_TEMP', 'value': 'P1_DIMM_A_TEMP', 'want': True},
        }

        for name, test in tests.items():
            with self.subTest(name):
                patterns = next(iter(label_patterns(test['text']).values()))
                self.assertEqual(label_admits(patterns, test['value']), test['want'])


class RuleAdmitsTest(unittest.TestCase):
    def test_platform_and_plugin_filters(self):
        tests = {
            'unrestricted alert': {'fields': {}, 'plugin': 'go.d.plugin', 'want': True},
            'Linux alert on a Linux plugin': {
                'fields': {'host_labels': '_os=linux'}, 'plugin': 'proc.plugin', 'want': True},
            'Linux alert on the FreeBSD plugin': {
                'fields': {'host_labels': '_os=linux'}, 'plugin': 'freebsd.plugin', 'want': False},
            'Linux alert on the Windows plugin': {
                'fields': {'host_labels': '_os=linux'}, 'plugin': 'windows.plugin', 'want': False},
            'FreeBSD alert on a Linux plugin': {
                'fields': {'host_labels': '_os=freebsd'}, 'plugin': 'proc.plugin', 'want': False},
            'Linux alert on a plugin that runs anywhere': {
                'fields': {'host_labels': '_os=linux'}, 'plugin': 'go.d.plugin', 'want': True},
            'platform excluded by a negation': {
                'fields': {'host_labels': '_os=!macos *'}, 'plugin': 'macos.plugin', 'want': False},
            'other platform past a negation': {
                'fields': {'host_labels': '_os=!macos *'}, 'plugin': 'freebsd.plugin', 'want': True},
            'Go plugin reports its name without the suffix': {
                'fields': {'chart_labels': '_collect_plugin=dem'}, 'plugin': 'dem.plugin', 'want': True},
            'C plugin reports its full name': {
                'fields': {'chart_labels': '_collect_plugin=proc.plugin'}, 'plugin': 'proc.plugin', 'want': True},
            'another plugin': {
                'fields': {'chart_labels': '_collect_plugin=dem'}, 'plugin': 'go.d.plugin', 'want': False},
            'another module': {
                'fields': {'chart_labels': '_collect_plugin=dem _collect_module=journey'}, 'plugin': 'dem.plugin',
                'module': 'rum', 'want': False},
            'the module': {
                'fields': {'chart_labels': '_collect_plugin=dem _collect_module=journey'}, 'plugin': 'dem.plugin',
                'module': 'journey', 'want': True},
            'instance chart labels do not decide ownership': {
                'fields': {'chart_labels': 'device=!wl* *'}, 'plugin': 'proc.plugin', 'want': True},
            'other host labels do not decide ownership': {
                'fields': {'host_labels': '_hostname=!*'}, 'plugin': 'go.d.plugin', 'want': True},
        }

        for name, test in tests.items():
            with self.subTest(name):
                admitted = rule_admits(rule('a', 'x.y', **test['fields']), test['plugin'], test.get('module', 'm'))
                self.assertEqual(admitted, test['want'])


class AlertOwnershipTest(unittest.TestCase):
    RULES = [
        rule('cpu_usage', 'system.cpu', 'src/health/health.d/cpu.conf', info='CPU usage'),
        rule('cpu_usage', 'system.cpu', 'src/health/health.d/cpu_bsd.conf', info='BSD', host_labels='_os=freebsd'),
        rule('sel_events', 'ipmi.events', 'src/health/health.d/ipmi.conf', template=False, info='SEL events'),
        rule('load_bsd', 'system.load', 'src/health/health.d/load.conf', info='load', host_labels='_os=freebsd'),
        rule('dbengine_full', 'netdata.dbengine', 'src/health/health.d/dbengine.conf', info='disk'),
        rule('cloud_errors', 'cloud.errors', 'src/health/health.d/cloud.conf', info='errors'),
    ]
    SUBJECTS = [
        alert_subject('proc', 'proc.plugin', '/proc/stat', ['system.cpu', 'system.load', 'netdata.dbengine']),
        alert_subject('freebsd', 'freebsd.plugin', 'kern.cp_time', ['system.cpu', 'system.load']),
        alert_subject('ipmi', 'ipmi.plugin', 'ipmi', ['ipmi_go.sel'], ['sel_events']),
        alert_subject('cloud', 'go.d.plugin', 'cloud', [], ['cloud_errors']),
    ]

    @staticmethod
    def row(name, source, metric, info):
        return {'name': name, 'link': conf_link(f'src/health/health.d/{source}'), 'metric': metric, 'info': info}

    def test_rows_follow_contexts_filters_and_listed_names_in_configuration_order(self):
        self.assertEqual(alert_ownership(self.RULES, self.SUBJECTS), {
            'proc': [self.row('cpu_usage', 'cpu.conf', 'system.cpu', 'CPU usage')],
            'freebsd': [
                self.row('cpu_usage', 'cpu.conf', 'system.cpu', 'CPU usage'),
                self.row('load_bsd', 'load.conf', 'system.load', 'load'),
            ],
            'ipmi': [self.row('sel_events', 'ipmi.conf', 'ipmi.events', 'SEL events')],
            'cloud': [self.row('cloud_errors', 'cloud.conf', 'cloud.errors', 'errors')],
        })

    def test_owners_are_the_subjects_whose_rows_name_the_alert(self):
        owners = alert_owners(alert_ownership(self.RULES, self.SUBJECTS))

        self.assertEqual({name: sorted(ids) for name, ids in owners.items()}, {
            'cpu_usage': ['freebsd', 'proc'],
            'load_bsd': ['freebsd'],
            'sel_events': ['ipmi'],
            'cloud_errors': ['cloud'],
        })

    def test_excluded_names_leave_the_rows(self):
        subject = alert_subject('lite', 'freebsd.plugin', 'm', ['system.cpu', 'system.load'], excluded=['load_bsd'])

        self.assertEqual(alert_ownership(self.RULES, [subject]), {
            'lite': [self.row('cpu_usage', 'cpu.conf', 'system.cpu', 'CPU usage')],
        })

    def test_listed_names_keep_only_the_admitted_variants(self):
        rules = [
            rule('events', 'linux.events', 'src/health/health.d/linux.conf', info='Linux', host_labels='_os=linux'),
            rule('events', 'any.events', 'src/health/health.d/any.conf', info='any'),
        ]
        subject = alert_subject('bsd', 'freebsd.plugin', 'm', [], ['events'])

        self.assertEqual(alert_ownership(rules, [subject]), {
            'bsd': [self.row('events', 'any.conf', 'any.events', 'any')],
        })

    def test_agent_alerts_are_those_on_agent_charts_only(self):
        rules = self.RULES + [rule('mixed', 'netdata.x'), rule('mixed', 'app.x'), rule('ml', 'anomaly_detection.y')]

        self.assertEqual(agent_internal_alerts(rules), ['dbengine_full', 'ml'])

    def test_listed_names_must_be_stock_owned_by_name_only_and_admitted(self):
        subjects = [
            alert_subject('ok', 'go.d.plugin', 'cloud', [], ['cloud_errors']),
            alert_subject('unknown', 'go.d.plugin', 'm', [], ['gone']),
            alert_subject('agent', 'go.d.plugin', 'm', [], ['dbengine_full']),
            alert_subject('derivable', 'proc.plugin', 'm', ['system.cpu'], ['cpu_usage']),
            alert_subject('not admitted', 'proc.plugin', 'm', [], ['load_bsd']),
        ]

        problems = [problem.split(': ')[0] for problem in check_listed_names(self.RULES, subjects)]

        self.assertEqual(problems, ['unknown', 'agent', 'derivable', 'not admitted'])

    def test_excluded_names_must_be_derived_and_not_listed(self):
        subjects = [
            alert_subject('ok', 'proc.plugin', 'm', ['system.cpu'], excluded=['cpu_usage']),
            alert_subject('not derived', 'proc.plugin', 'm', ['system.cpu'], excluded=['load_bsd']),
            alert_subject('unknown', 'proc.plugin', 'm', ['system.cpu'], excluded=['gone']),
            alert_subject('listed', 'ipmi.plugin', 'm', [], ['sel_events'], excluded=['sel_events']),
        ]
        not_derived = 'is not derived from a declared context; remove it from excluded_alerts'

        self.assertEqual(check_listed_names(self.RULES, subjects), [
            f"not derived: excluded alert 'load_bsd' {not_derived}",
            f"unknown: excluded alert 'gone' {not_derived}",
            "listed: excluded alert 'sel_events' is also listed",
        ])


class AttachAlertsTest(unittest.TestCase):
    def test_page_rows_and_export_owners_come_from_one_derivation(self):
        def item(module, contexts=(), names=(), profile_contexts=(), excluded=()):
            return {
                'meta': {'plugin_name': 'go.d.plugin', 'module_name': module, 'monitored_instance': {'name': module}},
                'metrics': {
                    'scopes': [{'name': 'global', 'metrics': [{'name': context} for context in contexts]}],
                    'profile_coverage': {'contexts': list(profile_contexts)},
                },
                'alerts': list(names),
                'excluded_alerts': list(excluded),
            }

        rules = [rule('requests', 'app.requests'), rule('listed', 'cloud.x'), rule('profiled', 'prometheus.app.y')]
        collectors = [
            item('app', contexts=['app.requests']),
            item('lite', contexts=['app.requests'], excluded=['requests']),
            item('cloud', names=['listed']),
        ]
        devices = [item('exporter', profile_contexts=['prometheus.app.y'])]

        owners = attach_alerts(rules, collectors, devices)

        self.assertEqual(
            {item['meta']['module_name']: [row['name'] for row in item['alerts']] for item in collectors + devices},
            {'app': ['requests'], 'lite': [], 'cloud': ['listed'], 'exporter': ['profiled']})
        self.assertEqual([item for item in collectors + devices if 'excluded_alerts' in item], [])
        self.assertEqual(owners, {
            'requests': {'go.d.plugin-app-app'},
            'listed': {'go.d.plugin-cloud-cloud'},
            'profiled': {'go.d.plugin-exporter-exporter'},
        })


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


class CheckClassificationTest(unittest.TestCase):
    def test_reports_missing_fields_values_outside_the_vocabulary_and_colliding_components(self):
        def classified(name, **fields):
            return {'name': name, 'source': 'x.conf', **fields}

        rules = [
            classified('ok', **{'class': 'Errors', 'type': 'System', 'component': 'Process'}),
            classified('ok_same_component', **{'class': 'Latency', 'type': 'System', 'component': 'Process'}),
            classified('no_component', **{'class': 'Errors', 'type': 'System'}),
            classified('blank_type', **{'class': 'Errors', 'type': '  ', 'component': 'CPU'}),
            classified('unknown_class', **{'class': 'Error', 'type': 'System', 'component': 'CPU'}),
            classified('unknown_type', **{'class': 'Errors', 'type': 'Other', 'component': 'CPU'}),
            classified('plural', **{'class': 'Errors', 'type': 'System', 'component': 'Processes'}),
            classified('case', **{'class': 'Errors', 'type': 'System', 'component': 'cpu'}),
            classified('policy', **{'class': 'Errors', 'type': 'Network', 'component': 'Policy'}),
            classified('policies', **{'class': 'Errors', 'type': 'Network', 'component': 'Policies'}),
            classified('key', **{'class': 'Errors', 'type': 'Network', 'component': 'Key'}),
            classified('keies', **{'class': 'Errors', 'type': 'Network', 'component': 'Keies'}),
        ]

        problems = [problem.split(': ', 1)[1].split(' is not one of ')[0] for problem in check_classification(rules)]

        self.assertEqual(problems, [
            'no_component: no component',
            'blank_type: no type',
            "unknown_class: class 'Error'",
            "unknown_type: type 'Other'",
            "plural: component 'Processes' collides with 'Process' (x.conf: ok)",
            "case: component 'cpu' collides with 'CPU' (x.conf: blank_type)",
            "policies: component 'Policies' collides with 'Policy' (x.conf: policy)",
        ])


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

    def test_every_stock_alert_is_classified_with_the_vocabulary(self):
        problems = check_classification(self.rules)

        self.assertEqual(problems, [], '\n'.join(problems))

    def test_the_reference_documents_the_vocabulary(self):
        classes = reference_table_values('#### Alert Line `class`')
        types = reference_table_values('#### Alert Line `type`')

        self.assertEqual((classes, types), (list(ALERT_CLASSES), list(ALERT_TYPES)))


class StockAlertOwnershipTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.rules = load_stock_rules()
        cls.subjects = corpus_subjects()

    def test_every_stock_alert_has_an_owner_except_agent_alerts(self):
        owned = set(alert_owners(alert_ownership(self.rules, self.subjects)))
        names = {rule['name'] for rule in self.rules}

        unowned = sorted(names - owned - set(agent_internal_alerts(self.rules)))

        self.assertEqual(unowned, sorted(UNOWNED))

    def test_the_platform_table_names_real_plugins(self):
        plugins = {subject['plugin_name'] for subject in self.subjects}

        self.assertEqual(sorted(set(PLUGIN_PLATFORMS) - plugins), [])

    def test_listed_alert_names_are_stock_and_not_derivable(self):
        problems = check_listed_names(self.rules, self.subjects)

        self.assertEqual(problems, [], '\n'.join(problems))


if __name__ == '__main__':
    unittest.main()
