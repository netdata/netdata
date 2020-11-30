# -*- coding: utf-8 -*-
# Description: alarms netdata python.d module
# Author: andrewm4894
# SPDX-License-Identifier: GPL-3.0-or-later

from json import loads

from bases.FrameworkServices.UrlService import UrlService

update_every = 10
disabled_by_default = False

DEFAULT_STATUS_MAP = {'CLEAR': 0, 'WARNING': 1, 'CRITICAL': 2}

ORDER = [
    'alarms',
]

CHARTS = {
}


class Service(UrlService):
    def __init__(self, configuration=None, name=None):
        UrlService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.url = self.configuration.get('url', 'http://127.0.0.1:19999/api/v1/alarms?all')
        self.status_map = self.configuration.get('status_map', DEFAULT_STATUS_MAP)
        self.chart_title = f"Alarms ({', '.join([f'{k}={self.status_map[k]}' for k in self.status_map])})"

    def validate_charts(self, name, data, algorithm='absolute', multiplier=1, divisor=1):
        for dim in data:
            if name not in self.charts:
                chart_params = [name] + ['alarms', self.chart_title, 'status', 'alarms', 'alarms.status', 'line']
                self.charts.add_chart(params=chart_params)
            if dim not in self.charts[name]:
                self.charts[name].add_dimension([dim, dim, algorithm, multiplier, divisor])

    def _get_data(self):
        raw_data = self._get_raw_data()
        if raw_data is None:
            return None
        raw_data = loads(raw_data)
        alarms = raw_data.get('alarms', {})
        data = {a: self.status_map[alarms[a]['status']] for a in alarms if alarms[a]['status'] in self.status_map}
        self.validate_charts('alarms', data)
        data['alarms_num'] = len(data)

        return data
