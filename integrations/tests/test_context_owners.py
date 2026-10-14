#!/usr/bin/env python3
"""The catalogue's ``contexts`` export: exact documented contexts and the context prefixes that identify a collector
whose contexts are created at runtime. The rules are documented in ``integrations/context_owners.py`` and
`.agents/skills/integrations-lifecycle/in-app-contract.md`.
"""

import sys
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
from gen_integrations import context_subjects, load_flows  # noqa: E402
from prometheus_profile_docs import project_prometheus_profile_coverage  # noqa: E402


def subject(record_id, contexts, declared=(), module=None):
    plugin, name = (module or f'go.d.plugin/{record_id}').split('/', 1)
    return context_subject(record_id, plugin, name, contexts, declared)


class NamespacePrefixesTest(unittest.TestCase):
    def test_prefixes(self):
        cases = {
            'one namespace': (['redis.memory', 'redis.clients'], {'redis.'}),
            'deeper common part': (['app.db.reads', 'app.db.writes'], {'app.db.'}),
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
                self.assertEqual(kept_prefixes(subjects), expected)

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


class CatalogueContextOwnersTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        items = load_collectors()
        project_prometheus_profile_coverage(items)
        cls.items = items + load_flows()
        cls.subjects = context_subjects(cls.items)
        cls.export = context_owners(cls.subjects)

    def test_every_declared_prefix_is_exported(self):
        problems = declared_prefix_problems(self.subjects)
        declared = sorted(
            (entry['prefix'], make_id(item['meta']))
            for item in self.items
            for entry in (item.get('metrics') or {}).get('dynamic_context_prefixes') or []
        )
        missing = [pair for pair in declared if pair[1] not in self.export['prefixes'].get(pair[0], [])]

        self.assertEqual((problems, missing), ([], []), '\n'.join(problems))

    def test_the_export_is_sorted_so_regeneration_is_byte_stable(self):
        unsorted = [key for part in self.export.values() for key, ids in part.items() if ids != sorted(ids)]

        self.assertEqual(([list(part) == sorted(part) for part in self.export.values()], unsorted), ([True, True], []))

    def test_no_prefix_takes_another_modules_contexts(self):
        module_of = {s['id']: s['module'] for s in self.subjects}
        prefix_modules = {prefix: {module_of[i] for i in ids} for prefix, ids in self.export['prefixes'].items()}
        problems = []
        for prefix, owners in prefix_modules.items():
            for s in self.subjects:
                if s['module'] in owners:
                    continue
                for context in s['contexts']:
                    longer = [p for p, m in prefix_modules.items()
                              if len(p) > len(prefix) and context.startswith(p) and s['module'] in m]
                    if context.startswith(prefix) and not longer:
                        problems.append(f'{prefix} ({", ".join(sorted(owners))}) covers {s["module"]}: {context}')

        self.assertEqual(problems, [], '\n'.join(problems[:20]))


if __name__ == '__main__':
    unittest.main()
