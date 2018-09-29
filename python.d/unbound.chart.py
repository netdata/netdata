# -*- coding: utf-8 -*-
# Description: unbound netdata python.d module
# Author: Austin S. Hemmelgarn (Ferroin)
# SPDX-License-Identifier: GPL-3.0+

import os
import sys

from copy import deepcopy

from bases.FrameworkServices.SocketService import SocketService
from bases.loaders import YamlOrderedLoader

PRECISION = 1000

ORDER = ['queries', 'recursion', 'reqlist']

CHARTS = {
    'queries': {
        'options': [None, 'Queries Processed', 'queries', 'Unbound', 'unbound.queries', 'line'],
        'lines': [
            ['ratelimit', 'ratelimited', 'absolute', 1, 1],
            ['cachemiss', 'cache_miss', 'absolute', 1, 1],
            ['cachehit', 'cache_hit', 'absolute', 1, 1],
            ['expired', 'expired', 'absolute', 1, 1],
            ['prefetch', 'prefetched', 'absolute', 1, 1],
            ['recursive', 'recursive', 'absolute', 1, 1]
        ]
    },
    'recursion': {
        'options': [None, 'Recursion Timings', 'seconds', 'Unbound', 'unbound.recursion', 'line'],
        'lines': [
            ['recursive_avg', 'average', 'absolute', 1, PRECISION],
            ['recursive_med', 'median', 'absolute', 1, PRECISION]
        ]
    },
    'reqlist': {
        'options': [None, 'Request List', 'items', 'Unbound', 'unbound.reqlist', 'line'],
        'lines': [
            ['reqlist_avg', 'average_size', 'absolute', 1, 1],
            ['reqlist_max', 'maximum_size', 'absolute', 1, 1],
            ['reqlist_overwritten', 'overwritten_requests', 'absolute', 1, 1],
            ['reqlist_exceeded', 'overruns', 'absolute', 1, 1],
            ['reqlist_current', 'current_size', 'absolute', 1, 1],
            ['reqlist_user', 'user_requests', 'absolute', 1, 1]
        ]
    }
}

# These get added too if we are told to use extended stats.
EXTENDED_ORDER = ['cache']

EXTENDED_CHARTS = {
    'cache': {
        'options': [None, 'Cache Sizes', 'items', 'Unbound', 'unbound.cache', 'stacked'],
        'lines': [
            ['cache_message', 'message_cache', 'absolute', 1, 1],
            ['cache_rrset', 'rrset_cache', 'absolute', 1, 1],
            ['cache_infra', 'infra_cache', 'absolute', 1, 1],
            ['cache_key', 'dnssec_key_cache', 'absolute', 1, 1],
            ['cache_dnscss', 'dnscrypt_Shared_Secret_cache', 'absolute', 1, 1],
            ['cache_dnscn', 'dnscrypt_Nonce_cache', 'absolute', 1, 1]
        ]
    }
}

# This is used as a templates for the per-thread charts.
PER_THREAD_CHARTS = {
    '_queries': {
        'options': [None, '{longname} Queries Processed', 'queries', 'Queries Processed',
                    'unbound.threads.queries', 'line'],
        'lines': [
            ['{shortname}_ratelimit', 'ratelimited', 'absolute', 1, 1],
            ['{shortname}_cachemiss', 'cache_miss', 'absolute', 1, 1],
            ['{shortname}_cachehit', 'cache_hit', 'absolute', 1, 1],
            ['{shortname}_expired', 'expired', 'absolute', 1, 1],
            ['{shortname}_prefetch', 'prefetched', 'absolute', 1, 1],
            ['{shortname}_recursive', 'recursive', 'absolute', 1, 1]
        ]
    },
    '_recursion': {
        'options': [None, '{longname} Recursion Timings', 'seconds', 'Recursive Timings',
                    'unbound.threads.recursion', 'line'],
        'lines': [
            ['{shortname}_recursive_avg', 'average', 'absolute', 1, PRECISION],
            ['{shortname}_recursive_med', 'median', 'absolute', 1, PRECISION]
        ]
    },
    '_reqlist': {
        'options': [None, '{longname} Request List', 'items', 'Request List', 'unbound.threads.reqlist', 'line'],
        'lines': [
            ['{shortname}_reqlist_avg', 'average_size', 'absolute', 1, 1],
            ['{shortname}_reqlist_max', 'maximum_size', 'absolute', 1, 1],
            ['{shortname}_reqlist_overwritten', 'overwritten_requests', 'absolute', 1, 1],
            ['{shortname}_reqlist_exceeded', 'overruns', 'absolute', 1, 1],
            ['{shortname}_reqlist_current', 'current_size', 'absolute', 1, 1],
            ['{shortname}_reqlist_user', 'user_requests', 'absolute', 1, 1]
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
    '{shortname}.num.queries_ip_ratelimited': ('{shortname}_ratelimit', 1),
    '{shortname}.num.cachehits': ('{shortname}_cachehit', 1),
    '{shortname}.num.cachemiss': ('{shortname}_cachemiss', 1),
    '{shortname}.num.zero_ttl': ('{shortname}_expired', 1),
    '{shortname}.num.prefetch': ('{shortname}_prefetch', 1),
    '{shortname}.num.recursivereplies': ('{shortname}_recursive', 1),
    '{shortname}.requestlist.avg': ('{shortname}_reqlist_avg', 1),
    '{shortname}.requestlist.max': ('{shortname}_reqlist_max', 1),
    '{shortname}.requestlist.overwritten': ('{shortname}_reqlist_overwritten', 1),
    '{shortname}.requestlist.exceeded': ('{shortname}_reqlist_exceeded', 1),
    '{shortname}.requestlist.current.all': ('{shortname}_reqlist_current', 1),
    '{shortname}.requestlist.current.user': ('{shortname}_reqlist_user', 1),
    '{shortname}.recursion.time.avg': ('{shortname}_recursive_avg', PRECISION),
    '{shortname}.recursion.time.median': ('{shortname}_recursive_med', PRECISION)
}


# Used to actually generate per-thread charts.
def _get_perthread_info(thread):
    sname = 'thread{0}'.format(thread)
    lname = 'Thread {0}'.format(thread)
    charts = dict()
    order = []
    statmap = dict()

    for item in PER_THREAD_CHARTS:
        cname = '{0}{1}'.format(sname, item)
        chart = deepcopy(PER_THREAD_CHARTS[item])
        chart['options'][1] = chart['options'][1].format(longname=lname)

        for index, line in enumerate(chart['lines']):
            chart['lines'][index][0] = line[0].format(shortname=sname)

        order.append(cname)
        charts[cname] = chart

    for key, value in PER_THREAD_STAT_MAP.items():
        statmap[key.format(shortname=sname)] = (value[0].format(shortname=sname), value[1])

    return (charts, order, statmap)


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
        self.statmap = deepcopy(STAT_MAP)
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
        tmporder = list()
        for thread in range(0, self.threads):
            charts, order, statmap = _get_perthread_info(thread)
            tmporder.extend(order)
            self.definitions.update(charts)
            self.statmap.update(statmap)
        self.order.extend(sorted(tmporder))

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
        for item in self.statmap:
            if item in tmp:
                data[self.statmap[item][0]] = float(tmp[item]) * self.statmap[item][1]
        return data
