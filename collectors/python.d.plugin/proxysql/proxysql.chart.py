# -*- coding: utf-8 -*-
# Description: Proxysql netdata python.d module
# Author: Ali Borhani (alibo)
# SPDX-License-Identifier: GPL-3.0+

from bases.FrameworkServices.MySQLService import MySQLService

# default module values (can be overridden per job in `config`)
# update_every = 3
priority = 60000
retries = 60


def query(table, *params):
    return 'SELECT {params} FROM {table}'.format(table=table, params=', '.join(params))


# https://github.com/sysown/proxysql/blob/master/doc/admin_tables.md#stats_mysql_global
QUERY_GLOBAL = query(
    "stats_mysql_global",
    "Variable_Name",
    "Variable_Value"
)

# https://github.com/sysown/proxysql/blob/master/doc/admin_tables.md#stats_mysql_connection_pool
QUERY_CONNECTION_POOL = query(
    "stats_mysql_connection_pool",
    "hostgroup",
    "srv_host",
    "srv_port",
    "status",
    "ConnUsed",
    "ConnFree",
    "ConnOK",
    "ConnERR",
    "Queries",
    "Bytes_data_sent",
    "Bytes_data_recv",
    "Latency_us"
)

# https://github.com/sysown/proxysql/blob/master/doc/admin_tables.md#stats_mysql_commands_counters
QUERY_COMMANDS = query(
    "stats_mysql_commands_counters",
    "Command",
    "Total_Time_us",
    "Total_cnt",
    "cnt_100us",
    "cnt_500us",
    "cnt_1ms",
    "cnt_5ms",
    "cnt_10ms",
    "cnt_50ms",
    "cnt_100ms",
    "cnt_500ms",
    "cnt_1s",
    "cnt_5s",
    "cnt_10s",
    "cnt_INFs"
)

GLOBAL_STATS = [
    'client_connections_aborted',
    'client_connections_connected',
    'client_connections_created',
    'client_connections_non_idle',
    'proxysql_uptime',
    'questions',
    'slow_queries'
]

CONNECTION_POOL_STATS = [
    'status',
    'connused',
    'connfree',
    'connok',
    'connerr',
    'queries',
    'bytes_data_sent',
    'bytes_data_recv',
    'latency_us'
]

ORDER = [
    'connections',
    'active_transactions',
    'questions',
    'pool_overall_net',
    'commands_count',
    'commands_duration',
    'pool_status',
    'pool_net',
    'pool_queries',
    'pool_latency',
    'pool_connection_used',
    'pool_connection_free',
    'pool_connection_ok',
    'pool_connection_error'
]

HISTOGRAM_ORDER = [
    '100us',
    '500us',
    '1ms',
    '5ms',
    '10ms',
    '50ms',
    '100ms',
    '500ms',
    '1s',
    '5s',
    '10s',
    'inf'
]

STATUS = {
    "ONLINE": 1,
    "SHUNNED": 2,
    "OFFLINE_SOFT": 3,
    "OFFLINE_HARD": 4
}

