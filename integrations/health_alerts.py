#!/usr/bin/env python3
"""Stock health alert metadata for the integrations catalogue.

Reads ``src/health/health.d/*.conf`` with the line rules of the Agent's ``health_readfile``, decides which integrations
own each stock alert, builds the integration pages' alert rows and the ``alerts`` export of
``integrations.js``/``integrations.json``, and checks the stock alerts' ``class``, ``type`` and ``component`` against
the classification vocabulary.
Text values drop quotes that wrap the whole value and turn inner double quotes into spaces; single quotes stay.
"""

from __future__ import annotations

import re
from pathlib import Path
from typing import Dict, Iterable, List, Mapping, Optional, Sequence, Tuple

from _common import AGENT_REPO, REPO_PATH

HEALTH_CONFIG_DIR = REPO_PATH / 'src' / 'health' / 'health.d'
CONF_LINK_PREFIX = f'https://github.com/{AGENT_REPO}/blob/master/'

RULE_KEYS = ('alarm', 'template')
EXPORTED_FIELDS = ('class', 'type', 'component', 'units', 'summary', 'info')

# Match lines the Agent folds into label patterns (health_config.c, PARSE_HEALTH_CONFIG_LINE_PATTERN_APPEND): the key
# names the label it implies, or None when the value carries its own `label=pattern` words.
HOST_LABEL_KEYS = {'os': '_os', 'hosts': '_hostname', 'host labels': None}
CHART_LABEL_KEYS = {'plugin': '_collect_plugin', 'module': '_collect_module', 'chart labels': None}

# Stock alert classification vocabulary. src/health/REFERENCE.md ("Alert Line `class`", "Alert Line `type`") defines
# each value; a test keeps those tables equal to these tuples. Components follow rules, not a list (REFERENCE.md,
# "Alert Line `component`").
CLASSIFICATION_FIELDS = ('class', 'type', 'component')
ALERT_CLASSES = ('Availability', 'Errors', 'Latency', 'Utilization', 'Workload')
ALERT_TYPES = (
    'Application', 'Containers', 'Database', 'DHCP', 'DNS', 'GPU', 'Hardware', 'Kubernetes', 'Messaging', 'Netdata',
    'Network', 'Storage', 'Synthetic', 'System', 'Virtual Machine', 'Web Server',
)

# Values of the `_os` host label (NETDATA_OS_TYPE in src/libnetdata/libnetdata-platform-fwd.h).
PLATFORMS = ('linux', 'freebsd', 'macos', 'windows')

# Plugins the build compiles for some platforms only: the daemon's built-in plugins per OS in
# packaging/cmake/Modules/NetdataDaemon.cmake, the external ones in NetdataOptions.cmake (eBPF and the Go IPMI and
# IBM plugins are Linux-only). A plugin not listed here runs on every platform.
PLUGIN_PLATFORMS = {
    'cgroups.plugin': ('linux',),
    'charts.d.plugin': ('linux', 'freebsd', 'macos'),
    'cups.plugin': ('linux', 'freebsd', 'macos'),
    'debugfs.plugin': ('linux',),
    'diskspace.plugin': ('linux',),
    'ebpf-go.plugin': ('linux',),
    'ebpf.plugin': ('linux',),
    'freebsd.plugin': ('freebsd',),
    'freeipmi.plugin': ('linux', 'freebsd'),
    'ibm.d.plugin': ('linux',),
    'ipmi.plugin': ('linux',),
    'macos.plugin': ('macos',),
    'netflow-plugin': ('linux', 'freebsd', 'macos'),
    'nfacct.plugin': ('linux',),
    'otel.plugin': ('linux', 'macos'),
    'perf.plugin': ('linux',),
    'proc.plugin': ('linux',),
    'slabinfo.plugin': ('linux',),
    'tc.plugin': ('linux',),
    'timex.plugin': ('linux', 'freebsd', 'macos'),
    'windows.plugin': ('windows',),
    'xenstat.plugin': ('linux',),
}

# Alerts on the Agent's own charts belong to no integration.
AGENT_INTERNAL_PREFIXES = ('netdata.', 'anomaly_detection.')


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


