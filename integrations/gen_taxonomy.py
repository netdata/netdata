#!/usr/bin/env python3

import argparse
import bisect
import json
import subprocess
import sys
import unicodedata
from dataclasses import dataclass
from pathlib import Path

from _common import (
    COLLECTOR_SOURCES,
    GITHUB_ACTIONS,
    INTEGRATIONS_PATH,
    REPO_PATH,
    WARNINGS,
    load_collectors,
    load_yaml,
    make_id,
    make_validator,
)

TAXONOMY_PATH = INTEGRATIONS_PATH / 'taxonomy'
SECTIONS_PATH = TAXONOMY_PATH / 'sections.yaml'
ICONS_PATH = TAXONOMY_PATH / 'icons.yaml'
OUTPUT_PATH = INTEGRATIONS_PATH / 'taxonomy.json'

SECTIONS_VALIDATOR = make_validator('./taxonomy_sections.json#')
COLLECTOR_TAXONOMY_VALIDATOR = make_validator('./taxonomy_collector.json#')
OUTPUT_VALIDATOR = make_validator('./taxonomy_output.json#')

FATAL = 'fatal'
WARNING = 'warning'

DISPLAY_KEYS = (
    'title',
    'short_name',
    'icon',
    'priority',
    'families',
    'tooltip',
    'menu_pattern',
    'hide_sub_icon',
    'force_visibility',
    'fallback_icon',
    'include_grand_parents',
    'properties',
)

WIDGET_KEYS = (
    'chart_library',
    'group_by',
    'group_by_label',
    'aggregation_method',
    'selected_dimensions',
    'dimensions_sort',
    'colors',
    'layout',
    'table_columns',
    'table_sort_by',
    'labels',
    'value_range',
    'eliminate_zero_dimensions',
    'context_items',
    'post_group_by',
    'show_post_aggregations',
    'grouping_method',
    'sparkline',
    'renderer',
)

SELECTOR_KEYS = ('context_prefix', 'context_prefix_exclude', 'collect_plugin')
SORTED_LIST_KEYS = set(SELECTOR_KEYS)

ITEM_COPY_KEYS = {
    'owned_context': ('context', *DISPLAY_KEYS, 'single_node'),
    'group': ('id', *DISPLAY_KEYS, 'section_filters', 'dyncfg', 'single_node'),
    'flatten': ('id', *DISPLAY_KEYS, 'single_node'),
    'selector': ('id', *DISPLAY_KEYS, *SELECTOR_KEYS, 'single_node'),
    'context': ('id', *DISPLAY_KEYS, 'contexts', *WIDGET_KEYS, 'single_node'),
    'grid': ('id', *DISPLAY_KEYS, 'renderer', 'single_node'),
    'first_available': ('id', *DISPLAY_KEYS, 'single_node'),
    'view_switch': ('id',),
}

PLACEMENT_COPY_KEYS = (
    'short_name',
    'icon',
    'priority',
    'families',
    'tooltip',
    'menu_pattern',
    'hide_sub_icon',
    'force_visibility',
    'fallback_icon',
    'include_grand_parents',
    'properties',
    'single_node',
)


@dataclass(frozen=True)
class Finding:
    code: str
    severity: str
    path: Path
    message: str
    line: int | None = None

    def render(self):
        location = str(self.path)
        if GITHUB_ACTIONS:
            line = f',line={self.line}' if self.line else ''
            level = 'error' if self.severity == FATAL else 'warning'
            return f'::{level} file={location}{line},title={self.code}::{self.message}'

        line = f':{self.line}' if self.line else ''
        return f'{location}{line}: {self.severity.upper()} {self.code}: {self.message}'


def relpath(path):
    try:
        return path.relative_to(REPO_PATH).as_posix()
    except ValueError:
        return path.as_posix()


def normalize_title(value):
    normalized = unicodedata.normalize('NFC', value or '')
    return normalized.casefold()


def path_segment(section):
    return section['id'].rsplit('.', 1)[-1]


def run_git(*args):
    try:
        return subprocess.check_output(
            ['git', '-C', str(REPO_PATH), *args],
            text=True,
            stderr=subprocess.DEVNULL,
        ).strip()
    except (subprocess.CalledProcessError, FileNotFoundError):
        return 'unknown'


def source_info():
    return {
        'netdata_commit': run_git('rev-parse', 'HEAD'),
        'generated_at': run_git('log', '-1', '--format=%cI'),
    }