CHARTS = {
    'pool_status': {
        'options': [None, 'ProxySQL Backend Status', 'status', 'status', 'proxysql.pool_status', 'line'],
        'lines': []
    },
    'pool_net': {
        'options': [None, 'ProxySQL Backend Bandwidth', 'kilobits/s', 'bandwidth', 'proxysql.pool_net', 'area'],
        'lines': []
    },
    'pool_overall_net': {
        'options': [None, 'ProxySQL Backend Overall Bandwidth', 'kilobits/s', 'overall_bandwidth',
                    'proxysql.pool_overall_net', 'area'],
        'lines': [
            ['bytes_data_recv', 'in', 'incremental', 8, 1024],
            ['bytes_data_sent', 'out', 'incremental', -8, 1024]
        ]
    },
    'questions': {
        'options': [None, 'ProxySQL Frontend Questions', 'questions/s', 'questions', 'proxysql.questions', 'line'],
        'lines': [
            ['questions', 'questions', 'incremental'],
            ['slow_queries', 'slow_queries', 'incremental']
        ]
    },
    'pool_queries': {
        'options': [None, 'ProxySQL Backend Queries', 'queries/s', 'queries', 'proxysql.queries', 'line'],
        'lines': []
    },
    'active_transactions': {
        'options': [None, 'ProxySQL Frontend Active Transactions', 'transactions/s', 'active_transactions',
                    'proxysql.active_transactions', 'line'],
        'lines': [
            ['active_transactions', 'active_transactions', 'absolute']
        ]
    },
    'pool_latency': {
        'options': [None, 'ProxySQL Backend Latency', 'ms', 'latency', 'proxysql.latency', 'line'],
        'lines': []
    },
    'connections': {
        'options': [None, 'ProxySQL Frontend Connections', 'connections/s', 'connections', 'proxysql.connections',
                    'line'],
        'lines': [
            ['client_connections_connected', 'connected', 'absolute'],
            ['client_connections_created', 'created', 'incremental'],
            ['client_connections_aborted', 'aborted', 'incremental'],
            ['client_connections_non_idle', 'non_idle', 'absolute']
        ]
    },
    'pool_connection_used': {
        'options': [None, 'ProxySQL Used Connections', 'connections', 'pool_connections',
                    'proxysql.pool_used_connections', 'line'],
        'lines': []
    },
    'pool_connection_free': {
        'options': [None, 'ProxySQL Free Connections', 'connections', 'pool_connections',
                    'proxysql.pool_free_connections', 'line'],
        'lines': []
    },
    'pool_connection_ok': {
        'options': [None, 'ProxySQL Established Connections', 'connections', 'pool_connections',
                    'proxysql.pool_ok_connections', 'line'],
        'lines': []
    },
    'pool_connection_error': {
        'options': [None, 'ProxySQL Error Connections', 'connections', 'pool_connections',
                    'proxysql.pool_error_connections', 'line'],
        'lines': []
    },
    'commands_count': {
        'options': [None, 'ProxySQL Commands', 'commands', 'commands', 'proxysql.commands_count', 'line'],
        'lines': []
    },
    'commands_duration': {
        'options': [None, 'ProxySQL Commands Duration', 'ms', 'commands', 'proxysql.commands_duration', 'line'],
        'lines': []
    }
}


