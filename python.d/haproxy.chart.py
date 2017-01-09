# -*- coding: utf-8 -*-
# Description: haproxy netdata python.d module
# Author: l2isbad

from base import UrlService

# default module values (can be overridden per job in `config`)
# update_every = 2
priority = 60000
retries = 60

# charts order (can be overridden if you want less charts, or different order)
ORDER = ['fbin', 'fbout', 'fscur', 'fqcur', 'bbin', 'bbout', 'bscur', 'bqcur']
CHARTS = {
    'fbin': {
        'options': [None, "Bytes in", "bytes/s", 'Frontend', 'f.bin', 'line'],
        'lines': [
        ]},
    'fbout': {
        'options': [None, "Bytes out", "bytes/s", 'Frontend', 'f.bout', 'line'],
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
        'options': [None, "Bytes in", "bytes/s", 'Backend', 'b.bin', 'line'],
        'lines': [
        ]},
    'bbout': {
        'options': [None, "Bytes out", "bytes/s", 'Backend', 'b.bout', 'line'],
        'lines': [
        ]},
    'bscur': {
        'options': [None, "Sessions active", "sessions", 'Backend', 'b.scur', 'line'],
        'lines': [
        ]},
    'bqcur': {
        'options': [None, "Sessions in queue", "sessions", 'Backend', 'b.qcur', 'line'],
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
        for chart in self.order_front:
            for _ in range(len(front_ends)):
                self.definitions[chart]['lines'].append(['_'.join([chart, front_ends[_]['# pxname']]),
                                                         front_ends[_]['# pxname'],
                                                         'incremental' if chart.startswith(
                                                             ('fb', 'bb')) else 'absolute'])
        for chart in self.order_back:
            for _ in range(len(back_ends)):
                self.definitions[chart]['lines'].append(['_'.join([chart, back_ends[_]['# pxname']]),
                                                         back_ends[_]['# pxname'],
                                                         'incremental' if chart.startswith(
                                                             ('fb', 'bb')) else 'absolute'])

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

        back_ends = [backend for backend in all_instances
                     if backend['svname'] == 'BACKEND' and backend['# pxname'] != 'stats']
        front_ends = [frontend for frontend in all_instances
                      if frontend['svname'] == 'FRONTEND' and frontend['# pxname'] != 'stats']

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

        return to_netdata
