# -*- coding: utf-8 -*-
# Description: PHP-FPM netdata python.d module
# Author: Pawel Krupa (paulfantom)

from base import UrlService

# default module values (can be overridden per job in `config`)
# update_every = 2
priority = 60000
retries = 60

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
        'options': [None, 'PHP-FPM Active Connections', 'connections', 'phpfpm', 'phpfpm.connections', 'line'],
        'lines': [
            ["active"],
            ["maxActive", 'max active'],
            ["idle"]
        ]},
    'requests': {
        'options': [None, 'PHP-FPM Requests', 'requests/s', 'phpfpm', 'phpfpm.requests', 'line'],
        'lines': [
            ["requests", None, "incremental"]
        ]},
    'performance': {
        'options': [None, 'PHP-FPM Performance', 'status', 'phpfpm', 'phpfpm.performance', 'line'],
        'lines': [
            ["reached", 'max children reached'],
            ["slow", 'slow requests']
        ]}
}


class Service(UrlService):
    def __init__(self, configuration=None, name=None):
        UrlService.__init__(self, configuration=configuration, name=name)
        if len(self.url) == 0:
            self.url = "http://localhost/status"
        self.order = ORDER
        self.definitions = CHARTS
        self.assignment = {"active processes": 'active',
                           "max active processes": 'maxActive',
                           "idle processes": 'idle',
                           "accepted conn": 'requests',
                           "max children reached": 'reached',
                           "slow requests": 'slow'}

    def _get_data(self):
        """
        Format data received from http request
        :return: dict
        """
        try:
            raw = self._get_raw_data().split('\n')
        except AttributeError:
            return None
        data = {}
        for row in raw:
            tmp = row.split(":")
            if str(tmp[0]) in self.assignment:
                try:
                    data[self.assignment[tmp[0]]] = int(tmp[1])
                except (IndexError, ValueError):
                    pass
        if len(data) == 0:
            return None
        return data
