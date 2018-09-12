# -*- coding: utf-8 -*-
# Description: puppet netdata python.d module
# Author: Andrey Galkin <andrey@futoin.org> (andvgal)
# SPDX-License-Identifier: GPL-3.0+
#---
# This module should work both with OpenSource and PE versions
# of PuppetServer and PuppetDB.
#
# NOTE: PuppetDB may be configured to require proper TLS
#       client certificate for security reasons. Use tls_key_file
#       and tls_cert_file options then.
#---

from bases.FrameworkServices.UrlService import UrlService
from json import loads
import socket

update_every = 5
priority = 60000
# very long clojure-based service startup time
retries = 180

MB = 1048576
CPU_SCALE = 1000
ORDER = [
    'jvm_heap',
    'jvm_nonheap',
    'cpu',
    'fd_open',
]
CHARTS = {
    'jvm_heap': {
        'options': [None, "JVM Heap", "MB", "resources", "puppet.jvm", "area"],
        'lines': [
            ["jvm_heap_committed", 'committed', "absolute", 1, MB],
            ["jvm_heap_used", 'used', "absolute", 1, MB],
        ],
        'variables': [
            ['jvm_heap_max'],
            ['jvm_heap_init'],
        ],
    },
    'jvm_nonheap': {
        'options': [None, "JVM Non-Heap", "MB", "resources", "puppet.jvm", "area"],
        'lines': [
            ["jvm_nonheap_committed", 'committed', "absolute", 1, MB],
            ["jvm_nonheap_used", 'used', "absolute", 1, MB],
        ],
        'variables': [
            ['jvm_nonheap_max'],
            ['jvm_nonheap_init'],
        ],
    },
    'cpu': {
        'options': [None, "CPU usage", "percentage", "resources", "puppet.cpu", "stacked"],
        'lines': [
            ["cpu_time", 'execution', "absolute", 1, CPU_SCALE],
            ["gc_time", 'GC', "absolute", 1, CPU_SCALE],
        ]
    },
    'fd_open': {
        'options': [None, "File Descriptors", "descriptors", "resources", "puppet.fdopen", "line"],
        'lines': [
            ["fd_used", 'used', "absolute"],
        ],
        'variables': [
            ['fd_max'],
        ],
    },
}

class Service(UrlService):
    def __init__(self, configuration=None, name=None):
        UrlService.__init__(self, configuration=configuration, name=name)
        self.url = 'https://{0}:8140'.format(socket.getfqdn())
        self.order = ORDER
        self.definitions = CHARTS

    def _get_data(self):
        #---
        # NOTE: there are several ways to retrieve data
        # 1. Only PE versions:
        #    https://puppet.com/docs/pe/2018.1/api_status/status_api_metrics_endpoints.html
        # 2. Inidividual Metrics API (JMX):
        #    https://puppet.com/docs/pe/2018.1/api_status/metrics_api.html
        # 3. Extended status at debug level:
        #    https://puppet.com/docs/pe/2018.1/api_status/status_api_json_endpoints.html
        #
        # For sake of simplicity and efficiency the status one is used..
        #---

        raw_data = self._get_raw_data(self.url + '/status/v1/services?level=debug')

        if raw_data is None:
            return None

        raw_data = loads(raw_data)
        data = {}

        try:
            try:
                jvm_metrics = raw_data['status-service']['status']['experimental']['jvm-metrics']
            except KeyError:
                jvm_metrics = raw_data['status-service']['status']['jvm-metrics']

            heap_mem = jvm_metrics['heap-memory']
            non_heap_mem = jvm_metrics['non-heap-memory']

            for k in ['max', 'committed', 'used', 'init']:
                data['jvm_heap_'+k] = heap_mem[k]
                data['jvm_nonheap_'+k] = non_heap_mem[k]

            fd_open = jvm_metrics['file-descriptors']
            data['fd_max'] = fd_open['max']
            data['fd_used'] = fd_open['used']

            data['cpu_time'] = int(jvm_metrics['cpu-usage'] * CPU_SCALE)
            data['gc_time'] = int(jvm_metrics['gc-cpu-usage'] * CPU_SCALE)
        except KeyError:
            pass


        return data or None
