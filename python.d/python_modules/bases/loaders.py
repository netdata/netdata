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


class YamlOrderedLoader:
    @staticmethod
    def load_config_from_file(file_name):
        opened, loaded = False, False
        try:
            stream = open(file_name, 'r')
            opened = True
            loader = YamlSafeLoader(stream)
            loaded = True
            parsed = loader.get_single_data() or dict()
        except Exception as error:
            return dict(), error
        else:
            return parsed, None
        finally:
            if opened:
                stream.close()
            if loaded:
                loader.dispose()


class SourceLoader:
    @staticmethod
    def load_module_from_file(name, path):
        try:
            loaded = SourceFileLoader(name, path)
            if isinstance(loaded, types.ModuleType):
                return loaded, None
            return loaded.load_module(), None
        except Exception as error:
            return None, error


class ModuleAndConfigLoader(YamlOrderedLoader, SourceLoader):
    pass
