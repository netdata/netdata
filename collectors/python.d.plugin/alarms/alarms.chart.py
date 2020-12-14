# -*- coding: utf-8 -*-
# Description: alarms netdata python.d module
# Author: andrewm4894
# SPDX-License-Identifier: GPL-3.0-or-later

from json import loads

from bases.FrameworkServices.UrlService import UrlService

update_every = 10
disabled_by_default = True


def charts_template(sm):
    order = [
        'alarms',
    ]

    mappings = ', '.join(['{0}={1}'.format(k, v) for k, v in sm.items()])
    charts = {
        'alarms': {
            'options': [None, 'Alarms ({0})'.format(mappings), 'status', 'alarms', 'alarms.status', 'line'],
            'lines': [],
            'variables': [
                ['alarms_num'],
            ]
        }
    }
    return order, charts


DEFAULT_STATUS_MAP = {'CLEAR': 0, 'WARNING': 1, 'CRITICAL': 2}

DEFAULT_URL = 'http://127.0.0.1:19999/api/v1/alarms?all'


class Service(UrlService):
    def __init__(self, configuration=None, name=None):
        UrlService.__init__(self, configuration=configuration, name=name)
        self.sm = self.configuration.get('status_map', DEFAULT_STATUS_MAP)
        self.order, self.definitions = charts_template(self.sm)
        self.url = self.configuration.get('url', DEFAULT_URL)
        self.collected_alarms = set()

    def _get_data(self):
        raw_data = self._get_raw_data()
        if raw_data is None:
            return None

        raw_data = loads(raw_data)
        alarms = raw_data.get('alarms', {})

        data = {a: self.sm[alarms[a]['status']] for a in alarms if alarms[a]['status'] in self.sm}
        self.update_charts(alarms, data)
        data['alarms_num'] = len(data)

        return data

    def update_charts(self, alarms, data):
        if not self.charts:
            return

        for a in data:
            if a not in self.collected_alarms:
                self.collected_alarms.add(a)
                self.charts['alarms'].add_dimension([a, a, 'absolute', '1', '1'])

        for a in list(self.collected_alarms):
            if a not in alarms:
                self.collected_alarms.remove(a)
                self.charts['alarms'].del_dimension(a, hide=False)
