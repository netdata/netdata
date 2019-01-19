# -*- coding: utf-8 -*-
# Description: haproxy netdata python.d module
# Author: l2isbad, ktarasz
# SPDX-License-Identifier: GPL-3.0-or-later

from collections import defaultdict
from re import compile as re_compile

try:
    from urlparse import urlparse
except ImportError:
    from urllib.parse import urlparse

from bases.FrameworkServices.SocketService import SocketService
from bases.FrameworkServices.UrlService import UrlService

# charts order (can be overridden if you want less charts, or different order)
ORDER = [
    'fbin',
    'fbout',
    'fscur',
    'fqcur',
    'fhrsp_1xx',
    'fhrsp_2xx',
    'fhrsp_3xx',
    'fhrsp_4xx',
    'fhrsp_5xx',
    'fhrsp_other',
    'fhrsp_total',
    'bbin',
    'bbout',
    'bscur',
    'bqcur',
    'bhrsp_1xx',
    'bhrsp_2xx',
    'bhrsp_3xx',
    'bhrsp_4xx',
    'bhrsp_5xx',
    'bhrsp_other',
    'bhrsp_total',
    'bqtime',
    'bttime',
    'brtime',
    'bctime',
    'health_sup',
    'health_sdown',
    'health_bdown',
    'health_idle'
]

CHARTS = {
    'fbin': {
        'options': [None, 'Kilobytes In', 'KiB/s', 'frontend', 'haproxy_f.bin', 'line'],
        'lines': []
    },
    'fbout': {
        'options': [None, 'Kilobytes Out', 'KiB/s', 'frontend', 'haproxy_f.bout', 'line'],
        'lines': []
    },
    'fscur': {
        'options': [None, 'Sessions Active', 'sessions', 'frontend', 'haproxy_f.scur', 'line'],
        'lines': []
    },
    'fqcur': {
        'options': [None, 'Session In Queue', 'sessions', 'frontend', 'haproxy_f.qcur', 'line'],
        'lines': []
    },
    'fhrsp_1xx': {
        'options': [None, 'HTTP responses with 1xx code', 'responses/s', 'frontend', 'haproxy_f.hrsp_1xx', 'line'],
        'lines': []
    },
    'fhrsp_2xx': {
        'options': [None, 'HTTP responses with 2xx code', 'responses/s', 'frontend', 'haproxy_f.hrsp_2xx', 'line'],
        'lines': []
    },
    'fhrsp_3xx': {
        'options': [None, 'HTTP responses with 3xx code', 'responses/s', 'frontend', 'haproxy_f.hrsp_3xx', 'line'],
        'lines': []
    },
    'fhrsp_4xx': {
        'options': [None, 'HTTP responses with 4xx code', 'responses/s', 'frontend', 'haproxy_f.hrsp_4xx', 'line'],
        'lines': []
    },
    'fhrsp_5xx': {
        'options': [None, 'HTTP responses with 5xx code', 'responses/s', 'frontend', 'haproxy_f.hrsp_5xx', 'line'],
        'lines': []
    },
    'fhrsp_other': {
        'options': [None, 'HTTP responses with other codes (protocol error)', 'responses/s', 'frontend',
                    'haproxy_f.hrsp_other', 'line'],
        'lines': []
    },
    'fhrsp_total': {
        'options': [None, 'HTTP responses', 'responses', 'frontend', 'haproxy_f.hrsp_total', 'line'],
        'lines': []
    },
    'bbin': {
        'options': [None, 'Kilobytes In', 'KiB/s', 'backend', 'haproxy_b.bin', 'line'],
        'lines': []
    },
    'bbout': {
        'options': [None, 'Kilobytes Out', 'KiB/s', 'backend', 'haproxy_b.bout', 'line'],
        'lines': []
    },
    'bscur': {
        'options': [None, 'Sessions Active', 'sessions', 'backend', 'haproxy_b.scur', 'line'],
        'lines': []
    },
    'bqcur': {
        'options': [None, 'Sessions In Queue', 'sessions', 'backend', 'haproxy_b.qcur', 'line'],
        'lines': []
    },
    'bhrsp_1xx': {
        'options': [None, 'HTTP responses with 1xx code', 'responses/s', 'backend', 'haproxy_b.hrsp_1xx', 'line'],
        'lines': []
    },
    'bhrsp_2xx': {
        'options': [None, 'HTTP responses with 2xx code', 'responses/s', 'backend', 'haproxy_b.hrsp_2xx', 'line'],
        'lines': []
    },
    'bhrsp_3xx': {
        'options': [None, 'HTTP responses with 3xx code', 'responses/s', 'backend', 'haproxy_b.hrsp_3xx', 'line'],
        'lines': []
    },
    'bhrsp_4xx': {
        'options': [None, 'HTTP responses with 4xx code', 'responses/s', 'backend', 'haproxy_b.hrsp_4xx', 'line'],
        'lines': []
    },
    'bhrsp_5xx': {
        'options': [None, 'HTTP responses with 5xx code', 'responses/s', 'backend', 'haproxy_b.hrsp_5xx', 'line'],
        'lines': []
    },
    'bhrsp_other': {
        'options': [None, 'HTTP responses with other codes (protocol error)', 'responses/s', 'backend',
                    'haproxy_b.hrsp_other', 'line'],
        'lines': []
    },
    'bhrsp_total': {
        'options': [None, 'HTTP responses (total)', 'responses/s', 'backend', 'haproxy_b.hrsp_total', 'line'],
        'lines': []
    },
    'bqtime': {
        'options': [None, 'The average queue time over the 1024 last requests', 'milliseconds', 'backend',
                    'haproxy_b.qtime', 'line'],
        'lines': []
    },
    'bctime': {
        'options': [None, 'The average connect time over the 1024 last requests', 'milliseconds', 'backend',
                    'haproxy_b.ctime', 'line'],
        'lines': []
    },
    'brtime': {
        'options': [None, 'The average response time over the 1024 last requests', 'milliseconds', 'backend',
                    'haproxy_b.rtime', 'line'],
        'lines': []
    },
    'bttime': {
        'options': [None, 'The average total session time over the 1024 last requests', 'milliseconds', 'backend',
                    'haproxy_b.ttime', 'line'],
        'lines': []
    },
    'health_sdown': {
        'options': [None, 'Backend Servers In DOWN State', 'failed servers', 'health', 'haproxy_hs.down', 'line'],
        'lines': []
    },
    'health_sup': {
        'options': [None, 'Backend Servers In UP State', 'health servers', 'health', 'haproxy_hs.up', 'line'],
        'lines': []
    },
    'health_bdown': {
        'options': [None, 'Is Backend Failed?', 'boolean', 'health', 'haproxy_hb.down', 'line'],
        'lines': []
    },
    'health_idle': {
        'options': [None, 'The Ratio Of Polling Time Vs Total Time', 'percentage', 'health', 'haproxy.idle', 'line'],
        'lines': [
            ['idle', None, 'absolute']
        ]
    }
}


