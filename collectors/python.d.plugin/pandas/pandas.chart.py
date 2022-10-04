# -*- coding: utf-8 -*-
# Description: pandas netdata python.d module
# Author: Andrew Maguire (andrewm4894)
# SPDX-License-Identifier: GPL-3.0-or-later

from google.oauth2 import service_account
import pandas as pd

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
        
        for chart_config in self.chart_configs:
            
            # get data and add dims to charts
            if chart_config['source_format'] == 'csv':
                df = pd.read_csv(chart_config['url'], storage_options={'User-Agent': 'netdata'})
            elif chart_config['source_format'] == 'json':
                df = pd.read_json(chart_config['url'], storage_options={'User-Agent': 'netdata'})
            else:
                raise ValueError('unsupported source_format')

            exec(chart_config['processing_code'])

            for dim in data:
                self.charts[chart_config['chart_name']].add_dimension([dim, dim, 'absolute', 1, 1])

        return True

    def get_data(self):
        """get data for each chart config"""

        for chart_config in self.chart_configs:

            # get data and add dims to charts
            if chart_config['source_format'] == 'csv':
                df = pd.read_csv(chart_config['url'], storage_options={'User-Agent': 'netdata'})
            elif chart_config['source_format'] == 'json':
                df = pd.read_json(chart_config['url'], storage_options={'User-Agent': 'netdata'})
            else:
                raise ValueError('unsupported source_format')

            exec(chart_config['processing_code'])

        return data