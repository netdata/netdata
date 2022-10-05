# -*- coding: utf-8 -*-
# Description: pandas netdata python.d module
# Author: Andrew Maguire (andrewm4894)
# SPDX-License-Identifier: GPL-3.0-or-later

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

    def check(self):
        """ensure charts and dims all confugured and that we can get data"""

        data = dict()

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
                
            exec(chart_config['processing_code'])
            data_tmp = locals()['data_tmp']
            data.update(data_tmp)

            for dim in data_tmp:
                self.charts[chart_config['chart_name']].add_dimension([dim, dim, 'absolute', 1, 1])

        return True

    def get_data(self):
        """get data for each chart config"""
        
        data = dict()

        for chart_config in self.chart_configs:

            exec(chart_config['processing_code'])
            data_tmp = locals()['data_tmp']
            data.update(data_tmp)

        return data