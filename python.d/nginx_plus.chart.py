# -*- coding: utf-8 -*-
# Description: nginx_plus netdata python.d module
# Author: Ilya Mashchenko (l2isbad)
# SPDX-License-Identifier: GPL-3.0+

import re

from collections import defaultdict
from copy import deepcopy
from json import loads

try:
    from collections import OrderedDict
except ImportError:
    from third_party.ordereddict import OrderedDict

from bases.FrameworkServices.UrlService import UrlService

# default module values (can be overridden per job in `config`)
update_every = 1
priority = 60000
retries = 60

# charts order (can be overridden if you want less charts, or different order)
ORDER = [
    'requests_total',
    'requests_current',
    'connections_statistics',
    'connections_workers',
    'ssl_handshakes',
    'ssl_session_reuses',
    'ssl_memory_usage',
    'processes'
]

CHARTS = {
    'requests_total': {
        'options': [None, 'Requests Total', 'requests/s', 'requests', 'nginx_plus.requests_total', 'line'],
        'lines': [
            ['requests_total', 'total', 'incremental']
        ]
    },
    'requests_current': {
        'options': [None, 'Requests Current', 'requests', 'requests', 'nginx_plus.requests_current', 'line'],
        'lines': [
            ['requests_current', 'current']
        ]
    },
    'connections_statistics': {
        'options': [None, 'Connections Statistics', 'connections/s',
                    'connections', 'nginx_plus.connections_statistics', 'stacked'],
        'lines': [
            ['connections_accepted', 'accepted', 'incremental'],
            ['connections_dropped', 'dropped', 'incremental']
        ]
    },
    'connections_workers': {
        'options': [None, 'Workers Statistics', 'workers',
                    'connections', 'nginx_plus.connections_workers', 'stacked'],
        'lines': [
            ['connections_idle', 'idle'],
            ['connections_active', 'active']
        ]
    },
    'ssl_handshakes': {
        'options': [None, 'SSL Handshakes', 'handshakes/s', 'ssl', 'nginx_plus.ssl_handshakes', 'stacked'],
        'lines': [
            ['ssl_handshakes', 'successful', 'incremental'],
            ['ssl_handshakes_failed', 'failed', 'incremental']
        ]
    },
    'ssl_session_reuses': {
        'options': [None, 'Session Reuses', 'sessions/s', 'ssl', 'nginx_plus.ssl_session_reuses', 'line'],
        'lines': [
            ['ssl_session_reuses', 'reused', 'incremental']
        ]
    },
    'ssl_memory_usage': {
        'options': [None, 'Memory Usage', '%', 'ssl', 'nginx_plus.ssl_memory_usage', 'area'],
        'lines': [
            ['ssl_memory_usage', 'usage', 'absolute', 1, 100]
        ]
    },
    'processes': {
        'options': [None, 'Processes', 'processes', 'processes', 'nginx_plus.processes', 'line'],
        'lines': [
            ['processes_respawned', 'respawned']
        ]
    }
}


def cache_charts(cache):
    family = 'cache {0}'.format(cache.real_name)
    charts = OrderedDict()

    charts['{0}_traffic'.format(cache.name)] = {
        'options': [None, 'Traffic', 'KB', family, 'nginx_plus.cache_traffic', 'stacked'],
        'lines': [
            ['_'.join([cache.name, 'hit_bytes']), 'served', 'absolute', 1, 1024],
            ['_'.join([cache.name, 'miss_bytes_written']), 'written', 'absolute', 1, 1024],
            ['_'.join([cache.name, 'miss_bytes']), 'bypass', 'absolute', 1, 1024]
        ]
    }
    charts['{0}_memory_usage'.format(cache.name)] = {
        'options': [None, 'Memory Usage', '%', family, 'nginx_plus.cache_memory_usage', 'area'],
        'lines': [
            ['_'.join([cache.name, 'memory_usage']), 'usage', 'absolute', 1, 100],
        ]
    }
    return charts