def discover_taxonomy_files():
    files = []
    for _, root, recursive in COLLECTOR_SOURCES:
        if root.exists() and root.is_dir() and recursive:
            files.extend(root.glob('*/taxonomy.yaml'))
        elif root.exists() and root.is_file() and root.name == 'taxonomy.yaml':
            files.append(root)
    return sorted(set(files), key=lambda p: relpath(p))


def validate_schema(validator, data, path, default_code, findings):
    valid = True
    for error in sorted(validator.iter_errors(data), key=lambda e: list(e.absolute_path)):
        valid = False
        code = default_code
        absolute_path = [str(part) for part in error.absolute_path]
        if 'single_node' in absolute_path:
            code = 'TAX021'
        elif error.validator == 'additionalProperties' and 'section_path' in error.message:
            code = 'TAX028'
        findings.append(Finding(
            code=code,
            severity=FATAL,
            path=path,
            message=f'{error.message} (schema path: {"/".join(str(p) for p in error.absolute_schema_path)})',
        ))
    return valid


def load_icons(findings):
    data = load_yaml(ICONS_PATH)
    if not data:
        findings.append(Finding('TAX001', FATAL, ICONS_PATH, 'Unable to load taxonomy icon registry.'))
        return set()
    icons = data.get('icons', [])
    seen = set()
    for icon in icons:
        if icon in seen:
            findings.append(Finding('TAX028', FATAL, ICONS_PATH, f'Duplicate icon id: {icon}'))
        seen.add(icon)
    return seen


def load_sections(findings, icons):
    data = load_yaml(SECTIONS_PATH)
    if not data:
        findings.append(Finding('TAX001', FATAL, SECTIONS_PATH, 'Unable to load taxonomy sections registry.'))
        return [], {}

    if not validate_schema(SECTIONS_VALIDATOR, data, SECTIONS_PATH, 'TAX001', findings):
        return [], {}

    sections = data['sections']
    by_id = {}
    for section in sections:
        section_id = section['id']
        if section_id in by_id:
            findings.append(Finding('TAX006', FATAL, SECTIONS_PATH, f'Duplicate section id: {section_id}'))
        by_id[section_id] = section

    for section in sections:
        icon = section.get('icon')
        if icon and icon not in icons:
            findings.append(Finding('TAX028', FATAL, SECTIONS_PATH, f'Section {section["id"]} references unknown icon: {icon}'))

        parent_id = section.get('parent_id')
        if parent_id and parent_id not in by_id:
            findings.append(Finding('TAX028', FATAL, SECTIONS_PATH, f'Section {section["id"]} references unknown parent_id: {parent_id}'))

        deprecation = section.get('deprecation', {})
        replacement_id = deprecation.get('replacement_id')
        if replacement_id and replacement_id not in by_id:
            findings.append(Finding('TAX028', FATAL, SECTIONS_PATH, f'Section {section["id"]} references unknown replacement_id: {replacement_id}'))

    paths = {}

    def resolve_path(section_id, visiting):
        if section_id in paths:
            return paths[section_id]
        if section_id in visiting:
            findings.append(Finding('TAX028', FATAL, SECTIONS_PATH, f'Section parent cycle includes: {section_id}'))
            return section_id
        section = by_id[section_id]
        parent_id = section.get('parent_id')
        if not parent_id:
            paths[section_id] = section_id
            return section_id
        paths[section_id] = f'{resolve_path(parent_id, visiting | {section_id})}.{path_segment(section)}'
        return paths[section_id]

    for section_id in by_id:
        resolve_path(section_id, set())

    emitted = []
    for section in sorted(sections, key=lambda s: (s['section_order'], normalize_title(s['title']), s['id'])):
        item = {
            'id': section['id'],
            'path': paths[section['id']],
            'title': section['title'],
            'section_order': section['section_order'],
            'status': section['status'],
        }
        for key in ('parent_id', 'short_name', 'icon', 'deprecation'):
            if key in section:
                item[key] = section[key]
        extras = {k: v for k, v in section.items() if k.startswith('x_')}
        if extras:
            item['_extra'] = extras
        emitted.append(item)

    return emitted, {section_id: (section, paths[section_id]) for section_id, section in by_id.items()}


