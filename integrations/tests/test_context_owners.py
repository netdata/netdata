#!/usr/bin/env python3
"""The catalogue's ``contexts`` export: exact documented contexts and the context prefixes that identify a collector
whose contexts are created at runtime. The rules are documented in ``integrations/context_owners.py`` and
`.agents/skills/integrations-lifecycle/in-app-contract.md`.
"""

import json
import sys
import tempfile
import unittest
from pathlib import Path

INTEGRATIONS_DIR = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(INTEGRATIONS_DIR))

from _common import load_collectors, make_id  # noqa: E402
from context_owners import (  # noqa: E402
    context_owners,
    context_subject,
    declared_prefix_problems,
    kept_prefixes,
    namespace_prefixes,
)
from gen_integrations import (  # noqa: E402
    build_context_index,
    context_subjects,
    declared_contexts,
    load_flows,
    render_integrations,
    render_json,
)
from prometheus_profile_docs import project_prometheus_profile_coverage  # noqa: E402


def subject(record_id, contexts, declared=(), module=None):
    plugin, name = (module or f'go.d.plugin/{record_id}').split('/', 1)
    return context_subject(record_id, plugin, name, contexts, declared)


def prefix_takeovers(subjects, export):
    """Contexts a module documents that the longest exported prefix gives to another module (exact matches aside)."""
    module_of = {s['id']: s['module'] for s in subjects}
    problems = []
    for s in subjects:
        for context in s['contexts']:
            matching = [prefix for prefix in export['prefixes'] if context.startswith(prefix)]
            if not matching:
                continue
            longest = max(matching, key=len)
            owners = {module_of[i] for i in export['prefixes'][longest]}
            if s['module'] not in owners:
                problems.append(f'{s["module"]}: {context} -> {longest} ({", ".join(sorted(owners))})')
    return problems


class NamespacePrefixesTest(unittest.TestCase):
    def test_prefixes(self):
        cases = {
            'one namespace': (['redis.memory', 'redis.clients'], {'redis.'}),
            'deeper common part': (['app.db.reads', 'app.db.writes'], {'app.db.'}),
            'dot-aligned, not character-aligned': (['app.db1.x', 'app.db2.y'], {'app.'}),
            'one segment always remains': (['app.db.reads', 'app.db'], {'app.'}),
            'a single context': (['app.memory'], {'app.'}),
            'one prefix per namespace': (['a.x', 'a.y', 'b.c.d'], {'a.', 'b.c.'}),
            'the Agent namespace is skipped': (['netdata.plugin_x.cpu', 'app.cpu'], {'app.'}),
            'a context without a dot has no namespace': (['standalone'], set()),
        }
        for name, (contexts, expected) in cases.items():
            with self.subTest(name):
                self.assertEqual(namespace_prefixes(contexts), expected)


class KeptPrefixesTest(unittest.TestCase):
    def test_kept(self):
        cases = {
            'a namespace no other module uses': (
                [subject('redis', ['redis.memory']), subject('mysql', ['mysql.queries'])],
                {'redis': {'redis.'}, 'mysql': {'mysql.'}},
            ),
            'a shared namespace is nobody\'s prefix': (
                [subject('proc', ['system.cpu', 'system.load']), subject('freebsd', ['system.cpu'])],
                {'proc': set(), 'freebsd': set()},
            ),
            'records of one module share their prefix': (
                [subject('Redis', ['redis.memory'], module='go.d.plugin/redis'),
                 subject('Valkey', ['redis.memory'], module='go.d.plugin/redis')],
                {'Redis': {'redis.'}, 'Valkey': {'redis.'}},
            ),
            'a longer prefix of the other module resolves the overlap': (
                [subject('snmp', [], declared=['snmp.']),
                 subject('snmp_traps', ['snmp.trap.received'], declared=['snmp.trap.'])],
                {'snmp': {'snmp.'}, 'snmp_traps': {'snmp.trap.'}},
            ),
            'a shorter prefix of the other module does not': (
                [subject('a', ['foo.bar.x']), subject('b', ['foo.bar.z', 'foo.q'])],
                {'a': set(), 'b': set()},
            ),
            'a dropped longer prefix no longer resolves it': (
                [subject('systemdunits', ['systemd.service_unit_state']),
                 subject('cgroups', ['systemd.service.cpu']),
                 subject('other', ['systemd.service.cpu'])],
                {'systemdunits': set(), 'cgroups': set(), 'other': set()},
            ),
            'a declared prefix without documented contexts': (
                [subject('azure', [], declared=['azure_monitor.']),
                 subject('azure_sql', ['azure_monitor.sql_database.cpu'])],
                {'azure': {'azure_monitor.'}, 'azure_sql': {'azure_monitor.sql_database.'}},
            ),
        }
        for name, (subjects, expected) in cases.items():
            with self.subTest(name):
                self.assertEqual(
                    (kept_prefixes(subjects), prefix_takeovers(subjects, context_owners(subjects))),
                    (expected, []),
                )

    def test_a_declared_prefix_covering_another_module_is_reported(self):
        subjects = [subject('sql', [], declared=['mysql.']), subject('mysql', ['mysql.queries.total', 'mysql.qps'])]

        self.assertEqual(
            (kept_prefixes(subjects)['sql'], declared_prefix_problems(subjects)),
            (set(), ["sql: declared prefix 'mysql.' also covers go.d.plugin/mysql: mysql.qps, "
                     "go.d.plugin/mysql: mysql.queries.total"]),
        )


