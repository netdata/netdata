# -*- coding: utf-8 -*-
# Description:
# Author: Ilya Mashchenko (l2isbad)
# SPDX-License-Identifier: GPL-3.0+

import types

from sys import version_info

PY_VERSION = version_info[:2]

try:
    if PY_VERSION > (3, 1):
        from pyyaml3 import SafeLoader as YamlSafeLoader
    else:
        from pyyaml2 import SafeLoader as YamlSafeLoader
except ImportError:
    from yaml import SafeLoader as YamlSafeLoader


if PY_VERSION > (3, 1):
    from importlib.machinery import SourceFileLoader
    DEFAULT_MAPPING_TAG = 'tag:yaml.org,2002:map'
else:
    from imp import load_source as SourceFileLoader
    DEFAULT_MAPPING_TAG = u'tag:yaml.org,2002:map'

try:
    from collections import OrderedDict
except ImportError:
    from third_party.ordereddict import OrderedDict


def dict_constructor(loader, node):
    return OrderedDict(loader.construct_pairs(node))


YamlSafeLoader.add_constructor(DEFAULT_MAPPING_TAG, dict_constructor)


def yaml_load(stream):
    loader = YamlSafeLoader(stream)
    try:
        return loader.get_single_data()
    finally:
        loader.dispose()


def load_module(name, path):
    module = SourceFileLoader(name, path)
    if isinstance(module, types.ModuleType):
        return module
    return module.load_module()


def safe_load_module(name, path):
    try:
        return load_module(name, path), None
    except Exception as error:
        return None, error


def load_config(file_name):
    with open(file_name, 'r') as stream:
        return yaml_load(stream)


def safe_load_config(file_name):
    try:
        return load_config(file_name), None
    except Exception as error:
        return None, error
