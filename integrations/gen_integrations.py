#!/usr/bin/env python3

import json
import re
import sys
from copy import deepcopy

from jsonschema import ValidationError

from _common import (
    AGENT_REPO,
    INTEGRATIONS_PATH,
    METADATA_PATTERN,
    REPO_PATH,
    debug,
    fail_on_warnings,
    load_collectors,
    load_yaml,
    make_id,
    make_validator,
    warn,
)

TEMPLATE_PATH = INTEGRATIONS_PATH / 'templates'
OUTPUT_PATH = INTEGRATIONS_PATH / 'integrations.js'
JSON_PATH = INTEGRATIONS_PATH / 'integrations.json'
CATEGORIES_FILE = INTEGRATIONS_PATH / 'categories.yaml'
DISTROS_FILE = REPO_PATH / '.github' / 'data' / 'distros.yml'

FLOWS_SOURCES = [
    (AGENT_REPO, REPO_PATH / 'src' / 'crates' / 'netflow-plugin' / 'metadata.yaml', False),
]

DEPLOY_SOURCES = [
    (AGENT_REPO, INTEGRATIONS_PATH / 'deploy.yaml', False),
]

EXPORTER_SOURCES = [
    (AGENT_REPO, REPO_PATH / 'src' / 'exporting', True),
]

AGENT_NOTIFICATION_SOURCES = [
    (AGENT_REPO, REPO_PATH / 'src' / 'health' / 'notifications', True),
]

CLOUD_NOTIFICATION_SOURCES = [
    (AGENT_REPO, INTEGRATIONS_PATH / 'cloud-notifications' / 'metadata.yaml', False),
]

LOGS_SOURCES = [
    (AGENT_REPO, INTEGRATIONS_PATH / 'logs' / 'metadata.yaml', False),
]

AUTHENTICATION_SOURCES = [
    (AGENT_REPO, INTEGRATIONS_PATH / 'cloud-authentication' / 'metadata.yaml', False),
]

SECRETSTORE_SOURCES = [
    (AGENT_REPO, REPO_PATH / 'src' / 'go' / 'plugin' / 'agent' / 'secrets' / 'secretstore' / 'backends', True),
]

SERVICE_DISCOVERY_SOURCES = [
    (AGENT_REPO, REPO_PATH / 'src' / 'go' / 'plugin' / 'go.d' / 'discovery' / 'sdext' / 'discoverer', True),
]

COLLECTOR_RENDER_KEYS = [
    'alerts',
    'metrics',
    'functions',
    'overview',
    'related_resources',
    'setup',
    'troubleshooting',
]

FLOWS_RENDER_KEYS = [
    'alerts',
    'metrics',
    'functions',
    'overview',
    'related_resources',
    'setup',
    'troubleshooting',
]

EXPORTER_RENDER_KEYS = [
    'overview',
    'setup',
    'troubleshooting',
]

AGENT_NOTIFICATION_RENDER_KEYS = [
    'overview',
    'setup',
    'troubleshooting',
]

CLOUD_NOTIFICATION_RENDER_KEYS = [
    'setup',
    'troubleshooting',
]

LOGS_RENDER_KEYS = [
    'overview',
    'setup',
]

AUTHENTICATION_RENDER_KEYS = [
    'overview',
    'setup',
    'troubleshooting',
]

SECRETSTORE_RENDER_KEYS = [
    'overview',
    'setup',
    'collector_configs',
    'troubleshooting',
]

SERVICE_DISCOVERY_RENDER_KEYS = [
    'overview',
    'setup',
    'services',
    'verify',
    'troubleshooting',
]

CUSTOM_TAG_PATTERN = re.compile('\\{% if .*?%\\}.*?\\{% /if %\\}|\\{%.*?%\\}', flags=re.DOTALL)
FIXUP_BLANK_PATTERN = re.compile('\\\\\\n *\\n')

CATEGORY_VALIDATOR = make_validator('./categories.json#')
DEPLOY_VALIDATOR = make_validator('./deploy.json#')
EXPORTER_VALIDATOR = make_validator('./exporter.json#')
AGENT_NOTIFICATION_VALIDATOR = make_validator('./agent_notification.json#')
CLOUD_NOTIFICATION_VALIDATOR = make_validator('./cloud_notification.json#')
LOGS_VALIDATOR = make_validator('./logs.json#')
AUTHENTICATION_VALIDATOR = make_validator('./authentication.json#')
FLOWS_VALIDATOR = make_validator('./flows.json#')
SECRETSTORE_VALIDATOR = make_validator('./secretstore.json#')
SERVICE_DISCOVERY_VALIDATOR = make_validator('./service_discovery.json#')

