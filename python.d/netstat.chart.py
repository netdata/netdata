# -*- coding: utf-8 -*-
# Description: example netdata python.d module
# Author: Pawel Krupa (paulfantom)

from base import ExecutableService

# default module values
# update_every = 4
priority = 40000
retries = 60

ORDER = ['tcp']

CHARTS = {
    'tcp': {
        'options': [None, 'TCP netstat', 'connections', 'netstat', 'netstat.tcp', 'area'],
        'lines': [
            ['ESTABLISHED', 'established'],
            ['SYN_SENT',    'syn_sent'],
            ['SYN_RECV',    'syn_recv'],
            ['FIN_WAIT1',   'fin_wait1'],
            ['FIN_WAIT2',   'fin_wait2'],
            ['TIME_WAIT',   'time_wait'],
            ['CLOSE',       'close'],
            ['CLOSE_WAIT',  'close_wait'],
            ['LISTEN',      'listen'],
            ['CLOSING',     'closing'],
        ]},
}

class Service(ExecutableService):
    def __init__(self, configuration=None, name=None):
        super(self.__class__,self).__init__(configuration=configuration, name=name)
        self.command = 'netstat -tan'
        self.order = ORDER
        self.definitions = CHARTS

    def _get_data(self):
        raw_data = self._get_raw_data()
        if not raw_data:
            return None
        data = {}
        for key in ['ESTABLISHED', 'SYN_SENT', 'SYN_RECV', 'FIN_WAIT1', 'FIN_WAIT2', 'TIME_WAIT', 'CLOSE', 'CLOSE_WAIT', 'LISTEN', 'CLOSING']:
            data[key] = 0

        for line in raw_data:
            tk = line.split()
            if tk[0] != 'tcp':
                continue
            data[tk[5]] += 1

        return data
