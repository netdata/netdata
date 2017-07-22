# -*- coding: utf-8 -*-
# Description: haproxy netdata python.d module
# Author: l2isbad

from collections import defaultdict
from re import compile as re_compile

try:
    from urlparse import urlparse
except ImportError:
    from urllib.parse import urlparse

from base import UrlService, SocketService

# default module values (can be overridden per job in `config`)
# update_every = 2
priority = 60000
retries = 60

# charts order (can be overridden if you want less charts, or different order)
ORDER = ['fbin', 'fbout', 'fscur', 'fqcur', 'bbin', 'bbout', 'bscur', 'bqcur',
         'health_sdown', 'health_bdown', 'health_idle']
CHARTS = {
    'fbin': {
        'options': [None, "Kilobytes In", "KB/s", 'frontend', 'haproxy_f.bin', 'line'],
        'lines': [
        ]},
    'fbout': {
        'options': [None, "Kilobytes Out", "KB/s", 'frontend', 'haproxy_f.bout', 'line'],
        'lines': [
        ]},
    'fscur': {
        'options': [None, "Sessions Active", "sessions", 'frontend', 'haproxy_f.scur', 'line'],
        'lines': [
        ]},
    'fqcur': {
        'options': [None, "Session In Queue", "sessions", 'frontend', 'haproxy_f.qcur', 'line'],
        'lines': [
        ]},
    'bbin': {
        'options': [None, "Kilobytes In", "KB/s", 'backend', 'haproxy_b.bin', 'line'],
        'lines': [
        ]},
    'bbout': {
        'options': [None, "Kilobytes Out", "KB/s", 'backend', 'haproxy_b.bout', 'line'],
        'lines': [
        ]},
    'bscur': {
        'options': [None, "Sessions Active", "sessions", 'backend', 'haproxy_b.scur', 'line'],
        'lines': [
        ]},
    'bqcur': {
        'options': [None, "Sessions In Queue", "sessions", 'backend', 'haproxy_b.qcur', 'line'],
        'lines': [
        ]},
    'health_sdown': {
        'options': [None, "Backend Servers In DOWN State", "failed servers", 'health',
                    'haproxy_hs.down', 'line'],
        'lines': [
        ]},
    'health_bdown': {
        'options': [None, "Is Backend Alive? 1 = DOWN", "failed backend", 'health', 'haproxy_hb.down', 'line'],
        'lines': [
        ]},
    'health_idle': {
        'options': [None, "The Ratio Of Polling Time Vs Total Time", "percent", 'health', 'haproxy.idle', 'line'],
        'lines': [
            ['idle', None, 'absolute']
        ]}
}

METRICS = {'bin': {'algorithm': 'incremental', 'divisor': 1024},
           'bout': {'algorithm': 'incremental', 'divisor': 1024},
           'scur': {'algorithm': 'absolute', 'divisor': 1},
           'qcur': {'algorithm': 'absolute', 'divisor': 1}}

REGEX = dict(url=re_compile(r'idle = (?P<idle>[0-9]+)'),
             socket=re_compile(r'Idle_pct: (?P<idle>[0-9]+)'))


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
            stat_data['hsdown_' + idx] = len([server for server in self.data['servers']
                                              if server_down(server, name)])
            stat_data['hbdown_' + idx] = 1 if backend.get('status') == 'DOWN' else 0
            for metric in METRICS:
                stat_data['_'.join(['backend', metric, idx])] = backend.get(metric) or 0
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
        for back in self.data['backend']:
            name, idx = back['# pxname'], back['# pxname'].replace('.', '_')
            for metric in METRICS:
                self.definitions['b' + metric]['lines'].append(['_'.join(['backend', metric, idx]),
                                                                name, METRICS[metric]['algorithm'], 1,
                                                                METRICS[metric]['divisor']])
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


def server_down(server, backend_name):
    return server.get('# pxname') == backend_name and server.get('status') == 'DOWN'


def url_remove_params(url):
    parsed = urlparse(url or str())
    return '{scheme}://{netloc}{path}'.format(scheme=parsed.scheme, netloc=parsed.netloc, path=parsed.path)
