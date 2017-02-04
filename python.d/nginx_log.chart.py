# -*- coding: utf-8 -*-
# Description: nginx log netdata python.d module
# Author: Pawel Krupa (paulfantom), l2isbad

from base import LogService
import re
import bisect

priority = 60000
retries = 60

ORDER = ['codes', 'bandwidth', 'unique']
CHARTS = {
    'codes': {
        'options': [None, 'Status codes', 'requests/s', 'requests', 'nginx_log.codes', 'stacked'],
        'lines': [
            ['2', '2xx', 'incremental'],
            ['5', '5xx', 'incremental'],
            ['3', '3xx', 'incremental'],
            ['4', '4xx', 'incremental'],
            ['1', '1xx', 'incremental']
        ]},
    'bandwidth': {
        'options': [None, 'Bandwidth', 'KB/s', 'bandwidth', 'nginx_log.bw', 'area'],
        'lines': [
            ['bandwidth', 'sent', 'incremental', 1, 1024]
        ]},
    'unique': {
        'options': [None, 'Unique visitors', 'number', 'unique visitors', 'nginx_log.uv', 'line'],
        'lines': [
            ['unique', 'visitors', 'absolute', 1, 1]
        ]}
}


class Service(LogService):
    def __init__(self, configuration=None, name=None):
        LogService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.regex = re.compile(r'(\d{1,3}(?:\.\d{1,3}){3}).*?((?<=\s)[1-5])\d\d (\d+)?')
        self.data = {str(k): 0 for k in range(1, 6)}
        self.data.update({'bandwidth': 0, 'unique': 0})
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
                self.data[code] += 1
                self.data['bandwidth'] += int(sent)
                if self.contains(address):
                    self.data['unique'] += 1
        return self.data

    def contains(self, address):
        index = bisect.bisect_left(self.unique, address)
        if index < len(self.unique):
            if self.unique[index] == address:
                return False
            else:
                bisect.insort_left(self.unique, address)
                return True
        else:
            bisect.insort_left(self.unique, address)
            return True