METRICS = {
    'bin': {'algorithm': 'incremental', 'divisor': 1024},
    'bout': {'algorithm': 'incremental', 'divisor': 1024},
    'scur': {'algorithm': 'absolute', 'divisor': 1},
    'qcur': {'algorithm': 'absolute', 'divisor': 1},
    'hrsp_1xx': {'algorithm': 'incremental', 'divisor': 1},
    'hrsp_2xx': {'algorithm': 'incremental', 'divisor': 1},
    'hrsp_3xx': {'algorithm': 'incremental', 'divisor': 1},
    'hrsp_4xx': {'algorithm': 'incremental', 'divisor': 1},
    'hrsp_5xx': {'algorithm': 'incremental', 'divisor': 1},
    'hrsp_other': {'algorithm': 'incremental', 'divisor': 1}
}


BACKEND_METRICS = {
    'qtime': {'algorithm': 'absolute', 'divisor': 1},
    'ctime': {'algorithm': 'absolute', 'divisor': 1},
    'rtime': {'algorithm': 'absolute', 'divisor': 1},
    'ttime': {'algorithm': 'absolute', 'divisor': 1}
}


REGEX = dict(url=re_compile(r'idle = (?P<idle>[0-9]+)'),
             socket=re_compile(r'Idle_pct: (?P<idle>[0-9]+)'))


