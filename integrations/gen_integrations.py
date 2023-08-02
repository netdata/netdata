#!/usr/bin/env python3

import json
import os
import sys

from pathlib import Path

from jsonschema import Draft7Validator, ValidationError
from referencing import Registry, Resource
from referencing.jsonschema import DRAFT7
from ruamel.yaml import YAML, YAMLError

AGENT_REPO = 'netdata/netdata'
GO_REPO = 'netdata/go.d.plugin'

INTEGRATIONS_PATH = Path(__file__).parent
TEMPLATE_PATH = INTEGRATIONS_PATH / 'templates'
OUTPUT_PATH = INTEGRATIONS_PATH / 'integrations.js'
CATEGORIES_FILE = INTEGRATIONS_PATH / 'categories.yaml'
REPO_PATH = INTEGRATIONS_PATH.parent
SCHEMA_PATH = INTEGRATIONS_PATH / 'schemas'
GO_REPO_PATH = REPO_PATH / 'go.d.plugin'
DISTROS_FILE = REPO_PATH / '.github' / 'data' / 'distros.yml'
METADATA_PATTERN = '*/metadata.yaml'

COLLECTOR_SOURCES = [
    (AGENT_REPO, REPO_PATH / 'collectors', True),
    (AGENT_REPO, REPO_PATH / 'collectors' / 'charts.d.plugin', True),
    (AGENT_REPO, REPO_PATH / 'collectors' / 'python.d.plugin', True),
    (GO_REPO, GO_REPO_PATH / 'modules', True),
]

DEPLOY_SOURCES = [
    (AGENT_REPO, INTEGRATIONS_PATH / 'deploy.yaml', False),
]

EXPORTER_SOURCES = [
    (AGENT_REPO, REPO_PATH / 'exporting', True),
]

NOTIFICATION_SOURCES = [
    (AGENT_REPO, REPO_PATH / 'health' / 'notifications', True),
    (AGENT_REPO, INTEGRATIONS_PATH / 'cloud-notifications' / 'metadata.yaml', False),
]