_jinja_env = False


def get_jinja_env():
    global _jinja_env

    if not _jinja_env:
        from jinja2 import Environment, FileSystemLoader, select_autoescape

        _jinja_env = Environment(
            loader=FileSystemLoader(TEMPLATE_PATH),
            autoescape=select_autoescape(),
            block_start_string='[%',
            block_end_string='%]',
            variable_start_string='[[',
            variable_end_string=']]',
            comment_start_string='[#',
            comment_end_string='#]',
            trim_blocks=True,
            lstrip_blocks=True,
        )

        _jinja_env.globals.update(strfy=strfy, anchorfy=anchorfy)

    return _jinja_env


def strfy(value):
    if isinstance(value, bool):
        return "yes" if value else "no"
    if isinstance(value, str):
        return ' '.join([v.strip() for v in value.strip().split("\n") if v]).replace('|', '/')
    return value


def anchorfy(value):
    if value is None:
        return ''

    anchor = str(value).strip().lower()
    anchor = re.sub(r'[^a-z0-9]+', '-', anchor)
    anchor = re.sub(r'-{2,}', '-', anchor).strip('-')

    return anchor


def get_section_template_name(item, key):
    integration_type = item.get('integration_type')

    if key == 'setup':
        if integration_type == 'secretstore':
            return 'setup-secretstore.md'
        if integration_type == 'service_discovery':
            return 'setup-service_discovery.md'
        if integration_type == 'logs':
            return 'setup-logs.md'
        return 'setup-generic.md'

    if integration_type == 'service_discovery':
        if key == 'services':
            return 'sd-services.md'
        if key == 'verify':
            return 'sd-verify.md'

    return f'{key}.md'


def get_category_sets(categories):
    default = set()
    valid = set()

    for c in categories:
        if 'id' in c:
            valid.add(c['id'])

        if c.get('collector_default', False):
            default.add(c['id'])

        if 'children' in c and c['children']:
            d, v = get_category_sets(c['children'])
            default |= d
            valid |= v

    return (default, valid)


def load_categories():
    categories = load_yaml(CATEGORIES_FILE)

    if not categories:
        sys.exit(1)

    try:
        CATEGORY_VALIDATOR.validate(categories)
    except ValidationError as e:
        warn(
            f'Failed to validate {CATEGORIES_FILE} against the schema: {e.message} (path: {"/".join(str(p) for p in e.absolute_path)})',
            CATEGORIES_FILE)
        sys.exit(1)

    return categories


def load_flows():
    ret = []

    for repo, path, match in FLOWS_SOURCES:
        if match and path.exists() and path.is_dir():
            files = list(path.glob(METADATA_PATTERN))
        elif not match and path.exists() and path.is_file():
            files = [path]
        else:
            files = []

        for file in files:
            debug(f'Loading {file}.')
            data = load_yaml(file)

            if not data:
                continue

            try:
                FLOWS_VALIDATOR.validate(data)
            except ValidationError as e:
                warn(
                    f'Failed to validate {file} against the schema: {e.message} (path: {"/".join(str(p) for p in e.absolute_path)})',
                    file)
                continue

            for idx, item in enumerate(data['modules']):
                item['meta']['plugin_name'] = data['plugin_name']
                item['integration_type'] = 'flows'
                item['_src_path'] = file
                item['_repo'] = repo
                item['_index'] = idx
                ret.append(item)

    return ret


def _load_deploy_file(file, repo):
    ret = []
    debug(f'Loading {file}.')
    data = load_yaml(file)

    if not data:
        return []

    try:
        DEPLOY_VALIDATOR.validate(data)
    except ValidationError as e:
        warn(
            f'Failed to validate {file} against the schema: {e.message} (path: {"/".join(str(p) for p in e.absolute_path)})',
            file)
        return []

    for idx, item in enumerate(data):
        item['integration_type'] = 'deploy'
        item['_src_path'] = file
        item['_repo'] = repo
        item['_index'] = idx
        ret.append(item)

    return ret


def load_deploy():
    ret = []

    for repo, path, match in DEPLOY_SOURCES:
        if match and path.exists() and path.is_dir():
            for file in path.glob(METADATA_PATTERN):
                ret.extend(_load_deploy_file(file, repo))
        elif not match and path.exists() and path.is_file():
            ret.extend(_load_deploy_file(path, repo))

    return ret