def module_contexts(module):
    contexts = []
    for scope in module.get('metrics', {}).get('scopes', []):
        for metric in scope.get('metrics', []):
            name = metric.get('name')
            if name and name not in contexts:
                contexts.append(name)
    return contexts


def dynamic_declarations(module):
    metrics = module.get('metrics', {})
    prefixes = {item['prefix'] for item in metrics.get('dynamic_context_prefixes', [])}
    plugins = {item['plugin'] for item in metrics.get('dynamic_collect_plugins', [])}
    return prefixes, plugins


def build_metadata_indexes(findings):
    warning_start = len(WARNINGS)
    modules = load_collectors()
    for path, message in WARNINGS[warning_start:]:
        findings.append(Finding('TAX001', FATAL, Path(path), message))

    by_path_module = {}
    context_to_modules = {}
    contexts_by_plugin = {}
    all_contexts = set()

    for module in modules:
        src_path = Path(module['_src_path'])
        meta = module['meta']
        key = (src_path, meta['plugin_name'], meta['module_name'])
        by_path_module.setdefault(key, []).append(module)

        contexts = module_contexts(module)
        contexts_by_plugin.setdefault(meta['plugin_name'], set()).update(contexts)
        for context in contexts:
            all_contexts.add(context)
            context_to_modules.setdefault(context, set()).add(key)

    return {
        'modules': modules,
        'by_path_module': by_path_module,
        'context_to_modules': context_to_modules,
        'contexts_by_plugin': contexts_by_plugin,
        'all_contexts': sorted(all_contexts),
    }


def prescan_removed_shapes(data, path, findings):
    def walk(node):
        if isinstance(node, dict):
            if 'multi_node' in node and node.get('type') != 'view_switch':
                findings.append(Finding('TAX022', FATAL, path, '`multi_node:` is accepted only inside `type: view_switch`.'))
            for key, value in node.items():
                if key.endswith('_extend'):
                    findings.append(Finding('TAX023', FATAL, path, f'List-merge field `{key}:` is not accepted in taxonomy v1.'))
                walk(value)
        elif isinstance(node, list):
            for item in node:
                walk(item)

    walk(data)


def collector_ids(modules):
    ids = []
    for module in modules:
        ids.append(make_id(module['meta']))
    return sorted(ids)


def merged_module_contexts(modules):
    contexts = set()
    for module in modules:
        contexts.update(module_contexts(module))
    return contexts


def merged_dynamic_declarations(modules, inline):
    prefixes = set()
    plugins = set()
    for module in modules:
        module_prefixes, module_plugins = dynamic_declarations(module)
        prefixes.update(module_prefixes)
        plugins.update(module_plugins)

    if inline:
        prefixes.update(item['prefix'] for item in inline.get('dynamic_context_prefixes', []))
        plugins.update(item['plugin'] for item in inline.get('dynamic_collect_plugins', []))

    return prefixes, plugins


def resolve_prefix(prefix, all_contexts):
    start = bisect.bisect_left(all_contexts, prefix)
    stop = bisect.bisect_left(all_contexts, prefix + chr(0x10ffff))
    return all_contexts[start:stop]


def is_context_prefix_declared(prefix, allowed_prefixes):
    return any(prefix.startswith(allowed) for allowed in allowed_prefixes)


def ordered_union(*sequences):
    seen = set()
    result = []
    for sequence in sequences:
        for item in sequence:
            if item not in seen:
                seen.add(item)
                result.append(item)
    return result


def ordered_dict_union(*sequences):
    seen = set()
    result = []
    for sequence in sequences:
        for item in sequence:
            key = json.dumps(item, sort_keys=True)
            if key not in seen:
                seen.add(key)
                result.append(item)
    return result


def node_label(node, fallback):
    if isinstance(node, str):
        return node
    return node.get('id') or node.get('context') or node.get('title') or fallback


def validate_icons(node, path, icons, findings, label):
    for key in ('icon', 'fallback_icon'):
        icon = node.get(key)
        if icon and icon not in icons:
            findings.append(Finding('TAX028', FATAL, path, f'Item `{label}` references unknown {key}: {icon}'))


def validate_override(parent, path, findings):
    if parent.get('type') == 'view_switch':
        return
    single_node = parent.get('single_node')
    if single_node is None:
        return
    if not single_node:
        findings.append(Finding('TAX024', WARNING, path, 'Empty `single_node:` block is equivalent to omitting it.'))
        return
    for key, value in single_node.items():
        if parent.get(key) == value:
            findings.append(Finding('TAX025', WARNING, path, f'`single_node.{key}` is identical to the top-level value.'))


