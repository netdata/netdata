# -*- coding: utf-8 -*-
# Description: PHP-FPM netdata python.d module
# Author: Pawel Krupa (paulfantom)
# Author: Ilya Mashchenko (ilyam8)
# SPDX-License-Identifier: GPL-3.0-or-later

import json
import re

from bases.FrameworkServices.UrlService import UrlService

REGEX = re.compile(r'([a-z][a-z ]+): ([\d.]+)')

POOL_INFO = [
    ('active processes', 'active'),
    ('max active processes', 'maxActive'),
    ('idle processes', 'idle'),
    ('accepted conn', 'requests'),
    ('max children reached', 'reached'),
    ('slow requests', 'slow')
]

PER_PROCESS_INFO = [
    ('request duration', 'ReqDur'),
    ('last request cpu', 'ReqCpu'),
    ('last request memory', 'ReqMem')
]


def average(collection):
    return sum(collection, 0.0) / max(len(collection), 1)


CALC = [
    ('min', min),
    ('max', max),
    ('avg', average)
]

ORDER = [
    'connections',
    'requests',
    'performance',
    'request_duration',
    'request_cpu',
    'request_mem',
]

CHARTS = {
    'connections': {
        'options': [None, 'PHP-FPM Active Connections', 'connections', 'active connections', 'phpfpm.connections',
                    'line'],
        'lines': [
            ['active'],
            ['maxActive', 'max active'],
            ['idle']
        ]
    },
    'requests': {
        'options': [None, 'PHP-FPM Requests', 'requests/s', 'requests', 'phpfpm.requests', 'line'],
        'lines': [
            ['requests', None, 'incremental']
        ]
    },
    'performance': {
        'options': [None, 'PHP-FPM Performance', 'status', 'performance', 'phpfpm.performance', 'line'],
        'lines': [
            ['reached', 'max children reached'],
            ['slow', 'slow requests']
        ]
    },
    'request_duration': {
        'options': [None, 'PHP-FPM Request Duration', 'milliseconds', 'request duration', 'phpfpm.request_duration',
                    'line'],
        'lines': [
            ['minReqDur', 'min', 'absolute', 1, 1000],
            ['maxReqDur', 'max', 'absolute', 1, 1000],
            ['avgReqDur', 'avg', 'absolute', 1, 1000]
        ]
    },
    'request_cpu': {
        'options': [None, 'PHP-FPM Request CPU', 'percentage', 'request CPU', 'phpfpm.request_cpu', 'line'],
        'lines': [
            ['minReqCpu', 'min'],
            ['maxReqCpu', 'max'],
            ['avgReqCpu', 'avg']
        ]
    },
    'request_mem': {
        'options': [None, 'PHP-FPM Request Memory', 'KB', 'request memory', 'phpfpm.request_mem', 'line'],
        'lines': [
            ['minReqMem', 'min', 'absolute', 1, 1024],
            ['maxReqMem', 'max', 'absolute', 1, 1024],
            ['avgReqMem', 'avg', 'absolute', 1, 1024]
        ]
    }
}


class Service(UrlService):
    def __init__(self, configuration=None, name=None):
        UrlService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.url = self.configuration.get('url', 'http://localhost/status?full&json')
        self.json = '&json' in self.url or '?json' in self.url
        self.json_full = self.url.endswith(('?full&json', '?json&full'))
        self.if_all_processes_running = dict(
            [(c_name + p_name, 0) for c_name, func in CALC for metric, p_name in PER_PROCESS_INFO]
        )

    def _get_data(self):
        """
        Format data received from http request
        :return: dict
        """
        raw = self._get_raw_data()
        if not raw:
            return None

        raw_json = parse_raw_data_(is_json=self.json, raw_data=raw)

        # Per Pool info: active connections, requests and performance charts
        to_netdata = fetch_data_(raw_data=raw_json, metrics_list=POOL_INFO)

        # Per Process Info: duration, cpu and memory charts (min, max, avg)
        if self.json_full:
            p_info = dict()
            to_netdata.update(self.if_all_processes_running)  # If all processes are in running state
            # Metrics are always 0 if the process is not in Idle state because calculation is done
            #  when the request processing has terminated
            for process in [p for p in raw_json['processes'] if p['state'] == 'Idle']:
                p_info.update(fetch_data_(raw_data=process, metrics_list=PER_PROCESS_INFO, pid=str(process['pid'])))

            if p_info:
                for new_name in PER_PROCESS_INFO:
                    for name, func in CALC:
                        to_netdata[name + new_name[1]] = func([p_info[k] for k in p_info if new_name[1] in k])

        return to_netdata or None


def fetch_data_(raw_data, metrics_list, pid=''):
    """
    :param raw_data: dict
    :param metrics_list: list
    :param pid: str
    :return: dict
    """
    result = dict()
    for metric, new_name in metrics_list:
        if metric in raw_data:
            result[new_name + pid] = float(raw_data[metric])
    return result


def parse_raw_data_(is_json, raw_data):
    """
    :param is_json: bool
    :param regex: compiled regular expr
    :param raw_data: dict
    :return: dict
    """
    if is_json:
        try:
            return json.loads(raw_data)
        except ValueError:
            return dict()
    else:
        raw_data = ' '.join(raw_data.split())
        return dict(REGEX.findall(raw_data))