def _load_exporter_file(file, repo):
    debug(f'Loading {file}.')
    data = load_yaml(file)

    if not data:
        return []

    try:
        EXPORTER_VALIDATOR.validate(data)
    except ValidationError as e:
        warn(
            f'Failed to validate {file} against the schema: {e.message} (path: {"/".join(str(p) for p in e.absolute_path)})',
            file)
        return []

    if 'id' in data:
        data['integration_type'] = 'exporter'
        data['_src_path'] = file
        data['_repo'] = repo
        data['_index'] = 0

        return [data]
    else:
        ret = []

        for idx, item in enumerate(data):
            item['integration_type'] = 'exporter'
            item['_src_path'] = file
            item['_repo'] = repo
            item['_index'] = idx
            ret.append(item)

        return ret


def load_exporters():
    ret = []

    for repo, path, match in EXPORTER_SOURCES:
        if match and path.exists() and path.is_dir():
            for file in path.glob(METADATA_PATTERN):
                ret.extend(_load_exporter_file(file, repo))
        elif not match and path.exists() and path.is_file():
            ret.extend(_load_exporter_file(path, repo))

    return ret


def _load_agent_notification_file(file, repo):
    debug(f'Loading {file}.')
    data = load_yaml(file)

    if not data:
        return []

    try:
        AGENT_NOTIFICATION_VALIDATOR.validate(data)
    except ValidationError as e:
        warn(
            f'Failed to validate {file} against the schema: {e.message} (path: {"/".join(str(p) for p in e.absolute_path)})',
            file)
        return []

    if 'id' in data:
        data['integration_type'] = 'agent_notification'
        data['_src_path'] = file
        data['_repo'] = repo
        data['_index'] = 0

        return [data]
    else:
        ret = []

        for idx, item in enumerate(data):
            item['integration_type'] = 'agent_notification'
            item['_src_path'] = file
            item['_repo'] = repo
            item['_index'] = idx
            ret.append(item)

        return ret


def load_agent_notifications():
    ret = []

    for repo, path, match in AGENT_NOTIFICATION_SOURCES:
        if match and path.exists() and path.is_dir():
            for file in path.glob(METADATA_PATTERN):
                ret.extend(_load_agent_notification_file(file, repo))
        elif not match and path.exists() and path.is_file():
            ret.extend(_load_agent_notification_file(path, repo))

    return ret


def _load_cloud_notification_file(file, repo):
    debug(f'Loading {file}.')
    data = load_yaml(file)

    if not data:
        return []

    try:
        CLOUD_NOTIFICATION_VALIDATOR.validate(data)
    except ValidationError as e:
        warn(
            f'Failed to validate {file} against the schema: {e.message} (path: {"/".join(str(p) for p in e.absolute_path)})',
            file)
        return []

    if 'id' in data:
        data['integration_type'] = 'cloud_notification'
        data['_src_path'] = file
        data['_repo'] = repo
        data['_index'] = 0

        return [data]
    else:
        ret = []

        for idx, item in enumerate(data):
            item['integration_type'] = 'cloud_notification'
            item['_src_path'] = file
            item['_repo'] = repo
            item['_index'] = idx
            ret.append(item)

        return ret


def load_cloud_notifications():
    ret = []

    for repo, path, match in CLOUD_NOTIFICATION_SOURCES:
        if match and path.exists() and path.is_dir():
            for file in path.glob(METADATA_PATTERN):
                ret.extend(_load_cloud_notification_file(file, repo))
        elif not match and path.exists() and path.is_file():
            ret.extend(_load_cloud_notification_file(path, repo))

    return ret


def _load_logs_file(file, repo):
    debug(f'Loading {file}.')
    data = load_yaml(file)

    if not data:
        return []

    try:
        LOGS_VALIDATOR.validate(data)
    except ValidationError as e:
        warn(
            f'Failed to validate {file} against the schema: {e.message} (path: {"/".join(str(p) for p in e.absolute_path)})',
            file)
        return []

    if 'id' in data:
        data['integration_type'] = 'logs'
        data['_src_path'] = file
        data['_repo'] = repo
        data['_index'] = 0

        return [data]
    else:
        ret = []

        for idx, item in enumerate(data):
            item['integration_type'] = 'logs'
            item['_src_path'] = file
            item['_repo'] = repo
            item['_index'] = idx
            ret.append(item)

        return ret


def load_logs():
    ret = []

    for repo, path, match in LOGS_SOURCES:
        if match and path.exists() and path.is_dir():
            for file in path.glob(METADATA_PATTERN):
                ret.extend(_load_logs_file(file, repo))
        elif not match and path.exists() and path.is_file():
            ret.extend(_load_logs_file(path, repo))

    return ret


