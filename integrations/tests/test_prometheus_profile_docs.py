#!/usr/bin/env python3

import sys
import tempfile
import unittest
from copy import deepcopy
from pathlib import Path

INTEGRATIONS_DIR = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(INTEGRATIONS_DIR))

from _common import load_collectors  # noqa: E402
from gen_integrations import get_jinja_env  # noqa: E402
from prometheus_profile_docs import (  # noqa: E402
    ProfileCoverageError,
    load_profile_catalog,
    project_prometheus_profile_coverage,
)


REPO_ROOT = INTEGRATIONS_DIR.parent
PROMETHEUS_METADATA = (
    REPO_ROOT / 'src' / 'go' / 'plugin' / 'go.d' / 'collector' / 'prometheus' / 'metadata.yaml'
)


def _count_unescaped_pipes(value):
    count = 0
    backslashes = 0
    for character in value:
        if character == '\\':
            backslashes += 1
            continue
        if character == '|' and backslashes % 2 == 0:
            count += 1
        backslashes = 0
    return count


class PrometheusProfileCatalogTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.catalog = load_profile_catalog()

    def test_all_stock_profiles_have_exact_catalogues(self):
        self.assertEqual(
            {profile: document['chart_count'] for profile, document in self.catalog.items()},
            {
                'ceph': 569,
                'fastapi': 9,
                'haproxy': 204,
                'litellm': 101,
                'process_runtime': 5,
                'python_gc': 3,
                'vllm': 103,
            },
        )
        self.assertEqual(sum(document['chart_count'] for document in self.catalog.values()), 994)

        expected_counts = {
            'ceph': (1780, 1786, 22),
            'fastapi': (10, 10, 1),
            'haproxy': (226, 226, 7),
            'litellm': (130, 166, 11),
            'process_runtime': (5, 5, 1),
            'python_gc': (3, 3, 1),
            'vllm': (129, 129, 15),
        }
        for profile, document in self.catalog.items():
            with self.subTest(profile=profile):
                metric_count, mapping_count, group_count = expected_counts[profile]
                rows = list(_profile_rows(document))
                self.assertEqual(len({row['prometheus_metric'] for row in rows}), metric_count)
                self.assertEqual(len(document['metric_groups']), group_count)
                self.assertEqual(len(rows), mapping_count)
                self.assertTrue(all(group['rows'] for group in document['metric_groups']))
                for row in _profile_rows(document):
                    self.assertEqual(
                        set(row),
                        {'prometheus_metric', 'netdata_chart', 'dimension', 'unit', 'scope'},
                    )

        ceph_groups = self.catalog['ceph']['metric_groups']
        self.assertEqual(ceph_groups[0]['name'], 'Capacity')
        self.assertEqual(ceph_groups[-1]['name'], 'NVMe-oF')
        self.assertIn(
            {
                'prometheus_metric': 'ceph_num_objects_degraded',
                'netdata_chart': 'Capacity / Object Health — Objects',
                'dimension': 'degraded',
                'unit': 'objects',
                'scope': 'Ceph cluster endpoint',
            },
            ceph_groups[0]['rows'],
        )

    def test_projection_detaches_inherited_yaml_mappings(self):
        collectors = load_collectors([('netdata/netdata', PROMETHEUS_METADATA, False)])
        generic = next(item for item in collectors if item['meta']['id'].endswith('-generic'))
        aws = next(item for item in collectors if item['meta']['id'].endswith('-aws_ec2'))
        self.assertIs(generic['metrics'], aws['metrics'])

        project_prometheus_profile_coverage(collectors, self.catalog)
        covered = {
            item['meta']['id']: item['metrics']['profile_coverage']
            for item in collectors
            if 'profile_coverage' in item['metrics']
        }
        self.assertEqual(
            set(covered),
            {
                'collector-go.d.plugin-prometheus-generic',
                'collector-go.d.plugin-prometheus-haproxy',
                'collector-go.d.plugin-prometheus-ceph',
                'collector-go.d.plugin-prometheus-litellm',
                'collector-go.d.plugin-prometheus-vllm',
            },
        )
        self.assertNotIn('profile_coverage', aws['metrics'])
        self.assertEqual(covered['collector-go.d.plugin-prometheus-ceph']['chart_count'], 569)
        self.assertNotIn('metric_count', covered['collector-go.d.plugin-prometheus-ceph'])
        self.assertNotIn('mapping_count', covered['collector-go.d.plugin-prometheus-ceph'])
        self.assertEqual(covered['collector-go.d.plugin-prometheus-litellm']['chart_count'], 109)
        self.assertEqual(covered['collector-go.d.plugin-prometheus-vllm']['chart_count'], 120)

    def test_supporting_profiles_include_operator_activation(self):
        collectors = load_collectors([('netdata/netdata', PROMETHEUS_METADATA, False)])
        project_prometheus_profile_coverage(collectors, self.catalog)
        vllm = next(item for item in collectors if item['meta']['id'].endswith('-vllm'))
        profiles = vllm['metrics']['profile_coverage']['profiles']
        self.assertEqual(
            [(profile['id'], profile['role']) for profile in profiles],
            [
                ('vllm', 'primary'),
                ('fastapi', 'supporting'),
                ('process_runtime', 'supporting'),
                ('python_gc', 'supporting'),
            ],
        )
        for profile in profiles[1:]:
            self.assertTrue(profile['activation'])
            self.assertEqual(profile['supported_by'], 'vLLM')
        self.assertTrue(all('open' not in profile for profile in profiles))

    def test_unknown_and_unmapped_profiles_fail_closed(self):
        collectors = load_collectors([('netdata/netdata', PROMETHEUS_METADATA, False)])
        generic = next(item for item in collectors if item['meta']['id'].endswith('-generic'))
        generic['_prometheus_profile_ids'].append('unknown_profile')
        with self.assertRaisesRegex(ProfileCoverageError, 'maps unknown profiles'):
            project_prometheus_profile_coverage(collectors, self.catalog)

        collectors = load_collectors([('netdata/netdata', PROMETHEUS_METADATA, False)])
        vllm = next(item for item in collectors if item['meta']['id'].endswith('-vllm'))
        del vllm['_prometheus_profile_ids']
        with self.assertRaisesRegex(ProfileCoverageError, r"without an integration mapping: \['vllm'\]"):
            project_prometheus_profile_coverage(collectors, self.catalog)

        wrong_collector = {
            '_prometheus_profile_ids': ['ceph'],
            'meta': {'id': 'wrong', 'plugin_name': 'go.d.plugin', 'module_name': 'haproxy'},
            'metrics': {},
        }
        with self.assertRaisesRegex(ProfileCoverageError, 'allowed only on go.d.plugin/prometheus modules'):
            project_prometheus_profile_coverage([wrong_collector], self.catalog)

    def test_rendered_catalogue_is_grouped_tables_and_complete(self):
        collectors = load_collectors([('netdata/netdata', PROMETHEUS_METADATA, False)])
        project_prometheus_profile_coverage(collectors, self.catalog)
        ceph = next(item for item in collectors if item['meta']['id'].endswith('-ceph'))
        template = get_jinja_env().get_template('metrics.md')

        clean = template.render(entry=ceph, clean=True)
        rich = template.render(entry=ceph, clean=False)
        self.assertEqual(clean, rich)
        self.assertEqual(clean.count('| Prometheus metric | Netdata chart | Dimension | Unit | Scope |'), 22)
        self.assertEqual(sum(line.startswith('| <code>') for line in clean.splitlines()), 1786)
        self.assertIn('Eligible metrics that are not covered by a curated chart', clean)
        self.assertNotIn('Operator question:', clean)
        self.assertNotIn('Operator question:', rich)
        self.assertNotIn('Source metric selectors', clean)
        self.assertNotIn('<details', clean)
        self.assertNotIn('data-prometheus-profile', clean)
        self.assertIn('#### Managers', clean)
        self.assertIn(
            '| <code>ceph_num_objects_degraded</code> | Capacity / Object Health — Objects | '
            '<code>degraded</code> | <code>objects</code> | Ceph cluster endpoint |',
            clean,
        )

    def test_rendered_catalogue_keeps_every_metric_to_dimension_mapping(self):
        collectors = load_collectors([('netdata/netdata', PROMETHEUS_METADATA, False)])
        project_prometheus_profile_coverage(collectors, self.catalog)
        litellm = next(item for item in collectors if item['meta']['id'].endswith('-litellm'))
        clean = get_jinja_env().get_template('metrics.md').render(entry=litellm, clean=True)
        rows = list(_profile_rows(self.catalog['litellm']))

        cases = {
            'Internal Service Request Outcomes': ('values of label outcome', 22),
            'Internal Service Failure Causes': ('values of label service', 11),
            'Internal Service Latency Distribution': ('matching series', 11),
            'Accumulated Internal Service Latency': (
                'values of label measurement',
                11,
            ),
        }
        for title, (dimension_name, mapping_count) in cases.items():
            with self.subTest(title=title):
                chart_rows = [row for row in rows if row['netdata_chart'].endswith(f'— {title}')]
                self.assertEqual(len(chart_rows), mapping_count)
                self.assertEqual({row['dimension'] for row in chart_rows}, {dimension_name})
                self.assertEqual(
                    sum(
                        f'— {title} | <code>{dimension_name}</code>' in line
                        for line in clean.splitlines()
                    ),
                    mapping_count,
                )

    def test_rendered_table_preserves_pipes_and_escapes_text(self):
        collectors = load_collectors([('netdata/netdata', PROMETHEUS_METADATA, False)])
        project_prometheus_profile_coverage(collectors, deepcopy(self.catalog))
        ceph = next(item for item in collectors if item['meta']['id'].endswith('-ceph'))
        row = ceph['metrics']['profile_coverage']['profiles'][0]['metric_groups'][0]['rows'][0]
        row['prometheus_metric'] = 'ceph_metric{status=~"ok\\|error"}'
        row['netdata_chart'] = '<unsafe \\| chart>'
        row['dimension'] = 'dimension\\|value'
        row['unit'] = 'unit\\|value'
        row['scope'] = 'scope\\|value'
        rendered = get_jinja_env().get_template('metrics.md').render(entry=ceph, clean=True)

        expected_row = (
            '| <code>ceph_metric&#123;status=~"ok&#92;\\|error"&#125;</code> | '
            '&lt;unsafe &#92;\\| chart&gt; | <code>dimension&#92;\\|value</code> | '
            '<code>unit&#92;\\|value</code> | scope&#92;\\|value |'
        )
        self.assertIn(expected_row, rendered)
        self.assertEqual(_count_unescaped_pipes(expected_row), 6)
        self.assertNotIn('&#124;', rendered)

    def test_view_chart_mismatch_fails_closed(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            design, runtime = _write_minimal_catalog(root, contexts=('other_requests',))
            with self.assertRaisesRegex(ProfileCoverageError, 'view/chart mismatch'):
                load_profile_catalog(design, runtime)

    def test_duplicate_runtime_context_fails_closed(self):
        with tempfile.TemporaryDirectory() as directory:
            design, runtime = _write_minimal_catalog(Path(directory), contexts=('requests', 'requests'))
            with self.assertRaisesRegex(ProfileCoverageError, 'duplicate runtime chart context'):
                load_profile_catalog(design, runtime)

    def test_family_mismatch_fails_closed(self):
        with tempfile.TemporaryDirectory() as directory:
            design, runtime = _write_minimal_catalog(Path(directory), family='Other')
            with self.assertRaisesRegex(ProfileCoverageError, 'does not match runtime family'):
                load_profile_catalog(design, runtime)

    def test_non_mapping_runtime_document_fails_closed(self):
        with tempfile.TemporaryDirectory() as directory:
            design, runtime = _write_minimal_catalog(Path(directory))
            (runtime / 'example.yaml').write_text('- invalid\n', encoding='utf-8')
            with self.assertRaisesRegex(ProfileCoverageError, 'runtime document must be a mapping'):
                load_profile_catalog(design, runtime)


def _profile_rows(profile):
    for group in profile['metric_groups']:
        yield from group['rows']


def _write_minimal_catalog(root, contexts=('requests',), family='Service'):
    design_root = root / 'design'
    design = design_root / 'example'
    runtime = root / 'runtime'
    design.mkdir(parents=True)
    runtime.mkdir()
    (design / 'PROFILE-DESIGN.yaml').write_text(
        '''\
version: v1
profile: example
match: example_*
namespace: example
documentation:
  title: Example
  summary: Example metrics.
composition:
  supports: {}
entities:
  service:
    grain: service
views:
  requests:
    family: Service
    question: How many requests complete?
    entity: service
''',
        encoding='utf-8',
    )
    charts = ''.join(
        f'''\
    - title: Request rate
      context: {context}
      units: requests/s
      dimensions:
        - selector: example_requests_total
          name: requests
'''
        for context in contexts
    )
    (runtime / 'example.yaml').write_text(
        f'''\
match: example_*
template:
  family: {family}
  context_namespace: example
  charts:
{charts}''',
        encoding='utf-8',
    )
    return design_root, runtime


if __name__ == '__main__':
    unittest.main()
