# -*- coding: utf-8 -*-
# Description: PHP-FPM netdata python.d plugin
# Author: Pawel Krupa (paulfantom)

from base import UrlService

# default module values (can be overridden per job in `config`)
# update_every = 2
priority = 60000
retries = 5

# default job configuration (overridden by python.d.plugin)
# config = {'local': {
#     'update_every': update_every,
#     'retries': retries,
#     'priority': priority,
#     'url': 'http://localhost/status'
# }}

# charts order (can be overridden if you want less charts, or different order)
ORDER = ['connections', 'requests', 'performance']

CHARTS = {
    'connections': {
        'options': "'' 'PHP-FPM Active Connections' 'connections' phpfpm phpfpm.connections line",
        'lines': [
            {"name": "active",
             "options": "'' absolute 1 1"},
            {"name": "maxActive",
             "options": "'max active' absolute 1 1"},
            {"name": "idle",
             "options": "'' absolute 1 1"}
        ]},
    'requests': {
        'options': "'' 'PHP-FPM Requests' 'requests/s' phpfpm phpfpm.requests line",
        'lines': [
            {"name": "requests",
             "options": "'' incremental 1 1"}
        ]},
    'performance': {

        'options': "'' 'PHP-FPM Performance' 'status' phpfpm phpfpm.performance line",
        'lines': [
            {"name": "reached",
             "options": "'max children reached' absolute 1 1"},
            {"name": "slow",
             "options": "'slow requests' absolute 1 1"}
        ]}
}


class Service(UrlService):
    url = "http://localhost/status"
    order = ORDER
    charts = CHARTS
    assignment = {"active processes": 'active',
                  "max active processes": 'maxActive',
                  "idle processes": 'idle',
                  "accepted conn": 'requests',
                  "max children reached": 'reached',
                  "slow requests": 'slow'}

    def _formatted_data(self):
        """
        Format data received from http request
        :return: dict
        """
        raw = self._get_data().split('\n')
        data = {}
        for row in raw:
            tmp = row.split(":")
            if str(tmp[0]) in self.assignment:
                try:
                    data[self.assignment[tmp[0]]] = int(tmp[1])
                except (IndexError, ValueError) as a:
                    pass
        if len(data) == 0:
            return None
        return data
