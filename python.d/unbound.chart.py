# -*- coding: utf-8 -*-
# Description: unbound netdata python.d module
# Author: Austin S. Hemmelgarn (Ferroin)

import os
import yaml

from base.FrameworkServices.SocketService import SocketService


ORDER = ['queries', 'reqlist', 'recursion']

CHARTS = {
    'queries': {
        'options': [None, 'Queries Processed', 'queries', 'Unbound', 'unbound.queries', 'line'],
        'lines': [
            ['ratelimit', 'Ratelimited', 'absolute', 1, 1],
            ['cachemiss', 'Cache Miss', 'absolute', 1, 1],
            ['cachehit', 'Cache Hit', 'absolute', 1, 1],
            ['expired', 'Expired', 'absolute', 1, 1],
            ['prefetch', 'Prefetched', 'absolute', 1, 1],
            ['recursive', 'Recursive', 'absolute', 1, 1]
        ]
    },
    'reqlist': {
        'options': [None, 'Request List', 'items', 'Unbound', 'unbound.reqlist', 'line'],
        'lines': [
            ['reqlist_avg', 'Average Size', 'absolute', 1, 1],
            ['reqlist_max', 'Maximum Size', 'absolute', 1, 1],
            ['reqlist_overwritten', 'Overwritten Requests', 'absolute', 1, 1],
            ['reqlist_exceeded', 'Overruns', 'absolute', 1, 1],
            ['reqlist_current', 'Current Size', 'absolute', 1, 1],
            ['reqlist_user', 'User Requests', 'absolute', 1, 1]
        ]
    },
    'recursion': {
        'options': [None, 'Recursion Timings', 'seconds', 'Unbound', 'unbound.recursion', 'line'],
        'lines': [
            ['recursive_avg', 'Average', 'absolute', 1, 1],
            ['recursive_med', 'Median', 'absolute', 1, 1]
        ]
    }
}

# These get added too if we are told to use extended stats.
EXTENDED_ORDER = ['cache']

EXTENDED_CHARTS = {
    'cache': {
        'options': [None, 'Cache Sizes', 'items', 'Unbound', 'unbound.cache', 'line'],
        'lines': [
            ['cache_message', 'Message Cache', 'absolute', 1, 1],
            ['cache_rrset', 'RRSet Cache', 'absolute', 1, 1],
            ['cache_infra', 'Infra Cache', 'absolute', 1, 1],
            ['cache_key', 'DNSSEC Key Cache', 'absolute', 1, 1],
            ['cache_dnscss', 'DNSCrypt Shared Secret Cache', 'absolute', 1, 1],
            ['cache_dnscn', 'DNSCrypt Nonce Cache', 'absolute', 1, 1]
        ]
    }
}

# This maps the Unbound stat names to our names.
STAT_MAP = {
    'total.num.queries_ip_ratelimited': 'ratelimit',
    'total.num.cachehits': 'cachehit',
    'total.num.cachemiss': 'cachemiss',
    'total.num.zero_ttl': 'expired',
    'total.num.prefetch': 'prefetch',
    'total.num.recursivereplies': 'recursive',
    'total.requestlist.avg': 'reqlist_avg',
    'total.requestlist.max': 'reqlist_max',
    'total.requestlist.overwritten': 'reqlist_overwritten',
    'total.requestlist.exceeded': 'reqlist_exceeded',
    'total.requestlist.current.all': 'reqlist_current',
    'total.requestlist.current.user': 'reqlist_user',
    'total.recursion.time.avg': 'recursive_avg',
    'total.recursion.time.median': 'recursive_med',
    'msg.cache.count': 'cache_message',
    'rrset.cache.count': 'cache_rrset',
    'infra.cache.count': 'cache_infra',
    'key.cache.count': 'cache_key',
    'dnscrypt_shared_secret.cache.count': 'cache_dnscss',
    'dnscrypt_nonce.cache.count': 'cache_dnscn'
}


class Service(SocketService):
    def __init__(self, configuration=None, name=None):
        # The unbound control protocol is always SSL encapsulated
        # unless it's used over a UNIX socket, so enable SSL _before_
        # doing the normal SocketService initialization.
        configuration['ssl'] = True
        self.port = 8935
        SocketService.__init__(self, configuration, name)
        self.ext = self.configuration.get('extended', None)
        self.ubconf = self.configuration.get('ubconf', '/etc/unbound/unbound.conf')
        self.order = ORDER
        self.definitions = CHARTS
        if self.ext:
            self.order = self.order + EXTENDED_ORDER
            self.definitions.update(EXTENDED_CHARTS)
        self.request = 'UBCT1 stats\n'
        self._parse_config()
        self.debug('Unbound config: {0}'.format(self.ubconf))
        if os.access(self.ubconf, os.R_OK):
            with open(self.ubconf, 'r') as ubconf:
                try:
                    conf = yaml.load(ubconf)
                except yaml.YAMLError:
                    conf = dict()
            if self.ext is None:
                if 'extended-statistics' in conf['server'].keys():
                    self.ext = conf['server']['extended-statistics']
            if 'remote-control' in conf.keys():
                if conf['remote-control'].get('control-use-cert', False):
                    if not self.key:
                        self.key = conf['remote-control'].get('control-key-file', '/etc/unbound/unbound_control.key')
                    if not self.cert:
                        self.cert = conf['remote-control'].get('control-cert-file', '/etc/unbound/unbound_control.pem')
                    self.port = conf['remote-control'].get('control-port', 8953)
                else:
                    if not self.unix_socket:
                        self.unix_socket = conf['remote-control'].get('control-interface')
        self.debug('Extended stats: {0}'.format(self.ext))
        if self.unix_socket:
            self.debug('Using unix socket: {0}'.format(self.unix_socket))
            for key in self.definitions.keys():
                self.definitions[key]['options'][4] = 'Local'
        else:
            self.debug('Connecting to: {0}:{1}'.format(self.host, self.port))
            self.debug('Using key: {0}'.format(self.key))
            self.debug('Using certificate: {0}'.format(self.cert))
            for key in self.definitions.keys():
                self.definitions[key]['options'][4] = self.host


    def check(self):
        # We need to check that auth works, otherwise there's no point.
        self._connect()
        self._disconnect()
        return bool(self._sock)

    def _check_raw_data(self, data):
        # The server will close the connection when it's done sending
        # data, so just keep looping until that happens.
        return False

    def _get_data(self):
        raw = self._get_raw_data()
        data = dict()
        tmp = dict()
        for line in raw.splitlines():
            stat = line.split('=')
            tmp[stat[0]] = stat[1]
        for item in STAT_MAP.keys():
            if item in tmp.keys():
                data[STAT_MAP[item]] = float(tmp[item])
        return data
