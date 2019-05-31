# -*- coding: utf-8 -*-
# Description: web log netdata python.d module
# Author: ilyam8
# SPDX-License-Identifier: GPL-3.0-or-later

import bisect
import os
import re
from collections import namedtuple, defaultdict
from copy import deepcopy

try:
    from itertools import filterfalse
except ImportError:
    from itertools import ifilter as filter
    from itertools import ifilterfalse as filterfalse

try:
    from sys import maxint
except ImportError:
    from sys import maxsize as maxint

from bases.collection import read_last_line
from bases.FrameworkServices.LogService import LogService


ORDER_APACHE_CACHE = [
    'apache_cache',
]

ORDER_WEB = [
    'response_statuses',
    'response_codes',
    'bandwidth',
    'response_time',
    'response_time_hist',
    'response_time_upstream',
    'response_time_upstream_hist',
    'requests_per_url',
    'requests_per_user_defined',
    'http_method',
    'vhost',
    'port',
    'http_version',
    'requests_per_ipproto',
    'clients',
    'clients_all'
]

ORDER_SQUID = [
    'squid_response_statuses',
    'squid_response_codes',
    'squid_detailed_response_codes',
    'squid_method',
    'squid_mime_type',
    'squid_hier_code',
    'squid_transport_methods',
    'squid_transport_errors',
    'squid_code',
    'squid_handling_opts',
    'squid_object_types',
    'squid_cache_events',
    'squid_bytes',
    'squid_duration',
    'squid_clients',
    'squid_clients_all'
]

CHARTS_WEB = {
    'response_codes': {
        'options': [None, 'Response Codes', 'requests/s', 'responses', 'web_log.response_codes', 'stacked'],
        'lines': [
            ['2xx', None, 'incremental'],
            ['5xx', None, 'incremental'],
            ['3xx', None, 'incremental'],
            ['4xx', None, 'incremental'],
            ['1xx', None, 'incremental'],
            ['0xx', 'other', 'incremental'],
            ['unmatched', None, 'incremental']
        ]
    },
    'bandwidth': {
        'options': [None, 'Bandwidth', 'kilobits/s', 'bandwidth', 'web_log.bandwidth', 'area'],
        'lines': [
            ['resp_length', 'received', 'incremental', 8, 1000],
            ['bytes_sent', 'sent', 'incremental', -8, 1000]
        ]
    },
    'response_time': {
        'options': [None, 'Processing Time', 'milliseconds', 'timings', 'web_log.response_time', 'area'],
        'lines': [
            ['resp_time_min', 'min', 'incremental', 1, 1000],
            ['resp_time_max', 'max', 'incremental', 1, 1000],
            ['resp_time_avg', 'avg', 'incremental', 1, 1000]
        ]
    },
    'response_time_hist': {
        'options': [None, 'Processing Time Histogram', 'requests/s', 'timings', 'web_log.response_time_hist', 'line'],
        'lines': []
    },
    'response_time_upstream': {
        'options': [None, 'Processing Time Upstream', 'milliseconds', 'timings',
                    'web_log.response_time_upstream', 'area'],
        'lines': [
            ['resp_time_upstream_min', 'min', 'incremental', 1, 1000],
            ['resp_time_upstream_max', 'max', 'incremental', 1, 1000],
            ['resp_time_upstream_avg', 'avg', 'incremental', 1, 1000]
        ]
    },
    'response_time_upstream_hist': {
        'options': [None, 'Processing Time Histogram', 'requests/s', 'timings',
                    'web_log.response_time_upstream_hist', 'line'],
        'lines': []
    },
    'clients': {
        'options': [None, 'Current Poll Unique Client IPs', 'unique ips', 'clients', 'web_log.clients', 'stacked'],
        'lines': [
            ['unique_cur_ipv4', 'ipv4', 'incremental', 1, 1],
            ['unique_cur_ipv6', 'ipv6', 'incremental', 1, 1]
        ]
    },
    'clients_all': {
        'options': [None, 'All Time Unique Client IPs', 'unique ips', 'clients', 'web_log.clients_all', 'stacked'],
        'lines': [
            ['unique_tot_ipv4', 'ipv4', 'absolute', 1, 1],
            ['unique_tot_ipv6', 'ipv6', 'absolute', 1, 1]
        ]
    },
    'http_method': {
        'options': [None, 'Requests Per HTTP Method', 'requests/s', 'http methods', 'web_log.http_method', 'stacked'],
        'lines': [
            ['GET', 'GET', 'incremental', 1, 1]
        ]
    },
    'http_version': {
        'options': [None, 'Requests Per HTTP Version', 'requests/s', 'http versions',
                    'web_log.http_version', 'stacked'],
        'lines': []
    },
    'requests_per_ipproto': {
        'options': [None, 'Requests Per IP Protocol', 'requests/s', 'ip protocols', 'web_log.requests_per_ipproto',
                    'stacked'],
        'lines': [
            ['req_ipv4', 'ipv4', 'incremental', 1, 1],
            ['req_ipv6', 'ipv6', 'incremental', 1, 1]
        ]
    },
    'response_statuses': {
        'options': [None, 'Response Statuses', 'requests/s', 'responses', 'web_log.response_statuses', 'stacked'],
        'lines': [
            ['successful_requests', 'success', 'incremental', 1, 1],
            ['server_errors', 'error', 'incremental', 1, 1],
            ['redirects', 'redirect', 'incremental', 1, 1],
            ['bad_requests', 'bad', 'incremental', 1, 1],
            ['other_requests', 'other', 'incremental', 1, 1]
        ]
    },
    'requests_per_url': {
        'options': [None, 'Requests Per Url', 'requests/s', 'urls', 'web_log.requests_per_url', 'stacked'],
        'lines': [
            ['url_pattern_other', 'other', 'incremental', 1, 1]
        ]
    },
    'requests_per_user_defined': {
        'options': [None, 'Requests Per User Defined Pattern', 'requests/s', 'user defined',
                    'web_log.requests_per_user_defined', 'stacked'],
        'lines': [
            ['user_pattern_other', 'other', 'incremental', 1, 1]
        ]
    },
    'port': {
        'options': [None, 'Requests Per Port', 'requests/s', 'port', 'web_log.port', 'stacked'],
        'lines': [
            ['port_80', 'http', 'incremental', 1, 1],
            ['port_443', 'https', 'incremental', 1, 1]
        ]
    },
    'vhost': {
        'options': [None, 'Requests Per Vhost', 'requests/s', 'vhost', 'web_log.vhost', 'stacked'],
        'lines': []
    }
}

