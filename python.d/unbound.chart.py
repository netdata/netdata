# -*- coding: utf-8 -*-
# Description: unbound netdata python.d module
# Author: Austin S. Hemmelgarn (Ferroin)

import os
import sys

from copy import deepcopy

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

# This is used as a templates for the per-thread charts.
PER_THREAD_CHARTS = {
    '_queries': {
        'options': [None, '{0} Queries Processed', 'queries', 'Queries Processed', 'unbound.threads.queries', 'line'],
        'lines': [
            ['{0}_ratelimit', 'Ratelimited', 'absolute', 1, 1],
            ['{0}_cachemiss', 'Cache Miss', 'absolute', 1, 1],
            ['{0}_cachehit', 'Cache Hit', 'absolute', 1, 1],
            ['{0}_expired', 'Expired', 'absolute', 1, 1],
            ['{0}_prefetch', 'Prefetched', 'absolute', 1, 1],
            ['{0}_recursive', 'Recursive', 'absolute', 1, 1]
        ]
    },
    '_reqlist': {
        'options': [None, '{0} Request List', 'items', 'Request List', 'unbound.threads.reqlist', 'line'],
        'lines': [
            ['{0}_reqlist_avg', 'Average Size', 'absolute', 1, 1],
            ['{0}_reqlist_max', 'Maximum Size', 'absolute', 1, 1],
            ['{0}_reqlist_overwritten', 'Overwritten Requests', 'absolute', 1, 1],
            ['{0}_reqlist_exceeded', 'Overruns', 'absolute', 1, 1],
            ['{0}_reqlist_current', 'Current Size', 'absolute', 1, 1],
            ['{0}_reqlist_user', 'User Requests', 'absolute', 1, 1]
        ]
    },
    '_recursion': {
        'options': [None, '{0} Recursion Timings', 'seconds', 'Recursive Timings', 'unbound.threads.recursion', 'line'],
        'lines': [
            ['{0}_recursive_avg', 'Average', 'absolute', 1, PRECISION],
            ['{0}_recursive_med', 'Median', 'absolute', 1, PRECISION]
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

# Same as above, but for per-thread stats.
PER_THREAD_STAT_MAP = {
    '{0}.num.queries_ip_ratelimited': ('{0}_ratelimit', 1),
    '{0}.num.cachehits': ('{0}_cachehit', 1),
    '{0}.num.cachemiss': ('{0}_cachemiss', 1),
    '{0}.num.zero_ttl': ('{0}_expired', 1),
    '{0}.num.prefetch': ('{0}_prefetch', 1),
    '{0}.num.recursivereplies': ('{0}_recursive', 1),
    '{0}.requestlist.avg': ('{0}_reqlist_avg', 1),
    '{0}.requestlist.max': ('{0}_reqlist_max', 1),
    '{0}.requestlist.overwritten': ('{0}_reqlist_overwritten', 1),
    '{0}.requestlist.exceeded': ('{0}_reqlist_exceeded', 1),
    '{0}.requestlist.current.all': ('{0}_reqlist_current', 1),
    '{0}.requestlist.current.user': ('{0}_reqlist_user', 1),
    '{0}.recursion.time.avg': ('{0}_recursive_avg', PRECISION),
    '{0}.recursion.time.median': ('{0}_recursive_med', PRECISION)
}


class Service(SocketService):
    def __init__(self, configuration=None, name=None):
        # The unbound control protocol is always TLS encapsulated
        # unless it's used over a UNIX socket, so enable TLS _before_
        # doing the normal SocketService initialization.
        configuration['tls'] = True
        self.port = 8935
        SocketService.__init__(self, configuration, name)
        self.ext = self.configuration.get('extended', None)
        self.ubconf = self.configuration.get('ubconf', None)
        self.perthread = self.configuration.get('per_thread', False)
        self.threads = None
        self.order = deepcopy(ORDER)
        self.definitions = deepcopy(CHARTS)
        self.request = 'UBCT1 stats\n'
        self._parse_config()
        self._auto_config()
        self.debug('Extended stats: {0}'.format(self.ext))
        self.debug('Per-thread stats: {0}'.format(self.perthread))
        if self.ext:
            self.order = self.order + EXTENDED_ORDER
            self.definitions.update(EXTENDED_CHARTS)
        if self.unix_socket:
            self.debug('Using unix socket: {0}'.format(self.unix_socket))
        else:
            self.debug('Connecting to: {0}:{1}'.format(self.host, self.port))
            self.debug('Using key: {0}'.format(self.key))
            self.debug('Using certificate: {0}'.format(self.cert))

    def _auto_config(self):
        if self.ubconf and os.access(self.ubconf, os.R_OK):
            self.debug('Unbound config: {0}'.format(self.ubconf))
            conf = YamlOrderedLoader.load_config_from_file(self.ubconf)[0]
            if self.ext is None:
                if 'extended-statistics' in conf['server']:
                    self.ext = conf['server']['extended-statistics']
            if 'remote-control' in conf:
                if conf['remote-control'].get('control-use-cert', False):
                    self.key = self.key or conf['remote-control'].get('control-key-file')
                    self.cert = self.cert or conf['remote-control'].get('control-cert-file')
                    self.port = self.port or conf['remote-control'].get('control-port')
                else:
                    self.unix_socket = self.unix_socket or conf['remote-control'].get('control-interface')
        else:
            self.debug('Unbound configuration not found.')
        if not self.key:
            self.key = '/etc/unbound/unbound_control.key'
        if not self.cert:
            self.cert = '/etc/unbound/unbound_control.pem'
        if not self.port:
            self.port = 8953

    def _generate_perthread_charts(self):
        # TODO: THis could probably be more efficient, but it's only
        # run once, so it probably doesn't really matter.
        for thread in range(0, self.threads):
            shortname = 'thread{0}'.format(thread)
            longname = 'Thread {0}'.format(thread)
            charts = dict()
            order = [[], [], []]
            statmap = dict()
            count = 0
            for item in PER_THREAD_CHARTS:
                chartname = '{0}{1}'.format(shortname, item)
                order[count].append(chartname)
                count += 1
                charts[chartname] = deepcopy(PER_THREAD_CHARTS[item])
                charts[chartname]['options'][1] = charts[chartname]['options'][1].format(longname)
                for line in range(0, len(charts[chartname]['lines'])):
                    charts[chartname]['lines'][line][0] = charts[chartname]['lines'][line][0].format(shortname)
            self.order = self.order + order[0] + order[1] + order[2]
            self.definitions.update(charts)
            for key, value in PER_THREAD_STAT_MAP.items():
                STAT_MAP[key.format(shortname)] = (value[0].format(shortname), value[1])

    def check(self):
        # Check if authentication is working.
        self._connect()
        result = bool(self._sock)
        self._disconnect()
        # If auth works, and we need per-thread charts, query the server
        # to see how many threads it's using.  This somewhat abuses the
        # SocketService API to get the data we need.
        if result and self.perthread:
            tmp = self.request
            if sys.version_info[0] < 3:
                self.request = 'UBCT1 status\n'
            else:
                self.request = b'UBCT1 status\n'
            raw = self._get_raw_data()
            for line in raw.splitlines():
                if line.startswith('threads'):
                    self.threads = int(line.split()[1])
                    self._generate_perthread_charts()
                    break
            if self.threads is None:
                self.info('Unable to auto-detect thread counts, disabling per-thread stats.')
                self.perthread = False
            self.request = tmp
        return result

    @staticmethod
    def _check_raw_data(data):
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
