#!/usr/bin/env python3

import sys
import tempfile
import unittest
import json
from pathlib import Path
from unittest.mock import patch

INTEGRATIONS_DIR = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(INTEGRATIONS_DIR))

import check_collector_taxonomy
import gen_taxonomy


class TaxonomySchemaTest(unittest.TestCase):
    def valid_taxonomy(self):
        return {
            'taxonomy_version': 1,
            'plugin_name': 'go.d.plugin',
            'module_name': 'apache',
            'placements': [
                {
                    'id': 'apache',
                    'section_id': 'applications.apache',
                    'title': 'Apache',
                    'items': ['apache.connections'],
                },
            ],
        }

    def test_valid_authoring_schema(self):
        errors = list(gen_taxonomy.COLLECTOR_TAXONOMY_VALIDATOR.iter_errors(self.valid_taxonomy()))
        self.assertEqual(errors, [])

    def test_section_path_authoring_is_rejected(self):
        data = self.valid_taxonomy()
        data['placements'][0]['section_path'] = ['applications', 'apache']
        errors = list(gen_taxonomy.COLLECTOR_TAXONOMY_VALIDATOR.iter_errors(data))
        self.assertTrue(errors)

    def test_old_contexts_authoring_is_rejected(self):
        data = self.valid_taxonomy()
        data['placements'][0]['contexts'] = data['placements'][0].pop('items')
        errors = list(gen_taxonomy.COLLECTOR_TAXONOMY_VALIDATOR.iter_errors(data))
        self.assertTrue(errors)

    def test_grid_rejects_string_shorthand(self):
        data = self.valid_taxonomy()
        data['placements'][0]['items'] = [
            {
                'type': 'grid',
                'id': 'apache-heads',
                'items': ['apache.connections'],
            },
        ]
        errors = list(gen_taxonomy.COLLECTOR_TAXONOMY_VALIDATOR.iter_errors(data))
        self.assertTrue(errors)

    def assert_schema_accepts_item(self, item):
        data = self.valid_taxonomy()
        data['placements'][0]['items'] = [item]
        errors = list(gen_taxonomy.COLLECTOR_TAXONOMY_VALIDATOR.iter_errors(data))
        self.assertEqual(errors, [])

    def assert_schema_rejects_item(self, item):
        data = self.valid_taxonomy()
        data['placements'][0]['items'] = [item]
        errors = list(gen_taxonomy.COLLECTOR_TAXONOMY_VALIDATOR.iter_errors(data))
        self.assertTrue(errors)

    def test_explicit_owned_context_is_accepted(self):
        self.assert_schema_accepts_item({
            'type': 'owned_context',
            'context': 'apache.connections',
        })

    def test_group_is_accepted(self):
        self.assert_schema_accepts_item({
            'type': 'group',
            'id': 'requests',
            'title': 'Requests',
            'items': ['apache.requests'],
        })

    def test_flatten_is_accepted(self):
        self.assert_schema_accepts_item({
            'type': 'flatten',
            'id': 'apache-flat',
            'title': 'Apache',
            'items': ['apache.connections'],
        })

    def test_first_available_is_accepted(self):
        self.assert_schema_accepts_item({
            'type': 'first_available',
            'items': [
                {
                    'type': 'context',
                    'contexts': ['apache.requests'],
                    'chart_library': 'number',
                },
            ],
        })

    def test_view_switch_is_accepted(self):
        self.assert_schema_accepts_item({
            'type': 'view_switch',
            'multi_node': {
                'type': 'context',
                'contexts': ['apache.requests'],
                'chart_library': 'bars',
            },
            'single_node': {
                'type': 'context',
                'contexts': ['apache.requests'],
                'chart_library': 'dygraph',
            },
        })

    def test_grid_rejects_owned_context(self):
        self.assert_schema_rejects_item({
            'type': 'grid',
            'id': 'apache-heads',
            'items': [
                {
                    'type': 'owned_context',
                    'context': 'apache.connections',
                },
            ],
        })

    def test_grid_rejects_selector(self):
        self.assert_schema_rejects_item({
            'type': 'grid',
            'id': 'apache-heads',
            'items': [
                {
                    'type': 'selector',
                    'id': 'apache-prefix',
                    'title': 'Apache prefix',
                    'context_prefix': ['apache.'],
                },
            ],
        })

    def test_flatten_rejects_nested_flatten(self):
        self.assert_schema_rejects_item({
            'type': 'flatten',
            'id': 'outer',
            'title': 'Outer',
            'items': [
                {
                    'type': 'flatten',
                    'id': 'inner',
                    'title': 'Inner',
                    'items': ['apache.connections'],
                },
            ],
        })

    def test_first_available_rejects_string_shorthand(self):
        self.assert_schema_rejects_item({
            'type': 'first_available',
            'items': ['apache.connections'],
        })

    def test_view_switch_rejects_string_branch(self):
        self.assert_schema_rejects_item({
            'type': 'view_switch',
            'multi_node': 'apache.connections',
            'single_node': {
                'type': 'context',
                'contexts': ['apache.connections'],
                'chart_library': 'number',
            },
        })

    def test_view_switch_rejects_flatten_branch(self):
        self.assert_schema_rejects_item({
            'type': 'view_switch',
            'multi_node': {
                'type': 'flatten',
                'id': 'flat',
                'title': 'Flat',
                'items': ['apache.connections'],
            },
            'single_node': {
                'type': 'context',
                'contexts': ['apache.connections'],
                'chart_library': 'number',
            },
        })

    def test_view_switch_rejects_nested_view_switch(self):
        self.assert_schema_rejects_item({
            'type': 'view_switch',
            'multi_node': {
                'type': 'view_switch',
                'multi_node': {
                    'type': 'context',
                    'contexts': ['apache.connections'],
                    'chart_library': 'number',
                },
                'single_node': {
                    'type': 'context',
                    'contexts': ['apache.connections'],
                    'chart_library': 'number',
                },
            },
            'single_node': {
                'type': 'context',
                'contexts': ['apache.connections'],
                'chart_library': 'number',
            },
        })

    def test_renderer_allows_x_extension(self):
        self.assert_schema_accepts_item({
            'type': 'context',
            'contexts': ['apache.connections'],
            'chart_library': 'number',
            'renderer': {
                'x_future_renderer_option': True,
            },
        })

    def test_renderer_rejects_unknown_non_extension_key(self):
        self.assert_schema_rejects_item({
            'type': 'context',
            'contexts': ['apache.connections'],
            'chart_library': 'number',
            'renderer': {
                'latetValue': 5,
            },
        })

    def test_renderer_fields_are_rejected_as_item_body_siblings(self):
        self.assert_schema_rejects_item({
            'type': 'context',
            'contexts': ['apache.connections'],
            'chart_library': 'number',
            'toolbox_elements': [],
        })

    def test_prescan_rejects_multi_node(self):
        findings = []
        gen_taxonomy.prescan_removed_shapes({'multi_node': {'title': 'Bad'}}, Path('taxonomy.yaml'), findings)
        self.assertEqual([finding.code for finding in findings], ['TAX022'])

    def test_optout_does_not_require_metadata(self):
        text = """taxonomy_version: 1
plugin_name: statsd.plugin
module_name: statsd
taxonomy_optout:
  reason: Operator-defined statsd synthetic charts have no static collector taxonomy.
"""
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / 'taxonomy.yaml'
            path.write_text(text)
            findings = []
            placements, optouts = gen_taxonomy.process_taxonomy_file(
                path,
                sections={},
                icons=set(),
                metadata_indexes={
                    'by_path_module': {},
                    'all_contexts': [],
                    'contexts_by_plugin': {},
                },
                ownership={},
                findings=findings,
            )
            self.assertEqual(placements, [])
            self.assertEqual(optouts[0]['collector_ids'], ['statsd.plugin-statsd'])
            self.assertEqual([finding.code for finding in findings], [])


