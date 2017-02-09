# -*- coding: utf-8 -*-
# Description: web log netdata python.d module
# Author: l2isbad

from base import LogService
import re
import bisect
from os import access, R_OK
from os.path import getsize
from collections import defaultdict, namedtuple

priority = 60000
retries = 60

ORDER = ['response_codes', 'response_time', 'requests_per_url', 'http_method', 'bandwidth', 'clients', 'clients_all']
CHARTS = {
    'response_codes': {
        'options': [None, 'Response Codes', 'requests/s', 'responses', 'web_log.response_codes', 'stacked'],
        'lines': [
            ['2xx', '2xx', 'absolute'],
            ['5xx', '5xx', 'absolute'],
            ['3xx', '3xx', 'absolute'],
            ['4xx', '4xx', 'absolute'],
            ['1xx', '1xx', 'absolute'],
            ['0xx', 'other', 'absolute'],
            ['unmatched', 'unmatched', 'absolute']
        ]},
    'bandwidth': {
        'options': [None, 'Bandwidth', 'KB/s', 'bandwidth', 'web_log.bandwidth', 'area'],
        'lines': [
            ['resp_length', 'received', 'absolute', 1, 1024],
            ['bytes_sent', 'sent', 'absolute', -1, 1024]
        ]},
    'response_time': {
        'options': [None, 'Processing Time', 'milliseconds', 'timings', 'web_log.response_time', 'area'],
        'lines': [
            ['resp_time_min', 'min', 'absolute', 1, 1],
            ['resp_time_max', 'max', 'absolute', 1, 1],
            ['resp_time_avg', 'avg', 'absolute', 1, 1]
        ]},
    'clients': {
        'options': [None, 'Current Poll Unique Client IPs', 'unique ips', 'unique clients', 'web_log.clients', 'line'],
        'lines': [
            ['unique_cur_ipv4', 'ipv4', 'absolute', 1, 1],
            ['unique_cur_ipv6', 'ipv6', 'absolute', 1, 1]
        ]},
    'clients_all': {
        'options': [None, 'All Time Unique Client IPs', 'unique ips', 'unique clients', 'web_log.clients_all', 'line'],
        'lines': [
            ['unique_tot_ipv4', 'ipv4', 'absolute', 1, 1],
            ['unique_tot_ipv6', 'ipv6', 'absolute', 1, 1]
        ]},
    'http_method': {
        'options': [None, 'Requests Per HTTP Method', 'requests/s', 'requests', 'web_log.http_method', 'stacked'],
        'lines': [
        ]}
}

NAMED_URL_PATTERN = namedtuple('URL_PATTERN', ['description', 'pattern'])