COLLECTOR_RENDER_KEYS = [
    'alerts',
    'metrics',
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

NOTIFICATION_RENDER_KEYS = [
    'overview',
    'setup',
    'troubleshooting',
]

GITHUB_ACTIONS = os.environ.get('GITHUB_ACTIONS', False)
DEBUG = os.environ.get('DEBUG', False)


def debug(msg):
    if GITHUB_ACTIONS:
        print(f':debug:{ msg }')
    elif DEBUG:
        print(f'>>> { msg }')
    else:
        pass


def warn(msg, path):
    if GITHUB_ACTIONS:
        print(f':warning file={ path }:{ msg }')
    else:
        print(f'!!! WARNING:{ path }:{ msg }')


def retrieve_from_filesystem(uri):
    path = SCHEMA_PATH / Path(uri)
    contents = json.loads(path.read_text())
    return Resource.from_contents(contents, DRAFT7)


registry = Registry(retrieve=retrieve_from_filesystem)

CATEGORY_VALIDATOR = Draft7Validator(
    {'$ref': './categories.json#'},
    registry=registry,
)

DEPLOY_VALIDATOR = Draft7Validator(
    {'$ref': './deploy.json#'},
    registry=registry,
)

EXPORTER_VALIDATOR = Draft7Validator(
    {'$ref': './exporter.json#'},
    registry=registry,
)

NOTIFICATION_VALIDATOR = Draft7Validator(
    {'$ref': './notification.json#'},
    registry=registry,
)

COLLECTOR_VALIDATOR = Draft7Validator(
    {'$ref': './collector.json#'},
    registry=registry,
)

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

    return _jinja_env


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


def get_collector_metadata_entries():
    ret = []

    for r, d, m in COLLECTOR_SOURCES:
        if d.exists() and d.is_dir() and m:
            for item in d.glob(METADATA_PATTERN):
                ret.append((r, item))
        elif d.exists() and d.is_file() and not m:
            if d.match(METADATA_PATTERN):
                ret.append(d)

    return ret


def load_yaml(src):
    yaml = YAML(typ='safe')

    if not src.is_file():
        warn(f'{ src } is not a file.', src)
        return False

    try:
        contents = src.read_text()
    except (IOError, OSError):
        warn(f'Failed to read { src }.', src)
        return False

    try:
        data = yaml.load(contents)
    except YAMLError:
        warn(f'Failed to parse { src } as YAML.', src)
        return False

    return data


def load_categories():
    categories = load_yaml(CATEGORIES_FILE)

    if not categories:
        sys.exit(1)

    try:
        CATEGORY_VALIDATOR.validate(categories)
    except ValidationError:
        warn(f'Failed to validate { CATEGORIES_FILE } against the schema.', CATEGORIES_FILE)
        sys.exit(1)

    return categories


def load_collectors():
    ret = []

    entries = get_collector_metadata_entries()

    for repo, path in entries:
        debug(f'Loading { path }.')
        data = load_yaml(path)

        if not data:
            continue

        try:
            COLLECTOR_VALIDATOR.validate(data)
        except ValidationError:
            warn(f'Failed to validate { path } against the schema.', path)
            continue

        for idx, item in enumerate(data['modules']):
            item['meta']['plugin_name'] = data['plugin_name']
            item['integration_type'] = 'collector'
            item['_src_path'] = path
            item['_repo'] = repo
            item['_index'] = idx
            ret.append(item)

    return ret


def _load_deploy_file(file, repo):
    ret = []
    debug(f'Loading { file }.')
    data = load_yaml(file)

    if not data:
        return []

    try:
        DEPLOY_VALIDATOR.validate(data)
    except ValidationError:
        warn(f'Failed to validate { file } against the schema.', file)
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
    debug(f'Loading { file }.')
    data = load_yaml(file)

    if not data:
        return []

    try:
        EXPORTER_VALIDATOR.validate(data)
    except ValidationError:
        warn(f'Failed to validate { file } against the schema.', file)
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


def _load_notification_file(file, repo):
    debug(f'Loading { file }.')
    data = load_yaml(file)

    if not data:
        return []

    try:
        NOTIFICATION_VALIDATOR.validate(data)
    except ValidationError:
        warn(f'Failed to validate { file } against the schema.', file)
        return []

    if 'id' in data:
        data['integration_type'] = 'notification'
        data['_src_path'] = file
        data['_repo'] = repo
        data['_index'] = 0

        return [data]
    else:
        ret = []

        for idx, item in enumerate(data):
            item['integration_type'] = 'notification'
            item['_src_path'] = file
            item['_repo'] = repo
            item['_index'] = idx
            ret.append(item)

        return ret


def load_notifications():
    ret = []

    for repo, path, match in NOTIFICATION_SOURCES:
        if match and path.exists() and path.is_dir():
            for file in path.glob(METADATA_PATTERN):
                ret.extend(_load_notification_file(file, repo))
        elif not match and path.exists() and path.is_file():
            ret.extend(_load_notification_file(path, repo))

    return ret


def make_id(meta):
    if 'monitored_instance' in meta:
        instance_name = meta['monitored_instance']['name'].replace(' ', '_')
    elif 'instance_name' in meta:
        instance_name = meta['instance_name']
    else:
        instance_name = '000_unknown'

    return f'{ meta["plugin_name"] }-{ meta["module_name"] }-{ instance_name }'


def make_edit_link(item):
    if item['_repo'] == 'netdata/go.d.plugin':
        item_path = item['_src_path'].relative_to(GO_REPO_PATH)
    else:
        item_path = item['_src_path'].relative_to(REPO_PATH)

    return f'https://github.com/{ item["_repo"] }/blob/master/{ item_path }'


def sort_integrations(integrations):
    integrations.sort(key=lambda i: i['_index'])
    integrations.sort(key=lambda i: i['_src_path'])
    integrations.sort(key=lambda i: i['id'])


def dedupe_integrations(integrations, ids):
    tmp_integrations = []

    for i in integrations:
        if ids.get(i['id'], False):
            first_path, first_index = ids[i['id']]
            warn(f'Duplicate integration ID found at { i["_src_path"] } index { i["_index"] } (original definition at { first_path } index { first_index }), ignoring that integration.', i['_src_path'])
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

    idmap = {i['id']: i for i in collectors}

    for item in collectors:
        debug(f'Processing { item["id"] }.')

        related = []

        for res in item['meta']['related_resources']['integrations']['list']:
            res_id = make_id(res)

            if res_id not in idmap.keys():
                warn(f'Could not find related integration { res_id }, ignoring it.', item['_src_path'])
                continue

            related.append({
                'plugin_name': res['plugin_name'],
                'module_name': res['module_name'],
                'id': res_id,
                'name': idmap[res_id]['meta']['monitored_instance']['name'],
                'info': idmap[res_id]['meta']['info_provided_to_referring_integrations'],
            })

        item_cats = set(item['meta']['monitored_instance']['categories'])
        bogus_cats = item_cats - valid_cats
        actual_cats = item_cats & valid_cats

        if bogus_cats:
            warn(f'Ignoring invalid categories: { ", ".join(bogus_cats) }', item["_src_path"])

        if not item_cats:
            item['meta']['monitored_instance']['categories'] = list(default_cats)
            warn(f'{ item["id"] } does not list any caregories, adding it to: { default_cats }', item["_src_path"])
        else:
            item['meta']['monitored_instance']['categories'] = list(actual_cats)

        for scope in item['metrics']['scopes']:
            if scope['name'] == 'global':
                scope['name'] = f'{ item["meta"]["monitored_instance"]["name"] } instance'

        for cfg_example in item['setup']['configuration']['examples']['list']:
            if 'folding' not in cfg_example:
                cfg_example['folding'] = {
                    'enabled': item['setup']['configuration']['examples']['folding']['enabled']
                }

        for key in COLLECTOR_RENDER_KEYS:
            if key in item.keys():
                template = get_jinja_env().get_template(f'{ key }.md')
                data = template.render(entry=item, related=related)

                if 'variables' in item['meta']['monitored_instance']:
                    template = get_jinja_env().from_string(data)
                    data = template.render(variables=item['meta']['monitored_instance']['variables'])
            else:
                data = ''

            item[key] = data

        item['edit_link'] = make_edit_link(item)

        del item['_src_path']
        del item['_repo']
        del item['_index']

    return collectors, ids


def render_deploy(distros, categories, deploy, ids):
    debug('Sorting deployments.')

    sort_integrations(deploy)

    debug('Checking deployment ids.')

    deploy, ids = dedupe_integrations(deploy, ids)

    template = get_jinja_env().get_template('platform_info.md')

    for item in deploy:
        debug(f'Processing { item["id"] }.')

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

        data = template.render(entries=entries)

        item['platform_info'] = data
        item['edit_link'] = make_edit_link(item)

        del item['_src_path']
        del item['_repo']
        del item['_index']

    return deploy, ids


def render_exporters(categories, exporters, ids):
    debug('Sorting exporters.')

    sort_integrations(exporters)

    debug('Checking exporter ids.')

    exporters, ids = dedupe_integrations(exporters, ids)

    for item in exporters:
        for key in EXPORTER_RENDER_KEYS:
            if key in item.keys():
                template = get_jinja_env().get_template(f'{ key }.md')
                data = template.render(entry=item)

                if 'variables' in item['meta']:
                    template = get_jinja_env().from_string(data)
                    data = template.render(variables=item['meta']['variables'])
            else:
                data = ''

            item[key] = data

        item['edit_link'] = make_edit_link(item)

        del item['_src_path']
        del item['_repo']
        del item['_index']

    return exporters, ids


def render_notifications(categories, notifications, ids):
    debug('Sorting notifications.')

    sort_integrations(notifications)

    debug('Checking notification ids.')

    notifications, ids = dedupe_integrations(notifications, ids)

    for item in notifications:
        for key in NOTIFICATION_RENDER_KEYS:
            if key in item.keys():
                template = get_jinja_env().get_template(f'{ key }.md')
                data = template.render(entry=item)

                if 'variables' in item['meta']:
                    template = get_jinja_env().from_string(data)
                    data = template.render(variables=item['meta']['variables'])
            else:
                data = ''

            item[key] = data

        item['edit_link'] = make_edit_link(item)

        del item['_src_path']
        del item['_repo']
        del item['_index']

    return notifications, ids


def render_integrations(categories, integrations):
    template = get_jinja_env().get_template('integrations.js')
    data = template.render(
        categories=json.dumps(categories),
        integrations=json.dumps(integrations),
    )
    OUTPUT_PATH.write_text(data)


def main():
    categories = load_categories()
    distros = load_yaml(DISTROS_FILE)
    collectors = load_collectors()
    deploy = load_deploy()
    exporters = load_exporters()
    notifications = load_notifications()

    collectors, ids = render_collectors(categories, collectors, dict())
    deploy, ids = render_deploy(distros, categories, deploy, ids)
    exporters, ids = render_exporters(categories, exporters, ids)
    notifications, ids = render_notifications(categories, notifications, ids)

    integrations = collectors + deploy + exporters + notifications
    render_integrations(categories, integrations)


if __name__ == '__main__':
    sys.exit(main())