def _load_authentication_file(file, repo):
    debug(f'Loading {file}.')
    data = load_yaml(file)

    if not data:
        return []

    try:
        AUTHENTICATION_VALIDATOR.validate(data)
    except ValidationError as e:
        warn(
            f'Failed to validate {file} against the schema: {e.message} (path: {"/".join(str(p) for p in e.absolute_path)})',
            file)
        return []

    if 'id' in data:
        data['integration_type'] = 'authentication'
        data['_src_path'] = file
        data['_repo'] = repo
        data['_index'] = 0

        return [data]
    else:
        ret = []

        for idx, item in enumerate(data):
            item['integration_type'] = 'authentication'
            item['_src_path'] = file
            item['_repo'] = repo
            item['_index'] = idx
            ret.append(item)

        return ret


def load_authentications():
    ret = []

    for repo, path, match in AUTHENTICATION_SOURCES:
        if match and path.exists() and path.is_dir():
            for file in path.glob(METADATA_PATTERN):
                ret.extend(_load_authentication_file(file, repo))
        elif not match and path.exists() and path.is_file():
            ret.extend(_load_authentication_file(path, repo))

    return ret


def _load_secretstore_file(file, repo):
    debug(f'Loading {file}.')
    data = load_yaml(file)

    if not data:
        return []

    try:
        SECRETSTORE_VALIDATOR.validate(data)
    except ValidationError as e:
        warn(
            f'Failed to validate {file} against the schema: {e.message} (path: {"/".join(str(p) for p in e.absolute_path)})',
            file)
        return []

    if 'id' in data:
        data['integration_type'] = 'secretstore'
        data['_src_path'] = file
        data['_repo'] = repo
        data['_index'] = 0

        return [data]
    else:
        ret = []

        for idx, item in enumerate(data):
            item['integration_type'] = 'secretstore'
            item['_src_path'] = file
            item['_repo'] = repo
            item['_index'] = idx
            ret.append(item)

        return ret


def load_secretstores():
    ret = []

    for repo, path, match in SECRETSTORE_SOURCES:
        if match and path.exists() and path.is_dir():
            for file in path.glob(METADATA_PATTERN):
                ret.extend(_load_secretstore_file(file, repo))
        elif not match and path.exists() and path.is_file():
            ret.extend(_load_secretstore_file(path, repo))

    return ret


def _load_service_discovery_file(file, repo):
    debug(f'Loading {file}.')
    data = load_yaml(file)

    if not data:
        return []

    try:
        SERVICE_DISCOVERY_VALIDATOR.validate(data)
    except ValidationError as e:
        warn(
            f'Failed to validate {file} against the schema: {e.message} (path: {"/".join(str(p) for p in e.absolute_path)})',
            file)
        return []

    if 'id' in data:
        data['integration_type'] = 'service_discovery'
        data['_src_path'] = file
        data['_repo'] = repo
        data['_index'] = 0

        return [data]
    else:
        ret = []

        for idx, item in enumerate(data):
            item['integration_type'] = 'service_discovery'
            item['_src_path'] = file
            item['_repo'] = repo
            item['_index'] = idx
            ret.append(item)

        return ret


def load_service_discoveries():
    ret = []

    for repo, path, match in SERVICE_DISCOVERY_SOURCES:
        if match and path.exists() and path.is_dir():
            for file in path.glob(METADATA_PATTERN):
                ret.extend(_load_service_discovery_file(file, repo))
        elif not match and path.exists() and path.is_file():
            ret.extend(_load_service_discovery_file(path, repo))

    return ret


def make_edit_link(item):
    item_path = item['_src_path'].relative_to(REPO_PATH)

    return f'https://github.com/{item["_repo"]}/blob/master/{item_path}'


def sort_integrations(integrations):
    integrations.sort(key=lambda i: i['_index'])
    integrations.sort(key=lambda i: i['_src_path'])
    integrations.sort(key=lambda i: i['id'])


def dedupe_integrations(integrations, ids):
    tmp_integrations = []

    for i in integrations:
        if ids.get(i['id'], False):
            first_path, first_index = ids[i['id']]
            warn(
                f'Duplicate integration ID found at {i["_src_path"]} index {i["_index"]} (original definition at {first_path} index {first_index}), ignoring that integration.',
                i['_src_path'])
        else:
            tmp_integrations.append(i)
            ids[i['id']] = (i['_src_path'], i['_index'])

    return tmp_integrations, ids


