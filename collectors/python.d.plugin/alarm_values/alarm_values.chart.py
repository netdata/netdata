# -*- coding: utf-8 -*-
# Description: alarm_values netdata python.d module
# Author: andrewm4894
# SPDX-License-Identifier: GPL-3.0-or-later

from json import loads

from bases.FrameworkServices.UrlService import UrlService

update_every = 5
disabled_by_default = True


def charts_template():
    order = [
        'alarm_values',
    ]

    charts = {
        'alarm_values': {
            'options': [None, 'Alarm Values', 'value', 'values', 'alarm.values', 'line'],
            'lines': [],
            'variables': [
                [],
            ]
        }
    }
    return order, charts


DEFAULT_URL = 'http://127.0.0.1:19999/api/v1/alarms?all'


class Service(UrlService):
    def __init__(self, configuration=None, name=None):
        UrlService.__init__(self, configuration=configuration, name=name)
        self.order, self.definitions = charts_template()
        self.url = self.configuration.get('url', DEFAULT_URL)
        self.collected_alarms = set()

    def _get_data(self):
        raw_data = self._get_raw_data()
        if raw_data is None:
            return None

        raw_data = loads(raw_data)

        alarms = raw_data.get('alarms', {})

        data = {a: alarms[a]['value'] * 100 for a in alarms if 'value' in alarms[a] and alarms[a]['value'] is not None}
        self.update_charts(alarms, data)
        data['alarms_num'] = len(data)

        return data

    def update_charts(self, alarms, data):
        if not self.charts:
            return

        for a in data:
            if a not in self.collected_alarms:
                self.collected_alarms.add(a)
                self.charts['alarm_values'].add_dimension([a, a, 'absolute', '1', 100])

        for a in list(self.collected_alarms):
            if a not in alarms:
                self.collected_alarms.remove(a)
                self.charts['alarm_values'].del_dimension(a, hide=False)