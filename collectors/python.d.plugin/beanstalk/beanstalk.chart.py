# -*- coding: utf-8 -*-
# Description: beanstalk netdata python.d module
# Author: ilyam8
# SPDX-License-Identifier: GPL-3.0-or-later

try:
    import beanstalkc

    BEANSTALKC = True
except ImportError:
    BEANSTALKC = False

from bases.FrameworkServices.SimpleService import SimpleService
from bases.loaders import load_yaml

ORDER = [
    'cpu_usage',
    'jobs_rate',
    'connections_rate',
    'commands_rate',
    'current_tubes',
    'current_jobs',
    'current_connections',
    'binlog',
    'uptime',
]

CHARTS = {
    'cpu_usage': {
        'options': [None, 'Cpu Usage', 'cpu time', 'server statistics', 'beanstalk.cpu_usage', 'area'],
        'lines': [
            ['rusage-utime', 'user', 'incremental'],
            ['rusage-stime', 'system', 'incremental']
        ]
    },
    'jobs_rate': {
        'options': [None, 'Jobs Rate', 'jobs/s', 'server statistics', 'beanstalk.jobs_rate', 'line'],
        'lines': [
            ['total-jobs', 'total', 'incremental'],
            ['job-timeouts', 'timeouts', 'incremental']
        ]
    },
    'connections_rate': {
        'options': [None, 'Connections Rate', 'connections/s', 'server statistics', 'beanstalk.connections_rate',
                    'area'],
        'lines': [
            ['total-connections', 'connections', 'incremental']
        ]
    },
    'commands_rate': {
        'options': [None, 'Commands Rate', 'commands/s', 'server statistics', 'beanstalk.commands_rate', 'stacked'],
        'lines': [
            ['cmd-put', 'put', 'incremental'],
            ['cmd-peek', 'peek', 'incremental'],
            ['cmd-peek-ready', 'peek-ready', 'incremental'],
            ['cmd-peek-delayed', 'peek-delayed', 'incremental'],
            ['cmd-peek-buried', 'peek-buried', 'incremental'],
            ['cmd-reserve', 'reserve', 'incremental'],
            ['cmd-use', 'use', 'incremental'],
            ['cmd-watch', 'watch', 'incremental'],
            ['cmd-ignore', 'ignore', 'incremental'],
            ['cmd-delete', 'delete', 'incremental'],
            ['cmd-release', 'release', 'incremental'],
            ['cmd-bury', 'bury', 'incremental'],
            ['cmd-kick', 'kick', 'incremental'],
            ['cmd-stats', 'stats', 'incremental'],
            ['cmd-stats-job', 'stats-job', 'incremental'],
            ['cmd-stats-tube', 'stats-tube', 'incremental'],
            ['cmd-list-tubes', 'list-tubes', 'incremental'],
            ['cmd-list-tube-used', 'list-tube-used', 'incremental'],
            ['cmd-list-tubes-watched', 'list-tubes-watched', 'incremental'],
            ['cmd-pause-tube', 'pause-tube', 'incremental']
        ]
    },
    'current_tubes': {
        'options': [None, 'Current Tubes', 'tubes', 'server statistics', 'beanstalk.current_tubes', 'area'],
        'lines': [
            ['current-tubes', 'tubes']
        ]
    },
    'current_jobs': {
        'options': [None, 'Current Jobs', 'jobs', 'server statistics', 'beanstalk.current_jobs', 'stacked'],
        'lines': [
            ['current-jobs-urgent', 'urgent'],
            ['current-jobs-ready', 'ready'],
            ['current-jobs-reserved', 'reserved'],
            ['current-jobs-delayed', 'delayed'],
            ['current-jobs-buried', 'buried']
        ]
    },
    'current_connections': {
        'options': [None, 'Current Connections', 'connections', 'server statistics',
                    'beanstalk.current_connections', 'line'],
        'lines': [
            ['current-connections', 'written'],
            ['current-producers', 'producers'],
            ['current-workers', 'workers'],
            ['current-waiting', 'waiting']
        ]
    },
    'binlog': {
        'options': [None, 'Binlog', 'records/s', 'server statistics', 'beanstalk.binlog', 'line'],
        'lines': [
            ['binlog-records-written', 'written', 'incremental'],
            ['binlog-records-migrated', 'migrated', 'incremental']
        ]
    },
    'uptime': {
        'options': [None, 'Uptime', 'seconds', 'server statistics', 'beanstalk.uptime', 'line'],
        'lines': [
            ['uptime'],
        ]
    }
}


