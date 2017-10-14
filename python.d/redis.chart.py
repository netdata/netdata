# -*- coding: utf-8 -*-
# Description: redis netdata python.d module
# Author: Pawel Krupa (paulfantom)

from bases.FrameworkServices.SocketService import SocketService

# default module values (can be overridden per job in `config`)
priority = 60000
retries = 60

# default job configuration (overridden by python.d.plugin)
# config = {'local': {
#             'update_every': update_every,
#             'retries': retries,
#             'priority': priority,
#             'host': 'localhost',
#             'port': 6379,
#             'unix_socket': None
#          }}

ORDER = ['operations', 'hit_rate', 'memory', 'keys', 'net', 'connections', 'clients', 'slaves', 'persistence',
         'bgsave_now', 'bgsave_health']

CHARTS = {
    'operations': {
        'options': [None, 'Redis Operations', 'operations/s', 'operations', 'redis.operations', 'line'],
        'lines': [
            ['total_commands_processed', 'commands', 'incremental'],
            ['instantaneous_ops_per_sec', 'operations', 'absolute']
        ]},
    'hit_rate': {
        'options': [None, 'Redis Hit rate', 'percent', 'hits', 'redis.hit_rate', 'line'],
        'lines': [
            ['hit_rate', 'rate', 'absolute']
        ]},
    'memory': {
        'options': [None, 'Redis Memory utilization', 'kilobytes', 'memory', 'redis.memory', 'line'],
        'lines': [
            ['used_memory', 'total', 'absolute', 1, 1024],
            ['used_memory_lua', 'lua', 'absolute', 1, 1024]
        ]},
    'net': {
        'options': [None, 'Redis Bandwidth', 'kilobits/s', 'network', 'redis.net', 'area'],
        'lines': [
            ['total_net_input_bytes', 'in', 'incremental', 8, 1024],
            ['total_net_output_bytes', 'out', 'incremental', -8, 1024]
        ]},
    'keys': {
        'options': [None, 'Redis Keys per Database', 'keys', 'keys', 'redis.keys', 'line'],
        'lines': [
            # lines are created dynamically in `check()` method
        ]},
    'connections': {
        'options': [None, 'Redis Connections', 'connections/s', 'connections', 'redis.connections', 'line'],
        'lines': [
            ['total_connections_received', 'received', 'incremental', 1],
            ['rejected_connections', 'rejected', 'incremental', -1]
        ]},
    'clients': {
        'options': [None, 'Redis Clients', 'clients', 'connections', 'redis.clients', 'line'],
        'lines': [
            ['connected_clients', 'connected', 'absolute', 1],
            ['blocked_clients', 'blocked', 'absolute', -1]
        ]},
    'slaves': {
        'options': [None, 'Redis Slaves', 'slaves', 'replication', 'redis.slaves', 'line'],
        'lines': [
            ['connected_slaves', 'connected', 'absolute']
        ]},
    'persistence': {
        'options': [None, 'Redis Persistence Changes Since Last Save', 'changes', 'persistence',
                    'redis.rdb_changes', 'line'],
        'lines': [
            ['rdb_changes_since_last_save', 'changes', 'absolute']
        ]},
    'bgsave_now': {
        'options': [None, 'Duration of the RDB Save Operation', 'seconds', 'persistence',
                    'redis.bgsave_now', 'absolute'],
        'lines': [
            ['rdb_bgsave_in_progress', 'rdb save', 'absolute']
        ]},
    'bgsave_health': {
        'options': [None, 'Status of the Last RDB Save Operation', 'status', 'persistence',
                    'redis.bgsave_health', 'line'],
        'lines': [
            ['rdb_last_bgsave_status', 'rdb save', 'absolute']
        ]}
}


class Service(SocketService):
    def __init__(self, configuration=None, name=None):
        SocketService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self._keep_alive = True
        self.chart_name = ""
        self.host = self.configuration.get('host', 'localhost')
        self.port = self.configuration.get('port', 6379)
        self.unix_socket = self.configuration.get('socket')
        password = self.configuration.get('pass', str())
        self.bgsave_time = 0
        self.requests = dict(request='INFO\r\n'.encode(),
                             password=' '.join(['AUTH', password, '\r\n']).encode() if password else None)
        self.request = self.requests['request']

    def _get_data(self):
        """
        Get data from socket
        :return: dict
        """
        if self.requests['password']:
            self.request = self.requests['password']
            raw = self._get_raw_data().strip()
            if raw != "+OK":
                self.error("invalid password")
                return None
            self.request = self.requests['request']
        response = self._get_raw_data()
        if response is None:
            # error has already been logged
            return None

        try:
            parsed = response.split("\n")
        except AttributeError:
            self.error("response is invalid/empty")
            return None

        data = dict()
        for line in parsed:
            if len(line) < 5 or line[0] == '$' or line[0] == '#':
                continue

            if line.startswith('db'):
                tmp = line.split(',')[0].replace('keys=', '')
                record = tmp.split(':')
                data[record[0]] = record[1]
                continue

            try:
                t = line.split(':')
                data[t[0]] = t[1]
            except (IndexError, ValueError):
                self.debug("invalid line received: " + str(line))
                pass

        if not data:
            self.error("received data doesn't have any records")
            return None

        try:
            data['hit_rate'] = (int(data['keyspace_hits']) * 100) / (int(data['keyspace_hits'])
                                                                     + int(data['keyspace_misses']))
        except (KeyError, ZeroDivisionError, TypeError):
            data['hit_rate'] = 0

        if data['rdb_bgsave_in_progress'] != '0\r':
            self.bgsave_time += self.update_every
        else:
            self.bgsave_time = 0

        data['rdb_last_bgsave_status'] = 0 if data['rdb_last_bgsave_status'] == 'ok\r' else 1
        data['rdb_bgsave_in_progress'] = self.bgsave_time

        return data

    def _check_raw_data(self, data):
        """
        Check if all data has been gathered from socket.
        Parse first line containing message length and check against received message
        :param data: str
        :return: boolean
        """
        length = len(data)
        supposed = data.split('\n')[0][1:-1]
        offset = len(supposed) + 4  # 1 dollar sing, 1 new line character + 1 ending sequence '\r\n'
        if not supposed.isdigit():
            return True
        supposed = int(supposed)

        if length - offset >= supposed:
            self.debug("received full response from redis")
            return True

        self.debug("waiting more data from redis")
        return False

    def check(self):
        """
        Parse configuration, check if redis is available, and dynamically create chart lines data
        :return: boolean
        """
        data = self._get_data()
        if data is None:
            return False

        for name in data:
            if name.startswith('db'):
                self.definitions['keys']['lines'].append([name, None, 'absolute'])
        return True