CHARTS_APACHE_CACHE = {
    'apache_cache': {
        'options': [None, 'Apache Cached Responses', 'percentage', 'cached', 'web_log.apache_cache_cache',
                    'stacked'],
        'lines': [
            ['hit', 'cache', 'percentage-of-absolute-row'],
            ['miss', None, 'percentage-of-absolute-row'],
            ['other', None, 'percentage-of-absolute-row']
        ]
    }
}

CHARTS_SQUID = {
    'squid_duration': {
        'options': [None, 'Elapsed Time The Transaction Busied The Cache',
                    'milliseconds', 'squid_timings', 'web_log.squid_duration', 'area'],
        'lines': [
            ['duration_min', 'min', 'incremental', 1, 1000],
            ['duration_max', 'max', 'incremental', 1, 1000],
            ['duration_avg', 'avg', 'incremental', 1, 1000]
        ]
    },
    'squid_bytes': {
        'options': [None, 'Amount Of Data Delivered To The Clients',
                    'kilobits/s', 'squid_bandwidth', 'web_log.squid_bytes', 'area'],
        'lines': [
            ['bytes', 'sent', 'incremental', 8, 1000]
        ]
    },
    'squid_response_statuses': {
        'options': [None, 'Response Statuses', 'responses/s', 'squid_responses', 'web_log.squid_response_statuses',
                    'stacked'],
        'lines': [
            ['successful_requests', 'success', 'incremental', 1, 1],
            ['server_errors', 'error', 'incremental', 1, 1],
            ['redirects', 'redirect', 'incremental', 1, 1],
            ['bad_requests', 'bad', 'incremental', 1, 1],
            ['other_requests', 'other', 'incremental', 1, 1]
        ]
    },
    'squid_response_codes': {
        'options': [None, 'Response Codes', 'responses/s', 'squid_responses',
                    'web_log.squid_response_codes', 'stacked'],
        'lines': [
            ['2xx', None, 'incremental'],
            ['5xx', None, 'incremental'],
            ['3xx', None, 'incremental'],
            ['4xx', None, 'incremental'],
            ['1xx', None, 'incremental'],
            ['0xx', None, 'incremental'],
            ['other', None, 'incremental'],
            ['unmatched', None, 'incremental']
        ]
    },
    'squid_code': {
        'options': [None, 'Responses Per Cache Result Of The Request',
                    'requests/s', 'squid_squid_cache', 'web_log.squid_code', 'stacked'],
        'lines': []
    },
    'squid_detailed_response_codes': {
        'options': [None, 'Detailed Response Codes',
                    'responses/s', 'squid_responses', 'web_log.squid_detailed_response_codes', 'stacked'],
        'lines': []
    },
    'squid_hier_code': {
        'options': [None, 'Responses Per Hierarchy Code',
                    'requests/s', 'squid_hierarchy', 'web_log.squid_hier_code', 'stacked'],
        'lines': []
    },
    'squid_method': {
        'options': [None, 'Requests Per Method',
                    'requests/s', 'squid_requests', 'web_log.squid_method', 'stacked'],
        'lines': []
    },
    'squid_mime_type': {
        'options': [None, 'Requests Per MIME Type',
                    'requests/s', 'squid_requests', 'web_log.squid_mime_type', 'stacked'],
        'lines': []
    },
    'squid_clients': {
        'options': [None, 'Current Poll Unique Client IPs', 'unique ips', 'squid_clients',
                    'web_log.squid_clients', 'stacked'],
        'lines': [
            ['unique_ipv4', 'ipv4', 'incremental'],
            ['unique_ipv6', 'ipv6', 'incremental']
        ]
    },
    'squid_clients_all': {
        'options': [None, 'All Time Unique Client IPs', 'unique ips', 'squid_clients',
                    'web_log.squid_clients_all', 'stacked'],
        'lines': [
            ['unique_tot_ipv4', 'ipv4', 'absolute'],
            ['unique_tot_ipv6', 'ipv6', 'absolute']
        ]
    },
    'squid_transport_methods': {
        'options': [None, 'Transport Methods', 'requests/s', 'squid_squid_transport',
                    'web_log.squid_transport_methods', 'stacked'],
        'lines': []
    },
    'squid_transport_errors': {
        'options': [None, 'Transport Errors', 'requests/s', 'squid_squid_transport',
                    'web_log.squid_transport_errors', 'stacked'],
        'lines': []
    },
    'squid_handling_opts': {
        'options': [None, 'Handling Opts', 'requests/s', 'squid_squid_cache',
                    'web_log.squid_handling_opts', 'stacked'],
        'lines': []
    },
    'squid_object_types': {
        'options': [None, 'Object Types', 'objects/s', 'squid_squid_cache',
                    'web_log.squid_object_types', 'stacked'],
        'lines': []
    },
    'squid_cache_events': {
        'options': [None, 'Cache Events', 'events/s', 'squid_squid_cache',
                    'web_log.squid_cache_events', 'stacked'],
        'lines': []
    }
}

