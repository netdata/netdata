#!/usr/bin/env python3

from copy import deepcopy
from pathlib import Path

from ruamel.yaml import YAML, YAMLError


INTEGRATIONS_PATH = Path(__file__).resolve().parent
REPO_PATH = INTEGRATIONS_PATH.parent
PROFILE_DESIGN_PATH = (
    REPO_PATH / 'src' / 'go' / 'plugin' / 'go.d' / 'collector' / 'prometheus' / 'profile-proofs'
)
PROFILE_RUNTIME_PATH = (
    REPO_PATH / 'src' / 'go' / 'plugin' / 'go.d' / 'config' / 'go.d' / 'prometheus.profiles' / 'default'
)


class ProfileCoverageError(ValueError):
    pass


def _load_yaml(path):
    yaml = YAML(typ='safe')
    try:
        return yaml.load(path.read_text(encoding='utf-8'))
    except (OSError, UnicodeError, YAMLError) as error:
        raise ProfileCoverageError(f'Could not load {path}: {error}') from error


def _require_text(value, field, profile):
    if not isinstance(value, str) or not value.strip():
        raise ProfileCoverageError(f'Profile {profile!r} requires non-empty {field}.')
    return value.strip()


def _profile_files(path, pattern, key):
    result = {}
    for file in sorted(path.glob(pattern)):
        document = _load_yaml(file)
        profile = document.get(key) if isinstance(document, dict) else None
        if key == '_filename':
            profile = file.stem
        if not isinstance(profile, str) or not profile:
            raise ProfileCoverageError(f'{file} does not declare a profile identity.')
        if profile in result:
            raise ProfileCoverageError(f'Duplicate profile identity {profile!r}: {result[profile][0]} and {file}.')
        result[profile] = (file, document)
    return result


def _chart_dimension(dimension, profile, context):
    selector = _require_text(dimension.get('selector'), f'chart {context!r} dimension selector', profile)
    if dimension.get('name'):
        name = str(dimension['name']).strip()
    elif dimension.get('name_from_label'):
        name = f'values of label {str(dimension["name_from_label"]).strip()}'
    else:
        name = 'matching series'
    return {'name': name, 'selector': selector}


def _runtime_charts(profile, runtime):
    template = runtime.get('template')
    if not isinstance(template, dict):
        raise ProfileCoverageError(f'Profile {profile!r} has no runtime template.')

    root_namespace = _require_text(template.get('context_namespace'), 'template.context_namespace', profile)
    charts = []
    contexts = set()

    def visit(node, family_parts, context_parts):
        family = node.get('family')
        if family is not None and str(family).strip():
            family_parts = [*family_parts, str(family).strip()]
        namespace = node.get('context_namespace')
        if namespace is not None and str(namespace).strip():
            context_parts = [*context_parts, str(namespace).strip()]

        for chart in node.get('charts') or []:
            chart_context = _require_text(chart.get('context'), 'chart context', profile)
            full_context = '.'.join([*context_parts, chart_context])
            prefix = root_namespace + '.'
            if not full_context.startswith(prefix):
                raise ProfileCoverageError(
                    f'Profile {profile!r} chart context {full_context!r} is outside namespace {root_namespace!r}.'
                )
            context = full_context[len(prefix):]
            if context in contexts:
                raise ProfileCoverageError(f'Profile {profile!r} has duplicate runtime chart context {context!r}.')
            contexts.add(context)

            chart_family_parts = list(family_parts)
            if chart.get('family') is not None and str(chart['family']).strip():
                chart_family_parts.append(str(chart['family']).strip())
            family_path = '/'.join(part for part in chart_family_parts if part)
            dimensions = [_chart_dimension(dimension, profile, context) for dimension in chart.get('dimensions') or []]
            if not dimensions:
                raise ProfileCoverageError(f'Profile {profile!r} chart {context!r} has no dimensions.')
            charts.append({
                'context': context,
                'title': _require_text(chart.get('title'), f'chart {context!r} title', profile),
                'family': _require_text(family_path, f'chart {context!r} family', profile),
                'units': _require_text(chart.get('units'), f'chart {context!r} units', profile),
                'dimensions': dimensions,
                'selectors': [dimension['selector'] for dimension in dimensions],
            })

        for group in node.get('groups') or []:
            visit(group, family_parts, context_parts)

    root = dict(template)
    root['context_namespace'] = None
    visit(root, [], [root_namespace])
    return charts


