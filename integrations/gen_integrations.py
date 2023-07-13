#!/usr/bin/env python3

import json
import sys

from pathlib import Path

import jinja2

from jsonschema import Draft7Validator, ValidationError
from referencing import Registry, Resource
from referencing.jsonschema import DRAFT7
from ruamel.yaml import YAML, YAMLError

INTEGRATIONS_PATH = Path(__file__).parent
TEMPLATE_PATH = INTEGRATIONS_PATH / 'templates'
OUTPUT_PATH = INTEGRATIONS_PATH / 'integrations.js'
CATEGORIES_FILE = INTEGRATIONS_PATH / 'categories.yaml'
REPO_PATH = INTEGRATIONS_PATH.parent
SCHEMA_PATH = REPO_PATH / 'collectors' / 'metadata' / 'schemas'
GO_REPO_PATH = REPO_PATH / 'go.d.plugin'
SINGLE_PATTERN = '*/metadata.yaml'
MULTI_PATTERN = '*/multi_metadata.yaml'

SOURCES = [
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


def retrieve_from_filesystem(uri):
    path = SCHEMA_PATH / Path(uri)
    contents = json.loads(path.read_text())
    return Resource.from_contents(contents, DRAFT7)


registry = Registry(retrieve=retrieve_from_filesystem)

SINGLE_VALIDATOR = Draft7Validator(
    {'$ref': './single-module.json#'},
    registry=registry,
)

MULTI_VALIDATOR = Draft7Validator(
    {'$ref': './multi-module.json#'},
    registry=registry,
)

JINJA_ENV = jinja2.Environment(
    loader=jinja2.FileSystemLoader(TEMPLATE_PATH),
    autoescape=jinja2.select_autoescape(),
    block_start_string='[%',
    block_end_string='%]',
    variable_start_string='[[',
    variable_end_string=']]',
    comment_start_string='[#',
    comment_end_string='#]',
    trim_blocks=True,
    lstrip_blocks=True,
)


def get_metadata_entries():
    single = []
    multi = []

    for d in SOURCES:
        if d.exists() and d.is_dir():
            single.extend(d.glob(SINGLE_PATTERN))
            multi.extend(d.glob(MULTI_PATTERN))

    return (single, multi)


def load_yaml(src):
    yaml = YAML(typ='safe')

    if not src.is_file():
        print(f':warning file={ src }:{ src } is not a file.')
        return False

    try:
        contents = src.read_text()
    except (IOError, OSError):
        print(f':warning file={ src }:Failed to load { src }.')
        return False

    try:
        data = yaml.load(contents)
    except YAMLError:
        print(f':warning file={ src }:Failed to parse { src } as YAML.')
        return False

    return data


def load_metadata():
    ret = []

    single, multi = get_metadata_entries()

    for path in single:
        print(f':debug:Loading { path }.')
        data = load_yaml(path)

        if not data:
            continue

        try:
            SINGLE_VALIDATOR.validate(data)
        except ValidationError:
            print(f':warning file={ path }:Failed to validate { path } against the schema.')
            continue

        ret.append(data)

    for path in multi:
        print(f':debug:Loading { path }.')
        data = load_yaml(path)

        if not data:
            continue

        try:
            MULTI_VALIDATOR.validate(data)
        except ValidationError:
            print(f':warning file={ path }:Failed to validate { path } against the schema.')
            continue

        for item in data['modules']:
            item['meta']['plugin_name'] = data['plugin_name']
            ret.append(item)

    return ret


def render_keys(integrations):
    for item in integrations:
        item['id'] = f'{ item["meta"]["plugin_name"] }-{ item["meta"]["module_name"] }'

        print(f':debug:Processing { item["id"] }')

        for scope in item['metrics']['scopes']:
            if scope['name'] == 'global':
                scope['name'] = f'{ item["meta"]["monitored_instance"]["name"] } instance'

        for key in RENDER_KEYS:
            template = JINJA_ENV.get_template(f'{ key }.md')
            data = template.render(entry=item)
            item[key] = data

    return integrations


def render_integrations(categories, integrations):
    template = JINJA_ENV.get_template('integrations.js')
    data = template.render(
        categories=json.dumps(categories),
        integrations=json.dumps(integrations),
    )
    OUTPUT_PATH.write_text(data)


def main():
    metadata = load_metadata()
    integrations = render_keys(metadata)
    categories = load_yaml(CATEGORIES_FILE)
    render_integrations(categories, integrations)


if __name__ == '__main__':
    sys.exit(main())