def render_collectors(categories, collectors, ids):
    debug('Computing default categories.')

    default_cats, valid_cats = get_category_sets(categories)

    debug('Generating collector IDs.')

    for item in collectors:
        item['id'] = make_id(item['meta'])

    debug('Sorting collectors.')

    sort_integrations(collectors)

    debug('Removing duplicate collectors.')

    collectors, ids = dedupe_integrations(collectors, ids)
    clean_collectors = []

    # Build hierarchical indexes for cascading related_resources lookup:
    #   Level 1: plugin_name + module_name + monitored_instance_name (exact)
    #   Level 2: plugin_name + module_name (all instances of that module)
    #   Level 3: plugin_name (all modules of that plugin)
    by_pm_instance = {}  # (plugin, module, instance) -> [items]
    by_pm = {}  # (plugin, module) -> [items]
    by_plugin = {}  # plugin -> [items]

    for i in collectors:
        m = i['meta']
        pn = m['plugin_name']
        mn = m['module_name']
        inst = m['monitored_instance']['name']

        by_pm_instance.setdefault((pn, mn, inst), []).append(i)
        by_pm.setdefault((pn, mn), []).append(i)
        by_plugin.setdefault(pn, []).append(i)

    def find_related(res):
        """Cascading lookup: try most specific first, relax until a match is found."""
        pn = res['plugin_name']
        mn = res.get('module_name')
        inst = res.get('monitored_instance_name')

        # Level 1: all three specified
        if mn and inst:
            matches = by_pm_instance.get((pn, mn, inst))
            if matches:
                return matches
            debug(f'No exact related_resources match for plugin={pn!r}, '
                  f'module={mn!r}, monitored_instance_name={inst!r}; '
                  f'falling back to all instances of that module.')

        # Level 2: plugin + module
        if mn:
            # When module_name is explicitly specified, don't fall back to
            # plugin-only — that would mask typos in the reference.
            return by_pm.get((pn, mn), [])

        # Level 3: plugin only (no module specified)
        return by_plugin.get(pn, [])

    for item in collectors:
        debug(f'Processing {item["id"]}.')

        item['edit_link'] = make_edit_link(item)

        clean_item = deepcopy(item)

        related = []
        seen_ids = set()

        for res in item['meta']['related_resources']['integrations']['list']:
            matches = find_related(res)

            if not matches:
                warn(f'Could not find related integration for {res}, ignoring it.', item['_src_path'])
                continue

            for match in matches:
                mid = match['id']

                # skip self-references and duplicates
                if mid == item['id'] or mid in seen_ids:
                    continue

                seen_ids.add(mid)
                related.append({
                    'plugin_name': match['meta']['plugin_name'],
                    'module_name': match['meta']['module_name'],
                    'id': mid,
                    'name': match['meta']['monitored_instance']['name'],
                    'info': match['meta']['info_provided_to_referring_integrations'],
                })

        item_cats = set(item['meta']['monitored_instance']['categories'])
        bogus_cats = item_cats - valid_cats
        actual_cats = item_cats & valid_cats

        if bogus_cats:
            warn(f'Ignoring invalid categories: {", ".join(bogus_cats)}', item["_src_path"])

        if not item_cats:
            item['meta']['monitored_instance']['categories'] = list(default_cats)
            warn(f'{item["id"]} does not list any caregories, adding it to: {default_cats}', item["_src_path"])
        else:
            item['meta']['monitored_instance']['categories'] = [x for x in
                                                                item['meta']['monitored_instance']['categories'] if
                                                                x in list(actual_cats)]

        for scope in item['metrics']['scopes']:
            if scope['name'] == 'global':
                scope['name'] = f'{item["meta"]["monitored_instance"]["name"]} instance'

        for cfg_example in item['setup']['configuration']['examples']['list']:
            if 'folding' not in cfg_example:
                cfg_example['folding'] = {
                    'enabled': item['setup']['configuration']['examples']['folding']['enabled']
                }

        for key in COLLECTOR_RENDER_KEYS:
            if key in item.keys():
                template = get_jinja_env().get_template(get_section_template_name(item, key))
                data = template.render(entry=item, related=related, clean=False)
                clean_data = template.render(entry=item, related=related, clean=True)

                if 'variables' in item['meta']['monitored_instance']:
                    template = get_jinja_env().from_string(data)
                    data = template.render(variables=item['meta']['monitored_instance']['variables'])
                    template = get_jinja_env().from_string(clean_data)
                    clean_data = template.render(variables=item['meta']['monitored_instance']['variables'])
            else:
                data = ''
                clean_data = ''

            item[key] = data
            clean_item[key] = clean_data

        for k in ['_src_path', '_repo', '_index']:
            del item[k], clean_item[k]

        clean_collectors.append(clean_item)

    return collectors, clean_collectors, ids


