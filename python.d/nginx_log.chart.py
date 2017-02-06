# -*- coding: utf-8 -*-
# Description: nginx log netdata python.d module
# Author: l2isbad

from base import LogService
import re
import bisect
from os import access, R_OK
from os.path import getsize
from collections import defaultdict

priority = 60000
retries = 60

ORDER = ['response_codes', 'request_time', 'bandwidth', 'clients_cur', 'clients_all']
CHARTS = {
    'response_codes': {
        'options': [None, 'Response Codes', 'requests/s', 'responses', 'nginx_log.response', 'stacked'],
        'lines': [
            ['2xx', '2xx', 'absolute'],
            ['5xx', '5xx', 'absolute'],
            ['3xx', '3xx', 'absolute'],
            ['4xx', '4xx', 'absolute'],
            ['1xx', '1xx', 'absolute'],
            ['0xx', 'other', 'absolute']
        ]},
    'bandwidth': {
        'options': [None, 'Bandwidth', 'KB/s', 'bandwidth', 'nginx_log.bw', 'area'],
        'lines': [
            ['resp_length', 'received', 'absolute', 1, 1024],
            ['bytes_sent', 'sent', 'absolute', -1, 1024]
        ]},
    'request_time': {
        'options': [None, 'Processing Time', 'milliseconds', 'timings', 'nginx_log.request', 'area'],
        'lines': [
            ['resp_time_min', 'min', 'absolute', 1, 1],
            ['resp_time_max', 'max', 'absolute', 1, 1],
            ['resp_time_avg', 'avg', 'absolute', 1, 1]
        ]},
    'clients_cur': {
        'options': [None, 'Current Poll Unique Client IPs', 'number', 'unique clients', 'nginx_log.clients', 'line'],
        'lines': [
            ['unique_cur', 'clients', 'absolute', 1, 1]
        ]},
    'clients_all': {
        'options': [None, 'All Time Unique Client IPs', 'number', 'unique clients', 'nginx_log.clients', 'line'],
        'lines': [
            ['unique_tot', 'clients', 'absolute', 1, 1]
        ]}
}

ALL_CODES = {
 '100': 'Continue',
 '101': 'Switching Protocols',
 '102': 'Processing',
 '200': 'OK',
 '201': 'Created',
 '202': 'Accepted',
 '204': 'No Content',
 '205': 'Reset Content',
 '206': 'Partial Content',
 '208': 'Already Reported',
 '226': 'IM Used',
 '300': 'Multiple Choices',
 '301': 'Moved Permanently',
 '302': 'Found',
 '303': 'See Other',
 '304': 'Not Modified',
 '305': 'Use Proxy',
 '307': 'Temporary Redirect',
 '308': 'Permanent Redirect',
 '400': 'Bad Request',
 '401': 'Unauthorized',
 '402': 'Payment Required',
 '403': 'Forbidden',
 '404': 'Not Found',
 '405': 'Method Not Allowed',
 '406': 'Not Acceptable',
 '407': 'Proxy Authentication Required',
 '408': 'Request Timeout',
 '409': 'Conflict',
 '410': 'Gone',
 '411': 'Length Required',
 '412': 'Precondition Failed',
 '413': 'Payload Too Large',
 '415': 'Unsupported Media Type',
 '416': 'Requested Range Not Satisfiable',
 '417': 'Expectation Failed',
 '421': 'Misdirected Request',
 '422': 'Unprocessable Entity',
 '423': 'Locked',
 '424': 'Failed Dependency',
 '426': 'Upgrade Required',
 '428': 'Precondition Required',
 '429': 'Too Many Requests',
 '431': 'Request Header Fields Too Large',
 '444': 'Connection Closed Without Response',
 '451': 'Unavailable For Legal Reasons',
 '499': 'Client Closed Request',
 '500': 'Internal Server Error',
 '501': 'Not Implemented',
 '502': 'Bad Gateway',
 '503': 'Service Unavailable',
 '504': 'Gateway Timeout',
 '505': 'HTTP Version Not Supported',
 '506': 'Variant Also Negotiates',
 '507': 'Insufficient Storage',
 '508': 'Loop Detected',
 '510': 'Not Extended',
 '511': 'Network Authentication Required',
 '599': 'Network Connect Timeout Error'}


