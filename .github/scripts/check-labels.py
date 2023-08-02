#!/usr/bin/env python3

'''
Validate GitHub label configuration.

Copyright (C) 2023 Netdata Inc.
'''

import re
import sys

from pathlib import Path

from ruamel.yaml import YAML as Yaml
from ruamel.yaml import YAMLError


SCRIPT_PATH = Path(__file__).resolve()
SCRIPT_DIR = SCRIPT_PATH.parent
SRC_DIR = SCRIPT_DIR.parent.parent


# Regex defining accepted label names.
# The limitations it describes are our own and are imposed for consistency.
LABEL_REGEX = re.compile('^([a-zA-Z0-9 ._/-])+$')

# Regex for matching six-digit hexadecimal strings.
COLOR_REGEX = re.compile('^[a-fA-F0-9]{6}$')

# These area labels are required to be present, but are not expected to
# have a corresponding source directory.
NODIR_AREAS = {
    'ci',
    'packaging/deb',
    'packaging/rpm',
}

# These collector labels are required to be present, but should not
# have directories under `collectors/`
NODIR_COLLECTORS = {
    'go.d',
    'new',
}

# These collectors should not have a label
IGNORE_COLLECTORS = {
    'checks',
}

# These collectors do not follow the usual naming convention
SPECIAL_COLLECTORS = {
    'plugins.d',
}

# These platform labels are required to be present, but do not have
# a corresponding entry in the distros.yml file.
EXTRA_PLATFORMS = {
    'docker',
    'freebsd',
    'k8s',
    'linux',
    'macos',
    'new',
    'windows',
}

YAML = Yaml(typ="safe")


def validate_labels_config(labels_config):
    '''Validate the labels config file.'''
    if not isinstance(labels_config, list):
        print('!!! Top level of labels configuration is not a list.')
        sys.exit(1)

    ret = True

    for idx, item in enumerate(labels_config):
        match item:
            case {'name': n, 'description': d, 'color': c, **other}:
                if other:
                    print(f'!!! Unrecognized keys for { n } in labels config.')
                    ret = False

                if not isinstance(n, str):
                    print(f'!!! Label name is not a string at index { idx }' +
                          ' in labels config.')
                    ret = False
                elif not LABEL_REGEX.match(n):
                    print(f'!!! Label name { n } is not valid. ' +
                          'Label names must consist only of English letters, numbers, ' +
                          '`/`, `-`, `_`, `.`, or spaces.')
                    ret = False

                if not isinstance(d, str):
                    print(f'!!! Description for label { n } is not a string.')
                    ret = False
                elif len(d) > 100:
                    print(f'!!! Description for label { n } is too long. GitHub limits descriptions to 100 characters.')
                    ret = False

                if not isinstance(c, str):
                    print(f'!!! Color for label { n } is not a string. Did you forget to quote it?')
                    ret = False
                elif not COLOR_REGEX.match(c):
                    print(f'!!! Color for label { n } is not a six-digit hexidecimal string.')
                    ret = False
            case {**other}:
                for k in {'name', 'description', 'color'}:
                    if k not in other.keys():
                        print(f'!!! Missing { k } entry at index { idx }' +
                              ' in labels config.')
                        ret = False
            case _:
                print(f'!!! Incorrect type at index { idx } in labels config.')
                sys.exit(1)

    if ret:
        print('>>> Labels config has valid syntax.')

    return ret


def validate_labeler_config(labeler_config):
    '''Validate the labeler config file.'''
    if not isinstance(labeler_config, dict):
        print('!!! Top level of labeler configuration is not a mapping.')

    ret = True

    for key, value in labeler_config.items():
        if not isinstance(key, str):
            print(f'!!! \'{ key }\' in labeler config is not a string.')
            ret = False

        if not isinstance(value, list):
            print(f'!!! Invalid type for value of { key } in labeler config.')
            ret = False
            continue

        match value:
            case [{'all': [*_], 'any': [*_]}]:
                pass
            case [{'all': [*_]}]:
                pass
            case [{'any': [*_]}]:
                pass
            case [*results] if all(map(lambda x: isinstance(x, str), results)):
                pass
            case [*results]:
                for idx, item in enumerate(results):
                    if not isinstance(item, str):
                        print(f'!!! Invalid item at index { idx } for '
                              f'{ key } in labeler configuration')
                        ret = False

    if ret:
        print('>>> Labeler config has valid syntax.')

    return ret


