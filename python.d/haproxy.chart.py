# -*- coding: utf-8 -*-
# Description: haproxy netdata python.d module
# Author: l2isbad

from base import UrlService, SocketService

# default module values (can be overridden per job in `config`)
# update_every = 2
priority = 60000
retries = 60

# charts order (can be overridden if you want less charts, or different order)
ORDER = ['fbin', 'fbout', 'fscur', 'fqcur', 'bbin', 'bbout', 'bscur', 'bqcur', 'health_sdown', 'health_bdown']
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
        'options': [None, "Is backend alive? 1 = DOWN", "failed backend", 'health', 'haproxy_hb.down', 'line'],
        'lines': [
        ]}
}


class Service(UrlService, SocketService):
    def __init__(self, configuration=None, name=None):
        if 'socket' in configuration:
            SocketService.__init__(self, configuration=configuration, name=name)
            self.poll_method = SocketService
            self.request = 'show stat\n'
        else:
            UrlService.__init__(self, configuration=configuration, name=name)
            self.poll_method = UrlService
        self.order = ORDER
        self.definitions = CHARTS
        self.order_front = [_ for _ in ORDER if _.startswith('f')]
        self.order_back = [_ for _ in ORDER if _.startswith('b')]
        self.charts = True

    def check(self):

        if self.poll_method.check(self):
            self.info('Plugin was started successfully. We are using %s.' % self.poll_method.__name__)
            return True
        else:
            return False

    def create_charts(self, front_ends, back_ends):
        for _ in enumerate(front_ends):
            idx = _[0]
            self.definitions['fbin']['lines'].append(['_'.join(['fbin', front_ends[idx]['# pxname']]),
                                                      front_ends[idx]['# pxname'], 'incremental', 1, 1024])
            self.definitions['fbout']['lines'].append(['_'.join(['fbout', front_ends[idx]['# pxname']]),
                                                       front_ends[idx]['# pxname'], 'incremental', 1, 1024])
            self.definitions['fscur']['lines'].append(['_'.join(['fscur', front_ends[idx]['# pxname']]),
                                                       front_ends[idx]['# pxname'], 'absolute'])
            self.definitions['fqcur']['lines'].append(['_'.join(['fqcur', front_ends[idx]['# pxname']]),
                                                       front_ends[idx]['# pxname'], 'absolute'])

        for _ in enumerate(back_ends):
            idx = _[0]
            self.definitions['bbin']['lines'].append(['_'.join(['bbin', back_ends[idx]['# pxname']]),
                                                      back_ends[idx]['# pxname'], 'incremental', 1, 1024])
            self.definitions['bbout']['lines'].append(['_'.join(['bbout', back_ends[idx]['# pxname']]),
                                                       back_ends[idx]['# pxname'], 'incremental', 1, 1024])
            self.definitions['bscur']['lines'].append(['_'.join(['bscur', back_ends[idx]['# pxname']]),
                                                       back_ends[idx]['# pxname'], 'absolute'])
            self.definitions['bqcur']['lines'].append(['_'.join(['bqcur', back_ends[idx]['# pxname']]),
                                                       back_ends[idx]['# pxname'], 'absolute'])
            self.definitions['health_sdown']['lines'].append(['_'.join(['hsdown', back_ends[idx]['# pxname']]),
                                                              back_ends[idx]['# pxname'], 'absolute'])
            self.definitions['health_bdown']['lines'].append(['_'.join(['hbdown', back_ends[idx]['# pxname']]),
                                                              back_ends[idx]['# pxname'], 'absolute'])

    def _get_data(self):
        """
        Format data received from http request
        :return: dict
        """
        raw_data = self.poll_method._get_raw_data(self)

        if not raw_data:
            return None
        else:
            raw_data = raw_data.splitlines()

        all_instances = [dict(zip(raw_data[0].split(','),
                                  raw_data[_].split(','))) for _ in range(1, len(raw_data))]

        back_ends = list(filter(is_backend, all_instances))
        front_ends = list(filter(is_frontend, all_instances))
        servers = list(filter(is_server, all_instances))

        if self.charts:
            self.create_charts(front_ends, back_ends)
            self.charts = False

        to_netdata = dict()

        for frontend in front_ends:
            for idx in self.order_front:
                to_netdata.update({'_'.join([idx, frontend['# pxname']]):
                                   int(frontend[idx[1:]]) if frontend.get(idx[1:]) else 0})

        for backend in back_ends:
            for idx in self.order_back:
                to_netdata.update({'_'.join([idx, backend['# pxname']]):
                                   int(backend[idx[1:]]) if backend.get(idx[1:]) else 0})

        for _ in enumerate(back_ends):
            idx = _[0]
            to_netdata.update({'_'.join(['hsdown', back_ends[idx]['# pxname']]):
                               len([server for server in servers if is_server_down(server, back_ends, idx)])})
            to_netdata.update({'_'.join(['hbdown', back_ends[idx]['# pxname']]):
                               1 if is_backend_down(back_ends, idx) else 0})

        return to_netdata

    @staticmethod
    def _check_raw_data(data):
        """
        Check if all data has been gathered from socket
        :param data: str
        :return: boolean
        """
        return not bool(data)


def is_backend(backend):
        return backend.get('svname') == 'BACKEND' and backend.get('# pxname') != 'stats'


def is_frontend(frontend):
        return frontend.get('svname') == 'FRONTEND' and frontend.get('# pxname') != 'stats'


def is_server(server):
        return not server.get('svname', '').startswith(('FRONTEND', 'BACKEND'))


def is_server_down(server, back_ends, idx):
    return server.get('# pxname') == back_ends[idx].get('# pxname') and server.get('status') == 'DOWN'


def is_backend_down(back_ends, idx):
    return back_ends[idx].get('status') == 'DOWN'
