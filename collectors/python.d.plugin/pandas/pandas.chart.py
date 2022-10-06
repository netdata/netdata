# -*- coding: utf-8 -*-
# Description: pandas netdata python.d module
# Author: Andrew Maguire (andrewm4894)
# SPDX-License-Identifier: GPL-3.0-or-later

import pandas as pd
import requests

from bases.FrameworkServices.SimpleService import SimpleService

ORDER = []

CHARTS = {}


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.chart_configs = self.configuration.get('chart_configs', None)

    def run_code(self, df_steps):
        """eval() each line of code and ensure the result is a pandas dataframe"""

        # process each line of code
        lines = df_steps.split(';')
        for line in lines:
            line_clean = line.strip('\n').strip(' ')
            if line_clean != '':
                df = eval(line_clean)
                assert isinstance(df, pd.DataFrame), 'The result of each evaluated line of `df_steps` must be of type `pd.DataFrame`'
        
        # take top row of final df as data to be collected by netdata
        data = df.to_dict(orient='records')[0]

        return data

    def check(self):
        """ensure charts and dims all configured and that we can get data"""

        data = dict()

        # add each chart as defined by the config
        for chart_config in self.chart_configs:
            if chart_config['name'] not in self.charts:
                chart_template = {
                    'options': [
                        chart_config['name'],
                        chart_config['title'],
                        chart_config['units'],
                        chart_config['family'],
                        chart_config['context'],
                        chart_config['type']
                        ],
                    'lines': []
                    }
                self.charts.add_chart([chart_config['name']] + chart_template['options'])
                
            data_tmp = self.run_code(chart_config['df_steps'])
            data.update(data_tmp)

            for dim in data_tmp:
                self.charts[chart_config['name']].add_dimension([dim, dim, 'absolute', 1, 1])

        return True

    def get_data(self):
        """get data for each chart config"""

        data = dict()

        for chart_config in self.chart_configs:
            data_tmp = self.run_code(chart_config['df_steps'])
            data.update(data_tmp)

        return data