def render_deploy(distros, categories, deploy, ids):
    debug('Sorting deployments.')

    sort_integrations(deploy)

    debug('Checking deployment ids.')

    deploy, ids = dedupe_integrations(deploy, ids)
    clean_deploy = []

    template = get_jinja_env().get_template('platform_info.md')

    for item in deploy:
        debug(f'Processing {item["id"]}.')
        item['edit_link'] = make_edit_link(item)
        clean_item = deepcopy(item)

        if item['platform_info']['group']:
            entries = [
                {
                    'version': i['version'],
                    'support': i['support_type'],
                    'arches': i.get('packages', {'arches': []})['arches'],
                    'notes': i['notes'],
                } for i in distros[item['platform_info']['group']] if i['distro'] == item['platform_info']['distro']
            ]
        else:
            entries = []

        data = template.render(entries=entries, clean=False)
        clean_data = template.render(entries=entries, clean=True)

        for method in clean_item['methods']:
            for command in method['commands']:
                command['command'] = CUSTOM_TAG_PATTERN.sub('', command['command'])
                command['command'] = FIXUP_BLANK_PATTERN.sub('', command['command'])

        item['platform_info'] = data
        clean_item['platform_info'] = clean_data

        if 'clean_additional_info' in item:
            clean_item['additional_info'] = item['clean_additional_info']
            del item['clean_additional_info'], clean_item['clean_additional_info']

        for k in ['_src_path', '_repo', '_index']:
            del item[k], clean_item[k]

        clean_deploy.append(clean_item)

    return deploy, clean_deploy, ids


def render_exporters(categories, exporters, ids):
    debug('Sorting exporters.')

    sort_integrations(exporters)

    debug('Checking exporter ids.')

    exporters, ids = dedupe_integrations(exporters, ids)

    clean_exporters = []

    for item in exporters:
        item['edit_link'] = make_edit_link(item)

        clean_item = deepcopy(item)

        for key in EXPORTER_RENDER_KEYS:
            if key in item.keys():
                template = get_jinja_env().get_template(get_section_template_name(item, key))
                data = template.render(entry=item, clean=False)
                clean_data = template.render(entry=item, clean=True)

                if 'variables' in item['meta']:
                    template = get_jinja_env().from_string(data)
                    data = template.render(variables=item['meta']['variables'], clean=False)
                    template = get_jinja_env().from_string(clean_data)
                    clean_data = template.render(variables=item['meta']['variables'], clean=True)
            else:
                data = ''
                clean_data = ''

            item[key] = data
            clean_item[key] = clean_data

        for k in ['_src_path', '_repo', '_index']:
            del item[k], clean_item[k]

        clean_exporters.append(clean_item)

    return exporters, clean_exporters, ids


def render_agent_notifications(categories, notifications, ids):
    debug('Sorting notifications.')

    sort_integrations(notifications)

    debug('Checking notification ids.')

    notifications, ids = dedupe_integrations(notifications, ids)

    clean_notifications = []

    for item in notifications:
        item['edit_link'] = make_edit_link(item)

        clean_item = deepcopy(item)

        for key in AGENT_NOTIFICATION_RENDER_KEYS:
            if key in item.keys():
                template = get_jinja_env().get_template(get_section_template_name(item, key))
                data = template.render(entry=item, clean=False)

                clean_data = template.render(entry=item, clean=True)

                if 'variables' in item['meta']:
                    template = get_jinja_env().from_string(data)
                    data = template.render(variables=item['meta']['variables'], clean=False)
                    template = get_jinja_env().from_string(clean_data)
                    clean_data = template.render(variables=item['meta']['variables'], clean=True)
            else:
                data = ''
                clean_data = ''

            item[key] = data
            clean_item[key] = clean_data

        for k in ['_src_path', '_repo', '_index']:
            del item[k], clean_item[k]

        clean_notifications.append(clean_item)

    return notifications, clean_notifications, ids


def render_cloud_notifications(categories, notifications, ids):
    debug('Sorting notifications.')

    sort_integrations(notifications)

    debug('Checking notification ids.')

    notifications, ids = dedupe_integrations(notifications, ids)

    clean_notifications = []

    for item in notifications:
        item['edit_link'] = make_edit_link(item)

        clean_item = deepcopy(item)

        for key in CLOUD_NOTIFICATION_RENDER_KEYS:
            if key in item.keys():
                template = get_jinja_env().get_template(get_section_template_name(item, key))
                data = template.render(entry=item, clean=False)
                clean_data = template.render(entry=item, clean=True)

                if 'variables' in item['meta']:
                    template = get_jinja_env().from_string(data)
                    data = template.render(variables=item['meta']['variables'], clean=False)
                    template = get_jinja_env().from_string(clean_data)
                    clean_data = template.render(variables=item['meta']['variables'], clean=True)
            else:
                data = ''
                clean_data = ''

            item[key] = data
            clean_item[key] = clean_data

        for k in ['_src_path', '_repo', '_index']:
            del item[k], clean_item[k]

        clean_notifications.append(clean_item)

    return notifications, clean_notifications, ids


