import json
import os
from pathlib import Path

from jsonschema import Draft7Validator, ValidationError
from referencing import Registry, Resource
from referencing.jsonschema import DRAFT7
from ruamel.yaml import YAML, YAMLError


AGENT_REPO = 'netdata/netdata'

INTEGRATIONS_PATH = Path(__file__).parent
REPO_PATH = INTEGRATIONS_PATH.parent
SCHEMA_PATH = INTEGRATIONS_PATH / 'schemas'
METADATA_PATTERN = '*/metadata.yaml'

COLLECTOR_SOURCES = [
    (AGENT_REPO, REPO_PATH / 'src' / 'collectors', True),
    (AGENT_REPO, REPO_PATH / 'src' / 'collectors' / 'charts.d.plugin', True),
    (AGENT_REPO, REPO_PATH / 'src' / 'collectors' / 'python.d.plugin', True),
    (AGENT_REPO, REPO_PATH / 'src' / 'collectors' / 'guides', True),
    (AGENT_REPO, REPO_PATH / 'src' / 'go' / 'plugin' / 'go.d' / 'collector', True),
    (AGENT_REPO, REPO_PATH / 'src' / 'go' / 'plugin' / 'scripts.d' / 'collector', True),
    (AGENT_REPO, REPO_PATH / 'src' / 'go' / 'plugin' / 'ibm.d' / 'modules', True),
    (AGENT_REPO, REPO_PATH / 'src' / 'go' / 'plugin' / 'ibm.d' / 'modules' / 'websphere', True),
    (AGENT_REPO, REPO_PATH / 'src' / 'crates' / 'netdata-otel', True),
]

GITHUB_ACTIONS = os.environ.get('GITHUB_ACTIONS', False)
DEBUG = os.environ.get('DEBUG', False)
WARNINGS = []


def debug(msg):
    if GITHUB_ACTIONS:
        print(f'::debug::{msg}')
    elif DEBUG:
        print(f'>>> {msg}')
    else:
        pass


def warn(msg, path):
    WARNINGS.append((str(path), msg))

    if GITHUB_ACTIONS:
        print(f'::warning file={path}::{msg}')
    else:
        print(f'!!! WARNING:{path}:{msg}')


def fail_on_warnings():
    if not WARNINGS:
        return 0

    warned_files = sorted({path for path, _ in WARNINGS})
    print(f'::error::Integrations generation failed with {len(WARNINGS)} warning(s) across {len(warned_files)} file(s).')

    for path in warned_files:
        print(f'::error file={path}::Metadata warnings in this file are now fatal for integrations generation.')

    return 1


def retrieve_from_filesystem(uri):
    path = SCHEMA_PATH / Path(uri)
    contents = json.loads(path.read_text())
    return Resource.from_contents(contents, DRAFT7)


registry = Registry(retrieve=retrieve_from_filesystem)


def make_validator(schema_ref):
    return Draft7Validator(
        {'$ref': schema_ref},
        registry=registry,
    )


COLLECTOR_VALIDATOR = make_validator('./collector.json#')


def get_collector_metadata_entries():
    ret = []

    for r, d, m in COLLECTOR_SOURCES:
        if d.exists() and d.is_dir() and m:
            for item in d.glob(METADATA_PATTERN):
                ret.append((r, item))
        elif d.exists() and d.is_file() and not m:
            if d.match(METADATA_PATTERN):
                ret.append((r, d))

    return ret


def load_yaml(src):
    yaml = YAML(typ='safe')

    if not src.is_file():
        warn(f'{src} is not a file.', src)
        return False

    try:
        contents = src.read_text()
    except (IOError, OSError):
        warn(f'Failed to read {src}.', src)
        return False

    try:
        data = yaml.load(contents)
    except YAMLError:
        warn(f'Failed to parse {src} as YAML.', src)
        return False

    return data


def load_collectors():
    ret = []

    entries = get_collector_metadata_entries()

    for repo, path in entries:
        debug(f'Loading {path}.')
        data = load_yaml(path)

        if not data:
            continue

        try:
            COLLECTOR_VALIDATOR.validate(data)
        except ValidationError as e:
            warn(
                f'Failed to validate {path} against the schema: {e.message} (path: {"/".join(str(p) for p in e.absolute_path)})',
                path)
            continue

        for idx, item in enumerate(data['modules']):
            item['meta']['plugin_name'] = data['plugin_name']
            item['integration_type'] = 'collector'
            item['_src_path'] = path
            item['_repo'] = repo
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

    return f'{meta["plugin_name"]}-{meta["module_name"]}-{instance_name}'
