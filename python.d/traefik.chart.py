# -*- coding: utf-8 -*-
# Description: traefik netdata python.d module
# Author: Alexandre Menezes (@ale_menezes)

from json import loads
from base import UrlService

# default module values (can be overridden per job in `config`)
update_every = 5
priority = 60000
retries = 60

HEALTH_STATS = [
    'average_response_time_sec',
    'total_response_time_sec',
    'total_count',
    'total_status_code_count.200',
    'total_status_code_count.201',
    'total_status_code_count.202',
    'total_status_code_count.203',
    'total_status_code_count.204',
    'total_status_code_count.205',
    'total_status_code_count.206',
    'total_status_code_count.300',
    'total_status_code_count.301',
    'total_status_code_count.302',
    'total_status_code_count.303',
    'total_status_code_count.304',
    'total_status_code_count.305',
    'total_status_code_count.306',
    'total_status_code_count.307',
    'total_status_code_count.400',
    'total_status_code_count.401',
    'total_status_code_count.404',
    'total_status_code_count.405',
    'total_status_code_count.406',
    'total_status_code_count.407',
    'total_status_code_count.408',
    'total_status_code_count.409',
    'total_status_code_count.410',
    'total_status_code_count.411',
    'total_status_code_count.412',
    'total_status_code_count.413',
    'total_status_code_count.414',
    'total_status_code_count.415',
    'total_status_code_count.416',
    'total_status_code_count.417',
    'total_status_code_count.500',
    'total_status_code_count.501',
    'total_status_code_count.502',
    'total_status_code_count.504',
    'total_status_code_count.505',
    'uptime_sec'
]

# charts order (can be overridden if you want less charts, or different order)
ORDER = [
    'avg_response_time', 'total_response_time', 'requests', 'status_code', 'uptime'
    ]

CHARTS = {
    'avg_response_time': {
        'options': [None, 'AVG Response Time', 'milliseconds', 'avg response time', 'traefik.avg_response_time', 'line'],
        'lines': [
            ['average_response_time_sec', None, 'absolute', 1000000, 1000]
        ]},
    'total_response_time': {
        'options': [None, 'Total Response Time', 'seconds', 'total response time', 'traefik.total_response_time', 'line'],
        'lines': [
            ['total_response_time_sec', None, 'absolute', 1, 1]
        ]},
    'requests': {
        'options': [None, 'traefik Requests', 'requests/s', 'requests', 'traefik.requests', 'line'],
        'lines': [
            ['total_count', None, 'incremental'],
        ]},
    'status_code': {
        'options': [None, 'status code', None, 'http status code', 'traefik.status_code_total', 'stacked'],
        'lines': [
            ['total_status_code_count_200', '200', 'incremental'],
            ['total_status_code_count_201', '201', 'incremental'],
            ['total_status_code_count_202', '202', 'incremental'],
            ['total_status_code_count_203', '203', 'incremental'],
            ['total_status_code_count_204', '204', 'incremental'],
            ['total_status_code_count_205', '205', 'incremental'],
            ['total_status_code_count_206', '206', 'incremental'],
            ['total_status_code_count_300', '300', 'incremental'],
            ['total_status_code_count_301', '301', 'incremental'],
            ['total_status_code_count_302', '302', 'incremental'],
            ['total_status_code_count_303', '303', 'incremental'],
            ['total_status_code_count_304', '304', 'incremental'],
            ['total_status_code_count_305', '305', 'incremental'],
            ['total_status_code_count_306', '306', 'incremental'],
            ['total_status_code_count_307', '307', 'incremental'],
            ['total_status_code_count_400', '400', 'incremental'],
            ['total_status_code_count_401', '401', 'incremental'],
            ['total_status_code_count_404', '404', 'incremental'],
            ['total_status_code_count_405', '405', 'incremental'],
            ['total_status_code_count_406', '406', 'incremental'],
            ['total_status_code_count_407', '407', 'incremental'],
            ['total_status_code_count_408', '408', 'incremental'],
            ['total_status_code_count_409', '409', 'incremental'],
            ['total_status_code_count_410', '410', 'incremental'],
            ['total_status_code_count_411', '411', 'incremental'],
            ['total_status_code_count_412', '412', 'incremental'],
            ['total_status_code_count_413', '413', 'incremental'],
            ['total_status_code_count_414', '414', 'incremental'],
            ['total_status_code_count_415', '415', 'incremental'],
            ['total_status_code_count_416', '416', 'incremental'],
            ['total_status_code_count_417', '417', 'incremental'],
            ['total_status_code_count_500', '500', 'incremental'],
            ['total_status_code_count_501', '501', 'incremental'],
            ['total_status_code_count_502', '502', 'incremental'],
            ['total_status_code_count_504', '504', 'incremental'],
            ['total_status_code_count_505', '505', 'incremental']
        ]},
    'uptime': {
        'options': [None, 'Uptime', 'seconds', 'uptime', 'traefik.uptime', 'line'],
        'lines': [
            ['uptime_sec', None, 'absolute']
        ]}
    }


class Service(UrlService):
    def __init__(self, configuration=None, name=None):
        UrlService.__init__(self, configuration=configuration, name=name)
        self.url = self.configuration.get('url', 'http://localhost:8080/health')
        self.order = ORDER
        self.definitions = CHARTS

    def _get_data(self):
        data = self._get_raw_data()

        if not data:
            return None

        data = loads(data)
        to_netdata = fetch_data_(raw_data=data, metrics=HEALTH_STATS)
        return to_netdata or None

def fetch_data_(raw_data, metrics):
    data = dict()
    for metric in metrics:
        value = raw_data
        metrics_list = metric.split('.')
        try:
            for m in metrics_list:
                value = value[m]
        except KeyError:
            continue
        data['_'.join(metrics_list)] = value
    return data
