# -*- coding: utf-8 -*-
# Description:  varnish netdata python.d module
# Author: ilyam8
# SPDX-License-Identifier: GPL-3.0-or-later

import re

from bases.FrameworkServices.ExecutableService import ExecutableService
from bases.collection import find_binary

ORDER = [
    'session_connections',
    'client_requests',
    'all_time_hit_rate',
    'current_poll_hit_rate',
    'cached_objects_expired',
    'cached_objects_nuked',
    'threads_total',
    'threads_statistics',
    'threads_queue_len',
    'backend_connections',
    'backend_requests',
    'esi_statistics',
    'memory_usage',
    'uptime'
]

CHARTS = {
    'session_connections': {
        'options': [None, 'Connections Statistics', 'connections/s',
                    'client metrics', 'varnish.session_connection', 'line'],
        'lines': [
            ['sess_conn', 'accepted', 'incremental'],
            ['sess_dropped', 'dropped', 'incremental']
        ]
    },
    'client_requests': {
        'options': [None, 'Client Requests', 'requests/s',
                    'client metrics', 'varnish.client_requests', 'line'],
        'lines': [
            ['client_req', 'received', 'incremental']
        ]
    },
    'all_time_hit_rate': {
        'options': [None, 'All History Hit Rate Ratio', 'percentage', 'cache performance',
                    'varnish.all_time_hit_rate', 'stacked'],
        'lines': [
            ['cache_hit', 'hit', 'percentage-of-absolute-row'],
            ['cache_miss', 'miss', 'percentage-of-absolute-row'],
            ['cache_hitpass', 'hitpass', 'percentage-of-absolute-row']]
    },
    'current_poll_hit_rate': {
        'options': [None, 'Current Poll Hit Rate Ratio', 'percentage', 'cache performance',
                    'varnish.current_poll_hit_rate', 'stacked'],
        'lines': [
            ['cache_hit', 'hit', 'percentage-of-incremental-row'],
            ['cache_miss', 'miss', 'percentage-of-incremental-row'],
            ['cache_hitpass', 'hitpass', 'percentage-of-incremental-row']
        ]
    },
    'cached_objects_expired': {
        'options': [None, 'Expired Objects', 'expired/s', 'cache performance',
                    'varnish.cached_objects_expired', 'line'],
        'lines': [
            ['n_expired', 'objects', 'incremental']
        ]
    },
    'cached_objects_nuked': {
        'options': [None, 'Least Recently Used Nuked Objects', 'nuked/s', 'cache performance',
                    'varnish.cached_objects_nuked', 'line'],
        'lines': [
            ['n_lru_nuked', 'objects', 'incremental']
        ]
    },
    'threads_total': {
        'options': [None, 'Number Of Threads In All Pools', 'number', 'thread related metrics',
                    'varnish.threads_total', 'line'],
        'lines': [
            ['threads', None, 'absolute']
        ]
    },
    'threads_statistics': {
        'options': [None, 'Threads Statistics', 'threads/s', 'thread related metrics',
                    'varnish.threads_statistics', 'line'],
        'lines': [
            ['threads_created', 'created', 'incremental'],
            ['threads_failed', 'failed', 'incremental'],
            ['threads_limited', 'limited', 'incremental']
        ]
    },
    'threads_queue_len': {
        'options': [None, 'Current Queue Length', 'requests', 'thread related metrics',
                    'varnish.threads_queue_len', 'line'],
        'lines': [
            ['thread_queue_len', 'in queue']
        ]
    },
    'backend_connections': {
        'options': [None, 'Backend Connections Statistics', 'connections/s', 'backend metrics',
                    'varnish.backend_connections', 'line'],
        'lines': [
            ['backend_conn', 'successful', 'incremental'],
            ['backend_unhealthy', 'unhealthy', 'incremental'],
            ['backend_reuse', 'reused', 'incremental'],
            ['backend_toolate', 'closed', 'incremental'],
            ['backend_recycle', 'resycled', 'incremental'],
            ['backend_fail', 'failed', 'incremental']
        ]
    },
    'backend_requests': {
        'options': [None, 'Requests To The Backend', 'requests/s', 'backend metrics',
                    'varnish.backend_requests', 'line'],
        'lines': [
            ['backend_req', 'sent', 'incremental']
        ]
    },
    'esi_statistics': {
        'options': [None, 'ESI Statistics', 'problems/s', 'esi related metrics', 'varnish.esi_statistics', 'line'],
        'lines': [
            ['esi_errors', 'errors', 'incremental'],
            ['esi_warnings', 'warnings', 'incremental']
        ]
    },
    'memory_usage': {
        'options': [None, 'Memory Usage', 'MiB', 'memory usage', 'varnish.memory_usage', 'stacked'],
        'lines': [
            ['memory_free', 'free', 'absolute', 1, 1 << 20],
            ['memory_allocated', 'allocated', 'absolute', 1, 1 << 20]]
    },
    'uptime': {
        'lines': [
            ['uptime', None, 'absolute']
        ],
        'options': [None, 'Uptime', 'seconds', 'uptime', 'varnish.uptime', 'line']
    }
}