class ContextOwnersTest(unittest.TestCase):
    def test_exact_contexts_and_prefixes_map_to_sorted_record_ids(self):
        subjects = [
            subject('Valkey', ['redis.memory'], module='go.d.plugin/redis'),
            subject('Redis', ['redis.memory', 'redis.clients'], module='go.d.plugin/redis'),
            subject('proc', ['system.cpu']),
            subject('freebsd', ['system.cpu']),
        ]

        self.assertEqual(context_owners(subjects), {
            'exact': {
                'redis.clients': ['Redis'],
                'redis.memory': ['Redis', 'Valkey'],
                'system.cpu': ['freebsd', 'proc'],
            },
            'prefixes': {'redis.': ['Redis', 'Valkey']},
        })

    def test_both_catalogue_files_carry_the_export(self):
        contexts = {'exact': {'disk.space': ['p-m-M']}, 'prefixes': {'disk.': ['p-m-M']}}

        with tempfile.TemporaryDirectory() as directory:
            js_path, json_path = Path(directory) / 'integrations.js', Path(directory) / 'integrations.json'
            render_integrations([], [], {}, contexts, js_path)
            render_json([], [], {}, contexts, json_path)
            from_js = json.loads(js_path.read_text().split('\nexport const contexts = ', 1)[1])
            from_json = json.loads(json_path.read_text())['contexts']

        self.assertEqual((from_js, from_json), (contexts, contexts))


class CatalogueContextOwnersTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        collectors = load_collectors()
        project_prometheus_profile_coverage(collectors)
        flows = load_flows()
        cls.items = collectors + flows
        cls.subjects = context_subjects(cls.items)
        cls.export = build_context_index(collectors, flows)

    def test_every_documented_context_of_collectors_and_flows_is_exported(self):
        missing = sorted(
            f'{make_id(item["meta"])}: {context}'
            for item in self.items
            for context in declared_contexts(item)
            if make_id(item['meta']) not in self.export['exact'].get(context, [])
        )

        self.assertEqual(missing, [])

    def test_every_declared_prefix_is_exported(self):
        problems = declared_prefix_problems(self.subjects)
        declared = sorted(
            (entry['prefix'], make_id(item['meta']))
            for item in self.items
            for entry in (item.get('metrics') or {}).get('dynamic_context_prefixes') or []
        )
        missing = [pair for pair in declared if pair[1] not in self.export['prefixes'].get(pair[0], [])]

        self.assertEqual((problems, missing), ([], []), '\n'.join(problems))

    def test_every_prefix_ends_with_a_dot(self):
        self.assertEqual([prefix for prefix in self.export['prefixes'] if not prefix.endswith('.')], [])

    def test_the_export_is_sorted_so_regeneration_is_byte_stable(self):
        unsorted = [key for part in self.export.values() for key, ids in part.items() if ids != sorted(ids)]

        self.assertEqual(([list(part) == sorted(part) for part in self.export.values()], unsorted), ([True, True], []))

    def test_no_prefix_takes_another_modules_contexts(self):
        problems = prefix_takeovers(self.subjects, self.export)

        self.assertEqual(problems, [], '\n'.join(problems[:20]))


if __name__ == '__main__':
    unittest.main()
