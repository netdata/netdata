# -*- coding: utf-8 -*-
# Description: nginx netdata python.d plugin
# Author: Pawel Krupa (paulfantom)

from base import UrlService

# default module values (can be overridden per job in `config`)
# update_every = 2
priority = 60000
retries = 5

# default job configuration (overridden by python.d.plugin)
# config = {'local': {
#             'update_every': update_every,
#             'retries': retries,
#             'priority': priority,
#             'url': 'http://localhost/stub_status'
#          }}

# charts order (can be overridden if you want less charts, or different order)
ORDER = ['connections', 'requests', 'connection_status', 'connect_rate']

CHARTS = {
    'connections': {
        'options': "'' 'nginx Active Connections' 'connections' nginx nginx.connections line",
        'lines': [
            {"name": "active",
             "options": "'' absolute 1 1"},
        ]},
    'requests': {
        'options': "'' 'nginx Requests' 'requests/s' nginx nginx.requests line",
        'lines': [
            {"name": "requests",
             "options": "'' incremental 1 1"}
        ]},
    'connection_status': {
        'options': "'' 'nginx Active Connections by Status' 'connections' nginx nginx.connection.status line",
        'lines': [
            {"name": "reading",
             "options": "'' absolute 1 1"},
            {"name": "writing",
             "options": "'' absolute 1 1"},
            {"name": "waiting",
             "options": "'' absolute 1 1"}
        ]},
    'connect_rate': {
        'options': "'' 'nginx Connections Rate' 'connections/s' nginx nginx.performance line",
        'lines': [
            {"name": "accepts",
             "options": "'accepted' incremental 1 1"},
            {"name": "handled",
             "options": "'' incremental 1 1"}
        ]}
}


class Service(UrlService):
    # url = "http://localhost/stub_status"
    url = "http://toothless.dragon/stub_status"
    order = ORDER
    charts = CHARTS

    def _formatted_data(self):
        """
        Format data received from http request
        :return: dict
        """
        raw = self._get_data().split(" ")
        try:
            return {'active': int(raw[2]),
                    'requests': int(raw[7]),
                    'reading': int(raw[8]),
                    'writing': int(raw[9]),
                    'waiting': int(raw[11]),
                    'accepts': int(raw[13]),
                    'handled': int(raw[15])}
        except ValueError:
            return None
