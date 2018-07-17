# -*- coding: utf-8 -*-
# Description: powerdns_recursor netdata python.d module
# Author: Luke Whitworth
# SPDX-License-Identifier: GPL-3.0+

from json import loads

from bases.FrameworkServices.UrlService import UrlService

priority = 60000
retries = 60
# update_every = 3

ORDER = ['questions-in', 'questions-out', 'answer-times', 'timeouts', 'drops', 'cache_usage', 'cache_size']
CHARTS = {
    'questions-in': {
        'options': [None, 'PowerDNS Recursor Questions In', 'count', 'questions', 'powerdns_recursor.questions-in', 'line'],
        'lines': [
            ['questions', None, 'incremental'],
            ['ipv6-questions', None, 'incremental'],
            ['tcp-questions', None, 'incremental']
        ]},
    'questions-out': {
        'options': [None, 'PowerDNS Recursor Questions Out', 'count', 'questions', 'powerdns_recursor.questions-out', 'line'],
        'lines': [
            ['all-outqueries', None, 'incremental'],
            ['ipv6-outqueries', None, 'incremental'],
            ['tcp-outqueries', None, 'incremental'],
            ['throttled-outqueries', None, 'incremental']
        ]},
   'answer-times': {
        'options': [None, 'PowerDNS Recursor Answer Times', 'count', 'performance', 'powerdns_recursor.answer-times', 'line'],
        'lines': [
            ['answers-slow', None, 'incremental'],
            ['answers0-1', None, 'incremental'],
            ['answers1-10', None, 'incremental'],
            ['answers10-100', None, 'incremental'],
            ['answers100-1000', None, 'incremental']
        ]},
   'timeouts': {
        'options': [None, 'PowerDNS Recursor Questions Time', 'count', 'performance', 'powerdns_recursor.timeouts', 'line'],
        'lines': [
            ['outgoing-timeouts', None, 'incremental'],
            ['outgoing4-timeouts', None, 'incremental'],
            ['outgoing6-timeouts', None, 'incremental']
        ]},
   'drops': {
        'options': [None, 'PowerDNS Recursor Drops', 'count', 'performance', 'powerdns_recursor.drops', 'line'],
        'lines': [
            ['over-capacity-drops', None, 'incremental']
        ]},
    'cache_usage': {
        'options': [None, 'PowerDNS Recursor Cache Usage', 'count', 'cache', 'powerdns_recursor.cache_usage', 'line'],
        'lines': [
            ['cache-hits', None, 'incremental'],
            ['cache-misses', None, 'incremental'],
            ['packetcache-hits', 'packet-cache-hit', 'incremental'],
            ['packetcache-misses', 'packet-cache-miss', 'incremental']
        ]},
    'cache_size': {
        'options': [None, 'PowerDNS Recursor Cache Size', 'count', 'cache', 'powerdns_recursor.cache_size', 'line'],
        'lines': [
            ['cache-entries', None, 'absolute'],
            ['packetcache-entries', None, 'absolute'],
            ['negcache-entries', None, 'absolute']
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