NAMED_PATTERN = namedtuple('PATTERN', ['description', 'func'])

DET_RESP_AGGR = ['', '_1xx', '_2xx', '_3xx', '_4xx', '_5xx', '_Other']

SQUID_CODES = {
    'TCP': 'squid_transport_methods',
    'UDP': 'squid_transport_methods',
    'NONE': 'squid_transport_methods',
    'CLIENT': 'squid_handling_opts',
    'IMS': 'squid_handling_opts',
    'ASYNC': 'squid_handling_opts',
    'SWAPFAIL': 'squid_handling_opts',
    'REFRESH': 'squid_handling_opts',
    'SHARED': 'squid_handling_opts',
    'REPLY': 'squid_handling_opts',
    'NEGATIVE': 'squid_object_types',
    'STALE': 'squid_object_types',
    'OFFLINE': 'squid_object_types',
    'INVALID': 'squid_object_types',
    'FAIL': 'squid_object_types',
    'MODIFIED': 'squid_object_types',
    'UNMODIFIED': 'squid_object_types',
    'REDIRECT': 'squid_object_types',
    'HIT': 'squid_cache_events',
    'MEM': 'squid_cache_events',
    'MISS': 'squid_cache_events',
    'DENIED': 'squid_cache_events',
    'NOFETCH': 'squid_cache_events',
    'TUNNEL': 'squid_cache_events',
    'ABORTED': 'squid_transport_errors',
    'TIMEOUT': 'squid_transport_errors'
}

REQUEST_REGEX = re.compile(r'(?P<method>[A-Z]+) (?P<url>[^ ]+) [A-Z]+/(?P<http_version>\d(?:.\d)?)')

MIME_TYPES = ['application', 'audio', 'example', 'font', 'image', 'message', 'model', 'multipart', 'text', 'video']


class Service(LogService):
    def __init__(self, configuration=None, name=None):
        """
        :param configuration:
        :param name:
        """
        LogService.__init__(self, configuration=configuration, name=name)
        self.configuration = configuration
        self.log_path = self.configuration.get('path')
        self.job = None

    def check(self):
        """
        :return: bool

        1. "log_path" is specified in the module configuration file
        2. "log_path" must be readable by netdata user and must exist
        3. "log_path' must not be empty. We need at least 1 line to find appropriate pattern to parse
        4. other checks depends on log "type"
        """

        log_type = self.configuration.get('type', 'web')
        log_types = dict(web=Web, apache_cache=ApacheCache, squid=Squid)

        if log_type not in log_types:
            self.error('bad log type {log_type}. Supported types: {types}'.format(log_type=log_type,
                                                                                  types=log_types.keys()))
            return False

        if not self.log_path:
            self.error('log path is not specified')
            return False

        if not (self._find_recent_log_file() and os.access(self.log_path, os.R_OK)):
            self.error('{log_file} not readable or not exist'.format(log_file=self.log_path))
            return False

        if not os.path.getsize(self.log_path):
            self.error('{log_file} is empty'.format(log_file=self.log_path))
            return False

        self.job = log_types[log_type](self)
        if self.job.check():
            self.order = self.job.order
            self.definitions = self.job.definitions
            return True
        return False

    def _get_data(self):
        return self.job.get_data(self._get_raw_data())


