# -*- coding: utf-8 -*-
# Description: litespeed netdata python.d module
# Author: Ilya Maschenko (l2isbad)

import glob
import re
import os

from bases.FrameworkServices.SimpleService import SimpleService


update_every = 10

# charts order (can be overridden if you want less charts, or different order)
ORDER = ['requests', 'pub_cache_hits', 'priv_cache_hits', 'static_hits']

CHARTS = {
    'requests': {
        'options': [None, 'Requests', 'req/sec', 'requests', 'litespeed.requests', 'line'],
        'lines': [
            ["requests", None, "incremental", 1, 100]
        ]},
    'pub_cache_hits': {
        'options': [None, 'Public Cache Hits', 'hits/sec', 'cache', 'litespeed.pub_cache', 'line'],
        'lines': [
            ["pub_cache_hits", "hits", "incremental", 1, 100]
        ]},
    'priv_cache_hits': {
        'options': [None, 'Private Cache Hits', 'hits/sec', 'cache', 'litespeed.priv_cache', 'line'],
        'lines': [
            ["priv_cache_hits", "hits", "incremental", 1, 100]
        ]},
    'static_hits': {
        'options': [None, 'Static Hits', 'hits/sec', 'static', 'litespeed.static', 'line'],
        'lines': [
            ["static_hits", "hits", "incremental", 1, 100]
        ]},
}

KEYS = [
    ('TOT_REQS', 'requests'),
    ('TOTAL_PUB_CACHE_HITS', 'pub_cache_hits'),
    ('TOTAL_PRIVATE_CACHE_HITS', 'priv_cache_hits'),
    ('TOTAL_STATIC_HITS', 'static_hits'),
]

RE = re.compile(r'([A-Z_]+): ([0-9.]+)')


class DataFile:
    def __init__(self, abs_path):
        self.path = abs_path
        self.mtime = os.stat(abs_path).st_mtime

    def is_changed(self):
        o = self.mtime
        n = os.stat(self.path).st_mtime
        self.mtime = n
        return o != n


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.path = self.configuration.get('path')
        self.files = list()
        self.data = {
            "requests": 0,
            "pub_cache_hits": 0,
            "priv_cache_hits": 0,
            "static_hits": 0,
        }

    def check(self):
        if not self.path:
            self.error('"path" not specified')
            return False

        fs = glob.glob(os.path.join(self.path, ".rtreport*"))

        if not fs:
            self.error('"{0}" has no "rtreport" files or dir is not readable'.format(self.path))
            return None

        self.debug("stats files:", fs)

        for f in fs:
            if not is_readable_file(f):
                self.error("{0} is not readable".format(f))
                continue
            self.files.append(DataFile(f))

        return bool(self.files)

    def get_data(self):
        """
        Format data received from http request
        :return: dict
        """
        for f in self.files:
            try:
                if not f.is_changed():
                    continue
                with open(f.path) as b:
                    lines = b.readlines()
            except (OSError, IOError) as err:
                self.error(err)
                return None
            else:
                self.parse_file(lines)

        return self.data

    def parse_file(self, lines):
        for line in lines:
            if not line.startswith("REQ_RATE []"):
                continue
            m = dict(RE.findall(line))
            for k, d in KEYS:
                self.data[d] = float(m[k]) * 100 if k in m else 0
            break


def is_readable_file(v):
    return os.path.isfile(v) and os.access(v, os.R_OK)
