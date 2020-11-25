# -*- coding: utf-8 -*-
# Description: alarms netdata python.d module
# Author: andrewm4894
# SPDX-License-Identifier: GPL-3.0-or-later

from json import loads

from bases.FrameworkServices.UrlService import UrlService

priority = 200200
update_every = 5
disabled_by_default = True

DEFAULT_STATUS_MAP = {'CLEAR': 0, 'WARNING': 1, 'CRITICAL': 2}

ORDER = [
    'alarms',
]

CHARTS = {
    'alarms': {
        'options': ['alarms', 'Alarms', 'status', 'alarms', 'alarms.status', 'line'],
        'lines': []
    }
}


class Service(UrlService):
    def __init__(self, configuration=None, name=None):
        UrlService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.status_map = self.configuration.get('status_map', DEFAULT_STATUS_MAP)

    def validate_charts(self, name, data, algorithm='absolute', multiplier=1, divisor=1):
        for dim in data:
            if name not in self.charts:
                chart_params = [name] + CHARTS[name]['options']
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
        data['alarms_num'] = sum(data.values())
        self.validate_charts('alarms', data)

        return data
