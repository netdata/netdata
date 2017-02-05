# -*- coding: utf-8 -*-
# Description: nginx log netdata python.d module
# Author: Pawel Krupa (paulfantom), l2isbad

from base import LogService
import re
import bisect
from os import access, R_OK
from os.path import getsize

priority = 60000
retries = 60

ORDER = ['response_codes', 'request_time', 'bandwidth', 'clients']
CHARTS = {
    'response_codes': {
        'options': [None, 'Response Codes', 'requests/s', 'Responses', 'nginx_log.response', 'stacked'],
        'lines': [
            ['2', '2xx', 'incremental'],
            ['5', '5xx', 'incremental'],
            ['3', '3xx', 'incremental'],
            ['4', '4xx', 'incremental'],
            ['1', '1xx', 'incremental'],
            ['0', 'other', 'incremental']
        ]},
    'bandwidth': {
        'options': [None, 'Bandwidth', 'KB/s', 'Bandwidth', 'nginx_log.bw', 'stacked'],
        'lines': [
            ['bytes_sent', 'sent', 'incremental', 1, 1024],
            ['resp_length', 'received', 'incremental', 1, 1024]
        ]},
    'request_time': {
        'options': [None, 'Processing Time', 'seconds', 'Requests', 'nginx_log.request', 'stacked'],
        'lines': [
            ['resp_time_min', 'min', 'absolute', 1, 1000],
            ['resp_time_avg', 'avg', 'absolute', 1, 1000],
            ['resp_time_max', 'max', 'absolute', 1, 1000]
        ]},
    'clients': {
        'options': [None, 'Unique Client IPs', 'number', 'Client IPs', 'nginx_log.clients', 'stacked'],
        'lines': [
            ['unique_cur', 'current_poll', 'absolute', 1, 1],
            ['foo', 'foo', 'absolute', 1, 1],
            ['unique_tot', 'all_time', 'absolute', 1, 1]
        ]}
}


class Service(LogService):
    def __init__(self, configuration=None, name=None):
        LogService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.log_path = self.configuration.get('path', '/var/log/nginx/access.log')
        self.regex = re.compile(r'(\d{1,3}(?:\.\d{1,3}){3}).*?((?<= )[1-9])\d\d (\d+) (\d+)? ?([\d.]+)?')
        self.data = {str(k): 0 for k in range(6)}
        self.data.update({'bytes_sent': 0, 'unique_tot': 0,
                          'resp_length': 0, 'foo': 0})
        self.unique_alltime = list()

    def check(self):
        if not access(self.log_path, R_OK):
            self.error('%s not readable or not exist' % self.log_path)
            return False

        if not getsize(self.log_path):
            self.error('%s is empty' % self.log_path)
            return False
        
        with open(self.log_path, 'rb') as logs:
            logs.seek(-2, 2)
            while logs.read(1) != b'\n':
                logs.seek(-2, 1)
                if logs.tell() == 0:
                    break
            last_line = logs.readline().decode(encoding='utf-8')

        parsed_line = self.regex.findall(last_line)
        if not parsed_line:
            self.error('Can\'t parse output')
            return False
        else:
            if parsed_line[0][4] == '':
                self.order.remove('request_time')
        return True

    def _get_data(self):
        """
        Parse new log lines
        :return: dict
        """
        raw = self._get_raw_data()
        if raw is None:
            return None

        request_time, unique_current = list(), list()
        request_counter = {'count': 0, 'sum': 0}
        self.data.update({'resp_time_min' : 0, 'resp_time_avg': 0, 'resp_time_max': 0, 'unique_cur': 0})

        for line in raw:
            match = self.regex.findall(line)
            if match:
                match_dict = dict(zip(['address', 'code', 'sent', 'resp_length', 'resp_time'], match[0]))
                try:
                    self.data[match_dict['code']] += 1
                except KeyError:
                    self.data['0'] += 1
                self.data['bytes_sent'] += int(match_dict['sent'])
                if match_dict['resp_length'] != '' and match_dict['resp_time'] != '':
                    self.data['resp_length'] += int(match_dict['resp_length'])
                    resp_time = float(match_dict['resp_time']) * 1000
                    bisect.insort_left(request_time, resp_time)
                    request_counter['count'] += 1
                    request_counter['sum'] += resp_time
                    
                if addr_not_in_pool(self.unique_alltime, match_dict['address'], self.data['unique_tot']):
                    self.data['unique_tot'] += 1
                if addr_not_in_pool(unique_current, match_dict['address'], self.data['unique_cur']):
                    self.data['unique_cur'] += 1

        if request_time:
            self.data['resp_time_min'] = request_time[0]
            self.data['resp_time_avg'] = float(request_counter['sum']) / request_counter['count']
            self.data['resp_time_max'] = request_time[-1]

        return self.data


def addr_not_in_pool(pool, address, pool_size):
    index = bisect.bisect_left(pool, address)
    if index < pool_size:
        if pool[index] == address:
            return False
        else:
            bisect.insort_left(pool, address)
            return True
    else:
        bisect.insort_left(pool, address)
        return True