def copy_fields(node, output, keys):
    for key in keys:
        if key in node:
            value = node[key]
            if key in SORTED_LIST_KEYS:
                value = sorted(value)
            output[key] = value


def emit_extra(node, output):
    extras = {k: v for k, v in node.items() if k.startswith('x_')}
    if extras:
        output['_extra'] = extras


def resolve_selectors(node, known_contexts, allowed_prefixes, allowed_plugins, metadata_indexes, path, findings):
    resolved = set()
    explicit = node.get('contexts', [])
    prefixes = node.get('context_prefix', [])
    excludes = node.get('context_prefix_exclude', [])
    collect_plugins = node.get('collect_plugin', [])

    for context in explicit:
        if context not in known_contexts:
            findings.append(Finding('TAX003', FATAL, path, f'Unknown context for this collector: {context}'))
        resolved.add(context)

    if excludes and not prefixes:
        findings.append(Finding('TAX029', FATAL, path, '`context_prefix_exclude:` requires `context_prefix:` on the same node.'))

    for prefix in prefixes:
        if not is_context_prefix_declared(prefix, allowed_prefixes):
            findings.append(Finding('TAX031', FATAL, path, f'context_prefix `{prefix}` is not declared in metadata.yaml metrics.dynamic_context_prefixes.'))
        for context in resolve_prefix(prefix, metadata_indexes['all_contexts']):
            resolved.add(context)

    for exclude in excludes if prefixes else []:
        if not any(exclude.startswith(prefix) for prefix in prefixes):
            findings.append(Finding('TAX029', FATAL, path, f'context_prefix_exclude `{exclude}` is not covered by context_prefix.'))
        for context in list(resolved):
            if context.startswith(exclude):
                resolved.remove(context)

    for plugin in collect_plugins:
        if plugin not in allowed_plugins:
            findings.append(Finding('TAX035', FATAL, path, f'collect_plugin `{plugin}` is not declared in metadata.yaml metrics.dynamic_collect_plugins.'))
        resolved.update(metadata_indexes['contexts_by_plugin'].get(plugin, set()))

    for context in explicit:
        if any(context.startswith(prefix) for prefix in prefixes):
            findings.append(Finding('TAX034', WARNING, path, f'Context `{context}` is redundant because it is covered by context_prefix.'))

    return sorted(resolved)


def resolve_node_contexts(node, known_contexts, allowed_prefixes, allowed_plugins, metadata_indexes, path, findings):
    return resolve_selectors(node, known_contexts, allowed_prefixes, allowed_plugins, metadata_indexes, path, findings)


def validate_literal_context(context, known_contexts, unresolved, path, findings):
    known = context in known_contexts
    if unresolved:
        if known:
            findings.append(Finding('TAX038', WARNING, path, f'Unresolved escape hatch is stale because context now exists: {context}'))
    elif not known:
        findings.append(Finding('TAX003', FATAL, path, f'Unknown context for this collector: {context}'))
    return known


def resolve_context_references(
        refs,
        known_contexts,
        allowed_prefixes,
        allowed_plugins,
        metadata_indexes,
        path,
        findings,
        referenced_literals,
        item_path):
    referenced = []
    unresolved_references = []
    for ref in refs:
        if isinstance(ref, str):
            known = validate_literal_context(ref, known_contexts, unresolved=False, path=path, findings=findings)
            referenced = ordered_union(referenced, [ref])
            referenced_literals.append((ref, item_path, path, False, known))
            continue

        if 'context' in ref:
            context = ref['context']
            known = validate_literal_context(context, known_contexts, unresolved=True, path=path, findings=findings)
            referenced = ordered_union(referenced, [context])
            referenced_literals.append((context, item_path, path, True, known))
            unresolved_references.append({
                'context': context,
                'reason': ref['unresolved']['reason'],
                'owner': ref['unresolved']['owner'],
                'expires': ref['unresolved']['expires'],
                'item_path': item_path,
            })
            continue

        resolved = resolve_selectors(ref, known_contexts, allowed_prefixes, allowed_plugins, metadata_indexes, path, findings)
        referenced = ordered_union(referenced, resolved)

    return referenced, unresolved_references