class Service(LogService):
    def __init__(self, configuration=None, name=None):
        LogService.__init__(self, configuration=configuration, name=name)
        # Vars from module configuration file
        self.log_path = self.configuration.get('path')
        self.detailed_response_codes = self.configuration.get('detailed_response_codes', True)
        self.all_time = self.configuration.get('all_time', True)
        self.url_pattern = self.configuration.get('categories')  # dict
        # REGEX: 1.IPv4 address 2.HTTP method 3. URL 4. Response code
        # 5. Bytes sent 6. Response length 7. Response process time
        self.regex = re.compile(r'([\da-f.:]+)'
                                r' -.*?"([A-Z]+)'
                                r' (.*?)"'
                                r' ([1-9]\d{2})'
                                r' (\d+)'
                                r' (\d+)?'
                                r' ?([\d.]+)?')
        # sorted list of unique IPs
        self.unique_all_time = list()
        # dict for values that should not be zeroed every poll
        self.storage = {'unique_tot_ipv4': 0, 'unique_tot_ipv6': 0}
        # if there is no new logs this dict + self.storage returned to netdata
        self.data = {'bytes_sent': 0, 'resp_length': 0, 'resp_time_min': 0,
                     'resp_time_max': 0, 'resp_time_avg': 0, 'unique_cur_ipv4': 0,
                     'unique_cur_ipv6': 0, '2xx': 0, '5xx': 0, '3xx': 0, '4xx': 0,
                     '1xx': 0, '0xx': 0, 'unmatched': 0}

    def check(self):
        if not self.log_path:
            self.error('log path is not specified')
            return False

        # log_path must be readable
        if not access(self.log_path, R_OK):
            self.error('%s not readable or not exist' % self.log_path)
            return False

        # log_path file should not be empty
        if not getsize(self.log_path):
            self.error('%s is empty' % self.log_path)
            return False

        # Read last line (or first if there is only one line)
        with open(self.log_path, 'rb') as logs:
            logs.seek(-2, 2)
            while logs.read(1) != b'\n':
                logs.seek(-2, 1)
                if logs.tell() == 0:
                    break
            last_line = logs.readline().decode(encoding='utf-8')

        # Parse last line
        parsed_line = self.regex.findall(last_line)
        if not parsed_line:
            self.error('Can\'t parse output')
            return False

        # parsed_line[0][6] - response process time
        self.create_charts(parsed_line[0][6])
        return True

    def create_charts(self, parsed_line):
        def find_job_name(override_name, name):
            add_to_name = override_name or name
            if add_to_name:
                return '_'.join(['web_log', add_to_name])
            else:
                return 'web_log'

        job_name = find_job_name(self.override_name, self.name)
        self.detailed_chart = 'CHART %s.detailed_response_codes ""' \
                              ' "Response Codes" requests/s responses' \
                              ' web_log.detailed_resp stacked 1 %s\n' % (job_name, self.update_every)
        self.http_method_chart = 'CHART %s.http_method' \
                                 ' "" "HTTP Methods" requests/s requests' \
                                 ' web_log.http_method stacked 2 %s\n' % (job_name, self.update_every)
        self.order = ORDER[:]
        self.definitions = CHARTS

        # Remove 'request_time' chart from ORDER if request_time not in logs
        if parsed_line == '':
            self.order.remove('request_time')
        # Remove 'clients_all' chart from ORDER if specified in the configuration
        if not self.all_time:
            self.order.remove('clients_all')
        # Add 'detailed_response_codes' chart if specified in the configuration
        if self.detailed_response_codes:
            self.order.append('detailed_response_codes')
            self.definitions['detailed_response_codes'] = {'options': [None, 'Detailed Response Codes', 'requests/s',
                                                                       'responses', 'web_log.detailed_resp', 'stacked'],
                                                           'lines': []}

        # Add 'requests_per_url' chart if specified in the configuration
        if self.url_pattern:
            self.url_pattern = [NAMED_URL_PATTERN(description=k, pattern=re.compile(v)) for k, v in self.url_pattern.items()]
            self.definitions['requests_per_url'] = {'options': [None, 'Requests Per Url', 'requests/s',
                                                                'requests', 'web_log.url_pattern', 'stacked'],
                                                    'lines': [['other_url', 'other', 'absolute']]}
            for elem in self.url_pattern:
                self.definitions['requests_per_url']['lines'].append([elem.description, elem.description, 'absolute'])
                self.data.update({elem.description: 0})
            self.data.update({'other_url': 0})
        else:
            self.order.remove('requests_per_url')

    def add_new_dimension(self, dimension, line_list, chart_string, key):
        self.storage.update({dimension: 0})
        # SET method check if dim in _dimensions
        self._dimensions.append(dimension)
        # UPDATE method do SET only if dim in definitions
        self.definitions[key]['lines'].append(line_list)
        chart = chart_string
        chart += "%s %s\n" % ('DIMENSION', ' '.join(line_list))
        print(chart)
        return chart

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
        default_dict = defaultdict(lambda: 0)

        for line in raw:
            match = self.regex.findall(line)
            if match:
                match_dict = dict(zip('address method url code sent resp_length resp_time'.split(), match[0]))
                try:
                    code = ''.join([match_dict['code'][0], 'xx'])
                    to_netdata[code] += 1
                except KeyError:
                    to_netdata['0xx'] += 1
                # detailed response code
                if self.detailed_response_codes:
                    self._get_data_detailed_response_codes(match_dict['code'], default_dict)
                # requests per url
                if self.url_pattern:
                    self._get_data_per_url(match_dict['url'], default_dict)
                # requests per http method
                self._get_data_http_method(match_dict['method'], default_dict)

                to_netdata['bytes_sent'] += int(match_dict['sent'])

                if match_dict['resp_length'] != '' and match_dict['resp_time'] != '':
                    to_netdata['resp_length'] += int(match_dict['resp_length'])
                    resp_time = float(match_dict['resp_time']) * 1000
                    bisect.insort_left(request_time, resp_time)
                    request_counter['count'] += 1
                    request_counter['sum'] += resp_time
                # unique clients ips
                if address_not_in_pool(self.unique_all_time, match_dict['address'],
                                       self.storage['unique_tot_ipv4'] + self.storage['unique_tot_ipv6']):
                    if '.' in match_dict['address']:
                        self.storage['unique_tot_ipv4'] += 1
                    else:
                        self.storage['unique_tot_ipv6'] += 1
                if address_not_in_pool(unique_current, match_dict['address'],
                                       to_netdata['unique_cur_ipv4'] + to_netdata['unique_cur_ipv6']):
                    if '.' in match_dict['address']:
                        to_netdata['unique_cur_ipv4'] += 1
                    else:
                        to_netdata['unique_cur_ipv6'] += 1
            else:
                to_netdata['unmatched'] += 1
        # timings
        if request_time:
            to_netdata['resp_time_min'] = request_time[0]
            to_netdata['resp_time_avg'] = float(request_counter['sum']) / request_counter['count']
            to_netdata['resp_time_max'] = request_time[-1]

        to_netdata.update(self.storage)
        to_netdata.update(default_dict)
        return to_netdata

    def _get_data_detailed_response_codes(self, code, default_dict):
        if code not in self.storage:
            chart_string_copy = self.detailed_chart
            self.detailed_chart = self.add_new_dimension(code, [code, code, 'absolute'],
                                                         chart_string_copy, 'detailed_response_codes')
        default_dict[code] += 1

    def _get_data_http_method(self, method, default_dict):
        if method not in self.storage:
            chart_string_copy = self.http_method_chart
            self.http_method_chart = self.add_new_dimension(method, [method, method, 'absolute'],
                                                            chart_string_copy, 'http_method')
        default_dict[method] += 1

    def _get_data_per_url(self, url, default_dict):
        match = None
        for elem in self.url_pattern:
            if elem.pattern.search(url):
                default_dict[elem.description] += 1
                match = True
                break
        if not match:
            default_dict['other_url'] += 1


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