def render_flows(categories, flows, ids):
    debug('Generating flow IDs.')

    for item in flows:
        item['id'] = make_id(item['meta'])

    debug('Sorting flows.')

    sort_integrations(flows)

    debug('Checking flow ids.')

    flows, ids = dedupe_integrations(flows, ids)

    clean_flows = []

    for item in flows:
        item['edit_link'] = make_edit_link(item)

        clean_item = deepcopy(item)

        for key in FLOWS_RENDER_KEYS:
            if key in item.keys():
                template = get_jinja_env().get_template(get_section_template_name(item, key))
                data = template.render(entry=item, clean=False)
                clean_data = template.render(entry=item, clean=True)

                if 'variables' in item['meta']:
                    template = get_jinja_env().from_string(data)
                    data = template.render(variables=item['meta']['variables'], clean=False)
                    template = get_jinja_env().from_string(clean_data)
                    clean_data = template.render(variables=item['meta']['variables'], clean=True)
            else:
                data = ''
                clean_data = ''

            item[key] = data
            clean_item[key] = clean_data

        for k in ['_src_path', '_repo', '_index']:
            del item[k], clean_item[k]

        clean_flows.append(clean_item)

    return flows, clean_flows, ids


def render_logs(categories, logs, ids):
    debug('Sorting logs.')

    sort_integrations(logs)

    debug('Checking log ids.')

    logs, ids = dedupe_integrations(logs, ids)

    clean_logs = []

    for item in logs:
        item['edit_link'] = make_edit_link(item)

        clean_item = deepcopy(item)

        for key in LOGS_RENDER_KEYS:
            if key in item.keys():
                template = get_jinja_env().get_template(get_section_template_name(item, key))
                data = template.render(entry=item, clean=False)
                clean_data = template.render(entry=item, clean=True)

                if 'variables' in item['meta']:
                    template = get_jinja_env().from_string(data)
                    data = template.render(variables=item['meta']['variables'], clean=False)
                    template = get_jinja_env().from_string(clean_data)
                    clean_data = template.render(variables=item['meta']['variables'], clean=True)
            else:
                data = ''
                clean_data = ''

            item[key] = data
            clean_item[key] = clean_data

        for k in ['_src_path', '_repo', '_index']:
            del item[k], clean_item[k]

        clean_logs.append(clean_item)

    return logs, clean_logs, ids


def render_authentications(categories, authentications, ids):
    debug('Sorting authentications.')

    sort_integrations(authentications)

    debug('Checking authentication ids.')

    authentications, ids = dedupe_integrations(authentications, ids)

    clean_authentications = []

    for item in authentications:
        item['edit_link'] = make_edit_link(item)

        clean_item = deepcopy(item)

        for key in AUTHENTICATION_RENDER_KEYS:

            if key in item.keys():
                template = get_jinja_env().get_template(get_section_template_name(item, key))
                data = template.render(entry=item, clean=False)
                clean_data = template.render(entry=item, clean=True)

                if 'variables' in item['meta']:
                    template = get_jinja_env().from_string(data)
                    data = template.render(variables=item['meta']['variables'], clean=False)
                    template = get_jinja_env().from_string(clean_data)
                    clean_data = template.render(variables=item['meta']['variables'], clean=True)
            else:
                data = ''
                clean_data = ''

            item[key] = data
            clean_item[key] = clean_data

        for k in ['_src_path', '_repo', '_index']:
            del item[k], clean_item[k]

        clean_authentications.append(clean_item)

    return authentications, clean_authentications, ids


