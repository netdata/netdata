# -*- coding: utf-8 -*-
# Description: powerdns netdata python.d module
# Author: l2isbad
# SPDX-License-Identifier: GPL-3.0+

from json import loads

from bases.FrameworkServices.UrlService import UrlService

priority = 60000
retries = 60
# update_every = 3

ORDER = ['questions', 'cache_usage', 'cache_size', 'latency']
CHARTS = {
    'questions': {
        'options': [None, 'PowerDNS Queries and Answers', 'count', 'questions', 'powerdns.questions', 'line'],
        'lines': [
            ['udp-queries', None, 'incremental'],
            ['udp-answers', None, 'incremental'],
            ['tcp-queries', None, 'incremental'],
            ['tcp-answers', None, 'incremental']
        ]},
    'cache_usage': {
        'options': [None, 'PowerDNS Cache Usage', 'count', 'cache', 'powerdns.cache_usage', 'line'],
        'lines': [
            ['query-cache-hit', None, 'incremental'],
            ['query-cache-miss', None, 'incremental'],
            ['packetcache-hit', 'packet-cache-hit', 'incremental'],
            ['packetcache-miss', 'packet-cache-miss', 'incremental']
        ]},
    'cache_size': {
        'options': [None, 'PowerDNS Cache Size', 'count', 'cache', 'powerdns.cache_size', 'line'],
        'lines': [
            ['query-cache-size', None, 'absolute'],
            ['packetcache-size', 'packet-cache-size', 'absolute'],
            ['key-cache-size', None, 'absolute'],
            ['meta-cache-size', None, 'absolute']
        ]},
    'latency': {
        'options': [None, 'PowerDNS Latency', 'microseconds', 'latency', 'powerdns.latency', 'line'],
        'lines': [
            ['latency', None, 'absolute']
        ]}

}


class Service(UrlService):
    def __init__(self, configuration=None, name=None):
        UrlService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS

    def _get_data(self):
        data = self._get_raw_data()
        if not data:
            return None
        return dict((d['name'], d['value']) for d in loads(data))