def register_ownership(contexts, ownership, current, owner_kind, path, ownership_conflicts):
    for context in contexts:
        previous = ownership.get(context)
        if previous and previous['owner'] != current:
            code = 'TAX036' if 'selector' in {previous['kind'], owner_kind} else 'TAX033'
            owners = tuple(sorted([previous['owner'], current]))
            ownership_conflicts[(code, context, owners)] = {
                'path': path,
            }
        ownership[context] = {
            'owner': current,
            'kind': owner_kind,
        }


def emit_ownership_conflicts(ownership_conflicts, findings):
    for code, context, owners in sorted(ownership_conflicts):
        findings.append(Finding(
            code,
            FATAL,
            ownership_conflicts[(code, context, owners)]['path'],
            f'Context `{context}` is owned by both {owners[0]} and {owners[1]}.',
        ))


def emit_item(
        node,
        position,
        known_contexts,
        allowed_prefixes,
        allowed_plugins,
        metadata_indexes,
        icons,
        ownership,
        ownership_conflicts,
        referenced_literals,
        owner_label,
        path,
        findings,
        index):
    if isinstance(node, str):
        label = f'{owner_label}.{node}'
        validate_literal_context(node, known_contexts, unresolved=False, path=path, findings=findings)
        register_ownership([node], ownership, f'{relpath(path)}:{label}', 'literal', path, ownership_conflicts)
        return {
            'type': 'owned_context',
            'context': node,
            'resolved_contexts': [node],
            'referenced_contexts': [],
            'unresolved_references': [],
        }

    kind = node['type']
    label = f'{owner_label}.{node_label(node, str(index))}'
    validate_icons(node, path, icons, findings, label)
    validate_override(node, path, findings)

    output = {'type': kind}
    copy_fields(node, output, ITEM_COPY_KEYS[kind])
    emit_extra(node, output)

    resolved_contexts = []
    referenced_contexts = []
    unresolved_references = []

    if kind == 'owned_context':
        context = node['context']
        validate_literal_context(context, known_contexts, unresolved=False, path=path, findings=findings)
        resolved_contexts = [context]
        register_ownership(resolved_contexts, ownership, f'{relpath(path)}:{label}', 'literal', path, ownership_conflicts)

    elif kind == 'selector':
        resolved_contexts = resolve_selectors(node, known_contexts, allowed_prefixes, allowed_plugins, metadata_indexes, path, findings)
        register_ownership(resolved_contexts, ownership, f'{relpath(path)}:{label}', 'selector', path, ownership_conflicts)

    elif kind in ('group', 'flatten'):
        children = []
        for child_index, child in enumerate(node.get('items', [])):
            emitted = emit_item(
                child,
                'structural',
                known_contexts,
                allowed_prefixes,
                allowed_plugins,
                metadata_indexes,
                icons,
                ownership,
                ownership_conflicts,
                referenced_literals,
                label,
                path,
                findings,
                child_index,
            )
            children.append(emitted)
            resolved_contexts = ordered_union(resolved_contexts, emitted['resolved_contexts'])
            referenced_contexts = ordered_union(referenced_contexts, emitted['referenced_contexts'])
            unresolved_references = ordered_dict_union(unresolved_references, emitted['unresolved_references'])
        output['items'] = children

    elif kind == 'context':
        referenced_contexts, unresolved_references = resolve_context_references(
            node['contexts'],
            known_contexts,
            allowed_prefixes,
            allowed_plugins,
            metadata_indexes,
            path,
            findings,
            referenced_literals,
            label,
        )

    elif kind == 'grid':
        children = []
        for child_index, child in enumerate(node.get('items', [])):
            emitted = emit_item(
                child,
                'display',
                known_contexts,
                allowed_prefixes,
                allowed_plugins,
                metadata_indexes,
                icons,
                ownership,
                ownership_conflicts,
                referenced_literals,
                label,
                path,
                findings,
                child_index,
            )
            children.append(emitted)
            resolved_contexts = ordered_union(resolved_contexts, emitted['resolved_contexts'])
            referenced_contexts = ordered_union(referenced_contexts, emitted['referenced_contexts'])
            unresolved_references = ordered_dict_union(unresolved_references, emitted['unresolved_references'])
        output['items'] = children

    elif kind == 'first_available':
        children = []
        for child_index, child in enumerate(node.get('items', [])):
            emitted = emit_item(
                child,
                'display',
                known_contexts,
                allowed_prefixes,
                allowed_plugins,
                metadata_indexes,
                icons,
                ownership,
                ownership_conflicts,
                referenced_literals,
                label,
                path,
                findings,
                child_index,
            )
            children.append(emitted)
            resolved_contexts = ordered_union(resolved_contexts, emitted['resolved_contexts'])
            referenced_contexts = ordered_union(referenced_contexts, emitted['referenced_contexts'])
            unresolved_references = ordered_dict_union(unresolved_references, emitted['unresolved_references'])
        output['items'] = children

    elif kind == 'view_switch':
        branch_position = 'structural' if position == 'structural' else 'display'
        for branch in ('multi_node', 'single_node'):
            emitted = emit_item(
                node[branch],
                branch_position,
                known_contexts,
                allowed_prefixes,
                allowed_plugins,
                metadata_indexes,
                icons,
                ownership,
                ownership_conflicts,
                referenced_literals,
                f'{label}.{branch}',
                path,
                findings,
                0,
            )
            output[branch] = emitted
            resolved_contexts = ordered_union(resolved_contexts, emitted['resolved_contexts'])
            referenced_contexts = ordered_union(referenced_contexts, emitted['referenced_contexts'])
            unresolved_references = ordered_dict_union(unresolved_references, emitted['unresolved_references'])

    if position != 'structural' and resolved_contexts:
        findings.append(Finding('TAX001', FATAL, path, f'Item `{label}` owns contexts from a display-only position.'))

    output['resolved_contexts'] = resolved_contexts
    output['referenced_contexts'] = referenced_contexts
    output['unresolved_references'] = unresolved_references
    return output