class Web:
    def __init__(self, service):
        self.service = service
        self.order = ORDER_WEB[:]
        self.definitions = deepcopy(CHARTS_WEB)
        self.pre_filter = check_patterns('filter', self.configuration.get('filter'))
        self.storage = dict()
        self.data = {
            'bytes_sent': 0,
            'resp_length': 0,
            'resp_time_min': 0,
            'resp_time_max': 0,
            'resp_time_avg': 0,
            'resp_time_upstream_min': 0,
            'resp_time_upstream_max': 0,
            'resp_time_upstream_avg': 0,
            'unique_cur_ipv4': 0,
            'unique_cur_ipv6': 0,
            '2xx': 0,
            '5xx': 0,
            '3xx': 0,
            '4xx': 0,
            '1xx': 0,
            '0xx': 0,
            'unmatched': 0,
            'req_ipv4': 0,
            'req_ipv6': 0,
            'unique_tot_ipv4': 0,
            'unique_tot_ipv6': 0,
            'successful_requests': 0,
            'redirects': 0,
            'bad_requests': 0,
            'server_errors': 0,
            'other_requests': 0,
            'GET': 0
        }

    def __getattr__(self, item):
        return getattr(self.service, item)

    def check(self):
        last_line = read_last_line(self.log_path)
        if not last_line:
            return False
        # Custom_log_format or predefined log format.
        if self.configuration.get('custom_log_format'):
            match_dict, error = self.find_regex_custom(last_line)
        else:
            match_dict, error = self.find_regex(last_line)

        # "match_dict" is None if there are any problems
        if match_dict is None:
            self.error(error)
            return False

        self.storage['unique_all_time'] = list()
        self.storage['url_pattern'] = check_patterns('url_pattern', self.configuration.get('categories'))
        self.storage['user_pattern'] = check_patterns('user_pattern', self.configuration.get('user_defined'))

        self.create_web_charts(match_dict)  # Create charts
        self.info('Collected data: %s' % list(match_dict.keys()))
        return True

    def create_web_charts(self, match_dict):
        """
        :param match_dict: dict: regex.search.groupdict(). Ex. {'address': '127.0.0.1', 'code': '200', 'method': 'GET'}
        :return:
        Create/remove additional charts depending on the 'match_dict' keys and configuration file options
        """
        if 'resp_time' not in match_dict:
            self.order.remove('response_time')
            self.order.remove('response_time_hist')
        if 'resp_time_upstream' not in match_dict:
            self.order.remove('response_time_upstream')
            self.order.remove('response_time_upstream_hist')

        # Add 'response_time_hist' and 'response_time_upstream_hist' charts if is specified in the configuration
        histogram = self.configuration.get('histogram', None)
        if isinstance(histogram, list):
            self.storage['bucket_index'] = histogram[:]
            self.storage['bucket_index'].append(maxint)
            self.storage['buckets'] = [0] * (len(histogram) + 1)
            self.storage['upstream_buckets'] = [0] * (len(histogram) + 1)
            hist_lines = self.definitions['response_time_hist']['lines']
            upstream_hist_lines = self.definitions['response_time_upstream_hist']['lines']
            for i, le in enumerate(histogram):
                hist_key = 'response_time_hist_%d' % i
                upstream_hist_key = 'response_time_upstream_hist_%d' % i
                hist_lines.append([hist_key, str(le), 'incremental', 1, 1])
                upstream_hist_lines.append([upstream_hist_key, str(le), 'incremental', 1, 1])

            hist_lines.append(['response_time_hist_%d' % len(histogram), '+Inf', 'incremental', 1, 1])
            upstream_hist_lines.append(['response_time_upstream_hist_%d' % len(histogram), '+Inf', 'incremental', 1, 1])
        elif histogram is not None:
            self.error('expect histogram list, but was {0}'.format(type(histogram)))

        if not self.configuration.get('all_time', True):
            self.order.remove('clients_all')

        # Add 'detailed_response_codes' chart if specified in the configuration
        if self.configuration.get('detailed_response_codes', True):
            if self.configuration.get('detailed_response_aggregate', True):
                codes = DET_RESP_AGGR[:1]
            else:
                codes = DET_RESP_AGGR[1:]

            for code in codes:
                self.order.append('detailed_response_codes%s' % code)
                self.definitions['detailed_response_codes%s' % code] = {
                    'options': [None, 'Detailed Response Codes %s' % code[1:], 'requests/s', 'responses',
                                'web_log.detailed_response_codes%s' % code, 'stacked'],
                    'lines': []
                }

        # Add 'requests_per_url' chart if specified in the configuration
        if self.storage['url_pattern']:
            for elem in self.storage['url_pattern']:
                dim = [elem.description, elem.description[12:], 'incremental']
                self.definitions['requests_per_url']['lines'].append(dim)
                self.data[elem.description] = 0
            self.data['url_pattern_other'] = 0
        else:
            self.order.remove('requests_per_url')

        # Add 'requests_per_user_defined' chart if specified in the configuration
        if self.storage['user_pattern'] and 'user_defined' in match_dict:
            for elem in self.storage['user_pattern']:
                dim = [elem.description, elem.description[13:], 'incremental']
                self.definitions['requests_per_user_defined']['lines'].append(dim)
                self.data[elem.description] = 0
            self.data['user_pattern_other'] = 0
        else:
            self.order.remove('requests_per_user_defined')

    def get_data(self, raw_data=None):
        """
        Parses new log lines
        :return: dict OR None
        None if _get_raw_data method fails.
        In all other cases - dict.
        """
        if not raw_data:
            return None if raw_data is None else self.data

        filtered_data = filter_data(raw_data=raw_data, pre_filter=self.pre_filter)

        unique_current = set()
        timings = defaultdict(lambda: dict(minimum=None, maximum=0, summary=0, count=0))

        for line in filtered_data:
            match = self.storage['regex'].search(line)
            if match:
                match_dict = match.groupdict()
                try:
                    code = match_dict['code'][0] + 'xx'
                    self.data[code] += 1
                except KeyError:
                    self.data['0xx'] += 1
                # detailed response code
                if self.configuration.get('detailed_response_codes', True):
                    self.get_data_per_response_codes_detailed(code=match_dict['code'])
                # response statuses
                self.get_data_per_statuses(code=match_dict['code'])
                # requests per user defined pattern
                if self.storage['user_pattern'] and 'user_defined' in match_dict:
                    self.get_data_per_pattern(row=match_dict['user_defined'],
                                              other='user_pattern_other',
                                              pattern=self.storage['user_pattern'])
                # method, url, http version
                self.get_data_from_request_field(match_dict=match_dict)
                # bandwidth sent
                bytes_sent = match_dict['bytes_sent'] if '-' not in match_dict['bytes_sent'] else 0
                self.data['bytes_sent'] += int(bytes_sent)
                # request processing time and bandwidth received
                if 'resp_length' in match_dict:
                    resp_length = match_dict['resp_length'] if '-' not in match_dict['resp_length'] else 0
                    self.data['resp_length'] += int(resp_length)
                if 'resp_time' in match_dict:
                    resp_time = self.storage['func_resp_time'](float(match_dict['resp_time']))
                    get_timings(timings=timings['resp_time'], time=resp_time)
                    if 'bucket_index' in self.storage:
                        get_hist(self.storage['bucket_index'], self.storage['buckets'], resp_time / 1000)
                if 'resp_time_upstream' in match_dict and match_dict['resp_time_upstream'] != '-':
                    resp_time_upstream = self.storage['func_resp_time'](float(match_dict['resp_time_upstream']))
                    get_timings(timings=timings['resp_time_upstream'], time=resp_time_upstream)
                    if 'bucket_index' in self.storage:
                        get_hist(self.storage['bucket_index'], self.storage['upstream_buckets'], resp_time / 1000)
                # requests per ip proto
                proto = 'ipv6' if ':' in match_dict['address'] else 'ipv4'
                self.data['req_' + proto] += 1
                # unique clients ips
                if self.configuration.get('all_time', True):
                    if address_not_in_pool(pool=self.storage['unique_all_time'],
                                           address=match_dict['address'],
                                           pool_size=self.data['unique_tot_ipv4'] + self.data['unique_tot_ipv6']):
                        self.data['unique_tot_' + proto] += 1
                if match_dict['address'] not in unique_current:
                    self.data['unique_cur_' + proto] += 1
                    unique_current.add(match_dict['address'])
            else:
                self.data['unmatched'] += 1

        # timings
        for elem in timings:
            self.data[elem + '_min'] += timings[elem]['minimum']
            self.data[elem + '_avg'] += timings[elem]['summary'] / timings[elem]['count']
            self.data[elem + '_max'] += timings[elem]['maximum']

        # histogram
        if 'bucket_index' in self.storage:
            buckets = self.storage['buckets']
            upstream_buckets = self.storage['upstream_buckets']
            for i in range(0, len(self.storage['bucket_index'])):
                hist_key = 'response_time_hist_%d' % i
                upstream_hist_key = 'response_time_upstream_hist_%d' % i
                self.data[hist_key] = buckets[i]
                self.data[upstream_hist_key] = upstream_buckets[i]

        return self.data

    def find_regex(self, last_line):
        """
        :param last_line: str: literally last line from log file
        :return: tuple where:
        [0]: dict or None:  match_dict or None
        [1]: str: error description
        We need to find appropriate pattern for current log file
        All logic is do a regex search through the string for all predefined patterns
        until we find something or fail.
        """
        # REGEX: 1.IPv4 address 2.HTTP method 3. URL 4. Response code
        # 5. Bytes sent 6. Response length 7. Response process time
        default = re.compile(r'(?P<address>[\da-f.:]+|localhost)'
                             r' -.*?"(?P<request>[^"]*)"'
                             r' (?P<code>[1-9]\d{2})'
                             r' (?P<bytes_sent>\d+|-)')

        apache_ext_insert = re.compile(r'(?P<address>[\da-f.:]+|localhost)'
                                       r' -.*?"(?P<request>[^"]*)"'
                                       r' (?P<code>[1-9]\d{2})'
                                       r' (?P<bytes_sent>\d+|-)'
                                       r' (?P<resp_length>\d+|-)'
                                       r' (?P<resp_time>\d+) ')

        apache_ext_append = re.compile(r'(?P<address>[\da-f.:]+|localhost)'
                                       r' -.*?"(?P<request>[^"]*)"'
                                       r' (?P<code>[1-9]\d{2})'
                                       r' (?P<bytes_sent>\d+|-)'
                                       r' .*?'
                                       r' (?P<resp_length>\d+|-)'
                                       r' (?P<resp_time>\d+)'
                                       r'(?: |$)')

        nginx_ext_insert = re.compile(r'(?P<address>[\da-f.:]+)'
                                      r' -.*?"(?P<request>[^"]*)"'
                                      r' (?P<code>[1-9]\d{2})'
                                      r' (?P<bytes_sent>\d+)'
                                      r' (?P<resp_length>\d+)'
                                      r' (?P<resp_time>\d+\.\d+) ')

        nginx_ext2_insert = re.compile(r'(?P<address>[\da-f.:]+)'
                                       r' -.*?"(?P<request>[^"]*)"'
                                       r' (?P<code>[1-9]\d{2})'
                                       r' (?P<bytes_sent>\d+)'
                                       r' (?P<resp_length>\d+)'
                                       r' (?P<resp_time>\d+\.\d+)'
                                       r' (?P<resp_time_upstream>[\d.-]+)')

        nginx_ext_append = re.compile(r'(?P<address>[\da-f.:]+)'
                                      r' -.*?"(?P<request>[^"]*)"'
                                      r' (?P<code>[1-9]\d{2})'
                                      r' (?P<bytes_sent>\d+)'
                                      r' .*?'
                                      r' (?P<resp_length>\d+)'
                                      r' (?P<resp_time>\d+\.\d+)')

        def func_usec(time):
            return time

        def func_sec(time):
            return time * 1000000

        r_regex = [apache_ext_insert, apache_ext_append,
                   nginx_ext2_insert, nginx_ext_insert, nginx_ext_append,
                   default]
        r_function = [func_usec, func_usec, func_sec, func_sec, func_sec, func_usec]
        regex_function = zip(r_regex, r_function)

        match_dict = dict()
        for regex, func in regex_function:
            match = regex.search(last_line)
            if match:
                self.storage['regex'] = regex
                self.storage['func_resp_time'] = func
                match_dict = match.groupdict()
                break

        return find_regex_return(match_dict=match_dict or None,
                                 msg='Unknown log format. You need to use "custom_log_format" feature.')

    def find_regex_custom(self, last_line):
        """
        :param last_line: str: literally last line from log file
        :return: tuple where:
        [0]: dict or None:  match_dict or None
        [1]: str: error description

        We are here only if "custom_log_format" is in logs. We need to make sure:
        1. "custom_log_format" is a dict
        2. "pattern" in "custom_log_format" and pattern is <str> instance
        3. if "time_multiplier" is in "custom_log_format" it must be <int> or <float> instance

        If all parameters is ok we need to make sure:
        1. Pattern search is success
        2. Pattern search contains named subgroups (?P<subgroup_name>) (= "match_dict")

        If pattern search is success we need to make sure:
        1. All mandatory keys ['address', 'code', 'bytes_sent', 'method', 'url'] are in "match_dict"

        If this is True we need to make sure:
        1. All mandatory key values from "match_dict" have the correct format
         ("code" is integer, "method" is uppercase word, etc)

        If non mandatory keys in "match_dict" we need to make sure:
        1. All non mandatory key values from match_dict ['resp_length', 'resp_time'] have the correct format
         ("resp_length" is integer or "-", "resp_time" is integer or float)

        """
        if not hasattr(self.configuration.get('custom_log_format'), 'keys'):
            return find_regex_return(msg='Custom log: "custom_log_format" is not a <dict>')

        pattern = self.configuration.get('custom_log_format', dict()).get('pattern')
        if not (pattern and isinstance(pattern, str)):
            return find_regex_return(msg='Custom log: "pattern" option is not specified or type is not <str>')

        resp_time_func = self.configuration.get('custom_log_format', dict()).get('time_multiplier') or 0

        if not isinstance(resp_time_func, (int, float)):
            return find_regex_return(msg='Custom log: "time_multiplier" is not an integer or a float')

        try:
            regex = re.compile(pattern)
        except re.error as error:
            return find_regex_return(msg='Pattern compile error: %s' % str(error))

        match = regex.search(last_line)
        if not match:
            return find_regex_return(msg='Custom log: pattern search FAILED')

        match_dict = match.groupdict() or None
        if match_dict is None:
            return find_regex_return(msg='Custom log: search OK but contains no named subgroups'
                                         ' (you need to use ?P<subgroup_name>)')
        mandatory_dict = {'address': r'[\w.:-]+',
                          'code': r'[1-9]\d{2}',
                          'bytes_sent': r'\d+|-'}
        optional_dict = {'resp_length': r'\d+|-',
                         'resp_time': r'[\d.]+',
                         'resp_time_upstream': r'[\d.-]+',
                         'method': r'[A-Z]+',
                         'http_version': r'\d(?:.\d)?'}

        mandatory_values = set(mandatory_dict) - set(match_dict)
        if mandatory_values:
            return find_regex_return(msg='Custom log: search OK but some mandatory keys (%s) are missing'
                                         % list(mandatory_values))
        for key in mandatory_dict:
            if not re.search(mandatory_dict[key], match_dict[key]):
                return find_regex_return(msg='Custom log: can\'t parse "%s": %s'
                                             % (key, match_dict[key]))

        optional_values = set(optional_dict) & set(match_dict)
        for key in optional_values:
            if not re.search(optional_dict[key], match_dict[key]):
                return find_regex_return(msg='Custom log: can\'t parse "%s": %s'
                                             % (key, match_dict[key]))

        dot_in_time = '.' in match_dict.get('resp_time', '')
        if dot_in_time:
            self.storage['func_resp_time'] = lambda time: time * (resp_time_func or 1000000)
        else:
            self.storage['func_resp_time'] = lambda time: time * (resp_time_func or 1)

        self.storage['regex'] = regex
        return find_regex_return(match_dict=match_dict)

    def get_data_from_request_field(self, match_dict):
        if match_dict.get('request'):
            match_dict = REQUEST_REGEX.search(match_dict['request'])
            if match_dict:
                match_dict = match_dict.groupdict()
            else:
                return
        # requests per url
        if match_dict.get('url') and self.storage['url_pattern']:
            self.get_data_per_pattern(row=match_dict['url'],
                                      other='url_pattern_other',
                                      pattern=self.storage['url_pattern'])
        # requests per http method
        if match_dict.get('method'):
            if match_dict['method'] not in self.data:
                self.charts['http_method'].add_dimension([match_dict['method'],
                                                          match_dict['method'],
                                                          'incremental'])
                self.data[match_dict['method']] = 0
            self.data[match_dict['method']] += 1
        # requests per http version
        if match_dict.get('http_version'):
            dim_id = match_dict['http_version'].replace('.', '_')
            if dim_id not in self.data:
                self.charts['http_version'].add_dimension([dim_id,
                                                           match_dict['http_version'],
                                                           'incremental'])
                self.data[dim_id] = 0
            self.data[dim_id] += 1
        # requests per port number
        if match_dict.get('port'):
            if match_dict['port'] not in self.data:
                self.charts['port'].add_dimension([match_dict['port'],
                                                   match_dict['port'],
                                                   'incremental'])
                self.data[match_dict['port']] = 0
            self.data[match_dict['port']] += 1
        # requests per vhost
        if match_dict.get('vhost'):
            dim_id = match_dict['vhost'].replace('.', '_')
            if dim_id not in self.data:
                self.charts['vhost'].add_dimension([dim_id,
                                                   match_dict['vhost'],
                                                   'incremental'])
                self.data[dim_id] = 0
            self.data[dim_id] += 1

    def get_data_per_response_codes_detailed(self, code):
        """
        :param code: str: CODE from parsed line. Ex.: '202, '499'
        :return:
        Calls add_new_dimension method If the value is found for the first time
        """
        if code not in self.data:
            if self.configuration.get('detailed_response_aggregate', True):
                self.charts['detailed_response_codes'].add_dimension([code, code, 'incremental'])
                self.data[code] = 0
            else:
                code_index = int(code[0]) if int(code[0]) < 6 else 6
                chart_key = 'detailed_response_codes' + DET_RESP_AGGR[code_index]
                self.charts[chart_key].add_dimension([code, code, 'incremental'])
                self.data[code] = 0
        self.data[code] += 1

    def get_data_per_pattern(self, row, other, pattern):
        """
        :param row: str:
        :param other: str:
        :param pattern: named tuple: (['pattern_description', 'regular expression'])
        :return:
        Scan through string looking for the first location where patterns produce a match for all user
        defined patterns
        """
        match = None
        for elem in pattern:
            if elem.func(row):
                self.data[elem.description] += 1
                match = True
                break
        if not match:
            self.data[other] += 1

    def get_data_per_statuses(self, code):
        """
        :param code: str: response status code. Ex.: '202', '499'
        :return:
        """
        code_class = code[0]
        if code_class == '2' or code == '304' or code_class == '1':
            self.data['successful_requests'] += 1
        elif code_class == '3':
            self.data['redirects'] += 1
        elif code_class == '4':
            self.data['bad_requests'] += 1
        elif code_class == '5':
            self.data['server_errors'] += 1
        else:
            self.data['other_requests'] += 1