def web_zone_charts(wz):
    charts = OrderedDict()
    family = 'web zone {name}'.format(name=wz.real_name)

    # Processing
    charts['zone_{name}_processing'.format(name=wz.name)] = {
        'options': [None, 'Zone "{name}" Processing'.format(name=wz.name), 'requests', family,
                    'nginx_plus.web_zone_processing', 'line'],
        'lines': [
            ['_'.join([wz.name, 'processing']), 'processing']
        ]
    }
    # Requests
    charts['zone_{name}_requests'.format(name=wz.name)] = {
        'options': [None, 'Zone "{name}" Requests'.format(name=wz.name), 'requests/s', family,
                    'nginx_plus.web_zone_requests', 'line'],
        'lines': [
            ['_'.join([wz.name, 'requests']), 'requests', 'incremental']
        ]
    }
    # Response Codes
    charts['zone_{name}_responses'.format(name=wz.name)] = {
        'options': [None, 'Zone "{name}" Responses'.format(name=wz.name), 'requests/s', family,
                    'nginx_plus.web_zone_responses', 'stacked'],
        'lines': [
            ['_'.join([wz.name, 'responses_2xx']), '2xx', 'incremental'],
            ['_'.join([wz.name, 'responses_5xx']), '5xx', 'incremental'],
            ['_'.join([wz.name, 'responses_3xx']), '3xx', 'incremental'],
            ['_'.join([wz.name, 'responses_4xx']), '4xx', 'incremental'],
            ['_'.join([wz.name, 'responses_1xx']), '1xx', 'incremental']
        ]
    }
    # Traffic
    charts['zone_{name}_net'.format(name=wz.name)] = {
        'options': [None, 'Zone "{name}" Traffic'.format(name=wz.name), 'kilobits/s', family,
                    'nginx_plus.zone_net', 'area'],
        'lines': [
            ['_'.join([wz.name, 'received']), 'received', 'incremental', 1, 1000],
            ['_'.join([wz.name, 'sent']), 'sent', 'incremental', -1, 1000]
        ]
    }
    return charts


def web_upstream_charts(wu):
    def dimensions(value, a='absolute', m=1, d=1):
        dims = list()
        for p in wu:
            dims.append(['_'.join([wu.name, p.server, value]), p.real_server, a, m, d])
        return dims

    charts = OrderedDict()
    family = 'web upstream {name}'.format(name=wu.real_name)

    # Requests
    charts['web_upstream_{name}_requests'.format(name=wu.name)] = {
        'options': [None, 'Peers Requests', 'requests/s', family, 'nginx_plus.web_upstream_requests', 'line'],
        'lines': dimensions('requests', 'incremental')
    }
    # Responses Codes
    charts['web_upstream_{name}_all_responses'.format(name=wu.name)] = {
        'options': [None, 'All Peers Responses', 'responses/s', family,
                    'nginx_plus.web_upstream_all_responses', 'stacked'],
        'lines': [
            ['_'.join([wu.name, 'responses_2xx']), '2xx', 'incremental'],
            ['_'.join([wu.name, 'responses_5xx']), '5xx', 'incremental'],
            ['_'.join([wu.name, 'responses_3xx']), '3xx', 'incremental'],
            ['_'.join([wu.name, 'responses_4xx']), '4xx', 'incremental'],
            ['_'.join([wu.name, 'responses_1xx']), '1xx', 'incremental'],
        ]
    }
    for peer in wu:
        charts['web_upstream_{0}_{1}_responses'.format(wu.name, peer.server)] = {
            'options': [None, 'Peer "{0}" Responses'.format(peer.real_server), 'responses/s', family,
                        'nginx_plus.web_upstream_peer_responses', 'stacked'],
            'lines': [
                ['_'.join([wu.name, peer.server, 'responses_2xx']), '2xx', 'incremental'],
                ['_'.join([wu.name, peer.server, 'responses_5xx']), '5xx', 'incremental'],
                ['_'.join([wu.name, peer.server, 'responses_3xx']), '3xx', 'incremental'],
                ['_'.join([wu.name, peer.server, 'responses_4xx']), '4xx', 'incremental'],
                ['_'.join([wu.name, peer.server, 'responses_1xx']), '1xx', 'incremental']
            ]
        }
    # Connections
    charts['web_upstream_{name}_connections'.format(name=wu.name)] = {
        'options': [None, 'Peers Connections', 'active', family, 'nginx_plus.web_upstream_connections', 'line'],
        'lines': dimensions('active')
    }
    charts['web_upstream_{name}_connections_usage'.format(name=wu.name)] = {
        'options': [None, 'Peers Connections Usage', '%', family, 'nginx_plus.web_upstream_connections_usage', 'line'],
        'lines': dimensions('connections_usage', d=100)
    }
    # Traffic
    charts['web_upstream_{0}_all_net'.format(wu.name)] = {
        'options': [None, 'All Peers Traffic', 'kilobits/s', family, 'nginx_plus.web_upstream_all_net', 'area'],
        'lines': [
            ['{0}_received'.format(wu.name), 'received', 'incremental', 1, 1000],
            ['{0}_sent'.format(wu.name), 'sent', 'incremental', -1, 1000]
        ]
    }
    for peer in wu:
        charts['web_upstream_{0}_{1}_net'.format(wu.name, peer.server)] = {
            'options': [None, 'Peer "{0}" Traffic'.format(peer.real_server), 'kilobits/s', family,
                        'nginx_plus.web_upstream_peer_traffic', 'area'],
            'lines': [
                ['{0}_{1}_received'.format(wu.name, peer.server), 'received', 'incremental', 1, 1000],
                ['{0}_{1}_sent'.format(wu.name, peer.server), 'sent', 'incremental', -1, 1000]
            ]
        }
    # Response Time
    for peer in wu:
        charts['web_upstream_{0}_{1}_timings'.format(wu.name, peer.server)] = {
            'options': [None, 'Peer "{0}" Timings'.format(peer.real_server), 'ms', family,
                        'nginx_plus.web_upstream_peer_timings', 'line'],
            'lines': [
                ['_'.join([wu.name, peer.server, 'header_time']), 'header'],
                ['_'.join([wu.name, peer.server, 'response_time']), 'response']
            ]
        }
    # Memory Usage
    charts['web_upstream_{name}_memory_usage'.format(name=wu.name)] = {
        'options': [None, 'Memory Usage', '%', family, 'nginx_plus.web_upstream_memory_usage', 'area'],
        'lines': [
            ['_'.join([wu.name, 'memory_usage']), 'usage', 'absolute', 1, 100]
        ]
    }
    # State
    charts['web_upstream_{name}_status'.format(name=wu.name)] = {
        'options': [None, 'Peers Status', 'state', family, 'nginx_plus.web_upstream_status', 'line'],
        'lines': dimensions('state')
    }
    # Downtime
    charts['web_upstream_{name}_downtime'.format(name=wu.name)] = {
        'options': [None, 'Peers Downtime', 'seconds', family, 'nginx_plus.web_upstream_peer_downtime', 'line'],
        'lines': dimensions('downtime', d=1000)
    }

    return charts


