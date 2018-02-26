# -*- coding: utf-8 -*-
# Description: netstat python.d module
# Author: Wing924

from base import SimpleService

# default module values
# update_every = 4
priority = 40000
retries = 60

ORDER = ['tcp']

CHARTS = {
    'tcp': {
        'options': [None, 'TCP netstat', 'connections', 'netstat', 'netstat.tcp', 'area'],
        'lines': [
            ['established'],
            ['syn_sent'],
            ['syn_recv'],
            ['fin_wait1'],
            ['fin_wait2'],
            ['time_wait'],
            ['close'],
            ['close_wait'],
            ['last_ack'],
            ['listen'],
            ['closing'],
        	['new_syn_recv'],
        ]},
}

ST_CODES = [
    'established',
    'syn_sent',
    'syn_recv',
    'fin_wait1',
    'fin_wait2',
    'time_wait',
    'close',
    'close_wait',
    'last_ack',
    'listen',
    'closing',
	'new_syn_recv',
]

class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS

    def _get_data(self):
        with open('/proc/net/tcp') as f:
            raw_data = f.readlines()

        if not raw_data:
            return None
        data = {}
        for key in ST_CODES:
            data[key] = 0

        for line in raw_data:
            tk = line.split()
            st_name = self._st_name(tk[3])
            if st_name:
                data[st_name] += 1

        return data

    def _st_name(self, st_code):
        try:
            return ST_CODES[int(st_code, 16) - 1]
        except Exception:
            return None