class TaxonomyResolverTest(unittest.TestCase):
    def test_path_segment_uses_last_id_component(self):
        self.assertEqual(gen_taxonomy.path_segment({'id': 'applications.postgres'}), 'postgres')

    def test_resolve_prefix_uses_sorted_contexts(self):
        contexts = ['apache.requests', 'snmp.ifaces.in', 'snmp.ifaces.out', 'zfs.pool']
        self.assertEqual(
            gen_taxonomy.resolve_prefix('snmp.', contexts),
            ['snmp.ifaces.in', 'snmp.ifaces.out'],
        )

    def test_context_prefix_can_narrow_declared_dynamic_namespace(self):
        findings = []
        contexts = ['snmp.device_prof_ifTraffic', 'snmp.license.state']
        resolved = gen_taxonomy.resolve_node_contexts(
            {'context_prefix': ['snmp.device_prof_']},
            known_contexts=set(),
            allowed_prefixes={'snmp.'},
            allowed_plugins=set(),
            metadata_indexes={'all_contexts': contexts, 'contexts_by_plugin': {}},
            path=Path('taxonomy.yaml'),
            findings=findings,
        )
        self.assertEqual(resolved, ['snmp.device_prof_ifTraffic'])
        self.assertEqual([finding.code for finding in findings], [])

    def test_context_prefix_exclude_requires_prefix(self):
        findings = []
        gen_taxonomy.resolve_node_contexts(
            {'context_prefix_exclude': ['snmp.license.']},
            known_contexts=set(),
            allowed_prefixes=set(),
            allowed_plugins=set(),
            metadata_indexes={'all_contexts': [], 'contexts_by_plugin': {}},
            path=Path('taxonomy.yaml'),
            findings=findings,
        )
        self.assertEqual([finding.code for finding in findings], ['TAX029'])

    def test_metadata_loader_warnings_are_taxonomy_findings(self):
        original_len = len(gen_taxonomy.WARNINGS)

        def fake_load_collectors():
            gen_taxonomy.WARNINGS.append(('metadata.yaml', 'invalid metadata'))
            return []

        try:
            findings = []
            with patch.object(gen_taxonomy, 'load_collectors', side_effect=fake_load_collectors):
                indexes = gen_taxonomy.build_metadata_indexes(findings)
            self.assertEqual(indexes['modules'], [])
            self.assertEqual([finding.code for finding in findings], ['TAX001'])
            self.assertEqual(findings[0].message, 'invalid metadata')
        finally:
            del gen_taxonomy.WARNINGS[original_len:]