METRICS = {
    'SERVER': [
        'processes.respawned',
        'connections.accepted',
        'connections.dropped',
        'connections.active',
        'connections.idle',
        'ssl.handshakes',
        'ssl.handshakes_failed',
        'ssl.session_reuses',
        'requests.total',
        'requests.current',
        'slabs.SSL.pages.free',
        'slabs.SSL.pages.used'
    ],
    'WEB_ZONE': [
        'processing',
        'requests',
        'responses.1xx',
        'responses.2xx',
        'responses.3xx',
        'responses.4xx',
        'responses.5xx',
        'discarded',
        'received',
        'sent'
    ],
    'WEB_UPSTREAM_PEER': [
        'id',
        'server',
        'name',
        'state',
        'active',
        'max_conns',
        'requests',
        'header_time',  # alive only
        'response_time',  # alive only
        'responses.1xx',
        'responses.2xx',
        'responses.3xx',
        'responses.4xx',
        'responses.5xx',
        'sent',
        'received',
        'downtime'
    ],
    'WEB_UPSTREAM_SUMMARY': [
        'responses.1xx',
        'responses.2xx',
        'responses.3xx',
        'responses.4xx',
        'responses.5xx',
        'sent',
        'received'
    ],
    'CACHE': [
        'hit.bytes',  # served
        'miss.bytes_written',  # written
        'miss.bytes'  # bypass

    ]
}

BAD_SYMBOLS = re.compile(r'[:/.-]+')


class Cache:
    key = 'caches'
    charts = cache_charts

    def __init__(self, **kw):
        self.real_name = kw['name']
        self.name = BAD_SYMBOLS.sub('_', self.real_name)

    def memory_usage(self, data):
        used = data['slabs'][self.real_name]['pages']['used']
        free = data['slabs'][self.real_name]['pages']['free']
        return used / float(free + used) * 1e4

    def get_data(self, raw_data):
        zone_data = raw_data['caches'][self.real_name]
        data = parse_json(zone_data, METRICS['CACHE'])
        data['memory_usage'] = self.memory_usage(raw_data)
        return dict(('_'.join([self.name, k]), v) for k, v in data.items())


