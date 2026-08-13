import json
import os
from pathlib import Path

from jsonschema import Draft7Validator, FormatChecker, ValidationError
from referencing import Registry, Resource
from referencing.jsonschema import DRAFT7
from ruamel.yaml import YAML, YAMLError

from descriptions import parentheses_are_balanced


AGENT_REPO = 'netdata/netdata'

INTEGRATIONS_PATH = Path(__file__).parent
REPO_PATH = INTEGRATIONS_PATH.parent
SCHEMA_PATH = INTEGRATIONS_PATH / 'schemas'
METADATA_PATTERN = '*/metadata.yaml'

COLLECTOR_SOURCES = [
    (AGENT_REPO, REPO_PATH / 'src' / 'collectors', True),
    (AGENT_REPO, REPO_PATH / 'src' / 'collectors' / 'charts.d.plugin', True),
    (AGENT_REPO, REPO_PATH / 'src' / 'collectors' / 'ebpf.plugin' / 'ebpfgo.plugin' / 'metadata.yaml', False),
    (AGENT_REPO, REPO_PATH / 'src' / 'collectors' / 'python.d.plugin', True),
    (AGENT_REPO, REPO_PATH / 'src' / 'collectors' / 'guides', True),
    (AGENT_REPO, REPO_PATH / 'src' / 'go' / 'plugin' / 'go.d' / 'collector', True),
    (AGENT_REPO, REPO_PATH / 'src' / 'go' / 'plugin' / 'scripts.d' / 'collector', True),
    (AGENT_REPO, REPO_PATH / 'src' / 'go' / 'plugin' / 'ibm.d' / 'modules', True),
    (AGENT_REPO, REPO_PATH / 'src' / 'go' / 'plugin' / 'ibm.d' / 'modules' / 'websphere', True),
    (AGENT_REPO, REPO_PATH / 'src' / 'crates' / 'otel-plugin' / 'metadata.yaml', False),
    (AGENT_REPO, REPO_PATH / 'src' / 'crates' / 'otel-plugin' / 'taxonomy.yaml', False),
]

FLOWS_SOURCES = [
    (AGENT_REPO, REPO_PATH / 'src' / 'crates' / 'netflow-plugin' / 'metadata.yaml', False),
]

EBPFGO_PATH = REPO_PATH / 'src' / 'collectors' / 'ebpf.plugin' / 'ebpfgo.plugin'

TAXONOMY_SOURCES = [
    *COLLECTOR_SOURCES,
    *FLOWS_SOURCES,
    (AGENT_REPO, REPO_PATH / 'src' / 'crates' / 'netflow-plugin' / 'taxonomy.yaml', False),
    # ebpf-go.plugin declares one taxonomy.yaml per module.  They must be listed
    # individually because the recursive src/collectors entry only globs
    # <plugin>/taxonomy.yaml, one level deep (see discover_taxonomy_files).
    (AGENT_REPO, EBPFGO_PATH / 'taxonomy.yaml', False),
    (AGENT_REPO, EBPFGO_PATH / 'dcstat' / 'taxonomy.yaml', False),
    (AGENT_REPO, EBPFGO_PATH / 'dns' / 'taxonomy.yaml', False),
    (AGENT_REPO, EBPFGO_PATH / 'socket' / 'taxonomy.yaml', False),
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


format_checker = FormatChecker()


@format_checker.checks('netdata-balanced-parentheses')
def _check_balanced_parentheses(instance):
    return not isinstance(instance, str) or parentheses_are_balanced(instance)


def make_validator(schema_ref):
    return Draft7Validator(
        {'$ref': schema_ref},
        registry=registry,
        format_checker=format_checker,
    )


COLLECTOR_VALIDATOR = make_validator('./collector.json#')


def get_metadata_entries(sources):
    ret = []

    for r, d, m in sources:
        if d.exists() and d.is_dir() and m:
            for item in d.glob(METADATA_PATTERN):
                ret.append((r, item))
        elif d.exists() and d.is_file() and not m:
            if d.match(METADATA_PATTERN):
                ret.append((r, d))

    return ret


def get_collector_metadata_entries():
    return get_metadata_entries(COLLECTOR_SOURCES)


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


def load_collectors(sources=None):
    ret = []

    entries = get_metadata_entries(COLLECTOR_SOURCES if sources is None else sources)

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
