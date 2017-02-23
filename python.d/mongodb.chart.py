# -*- coding: utf-8 -*-
# Description: mongodb netdata python.d module
# Author: l2isbad

from base import SimpleService
from copy import deepcopy
from datetime import datetime
try:
    from pymongo import MongoClient
    from pymongo.errors import PyMongoError
    PYMONGO = True
except ImportError:
    PYMONGO = False

# default module values (can be overridden per job in `config`)
# update_every = 2
priority = 60000
retries = 60

REPLSET_STATES = [
        ('1', 'primary'),
        ('8', 'down'),
        ('2', 'secondary'),
        ('3', 'recovering'),
        ('5', 'startup2'),
        ('4', 'fatal'),
        ('7', 'arbiter'),
        ('6', 'unknown'),
        ('9', 'rollback'),
        ('10', 'removed'),
        ('0', 'startup')]

# charts order (can be overridden if you want less charts, or different order)
ORDER = ['read_operations', 'write_operations', 'active_clients', 'journaling_transactions',
         'journaling_volume', 'background_flush_average', 'background_flush_last', 'background_flush_rate',
         'wiredtiger_read', 'wiredtiger_write', 'cursors', 'connections', 'memory', 'page_faults',
         'queued_requests', 'record_moves', 'wiredtiger_cache', 'wiredtiger_pages_evicted', 'asserts',
         'dbstats_objects', 'tcmalloc_generic', 'tcmalloc_metrics', 'command_total_rate', 'command_failed_rate']

