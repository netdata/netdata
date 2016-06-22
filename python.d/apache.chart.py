# -*- coding: utf-8 -*-
# Description: apache netdata python.d plugin
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
#             'url': 'http://www.apache.org/server-status?auto'
#          }}

# charts order (can be overridden if you want less charts, or different order)
ORDER = ['requests', 'connections', 'conns_async', 'net', 'workers', 'reqpersec', 'bytespersec', 'bytesperreq']

CHARTS = {
    'bytesperreq': {
        'options': "'' 'apache Lifetime Avg. Response Size' 'bytes/request' statistics apache.bytesperreq area",
        'lines': [
            {"name": "size_req",
             "options": "'' absolute 1 1000000"}
        ]},
    'workers': {
        'options': "'' 'apache Workers' 'workers' workers apache.workers stacked",
        'lines': [
            {"name": "idle",
             "options": "'' absolute 1 1"},
            {"name": "busy",
             "options": "'' absolute 1 1"}
        ]},
    'reqpersec': {
        'options': "'' 'apache Lifetime Avg. Requests/s' 'requests/s' statistics apache.reqpersec area",
        'lines': [
            {"name": "requests_sec",
             "options": "'' absolute 1 1000000"}
        ]},
    'bytespersec': {
        'options': "'' 'apache Lifetime Avg. Bandwidth/s' 'kilobits/s' statistics apache.bytesperreq area",
        'lines': [
            {"name": "size_sec",
             "options": "'' absolute 8 1000000000"}
        ]},
    'requests': {
        'options': "'' 'apache Requests' 'requests/s' requests apache.requests line",
        'lines': [
            {"name": "requests",
             "options": "'' incremental 1 1"}
        ]},
    'net': {
        'options': "'' 'apache Bandwidth' 'kilobits/s' bandwidth apache.net area",
        'lines': [
            {"name": "sent",
             "options": "'' incremental 8 1"}
        ]},
    'connections': {
        'options': "'' 'apache Connections' 'connections' connections apache.connections line",
        'lines': [
            {"name": "connections",
             "options": "'' absolute 1 1"}
        ]},
    'conns_async': {
        'options': "'' 'apache Async Connections' 'connections' connections apache.conns_async stacked",
        'lines': [
            {"name": "keepalive",
             "options": "'' absolute 1 1"},
            {"name": "closing",
             "options": "'' absolute 1 1"},
            {"name": "writing",
             "options": "'' absolute 1 1"}
        ]}
}


class Service(UrlService):
    def __init__(self, configuration=None, name=None):
        UrlService.__init__(self, configuration=configuration, name=name)
        if len(self.url) == 0:
            self.url = "http://localhost/server-status?auto"
        self.order = ORDER
        self.charts = CHARTS
        self.assignment = {"BytesPerReq": 'size_req',
                           "IdleWorkers": 'idle',
                           "BusyWorkers": 'busy',
                           "ReqPerSec": 'requests_sec',
                           "BytesPerSec": 'size_sec',
                           "Total Accesses": 'requests',
                           "Total kBytes": 'sent',
                           "ConnsTotal": 'connections',
                           "ConnsAsyncKeepAlive": 'keepalive',
                           "ConnsAsyncClosing": 'closing',
                           "ConnsAsyncWriting": 'writing'}

    def _formatted_data(self):
        """
        Format data received from http request
        :return: dict
        """
        raw = self._get_data().split("\n")
        data = {}
        for row in raw:
            tmp = row.split(":")
            if str(tmp[0]) in self.assignment:
                try:
                    multiplier = 1
                    if tmp[0] in ("BytesPerReq", "ReqPerSec", "BytesPerSec"):
                        multiplier = 1000
                    data[self.assignment[tmp[0]]] = int(float(tmp[1])*multiplier)
                except (IndexError, ValueError) as a:
                    print(a)
                    pass
        if len(data) == 0:
            return None
        return data