def _add_pattern(rule: dict, field: str, label: Optional[str], value: str) -> None:
    """Add one match line to a rule's label patterns as the Agent does: `*` matches anything and adds nothing, a
    value without `label=` is ignored where the key implies no label, and a later line goes first."""
    if value == '*' or (label is None and '=' not in value):
        return
    pattern = f'{label}={value}' if label else value
    rule[field] = f'{pattern} {rule[field]}' if field in rule else pattern


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
        elif key in HOST_LABEL_KEYS:
            _add_pattern(rule, 'host_labels', HOST_LABEL_KEYS[key], value)
        elif key in CHART_LABEL_KEYS:
            _add_pattern(rule, 'chart_labels', CHART_LABEL_KEYS[key], value)

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


def label_patterns(text: str) -> Dict[str, List[Tuple[bool, str]]]:
    """Split a rule's label patterns into ``{label: [(negative, glob), ...]}`` in match order.

    Follows health_prototypes.c (``simple_pattern_trim_around_equal``, ``health_config_add_key_to_values``): spaces
    around `=` are dropped, words are separated by spaces, `name=` starts a label and the words after it belong to that
    label until the next `name=`.
    """
    patterns: Dict[str, List[Tuple[bool, str]]] = {}
    label = ''
    for word in re.sub(r' ?= ?', '=', text).split():
        if '=' in word:
            label, word = word.split('=', 1)
        if word:
            negative = word.startswith('!')
            patterns.setdefault(label, []).append((negative, word[1:] if negative else word))
    return patterns


def _glob_matches(glob: str, value: str) -> bool:
    return re.fullmatch('.*'.join(re.escape(part) for part in glob.split('*')), value) is not None


def label_admits(patterns: Sequence[Tuple[bool, str]], value: str) -> bool:
    """The first pattern that matches decides (``pattern_array_label_match``); no match rejects."""
    for negative, glob in patterns:
        if _glob_matches(glob, value):
            return not negative
    return False


def rule_admits(rule: dict, plugin_name: str, module_name: str) -> bool:
    """Whether a rule can attach to charts of this plugin and module.

    The `_os` host-label patterns must admit a platform the plugin runs on (``PLUGIN_PLATFORMS``). The
    `_collect_plugin` and `_collect_module` chart-label patterns, when present, must admit the plugin and module: C
    plugins report their full name (`proc.plugin`), Go plugins drop the `.plugin` suffix (`go.d`), so both forms are
    tried. Other labels select instances, hosts or values and do not change ownership.
    """
    host = label_patterns(rule.get('host_labels', ''))
    if '_os' in host:
        platforms = PLUGIN_PLATFORMS.get(plugin_name, PLATFORMS)
        if not any(label_admits(host['_os'], platform) for platform in platforms):
            return False

    chart = label_patterns(rule.get('chart_labels', ''))
    plugin_values = {plugin_name, plugin_name.removesuffix('.plugin')}
    if '_collect_plugin' in chart and not any(label_admits(chart['_collect_plugin'], v) for v in plugin_values):
        return False
    if '_collect_module' in chart and not label_admits(chart['_collect_module'], module_name):
        return False
    return True


def agent_internal_alerts(rules: Iterable[dict]) -> List[str]:
    """Names of alerts whose every target is an Agent chart."""
    return sorted(
        name for name, group in rules_by_name(rules).items()
        if all(rule.get('on', '').startswith(AGENT_INTERNAL_PREFIXES) for rule in group)
    )


def alert_subject(subject_id: str, plugin_name: str, module_name: str, contexts: Iterable[str] = (),
                  names: Iterable[str] = (), excluded: Iterable[str] = ()) -> dict:
    """An integration as alert ownership sees it: the contexts it declares, the alert names it lists and the derived
    alert names it excludes because its monitored instance lacks the data they evaluate."""
    return {
        'id': subject_id,
        'plugin_name': plugin_name,
        'module_name': module_name,
        'contexts': frozenset(contexts),
        'names': tuple(names),
        'excluded': frozenset(excluded),
    }


def _admits_subject(rule: dict, subject: dict) -> bool:
    return rule_admits(rule, subject['plugin_name'], subject['module_name'])


def derived_rules(rules: Iterable[dict], subject: dict) -> List[dict]:
    """The rules a subject owns through its declared contexts, in configuration order."""
    return [rule for rule in rules if rule.get('on') in subject['contexts'] and _admits_subject(rule, subject)]


