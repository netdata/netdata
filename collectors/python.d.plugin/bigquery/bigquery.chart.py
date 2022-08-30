# -*- coding: utf-8 -*-
# Description: bigquery netdata python.d module
# Author: Andrew Maguire (andrewm4894)
# SPDX-License-Identifier: GPL-3.0-or-later

from google.oauth2 import service_account
import pandas_gbq

from bases.FrameworkServices.SimpleService import SimpleService

priority = 90000
update_every = 5

ORDER = []

CHARTS = {}


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.chart_configs = self.configuration.get('chart_configs', None)
        self.credentials = self.configuration.get('credentials', None)
        self.collected_dims = {}

    def check(self):
        
        for chart_config in self.chart_configs:
            if chart_config['chart_name'] not in self.charts:
                chart_template = {
                    'options': [None, chart_config['chart_name'], chart_config['chart_name'], chart_config['chart_name'], chart_config['chart_units'], chart_config['chart_type']],
                    'lines': []
                    }
                self.charts.add_chart([chart_config['chart_name']] + chart_template['options'])

        self.credentials = service_account.Credentials.from_service_account_file(self.credentials)

        data = dict()
        for chart_config in self.chart_configs:
            df = pandas_gbq.read_gbq(chart_config['sql'], project_id=chart_config['project_id'], credentials=self.credentials, progress_bar_type=None)
            chart_data = df.to_dict('records')[0] 
            data.update(chart_data)
            self.update_charts(chart_config['chart_name'], chart_data)

        return True

    def update_charts(self, chart, data, algorithm='absolute', multiplier=1, divisor=1):
        if not self.charts:
            return

        if chart not in self.collected_dims:
            self.collected_dims[chart] = set()

        for dim in data:
            if dim not in self.collected_dims[chart]:
                self.collected_dims[chart].add(dim)
                self.charts[chart].add_dimension([dim, dim, algorithm, multiplier, divisor])

        for dim in list(self.collected_dims[chart]):
            if dim not in data:
                self.collected_dims[chart].remove(dim)
                self.charts[chart].del_dimension(dim, hide=False)

    def get_data(self):

        data = dict()

        for chart_config in self.chart_configs:
            df = pandas_gbq.read_gbq(chart_config['sql'], project_id=chart_config['project_id'], credentials=self.credentials, progress_bar_type=None)
            chart_data = df.to_dict('records')[0] 
            data.update(chart_data)

        return data