def emit_referenced_only_findings(referenced_literals, ownership, findings):
    emitted = set()
    rows = sorted(referenced_literals, key=lambda row: (row[0], row[1], relpath(row[2])))
    for context, item_path, path, unresolved, known in rows:
        if unresolved or not known or context in ownership:
            continue
        key = (context, item_path, path)
        if key in emitted:
            continue
        emitted.add(key)
        findings.append(Finding(
            'TAX037',
            FATAL,
            path,
            f'Context `{context}` is referenced by widget `{item_path}` but is not owned by any taxonomy item.',
        ))


def process_taxonomy_file(path, sections, icons, metadata_indexes, ownership, findings, referenced_literals=None, ownership_conflicts=None):
    if referenced_literals is None:
        referenced_literals = []
    if ownership_conflicts is None:
        ownership_conflicts = {}

    data = load_yaml(path)
    if not data:
        findings.append(Finding('TAX001', FATAL, path, 'Unable to load taxonomy file.'))
        return [], []

    prescan_removed_shapes(data, path, findings)
    if not validate_schema(COLLECTOR_TAXONOMY_VALIDATOR, data, path, 'TAX001', findings):
        return [], []

    metadata_path = path.with_name('metadata.yaml')
    identity = (metadata_path, data['plugin_name'], data['module_name'])
    modules = metadata_indexes['by_path_module'].get(identity, [])
    inline = data.get('inline_dynamic_declarations')

    if modules and inline:
        findings.append(Finding('TAX029', FATAL, path, '`inline_dynamic_declarations:` is allowed only for collectors without metadata.yaml.'))

    known_contexts = merged_module_contexts(modules)
    allowed_prefixes, allowed_plugins = merged_dynamic_declarations(modules, inline)
    ids = collector_ids(modules) if modules else [f'{data["plugin_name"]}-{data["module_name"]}']

    if 'taxonomy_optout' in data:
        return [], [{
            'collector_ids': ids,
            'plugin_name': data['plugin_name'],
            'module_name': data['module_name'],
            'source_path': relpath(path),
            'reason': data['taxonomy_optout']['reason'],
        }]

    if not modules and not inline:
        findings.append(Finding('TAX001', FATAL, path, 'No matching metadata.yaml module found and no inline_dynamic_declarations provided.'))

    emitted = []
    placement_keys = set()

    for placement in data['placements']:
        placement_key = (placement['section_id'], placement['id'])
        if placement_key in placement_keys:
            findings.append(Finding('TAX006', FATAL, path, f'Duplicate placement in collector taxonomy: {placement["section_id"]}.{placement["id"]}'))
        placement_keys.add(placement_key)

        section = sections.get(placement['section_id'])
        if not section:
            findings.append(Finding('TAX028', FATAL, path, f'Unknown section_id: {placement["section_id"]}'))
            section_path = placement['section_id']
        else:
            section_entry, section_path = section
            if section_entry['status'] == 'deprecated':
                findings.append(Finding('TAX028', FATAL, path, f'New placements cannot target deprecated section_id: {placement["section_id"]}'))

        validate_icons(placement, path, icons, findings, placement['id'])
        validate_override(placement, path, findings)

        items = []
        resolved_contexts = []
        referenced_contexts = []
        unresolved_references = []
        for index, child in enumerate(placement['items']):
            emitted_child = emit_item(
                child,
                'structural',
                known_contexts,
                allowed_prefixes,
                allowed_plugins,
                metadata_indexes,
                icons,
                ownership,
                ownership_conflicts,
                referenced_literals,
                placement['id'],
                path,
                findings,
                index,
            )
            items.append(emitted_child)
            resolved_contexts = ordered_union(resolved_contexts, emitted_child['resolved_contexts'])
            referenced_contexts = ordered_union(referenced_contexts, emitted_child['referenced_contexts'])
            unresolved_references = ordered_dict_union(unresolved_references, emitted_child['unresolved_references'])

        item = {
            'collector_ids': ids,
            'plugin_name': data['plugin_name'],
            'module_name': data['module_name'],
            'source_path': relpath(path),
            'id': placement['id'],
            'section_id': placement['section_id'],
            'section_path': section_path,
            'title': placement['title'],
            'items': items,
            'resolved_contexts': resolved_contexts,
            'referenced_contexts': referenced_contexts,
            'unresolved_references': unresolved_references,
        }
        copy_fields(placement, item, PLACEMENT_COPY_KEYS)
        emit_extra(placement, item)
        emitted.append(item)

    return emitted, []


