#!/usr/bin/env python3

import json
import os
import sys

from pathlib import Path

from jsonschema import Draft7Validator, ValidationError
from referencing import Registry, Resource
from referencing.jsonschema import DRAFT7
from ruamel.yaml import YAML, YAMLError

INTEGRATIONS_PATH = Path(__file__).parent
TEMPLATE_PATH = INTEGRATIONS_PATH / 'templates'
OUTPUT_PATH = INTEGRATIONS_PATH / 'integrations.js'
CATEGORIES_FILE = INTEGRATIONS_PATH / 'categories.yaml'
REPO_PATH = INTEGRATIONS_PATH.parent
SCHEMA_PATH = INTEGRATIONS_PATH / 'schemas'
GO_REPO_PATH = REPO_PATH / 'go.d.plugin'
SINGLE_PATTERN = '*/metadata.yaml'
MULTI_PATTERN = '*/multi_metadata.yaml'

COLLECTOR_SOURCES = [
    REPO_PATH / 'collectors',
    REPO_PATH / 'collectors' / 'charts.d.plugin',
    REPO_PATH / 'collectors' / 'python.d.plugin',
    GO_REPO_PATH / 'modules',
]

RENDER_KEYS = [
    'alerts',
    'metrics',
    'overview',
    'related_resources',
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

SINGLE_VALIDATOR = Draft7Validator(
    {'$ref': './collection-single-module.json#'},
    registry=registry,
)

MULTI_VALIDATOR = Draft7Validator(
    {'$ref': './collection-multi-module.json#'},
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
    single = []
    multi = []

    for d in COLLECTOR_SOURCES:
        if d.exists() and d.is_dir():
            single.extend(d.glob(SINGLE_PATTERN))
            multi.extend(d.glob(MULTI_PATTERN))

    return (single, multi)


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


def load_metadata():
    ret = []

    single, multi = get_collector_metadata_entries()

    for path in single:
        debug(f'Loading { path }.')
        data = load_yaml(path)

        if not data:
            continue

        try:
            SINGLE_VALIDATOR.validate(data)
        except ValidationError:
            warn(f'Failed to validate { path } against the schema.', path)
            continue

        data['integration_type'] = 'collector'
        data['_src_path'] = path
        data['_index'] = 0
        ret.append(data)

    for path in multi:
        debug(f'Loading { path }.')
        data = load_yaml(path)

        if not data:
            continue

        try:
            MULTI_VALIDATOR.validate(data)
        except ValidationError:
            warn(f'Failed to validate { path } against the schema.', path)
            continue

        for idx, item in enumerate(data['modules']):
            item['meta']['plugin_name'] = data['plugin_name']
            item['integration_type'] = 'collector'
            item['_src_path'] = path
            item['_index'] = idx
            ret.append(item)

    return ret


def make_id(meta):
    if 'monitored_instance' in meta:
        instance_name = meta['monitored_instance']['name'].replace(' ', '_')
    elif 'instance_name' in meta:
        instance_name = meta['instance_name']
    else:
        instance_name = '000_unknown'

    return f'{ meta["plugin_name"] }-{ meta["module_name"] }-{ instance_name }'


def render_keys(integrations, categories):
    debug('Computing default categories.')

    default_cats, valid_cats = get_category_sets(categories)

    debug('Filtering integration types.')

    collectors = [i for i in integrations if i['integration_type'] == 'collector']

    debug('Generating collector IDs.')

    for item in collectors:
        item['id'] = make_id(item['meta'])

    collectors.sort(key=lambda i: i['_index'])
    collectors.sort(key=lambda i: i['_src_path'])
    collectors.sort(key=lambda i: i['id'])

    ids = {i['id']: False for i in collectors}

    tmp_collectors = []

    for i in collectors:
        if ids[i['id']]:
            first_path, first_index = ids[i['id']]
            warn(f'Duplicate integration ID found at { i["_src_path"] } index { i["_index"] } (original definition at { first_path } index { first_index }), ignoring that integration.', i['_src_path'])
        else:
            tmp_collectors.append(i)
            ids[i['id']] = (i['_src_path'], i['_index'])

    collectors = tmp_collectors

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

        for key in RENDER_KEYS:
            template = get_jinja_env().get_template(f'{ key }.md')
            data = template.render(entry=item, related=related)
            item[key] = data

        del item['_src_path']
        del item['_index']

    return collectors


def render_integrations(categories, integrations):
    template = get_jinja_env().get_template('integrations.js')
    data = template.render(
        categories=json.dumps(categories),
        integrations=json.dumps(integrations),
    )
    OUTPUT_PATH.write_text(data)


def main():
    metadata = load_metadata()
    categories = load_categories()
    integrations = render_keys(metadata, categories)
    render_integrations(categories, integrations)


if __name__ == '__main__':
    sys.exit(main())