def _family_tree(charts):
    roots = []
    nodes = {}
    for chart in charts:
        parent_key = ()
        for part in chart['family'].split('/'):
            key = (*parent_key, part)
            if key not in nodes:
                node = {
                    'name': part,
                    'path': '/'.join(key),
                    'chart_count': 0,
                    'charts': [],
                    'children': [],
                }
                nodes[key] = node
                if parent_key:
                    nodes[parent_key]['children'].append(node)
                else:
                    roots.append(node)
            nodes[key]['chart_count'] += 1
            parent_key = key
        nodes[parent_key]['charts'].append(chart)
    return roots


def _validate_profile_file_sets(designs, runtimes):
    if set(designs) != set(runtimes):
        missing_design = sorted(set(runtimes) - set(designs))
        missing_runtime = sorted(set(designs) - set(runtimes))
        raise ProfileCoverageError(
            'Profile design/runtime set mismatch: '
            f'missing designs={missing_design}, missing runtimes={missing_runtime}.'
        )


def _validate_profile_identity(profile, design_file, design, runtime_file, runtime):
    if not isinstance(runtime, dict):
        raise ProfileCoverageError(f'Profile {profile!r} runtime document must be a mapping: {runtime_file}.')
    for field in ('match', 'app'):
        if design.get(field) != runtime.get(field):
            raise ProfileCoverageError(
                f'Profile {profile!r} {field} differs between {design_file} and {runtime_file}.'
            )
    namespace = _require_text(design.get('namespace'), 'namespace', profile)
    if namespace != runtime.get('template', {}).get('context_namespace'):
        raise ProfileCoverageError(
            f'Profile {profile!r} namespace differs between {design_file} and {runtime_file}.'
        )


def _join_profile_charts(profile, design, runtime):
    entities = design.get('entities', {})
    views = design.get('views', {})
    runtime_charts = _runtime_charts(profile, runtime)
    runtime_by_context = {chart['context']: chart for chart in runtime_charts}
    missing = sorted(set(views) - set(runtime_by_context))
    extra = sorted(set(runtime_by_context) - set(views))
    if missing or extra:
        raise ProfileCoverageError(
            f'Profile {profile!r} view/chart mismatch: missing charts={missing}, extra charts={extra}.'
        )

    charts = []
    for runtime_chart in runtime_charts:
        context = runtime_chart['context']
        view = views[context]
        if view.get('family') != runtime_chart['family']:
            raise ProfileCoverageError(
                f'Profile {profile!r} view {context!r} family {view.get("family")!r} '
                f'does not match runtime family {runtime_chart["family"]!r}.'
            )
        entity_id = view.get('entity')
        if entity_id not in entities:
            raise ProfileCoverageError(
                f'Profile {profile!r} view {context!r} references unknown entity {entity_id!r}.'
            )
        chart = deepcopy(runtime_chart)
        chart['question'] = _require_text(view.get('question'), f'view {context!r} question', profile)
        chart['entity_scope'] = _require_text(
            entities[entity_id].get('grain'), f'entity {entity_id!r} grain', profile
        )
        charts.append(chart)
    return charts


def _profile_supports(profile, design):
    supports = []
    for support_id, dependency in design.get('composition', {}).get('supports', {}).items():
        supports.append({
            'id': support_id,
            'activation': _require_text(
                dependency.get('activation'), f'composition.supports.{support_id}.activation', profile
            ),
        })
    return supports


