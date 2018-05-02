# -*- coding: utf-8 -*-
# Description: unbound netdata python.d module
# Author: Austin S. Hemmelgarn (Ferroin)

import os

from bases.FrameworkServices.SocketService import SocketService
from bases.loaders import YamlOrderedLoader

PRECISION = 1000

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
            ['recursive_avg', 'Average', 'absolute', 1, PRECISION],
            ['recursive_med', 'Median', 'absolute', 1, PRECISION]
        ]
    }
}

# These get added too if we are told to use extended stats.
EXTENDED_ORDER = ['cache']

EXTENDED_CHARTS = {
    'cache': {
        'options': [None, 'Cache Sizes', 'items', 'Unbound', 'unbound.cache', 'stacked'],
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

# This maps the Unbound stat names to our names and precision requiremnets.
STAT_MAP = {
    'total.num.queries_ip_ratelimited': ('ratelimit', 1),
    'total.num.cachehits': ('cachehit', 1),
    'total.num.cachemiss': ('cachemiss', 1),
    'total.num.zero_ttl': ('expired', 1),
    'total.num.prefetch': ('prefetch', 1),
    'total.num.recursivereplies': ('recursive', 1),
    'total.requestlist.avg': ('reqlist_avg', 1),
    'total.requestlist.max': ('reqlist_max', 1),
    'total.requestlist.overwritten': ('reqlist_overwritten', 1),
    'total.requestlist.exceeded': ('reqlist_exceeded', 1),
    'total.requestlist.current.all': ('reqlist_current', 1),
    'total.requestlist.current.user': ('reqlist_user', 1),
    'total.recursion.time.avg': ('recursive_avg', PRECISION),
    'total.recursion.time.median': ('recursive_med', PRECISION),
    'msg.cache.count': ('cache_message', 1),
    'rrset.cache.count': ('cache_rrset', 1),
    'infra.cache.count': ('cache_infra', 1),
    'key.cache.count': ('cache_key', 1),
    'dnscrypt_shared_secret.cache.count': ('cache_dnscss', 1),
    'dnscrypt_nonce.cache.count': ('cache_dnscn', 1)
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
        self.ubconf = self.configuration.get('ubconf', None)
        self.order = ORDER
        self.definitions = CHARTS
        self.request = 'UBCT1 stats\n'
        self._parse_config()
        self._auto_config()
        self.debug('Extended stats: {0}'.format(self.ext))
        if self.ext:
            self.order = self.order + EXTENDED_ORDER
            self.definitions.update(EXTENDED_CHARTS)
        if self.unix_socket:
            self.debug('Using unix socket: {0}'.format(self.unix_socket))
            for key in self.definitions:
                self.definitions[key]['options'][4] = 'Local'
        else:
            self.debug('Connecting to: {0}:{1}'.format(self.host, self.port))
            self.debug('Using key: {0}'.format(self.key))
            self.debug('Using certificate: {0}'.format(self.cert))
            for key in self.definitions:
                self.definitions[key]['options'][4] = self.host

    def _auto_config(self):
        if self.ubconf and os.access(self.ubconf, os.R_OK):
            self.debug('Unbound config: {0}'.format(self.ubconf))
            conf = YamlOrderedLoader.load_config_from_file(self.ubconf)[0]
            if self.ext is None:
                if 'extended-statistics' in conf['server']:
                    self.ext = conf['server']['extended-statistics']
            if 'remote-control' in conf:
                if conf['remote-control'].get('control-use-cert', False):
                    if not self.key:
                        self.key = conf['remote-control'].get('control-key-file')
                    if not self.cert:
                        self.cert = conf['remote-control'].get('control-cert-file')
                    if not self.port:
                        self.port = conf['remote-control'].get('control-port')
                else:
                    if not self.unix_socket:
                        self.unix_socket = conf['remote-control'].get('control-interface')
        else:
            self.debug('Unbound configuration not found.')
        if not self.key:
            self.key = '/etc/unbound/unbound_control.key'
        if not self.cert:
            self.cert = '/etc/unbound/unbound_control.pem'
        if not self.port:
            self.port = 8953

    def check(self):
        # We need to check that auth works, otherwise there's no point.
        self._connect()
        result = bool(self._sock)
        self._disconnect()
        return result

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
        for item in STAT_MAP:
            if item in tmp:
                data[STAT_MAP[item][0]] = float(tmp[item]) * STAT_MAP[item][1]
        return data
