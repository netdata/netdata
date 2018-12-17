# -*- coding: utf-8 -*-
# Description: powerdns netdata python.d module
# Author: Ilya Mashchenko (l2isbad)
# Author: Luke Whitworth
# SPDX-License-Identifier: GPL-3.0-or-later

from json import loads

from bases.FrameworkServices.UrlService import UrlService


ORDER = [
    'questions',
    'cache_usage',
    'cache_size',
    'latency',
]

CHARTS = {
    'questions': {
        'options': [None, 'PowerDNS Queries and Answers', 'count', 'questions', 'powerdns.questions', 'line'],
        'lines': [
            ['udp-queries', None, 'incremental'],
            ['udp-answers', None, 'incremental'],
            ['tcp-queries', None, 'incremental'],
            ['tcp-answers', None, 'incremental']
        ]
    },
    'cache_usage': {
        'options': [None, 'PowerDNS Cache Usage', 'count', 'cache', 'powerdns.cache_usage', 'line'],
        'lines': [
            ['query-cache-hit', None, 'incremental'],
            ['query-cache-miss', None, 'incremental'],
            ['packetcache-hit', 'packet-cache-hit', 'incremental'],
            ['packetcache-miss', 'packet-cache-miss', 'incremental']
        ]
    },
    'cache_size': {
        'options': [None, 'PowerDNS Cache Size', 'count', 'cache', 'powerdns.cache_size', 'line'],
        'lines': [
            ['query-cache-size', None, 'absolute'],
            ['packetcache-size', 'packet-cache-size', 'absolute'],
            ['key-cache-size', None, 'absolute'],
            ['meta-cache-size', None, 'absolute']
        ]
    },
    'latency': {
        'options': [None, 'PowerDNS Latency', 'microseconds', 'latency', 'powerdns.latency', 'line'],
        'lines': [
            ['latency', None, 'absolute']
        ]
    }
}

RECURSOR_ORDER = ['questions-in', 'questions-out', 'answer-times', 'timeouts', 'drops', 'cache_usage', 'cache_size']

RECURSOR_CHARTS = {
    'questions-in': {
        'options': [None, 'PowerDNS Recursor Questions In', 'count', 'questions', 'powerdns_recursor.questions-in',
                    'line'],
        'lines': [
            ['questions', None, 'incremental'],
            ['ipv6-questions', None, 'incremental'],
            ['tcp-questions', None, 'incremental']
        ]
    },
    'questions-out': {
        'options': [None, 'PowerDNS Recursor Questions Out', 'count', 'questions', 'powerdns_recursor.questions-out',
                    'line'],
        'lines': [
            ['all-outqueries', None, 'incremental'],
            ['ipv6-outqueries', None, 'incremental'],
            ['tcp-outqueries', None, 'incremental'],
            ['throttled-outqueries', None, 'incremental']
        ]
    },
    'answer-times': {
        'options': [None, 'PowerDNS Recursor Answer Times', 'count', 'performance', 'powerdns_recursor.answer-times',
                    'line'],
        'lines': [
            ['answers-slow', None, 'incremental'],
            ['answers0-1', None, 'incremental'],
            ['answers1-10', None, 'incremental'],
            ['answers10-100', None, 'incremental'],
            ['answers100-1000', None, 'incremental']
        ]
    },
    'timeouts': {
        'options': [None, 'PowerDNS Recursor Questions Time', 'count', 'performance', 'powerdns_recursor.timeouts',
                    'line'],
        'lines': [
            ['outgoing-timeouts', None, 'incremental'],
            ['outgoing4-timeouts', None, 'incremental'],
            ['outgoing6-timeouts', None, 'incremental']
        ]
    },
    'drops': {
        'options': [None, 'PowerDNS Recursor Drops', 'count', 'performance', 'powerdns_recursor.drops', 'line'],
        'lines': [
            ['over-capacity-drops', None, 'incremental']
        ]
    },
    'cache_usage': {
        'options': [None, 'PowerDNS Recursor Cache Usage', 'count', 'cache', 'powerdns_recursor.cache_usage', 'line'],
        'lines': [
            ['cache-hits', None, 'incremental'],
            ['cache-misses', None, 'incremental'],
            ['packetcache-hits', 'packet-cache-hit', 'incremental'],
            ['packetcache-misses', 'packet-cache-miss', 'incremental']
        ]
    },
    'cache_size': {
        'options': [None, 'PowerDNS Recursor Cache Size', 'count', 'cache', 'powerdns_recursor.cache_size', 'line'],
        'lines': [
            ['cache-entries', None, 'absolute'],
            ['packetcache-entries', None, 'absolute'],
            ['negcache-entries', None, 'absolute']
        ]
    }
}


class Service(UrlService):
    def __init__(self, configuration=None, name=None):
        UrlService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS

    def check(self):
        self._manager = self._build_manager()
        if not self._manager:
            return None

        d = self._get_data()
        if not d:
            return False

        if is_recursor(d):
            self.order = RECURSOR_ORDER
            self.definitions = RECURSOR_CHARTS
            self.module_name = 'powerdns_recursor'

        return True

    def _get_data(self):
        data = self._get_raw_data()
        if not data:
            return None
        return dict((d['name'], d['value']) for d in loads(data))


def is_recursor(d):
    return 'over-capacity-drops' in d and 'tcp-questions' in d