def render_secretstores(categories, secretstores, ids):
    debug('Sorting secretstores.')

    sort_integrations(secretstores)

    debug('Checking secretstore ids.')

    secretstores, ids = dedupe_integrations(secretstores, ids)

    clean_secretstores = []

    for item in secretstores:
        item['edit_link'] = make_edit_link(item)

        clean_item = deepcopy(item)
        collector_configs = item.get('collector_configs', {})
        collector_configs_summary = {}
        if isinstance(collector_configs, dict):
            summary = collector_configs.get('summary', {})
            if isinstance(summary, dict):
                collector_configs_summary = deepcopy(summary)

        item['collector_configs_summary'] = deepcopy(collector_configs_summary)
        clean_item['collector_configs_summary'] = deepcopy(collector_configs_summary)

        for key in SECRETSTORE_RENDER_KEYS:
            if key in item.keys():
                template = get_jinja_env().get_template(get_section_template_name(item, key))
                data = template.render(entry=item, clean=False)
                clean_data = template.render(entry=item, clean=True)

                if 'variables' in item['meta']:
                    template = get_jinja_env().from_string(data)
                    data = template.render(variables=item['meta']['variables'], clean=False)
                    template = get_jinja_env().from_string(clean_data)
                    clean_data = template.render(variables=item['meta']['variables'], clean=True)
            else:
                data = ''
                clean_data = ''

            item[key] = data
            clean_item[key] = clean_data

        for k in ['_src_path', '_repo', '_index']:
            del item[k], clean_item[k]

        clean_secretstores.append(clean_item)

    return secretstores, clean_secretstores, ids


def render_service_discoveries(categories, service_discoveries, ids):
    debug('Sorting service discoveries.')

    sort_integrations(service_discoveries)

    debug('Checking service discovery ids.')

    service_discoveries, ids = dedupe_integrations(service_discoveries, ids)

    clean_service_discoveries = []

    for item in service_discoveries:
        item['edit_link'] = make_edit_link(item)

        clean_item = deepcopy(item)

        for key in SERVICE_DISCOVERY_RENDER_KEYS:
            if key in item.keys():
                template = get_jinja_env().get_template(get_section_template_name(item, key))
                data = template.render(entry=item, clean=False)
                clean_data = template.render(entry=item, clean=True)

                if 'variables' in item['meta']:
                    template = get_jinja_env().from_string(data)
                    data = template.render(variables=item['meta']['variables'], clean=False)
                    template = get_jinja_env().from_string(clean_data)
                    clean_data = template.render(variables=item['meta']['variables'], clean=True)
            else:
                data = ''
                clean_data = ''

            item[key] = data
            clean_item[key] = clean_data

        for k in ['_src_path', '_repo', '_index']:
            del item[k], clean_item[k]

        clean_service_discoveries.append(clean_item)

    return service_discoveries, clean_service_discoveries, ids


def convert_local_links(text, prefix):
    return text.replace("](/", f"]({prefix}/")


def render_integrations(categories, integrations):
    template = get_jinja_env().get_template('integrations.js')
    data = template.render(
        categories=json.dumps(categories, indent=4),
        integrations=json.dumps(integrations, indent=4),
    )
    data = convert_local_links(data, "https://github.com/netdata/netdata/blob/master")
    OUTPUT_PATH.write_text(data)


def render_json(categories, integrations):
    JSON_PATH.write_text(json.dumps({
        'categories': categories,
        'integrations': integrations,
    }, indent=4))


def main():
    categories = load_categories()
    distros = load_yaml(DISTROS_FILE)
    collectors = load_collectors()
    deploy = load_deploy()
    exporters = load_exporters()
    agent_notifications = load_agent_notifications()
    cloud_notifications = load_cloud_notifications()
    logs = load_logs()
    flows = load_flows()
    authentications = load_authentications()
    secretstores = load_secretstores()
    service_discoveries = load_service_discoveries()

    collectors, clean_collectors, ids = render_collectors(categories, collectors, dict())
    deploy, clean_deploy, ids = render_deploy(distros, categories, deploy, ids)
    exporters, clean_exporters, ids = render_exporters(categories, exporters, ids)
    agent_notifications, clean_agent_notifications, ids = render_agent_notifications(categories, agent_notifications,
                                                                                     ids)
    cloud_notifications, clean_cloud_notifications, ids = render_cloud_notifications(categories, cloud_notifications,
                                                                                     ids)
    logs, clean_logs, ids = render_logs(categories, logs, ids)
    flows, clean_flows, ids = render_flows(categories, flows, ids)
    authentications, clean_authentications, ids = render_authentications(categories, authentications, ids)
    secretstores, clean_secretstores, ids = render_secretstores(categories, secretstores, ids)
    service_discoveries, clean_service_discoveries, ids = render_service_discoveries(categories, service_discoveries,
                                                                                     ids)

    integrations = collectors + deploy + exporters + agent_notifications + cloud_notifications + logs + flows + authentications + secretstores + service_discoveries
    render_integrations(categories, integrations)

    clean_integrations = clean_collectors + clean_deploy + clean_exporters + clean_agent_notifications + clean_cloud_notifications + clean_logs + clean_flows + clean_authentications + clean_secretstores + clean_service_discoveries
    render_json(categories, clean_integrations)

    return fail_on_warnings()


if __name__ == '__main__':
    sys.exit(main())
