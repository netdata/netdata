# -*- coding: utf-8 -*-
# Description: apache netdata python.d module
# Author: Pawel Krupa (paulfantom)

from bases.FrameworkServices.UrlService import UrlService

# default module values (can be overridden per job in `config`)
# update_every = 2
priority = 60000
retries = 60

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
        'options': [None, 'apache Lifetime Avg. Response Size', 'bytes/request',
                    'statistics', 'apache.bytesperreq', 'area'],
        'lines': [
            ["size_req"]
        ]},
    'workers': {
        'options': [None, 'apache Workers', 'workers', 'workers', 'apache.workers', 'stacked'],
        'lines': [
            ["idle"],
            ["idle_servers", 'idle'],
            ["busy"],
            ["busy_servers", 'busy']
        ]},
    'reqpersec': {
        'options': [None, 'apache Lifetime Avg. Requests/s', 'requests/s', 'statistics',
                    'apache.reqpersec', 'area'],
        'lines': [
            ["requests_sec"]
        ]},
    'bytespersec': {
        'options': [None, 'apache Lifetime Avg. Bandwidth/s', 'kilobits/s', 'statistics',
                    'apache.bytesperreq', 'area'],
        'lines': [
            ["size_sec", None, 'absolute', 8, 1000]
        ]},
    'requests': {
        'options': [None, 'apache Requests', 'requests/s', 'requests', 'apache.requests', 'line'],
        'lines': [
            ["requests", None, 'incremental']
        ]},
    'net': {
        'options': [None, 'apache Bandwidth', 'kilobits/s', 'bandwidth', 'apache.net', 'area'],
        'lines': [
            ["sent", None, 'incremental', 8, 1]
        ]},
    'connections': {
        'options': [None, 'apache Connections', 'connections', 'connections', 'apache.connections', 'line'],
        'lines': [
            ["connections"]
        ]},
    'conns_async': {
        'options': [None, 'apache Async Connections', 'connections', 'connections', 'apache.conns_async', 'stacked'],
        'lines': [
            ["keepalive"],
            ["closing"],
            ["writing"]
        ]}
}

ASSIGNMENT = {"BytesPerReq": 'size_req',
              "IdleWorkers": 'idle',
              "IdleServers": 'idle_servers',
              "BusyWorkers": 'busy',
              "BusyServers": 'busy_servers',
              "ReqPerSec": 'requests_sec',
              "BytesPerSec": 'size_sec',
              "Total Accesses": 'requests',
              "Total kBytes": 'sent',
              "ConnsTotal": 'connections',
              "ConnsAsyncKeepAlive": 'keepalive',
              "ConnsAsyncClosing": 'closing',
              "ConnsAsyncWriting": 'writing'}


class Service(UrlService):
    def __init__(self, configuration=None, name=None):
        UrlService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.url = self.configuration.get('url', 'http://localhost/server-status?auto')

    def check(self):
        self._manager = self._build_manager()
        data = self._get_data()
        if not data:
            return None

        if 'idle_servers' in data:
            self.module_name = 'lighttpd'
            for chart in self.definitions:
                opts = self.definitions[chart]['options']
                opts[1] = opts[1].replace('apache', 'lighttpd')
                opts[4] = opts[4].replace('apache', 'lighttpd')
        return True

    def _get_data(self):
        """
        Format data received from http request
        :return: dict
        """
        raw_data = self._get_raw_data()
        if not raw_data:
            return None
        data = dict()

        for row in raw_data.split('\n'):
            tmp = row.split(":")
            if tmp[0] in ASSIGNMENT:
                try:
                    data[ASSIGNMENT[tmp[0]]] = int(float(tmp[1]))
                except (IndexError, ValueError):
                    continue
        return data or None
