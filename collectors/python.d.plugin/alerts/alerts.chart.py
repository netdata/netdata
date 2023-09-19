# -*- coding: utf-8 -*-
# Description: alerts netdata python.d module
# Author: andrewm4894
# SPDX-License-Identifier: GPL-3.0-or-later

from json import loads

from bases.FrameworkServices.UrlService import UrlService

update_every = 10
disabled_by_default = False


def charts_template():
    order = [
        'counts',
        'warning',
        'critical'
    ]

    charts = {
        'counts': {
            'options': [None, 'Alert Status', 'alerts', 'alerts', 'alerts.counts', 'stacked'],
            'lines': [],
        },
        'warning': {
            'options': [None, 'Warning Alerts', 'active', 'active', 'alerts.warning', 'stacked'],
            'lines': [],
        },
        'critical': {
            'options': [None, 'Critical Alerts', 'active', 'active', 'alerts.warning', 'stacked'],
            'lines': [],
        },
    }
    return order, charts


DEFAULT_URL = 'http://127.0.0.1:19999/api/v1/alarms?all'
DEFAULT_ALARM_CONTAINS_WORDS = ''
DEFAULT_ALARM_EXCLUDES_WORDS = ''


class Service(UrlService):
    def __init__(self, configuration=None, name=None):
        UrlService.__init__(self, configuration=configuration, name=name)
        self.order, self.definitions = charts_template()
        self.url = self.configuration.get('url', DEFAULT_URL)
        self.collected_dims = {'counts': {'clear','warning','critical'}, 'warning': set(), 'critical': set()}
        self.alert_contains_words = self.configuration.get('alert_contains_words', DEFAULT_ALARM_CONTAINS_WORDS)
        self.alert_contains_words_list = [alert_contains_word.lstrip(' ').rstrip(' ') for alert_contains_word in self.alert_contains_words.split(',')]
        self.alert_excludes_words = self.configuration.get('alert_excludes_words', DEFAULT_ALARM_EXCLUDES_WORDS)
        self.alert_excludes_words_list = [alert_excludes_word.lstrip(' ').rstrip(' ') for alert_excludes_word in self.alert_excludes_words.split(',')]

    def _get_data(self):
        raw_data = self._get_raw_data()
        if raw_data is None:
            return None

        raw_data = loads(raw_data)
        alerts = raw_data.get('alarms', {})
        if self.alert_contains_words != '':
            alerts = {alert_name: alerts[alert_name] for alert_name in alerts for alert_contains_word in
                      self.alert_contains_words_list if alert_contains_word in alert_name}
        if self.alert_excludes_words != '':
            alerts = {alert_name: alerts[alert_name] for alert_name in alerts for alert_excludes_word in
                      self.alert_excludes_words_list if alert_excludes_word not in alert_name}

        data = {
            'clear': len([a for a in alerts if alerts[a]['status'] == 'CLEAR'])
        }
        data_warning = {'{}_warning'.format(a): 1 for a in alerts if alerts[a]['status'] == 'WARNING'}
        data['warning'] = len(data_warning)
        self.update_charts('warning', data_warning)
        data.update(data_warning)
        data_critical = {'{}_critical'.format(a): 1 for a in alerts if alerts[a]['status'] == 'CRITICAL'}
        data['critical'] = len(data_critical)
        self.update_charts('critical', data_critical)
        data.update(data_critical)

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