class Service(MySQLService):
    def __init__(self, configuration=None, name=None):
        MySQLService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.queries = dict(
            global_status=QUERY_GLOBAL,
            connection_pool_status=QUERY_CONNECTION_POOL,
            commands_status=QUERY_COMMANDS
        )

    def _get_data(self):
        raw_data = self._get_raw_data(description=True)

        if not raw_data:
            return None

        to_netdata = dict()

        if 'global_status' in raw_data:
            global_status = dict(raw_data['global_status'][0])
            for key in global_status:
                if key.lower() in GLOBAL_STATS:
                    to_netdata[key.lower()] = global_status[key]

        if 'connection_pool_status' in raw_data:

            to_netdata['bytes_data_recv'] = 0
            to_netdata['bytes_data_sent'] = 0

            for record in raw_data['connection_pool_status'][0]:
                backend = self.generate_backend(record)
                name = self.generate_backend_name(backend)

                for key in backend:
                    if key in CONNECTION_POOL_STATS:
                        if key == 'status':
                            backend[key] = self.convert_status(backend[key])

                            if len(self.charts) > 0:
                                if (name + '_status') not in self.charts['pool_status']:
                                    self.add_backend_dimensions(name)

                        to_netdata["{0}_{1}".format(name, key)] = backend[key]

                    if key == 'bytes_data_recv':
                        to_netdata['bytes_data_recv'] += int(backend[key])

                    if key == 'bytes_data_sent':
                        to_netdata['bytes_data_sent'] += int(backend[key])

        if 'commands_status' in raw_data:
            for record in raw_data['commands_status'][0]:
                cmd = self.generate_command_stats(record)
                name = cmd['name']

                if len(self.charts) > 0:
                    if (name + '_count') not in self.charts['commands_count']:
                        self.add_command_dimensions(name)
                        self.add_histogram_chart(cmd)

                    to_netdata[name + '_count'] = cmd['count']
                    to_netdata[name + '_duration'] = cmd['duration']
                    for histogram in cmd['histogram']:
                        dimId = 'commands_histogram_{0}_{1}'.format(name, histogram)
                        to_netdata[dimId] = cmd['histogram'][histogram]

        return to_netdata or None

    def add_backend_dimensions(self, name):
        self.charts['pool_status'].add_dimension([name + '_status', name, 'absolute'])
        self.charts['pool_net'].add_dimension([name + '_bytes_data_recv', 'from_' + name, 'incremental', 8, 1024])
        self.charts['pool_net'].add_dimension([name + '_bytes_data_sent', 'to_' + name, 'incremental', -8, 1024])
        self.charts['pool_queries'].add_dimension([name + '_queries', name, 'incremental'])
        self.charts['pool_latency'].add_dimension([name + '_latency_us', name, 'absolute', 1, 1000])
        self.charts['pool_connection_used'].add_dimension([name + '_connused', name, 'absolute'])
        self.charts['pool_connection_free'].add_dimension([name + '_connfree', name, 'absolute'])
        self.charts['pool_connection_ok'].add_dimension([name + '_connok', name, 'incremental'])
        self.charts['pool_connection_error'].add_dimension([name + '_connerr', name, 'incremental'])

    def add_command_dimensions(self, cmd):
        self.charts['commands_count'].add_dimension([cmd + '_count', cmd, 'incremental'])
        self.charts['commands_duration'].add_dimension([cmd + '_duration', cmd, 'incremental', 1, 1000])

    def add_histogram_chart(self, cmd):
        chart = self.charts.add_chart(self.histogram_chart(cmd))

        for histogram in HISTOGRAM_ORDER:
            dimId = 'commands_histogram_{0}_{1}'.format(cmd['name'], histogram)
            chart.add_dimension([dimId, histogram, 'incremental'])

    @staticmethod
    def histogram_chart(cmd):
        return [
            'commands_historgram_' + cmd['name'],
            None,
            'ProxySQL {0} Command Histogram'.format(cmd['name'].title()),
            'commands',
            'commands_histogram',
            'proxysql.commands_histogram_' + cmd['name'],
            'stacked'
        ]

    @staticmethod
    def generate_backend(data):
        return {
            'hostgroup': data[0],
            'srv_host': data[1],
            'srv_port': data[2],
            'status': data[3],
            'connused': data[4],
            'connfree': data[5],
            'connok': data[6],
            'connerr': data[7],
            'queries': data[8],
            'bytes_data_sent': data[9],
            'bytes_data_recv': data[10],
            'latency_us': data[11]
        }

    @staticmethod
    def generate_command_stats(data):
        return {
            'name': data[0].lower(),
            'duration': data[1],
            'count': data[2],
            'histogram': {
                '100us': data[3],
                '500us': data[4],
                '1ms': data[5],
                '5ms': data[6],
                '10ms': data[7],
                '50ms': data[8],
                '100ms': data[9],
                '500ms': data[10],
                '1s': data[11],
                '5s': data[12],
                '10s': data[13],
                'inf': data[14]
            }
        }

    @staticmethod
    def generate_backend_name(backend):
        hostgroup = backend['hostgroup'].replace(' ', '_').lower()
        host = backend['srv_host'].replace('.', '_')

        return "{0}_{1}_{2}".format(hostgroup, host, backend['srv_port'])

    @staticmethod
    def convert_status(status):
        if status in STATUS:
            return STATUS[status]
        return -1