class ApacheCache:
    def __init__(self, service):
        self.service = service
        self.order = ORDER_APACHE_CACHE
        self.definitions = CHARTS_APACHE_CACHE

    @staticmethod
    def check():
        return True

    @staticmethod
    def get_data(raw_data=None):
        data = dict(hit=0, miss=0, other=0)
        if not raw_data:
            return None if raw_data is None else data

        for line in raw_data:
            if 'cache hit' in line:
                data['hit'] += 1
            elif 'cache miss' in line:
                data['miss'] += 1
            else:
                data['other'] += 1
        return data


class Squid:
    def __init__(self, service):
        self.service = service
        self.order = ORDER_SQUID
        self.definitions = CHARTS_SQUID
        self.pre_filter = check_patterns('filter', self.configuration.get('filter'))
        self.storage = dict()
        self.data = {
            'duration_max': 0,
            'duration_avg': 0,
            'duration_min': 0,
            'bytes': 0,
            '0xx': 0,
            '1xx': 0,
            '2xx': 0,
            '3xx': 0,
            '4xx': 0,
            '5xx': 0,
            'other': 0,
            'unmatched': 0,
            'unique_ipv4': 0,
            'unique_ipv6': 0,
            'unique_tot_ipv4': 0,
            'unique_tot_ipv6': 0,
            'successful_requests': 0,
            'redirects': 0,
            'bad_requests': 0,
            'server_errors': 0,
            'other_requests': 0
        }

    def __getattr__(self, item):
        return getattr(self.service, item)

    def check(self):
        last_line = read_last_line(self.log_path)
        if not last_line:
            return False
        self.storage['unique_all_time'] = list()
        self.storage['regex'] = re.compile(r'[0-9.]+\s+(?P<duration>[0-9]+)'
                                           r' (?P<client_address>[\da-f.:]+)'
                                           r' (?P<squid_code>[A-Z_]+)/'
                                           r'(?P<http_code>[0-9]+)'
                                           r' (?P<bytes>[0-9]+)'
                                           r' (?P<method>[A-Z_]+)'
                                           r' (?P<url>[^ ]+)'
                                           r' (?P<user>[^ ]+)'
                                           r' (?P<hier_code>[A-Z_]+)/[\da-z.:-]+'
                                           r' (?P<mime_type>[A-Za-z-]*)')

        match = self.storage['regex'].search(last_line)
        if not match:
            self.error('Regex not matches (%s)' % self.storage['regex'].pattern)
            return False
        self.storage['dynamic'] = {
            'http_code': {
                 'chart': 'squid_detailed_response_codes',
                 'func_dim_id': None,
                 'func_dim': None
            },
            'hier_code': {
                'chart': 'squid_hier_code',
                'func_dim_id': None,
                'func_dim': lambda v: v.replace('HIER_', '')
            },
            'method': {
                'chart': 'squid_method',
                'func_dim_id': None,
                'func_dim': None
            },
            'mime_type': {
                'chart': 'squid_mime_type',
                'func_dim_id': lambda v: str.lower(v) if str.lower(v) in MIME_TYPES else 'unknown',
                'func_dim': None
            }
        }
        if not self.configuration.get('all_time', True):
            self.order.remove('squid_clients_all')
        return True

    def get_data(self, raw_data=None):
        if not raw_data:
            return None if raw_data is None else self.data

        filtered_data = filter_data(raw_data=raw_data, pre_filter=self.pre_filter)

        unique_ip = set()
        timings = defaultdict(lambda: dict(minimum=None, maximum=0, summary=0, count=0))

        for row in filtered_data:
            match = self.storage['regex'].search(row)
            if match:
                match = match.groupdict()
                if match['duration'] != '0':
                    get_timings(timings=timings['duration'], time=float(match['duration']) * 1000)
                try:
                    self.data[match['http_code'][0] + 'xx'] += 1
                except KeyError:
                    self.data['other'] += 1

                self.get_data_per_statuses(match['http_code'])

                self.get_data_per_squid_code(match['squid_code'])

                self.data['bytes'] += int(match['bytes'])

                proto = 'ipv4' if '.' in match['client_address'] else 'ipv6'
                # unique clients ips
                if self.configuration.get('all_time', True):
                    if address_not_in_pool(pool=self.storage['unique_all_time'],
                                           address=match['client_address'],
                                           pool_size=self.data['unique_tot_ipv4'] + self.data['unique_tot_ipv6']):
                        self.data['unique_tot_' + proto] += 1

                if match['client_address'] not in unique_ip:
                    self.data['unique_' + proto] += 1
                    unique_ip.add(match['client_address'])

                for key, values in self.storage['dynamic'].items():
                    if match[key] == '-':
                        continue
                    dimension_id = values['func_dim_id'](match[key]) if values['func_dim_id'] else match[key]
                    if dimension_id not in self.data:
                        dimension = values['func_dim'](match[key]) if values['func_dim'] else dimension_id
                        self.charts[values['chart']].add_dimension([dimension_id,
                                                                    dimension,
                                                                    'incremental'])
                        self.data[dimension_id] = 0
                    self.data[dimension_id] += 1
            else:
                self.data['unmatched'] += 1

        for elem in timings:
            self.data[elem + '_min'] += timings[elem]['minimum']
            self.data[elem + '_avg'] += timings[elem]['summary'] / timings[elem]['count']
            self.data[elem + '_max'] += timings[elem]['maximum']
        return self.data

    def get_data_per_statuses(self, code):
        """
        :param code: str: response status code. Ex.: '202', '499'
        :return:
        """
        code_class = code[0]
        if code_class == '2' or code == '304' or code_class == '1' or code == '000':
            self.data['successful_requests'] += 1
        elif code_class == '3':
            self.data['redirects'] += 1
        elif code_class == '4':
            self.data['bad_requests'] += 1
        elif code_class == '5' or code_class == '6':
            self.data['server_errors'] += 1
        else:
            self.data['other_requests'] += 1

    def get_data_per_squid_code(self, code):
        """
        :param code: str: squid response code. Ex.: 'TCP_MISS', 'TCP_MISS_ABORTED'
        :return:
        """
        if code not in self.data:
            self.charts['squid_code'].add_dimension([code, code, 'incremental'])
            self.data[code] = 0
        self.data[code] += 1

        for tag in code.split('_'):
            try:
                chart_key = SQUID_CODES[tag]
            except KeyError:
                continue
            dimension_id = '_'.join(['code_detailed', tag])
            if dimension_id not in self.data:
                self.charts[chart_key].add_dimension([dimension_id, tag, 'incremental'])
                self.data[dimension_id] = 0
            self.data[dimension_id] += 1


