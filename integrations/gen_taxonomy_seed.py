#!/usr/bin/env python3

import argparse
import sys
from pathlib import Path

from ruamel.yaml import YAML

from _common import load_yaml


def contexts_from_metadata(metadata, module_name):
    modules = [
        module for module in metadata.get('modules', [])
        if module.get('meta', {}).get('module_name') == module_name
    ]
    contexts = []
    for module in modules:
        for scope in module.get('metrics', {}).get('scopes', []):
            for metric in scope.get('metrics', []):
                name = metric.get('name')
                if name and name not in contexts:
                    contexts.append(name)
    return contexts


def default_module_name(metadata):
    modules = metadata.get('modules', [])
    if len(modules) != 1:
        return None
    return modules[0]['meta']['module_name']


def monitored_title(metadata, module_name):
    for module in metadata.get('modules', []):
        if module.get('meta', {}).get('module_name') == module_name:
            return module.get('meta', {}).get('monitored_instance', {}).get('name', module_name)
    return module_name


def build_seed(metadata_path, module_name, section_id, placement_id, icon):
    metadata = load_yaml(metadata_path)
    if not metadata:
        raise SystemExit(f'Failed to load {metadata_path}')

    if not module_name:
        module_name = default_module_name(metadata)
    if not module_name:
        raise SystemExit('metadata.yaml has multiple modules; pass --module-name explicitly.')

    contexts = contexts_from_metadata(metadata, module_name)
    if not contexts:
        raise SystemExit(f'No metric contexts found for module {module_name}.')

    if not placement_id:
        placement_id = module_name.replace('_', '-')
    title = monitored_title(metadata, module_name)

    placement = {
        'id': placement_id,
        'section_id': section_id,
        'title': title,
        'items': contexts,
    }
    if icon:
        placement['icon'] = icon

    return {
        'taxonomy_version': 1,
        'plugin_name': metadata['plugin_name'],
        'module_name': module_name,
        'placements': [placement],
    }


def main():
    parser = argparse.ArgumentParser(description='Seed a collector taxonomy.yaml from metadata.yaml contexts.')
    parser.add_argument('metadata_yaml', type=Path)
    parser.add_argument('--module-name', help='metadata.yaml module name to seed.')
    parser.add_argument('--section-id', default='TODO.section', help='Initial section_id value.')
    parser.add_argument('--placement-id', help='Initial placement id. Defaults to module-name with underscores replaced.')
    parser.add_argument('--icon', help='Optional icon id.')
    parser.add_argument('--output', type=Path, help='Write to this path instead of stdout.')
    args = parser.parse_args()

    seed = build_seed(args.metadata_yaml, args.module_name, args.section_id, args.placement_id, args.icon)
    yaml = YAML()
    yaml.default_flow_style = False
    yaml.indent(mapping=2, sequence=4, offset=2)

    if args.output:
        with args.output.open('w') as fp:
            yaml.dump(seed, fp)
    else:
        yaml.dump(seed, sys.stdout)

    return 0


if __name__ == '__main__':
    sys.exit(main())
