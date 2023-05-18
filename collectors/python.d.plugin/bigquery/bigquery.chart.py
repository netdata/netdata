# -*- coding: utf-8 -*-
# Description: bigquery netdata python.d module
# Author: Andrew Maguire (andrewm4894)
# SPDX-License-Identifier: GPL-3.0-or-later

from google.oauth2 import service_account
import pandas_gbq

from bases.FrameworkServices.SimpleService import SimpleService

ORDER = []

CHARTS = {}


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.chart_configs = self.configuration.get('chart_configs', None)
        self.credentials = self.configuration.get('credentials', None)

    def check(self):
        """ensure charts and dims all confugured and that we can get data"""

        # add each chart as defined by the config
        for chart_config in self.chart_configs:
            if chart_config['chart_name'] not in self.charts:
                chart_template = {
                    'options': [
                        chart_config['chart_name'],
                        chart_config['chart_title'],
                        chart_config['chart_units'],
                        chart_config['chart_family'],
                        chart_config['chart_context'],
                        chart_config['chart_type']
                        ],
                    'lines': []
                    }
                self.charts.add_chart([chart_config['chart_name']] + chart_template['options'])

        # get credentials if set
        if self.credentials:
            self.credentials = service_account.Credentials.from_service_account_file(self.credentials)

        # get data and add dims to charts
        data = dict()
        for chart_config in self.chart_configs:
            df = pandas_gbq.read_gbq(
                chart_config['sql'],
                project_id=chart_config['project_id'],
                credentials=self.credentials,
                progress_bar_type=None)
            chart_data = df.to_dict('records')[0]
            data.update(chart_data)
            for dim in chart_data:
                self.charts[chart_config['chart_name']].add_dimension([dim, dim, 'absolute', 1, 1])

        return True

    def get_data(self):
        """get data for each chart config"""

        data = dict()

        for chart_config in self.chart_configs:
            df = pandas_gbq.read_gbq(
                chart_config['sql'],
                project_id=chart_config['project_id'],
                credentials=self.credentials,
                progress_bar_type=None
            )
            chart_data = df.to_dict('records')[0]
            data.update(chart_data)

        return data