def check_labeler_labels(labels, labeler_config):
    '''Confirm that all labels in the labeler config exist.'''
    missing_labeler_labels = {x for x in labeler_config.keys()
                              if x not in labels}

    if missing_labeler_labels:
        print('!!! Missing labels found in labeler config:' +
              f'{ str(missing_labeler_labels) }')
        return False

    print('>>> All labels in labeler config are defined in labels config.')
    return True


def sub_labels(labels, prefix):
    '''Provide a list of sub-labels that match the given label prefix.'''
    return ['/'.join(i.split('/')[1:]) for i in labels if i.startswith(prefix)]


def check_area_labels(labels):
    '''Check the consistency of the area labels.'''
    ret = True
    areas = sub_labels(labels, 'area/')

    for area in areas:
        areapath = SRC_DIR / area
        if area in NODIR_AREAS:
            continue
        else:
            if not areapath.is_dir():
                print(f'!!! No corresponding directory found for label area/{ area }.')
                ret = False

    for area in NODIR_AREAS:
        if area not in areas:
            print(f'!!! The areas/{ area } label is not defined, but should be.')
            ret = False

    if ret:
        print('>>> All area labels appear to be valid.')

    return ret


def check_collector_labels(labels):
    '''Check the consistency of the collector labels.'''
    COLLECTOR_PREFIX = SRC_DIR / 'collectors'

    ret = True
    collectors = set(sub_labels(labels, 'collectors/'))

    collector_dirs = {d.name for d in COLLECTOR_PREFIX.iterdir() if d.is_dir()}
    collector_dirs = {
        (d if d in SPECIAL_COLLECTORS else '.'.join(d.split('.')[:-1])) for d in collector_dirs
    } | NODIR_COLLECTORS

    missing_labels = collector_dirs - collectors - IGNORE_COLLECTORS
    missing_dirs = collectors - collector_dirs

    if missing_labels:
        print(f'!!! The following collectors do not have associated labels: { missing_labels }.')
        ret = False

    if missing_dirs:
        print(f'!!! The following collector labels do not have associated code: { missing_dirs }.')
        ret = False

    if ret:
        print('>>> All collector labels appear to be correct.')

    return ret


def alias_platform(plat):
    '''Translate from a distro name to a platform label name.'''
    match plat:
        case 'almalinux': return 'linux/rhel'
        case 'centos': return 'linux/rhel'
        case 'opensuse': return 'linux/suse'
        case 'oraclelinux': return 'linux/oracle'
        case plat if plat in EXTRA_PLATFORMS: return plat
        case _: return f'linux/{ plat }'


def check_platform_labels(labels):
    '''Check the consistency of the platform labels.'''
    ret = True
    platforms = set(sub_labels(labels, 'platform/'))

    try:
        distros_config = YAML.load(SRC_DIR / '.github' / 'data' / 'distros.yml')
    except (YAMLError, IOError, OSError):
        print('!!! Failed to load distros configuration file.')
        sys.exit(1)

    distros = {d['distro'] for d in distros_config['include']} | EXTRA_PLATFORMS
    distros = {alias_platform(d) for d in distros}

    missing_labels = distros - platforms
    missing_distros = platforms - distros

    if missing_labels:
        print(f'!!! The following platforms do not have associated labels: { missing_labels }.')
        ret = False

    if missing_distros:
        print(f'!!! The following platform labels do not have associated platform configs: { missing_distros }.')
        ret = False

    if ret:
        print('>>> All platform labels appear to be correct.')

    return ret


try:
    LABELS_CONFIG = YAML.load(SRC_DIR / '.github' / 'labels.yml')
except (YAMLError, IOError, OSError):
    print('!!! Failed to load labels configuration file.')
    sys.exit(1)

try:
    LABELER_CONFIG = YAML.load(SRC_DIR / '.github' / 'labeler.yml')
except (YAMLError, IOError, OSError):
    print('!!! Failed to load labeler configuration file.')
    sys.exit(1)

if not all([
        validate_labels_config(LABELS_CONFIG),
        validate_labeler_config(LABELER_CONFIG),
       ]):
    sys.exit(1)

LABELS = [x['name'] for x in LABELS_CONFIG]

if not all([
        check_labeler_labels(LABELS, LABELER_CONFIG),
        check_area_labels(LABELS),
        check_collector_labels(LABELS),
        check_platform_labels(LABELS),
       ]):
    sys.exit(1)