def alert_rows(rules: Sequence[dict], subject: dict, internal: Iterable[str] = ()) -> List[dict]:
    """The page rows of one subject, in configuration order: the rules on its declared contexts and the rules of the
    alert names it lists, each admitted by the rule's platform and plugin filters, without the names it excludes.

    One row per alert name and target; Agent-internal alerts are never owned.
    """
    names = set(subject['names'])
    internal = set(internal)
    rows, seen = [], set()

    for rule in rules:
        if rule['name'] in internal or rule['name'] in subject['excluded'] or not _admits_subject(rule, subject):
            continue
        if rule.get('on') not in subject['contexts'] and rule['name'] not in names:
            continue
        key = (rule['name'], rule.get('on'))
        if key in seen:
            continue
        seen.add(key)
        rows.append({
            'name': rule['name'],
            'link': conf_link(rule['source']),
            'metric': rule.get('on', ''),
            'info': rule.get('info', ''),
        })

    return rows


def alert_ownership(rules: Sequence[dict], subjects: Iterable[dict]) -> Dict[str, List[dict]]:
    """Page rows per subject id; the single source of both the pages and the export's owners."""
    internal = agent_internal_alerts(rules)
    return {subject['id']: alert_rows(rules, subject, internal) for subject in subjects}


def alert_owners(ownership: Mapping[str, Iterable[dict]]) -> Dict[str, set]:
    owners: Dict[str, set] = {}
    for subject_id, rows in ownership.items():
        for row in rows:
            owners.setdefault(row['name'], set()).add(subject_id)
    return owners


def check_listed_names(rules: Sequence[dict], subjects: Iterable[dict]) -> List[str]:
    """Problems with alert names an integration lists or excludes: a listed name is a stock, non-Agent alert that its
    contexts cannot derive and whose platform and plugin filters admit the integration; an excluded name is one its
    contexts derive and it does not list."""
    grouped = rules_by_name(rules)
    internal = set(agent_internal_alerts(rules))
    problems = []

    for subject in subjects:
        derivable = {rule['name'] for rule in derived_rules(rules, subject)}
        for name in subject['names']:
            where = f'{subject["id"]}: alert {name!r}'
            if name not in grouped:
                problems.append(f'{where} is not defined in {HEALTH_CONFIG_DIR.relative_to(REPO_PATH)}')
            elif name in internal:
                problems.append(f'{where} targets only Agent charts')
            elif name in derivable:
                problems.append(f'{where} is derived from a declared context; remove it from the alerts list')
            elif not any(_admits_subject(rule, subject) for rule in grouped[name]):
                problems.append(f'{where} excludes this plugin or its platforms')
        for name in sorted(subject['excluded']):
            where = f'{subject["id"]}: excluded alert {name!r}'
            if name in subject['names']:
                problems.append(f'{where} is also listed')
            elif name not in derivable:
                problems.append(f'{where} is not derived from a declared context; remove it from excluded_alerts')

    return problems


def _plurals(word: str) -> Tuple[str, ...]:
    """English plural endings: -s, -es, and -ies for a consonant followed by y."""
    plurals = (word + 's', word + 'es')
    if len(word) > 1 and word.endswith('y') and word[-2] not in 'aeiou':
        plurals += (word[:-1] + 'ies',)
    return plurals


def _components_collide(first: str, second: str) -> bool:
    first, second = first.casefold(), second.casefold()
    return first == second or first in _plurals(second) or second in _plurals(first)


def check_classification(rules: Iterable[dict]) -> List[str]:
    """Problems with the ``class``, ``type`` and ``component`` of stock rules.

    Every rule sets all three (an empty or whitespace-only value counts as missing); class and type come from the
    vocabulary; one component is never spelled two ways that differ only in case or a plural ending.
    """
    problems = []
    spellings: Dict[str, str] = {}

    for rule in rules:
        where = f'{rule.get("source", "?")}: {rule["name"]}'
        values = {field: (rule.get(field) or '').strip() for field in CLASSIFICATION_FIELDS}
        for field, value in values.items():
            if not value:
                problems.append(f'{where}: no {field}')
        if values['class'] and values['class'] not in ALERT_CLASSES:
            problems.append(f'{where}: class {values["class"]!r} is not one of {", ".join(ALERT_CLASSES)}')
        if values['type'] and values['type'] not in ALERT_TYPES:
            problems.append(f'{where}: type {values["type"]!r} is not one of {", ".join(ALERT_TYPES)}')
        component = values['component']
        if not component or component in spellings:
            continue
        for seen, seen_where in spellings.items():
            if _components_collide(component, seen):
                problems.append(f'{where}: component {component!r} collides with {seen!r} ({seen_where})')
        spellings[component] = where

    return problems
