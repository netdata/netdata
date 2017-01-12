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
        'options': [None, "Kilobytes in", "kilobytes in/s", 'Frontend', 'haproxy_f.bin', 'line'],
        'lines': [
        ]},
    'fbout': {
        'options': [None, "Kilobytes out", "kilobytes out/s", 'Frontend', 'haproxy_f.bout', 'line'],
        'lines': [
        ]},
    'fscur': {
        'options': [None, "Sessions active", "sessions", 'Frontend', 'haproxy_f.scur', 'line'],
        'lines': [
        ]},
    'fqcur': {
        'options': [None, "Session in queue", "sessions", 'Frontend', 'haproxy_f.qcur', 'line'],
        'lines': [
        ]},
    'bbin': {
        'options': [None, "Kilobytes in", "kilobytes in/s", 'Backend', 'haproxy_b.bin', 'line'],
        'lines': [
        ]},
    'bbout': {
        'options': [None, "Kilobytes out", "kilobytes out/s", 'Backend', 'haproxy_b.bout', 'line'],
        'lines': [
        ]},
    'bscur': {
        'options': [None, "Sessions active", "sessions", 'Backend', 'haproxy_b.scur', 'line'],
        'lines': [
        ]},
    'bqcur': {
        'options': [None, "Sessions in queue", "sessions", 'Backend', 'haproxy_b.qcur', 'line'],
        'lines': [
        ]},
    'health_sdown': {
        'options': [None, "Number of servers in backend in DOWN state", "failed servers", 'Health', 'haproxy_hs.down', 'line'],
        'lines': [
        ]},
    'health_bdown': {
        'options': [None, "Is backend alive? 1 = DOWN", "failed backend", 'Health', 'haproxy_hb.down', 'line'],
        'lines': [
        ]}
}


class Service(UrlService, SocketService):
    def __init__(self, configuration=None, name=None):
        SocketService.__init__(self, configuration=configuration, name=name)
        self.user = self.configuration.get('user')
        self.password = self.configuration.get('pass')
        self.request = 'show stat\n'
        self.poll_method = (UrlService, SocketService)
        self.order = ORDER
        self.order_front = [_ for _ in ORDER if _.startswith('f')]
        self.order_back = [_ for _ in ORDER if _.startswith('b')]
        self.definitions = CHARTS
        self.charts = True

    def check(self):
        if self.configuration.get('url'):
            self.poll_method = self.poll_method[0]
            url = self.configuration.get('url')
            if not url.endswith(';csv;norefresh'):
                self.error('Bad url(%s). Must be http://<ip.address>:<port>/<url>;csv;norefresh' % url)
                return False
        elif self.configuration.get('socket'):
            self.poll_method = self.poll_method[1]
        else:
            self.error('No configuration is specified')
            return False

        if self.poll_method.check(self):
            self.info('Plugin was started succesfully. We are using %s.' % self.poll_method.__name__)
            return True

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
            self.definitions['health_sdown']['lines'].append(['_'.join(['hsdown', back_ends[_]['# pxname']]), back_ends[_]['# pxname'], 'absolute'])
            self.definitions['health_bdown']['lines'].append(['_'.join(['hbdown', back_ends[_]['# pxname']]), back_ends[_]['# pxname'], 'absolute'])
                
    def _get_data(self):
        """
        Format data received from http request
        :return: dict
        """
        try:
            raw_data = self.poll_method._get_raw_data(self).splitlines()
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
            to_netdata.update({'_'.join(['hsdown', back_ends[_]['# pxname']]):
                           len([server for server in servers if is_server_down(server, back_ends, _)])})
            to_netdata.update({'_'.join(['hbdown', back_ends[_]['# pxname']]): 1 if is_backend_down(back_ends, _) else 0})

        return to_netdata

    def _check_raw_data(self, data):
        """
        Check if all data has been gathered from socket
        :param data: str
        :return: boolean
        """
        return not bool(data)

def is_backend(backend):
    try:
        return backend['svname'] == 'BACKEND' and backend['# pxname'] != 'stats'
    except Exception:
        return False

def is_frontend(frontend):
    try:
        return frontend['svname'] == 'FRONTEND' and frontend['# pxname'] != 'stats'
    except Exception:
        return False

def is_server(server):
    try:
        return not server['svname'].startswith(('FRONTEND', 'BACKEND'))
    except Exception:
        return False

def is_server_down(server, back_ends, _):
    try:
        return server['# pxname'] == back_ends[_]['# pxname'] and server['status'] == 'DOWN'
    except Exception:
        return False

def is_backend_down(back_ends, _):
    try:
        return back_ends[_]['status'] == 'DOWN'
    except Exception:
        return False