def _validate_support_graph(catalog):
    for profile, document in catalog.items():
        for support in document['supports']:
            if support['id'] not in catalog:
                raise ProfileCoverageError(
                    f'Profile {profile!r} references unknown supporting profile {support["id"]!r}.'
                )

    def check_cycles(profile, stack):
        if profile in stack:
            raise ProfileCoverageError(f'Profile support cycle: {" -> ".join([*stack, profile])}.')
        for support in catalog[profile]['supports']:
            check_cycles(support['id'], [*stack, profile])

    for profile in catalog:
        check_cycles(profile, [])


def load_profile_catalog(design_path=PROFILE_DESIGN_PATH, runtime_path=PROFILE_RUNTIME_PATH):
    designs = _profile_files(Path(design_path), '*/PROFILE-DESIGN.yaml', 'profile')
    runtimes = _profile_files(Path(runtime_path), '*.yaml', '_filename')
    _validate_profile_file_sets(designs, runtimes)

    catalog = {}
    for profile in sorted(designs):
        design_file, design = designs[profile]
        runtime_file, runtime = runtimes[profile]
        _validate_profile_identity(profile, design_file, design, runtime_file, runtime)
        charts = _join_profile_charts(profile, design, runtime)
        documentation = design.get('documentation', {})
        catalog[profile] = {
            'id': profile,
            'title': _require_text(documentation.get('title'), 'documentation.title', profile),
            'summary': _require_text(documentation.get('summary'), 'documentation.summary', profile),
            'chart_count': len(charts),
            'families': _family_tree(charts),
            'supports': _profile_supports(profile, design),
        }

    _validate_support_graph(catalog)
    return catalog


def _module_profiles(primary_ids, catalog):
    profiles = []
    seen = set()

    for profile_id in primary_ids:
        if profile_id in seen:
            continue
        profile = deepcopy(catalog[profile_id])
        profile.update({'role': 'primary', 'activation': '', 'supported_by': '', 'open': True})
        profiles.append(profile)
        seen.add(profile_id)

    def add_supports(parent_id):
        for support in catalog[parent_id]['supports']:
            support_id = support['id']
            if support_id in seen:
                continue
            profile = deepcopy(catalog[support_id])
            profile.update({
                'role': 'supporting',
                'activation': support['activation'],
                'supported_by': catalog[parent_id]['title'],
                'open': False,
            })
            profiles.append(profile)
            seen.add(support_id)
            add_supports(support_id)

    for profile_id in primary_ids:
        add_supports(profile_id)
    return profiles


def project_prometheus_profile_coverage(collectors, catalog=None):
    catalog = load_profile_catalog() if catalog is None else catalog
    directly_mapped = set()

    for item in collectors:
        primary_ids = item.pop('_prometheus_profile_ids', None)
        if primary_ids is None:
            continue
        meta = item.get('meta', {})
        if meta.get('plugin_name') != 'go.d.plugin' or meta.get('module_name') != 'prometheus':
            raise ProfileCoverageError(
                f'Profile coverage is allowed only on go.d.plugin/prometheus modules, not {meta.get("id")!r}.'
            )
        unknown = sorted(set(primary_ids) - set(catalog))
        if unknown:
            raise ProfileCoverageError(f'Prometheus module {meta.get("id")!r} maps unknown profiles {unknown}.')
        directly_mapped.update(primary_ids)
        profiles = _module_profiles(primary_ids, catalog)
        # YAML merge anchors can make inherited modules share this mapping in memory.
        item['metrics'] = deepcopy(item['metrics'])
        item['metrics']['profile_coverage'] = {
            'chart_count': sum(profile['chart_count'] for profile in profiles),
            'profiles': profiles,
        }

    missing = sorted(set(catalog) - directly_mapped)
    if missing:
        raise ProfileCoverageError(f'Stock Prometheus profiles without an integration mapping: {missing}.')
    return collectors
