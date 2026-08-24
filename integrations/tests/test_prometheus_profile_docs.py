#!/usr/bin/env python3

import sys
import tempfile
import unittest
from pathlib import Path

INTEGRATIONS_DIR = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(INTEGRATIONS_DIR))

from _common import load_collectors  # noqa: E402
from gen_docs_integrations import clean_and_write  # noqa: E402
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

        public_chart_fields = {
            'context',
            'title',
            'family',
            'units',
            'dimensions',
            'selectors',
            'entity_scope',
        }
        for profile, document in self.catalog.items():
            for chart in _profile_charts(document).values():
                with self.subTest(profile=profile, context=chart['context']):
                    self.assertEqual(set(chart), public_chart_fields)

        ceph_contexts = {
            chart['context']
            for family in self.catalog['ceph']['families']
            for chart in _all_family_charts(family)
        }
        self.assertIn('health.health_checks.state', ceph_contexts)

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

    def test_rendered_catalogue_is_hierarchical_and_complete(self):
        collectors = load_collectors([('netdata/netdata', PROMETHEUS_METADATA, False)])
        project_prometheus_profile_coverage(collectors, self.catalog)
        ceph = next(item for item in collectors if item['meta']['id'].endswith('-ceph'))
        template = get_jinja_env().get_template('metrics.md')

        clean = template.render(entry=ceph, clean=True)
        rich = template.render(entry=ceph, clean=False)
        self.assertEqual(clean.count('data-prometheus-profile-chart'), 569)
        self.assertEqual(rich.count('<!-- prometheus-profile-chart -->'), 569)
        self.assertIn('Eligible metrics that are not covered by a curated chart', clean)
        self.assertNotIn('Operator question:', clean)
        self.assertNotIn('Operator question:', rich)
        self.assertIn('Entity scope:', clean)
        self.assertIn('Source metric selectors', clean)
        self.assertIn('Managers (1 chart)', clean)
        self.assertIn('Managers (1 chart)', rich)
        self.assertNotIn('(1 charts)', clean)
        self.assertNotIn('(1 charts)', rich)
        self.assertNotIn('| Metric |', clean)

    def test_rendered_catalogue_deduplicates_dimension_names_not_selectors(self):
        collectors = load_collectors([('netdata/netdata', PROMETHEUS_METADATA, False)])
        project_prometheus_profile_coverage(collectors, self.catalog)
        litellm = next(item for item in collectors if item['meta']['id'].endswith('-litellm'))
        clean = get_jinja_env().get_template('metrics.md').render(entry=litellm, clean=True)
        charts = _profile_charts(self.catalog['litellm'])

        cases = {
            'internal_services.request_outcomes': ('Internal Service Request Outcomes', 'values of label outcome', 22),
            'internal_services.failure_causes': ('Internal Service Failure Causes', 'values of label service', 11),
            'internal_services.latency_distribution': ('Internal Service Latency Distribution', 'matching series', 11),
            'internal_services.accumulated_latency': (
                'Accumulated Internal Service Latency',
                'values of label measurement',
                11,
            ),
        }
        for context, (title, dimension_name, selector_count) in cases.items():
            with self.subTest(context=context):
                chart = charts[context]
                self.assertEqual(len(chart['selectors']), selector_count)
                self.assertEqual({dimension['name'] for dimension in chart['dimensions']}, {dimension_name})

                start = clean.index(f'<summary>{title}</summary>')
                end = clean.find('<details data-prometheus-profile-chart>', start + 1)
                block = clean[start:end if end != -1 else None]
                self.assertEqual(block.count(f'`{dimension_name}`'), 1)
                self.assertIn(f'<summary>Source metric selectors ({selector_count})</summary>', block)

    def test_generated_markdown_preserves_catalogue_hooks(self):
        markdown = (
            '<!-- prometheus-profile-catalog -->\n'
            '{% details open=true summary="Coverage" %}\n'
            '<!-- prometheus-profile-chart -->\n'
            '{% details summary="Requests" %}\n'
            '{% /details %}\n{% /details %}\n'
        )
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'integration.md'
            clean_and_write(markdown, path)
            rendered = path.read_text(encoding='utf-8')
        self.assertIn(
            '<details open data-prometheus-profile-catalog>\n<summary>Coverage</summary>\n',
            rendered,
        )
        self.assertIn(
            '<details data-prometheus-profile-chart>\n<summary>Requests</summary>\n',
            rendered,
        )
        self.assertNotIn(
            '<details open data-prometheus-profile-catalog><summary>',
            rendered,
        )
        self.assertNotIn('<details data-prometheus-profile-chart><summary>', rendered)
        self.assertNotIn('prometheus-profile-catalog -->', rendered)

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


def _all_family_charts(family):
    yield from family['charts']
    for child in family['children']:
        yield from _all_family_charts(child)


def _profile_charts(profile):
    return {
        chart['context']: chart
        for family in profile['families']
        for chart in _all_family_charts(family)
    }


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
