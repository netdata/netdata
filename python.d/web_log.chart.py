# -*- coding: utf-8 -*-
# Description: web log netdata python.d module
# Author: l2isbad

from base import LogService
import re
import bisect
from os import access, R_OK
from os.path import getsize
from collections import defaultdict, namedtuple
from copy import deepcopy
try:
    from itertools import zip_longest
except ImportError:
    from itertools import izip_longest as zip_longest

priority = 60000
retries = 60

ORDER = ['response_codes', 'bandwidth', 'response_time', 'requests_per_url', 'http_method', 'requests_per_ipproto', 'clients', 'clients_all']
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
            ['resp_time_min', 'min', 'absolute', 1, 1000],
            ['resp_time_max', 'max', 'absolute', 1, 1000],
            ['resp_time_avg', 'avg', 'absolute', 1, 1000]
        ]},
    'clients': {
        'options': [None, 'Current Poll Unique Client IPs', 'unique ips', 'clients', 'web_log.clients', 'stacked'],
        'lines': [
            ['unique_cur_ipv4', 'ipv4', 'absolute', 1, 1],
            ['unique_cur_ipv6', 'ipv6', 'absolute', 1, 1]
        ]},
    'clients_all': {
        'options': [None, 'All Time Unique Client IPs', 'unique ips', 'clients', 'web_log.clients_all', 'stacked'],
        'lines': [
            ['unique_tot_ipv4', 'ipv4', 'absolute', 1, 1],
            ['unique_tot_ipv6', 'ipv6', 'absolute', 1, 1]
        ]},
    'http_method': {
        'options': [None, 'Requests Per HTTP Method', 'requests/s', 'http methods', 'web_log.http_method', 'stacked'],
        'lines': [
        ]},
    'requests_per_ipproto': {
        'options': [None, 'Requests Per IP Protocol', 'requests/s', 'ip protocols', 'web_log.requests_per_ipproto', 'stacked'],
        'lines': [
            ['req_ipv4', 'ipv4', 'absolute', 1, 1],
            ['req_ipv6', 'ipv6', 'absolute', 1, 1]
        ]}
}

NAMED_URL_PATTERN = namedtuple('URL_PATTERN', ['description', 'pattern'])