class WebZone:
    key = 'server_zones'
    charts = web_zone_charts

    def __init__(self, **kw):
        self.real_name = kw['name']
        self.name = BAD_SYMBOLS.sub('_', self.real_name)

    def get_data(self, raw_data):
        zone_data = raw_data['server_zones'][self.real_name]
        data = parse_json(zone_data, METRICS['WEB_ZONE'])
        return dict(('_'.join([self.name, k]), v) for k, v in data.items())


class WebUpstream:
    key = 'upstreams'
    charts = web_upstream_charts

    def __init__(self, **kw):
        self.real_name = kw['name']
        self.name = BAD_SYMBOLS.sub('_', self.real_name)
        self.peers = OrderedDict()

        peers = kw['response']['upstreams'][self.real_name]['peers']
        for peer in peers:
            self.add_peer(peer['id'], peer['server'])

    def __iter__(self):
        return iter(self.peers.values())

    def add_peer(self, idx, server):
        peer = WebUpstreamPeer(idx, server)
        self.peers[peer.real_server] = peer
        return peer

    def peers_stats(self, peers):
        peers = {int(peer['id']): peer for peer in peers}
        data = dict()
        for peer in self.peers.values():
            if not peer.active:
                continue
            try:
                data.update(peer.get_data(peers[peer.id]))
            except KeyError:
                peer.active = False
        return data

    def memory_usage(self, data):
        used = data['slabs'][self.real_name]['pages']['used']
        free = data['slabs'][self.real_name]['pages']['free']
        return used / float(free + used) * 1e4

    def summary_stats(self, data):
        rv = defaultdict(int)
        for metric in METRICS['WEB_UPSTREAM_SUMMARY']:
            for peer in self.peers.values():
                if peer.active:
                    metric = '_'.join(metric.split('.'))
                    rv[metric] += data['_'.join([peer.server, metric])]
        return rv

    def get_data(self, raw_data):
        data = dict()
        peers = raw_data['upstreams'][self.real_name]['peers']
        data.update(self.peers_stats(peers))
        data.update(self.summary_stats(data))
        data['memory_usage'] = self.memory_usage(raw_data)
        return dict(('_'.join([self.name, k]), v) for k, v in data.items())


class WebUpstreamPeer:
    def __init__(self, idx, server):
        self.id = idx
        self.real_server = server
        self.server = BAD_SYMBOLS.sub('_', self.real_server)
        self.active = True

    def get_data(self, raw):
        data = dict(header_time=0, response_time=0, max_conns=0)
        data.update(parse_json(raw, METRICS['WEB_UPSTREAM_PEER']))
        data['connections_usage'] = 0 if not data['max_conns'] else data['active'] / float(data['max_conns']) * 1e4
        data['state'] = int(data['state'] == 'up')
        return dict(('_'.join([self.server, k]), v) for k, v in data.items())


class Service(UrlService):
    def __init__(self, configuration=None, name=None):
        UrlService.__init__(self, configuration=configuration, name=name)
        self.order = list(ORDER)
        self.definitions = deepcopy(CHARTS)
        self.objects = dict()

    def check(self):
        if not self.url:
            self.error('URL is not defined')
            return None

        self._manager = self._build_manager()
        if not self._manager:
            return None

        raw_data = self._get_raw_data()
        if not raw_data:
            return None

        try:
            response = loads(raw_data)
        except ValueError:
            return None

        for obj_cls in [WebZone, WebUpstream, Cache]:
            for obj_name in response.get(obj_cls.key, list()):
                obj = obj_cls(name=obj_name, response=response)
                self.objects[obj.real_name] = obj
                charts = obj_cls.charts(obj)
                for chart in charts:
                    self.order.append(chart)
                    self.definitions[chart] = charts[chart]

        return bool(self.objects)

    def _get_data(self):
        """
        Format data received from http request
        :return: dict
        """
        raw_data = self._get_raw_data()
        if not raw_data:
            return None
        response = loads(raw_data)

        data = parse_json(response, METRICS['SERVER'])
        data['ssl_memory_usage'] = data['slabs_SSL_pages_used'] / float(data['slabs_SSL_pages_free']) * 1e4

        for obj in self.objects.values():
            if obj.real_name in response[obj.key]:
                data.update(obj.get_data(response))

        return data


def parse_json(raw_data, metrics):
    data = dict()
    for metric in metrics:
        value = raw_data
        metrics_list = metric.split('.')
        try:
            for m in metrics_list:
                value = value[m]
        except KeyError:
            continue
        data['_'.join(metrics_list)] = value
    return data
