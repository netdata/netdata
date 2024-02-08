# -*- coding: utf-8 -*-
# Description: alarms netdata python.d module
# Author: andrewm4894
# SPDX-License-Identifier: GPL-3.0-or-later

from json import loads

from bases.FrameworkServices.UrlService import UrlService

update_every = 10
disabled_by_default = True


def charts_template(sm, alarm_status_chart_type='line'):
    order = [
        'alarms',
        'values'
    ]

    mappings = ', '.join(['{0}={1}'.format(k, v) for k, v in sm.items()])
    charts = {
        'alarms': {
            'options': [None, 'Alarms ({0})'.format(mappings), 'status', 'status', 'alarms.status', alarm_status_chart_type],
            'lines': [],
            'variables': [
                ['alarms_num'],
            ]
        },
        'values': {
            'options': [None, 'Alarm Values', 'value', 'value', 'alarms.value', 'line'],
            'lines': [],
        }
    }
    return order, charts


DEFAULT_STATUS_MAP = {'CLEAR': 0, 'WARNING': 1, 'CRITICAL': 2}
DEFAULT_URL = 'http://127.0.0.1:19999/api/v1/alarms?all'
DEFAULT_COLLECT_ALARM_VALUES = False
DEFAULT_ALARM_STATUS_CHART_TYPE = 'line'
DEFAULT_ALARM_CONTAINS_WORDS = ''
DEFAULT_ALARM_EXCLUDES_WORDS = ''

class Service(UrlService):
    def __init__(self, configuration=None, name=None):
        UrlService.__init__(self, configuration=configuration, name=name)
        self.sm = self.configuration.get('status_map', DEFAULT_STATUS_MAP)
        self.alarm_status_chart_type = self.configuration.get('alarm_status_chart_type', DEFAULT_ALARM_STATUS_CHART_TYPE)
        self.order, self.definitions = charts_template(self.sm, self.alarm_status_chart_type)
        self.url = self.configuration.get('url', DEFAULT_URL)
        self.collect_alarm_values = bool(self.configuration.get('collect_alarm_values', DEFAULT_COLLECT_ALARM_VALUES))
        self.collected_dims = {'alarms': set(), 'values': set()}
        self.alarm_contains_words = self.configuration.get('alarm_contains_words', DEFAULT_ALARM_CONTAINS_WORDS)
        self.alarm_contains_words_list = [alarm_contains_word.lstrip(' ').rstrip(' ') for alarm_contains_word in self.alarm_contains_words.split(',')]
        self.alarm_excludes_words = self.configuration.get('alarm_excludes_words', DEFAULT_ALARM_EXCLUDES_WORDS)
        self.alarm_excludes_words_list = [alarm_excludes_word.lstrip(' ').rstrip(' ') for alarm_excludes_word in self.alarm_excludes_words.split(',')]

    def _get_data(self):
        raw_data = self._get_raw_data()
        if raw_data is None:
            return None

        raw_data = loads(raw_data)
        alarms = raw_data.get('alarms', {})
        if self.alarm_contains_words != '':
            alarms = {alarm_name: alarms[alarm_name] for alarm_name in alarms for alarm_contains_word in
                      self.alarm_contains_words_list if alarm_contains_word in alarm_name}
        if self.alarm_excludes_words != '':
            alarms = {alarm_name: alarms[alarm_name] for alarm_name in alarms for alarm_excludes_word in
                      self.alarm_excludes_words_list if alarm_excludes_word not in alarm_name}

        data = {a: self.sm[alarms[a]['status']] for a in alarms if alarms[a]['status'] in self.sm}
        self.update_charts('alarms', data)
        data['alarms_num'] = len(data)

        if self.collect_alarm_values:
            data_values = {'{}_value'.format(a): alarms[a]['value'] * 100 for a in alarms if 'value' in alarms[a] and alarms[a]['value'] is not None}
            self.update_charts('values', data_values, divisor=100)
            data.update(data_values)

        return data

    def update_charts(self, chart, data, algorithm='absolute', multiplier=1, divisor=1):
        if not self.charts:
            return

        for dim in data:
            if dim not in self.collected_dims[chart]:
                self.collected_dims[chart].add(dim)
                self.charts[chart].add_dimension([dim, dim, algorithm, multiplier, divisor])

        for dim in list(self.collected_dims[chart]):
            if dim not in data:
                self.collected_dims[chart].remove(dim)
                self.charts[chart].del_dimension(dim, hide=False)
