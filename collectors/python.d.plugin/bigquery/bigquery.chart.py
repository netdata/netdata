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
        self.sql = 'select rand()*10000 as random0, rand()*10000 as random1'
        self.project_id = 'netdata-analytics-bi'
        self.credentials = '/tmp/key.json'
        self.chart_name = 'random'
        self.chart_type = 'line'
        self.chart_units = 'n'
        self.collected_dims = {}

    @staticmethod
    def check():

        if self.chart_name not in self.charts:
            self.charts[self.chart_name] = {
                'options': [None, self.chart_name, self.chart_name, self.chart_name, self.chart_units, self.chart_type],
                'lines': []
                }

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

        credentials = service_account.Credentials.from_service_account_file(self.credentials)
        df = pandas_gbq.read_gbq(self.sql, project_id=self.project_id, credentials=credentials, progress_bar_type=None)
        print(df)
        data = df.to_dict('records')[0]
        print(data)
        self.update_charts(self.chart_name, data)

        return data