class TaxonomyOwnershipTest(unittest.TestCase):
    def metadata_indexes(self, tmp, context='apache.connections', contexts=None, dynamic_prefixes=None):
        metadata_path = Path(tmp) / 'metadata.yaml'
        if contexts is None:
            contexts = [context]
        metrics = {
            'scopes': [
                {
                    'metrics': [
                        {'name': name}
                        for name in contexts
                    ],
                },
            ],
        }
        if dynamic_prefixes:
            metrics['dynamic_context_prefixes'] = [
                {
                    'prefix': prefix,
                    'reason': 'test dynamic contexts',
                }
                for prefix in dynamic_prefixes
            ]
        module = {
            '_src_path': str(metadata_path),
            'meta': {
                'plugin_name': 'go.d.plugin',
                'module_name': 'apache',
            },
            'metrics': metrics,
        }
        return {
            'by_path_module': {
                (metadata_path, 'go.d.plugin', 'apache'): [module],
            },
            'all_contexts': sorted(contexts),
            'contexts_by_plugin': {},
        }

    def write_taxonomy(self, tmp, item):
        path = Path(tmp) / 'taxonomy.yaml'
        path.write_text("""taxonomy_version: 1
plugin_name: go.d.plugin
module_name: apache
placements:
  - id: apache
    section_id: applications.apache
    title: Apache
    items:
""")
        with path.open('a') as fp:
            fp.write(item)
        return path

    def test_referenced_context_without_owner_is_fatal(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = self.write_taxonomy(tmp, """      - type: context
        contexts: [apache.connections]
        chart_library: number
""")
            ownership = {}
            referenced_literals = []
            findings = []
            gen_taxonomy.process_taxonomy_file(
                path,
                sections={'applications.apache': ({'status': 'active', 'section_order': 1}, 'applications.apache')},
                icons=set(),
                metadata_indexes=self.metadata_indexes(tmp),
                ownership=ownership,
                findings=findings,
                referenced_literals=referenced_literals,
            )
            gen_taxonomy.emit_referenced_only_findings(referenced_literals, ownership, findings)
            self.assertEqual([finding.code for finding in findings], ['TAX037'])

    def test_referenced_context_owned_elsewhere_is_allowed(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = self.write_taxonomy(tmp, """      - apache.connections
      - type: context
        contexts: [apache.connections]
        chart_library: number
""")
            ownership = {}
            referenced_literals = []
            findings = []
            gen_taxonomy.process_taxonomy_file(
                path,
                sections={'applications.apache': ({'status': 'active', 'section_order': 1}, 'applications.apache')},
                icons=set(),
                metadata_indexes=self.metadata_indexes(tmp),
                ownership=ownership,
                findings=findings,
                referenced_literals=referenced_literals,
            )
            gen_taxonomy.emit_referenced_only_findings(referenced_literals, ownership, findings)
            self.assertEqual([finding.code for finding in findings], [])

    def test_unknown_literal_context_is_tax003(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = self.write_taxonomy(tmp, """      - apache.unknown
""")
            findings = []
            gen_taxonomy.process_taxonomy_file(
                path,
                sections={'applications.apache': ({'status': 'active', 'section_order': 1}, 'applications.apache')},
                icons=set(),
                metadata_indexes=self.metadata_indexes(tmp),
                ownership={},
                findings=findings,
            )
            self.assertEqual([finding.code for finding in findings], ['TAX003'])

    def test_selector_overlap_uses_tax036(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = self.write_taxonomy(tmp, """      - apache.connections
      - type: selector
        id: apache-prefix
        title: Apache prefix
        context_prefix: [apache.]
""")
            ownership = {}
            ownership_conflicts = {}
            referenced_literals = []
            findings = []
            gen_taxonomy.process_taxonomy_file(
                path,
                sections={'applications.apache': ({'status': 'active', 'section_order': 1}, 'applications.apache')},
                icons=set(),
                metadata_indexes=self.metadata_indexes(tmp, dynamic_prefixes=['apache.']),
                ownership=ownership,
                findings=findings,
                referenced_literals=referenced_literals,
                ownership_conflicts=ownership_conflicts,
            )
            gen_taxonomy.emit_ownership_conflicts(ownership_conflicts, findings)
            self.assertEqual([finding.code for finding in findings], ['TAX036'])

    def test_duplicate_literal_ownership_uses_tax033_once(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = self.write_taxonomy(tmp, """      - apache.connections
      - type: group
        id: duplicate
        title: Duplicate
        items:
          - apache.connections
""")
            ownership = {}
            ownership_conflicts = {}
            findings = []
            gen_taxonomy.process_taxonomy_file(
                path,
                sections={'applications.apache': ({'status': 'active', 'section_order': 1}, 'applications.apache')},
                icons=set(),
                metadata_indexes=self.metadata_indexes(tmp),
                ownership=ownership,
                findings=findings,
                ownership_conflicts=ownership_conflicts,
            )
            gen_taxonomy.emit_ownership_conflicts(ownership_conflicts, findings)
            self.assertEqual([finding.code for finding in findings], ['TAX033'])

    def test_stale_unresolved_reference_warns(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = self.write_taxonomy(tmp, """      - type: context
        contexts:
          - context: apache.connections
            unresolved:
              reason: staged rename
              owner: cloud-frontend
              expires: "2026-08-01"
        chart_library: number
""")
            ownership = {}
            referenced_literals = []
            findings = []
            gen_taxonomy.process_taxonomy_file(
                path,
                sections={'applications.apache': ({'status': 'active', 'section_order': 1}, 'applications.apache')},
                icons=set(),
                metadata_indexes=self.metadata_indexes(tmp),
                ownership=ownership,
                findings=findings,
                referenced_literals=referenced_literals,
            )
            gen_taxonomy.emit_referenced_only_findings(referenced_literals, ownership, findings)
            self.assertEqual([finding.code for finding in findings], ['TAX038'])

    def test_unresolved_reference_payload_is_emitted(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = self.write_taxonomy(tmp, """      - type: context
        contexts:
          - context: apache.future
            unresolved:
              reason: staged rename
              owner: cloud-frontend
              expires: "2026-08-01"
        chart_library: number
""")
            findings = []
            placements, _ = gen_taxonomy.process_taxonomy_file(
                path,
                sections={'applications.apache': ({'status': 'active', 'section_order': 1}, 'applications.apache')},
                icons=set(),
                metadata_indexes=self.metadata_indexes(tmp),
                ownership={},
                findings=findings,
            )
            self.assertEqual([finding.code for finding in findings], [])
            unresolved = placements[0]['unresolved_references']
            self.assertEqual(unresolved, [
                {
                    'context': 'apache.future',
                    'reason': 'staged rename',
                    'owner': 'cloud-frontend',
                    'expires': '2026-08-01',
                    'item_path': 'apache.0',
                },
            ])
            self.assertEqual(placements[0]['items'][0]['unresolved_references'], unresolved)


class TouchedCollectorGateTest(unittest.TestCase):
    def test_metadata_metrics_spans_ignore_overview_blocks(self):
        text = """plugin_name: go.d.plugin
modules:
  - meta:
      module_name: demo
    overview:
      data_collection:
        metrics_description: demo
    metrics:
      folding:
        title: Metrics
      scopes: []
    setup:
      configuration: {}
"""
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / 'metadata.yaml'
            path.write_text(text)
            self.assertEqual(check_collector_taxonomy.metadata_metrics_spans(path), [(8, 11)])

    def test_metadata_metrics_spans_do_not_depend_on_four_space_indent(self):
        text = """plugin_name: go.d.plugin
modules:
- meta:
    module_name: demo
  metrics:
    folding:
      title: Metrics
    scopes: []
  setup:
    configuration: {}
"""
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / 'metadata.yaml'
            path.write_text(text)
            self.assertEqual(check_collector_taxonomy.metadata_metrics_spans(path), [(5, 8)])

    def test_range_intersection(self):
        spans = [(7, 10)]
        self.assertFalse(check_collector_taxonomy.range_intersects_spans(3, 1, spans))
        self.assertTrue(check_collector_taxonomy.range_intersects_spans(8, 1, spans))

    def test_missing_metrics_block_with_diff_is_touched(self):
        text = """plugin_name: go.d.plugin
modules:
  - meta:
      module_name: demo
    setup:
      configuration: {}
"""
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / 'metadata.yaml'
            path.write_text(text)
            with patch.object(check_collector_taxonomy, 'run_git', return_value='@@ -8,4 +0,0 @@\n-    metrics:\n'):
                self.assertTrue(check_collector_taxonomy.metadata_metrics_touched('base...head', path))

    def test_missing_metrics_block_without_diff_is_not_touched(self):
        text = """plugin_name: go.d.plugin
modules:
  - meta:
      module_name: demo
"""
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / 'metadata.yaml'
            path.write_text(text)
            with patch.object(check_collector_taxonomy, 'run_git', return_value=''):
                self.assertFalse(check_collector_taxonomy.metadata_metrics_touched('base...head', path))

    def test_deleted_collector_does_not_require_taxonomy(self):
        with tempfile.TemporaryDirectory() as tmp:
            collector_dir = Path(tmp) / 'demo'
            collector_dir.mkdir()
            with patch.object(check_collector_taxonomy, 'touched_collectors', return_value=[collector_dir]):
                findings = check_collector_taxonomy.check_touched_coverage('base...head')
            self.assertEqual(findings, [])


class TaxonomyDeterminismTest(unittest.TestCase):
    def test_build_taxonomy_is_byte_identical_for_ten_runs(self):
        outputs = []
        for _ in range(10):
            taxonomy, findings = gen_taxonomy.build_taxonomy()
            self.assertEqual([finding for finding in findings if finding.severity == gen_taxonomy.FATAL], [])
            outputs.append(json.dumps(taxonomy, indent=2, sort_keys=True) + '\n')
        self.assertEqual(len(set(outputs)), 1)


if __name__ == '__main__':
    unittest.main()
