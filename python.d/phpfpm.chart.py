# -*- coding: utf-8 -*-
# Description: PHP-FPM netdata python.d module
# Author: Pawel Krupa (paulfantom)

from base import UrlService
import json

# default module values (can be overridden per job in `config`)
# update_every = 2
priority = 60000
retries = 60

# default job configuration (overridden by python.d.plugin)
# config = {'local': {
#     'update_every': update_every,
#     'retries': retries,
#     'priority': priority,
#     'url': 'http://localhost/status?full&json'
# }}

# charts order (can be overridden if you want less charts, or different order)
ORDER = ['connections', 'requests', 'performance', 'request_duration', 'request_cpu', 'request_mem']

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
        ]},
    'request_duration': {
        'options': [None, 'PHP-FPM Request Duration', 'milliseconds', 'phpfpm', 'phpfpm.request_duration', 'line'],
        'lines': [
            ["maxReqDur", 'max request duration'],
            ["avgReqDur", 'average request duration']
        ]},
    'request_cpu': {
        'options': [None, 'PHP-FPM Request CPU', 'percent', 'phpfpm', 'phpfpm.request_cpu', 'line'],
        'lines': [
            ["maxReqCPU", 'max request cpu'],
            ["avgReqCPU", 'average request cpu']
        ]},
    'request_mem': {
        'options': [None, 'PHP-FPM Request Memory', 'kilobytes', 'phpfpm', 'phpfpm.request_mem', 'line'],
        'lines': [
            ["maxReqMem", 'max request memory'],
            ["avgReqMem", 'average request memory']
        ]}
}


class Service(UrlService):
    def __init__(self, configuration=None, name=None):
        UrlService.__init__(self, configuration=configuration, name=name)
        if len(self.url) == 0:
            self.url = "http://localhost/status?full&json"
        self.order = ORDER
        self.definitions = CHARTS
        self.assignment = {"active processes": 'active',
                           "max active processes": 'maxActive',
                           "idle processes": 'idle',
                           "accepted conn": 'requests',
                           "max children reached": 'reached',
                           "slow requests": 'slow'}
        self.proc_assignment = {"request duration": 'ReqDur',
                                "last request cpu": 'ReqCPU',
                                "last request memory": 'ReqMem'}

    def _get_data(self):
        """
        Format data received from http request
        :return: dict
        """
        try:
            raw = self._get_raw_data()
        except AttributeError:
            return None

        if '?json' in self.url or '&json' in self.url:
            try:
                raw_json = json.loads(raw)
            except ValueError:
                return None
            data = {self.assignment[k]: v for k, v in raw_json.items() if k in self.assignment}
            if '&full' in self.url or '?full' in self.url:
                c = 0
                for proc in raw_json['processes']:
                    if proc['state'] != 'Idle':
                        continue
                    c += 1
                    for k, v in self.proc_assignment.items():
                        d = proc[k]
                        if v == 'ReqDur':
                            d = d/1000
                        if v == 'ReqMem':
                            d = d/1024
                        if 'max' + v not in data or data['max' + v] < d:
                            data['max' + v] = d
                        if 'avg' + v not in data:
                            data['avg' + v] = 0
                        data['avg' + v] = (data['avg' + v] + d) / c
            if len(data) == 0:
                return None
            return data

        raw = raw.split('\n')
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
