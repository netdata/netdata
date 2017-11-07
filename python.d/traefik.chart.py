# -*- coding: utf-8 -*-
# Description: traefik netdata python.d module
# Author: Alexandre Menezes (@ale_menezes)

from collections import namedtuple
from json import loads
from socket import gethostbyname, gaierror
from threading import Thread
try:
        from queue import Queue
except ImportError:
        from Queue import Queue

#from bases.FrameworkServices.UrlService import UrlService
from base import UrlService

# default module values (can be overridden per job in `config`)
update_every = 5
priority = 60000
retries = 60

METHODS = namedtuple('METHODS', ['get_data', 'url', 'run'])

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
    'avg_response_time_sec', 'total_response_time_sec', 'status_code', 'uptime_sec'
    ]

CHARTS = {
    'avg_response_time_sec': {
        'options': [None, 'avg response time', 'seconds', 'avg response time', 'traefik.avg_response_time_sec', 'line'],
        'lines': [
            ['average_response_time_sec', 'time', 'divisor', 1000]
        ]},
    'total_response_time_sec': {
        'options': [None, 'total response time', 'seconds', 'total response time', 'traefik.total_response_time_sec', 'line'],
        'lines': [
            ['total_response_time_sec', 'total', 'absolute']
        ]},
    'status_code': {
        'options': [None, 'status code', 'total', 'http status code', 'traefik.status_code_total', 'area'],
        'lines': [
            ['total_count', 'total', 'absolute'],
            ['total_status_code_count_200', '200', 'absolute'],
            ['total_status_code_count_201', '201', 'absolute'],
            ['total_status_code_count_202', '202', 'absolute'],
            ['total_status_code_count_203', '203', 'absolute'],
            ['total_status_code_count_204', '204', 'absolute'],
            ['total_status_code_count_205', '205', 'absolute'],
            ['total_status_code_count_206', '206', 'absolute'],
            ['total_status_code_count_300', '300', 'absolute'],
            ['total_status_code_count_301', '301', 'absolute'],
            ['total_status_code_count_302', '302', 'absolute'],
            ['total_status_code_count_303', '303', 'absolute'],
            ['total_status_code_count_304', '304', 'absolute'],
            ['total_status_code_count_305', '305', 'absolute'],
            ['total_status_code_count_306', '306', 'absolute'],
            ['total_status_code_count_307', '307', 'absolute'],
            ['total_status_code_count_400', '400', 'absolute'],
            ['total_status_code_count_401', '401', 'absolute'],
            ['total_status_code_count_404', '404', 'absolute'],
            ['total_status_code_count_405', '405', 'absolute'],
            ['total_status_code_count_406', '406', 'absolute'],
            ['total_status_code_count_407', '407', 'absolute'],
            ['total_status_code_count_408', '408', 'absolute'],
            ['total_status_code_count_409', '409', 'absolute'],
            ['total_status_code_count_410', '410', 'absolute'],
            ['total_status_code_count_411', '411', 'absolute'],
            ['total_status_code_count_412', '412', 'absolute'],
            ['total_status_code_count_413', '413', 'absolute'],
            ['total_status_code_count_414', '414', 'absolute'],
            ['total_status_code_count_415', '415', 'absolute'],
            ['total_status_code_count_416', '416', 'absolute'],
            ['total_status_code_count_417', '417', 'absolute'],
            ['total_status_code_count_500', '500', 'absolute'],
            ['total_status_code_count_501', '501', 'absolute'],
            ['total_status_code_count_502', '502', 'absolute'],
            ['total_status_code_count_504', '504', 'absolute'],
            ['total_status_code_count_505', '505', 'absolute']
        ]},
    'uptime_sec': {
        'options': [None, 'uptime', 'seconds', 'uptime', 'traefik.uptime_sec', 'line'],
        'lines': [
            ['uptime_sec', 'uptime', 'absolute']
        ]}
    }


class Service(UrlService):
    def __init__(self, configuration=None, name=None):
        UrlService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.host = self.configuration.get('host')
        self.port = self.configuration.get('port', 8080)
        self.url = '{scheme}://{host}:{port}'.format(scheme=self.configuration.get('scheme', 'http'),
                                                     host=self.host,
                                                     port=self.port)
        self.methods = list()

    def check(self):
        if not all([self.host,
                    self.port,
                    isinstance(self.host, str),
                    isinstance(self.port, (str, int))]):
            self.error('Host is not defined in the module configuration file')
            return False

        try:
            self.host = gethostbyname(self.host)
        except gaierror as error:
            self.error(str(error))
            return False

        self.methods = [METHODS(get_data=self._get_health_stats,
                                url=self.url + '/health',
                                run=self.configuration.get('health_status', True))
                        ]
        return UrlService.check(self)

    def _get_data(self):
        threads = list()
        queue = Queue()
        result = dict()

        for method in self.methods:
            if not method.run:
                continue
            th = Thread(target=method.get_data,
                        args=(queue, method.url))
            th.start()
            threads.append(th)

        for thread in threads:
            thread.join()
            result.update(queue.get())

        return result or None

    def _get_health_stats(self, queue, url):
        """
        Format data received from http request
        :return: dict
        """
        raw_data = self._get_raw_data(url)

        if not raw_data:
            return queue.put(dict())

        data = loads(raw_data)
        to_netdata = fetch_data_(raw_data=data,
                                 metrics=HEALTH_STATS)

        return queue.put(to_netdata)

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
