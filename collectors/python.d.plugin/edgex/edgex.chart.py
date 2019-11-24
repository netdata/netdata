# -*- coding: utf-8 -*-
# Description: PHP-FPM netdata python.d module
# Author: Pawel Krupa (paulfantom)
# Author: Ilya Mashchenko (ilyam8)
# SPDX-License-Identifier: GPL-3.0-or-later

import json
import threading

from collections import namedtuple

try:
    from Queue import Queue
except ImportError:
    from queue import Queue

from bases.FrameworkServices.UrlService import UrlService

METHODS = namedtuple('METHODS', ['get_data', 'url', 'run'])

ORDER = [
    'events_throughput',
    'readings_throughput',
    'devices_number'
]

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
    },
    'devices_number':{
        'options': [None, 'Number of registered devices in the EdgeX platform', 'devices', 'devices number', 'edgex.devices_number', 
                    'line'],
        'lines': [
            ['number_of_devices', None, 'absolute'],
        ]
    }
 } #
# 'memstats_heap': {
#         'options': ['heap', 'memory: size of heap memory structures', 'KiB', 'memstats',
#                     'expvar.memstats.heap', 'line'],
#         'lines': [
#             ['core-data-alloc', 'alloc', 'absolute', 1, 1024],
#             ['core-metdata_alloc', 'alloc', 'absolute', 1, 1024],
#             ['core-command_alloc', 'alloc', 'absolute', 1, 1024],
#             ['core-sys_mgmt', 'alloc', 'absolute', 1, 1024],
#         ]
#     }
# }

def get_survive_any(method):
    def w(*args):
        try:
            method(*args)
        except Exception as error:
            self, queue, url = args[0], args[1], args[2]
            self.error("error during '{0}' : {1}".format(url, error))
            queue.put(dict())

    return w

class Service(UrlService):
    def __init__(self, configuration=None, name=None):
        UrlService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.url_core_data_events = self.configuration.get('url_core_data', 'http://localhost:48080') + '/api/v1/event/count'
        self.url_core_data_readings = self.configuration.get('url_core_data', 'http://localhost:48080') + '/api/v1/reading/count'
        self.url_core_metadata_devices = self.configuration.get('url_core_metadata', 'http://localhost:48081') + '/api/v1/device'
        # self.url_core_sys_mgmt = self.configuration.get('url_sys_mgmt' 'http://localhost:48090/avi/v1/metrics/edgex-support')
       
        #  - sys_mgmt is not yet certain whether it is neaded since the metrics are collected natively from netdata in the form of container metrics.
        #  - In the Fuji release, there is a distinction between the metrics returned natively from each service and from sys-mgmt
        #    The metrics returned from each service are raw (alloc, malloc, etc.) while the metrics returned from the sys-mgmt endpoint
        #    are processed (mem.average, cpu.average, etc.) and for this reason the sys-mgmt endpoint has a considerable lag (2-3s per request).
        #  - Testing is needed to ascertain which endpoints can be used for real-time monitoring while providing useful information.

    def check(self):
        self.methods = [
            METHODS(
                get_data=self._get_core_throughput_data,
                url=self.url_core_data_readings,
                run=self.configuration.get('events_per_second', True),
            ),
            METHODS(
                get_data=self._get_core_throughput_data,
                url=self.url_core_data_events,
                run=self.configuration.get('readings_per_second', True)
            ),
            METHODS(
                get_data=self._get_device_info,
                url=self.url_core_metadata_devices,
                run=self.configuration.get('number_of_devices', True)
            )
         ]
        return UrlService.check(self)

    def _get_data(self):
        threads = list()
        queue = Queue()
        result = {}
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

    @get_survive_any
    def _get_core_throughput_data(self, queue, url):
            data = {}
            raw = self._get_raw_data(url)
            if not raw:
                return queue.put({})
            raw = int(raw)
            if 'event' in url: 
                data['events/s'] = raw
            elif 'reading' in url:
                data['readings/s'] = raw
            return queue.put(data)

    @get_survive_any
    def _get_device_info(self, queue, url):
            data = {}
            raw = self._get_raw_data(url) #json string
            if not raw:
                return queue.put({})
            parsed = json.loads(raw) #python object
            data['number_of_devices'] = len(parsed) #int
            return queue.put(data)