def get_timings(timings, time):
    """
    :param timings:
    :param time:
    :return:
    """
    if timings['minimum'] is None:
        timings['minimum'] = time
    if time > timings['maximum']:
        timings['maximum'] = time
    elif time < timings['minimum']:
        timings['minimum'] = time
    timings['summary'] += time
    timings['count'] += 1


def get_hist(index, buckets, time):
    """
    :param index:   histogram index (Ex. [10, 50, 100, 150, ...])
    :param buckets: histogram buckets
    :param time:    time
    :return: None
    """
    for i in range(len(index)-1, -1, -1):
        if time <= index[i]:
            buckets[i] += 1
        else:
            break


def address_not_in_pool(pool, address, pool_size):
    """
    :param pool: list of ip addresses
    :param address: ip address
    :param pool_size: current pool size
    :return: True if address not in pool. False otherwise.
    """
    index = bisect.bisect_left(pool, address)
    if index < pool_size:
        if pool[index] == address:
            return False
        bisect.insort_left(pool, address)
        return True
    bisect.insort_left(pool, address)
    return True


def find_regex_return(match_dict=None, msg='Generic error message'):
    """
    :param match_dict: dict: re.search.groupdict() or None
    :param msg: str: error description
    :return: tuple:
    """
    return match_dict, msg


