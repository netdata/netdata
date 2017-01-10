# -*- coding: utf-8 -*-
# Description: haproxy netdata python.d module
# Author: l2isbad

from base import UrlService

# default module values (can be overridden per job in `config`)
# update_every = 2
priority = 60000
retries = 60

# charts order (can be overridden if you want less charts, or different order)
ORDER = ['fbin', 'fbout', 'fscur', 'fqcur', 'bbin', 'bbout', 'bscur', 'bqcur', 'health_down']
CHARTS = {
    'fbin': {
        'options': [None, "Kilobytes in", "kilobytes in/s", 'Frontend', 'f.bin', 'line'],
        'lines': [
        ]},
    'fbout': {
        'options': [None, "Kilobytes out", "kilobytes out/s", 'Frontend', 'f.bout', 'line'],
        'lines': [
        ]},
    'fscur': {
        'options': [None, "Sessions active", "sessions", 'Frontend', 'f.scur', 'line'],
        'lines': [
        ]},
    'fqcur': {
        'options': [None, "Session in queue", "sessions", 'Frontend', 'f.qcur', 'line'],
        'lines': [
        ]},
    'bbin': {
        'options': [None, "Kilobytes in", "kilobytes in/s", 'Backend', 'b.bin', 'line'],
        'lines': [
        ]},
    'bbout': {
        'options': [None, "Kilobytes out", "kilobytes out/s", 'Backend', 'b.bout', 'line'],
        'lines': [
        ]},
    'bscur': {
        'options': [None, "Sessions active", "sessions", 'Backend', 'b.scur', 'line'],
        'lines': [
        ]},
    'bqcur': {
        'options': [None, "Sessions in queue", "sessions", 'Backend', 'b.qcur', 'line'],
        'lines': [
        ]},
    'health_down': {
        'options': [None, "Servers in DOWN state", "servers", 'Health', 'h.down', 'line'],
        'lines': [
        ]}
}


class Service(UrlService):
    def __init__(self, configuration=None, name=None):
        UrlService.__init__(self, configuration=configuration, name=name)
        self.url = "http://127.0.0.1:7000/haproxy_stats;csv"
        self.order = ORDER
        self.order_front = [_ for _ in ORDER if _.startswith('f')]
        self.order_back = [_ for _ in ORDER if _.startswith('b')]
        self.definitions = CHARTS
        self.charts = True

    def create_charts(self, front_ends, back_ends):
        for _ in range(len(front_ends)):
            self.definitions['fbin']['lines'].append(['_'.join(['fbin', front_ends[_]['# pxname']]), front_ends[_]['# pxname'], 'incremental', 1, 1024])
            self.definitions['fbout']['lines'].append(['_'.join(['fbout', front_ends[_]['# pxname']]), front_ends[_]['# pxname'], 'incremental', 1, 1024])
            self.definitions['fscur']['lines'].append(['_'.join(['fscur', front_ends[_]['# pxname']]), front_ends[_]['# pxname'], 'absolute'])
            self.definitions['fqcur']['lines'].append(['_'.join(['fqcur', front_ends[_]['# pxname']]), front_ends[_]['# pxname'], 'absolute'])
        
        for _ in range(len(back_ends)):
            self.definitions['bbin']['lines'].append(['_'.join(['bbin', back_ends[_]['# pxname']]), back_ends[_]['# pxname'], 'incremental', 1, 1024])
            self.definitions['bbout']['lines'].append(['_'.join(['bbout', back_ends[_]['# pxname']]), back_ends[_]['# pxname'], 'incremental', 1, 1024])
            self.definitions['bscur']['lines'].append(['_'.join(['bscur', back_ends[_]['# pxname']]), back_ends[_]['# pxname'], 'absolute'])
            self.definitions['bqcur']['lines'].append(['_'.join(['bqcur', back_ends[_]['# pxname']]), back_ends[_]['# pxname'], 'absolute'])
            self.definitions['health_down']['lines'].append(['_'.join(['hdown', back_ends[_]['# pxname']]), back_ends[_]['# pxname'], 'absolute'])
                
    def _get_data(self):
        """
        Format data received from http request
        :return: dict
        """
        if self.url[-4:] != ';csv':
            self.url += ';csv'
            self.info('Url rewritten to %s' % self.url)

        try:
            raw_data = self._get_raw_data().splitlines()
        except Exception as e:
            self.error(str(e))
            return None

        all_instances = [dict(zip(raw_data[0].split(','), raw_data[_].split(','))) for _ in range(1, len(raw_data))]

        back_ends = list(filter(is_backend, all_instances))
        front_ends = list(filter(is_frontend, all_instances))
        servers = list(filter(is_server, all_instances))

        if self.charts:
            self.create_charts(front_ends, back_ends)
            self.charts = False

        to_netdata = dict()

        for frontend in front_ends:
            for _ in self.order_front:
                to_netdata.update({'_'.join([_, frontend['# pxname']]): int(frontend[_[1:]]) if frontend.get(_[1:]) else 0})

        for backend in back_ends:
            for _ in self.order_back:
                to_netdata.update({'_'.join([_, backend['# pxname']]): int(backend[_[1:]]) if backend.get(_[1:]) else 0})

        for _ in range(len(back_ends)):
            to_netdata.update({'_'.join(['hdown', back_ends[_]['# pxname']]):
                           len([server for server in servers if is_server_up(server, back_ends, _)])})

        return to_netdata

def is_backend(backend):
    return backend['svname'] == 'BACKEND' and backend['# pxname'] != 'stats'

def is_frontend(frontend):
    return frontend['svname'] == 'FRONTEND' and frontend['# pxname'] != 'stats'

def is_server(server):
    return not server['svname'].startswith(('FRONTEND', 'BACKEND'))

def is_server_up(server, back_ends, _):
    return server['# pxname'] == back_ends[_]['# pxname'] and server['status'] != 'UP'