CHARTS = {
    'read_operations': {
        'options': [None, 'Received read requests', 'requests/s', 'throughput metrics',
                    'mongodb.read_operations', 'line'],
        'lines': [
            ['readWriteOper_query', 'query', 'incremental'],
            ['readWriteOper_getmore', 'getmore', 'incremental']
        ]},
    'write_operations': {
        'options': [None, 'Received write requests', 'requests/s', 'throughput metrics',
                    'mongodb.write_operations', 'line'],
        'lines': [
            ['readWriteOper_insert', 'insert', 'incremental'],
            ['readWriteOper_update', 'update', 'incremental'],
            ['readWriteOper_delete', 'delete', 'incremental']
        ]},
    'active_clients': {
        'options': [None, 'Clients with read or write operations in progress or queued', 'clients',
                    'throughput metrics', 'mongodb.active_clients', 'line'],
        'lines': [
            ['activeClients_readers', 'readers', 'absolute'],
            ['activeClients_writers', 'writers', 'absolute']
            ]},
    'journaling_transactions': {
        'options': [None, 'Transactions that have been written to the journal', 'commits',
                    'database performance', 'mongodb.journaling_transactions', 'line'],
        'lines': [
            ['journalTrans_commits', 'commits', 'absolute']
            ]},
    'journaling_volume': {
        'options': [None, 'Volume of data written to the journal', 'MB', 'database performance',
                    'mongodb.journaling_volume', 'line'],
        'lines': [
            ['journalTrans_journaled', 'volume', 'absolute', 1, 100]
            ]},
    'background_flush_average': {
        'options': [None, 'Average time taken by flushes to execute', 'ms', 'database performance',
                    'mongodb.background_flush_average', 'line'],
        'lines': [
            ['background_flush_average', 'time', 'absolute', 1, 100]
            ]},
    'background_flush_last': {
        'options': [None, 'Time taken by the last flush operation to execute', 'ms', 'database performance',
                    'mongodb.background_flush_last', 'line'],
        'lines': [
            ['background_flush_last', 'time', 'absolute', 1, 100]
            ]},
    'background_flush_rate': {
        'options': [None, 'Flushes rate', 'flushes', 'database performance', 'mongodb.background_flush_rate', 'line'],
        'lines': [
            ['background_flush_rate', 'flushes', 'incremental', 1, 1]
            ]},
    'wiredtiger_read': {
        'options': [None, 'Read tickets in use and remaining', 'tickets', 'database performance',
                    'mongodb.wiredtiger_read', 'stacked'],
        'lines': [
            ['wiredTigerRead_available', 'available', 'absolute', 1, 1],
            ['wiredTigerRead_out', 'inuse', 'absolute', 1, 1]
            ]},
    'wiredtiger_write': {
        'options': [None, 'Write tickets in use and remaining', 'tickets', 'database performance',
                    'mongodb.wiredtiger_write', 'stacked'],
        'lines': [
            ['wiredTigerWrite_available', 'available', 'absolute', 1, 1],
            ['wiredTigerWrite_out', 'inuse', 'absolute', 1, 1]
            ]},
    'cursors': {
        'options': [None, 'Currently openned cursors, cursors with timeout disabled and timed out cursors',
                    'cursors', 'database performance', 'mongodb.cursors', 'stacked'],
        'lines': [
            ['cursor_total', 'openned', 'absolute', 1, 1],
            ['cursor_noTimeout', 'notimeout', 'absolute', 1, 1],
            ['cursor_timedOut', 'timedout', 'incremental', 1, 1]
            ]},
    'connections': {
        'options': [None, 'Currently connected clients and unused connections', 'connections',
                    'resource utilization', 'mongodb.connections', 'stacked'],
        'lines': [
            ['connections_available', 'unused', 'absolute', 1, 1],
            ['connections_current', 'connected', 'absolute', 1, 1]
            ]},
    'memory': {
        'options': [None, 'Memory metrics', 'MB', 'resource utilization', 'mongodb.memory', 'stacked'],
        'lines': [
            ['memory_virtual', 'virtual', 'absolute', 1, 1],
            ['memory_resident', 'resident', 'absolute', 1, 1],
            ['memory_mapped', 'mapped', 'absolute', 1, 1]
            ]},
    'page_faults': {
        'options': [None, 'Number of times MongoDB had to fetch data from disk', 'request/s',
                    'resource utilization', 'mongodb.page_faults', 'line'],
        'lines': [
            ['page_faults', 'page_faults', 'incremental', 1, 1]
            ]},
    'queued_requests': {
        'options': [None, 'Currently queued read and wrire requests', 'requests', 'resource saturation',
                    'mongodb.queued_requests', 'line'],
        'lines': [
            ['currentQueue_readers', 'readers', 'absolute', 1, 1],
            ['currentQueue_writers', 'writers', 'absolute', 1, 1]
            ]},
    'record_moves': {
        'options': [None, 'Number of times documents had to be moved on-disk', 'number',
                    'resource saturation', 'mongodb.record_moves', 'line'],
        'lines': [
            ['record_moves', 'moves', 'incremental', 1, 1]
            ]},
    'asserts': {
        'options': [None, 'Number of message, warning, regular, corresponding to errors generated'
                          ' by users assertions raised', 'number', 'errors (asserts)', 'mongodb.asserts', 'line'],
        'lines': [
            ['errors_msg', 'msg', 'incremental', 1, 1],
            ['errors_warning', 'warning', 'incremental', 1, 1],
            ['errors_regular', 'regular', 'incremental', 1, 1],
            ['errors_user', 'user', 'incremental', 1, 1]
            ]},
    'wiredtiger_cache': {
        'options': [None, 'Amount of space taken by cached data and by dirty data in the cache',
                    'KB', 'resource utilization', 'mongodb.wiredtiger_cache', 'stacked'],
        'lines': [
            ['wiredTiger_bytes_in_cache', 'cached', 'absolute', 1, 1024],
            ['wiredTiger_dirty_in_cache', 'dirty', 'absolute', 1, 1024]
            ]},
    'wiredtiger_pages_evicted': {
        'options': [None, 'Pages evicted from the cache',
                    'pages', 'resource utilization', 'mongodb.wiredtiger_pages_evicted', 'stacked'],
        'lines': [
            ['wiredTiger_unmodified_pages_evicted', 'unmodified', 'absolute', 1, 1],
            ['wiredTiger_modified_pages_evicted', 'modified', 'absolute', 1, 1]
            ]},
    'dbstats_objects': {
        'options': [None, 'Number of documents in the database among all the collections', 'documents',
                    'storage size metrics', 'mongodb.dbstats_objects', 'stacked'],
        'lines': [
            ]},
    'tcmalloc_generic': {
        'options': [None, 'Tcmalloc generic metrics', 'MB', 'tcmalloc', 'mongodb.tcmalloc_generic', 'stacked'],
        'lines': [
            ['current_allocated_bytes', 'allocated', 'absolute', 1, 1048576],
            ['heap_size', 'heap_size', 'absolute', 1, 1048576]
            ]},
    'tcmalloc_metrics': {
        'options': [None, 'Tcmalloc metrics', 'KB', 'tcmalloc', 'mongodb.tcmalloc_metrics', 'stacked'],
        'lines': [
            ['central_cache_free_bytes', 'central_cache_free', 'absolute', 1, 1024],
            ['current_total_thread_cache_bytes', 'current_total_thread_cache', 'absolute', 1, 1024],
            ['pageheap_free_bytes', 'pageheap_free', 'absolute', 1, 1024],
            ['pageheap_unmapped_bytes', 'pageheap_unmapped', 'absolute', 1, 1024],
            ['thread_cache_free_bytes', 'thread_cache_free', 'absolute', 1, 1024],
            ['transfer_cache_free_bytes', 'transfer_cache_free', 'absolute', 1, 1024]
            ]},
    'command_total_rate': {
        'options': [None, 'Commands total rate', 'commands/s', 'commands', 'mongodb.command_total_rate', 'stacked'],
        'lines': [
            ['count_total', 'count', 'incremental', 1, 1],
            ['createIndexes_total', 'createIndexes', 'incremental', 1, 1],
            ['delete_total', 'delete', 'incremental', 1, 1],
            ['eval_total', 'eval', 'incremental', 1, 1],
            ['findAndModify_total', 'findAndModify', 'incremental', 1, 1],
            ['insert_total', 'insert', 'incremental', 1, 1],
            ['update_total', 'update', 'incremental', 1, 1]
            ]},
    'command_failed_rate': {
        'options': [None, 'Commands failed rate', 'commands/s', 'commands', 'mongodb.command_failed_rate', 'stacked'],
        'lines': [
            ['count_failed', 'count', 'incremental', 1, 1],
            ['createIndexes_failed', 'createIndexes', 'incremental', 1, 1],
            ['delete_dailed', 'delete', 'incremental', 1, 1],
            ['eval_failed', 'eval', 'incremental', 1, 1],
            ['findAndModify_failed', 'findAndModify', 'incremental', 1, 1],
            ['insert_failed', 'insert', 'incremental', 1, 1],
            ['update_failed', 'update', 'incremental', 1, 1]
            ]}
}


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.user = self.configuration.get('user')
        self.password = self.configuration.get('pass')
        self.host = self.configuration.get('host', '127.0.0.1')
        self.port = self.configuration.get('port', 27017)
        self.timeout = self.configuration.get('timeout', 100)

    def check(self):
        if not PYMONGO:
            self.error('Pymongo module is needed to use mongodb.chart.py')
            return False
        self.connection, server_status, error = self._create_connection()
        if error:
            self.error(error)
            return False

        self.repl = 'repl' in server_status
        try:
            self.databases = self.connection.database_names()
        except PyMongoError as error:
            self.databases = list()
            self.info('Can\'t collect databases: %s' % str(error))

        self.create_charts_(server_status)

        return True

    def create_charts_(self, server_status):

        self.order = ORDER[:]
        self.definitions = deepcopy(CHARTS)
        self.ss = dict()

        for elem in ['dur', 'backgroundFlushing', 'wiredTiger', 'tcmalloc', 'cursor', 'commands']:
            self.ss[elem] = in_server_status(elem, server_status)

        if not self.ss['dur']:
            self.order.remove('journaling_transactions')
            self.order.remove('journaling_volume')

        if not self.ss['backgroundFlushing']:
            self.order.remove('background_flush_average')
            self.order.remove('background_flush_last')
            self.order.remove('background_flush_rate')

        if not self.ss['cursor']:
            self.order.remove('cursors')

        if not self.ss['wiredTiger']:
            self.order.remove('wiredtiger_write')
            self.order.remove('wiredtiger_read')
            self.order.remove('wiredtiger_cache')

        if not self.ss['tcmalloc']:
            self.order.remove('tcmalloc_generic')
            self.order.remove('tcmalloc_metrics')

        if not self.ss['commands']:
            self.order.remove('command_total_rate')
            self.order.remove('command_failed_rate')

        for dbase in self.databases:
            self.order.append('_'.join([dbase, 'dbstats']))
            self.definitions['_'.join([dbase, 'dbstats'])] = {
                    'options': [None, '%s: size of all documents, indexes, extents' % dbase, 'KB',
                                'storage size metrics', 'mongodb.dbstats', 'line'],
                    'lines': [
                             ['_'.join([dbase, 'dataSize']), 'documents', 'absolute', 1, 1024],
                             ['_'.join([dbase, 'indexSize']), 'indexes', 'absolute', 1, 1024],
                             ['_'.join([dbase, 'storageSize']), 'extents', 'absolute', 1, 1024]
                      ]}
            self.definitions['dbstats_objects']['lines'].append(['_'.join([dbase, 'objects']), dbase, 'absolute'])

        if server_status.get('repl'):
            def create_heartbeat_lines(hosts):
                lines = list()
                for host in hosts:
                    dim_id = '_'.join([host, 'heartbeat_lag'])
                    lines.append([dim_id, host, 'absolute', 1, 1000])
                return lines

            def create_state_lines(states):
                lines = list()
                for state, description in states:
                    dim_id = '_'.join([host, 'state', state])
                    lines.append([dim_id, description, 'absolute', 1, 1])
                return lines

            all_hosts = server_status['repl']['hosts']
            this_host = server_status['repl']['me']
            other_hosts = [host for host in all_hosts if host != this_host]

            # Create "heartbeat delay" charts
            self.order.append('heartbeat_delay')
            self.definitions['heartbeat_delay'] = {
                       'options': [None, 'Latency between this node and replica set members (lastHeartbeatRecv)',
                                   'seconds', 'replication', 'mongodb.replication_heartbeat_delay', 'stacked'],
                       'lines': create_heartbeat_lines(other_hosts)}
            # Create "replica set members state" chart
            for host in all_hosts:
                chart_name = '_'.join([host, 'state'])
                self.order.append(chart_name)
                self.definitions[chart_name] = {
                       'options': [None, '%s state' % host, 'state',
                                   'replication', 'mongodb.replication_state', 'line'],
                       'lines': create_state_lines(REPLSET_STATES)}

    def _get_raw_data(self):
        raw_data = dict()

        raw_data.update(self.get_serverstatus_() or dict())
        raw_data.update(self.get_dbstats_() or dict())
        raw_data.update(self.get_replsetgetstatus_() or dict())

        return raw_data or None

    def get_serverstatus_(self):
        raw_data = dict()
        try:
            raw_data['serverStatus'] = self.connection.admin.command('serverStatus')
        except PyMongoError:
            return None
        else:
            return raw_data

    def get_dbstats_(self):
        if not self.databases:
            return None

        raw_data = dict()
        raw_data['dbStats'] = dict()
        try:
            for dbase in self.databases:
                raw_data['dbStats'][dbase] = self.connection[dbase].command('dbStats')
        except PyMongoError:
            return None
        else:
            return raw_data

    def get_replsetgetstatus_(self):
        if not self.repl:
            return None

        raw_data = dict()
        try:
            raw_data['replSetGetStatus'] = self.connection.admin.command('replSetGetStatus')
        except PyMongoError:
            return None
        else:
            return raw_data

    def _get_data(self):
        """
        :return: dict
        """
        raw_data = self._get_raw_data()

        if not raw_data:
            return None

        to_netdata = dict()
        serverStatus = raw_data['serverStatus']
        dbStats = raw_data.get('dbStats')
        replSetGetStatus = raw_data.get('replSetGetStatus')
        utc_now = datetime.utcnow()

        # serverStatus
        to_netdata.update(update_dict_key(serverStatus['opcounters'], 'readWriteOper'))
        to_netdata.update(update_dict_key(serverStatus['globalLock']['activeClients'], 'activeClients'))
        to_netdata.update(update_dict_key(serverStatus['connections'], 'connections'))
        to_netdata.update(update_dict_key(serverStatus['mem'], 'memory'))
        to_netdata.update(update_dict_key(serverStatus['globalLock']['currentQueue'], 'currentQueue'))
        to_netdata.update(update_dict_key(serverStatus['asserts'], 'errors'))
        to_netdata['page_faults'] = serverStatus['extra_info']['page_faults']
        to_netdata['record_moves'] = serverStatus['metrics']['record']['moves']

        if self.ss['dur']:
            to_netdata['journalTrans_commits'] = serverStatus['dur']['commits']
            to_netdata['journalTrans_journaled'] = int(serverStatus['dur']['journaledMB'] * 100)

        if self.ss['backgroundFlushing']:
            to_netdata['background_flush_average'] = int(serverStatus['backgroundFlushing']['average_ms'] * 100)
            to_netdata['background_flush_last'] = int(serverStatus['backgroundFlushing']['last_ms'] * 100)
            to_netdata['background_flush_rate'] = serverStatus['backgroundFlushing']['flushes']

        if self.ss['cursor']:
            to_netdata['cursor_timedOut'] = serverStatus['metrics']['cursor']['timedOut']
            to_netdata.update(update_dict_key(serverStatus['metrics']['cursor']['open'], 'cursor'))

        if self.ss['wiredTiger']:
            wired_tiger = serverStatus['wiredTiger']
            to_netdata.update(update_dict_key(serverStatus['wiredTiger']['concurrentTransactions']['read'],
                                              'wiredTigerRead'))
            to_netdata.update(update_dict_key(serverStatus['wiredTiger']['concurrentTransactions']['write'],
                                              'wiredTigerWrite'))
            to_netdata['wiredTiger_bytes_in_cache'] = wired_tiger['cache']['bytes currently in the cache']
            to_netdata['wiredTiger_dirty_in_cache'] = wired_tiger['cache']['tracked dirty bytes in the cache']
            to_netdata['wiredTiger_unmodified_pages_evicted'] = wired_tiger['cache']['unmodified pages evicted']
            to_netdata['wiredTiger_modified_pages_evicted'] = wired_tiger['cache']['modified pages evicted']

        if self.ss['tcmalloc']:
            to_netdata.update(serverStatus['tcmalloc']['generic'])
            to_netdata.update(dict([(k, v) for k, v in serverStatus['tcmalloc']['tcmalloc'].items()
                                    if int_or_float(v)]))

        if self.ss['commands']:
            for elem in ['count', 'createIndexes', 'delete', 'eval', 'findAndModify', 'insert', 'update']:
                to_netdata.update(update_dict_key(serverStatus['metrics']['commands'][elem], elem))

        # dbStats
        if dbStats:
            for dbase in dbStats:
                to_netdata.update(update_dict_key(dbStats[dbase], dbase))

        # replSetGetStatus
        if replSetGetStatus:
            other_hosts = list()
            members = replSetGetStatus['members']
            for member in members:
                if not member.get('self'):
                    other_hosts.append(member)
                # Replica set members state
                for elem in REPLSET_STATES:
                    state = elem[0]
                    to_netdata.update({'_'.join([member['name'], 'state', state]): 0})
                to_netdata.update({'_'.join([member['name'], 'state', str(member['state'])]): member['state']})
            # Heartbeat lag calculation
            for other in other_hosts:
                if other['lastHeartbeatRecv'] != datetime(1970, 1, 1, 0, 0):
                    node = other['name'] + '_heartbeat_lag'
                    to_netdata[node] = int(lag_calculation(utc_now - other['lastHeartbeatRecv']) * 1000)

        return to_netdata

    def _create_connection(self):
        conn_vars = {'host': self.host, 'port': self.port}
        if hasattr(MongoClient, 'server_selection_timeout'):
            conn_vars.update({'serverselectiontimeoutms': self.timeout})
        try:
            connection = MongoClient(**conn_vars)
            if self.user and self.password:
                connection.admin.authenticate(name=self.user, password=self.password)
            # elif self.user:
            #     connection.admin.authenticate(name=self.user, mechanism='MONGODB-X509')
            server_status = connection.admin.command('serverStatus')
        except PyMongoError as error:
            return None, None, str(error)
        else:
            return connection, server_status, None


def update_dict_key(collection, string):
    return dict([('_'.join([string, k]), int(round(v))) for k, v in collection.items() if int_or_float(v)])


def int_or_float(value):
    return isinstance(value, (int, float))


def in_server_status(elem, server_status):
    return elem in server_status or elem in server_status['metrics']


def lag_calculation(lag):
    if hasattr(lag, 'total_seconds'):
        return lag.total_seconds()
    else:
        return (lag.microseconds + (lag.seconds + lag.days * 24 * 3600) * 10 ** 6) / 10.0 ** 6
