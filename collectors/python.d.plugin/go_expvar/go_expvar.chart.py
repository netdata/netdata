# -*- coding: utf-8 -*-
# Description: go_expvar netdata python.d module
# Author: Jan Kral (kralewitz)
# SPDX-License-Identifier: GPL-3.0-or-later

from __future__ import division
import json

from bases.FrameworkServices.UrlService import UrlService

# default module values (can be overridden per job in `config`)
# update_every = 2
priority = 60000

MEMSTATS_CHARTS = {
    'memstats_heap': {
        'options': ['heap', 'memory: size of heap memory structures', 'kB', 'memstats',
                    'expvar.memstats.heap', 'line'],
        'lines': [
            ['memstats_heap_alloc', 'alloc', 'absolute', 1, 1024],
            ['memstats_heap_inuse', 'inuse', 'absolute', 1, 1024]
        ]
    },
    'memstats_stack': {
        'options': ['stack', 'memory: size of stack memory structures', 'kB', 'memstats',
                    'expvar.memstats.stack', 'line'],
        'lines': [
            ['memstats_stack_inuse', 'inuse', 'absolute', 1, 1024]
        ]
    },
    'memstats_mspan': {
        'options': ['mspan', 'memory: size of mspan memory structures', 'kB', 'memstats',
                    'expvar.memstats.mspan', 'line'],
        'lines': [
            ['memstats_mspan_inuse', 'inuse', 'absolute', 1, 1024]
        ]
    },
    'memstats_mcache': {
        'options': ['mcache', 'memory: size of mcache memory structures', 'kB', 'memstats',
                    'expvar.memstats.mcache', 'line'],
        'lines': [
            ['memstats_mcache_inuse', 'inuse', 'absolute', 1, 1024]
        ]
    },
    'memstats_live_objects': {
        'options': ['live_objects', 'memory: number of live objects', 'objects', 'memstats',
                    'expvar.memstats.live_objects', 'line'],
        'lines': [
            ['memstats_live_objects', 'live']
        ]
    },
    'memstats_sys': {
        'options': ['sys', 'memory: size of reserved virtual address space', 'kB', 'memstats',
                    'expvar.memstats.sys', 'line'],
        'lines': [
            ['memstats_sys', 'sys', 'absolute', 1, 1024]
        ]
    },
    'memstats_gc_pauses': {
        'options': ['gc_pauses', 'memory: average duration of GC pauses', 'ns', 'memstats',
                    'expvar.memstats.gc_pauses', 'line'],
        'lines': [
            ['memstats_gc_pauses', 'avg']
        ]
    }
}

MEMSTATS_ORDER = ['memstats_heap', 'memstats_stack', 'memstats_mspan', 'memstats_mcache',
                  'memstats_sys', 'memstats_live_objects', 'memstats_gc_pauses']


def flatten(d, top='', sep='.'):
    items = []
    for key, val in d.items():
        nkey = top + sep + key if top else key
        if isinstance(val, dict):
            items.extend(flatten(val, nkey, sep=sep).items())
        else:
            items.append((nkey, val))
    return dict(items)


