# -*- coding: utf-8 -*-
# Description: tomcat netdata python.d module
# Author: Wing924

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
            ["resp.other",  'Other', 'incremental'],
            ["resp.1xx",    '1xx',   'incremental'],
            ["resp.2xx",    '2xx',   'incremental'],
            ["resp.3xx",    '3xx',   'incremental'],
            ["resp.4xx",    '4xx',   'incremental'],
            ["resp.5xx",    '5xx',   'incremental'],
        ]},
    'threads': {
        'options': [None, "Threads", "current threads", "threads", "springboot.threads", "area"],
        'lines': [
            ["threads.daemon", 'daemon', 'absolute'],
            ["threads", 'total', 'absolute'],
        ]},
    'gc_time': {
        'options': [None, "GC Time", "milliseconds", "garbage collection", "springboot.gc_time", "area"],
        'lines': [
            ["gc.copy.time",                'Copy',                 'incremental'],
            ["gc.marksweepcompact.time",    'MarkSweepCompact',     'incremental'],
            ["gc.parnew.time",              'ParNew',               'incremental'],
            ["gc.concurrentmarksweep.time", 'ConcurrentMarkSweep',  'incremental'],
            ["gc.ps_scavenge.time",         'PS Scavenge',          'incremental'],
            ["gc.ps_marksweep.time",        'PS MarkSweep',         'incremental'],
            ["gc.g1_young_generation.time", 'G1 Young Generation',  'incremental'],
            ["gc.g1_old_generation.time",   'G1 Old Generation',    'incremental'],
        ]},
    'gc_ope': {
        'options': [None, "GC Operations", "operations/s", "garbage collection", "springboot.gc_ope", "area"],
        'lines': [
            ["gc.copy.count",                'Copy',                 'incremental'],
            ["gc.marksweepcompact.count",    'MarkSweepCompact',     'incremental'],
            ["gc.parnew.count",              'ParNew',               'incremental'],
            ["gc.concurrentmarksweep.count", 'ConcurrentMarkSweep',  'incremental'],
            ["gc.ps_scavenge.count",         'PS Scavenge',          'incremental'],
            ["gc.ps_marksweep.count",        'PS MarkSweep',         'incremental'],
            ["gc.g1_young_generation.count", 'G1 Young Generation',  'incremental'],
            ["gc.g1_old_generation.count",   'G1 Old Generation',    'incremental'],
        ]},
    'heap': {
        'options': [None, "Heap Memory Usage", "KB", "heap memory", "springboot.heap", "area"],
        'lines': [
            ["heap.committed", 'committed', "absolute"],
            ["heap.used", 'used', "absolute"],
        ]},
}

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
        data = None
        raw_data = self._get_raw_data()
        if raw_data:
            try:
                data = json.loads(raw_data)
            except Excepton:
                self.debug('%s is not a vaild JSON page' % self.url)
                return None

            for key in ['1xx', '2xx', '3xx', '4xx', '5xx', 'other']:
                data['resp.' + key] = 0
            for key, value in data.iteritems():
                if 'counter.status.' in key:
                    try:
                        status_type = key[15:16] + 'xx'
                    except:
                        status_type = 'other'

                    data['resp.' + status_type] += value

        return data or None

    def _setup_charts(self):
        order = []
        definitions = {}
        defaults = self.configuration.get('defaults', {})

        for chart in DEFAULT_ORDER:
            if defaults.get(chart, True):
                order.append(chart)
                definitions[chart] = DEFAULT_CHARTS[chart]

        for extra in self.configuration.get('extras', []):
            self._add_extra_chart(definitions, extra)
            order.append(extra['id'])

        self.order = order
        self.definitions = definitions

    def _add_extra_chart(self, charts, chart):
        chart_id  = chart.get('id',      None) or die('id is not defined in extra chart')
        options   = chart.get('options', None) or die('option is not defined in extra chart: %s' % chart_id)
        lines     = chart.get('lines',   None) or die('lines is not defined in extra chart: %s' % chart_id)

        title     = options.get('title', None) or die('title is missing: %s' chart_id)
        units     = options.get('units', None) or die('units is missing: %s' chart_id)
        family    = options.get('family',    title)
        context   = options.get('context',   'springboot.' + title)
        charttype = options.get('charttype', 'line')

        result = {
            'options': [None, title, units, family, context, charttype],
            'lines': [],
        }

        for line in lines:
            dimension  = line.get('dimension',  None) or self.die('dimension is missing: %s' chart_id)
            name       = line.get('name',       dimension)
            algorithm  = line.get('algorithm',  'absolute')
            multiplier = line.get('multiplier', 1)
            divisor    = line.get('divisor',    1)
            result['lines'].append([dimension, name, algorithm, multiplier, divisor])

        charts[chart_id] = result

    @classmethod
    def die(error_message):
        raise Exception(error_message)
