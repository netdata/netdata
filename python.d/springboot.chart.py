# -*- coding: utf-8 -*-
# Description: tomcat netdata python.d module
# Author: Wing924
# SPDX-License-Identifier: GPL-3.0-or-later

import json
from bases.FrameworkServices.UrlService import UrlService

# default module values (can be overridden per job in `config`)
# update_every = 2
priority = 60000
retries = 60


DEFAULT_ORDER = ['response_code', 'threads', 'gc_time', 'gc_ope', 'heap']

DEFAULT_CHARTS = {
    'response_code': {
        'options': [None, "Response Codes", "requests/s", "response", "springboot.response_code", "stacked"],
        'lines': [
            ["resp_other", 'Other', 'incremental'],
            ["resp_1xx", '1xx', 'incremental'],
            ["resp_2xx", '2xx', 'incremental'],
            ["resp_3xx", '3xx', 'incremental'],
            ["resp_4xx", '4xx', 'incremental'],
            ["resp_5xx", '5xx', 'incremental'],
        ]
    },
    'threads': {
        'options': [None, "Threads", "current threads", "threads", "springboot.threads", "area"],
        'lines': [
            ["threads_daemon", 'daemon', 'absolute'],
            ["threads", 'total', 'absolute'],
        ]
    },
    'gc_time': {
        'options': [None, "GC Time", "milliseconds", "garbage collection", "springboot.gc_time", "stacked"],
        'lines': [
            ["gc_copy_time", 'Copy', 'incremental'],
            ["gc_marksweepcompact_time", 'MarkSweepCompact', 'incremental'],
            ["gc_parnew_time", 'ParNew', 'incremental'],
            ["gc_concurrentmarksweep_time", 'ConcurrentMarkSweep', 'incremental'],
            ["gc_ps_scavenge_time", 'PS Scavenge', 'incremental'],
            ["gc_ps_marksweep_time", 'PS MarkSweep', 'incremental'],
            ["gc_g1_young_generation_time", 'G1 Young Generation', 'incremental'],
            ["gc_g1_old_generation_time", 'G1 Old Generation', 'incremental'],
        ]
    },
    'gc_ope': {
        'options': [None, "GC Operations", "operations/s", "garbage collection", "springboot.gc_ope", "stacked"],
        'lines': [
            ["gc_copy_count", 'Copy', 'incremental'],
            ["gc_marksweepcompact_count", 'MarkSweepCompact', 'incremental'],
            ["gc_parnew_count", 'ParNew', 'incremental'],
            ["gc_concurrentmarksweep_count", 'ConcurrentMarkSweep', 'incremental'],
            ["gc_ps_scavenge_count", 'PS Scavenge', 'incremental'],
            ["gc_ps_marksweep_count", 'PS MarkSweep', 'incremental'],
            ["gc_g1_young_generation_count", 'G1 Young Generation', 'incremental'],
            ["gc_g1_old_generation_count", 'G1 Old Generation', 'incremental'],
        ]
    },
    'heap': {
        'options': [None, "Heap Memory Usage", "KB", "heap memory", "springboot.heap", "area"],
        'lines': [
            ["heap_committed", 'committed', "absolute"],
            ["heap_used", 'used', "absolute"],
        ]
    }
}


class ExtraChartError(ValueError):
    pass


class Service(UrlService):
    def __init__(self, configuration=None, name=None):
        UrlService.__init__(self, configuration=configuration, name=name)
        self.url = self.configuration.get('url', "http://localhost:8080/metrics")
        self._setup_charts()

    def _get_data(self):
        """
        Format data received from http request
        :return: dict
        """
        raw_data = self._get_raw_data()
        if not raw_data:
            return None

        try:
            data = json.loads(raw_data)
        except ValueError:
            self.debug('%s is not a vaild JSON page' % self.url)
            return None

        result = {
            'resp_1xx': 0,
            'resp_2xx': 0,
            'resp_3xx': 0,
            'resp_4xx': 0,
            'resp_5xx': 0,
            'resp_other': 0,
        }

        for key, value in data.iteritems():
            if 'counter.status.' in key:
                status_type = key[15:16] + 'xx'
                if status_type[0] not in '12345':
                    status_type = 'other'
                result['resp_' + status_type] += value
            else:
                result[key.replace('.', '_')] = value

        return result or None

    def _setup_charts(self):
        self.order = []
        self.definitions = {}
        defaults = self.configuration.get('defaults', {})

        for chart in DEFAULT_ORDER:
            if defaults.get(chart, True):
                self.order.append(chart)
                self.definitions[chart] = DEFAULT_CHARTS[chart]

        for extra in self.configuration.get('extras', []):
            self._add_extra_chart(extra)
            self.order.append(extra['id'])

    def _add_extra_chart(self, chart):
        chart_id = chart.get('id', None) or self.die('id is not defined in extra chart')
        options = chart.get('options', None) or self.die('option is not defined in extra chart: %s' % chart_id)
        lines = chart.get('lines', None) or self.die('lines is not defined in extra chart: %s' % chart_id)

        title = options.get('title', None) or self.die('title is missing: %s' % chart_id)
        units = options.get('units', None) or self.die('units is missing: %s' % chart_id)
        family = options.get('family', title)
        context = options.get('context', 'springboot.' + title)
        charttype = options.get('charttype', 'line')

        result = {
            'options': [None, title, units, family, context, charttype],
            'lines': [],
        }

        for line in lines:
            dimension = line.get('dimension',  None) or self.die('dimension is missing: %s' % chart_id)
            name = line.get('name', dimension)
            algorithm = line.get('algorithm', 'absolute')
            multiplier = line.get('multiplier', 1)
            divisor = line.get('divisor', 1)
            result['lines'].append([dimension, name, algorithm, multiplier, divisor])

        self.definitions[chart_id] = result

    @staticmethod
    def die(error_message):
        raise ExtraChartError(error_message)