class Service(LogService):
    def __init__(self, configuration=None, name=None):
        LogService.__init__(self, configuration=configuration, name=name)
        # Variables from module configuration file
        self.log_path = self.configuration.get('path')
        self.detailed_response_codes = self.configuration.get('detailed_response_codes', True)
        self.all_time = self.configuration.get('all_time', True)
        self.url_pattern = self.configuration.get('categories')  # dict
        self.regex = None  # will be assigned in 'find_regex' method
        self.resp_time_func = None  # will be assigned in 'find_regex' method
        self._get_data = None  # will be assigned in 'check' method.
        self.order = None  # will be assigned in 'create_*_method' method.
        self.definitions = None  # will be assigned in 'create_*_method' method.
        self.detailed_chart = None  # will be assigned in 'create_*_method' method.
        self.http_method_chart = None  # will be assigned in 'create_*_method' method.
        # sorted list of unique IPs
        self.unique_all_time = list()
        # dict for values that should not be zeroed every poll
        self.storage = {'unique_tot_ipv4': 0, 'unique_tot_ipv6': 0}
        # if there is no new logs this dict + self.storage returned to netdata
        self.data = {'bytes_sent': 0, 'resp_length': 0, 'resp_time_min': 0,
                     'resp_time_max': 0, 'resp_time_avg': 0, 'unique_cur_ipv4': 0,
                     'unique_cur_ipv6': 0, '2xx': 0, '5xx': 0, '3xx': 0, '4xx': 0,
                     '1xx': 0, '0xx': 0, 'unmatched': 0, 'req_ipv4': 0, 'req_ipv6': 0}

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
        regex_name = self.find_regex(last_line)
        if not regex_name:
            self.error('Can\'t parse %s' % self.log_path)
            return False

        if regex_name.startswith('access_'):
            self.create_access_charts(regex_name)
            if regex_name == 'access_default':
                self.info('Not all data collected. You need to modify LogFormat.')
            self._get_data = self._get_access_data
            self.info('Used regex: %s' % regex_name)
            return True
        else:
            # If it's not access_logs.. Not used at the moment
            return False

    def find_regex(self, last_line):
        """
        :param last_line: str: literally last line from log file
        :return: regex_name
        It's sad but different web servers has different logs formats
        We need to find appropriate regex for current log file
        All logic is do a regex search through the string for all patterns
        until we find something or fail.
        """
        # REGEX: 1.IPv4 address 2.HTTP method 3. URL 4. Response code
        # 5. Bytes sent 6. Response length 7. Response process time
        access_default = re.compile(r'([\da-f.:]+)'
                                    r' -.*?"([A-Z]+)'
                                    r' (.*?)"'
                                    r' ([1-9]\d{2})'
                                    r' (\d+)')

        access_apache_ext = re.compile(r'([\da-f.:]+)'
                                       r' -.*?"([A-Z]+)'
                                       r' (.*?)"'
                                       r' ([1-9]\d{2})'
                                       r' (\d+)'
                                       r' (\d+)'
                                       r' (\d+) ')

        access_nginx_ext = re.compile(r'([\da-f.:]+)'
                                      r' -.*?"([A-Z]+)'
                                      r' (.*?)"'
                                      r' ([1-9]\d{2})'
                                      r' (\d+)'
                                      r' (\d+)'
                                      r' ([\d.]+) ')

        regex_function = zip([access_apache_ext, access_nginx_ext, access_default],
                             [lambda x: x, lambda x: x * 1000000, lambda x: x],
                             ['access_apache_ext', 'access_nginx_ext', 'access_default'])
        regex_name = None
        for regex, function, name in regex_function:
            if regex.search(last_line):
                self.regex = regex
                self.resp_time_func = function
                regex_name = name
                break
        return regex_name

    def create_access_charts(self, regex_name):
        """
        :param regex_name: str: regex name from 'find_regex' method. Ex.: 'apache_extended', 'nginx_extended'
        :return:
        Create additional charts depending on the 'find_regex' result (parsed_line) and configuration file
        1. 'time_response' chart is removed if there is no 'time_response' in logs.
        2. We need to change divisor for 'response_time' chart for apache (time in microseconds in logs)
        3. Other stuff is just remove/add chart depending on yes/no in conf
        """
        def find_job_name(override_name, name):
            """
            :param override_name: str: 'name' var from configuration file
            :param name: str: 'job_name' from configuration file
            :return: str: new job name
            We need this for dynamic charts. Actually same logic as in python.d.plugin.
            """
            add_to_name = override_name or name
            if add_to_name:
                return '_'.join(['web_log', re.sub('\s+', '_', add_to_name)])
            else:
                return 'web_log'

        self.order = ORDER[:]
        self.definitions = deepcopy(CHARTS)

        job_name = find_job_name(self.override_name, self.name)
        self.detailed_chart = 'CHART %s.detailed_response_codes ""' \
                              ' "Detailed Response Codes" requests/s responses' \
                              ' web_log.detailed_response_codes stacked 1 %s\n' % (job_name, self.update_every)
        self.http_method_chart = 'CHART %s.http_method' \
                                 ' "" "Requests Per HTTP Method" requests/s "http methods"' \
                                 ' web_log.http_method stacked 2 %s\n' % (job_name, self.update_every)

        # Remove 'request_time' chart from ORDER if request_time not in logs
        if regex_name == 'access_default':
            self.order.remove('response_time')
        # Remove 'clients_all' chart from ORDER if specified in the configuration
        if not self.all_time:
            self.order.remove('clients_all')
        # Add 'detailed_response_codes' chart if specified in the configuration
        if self.detailed_response_codes:
            self.order.append('detailed_response_codes')
            self.definitions['detailed_response_codes'] = {'options': [None, 'Detailed Response Codes', 'requests/s',
                                                                       'responses', 'web_log.detailed_response_codes', 'stacked'],
                                                           'lines': []}

        # Add 'requests_per_url' chart if specified in the configuration
        if self.url_pattern:
            self.url_pattern = [NAMED_URL_PATTERN(description=k, pattern=re.compile(v)) for k, v in self.url_pattern.items()]
            self.definitions['requests_per_url'] = {'options': [None, 'Requests Per Url', 'requests/s',
                                                                'urls', 'web_log.requests_per_url', 'stacked'],
                                                    'lines': [['other_url', 'other', 'absolute']]}
            for elem in self.url_pattern:
                self.definitions['requests_per_url']['lines'].append([elem.description, elem.description, 'absolute'])
                self.data.update({elem.description: 0})
            self.data.update({'other_url': 0})
        else:
            self.order.remove('requests_per_url')

    def add_new_dimension(self, dimension, line_list, chart_string, key):
        """
        :param dimension: str: response status code. Ex.: '202', '499'
        :param line_list: list: Ex.: ['202', '202', 'Absolute']
        :param chart_string: Current string we need to pass to netdata to rebuild the chart
        :param key: str: CHARTS dict key (chart name). Ex.: 'response_time'
        :return: str: new chart string = previous + new dimensions
        """
        self.data.update({dimension: 0})
        # SET method check if dim in _dimensions
        self._dimensions.append(dimension)
        # UPDATE method do SET only if dim in definitions
        self.definitions[key]['lines'].append(line_list)
        chart = chart_string
        chart += "%s %s\n" % ('DIMENSION', ' '.join(line_list))
        print(chart)
        return chart

    def _get_access_data(self):
        """
        Parse new log lines
        :return: dict OR None
        None if _get_raw_data method fails.
        In all other cases - dict.
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
            match = self.regex.search(line)
            if match:
                match_dict = dict(zip_longest('address method url code sent resp_length resp_time'.split(),
                                              match.groups()))
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
                # bandwidth sent
                to_netdata['bytes_sent'] += int(match_dict['sent'])
                # request processing time and bandwidth received
                if match_dict['resp_length'] and match_dict['resp_time']:
                    to_netdata['resp_length'] += int(match_dict['resp_length'])
                    resp_time = self.resp_time_func(float(match_dict['resp_time']))
                    bisect.insort_left(request_time, resp_time)
                    request_counter['count'] += 1
                    request_counter['sum'] += resp_time
                # requests per ip proto
                proto = 'ipv4' if '.' in match_dict['address'] else 'ipv6'
                to_netdata['req_' + proto] += 1
                # unique clients ips
                if address_not_in_pool(self.unique_all_time, match_dict['address'],
                                       self.storage['unique_tot_ipv4'] + self.storage['unique_tot_ipv6']):
                        self.storage['unique_tot_' + proto] += 1
                if address_not_in_pool(unique_current, match_dict['address'],
                                       to_netdata['unique_cur_ipv4'] + to_netdata['unique_cur_ipv6']):
                        to_netdata['unique_cur_' + proto] += 1
            else:
                to_netdata['unmatched'] += 1
        # timings
        if request_time:
            to_netdata['resp_time_min'] = request_time[0]
            to_netdata['resp_time_avg'] = round(float(request_counter['sum']) / request_counter['count'])
            to_netdata['resp_time_max'] = request_time[-1]

        to_netdata.update(self.storage)
        to_netdata.update(default_dict)
        return to_netdata

    def _get_data_detailed_response_codes(self, code, default_dict):
        """
        :param code: str: CODE from parsed line. Ex.: '202, '499'
        :param default_dict: defaultdict
        :return:
        Calls add_new_dimension method If the value is found for the first time
        """
        if code not in self.data:
            chart_string_copy = self.detailed_chart
            self.detailed_chart = self.add_new_dimension(code, [code, code, 'absolute'],
                                                         chart_string_copy, 'detailed_response_codes')
        default_dict[code] += 1

    def _get_data_http_method(self, method, default_dict):
        """
        :param method: str: METHOD from parsed line. Ex.: 'GET', 'POST'
        :param default_dict: defaultdict
        :return:
        Calls add_new_dimension method If the value is found for the first time
        """
        if method not in self.data:
            chart_string_copy = self.http_method_chart
            self.http_method_chart = self.add_new_dimension(method, [method, method, 'absolute'],
                                                            chart_string_copy, 'http_method')
        default_dict[method] += 1

    def _get_data_per_url(self, url, default_dict):
        """
        :param url: str: URL from parsed line
        :param default_dict: defaultdict
        :return:
        Scan through string looking for the first location where patterns produce a match for all user
        defined patterns
        """
        match = None
        for elem in self.url_pattern:
            if elem.pattern.search(url):
                default_dict[elem.description] += 1
                match = True
                break
        if not match:
            default_dict['other_url'] += 1


def address_not_in_pool(pool, address, pool_size):
    """
    :param pool: list of ip addresses
    :param address: ip address
    :param pool_size: current size of pool
    :return: True if address not pool and False address in pool
    If address not in pool function add address to pool.
    """
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
