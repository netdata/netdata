# -*- coding: utf-8 -*-
# Description: nginx log netdata python.d module
# Author: Pawel Krupa (paulfantom), l2isbad

from base import LogService
import re
import bisect

priority = 60000
retries = 60

ORDER = ['codes', 'bandwidth', 'clients']
CHARTS = {
    'codes': {
        'options': [None, 'Response Codes', 'requests/s', 'Responses', 'nginx_log.codes', 'stacked'],
        'lines': [
            ['2', '2xx', 'incremental'],
            ['5', '5xx', 'incremental'],
            ['3', '3xx', 'incremental'],
            ['4', '4xx', 'incremental'],
            ['1', '1xx', 'incremental'],
            ['0', 'other', 'incremental']
        ]},
    'bandwidth': {
        'options': [None, 'Bandwidth', 'KB/s', 'Bandwidth', 'nginx_log.bw', 'area'],
        'lines': [
            ['bandwidth', 'sent', 'incremental', 1, 1024]
        ]},
    'clients': {
        'options': [None, 'Unique Client IPs', 'number', 'Client IPs', 'nginx_log.clients', 'stacked'],
        'lines': [
            ['unique_cur', 'current_poll', 'incremental', 1, 1],
            ['unique_tot', 'all_time', 'absolute', 1, 1]
        ]}
}


class Service(LogService):
    def __init__(self, configuration=None, name=None):
        LogService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.regex = re.compile(r'(\d{1,3}(?:\.\d{1,3}){3}).*?((?<=\s)[1-5])\d\d (\d+)?')
        self.data = {str(k): 0 for k in range(6)}
        self.data.update({'bandwidth': 0, 'unique_cur': 0, 'unique_tot': 0})
        self.unique = list()

    def _get_data(self):
        '''
        Parse new log lines
        :return: dict
        '''
        raw = self._get_raw_data()
        if raw is None: return None

        for line in raw:
            match = self.regex.findall(line)
            if match:
                address, code, sent = match[0][0], match[0][1], match[0][2] or 0
                try:
                    self.data[code] += 1
                except KeyError:
                    self.data['0'] += 1
                self.data['bandwidth'] += int(sent)
                if self.contains(address):
                    self.data['unique_cur'] += 1
                    self.data['unique_tot'] += 1
        return self.data

    def contains(self, address):
        index = bisect.bisect_left(self.unique, address)
        if index < self.data['unique_tot']:
            if self.unique[index] == address:
                return False
            else:
                bisect.insort_left(self.unique, address)
                return True
        else:
            bisect.insort_left(self.unique, address)
            return True