def backend_charts_template(name):
    order = [
        '{0}_response_statistics'.format(name),
    ]

    charts = {
        order[0]: {
            'options': [None, 'Backend "{0}"'.format(name), 'kilobits/s', 'backend response statistics',
                        'varnish.backend', 'area'],
            'lines': [
                ['{0}_beresp_hdrbytes'.format(name), 'header', 'incremental', 8, 1000],
                ['{0}_beresp_bodybytes'.format(name), 'body', 'incremental', -8, 1000]
            ]
        },
    }

    return order, charts


def disk_charts_template(name):
    order = [
        'disk_{0}_usage'.format(name),
    ]

    charts = {
        order[0]: {
            'options': [None, 'Disk "{0}" Usage'.format(name), 'KiB', 'disk usage', 'varnish.disk_usage', 'stacked'],
            'lines': [
                ['{0}.g_space'.format(name), 'free', 'absolute', 1, 1 << 10],
                ['{0}.g_bytes'.format(name), 'allocated', 'absolute', 1, 1 << 10]
            ]
        },
    }

    return order, charts


VARNISHSTAT = 'varnishstat'

re_version = re.compile(r'varnish-(?P<major>\d+)\.(?P<minor>\d+)\.(?P<patch>\d+)')


class VarnishVersion:
    def __init__(self, major, minor, patch):
        self.major = major
        self.minor = minor
        self.patch = patch

    def __str__(self):
        return '{0}.{1}.{2}'.format(self.major, self.minor, self.patch)


class Parser:
    _backend_new = re.compile(r'VBE.([\d\w_.]+)\(.*?\).(beresp[\w_]+)\s+(\d+)')
    _backend_old = re.compile(r'VBE\.[\d\w-]+\.([\w\d_]+).(beresp[\w_]+)\s+(\d+)')
    _default = re.compile(r'([A-Z]+\.)?([\d\w_.]+)\s+(\d+)')

    def __init__(self):
        self.re_default = None
        self.re_backend = None

    def init(self, data):
        data = ''.join(data)
        parsed_main = Parser._default.findall(data)
        if parsed_main:
            self.re_default = Parser._default

        parsed_backend = Parser._backend_new.findall(data)
        if parsed_backend:
            self.re_backend = Parser._backend_new
        else:
            parsed_backend = Parser._backend_old.findall(data)
            if parsed_backend:
                self.re_backend = Parser._backend_old

    def server_stats(self, data):
        return self.re_default.findall(''.join(data))

    def backend_stats(self, data):
        return self.re_backend.findall(''.join(data))