def tube_chart_template(name):
    order = [
        '{0}_jobs_rate'.format(name),
        '{0}_jobs'.format(name),
        '{0}_connections'.format(name),
        '{0}_commands'.format(name),
        '{0}_pause'.format(name)
    ]
    family = 'tube {0}'.format(name)

    charts = {
        order[0]: {
            'options': [None, 'Job Rate', 'jobs/s', family, 'beanstalk.jobs_rate', 'area'],
            'lines': [
                ['_'.join([name, 'total-jobs']), 'jobs', 'incremental']
            ]
        },
        order[1]: {
            'options': [None, 'Jobs', 'jobs', family, 'beanstalk.jobs', 'stacked'],
            'lines': [
                ['_'.join([name, 'current-jobs-urgent']), 'urgent'],
                ['_'.join([name, 'current-jobs-ready']), 'ready'],
                ['_'.join([name, 'current-jobs-reserved']), 'reserved'],
                ['_'.join([name, 'current-jobs-delayed']), 'delayed'],
                ['_'.join([name, 'current-jobs-buried']), 'buried']
            ]
        },
        order[2]: {
            'options': [None, 'Connections', 'connections', family, 'beanstalk.connections', 'stacked'],
            'lines': [
                ['_'.join([name, 'current-using']), 'using'],
                ['_'.join([name, 'current-waiting']), 'waiting'],
                ['_'.join([name, 'current-watching']), 'watching']
            ]
        },
        order[3]: {
            'options': [None, 'Commands', 'commands/s', family, 'beanstalk.commands', 'stacked'],
            'lines': [
                ['_'.join([name, 'cmd-delete']), 'deletes', 'incremental'],
                ['_'.join([name, 'cmd-pause-tube']), 'pauses', 'incremental']
            ]
        },
        order[4]: {
            'options': [None, 'Pause', 'seconds', family, 'beanstalk.pause', 'stacked'],
            'lines': [
                ['_'.join([name, 'pause']), 'since'],
                ['_'.join([name, 'pause-time-left']), 'left']
            ]
        }
    }

    return order, charts


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.configuration = configuration
        self.order = list(ORDER)
        self.definitions = dict(CHARTS)
        self.conn = None
        self.alive = True

    def check(self):
        if not BEANSTALKC:
            self.error("'beanstalkc' module is needed to use beanstalk.chart.py")
            return False

        self.conn = self.connect()

        return True if self.conn else False

    def get_data(self):
        """
        :return: dict
        """
        if not self.is_alive():
            return None

        active_charts = self.charts.active_charts()
        data = dict()

        try:
            data.update(self.conn.stats())

            for tube in self.conn.tubes():
                stats = self.conn.stats_tube(tube)

                if tube + '_jobs_rate' not in active_charts:
                    self.create_new_tube_charts(tube)

                for stat in stats:
                    data['_'.join([tube, stat])] = stats[stat]

        except beanstalkc.SocketError:
            self.alive = False
            return None

        return data or None

    def create_new_tube_charts(self, tube):
        order, charts = tube_chart_template(tube)

        for chart_name in order:
            params = [chart_name] + charts[chart_name]['options']
            dimensions = charts[chart_name]['lines']

            new_chart = self.charts.add_chart(params)
            for dimension in dimensions:
                new_chart.add_dimension(dimension)

    def connect(self):
        host = self.configuration.get('host', '127.0.0.1')
        port = self.configuration.get('port', 11300)
        timeout = self.configuration.get('timeout', 1)
        try:
            return beanstalkc.Connection(host=host,
                                         port=port,
                                         connect_timeout=timeout,
                                         parse_yaml=load_yaml)
        except beanstalkc.SocketError as error:
            self.error('Connection to {0}:{1} failed: {2}'.format(host, port, error))
            return None

    def reconnect(self):
        try:
            self.conn.reconnect()
            self.alive = True
            return True
        except beanstalkc.SocketError:
            return False

    def is_alive(self):
        if not self.alive:
            return self.reconnect()
        return True
