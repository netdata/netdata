# -*- coding: utf-8 -*-
# Description: PHP-FPM netdata python.d module
# Author: Pawel Krupa (paulfantom)
# Author: Ilya Mashchenko (ilyam8)
# SPDX-License-Identifier: GPL-3.0-or-later

import json
import re

from bases.FrameworkServices.UrlService import UrlService


REGEX = re.compile(r'([a-z][a-z ]+): ([\d.]+)')

POOL_INFO = [
    ('active processes', 'active'),
    ('max active processes', 'maxActive'),
    ('idle processes', 'idle'),
    ('accepted conn', 'requests'),
    ('max children reached', 'reached'),
    ('slow requests', 'slow')
]

PER_PROCESS_INFO = [
    ('request duration', 'ReqDur'),
    ('last request cpu', 'ReqCpu'),
    ('last request memory', 'ReqMem')
]


def average(collection):
    return sum(collection, 0.0) / max(len(collection), 1)


CALC = [
    ('min', min),
    ('max', max),
    ('avg', average)
]

ORDER = [
    'events_throughput',
    'readings_throughput',
    'number_of_devices'
    'alloc',
    'malloc',
    'liveobjects'
]

# ORDER = [
    
#     'connections',
#     'requests',
#     'performance',
#     'request_duration',
#     'request_cpu',
#     'request_mem',
# ]

#TODO add more services (lines) in the memstats_heap chart
#TODO add more charts (malloc, liveobjects, frees, etc.)

CHARTS = {
    'events_throughput':{
    'options': [None, 'Events per second in the EdgeX platform', 'events/s', 'events', 'edgex.events', 
                'line'],
    'lines': [
        ['events/s', None, 'incremental'],
    ]
},
'readings_throughput':{
    'options': [None, 'Readings per second in the EdgeX platform', 'readings/s', 'readings', 'edgex.readings', 
                'line'],
    'lines': [
        ['readings/s', None, 'incremental'],
    ]
}, #
'memstats_heap': {
        'options': ['heap', 'memory: size of heap memory structures', 'KiB', 'memstats',
                    'expvar.memstats.heap', 'line'],
        'lines': [
            ['core-data-alloc', 'alloc', 'absolute', 1, 1024],
            ['core-metdata_alloc', 'alloc', 'absolute', 1, 1024],
            ['core-command_alloc', 'alloc', 'absolute', 1, 1024],
            ['core-sys_mgmt', 'alloc', 'absolute', 1, 1024],
        ]
    }
}


class Service(UrlService):
    def __init__(self, configuration=None, name=None):
        UrlService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.url_core_data = self.configuration.get('url_core_data', 'http://localhost:48080/avi/v1/events/count')
        self.url_core_data = self.configuration.get('url_core_data', 'http://localhost:48080/avi/v1/readings/count')
        self.url_core_sys_mgmt = self.configuration.get('url_sys_mgmt' 'http://localhost:48090/avi/v1/metrics/edgex-support')
        self.edgex_services = ['edgex-support-notifications', 'edgex-core-data', 'edgex-core-metdata', 'edgex-core-command']

     def check(self):

         self.methods = [
            METHODS(
                get_data=self._get_core_data,
                url=self.url_core_data
                run=self.configuration.get('node_stats', True),
            ),
            METHODS(
                get_data=self._get_sys_mgmt,
                url=self.url_sys_mgmt,
                run=self.configuration.get('cluster_health', True)
            )
         ]
         return UrlService.check(self)

    def _get_data(self):
        threads = list()
        queue = Queue()
        result = dict()

        for method in self.methods:
            if not method.run:
                continue
            th = threading.Thread(
                target=method.get_data,
                args=(queue, method.url),
            )
            th.daemon = True
            th.start()
            threads.append(th)

        for thread in threads:
            thread.join()
            result.update(queue.get())

        return result or None


def _get_core_data(self, queue, url):
        raw = self._get_raw_data(url)
        if not raw:
            return queue.put(dict())
        parsed = self.json_parse(raw)
        if not parsed:
            return queue.put(dict())

        data = fetch_data(raw_data=parsed, metrics=HEALTH_STATS)
        dummy = {
            'status_green': 0,
            'status_red': 0,
            'status_yellow': 0,
        }
        data.update(dummy)
        current_status = 'status_' + parsed['status']
        data[current_status] = 1

        return queue.put(data)



