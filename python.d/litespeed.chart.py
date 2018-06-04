# -*- coding: utf-8 -*-
# Description: litespeed netdata python.d module
# Author: Ilya Maschenko (l2isbad)

import glob
import re
import os

from collections import defaultdict

from bases.FrameworkServices.SimpleService import SimpleService


update_every = 10

# charts order (can be overridden if you want less charts, or different order)
ORDER = [
    'net_throughput_http', 'net_throughput_https',
    'requests', 'pub_cache_hits', 'priv_cache_hits', 'static_hits']

CHARTS = {
    'net_throughput_http': {
        'options': [
            None, 'Network Throughput HTTP', 'kilobytes/s', 'net throughput', 'litespeed.net_throughput', 'area'],
        'lines': [
            ["bps_in", "in", "absolute"],
            ["bps_out", "out", "absolute", -1]
        ]},
    'net_throughput_https': {
        'options': [
            None, 'Network Throughput HTTPS', 'kilobytes/s', 'net throughput', 'litespeed.net_throughput', 'area'],
        'lines': [
            ["ssl_bps_in", "in", "absolute"],
            ["ssl_bps_out", "out", "absolute", -1]
        ]},
    'requests': {
        'options': [None, 'Requests', 'req/s', 'requests', 'litespeed.requests', 'line'],
        'lines': [
            ["requests", None, "incremental"]
        ]},
    'pub_cache_hits': {
        'options': [None, 'Public Cache Hits', 'hits/s', 'cache', 'litespeed.pub_cache', 'line'],
        'lines': [
            ["pub_cache_hits", "hits", "incremental"]
        ]},
    'priv_cache_hits': {
        'options': [None, 'Private Cache Hits', 'hits/s', 'cache', 'litespeed.priv_cache', 'line'],
        'lines': [
            ["priv_cache_hits", "hits", "incremental"]
        ]},
    'static_hits': {
        'options': [None, 'Static Hits', 'hits/s', 'static', 'litespeed.static', 'line'],
        'lines': [
            ["static_hits", "hits", "incremental"]
        ]},
}

KEYS = [
    ('BPS_IN', 'bps_in'),
    ('BPS_OUT', 'bps_out'),
    ('SSL_BPS_IN', 'ssl_bps_in'),
    ('SSL_BPS_OUT', 'ssl_bps_out'),
    ('TOT_REQS', 'requests'),
    ('TOTAL_PUB_CACHE_HITS', 'pub_cache_hits'),
    ('TOTAL_PRIVATE_CACHE_HITS', 'priv_cache_hits'),
    ('TOTAL_STATIC_HITS', 'static_hits'),
]

RE = re.compile(r'([A-Z_]+): ([0-9.]+)')


class DataFile:
    def __init__(self, abs_path):
        self.path = abs_path
        self.mtime = 0

    def is_updated(self):
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
            "bps_in": 0,
            "bps_out": 0,
            "ssl_bps_in": 0,
            "ssl_bps_out": 0,
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
        data = defaultdict(int)

        for f in self.files:
            try:
                if not f.is_updated():
                    continue
                with open(f.path) as b:
                    lines = b.readlines()
            except (OSError, IOError) as err:
                self.error(err)
                return None
            else:
                parse_file(data, lines)

        if data:
            self.data = dict(data)

        return self.data


def parse_file(data, lines):
    for line in lines:
        if not line.startswith(("BPS_IN", "REQ_RATE []")):
            continue
        m = dict(RE.findall(line))
        for k, d in KEYS:
            if k in m:
                data[d] += int(m[k])


def is_readable_file(v):
    return os.path.isfile(v) and os.access(v, os.R_OK)
