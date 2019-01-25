# -*- coding: utf-8 -*-
# Description: example netdata python.d module
# Author: Put your name here (your github login)
# SPDX-License-Identifier: GPL-3.0-or-later

from copy import deepcopy
from random import SystemRandom

from bases.FrameworkServices.PrometheusService import PrometheusService, get_metric_by_name

disabled_by_default = True


priority = 90000

ORDER = [
    'jobs_processed',
    'jobs_unprocessed',
    'memory',
]

CHARTS = {
    'jobs_processed': {
        'options': [None, 'Jobs Processed Rate', 'jobs/s', 'jobs', 'example.jobs_processed', 'line'],
        'lines': [
            ['example_jobs_processed', 'processed', 'incremental'],
        ]
    },
    'jobs_unprocessed': {
        'options': [None, 'Jobs Unprocessed', 'jobs', 'jobs', 'example.jobs_unprocessed', 'line'],
        'lines': [
            ['example_jobs_unprocessed', 'unprocessed'],
        ]
    },
    'memory': {
        'options': [None, 'Memory Usage', 'KiB', 'memory', 'example.memory', 'line'],
        'lines': [
            ['example_memory_used_bytes', 'usage', 'absolute', 1, 1024],
        ]
    },
}

METRICS_SAMPLE = '''
# Jobs Processed
#
# HELP example_jobs_processed_total Jobs Processed by the Example Service
# TYPE example_jobs_processed_total counter
example_jobs_processed_total{type="service",name="Example"} %d
#
# Jobs Unprocessed
#
# HELP example_jobs_unprocessed Jobs Unprocessed by the Example Service
# TYPE example_jobs_unprocessed gauge
example_jobs_unprocessed{type="service",name="Example"} %d
#
# Memory Usage
#
# HELP example_memory_used_bytes Memory used by the Example Service in bytes
# TYPE example_memory_used_bytes gauge
example_memory_used_bytes{type="service",name="Example"} %d
'''


class MockPrometheusExporter:
    def __init__(self):
        self.p = 0
        self.random = SystemRandom()

    def export(self):
        self.p += self.random.randint(10, 30)

        return METRICS_SAMPLE % (
            self.p,
            self.random.randint(1, 10),
            self.random.randint(1024 * 1024, 8192 * 1024),
        )


class Service(PrometheusService):
    def __init__(self, configuration=None, name=None):
        PrometheusService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = deepcopy(CHARTS)
        self.random = SystemRandom()
        self.exp = MockPrometheusExporter()
        self.url = "http://127.0.0.1"

    def _scrape(self, *args, **kwargs):
        return self.exp.export()

    def _get_data(self):
        metrics = self._get_raw_data(self)

        if not metrics:
            return None

        data = dict()

        m = get_metric_by_name(metrics, 'example_jobs_processed')
        if m:
            data[m.name] = m.samples[0].value

        m = get_metric_by_name(metrics, 'example_jobs_unprocessed')
        if m:
            data[m.name] = m.samples[0].value

        m = get_metric_by_name(metrics, 'example_memory_used_bytes')
        if m:
            data[m.name] = m.samples[0].value

        return data or None
