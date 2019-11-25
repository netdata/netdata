# -*- coding: utf-8 -*-
# Description: EdgeX Platform netdata python.d module
# Author: Odysseas Lamtzidis (OdysLam)
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
    'events_count',
    'devices_number'
]

CHARTS = {
    'events_throughput':{
        'options': [None, 'Events per second in the EdgeX platform', 'events/s', 'throughput','edgex.events',
                    'line'],
        'lines': [
            ['events', None, 'incremental']
        ]
    },
    'readings_throughput':{
        'options': [None, 'Readings per second in the EdgeX platform', 'readings/s', 'throughput', 'edgex.readings', 
                    'line'],
        'lines': [
            ['readings', None, 'incremental']
        ]
    },
    'events_count':{
        'options': [None, 'Total number of events and readings in the EdgeX platform', 'events', 'events', 'edgex.events_readings_abs', 
                    'line'],
        'lines': [
            ['readings', None, 'absolute'],
            ['events', None, 'absolute']
        ]
    },
    'devices_number':{
        'options': [None, 'Number of registered devices in the EdgeX platform', 'devices', 'registered devices', 'edgex.devices_number', 
                    'line'],
        'lines': [
            ['registered_devices', None, 'absolute'],
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
        self.url = '{protocol}://{host}'.format(
            protocol=self.configuration.get('protocol', 'http'),
            host=self.configuration.get('host', 'localhost')
        )
        self.edgex_ports = {
            'core_data': self.configuration.get('port_data', '48080'),
            'core_metadata': self.configuration.get('port_metadata', '48081')
        }
        self.url_core_data_readings = '{url}:{port}/api/v1/reading/count'.format(
            url = self.url,
            port = self.edgex_ports['core_data']
        )
        self.url_core_data_events = '{url}:{port}/api/v1/event/count'.format(
            url = self.url,
            port = self.edgex_ports['core_data']
        )
        self.url_core_metadata_devices = '{url}:{port}/api/v1/device'.format(
            url = self.url,
            port = self.edgex_ports['core_metadata']
        )
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
                run=self.configuration.get('events_per_second', False),
            ),
            METHODS(
                get_data=self._get_core_throughput_data,
                url=self.url_core_data_events,
                run=self.configuration.get('readings_per_second', False)
            ),
            METHODS(
                get_data=self._get_device_info,
                url=self.url_core_metadata_devices,
                run=self.configuration.get('number_of_devices', False)
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
                data['events'] = raw
            elif 'reading' in url:
                data['readings'] = raw
            return queue.put(data)

    @get_survive_any
    def _get_device_info(self, queue, url):
            data = {}
            raw = self._get_raw_data(url) #json string
            if not raw:
                return queue.put({})
            parsed = json.loads(raw) #python object
            data['registered_devices'] = len(parsed) #int
            return queue.put(data)