class Service(LogService):
    def __init__(self, configuration=None, name=None):
        LogService.__init__(self, configuration=configuration, name=name)
        self.log_path = self.configuration.get('path', '/var/log/nginx/access.log')
        self.detailed_response_codes = self.configuration.get('detailed_response_codes', False)
        self.regex = re.compile(r'(\d{1,3}(?:\.\d{1,3}){3}).*?((?<= )[1-9]\d{2}) (\d+) (\d+)? ?([\d.]+)?')
        # sorted list of unique IPs
        self.unique_alltime = list()
        # all values that should not be zeroed every poll
        self.storage = {'unique_tot': 0}
        self.data = {'bytes_sent': 0, 'resp_length': 0, 'resp_time_min': 0,
                     'resp_time_max': 0, 'resp_time_avg': 0, 'unique_cur': 0,
                     'unique_tot': 0, '2xx': 0, '5xx': 0, '3xx': 0, '4xx': 0,
                     '1xx': 0, '0xx': 0}

    def check(self):
        # Can't start if log path is not readable by netdata
        if not access(self.log_path, R_OK):
            self.error('%s not readable or not exist' % self.log_path)
            return False

        # Can't start if file is empty
        if not getsize(self.log_path):
            self.error('%s is empty' % self.log_path)
            return False

        # Read the last line from file
        with open(self.log_path, 'rb') as logs:
            logs.seek(-2, 2)
            while logs.read(1) != b'\n':
                logs.seek(-2, 1)
                if logs.tell() == 0:
                    break
            last_line = logs.readline().decode(encoding='utf-8')

        # We need to make sure we can to parse the line
        parsed_line = self.regex.findall(last_line)
        if not parsed_line:
            self.error('Can\'t parse output')
            return False

        # Pass 5th element from parsed line result (response time)
        self.create_charts(parsed_line[0][4])
        return True

    def create_charts(self, parsed_line):
        # This thing needed for adding dynamic dimensions
        self.detailed_chart = 'CHART nginx_log_local.detailed_response_codes ""' \
                              ' "Response Codes" requests responses nginx_log.detailed stacked 14 1\n'
        self.order = ORDER
        self.definitions = CHARTS

        # Remove 'request_time' chart if there is not 'reponse_time' in logs
        if parsed_line == '':
            self.order.remove('request_time')
 
        # Add detailed_response_codes chart if specified in the configuration
        if self.detailed_response_codes:
            self.order.append('detailed_response_codes')
            self.definitions['detailed_response_codes'] = {'options': [None, 'Detailed Response Codes', 'requests/s',
                                                                       'responses', 'nginx_log.detailed_resp', 'stacked'],
                                                           'lines': []}

    def add_new_dimension(self, code, line):
        # add new dimension to STORAGE dict. If code appear once we it will always be as current value or zero
        self.storage.update({code: 0})
        # Pass new dimension to NETDATA
        self._dimensions.append(code)
        self.definitions['detailed_response_codes']['lines'].append(line)
        self.detailed_chart += "%s %s\n" % ('DIMENSION', ' '.join(line))
        print(self.detailed_chart)

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
        to_netdata = dict()
        to_netdata.update(self.data)
        detailed_dict = defaultdict(lambda: 0)

        for line in raw:
            match = self.regex.findall(line)
            if match:
                match_dict = dict(zip(['address', 'code', 'sent', 'resp_length', 'resp_time'], match[0]))
                try:
                    code = ''.join([match_dict['code'][0], 'xx'])
                    to_netdata[code] += 1
                except KeyError:
                    to_netdata['0xx'] += 1

                if self.detailed_response_codes: self._get_detailed_data(match_dict['code'], detailed_dict)
                
                to_netdata['bytes_sent'] += int(match_dict['sent'])
                
                if match_dict['resp_length'] != '' and match_dict['resp_time'] != '':
                    to_netdata['resp_length'] += int(match_dict['resp_length'])
                    resp_time = float(match_dict['resp_time']) * 1000
                    bisect.insort_left(request_time, resp_time)
                    request_counter['count'] += 1
                    request_counter['sum'] += resp_time
                    
                if address_not_in_pool(self.unique_alltime, match_dict['address'], self.storage['unique_tot']):
                    self.storage['unique_tot'] += 1
                if address_not_in_pool(unique_current, match_dict['address'], to_netdata['unique_cur']):
                    to_netdata['unique_cur'] += 1

        if request_time:
            to_netdata['resp_time_min'] = request_time[0]
            to_netdata['resp_time_avg'] = float(request_counter['sum']) / request_counter['count']
            to_netdata['resp_time_max'] = request_time[-1]

        to_netdata.update(self.storage)
        to_netdata.update(detailed_dict)

        return to_netdata

    def _get_detailed_data(self, code, detailed_dict):
        if code not in self.storage:
            self.add_new_dimension(code, [code, code, 'absolute'])
        detailed_dict[code] += 1
        

def address_not_in_pool(pool, address, pool_size):
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