class Service(ExecutableService):
    def __init__(self, configuration=None, name=None):
        ExecutableService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.instance_name = configuration.get('instance_name')
        self.parser = Parser()
        self.command = None
        self.collected_vbe = set()
        self.collected_smf = set()

    def create_command(self):
        varnishstat = find_binary(VARNISHSTAT)

        if not varnishstat:
            self.error("can't locate '{0}' binary or binary is not executable by user netdata".format(VARNISHSTAT))
            return False

        command = [varnishstat, '-V']
        reply = self._get_raw_data(stderr=True, command=command)
        if not reply:
            self.error(
                "no output from '{0}'. Is varnish running? Not enough privileges?".format(' '.join(self.command)))
            return False

        ver = parse_varnish_version(reply)
        if not ver:
            self.error("failed to parse reply from '{0}', used regex :'{1}', reply : {2}".format(
                ' '.join(command), re_version.pattern, reply))
            return False

        if self.instance_name:
            self.command = [varnishstat, '-1', '-n', self.instance_name]
        else:
            self.command = [varnishstat, '-1']

        if ver.major > 4:
            self.command.extend(['-t', '1'])

        self.info("varnish version: {0}, will use command: '{1}'".format(ver, ' '.join(self.command)))

        return True

    def check(self):
        if not self.create_command():
            return False

        # STDOUT is not empty
        reply = self._get_raw_data()
        if not reply:
            self.error("no output from '{0}'. Is it running? Not enough privileges?".format(' '.join(self.command)))
            return False

        self.parser.init(reply)

        # Output is parsable
        if not self.parser.re_default:
            self.error('cant parse the output...')
            return False

        return True

    def get_data(self):
        """
        Format data received from shell command
        :return: dict
        """
        raw = self._get_raw_data()
        if not raw:
            return None

        data = dict()
        server_stats = self.parser.server_stats(raw)
        if not server_stats:
            return None

        stats = dict((param, value) for _, param, value in server_stats)
        data.update(stats)

        self.get_vbe_backends(data, raw)
        self.get_smf_disks(server_stats)

        # varnish 5 uses default.g_bytes and default.g_space
        data['memory_allocated'] = data.get('s0.g_bytes') or data.get('default.g_bytes')
        data['memory_free'] = data.get('s0.g_space') or data.get('default.g_space')

        return data

    def get_vbe_backends(self, data, raw):
        if not self.parser.re_backend:
            return
        stats = self.parser.backend_stats(raw)
        if not stats:
            return

        for (name, param, value) in stats:
            data['_'.join([name, param])] = value
            if name in self.collected_vbe:
                continue
            self.collected_vbe.add(name)
            self.add_backend_charts(name)

    def get_smf_disks(self, server_stats):
        #  [('SMF.', 'ssdStorage.c_req', '47686'),
        #  ('SMF.', 'ssdStorage.c_fail', '0'),
        #  ('SMF.', 'ssdStorage.c_bytes', '668102656'),
        #  ('SMF.', 'ssdStorage.c_freed', '140980224'),
        #  ('SMF.', 'ssdStorage.g_alloc', '39753'),
        #  ('SMF.', 'ssdStorage.g_bytes', '527122432'),
        #  ('SMF.', 'ssdStorage.g_space', '53159968768'),
        #  ('SMF.', 'ssdStorage.g_smf', '40130'),
        #  ('SMF.', 'ssdStorage.g_smf_frag', '311'),
        #  ('SMF.', 'ssdStorage.g_smf_large', '66')]
        disks = [name for typ, name, _ in server_stats if typ.startswith('SMF') and name.endswith('g_space')]
        if not disks:
            return
        for disk in disks:
            disk = disk.split('.')[0]  # ssdStorage
            if disk in self.collected_smf:
                continue
            self.collected_smf.add(disk)
            self.add_disk_charts(disk)

    def add_backend_charts(self, backend_name):
        self.add_charts(backend_name, backend_charts_template)

    def add_disk_charts(self, disk_name):
        self.add_charts(disk_name, disk_charts_template)

    def add_charts(self, name, charts_template):
        order, charts = charts_template(name)

        for chart_name in order:
            params = [chart_name] + charts[chart_name]['options']
            dimensions = charts[chart_name]['lines']

            new_chart = self.charts.add_chart(params)
            for dimension in dimensions:
                new_chart.add_dimension(dimension)


def parse_varnish_version(lines):
    m = re_version.search(lines[0])
    if not m:
        return None

    m = m.groupdict()
    return VarnishVersion(
        int(m['major']),
        int(m['minor']),
        int(m['patch']),
    )
