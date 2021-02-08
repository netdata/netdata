# -*- coding: utf-8 -*-
# Description: aggregator netdata python.d module
# Author: andrewm4894
# SPDX-License-Identifier: GPL-3.0-or-later

import urllib3
import json

from bases.FrameworkServices.SimpleService import SimpleService

http = urllib3.PoolManager()

ORDER = []

CHARTS = {}


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.definitions = CHARTS
        self.parent = self.configuration.get('parent', '127.0.0.1:19999')
        self.child_contains = self.configuration.get('child_contains', '')
        self.child_not_contains = self.configuration.get('child_not_contains', '')
        self.charts_to_agg = self.configuration.get('charts_to_agg', None)
        self.charts_to_agg = {
            self.charts_to_agg[n]['name']: {
                'agg_func': self.charts_to_agg[n].get('agg_func','mean'),
                'exclude_dims': self.charts_to_agg[n].get('exclude_dims','').split(',')
                } 
                for n in range(0,len(self.charts_to_agg))
        }
        self.refresh_children_every_n = self.configuration.get('refresh_children_every_n', 60)
        self.children = []
        self.parent_chart_defs = self.get_charts()
        self.child_chart_defs = self.get_charts()
        self.allmetrics = {}
        self.allmetrics_list = {c: {} for c in self.charts_to_agg}

    def check(self):
        if len(self.get_children()) >= 1:
            return True
        else:
            return False

    def validate_charts(self, name, data, title, units, family, context, chart_type='line', algorithm='absolute', multiplier=1, divisor=1):
        """Check if chart is defined, add it if not, then add each dimension as needed.
        """
        config = {'options': [name, title, units, family, context, chart_type]}
        if name not in self.charts:
            params = [name] + config['options']
            self.charts.add_chart(params=params)
        for dim in data:
            if dim not in self.charts[name]:
                self.charts[name].add_dimension([dim, dim, algorithm, multiplier, divisor])

    def get_charts(self, child=None):
        """Pull charts metedata from the Netdata REST API.
        """
        if child:
            url = 'http://{}/host/{}/api/v1/charts'.format(self.parent, child)
        else:
            url = 'http://{}/api/v1/charts'.format(self.parent)
        response = http.request('GET', url)
        data = json.loads(response.data.decode('utf-8'))
        return data.get('charts', {})

    def get_children(self):
        """Pull list of children from the Netdata REST API.
        """
        url = 'http://{}/api/v1/info'.format(self.parent)
        response = http.request('GET', url)
        data = json.loads(response.data.decode('utf-8'))
        return data.get('mirrored_hosts', {})

    def get_children_to_agg(self):
        """Define the list of children to aggregate over.
        """        
        self.children = self.get_children()
        if self.child_contains:
            self.children = [child for child in self.children if any(c in child for c in self.child_contains.split(','))]
        if self.child_not_contains:
            self.children = [child for child in self.children if not any(c in child for c in self.child_not_contains.split(','))]
        self.child_chart_defs = self.get_charts(child=self.children[0])
        self.info('aggregating data from {}'.format(self.children))

    def get_allmetrics(self, child):
        """Get allmetrics data into a json dictionary.
        """
        url = 'http://{}/host/{}/api/v1/allmetrics?format=json'.format(self.parent, child)
        response = http.request('GET', url)
        data = json.loads(response.data.decode('utf-8'))
        return data

    def scrape_children(self):
        """Get the chart dimension level data from the /allmetrics endpoint of each child.
        """
        for child in self.children:
            allmetrics_child = self.get_allmetrics(child)
            self.allmetrics[child] = {
                allmetrics_child.get(chart, {}).get('name', ''): allmetrics_child.get(chart, {}).get('dimensions', {})
                for chart in allmetrics_child if chart in self.charts_to_agg
            }

    def append_metrics(self):
        """For each chart in scope aggregate each dimension from each child into a list.
        """
        for child in self.allmetrics:
            for chart in self.allmetrics[child]:
                for dim in self.allmetrics[child][chart]:
                    if dim not in self.charts_to_agg[chart]['exclude_dims']:
                        if dim not in self.allmetrics_list[chart]:
                            self.allmetrics_list[chart][dim] = [self.allmetrics[child][chart][dim]['value']]
                        else:
                            self.allmetrics_list[chart][dim].append(self.allmetrics[child][chart][dim]['value'])

    def aggregate_data(self):
        """Aggregate the list of data for each dimension to one number.
        """
        data = {}
        for chart in self.allmetrics_list:
            data_chart = {}
            out_chart = f"{chart.replace('.','_')}"
            for dim in self.allmetrics_list[chart]:
                out_dim = f"{chart.replace('.','_')}_{dim}"
                x = self.allmetrics_list[chart][dim]
                if self.charts_to_agg[chart]['agg_func'] == 'min':
                    data_chart[out_dim] = min(x) * 1000
                elif self.charts_to_agg[chart]['agg_func'] == 'max':
                    data_chart[out_dim] = max(x) * 1000
                elif self.charts_to_agg[chart]['agg_func'] == 'sum':
                    data_chart[out_dim] = sum(x) * 1000
                elif self.charts_to_agg[chart]['agg_func'] == 'mean':
                    data_chart[out_dim] = ( sum(x) / len(x) ) * 1000
                else:
                    data_chart[out_dim] = ( sum(x) / len(x) ) * 1000

            self.validate_charts(
                name=out_chart,
                title=out_chart,
                units=self.child_chart_defs.get(chart,self.parent_chart_defs.get(chart, {'units': ''})).get('units', ''),
                family=chart.replace('.','_'),
                context=out_chart,
                chart_type=self.child_chart_defs.get(chart,self.parent_chart_defs.get(chart, {'chart_type': 'line'})).get('chart_type', 'line'),
                data=data_chart,
                divisor=1000
            )

            data = {**data, **data_chart}

        return data

    def reset_data(self):
        """Reset list of metrics for each chart to be empty.
        """
        self.allmetrics_list = {c: {} for c in self.charts_to_agg}

    def get_data(self):

        if len(self.children) <= 1 or self.runs_counter % self.refresh_children_every_n == 0:
            self.get_children_to_agg()

        if len(self.children) > 0:
            self.scrape_children()
            self.append_metrics()
            data = self.aggregate_data()
            self.reset_data()

        return data