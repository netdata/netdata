#!/usr/bin/env python3
"""Stock health alert metadata for the integrations catalogue.

Reads ``src/health/health.d/*.conf`` with the line rules of the Agent's ``health_readfile`` and builds the ``alerts``
export of ``integrations.js``/``integrations.json``, checks the ``alerts`` rows of integration metadata against it, and
checks the stock alerts' ``class``, ``type`` and ``component`` against the classification vocabulary.
Text values drop quotes that wrap the whole value and turn inner double quotes into spaces; single quotes stay.
"""

from __future__ import annotations

from pathlib import Path
from typing import AbstractSet, Dict, Iterable, List, Mapping, Optional, Tuple

from _common import AGENT_REPO, REPO_PATH

HEALTH_CONFIG_DIR = REPO_PATH / 'src' / 'health' / 'health.d'
CONF_LINK_PREFIX = f'https://github.com/{AGENT_REPO}/blob/master/'

RULE_KEYS = ('alarm', 'template')
EXPORTED_FIELDS = ('class', 'type', 'component', 'units', 'summary', 'info')

# Stock alert classification vocabulary. src/health/REFERENCE.md ("Alert Line `class`", "Alert Line `type`") defines
# each value; a test keeps those tables equal to these tuples. Components follow rules, not a list (REFERENCE.md,
# "Alert Line `component`").
CLASSIFICATION_FIELDS = ('class', 'type', 'component')
ALERT_CLASSES = ('Availability', 'Errors', 'Latency', 'Utilization', 'Workload')
ALERT_TYPES = (
    'Application', 'Containers', 'Database', 'DHCP', 'DNS', 'GPU', 'Hardware', 'Kubernetes', 'Messaging', 'Netdata',
    'Network', 'Storage', 'Synthetic', 'System', 'Virtual Machine', 'Web Server',
)


def _trim_all(text: str) -> str:
    return ' '.join(text.split())


def _strip_quotes(text: str) -> str:
    if len(text) >= 2 and text[0] == text[-1] and text[0] in '"\'':
        text = text[1:-1]
    return text.replace('"', ' ')


def _logical_lines(text: str) -> Iterable[str]:
    pending = ''
    for raw in text.split('\n'):
        line = (pending + raw).strip()
        pending = ''
        if not line or line.startswith('#'):
            continue
        if line.endswith('\\'):
            pending = line[:-1] + ' '
            continue
        yield line
    if pending.strip():
        yield pending.strip()


def parse_health_config(text: str, source: str = '') -> List[dict]:
    """Return the rules of one health configuration file, in file order."""
    rules = []
    rule = None

    for line in _logical_lines(text):
        key, sep, value = line.partition(':')
        if not sep:
            continue

        key = _trim_all(key).lower()
        value = _trim_all(value)
        if not key or not value:
            continue

        if key in RULE_KEYS:
            rule = {'name': value, 'template': key == 'template', 'source': source}
            rules.append(rule)
        elif rule is None:
            continue
        elif key == 'on':
            rule['on'] = value
        elif key in EXPORTED_FIELDS:
            rule[key] = _strip_quotes(value)

    return rules


def load_stock_rules(directory: Path = HEALTH_CONFIG_DIR) -> List[dict]:
    rules = []
    for path in sorted(directory.glob('*.conf')):
        text = path.read_text(encoding='utf-8', errors='replace')
        rules.extend(parse_health_config(text, path.relative_to(REPO_PATH).as_posix()))
    return rules


def rules_by_name(rules: Iterable[dict]) -> Dict[str, List[dict]]:
    grouped: Dict[str, List[dict]] = {}
    for rule in rules:
        grouped.setdefault(rule['name'], []).append(rule)
    return grouped


def build_alert_index(rules: Iterable[dict], owners: Optional[Mapping[str, Iterable[str]]] = None) -> Dict[str, dict]:
    """One entry per alert name: the first rule supplies the fields, `on` lists every rule's target."""
    owners = owners or {}
    index = {}

    for name, group in sorted(rules_by_name(rules).items()):
        first = group[0]
        entry = {'template': first['template']}
        targets = [rule['on'] for rule in group if rule.get('on')]
        entry['on'] = list(dict.fromkeys(targets))
        for field in EXPORTED_FIELDS:
            if field in first:
                entry[field] = first[field]
        entry['source'] = first['source']
        entry['integrations'] = sorted(set(owners.get(name, ())))
        index[name] = entry

    return index


def conf_link(source: str) -> str:
    return CONF_LINK_PREFIX + source


def _components_collide(first: str, second: str) -> bool:
    first, second = first.casefold(), second.casefold()
    return first == second or any(first == second + suffix or second == first + suffix for suffix in ('s', 'es'))


def check_classification(rules: Iterable[dict]) -> List[str]:
    """Problems with the ``class``, ``type`` and ``component`` of stock rules.

    Every rule sets all three; class and type come from the vocabulary; one component is never spelled two ways that
    differ only in case or a plural ending.
    """
    problems = []
    spellings: Dict[str, str] = {}

    for rule in rules:
        where = f'{rule.get("source", "?")}: {rule["name"]}'
        for field in CLASSIFICATION_FIELDS:
            if field not in rule:
                problems.append(f'{where}: no {field}')
        if 'class' in rule and rule['class'] not in ALERT_CLASSES:
            problems.append(f'{where}: class {rule["class"]!r} is not one of {", ".join(ALERT_CLASSES)}')
        if 'type' in rule and rule['type'] not in ALERT_TYPES:
            problems.append(f'{where}: type {rule["type"]!r} is not one of {", ".join(ALERT_TYPES)}')
        component = rule.get('component')
        if component is None or component in spellings:
            continue
        for seen, seen_where in spellings.items():
            if _components_collide(component, seen):
                problems.append(f'{where}: component {component!r} collides with {seen!r} ({seen_where})')
        spellings[component] = where

    return problems


def check_alert_rows(
    rows: Iterable[tuple], rules: Iterable[dict], metric_elsewhere: AbstractSet[Tuple[str, str]] = frozenset()
) -> List[str]:
    """Problems with metadata ``alerts`` rows, given as ``(where, row)`` pairs.

    A row must name a stock alert, link a file that defines it, and have ``metric`` equal to one of the alert's ``on``
    targets. An alarm's ``on`` is a chart id; ``metric_elsewhere`` lists the ``(name, metric)`` pairs whose chart id
    differs from the context the row documents.
    """
    grouped = rules_by_name(rules)
    problems = []

    for where, row in rows:
        name = row.get('name')
        group = grouped.get(name)
        if not group:
            problems.append(f'{where}: alert {name!r} is not defined in {HEALTH_CONFIG_DIR.relative_to(REPO_PATH)}')
            continue

        links = sorted({conf_link(rule['source']) for rule in group})
        if row.get('link') not in links:
            problems.append(f'{where}: alert {name!r} link {row.get("link")!r} is not {" or ".join(links)}')

        targets = sorted({rule['on'] for rule in group if rule.get('on')})
        if row.get('metric') not in targets and (name, row.get('metric')) not in metric_elsewhere:
            problems.append(f'{where}: alert {name!r} metric {row.get("metric")!r} is not {" or ".join(targets)}')

    return problems