def build_taxonomy():
    findings = []
    icons = load_icons(findings)
    section_entries, sections = load_sections(findings, icons)
    metadata_indexes = build_metadata_indexes(findings)
    ownership = {}
    ownership_conflicts = {}
    referenced_literals = []
    placements = []
    opted_out_collectors = []

    for path in discover_taxonomy_files():
        new_placements, new_optouts = process_taxonomy_file(path, sections, icons, metadata_indexes, ownership, findings, referenced_literals, ownership_conflicts)
        placements.extend(new_placements)
        opted_out_collectors.extend(new_optouts)

    emit_ownership_conflicts(ownership_conflicts, findings)
    emit_referenced_only_findings(referenced_literals, ownership, findings)

    placements.sort(key=lambda item: (
        sections.get(item['section_id'], ({'section_order': 100000}, item['section_path']))[0]['section_order'],
        item.get('priority', 1000),
        normalize_title(item['title']),
        item['id'],
        item['source_path'],
    ))

    taxonomy = {
        'taxonomy_schema_version': 1,
        'source': source_info(),
        'sections': section_entries,
        'placements': placements,
        'opted_out_collectors': sorted(opted_out_collectors, key=lambda item: (item['plugin_name'], item['module_name'], item['source_path'])),
    }

    validate_schema(OUTPUT_VALIDATOR, taxonomy, OUTPUT_PATH, 'TAX001', findings)
    return taxonomy, findings


def write_json(path, data):
    path.write_text(json.dumps(data, indent=2, sort_keys=True) + '\n')


def main():
    parser = argparse.ArgumentParser(description='Generate Netdata collector taxonomy artifact.')
    parser.add_argument('--check-only', action='store_true', help='Validate taxonomy sources without writing taxonomy.json.')
    parser.add_argument('--output', type=Path, default=OUTPUT_PATH, help='Output JSON path.')
    parser.add_argument('--findings-json', type=Path, help='Optional path for machine-readable findings.')
    args = parser.parse_args()

    taxonomy, findings = build_taxonomy()

    for finding in findings:
        print(finding.render(), file=sys.stderr)

    if args.findings_json:
        write_json(args.findings_json, [
            {
                'code': finding.code,
                'severity': finding.severity,
                'path': relpath(finding.path),
                'line': finding.line,
                'message': finding.message,
            }
            for finding in findings
        ])

    if any(finding.severity == FATAL for finding in findings):
        return 1

    if not args.check_only:
        write_json(args.output, taxonomy)

    return 0


if __name__ == '__main__':
    sys.exit(main())