def check_patterns(string, dimension_regex_dict):
    """
    :param string: str:
    :param dimension_regex_dict: dict: ex. {'dim1': '<pattern1>', 'dim2': '<pattern2>'}
    :return: list of named tuples or None:
     We need to make sure all patterns are valid regular expressions
    """
    if not hasattr(dimension_regex_dict, 'keys'):
        return None

    result = list()

    def valid_pattern(pattern):
        """
        :param pattern: str
        :return: re.compile(pattern) or None
        """
        if not isinstance(pattern, str):
            return False
        try:
            return re.compile(pattern)
        except re.error:
            return False

    def func_search(pattern):
        def closure(v):
            return pattern.search(v)

        return closure

    for dimension, regex in dimension_regex_dict.items():
        valid = valid_pattern(regex)
        if isinstance(dimension, str) and valid_pattern:
            func = func_search(valid)
            result.append(NAMED_PATTERN(description='_'.join([string, dimension]),
                                        func=func))
    return result or None


def filter_data(raw_data, pre_filter):
    """
    :param raw_data:
    :param pre_filter:
    :return:
    """

    if not pre_filter:
        return raw_data
    filtered = raw_data
    for elem in pre_filter:
        if elem.description == 'filter_include':
            filtered = filter(elem.func, filtered)
        elif elem.description == 'filter_exclude':
            filtered = filterfalse(elem.func, filtered)
    return filtered