# TODO: the code is unreadable
class Service(UrlService, SocketService):
    def __init__(self, configuration=None, name=None):
        if 'socket' in configuration:
            SocketService.__init__(self, configuration=configuration, name=name)
            self.poll = SocketService
            self.options_ = dict(regex=REGEX['socket'],
                                 stat='show stat\n'.encode(),
                                 info='show info\n'.encode())
        else:
            UrlService.__init__(self, configuration=configuration, name=name)
            self.poll = UrlService
            self.options_ = dict(regex=REGEX['url'],
                                 stat=self.url,
                                 info=url_remove_params(self.url))
        self.order = ORDER
        self.definitions = CHARTS

    def check(self):
        if self.poll.check(self):
            self.create_charts()
            self.info('We are using %s.' % self.poll.__name__)
            return True
        return False

    def _get_data(self):
        to_netdata = dict()
        self.request, self.url = self.options_['stat'], self.options_['stat']
        stat_data = self._get_stat_data()
        self.request, self.url = self.options_['info'], self.options_['info']
        info_data = self._get_info_data(regex=self.options_['regex'])

        to_netdata.update(stat_data)
        to_netdata.update(info_data)
        return to_netdata or None

    def _get_stat_data(self):
        """
        :return: dict
        """
        raw_data = self.poll._get_raw_data(self)

        if not raw_data:
            return dict()

        raw_data = raw_data.splitlines()
        self.data = parse_data_([dict(zip(raw_data[0].split(','), raw_data[_].split(',')))
                                 for _ in range(1, len(raw_data))])
        if not self.data:
            return dict()

        stat_data = dict()

        for frontend in self.data['frontend']:
            for metric in METRICS:
                idx = frontend['# pxname'].replace('.', '_')
                stat_data['_'.join(['frontend', metric, idx])] = frontend.get(metric) or 0

        for backend in self.data['backend']:
            name, idx = backend['# pxname'], backend['# pxname'].replace('.', '_')
            stat_data['hsup_' + idx] = len([server for server in self.data['servers']
                                            if server_status(server, name, 'UP')])
            stat_data['hsdown_' + idx] = len([server for server in self.data['servers']
                                              if server_status(server, name, 'DOWN')])
            stat_data['hbdown_' + idx] = 1 if backend.get('status') == 'DOWN' else 0
            for metric in BACKEND_METRICS:
                stat_data['_'.join(['backend', metric, idx])] = backend.get(metric) or 0
            hrsp_total = 0
            for metric in METRICS:
                stat_data['_'.join(['backend', metric, idx])] = backend.get(metric) or 0
                if metric.startswith('hrsp_'):
                    hrsp_total += int(backend.get(metric) or 0)
            stat_data['_'.join(['backend', 'hrsp_total', idx])] = hrsp_total
        return stat_data

    def _get_info_data(self, regex):
        """
        :return: dict
        """
        raw_data = self.poll._get_raw_data(self)
        if not raw_data:
            return dict()

        match = regex.search(raw_data)
        return match.groupdict() if match else dict()

    @staticmethod
    def _check_raw_data(data):
        """
        Check if all data has been gathered from socket
        :param data: str
        :return: boolean
        """
        return not bool(data)

    def create_charts(self):
        for front in self.data['frontend']:
            name, idx = front['# pxname'], front['# pxname'].replace('.', '_')
            for metric in METRICS:
                self.definitions['f' + metric]['lines'].append(['_'.join(['frontend', metric, idx]),
                                                                name, METRICS[metric]['algorithm'], 1,
                                                                METRICS[metric]['divisor']])
            self.definitions['fhrsp_total']['lines'].append(['_'.join(['frontend', 'hrsp_total', idx]),
                                                            name, 'incremental', 1, 1])
        for back in self.data['backend']:
            name, idx = back['# pxname'], back['# pxname'].replace('.', '_')
            for metric in METRICS:
                self.definitions['b' + metric]['lines'].append(['_'.join(['backend', metric, idx]),
                                                                name, METRICS[metric]['algorithm'], 1,
                                                                METRICS[metric]['divisor']])
            self.definitions['bhrsp_total']['lines'].append(['_'.join(['backend', 'hrsp_total', idx]),
                                                            name, 'incremental', 1, 1])
            for metric in BACKEND_METRICS:
                self.definitions['b' + metric]['lines'].append(['_'.join(['backend', metric, idx]),
                                                                name, BACKEND_METRICS[metric]['algorithm'], 1,
                                                                BACKEND_METRICS[metric]['divisor']])
            self.definitions['health_sup']['lines'].append(['hsup_' + idx, name, 'absolute'])
            self.definitions['health_sdown']['lines'].append(['hsdown_' + idx, name, 'absolute'])
            self.definitions['health_bdown']['lines'].append(['hbdown_' + idx, name, 'absolute'])


def parse_data_(data):
    def is_backend(backend):
        return backend.get('svname') == 'BACKEND' and backend.get('# pxname') != 'stats'

    def is_frontend(frontend):
        return frontend.get('svname') == 'FRONTEND' and frontend.get('# pxname') != 'stats'

    def is_server(server):
        return not server.get('svname', '').startswith(('FRONTEND', 'BACKEND'))

    if not data:
        return None

    result = defaultdict(list)
    for elem in data:
        if is_backend(elem):
            result['backend'].append(elem)
            continue
        elif is_frontend(elem):
            result['frontend'].append(elem)
            continue
        elif is_server(elem):
            result['servers'].append(elem)

    return result or None


def server_status(server, backend_name, status='DOWN'):
    return server.get('# pxname') == backend_name and server.get('status') == status


def url_remove_params(url):
    parsed = urlparse(url or str())
    return '{scheme}://{netloc}{path}'.format(scheme=parsed.scheme, netloc=parsed.netloc, path=parsed.path)