class Service(UrlService):
    def __init__(self, configuration=None, name=None):
        UrlService.__init__(self, configuration=configuration, name=name)

        # if memstats collection is enabled, add the charts and their order
        if self.configuration.get('collect_memstats'):
            self.definitions = dict(MEMSTATS_CHARTS)
            self.order = list(MEMSTATS_ORDER)
        else:
            self.definitions = dict()
            self.order = list()

        # if extra charts are defined, parse their config
        extra_charts = self.configuration.get('extra_charts')
        if extra_charts:
            self._parse_extra_charts_config(extra_charts)

    def check(self):
        """
        Check if the module can collect data:
        1) At least one JOB configuration has to be specified
        2) The JOB configuration needs to define the URL and either collect_memstats must be enabled or at least one
           extra_chart must be defined.

        The configuration and URL check is provided by the UrlService class.
        """

        if not (self.configuration.get('extra_charts') or self.configuration.get('collect_memstats')):
            self.error('Memstats collection is disabled and no extra_charts are defined, disabling module.')
            return False

        return UrlService.check(self)

    def _parse_extra_charts_config(self, extra_charts_config):

        # a place to store the expvar keys and their types
        self.expvars = dict()

        for chart in extra_charts_config:

            chart_dict = dict()
            chart_id = chart.get('id')
            chart_lines = chart.get('lines')
            chart_opts = chart.get('options', dict())

            if not all([chart_id, chart_lines]):
                self.info('Chart {0} has no ID or no lines defined, skipping'.format(chart))
                continue

            chart_dict['options'] = [
                chart_opts.get('name', ''),
                chart_opts.get('title', ''),
                chart_opts.get('units', ''),
                chart_opts.get('family', ''),
                chart_opts.get('context', ''),
                chart_opts.get('chart_type', 'line')
            ]
            chart_dict['lines'] = list()

            # add the lines to the chart
            for line in chart_lines:

                ev_key = line.get('expvar_key')
                ev_type = line.get('expvar_type')
                line_id = line.get('id')

                if not all([ev_key, ev_type, line_id]):
                    self.info('Line missing expvar_key, expvar_type, or line_id, skipping: {0}'.format(line))
                    continue

                if ev_type not in ['int', 'float']:
                    self.info('Unsupported expvar_type "{0}". Must be "int" or "float"'.format(ev_type))
                    continue

                if ev_key in self.expvars:
                    self.info('Duplicate expvar key {0}: skipping line.'.format(ev_key))
                    continue

                self.expvars[ev_key] = (ev_type, line_id)

                chart_dict['lines'].append(
                    [
                        line.get('id', ''),
                        line.get('name', ''),
                        line.get('algorithm', ''),
                        line.get('multiplier', 1),
                        line.get('divisor', 100 if ev_type == 'float' else 1),
                        line.get('hidden', False)
                    ]
                )

            self.order.append(chart_id)
            self.definitions[chart_id] = chart_dict

    def _get_data(self):
        """
        Format data received from http request
        :return: dict
        """

        raw_data = self._get_raw_data()
        if not raw_data:
            return None

        data = json.loads(raw_data)

        expvars = dict()
        if self.configuration.get('collect_memstats'):
            expvars.update(self._parse_memstats(data))

        if self.configuration.get('extra_charts'):
            # the memstats part of the data has been already parsed, so we remove it before flattening and checking
            #   the rest of the data, thus avoiding needless iterating over the multiply nested memstats dict.
            del (data['memstats'])
            flattened = flatten(data)
            for k, v in flattened.items():
                ev = self.expvars.get(k)
                if not ev:
                    # expvar is not defined in config, skip it
                    continue
                try:
                    key_type, line_id = ev
                    if key_type == 'int':
                        expvars[line_id] = int(v)
                    elif key_type == 'float':
                        # if the value type is float, multiply it by 1000 and set line divisor to 1000
                        expvars[line_id] = float(v) * 100
                except ValueError:
                    self.info('Failed to parse value for key {0} as {1}, ignoring key.'.format(k, key_type))
                    del self.expvars[k]

        return expvars

    @staticmethod
    def _parse_memstats(data):

        memstats = data['memstats']

        # calculate the number of live objects in memory
        live_objs = int(memstats['Mallocs']) - int(memstats['Frees'])

        # calculate GC pause times average
        # the Go runtime keeps the last 256 GC pause durations in a circular buffer,
        #  so we need to filter out the 0 values before the buffer is filled
        gc_pauses = memstats['PauseNs']
        try:
            gc_pause_avg = sum(gc_pauses) / len([x for x in gc_pauses if x > 0])
        # no GC cycles have occured yet
        except ZeroDivisionError:
            gc_pause_avg = 0

        return {
            'memstats_heap_alloc': memstats['HeapAlloc'],
            'memstats_heap_inuse': memstats['HeapInuse'],
            'memstats_stack_inuse': memstats['StackInuse'],
            'memstats_mspan_inuse': memstats['MSpanInuse'],
            'memstats_mcache_inuse': memstats['MCacheInuse'],
            'memstats_sys': memstats['Sys'],
            'memstats_live_objects': live_objs,
            'memstats_gc_pauses': gc_pause_avg,
        }
