# -*- coding: utf-8 -*-
# Description: mongodb netdata python.d module
# Author: ilyam8
# SPDX-License-Identifier: GPL-3.0-or-later

import ssl

from copy import deepcopy
from datetime import datetime
from sys import exc_info

try:
    from pymongo import MongoClient, ASCENDING, DESCENDING
    from pymongo.errors import PyMongoError

    PYMONGO = True
except ImportError:
    PYMONGO = False

from bases.FrameworkServices.SimpleService import SimpleService

REPL_SET_STATES = [
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
    ('0', 'startup')
]


def multiply_by_100(value):
    return value * 100


DEFAULT_METRICS = [
    ('opcounters.delete', None, None),
    ('opcounters.update', None, None),
    ('opcounters.insert', None, None),
    ('opcounters.query', None, None),
    ('opcounters.getmore', None, None),
    ('globalLock.activeClients.readers', 'activeClients_readers', None),
    ('globalLock.activeClients.writers', 'activeClients_writers', None),
    ('connections.available', 'connections_available', None),
    ('connections.current', 'connections_current', None),
    ('mem.mapped', None, None),
    ('mem.resident', None, None),
    ('mem.virtual', None, None),
    ('globalLock.currentQueue.readers', 'currentQueue_readers', None),
    ('globalLock.currentQueue.writers', 'currentQueue_writers', None),
    ('asserts.msg', None, None),
    ('asserts.regular', None, None),
    ('asserts.user', None, None),
    ('asserts.warning', None, None),
    ('extra_info.page_faults', None, None),
    ('metrics.record.moves', None, None),
    ('backgroundFlushing.average_ms', None, multiply_by_100),
    ('backgroundFlushing.last_ms', None, multiply_by_100),
    ('backgroundFlushing.flushes', None, multiply_by_100),
    ('metrics.cursor.timedOut', None, None),
    ('metrics.cursor.open.total', 'cursor_total', None),
    ('metrics.cursor.open.noTimeout', None, None),
    ('cursors.timedOut', None, None),
    ('cursors.totalOpen', 'cursor_total', None)
]

DUR = [
    ('dur.commits', None, None),
    ('dur.journaledMB', None, multiply_by_100)
]

WIREDTIGER = [
    ('wiredTiger.concurrentTransactions.read.available', 'wiredTigerRead_available', None),
    ('wiredTiger.concurrentTransactions.read.out', 'wiredTigerRead_out', None),
    ('wiredTiger.concurrentTransactions.write.available', 'wiredTigerWrite_available', None),
    ('wiredTiger.concurrentTransactions.write.out', 'wiredTigerWrite_out', None),
    ('wiredTiger.cache.bytes currently in the cache', None, None),
    ('wiredTiger.cache.tracked dirty bytes in the cache', None, None),
    ('wiredTiger.cache.maximum bytes configured', None, None),
    ('wiredTiger.cache.unmodified pages evicted', 'unmodified', None),
    ('wiredTiger.cache.modified pages evicted', 'modified', None)
]

TCMALLOC = [
    ('tcmalloc.generic.current_allocated_bytes', None, None),
    ('tcmalloc.generic.heap_size', None, None),
    ('tcmalloc.tcmalloc.central_cache_free_bytes', None, None),
    ('tcmalloc.tcmalloc.current_total_thread_cache_bytes', None, None),
    ('tcmalloc.tcmalloc.pageheap_free_bytes', None, None),
    ('tcmalloc.tcmalloc.pageheap_unmapped_bytes', None, None),
    ('tcmalloc.tcmalloc.thread_cache_free_bytes', None, None),
    ('tcmalloc.tcmalloc.transfer_cache_free_bytes', None, None)
]

COMMANDS = [
    ('metrics.commands.count.total', 'count_total', None),
    ('metrics.commands.createIndexes.total', 'createIndexes_total', None),
    ('metrics.commands.delete.total', 'delete_total', None),
    ('metrics.commands.eval.total', 'eval_total', None),
    ('metrics.commands.findAndModify.total', 'findAndModify_total', None),
    ('metrics.commands.insert.total', 'insert_total', None),
    ('metrics.commands.delete.total', 'delete_total', None),
    ('metrics.commands.count.failed', 'count_failed', None),
    ('metrics.commands.createIndexes.failed', 'createIndexes_failed', None),
    ('metrics.commands.delete.failed', 'delete_failed', None),
    ('metrics.commands.eval.failed', 'eval_failed', None),
    ('metrics.commands.findAndModify.failed', 'findAndModify_failed', None),
    ('metrics.commands.insert.failed', 'insert_failed', None),
    ('metrics.commands.delete.failed', 'delete_failed', None)
]

LOCKS = [
    ('locks.Collection.acquireCount.R', 'Collection_R', None),
    ('locks.Collection.acquireCount.r', 'Collection_r', None),
    ('locks.Collection.acquireCount.W', 'Collection_W', None),
    ('locks.Collection.acquireCount.w', 'Collection_w', None),
    ('locks.Database.acquireCount.R', 'Database_R', None),
    ('locks.Database.acquireCount.r', 'Database_r', None),
    ('locks.Database.acquireCount.W', 'Database_W', None),
    ('locks.Database.acquireCount.w', 'Database_w', None),
    ('locks.Global.acquireCount.R', 'Global_R', None),
    ('locks.Global.acquireCount.r', 'Global_r', None),
    ('locks.Global.acquireCount.W', 'Global_W', None),
    ('locks.Global.acquireCount.w', 'Global_w', None),
    ('locks.Metadata.acquireCount.R', 'Metadata_R', None),
    ('locks.Metadata.acquireCount.w', 'Metadata_w', None),
    ('locks.oplog.acquireCount.r', 'oplog_r', None),
    ('locks.oplog.acquireCount.w', 'oplog_w', None)
]

DBSTATS = [
    'dataSize',
    'indexSize',
    'storageSize',
    'objects'
]

# charts order (can be overridden if you want less charts, or different order)
ORDER = [
    'read_operations',
    'write_operations',
    'active_clients',
    'journaling_transactions',
    'journaling_volume',
    'background_flush_average',
    'background_flush_last',
    'background_flush_rate',
    'wiredtiger_read',
    'wiredtiger_write',
    'cursors',
    'connections',
    'memory',
    'page_faults',
    'queued_requests',
    'record_moves',
    'wiredtiger_cache',
    'wiredtiger_pages_evicted',
    'asserts',
    'locks_collection',
    'locks_database',
    'locks_global',
    'locks_metadata',
    'locks_oplog',
    'dbstats_objects',
    'tcmalloc_generic',
    'tcmalloc_metrics',
    'command_total_rate',
    'command_failed_rate'
]

CHARTS = {
    'read_operations': {
        'options': [None, 'Received read requests', 'requests/s', 'throughput metrics',
                    'mongodb.read_operations', 'line'],
        'lines': [
            ['query', None, 'incremental'],
            ['getmore', None, 'incremental']
        ]
    },
    'write_operations': {
        'options': [None, 'Received write requests', 'requests/s', 'throughput metrics',
                    'mongodb.write_operations', 'line'],
        'lines': [
            ['insert', None, 'incremental'],
            ['update', None, 'incremental'],
            ['delete', None, 'incremental']
        ]
    },
    'active_clients': {
        'options': [None, 'Clients with read or write operations in progress or queued', 'clients',
                    'throughput metrics', 'mongodb.active_clients', 'line'],
        'lines': [
            ['activeClients_readers', 'readers', 'absolute'],
            ['activeClients_writers', 'writers', 'absolute']
        ]
    },
    'journaling_transactions': {
        'options': [None, 'Transactions that have been written to the journal', 'commits',
                    'database performance', 'mongodb.journaling_transactions', 'line'],
        'lines': [
            ['commits', None, 'absolute']
        ]
    },
    'journaling_volume': {
        'options': [None, 'Volume of data written to the journal', 'MiB', 'database performance',
                    'mongodb.journaling_volume', 'line'],
        'lines': [
            ['journaledMB', 'volume', 'absolute', 1, 100]
        ]
    },
    'background_flush_average': {
        'options': [None, 'Average time taken by flushes to execute', 'milliseconds', 'database performance',
                    'mongodb.background_flush_average', 'line'],
        'lines': [
            ['average_ms', 'time', 'absolute', 1, 100]
        ]
    },
    'background_flush_last': {
        'options': [None, 'Time taken by the last flush operation to execute', 'milliseconds', 'database performance',
                    'mongodb.background_flush_last', 'line'],
        'lines': [
            ['last_ms', 'time', 'absolute', 1, 100]
        ]
    },
    'background_flush_rate': {
        'options': [None, 'Flushes rate', 'flushes', 'database performance', 'mongodb.background_flush_rate', 'line'],
        'lines': [
            ['flushes', 'flushes', 'incremental', 1, 1]
        ]
    },
    'wiredtiger_read': {
        'options': [None, 'Read tickets in use and remaining', 'tickets', 'database performance',
                    'mongodb.wiredtiger_read', 'stacked'],
        'lines': [
            ['wiredTigerRead_available', 'available', 'absolute', 1, 1],
            ['wiredTigerRead_out', 'inuse', 'absolute', 1, 1]
        ]
    },
    'wiredtiger_write': {
        'options': [None, 'Write tickets in use and remaining', 'tickets', 'database performance',
                    'mongodb.wiredtiger_write', 'stacked'],
        'lines': [
            ['wiredTigerWrite_available', 'available', 'absolute', 1, 1],
            ['wiredTigerWrite_out', 'inuse', 'absolute', 1, 1]
        ]
    },
    'cursors': {
        'options': [None, 'Currently openned cursors, cursors with timeout disabled and timed out cursors',
                    'cursors', 'database performance', 'mongodb.cursors', 'stacked'],
        'lines': [
            ['cursor_total', 'openned', 'absolute', 1, 1],
            ['noTimeout', None, 'absolute', 1, 1],
            ['timedOut', None, 'incremental', 1, 1]
        ]
    },
    'connections': {
        'options': [None, 'Currently connected clients and unused connections', 'connections',
                    'resource utilization', 'mongodb.connections', 'stacked'],
        'lines': [
            ['connections_available', 'unused', 'absolute', 1, 1],
            ['connections_current', 'connected', 'absolute', 1, 1]
        ]
    },
    'memory': {
        'options': [None, 'Memory metrics', 'MiB', 'resource utilization', 'mongodb.memory', 'stacked'],
        'lines': [
            ['virtual', None, 'absolute', 1, 1],
            ['resident', None, 'absolute', 1, 1],
            ['nonmapped', None, 'absolute', 1, 1],
            ['mapped', None, 'absolute', 1, 1]
        ]
    },
    'page_faults': {
        'options': [None, 'Number of times MongoDB had to fetch data from disk', 'request/s',
                    'resource utilization', 'mongodb.page_faults', 'line'],
        'lines': [
            ['page_faults', None, 'incremental', 1, 1]
        ]
    },
    'queued_requests': {
        'options': [None, 'Currently queued read and write requests', 'requests', 'resource saturation',
                    'mongodb.queued_requests', 'line'],
        'lines': [
            ['currentQueue_readers', 'readers', 'absolute', 1, 1],
            ['currentQueue_writers', 'writers', 'absolute', 1, 1]
        ]
    },
    'record_moves': {
        'options': [None, 'Number of times documents had to be moved on-disk', 'number',
                    'resource saturation', 'mongodb.record_moves', 'line'],
        'lines': [
            ['moves', None, 'incremental', 1, 1]
        ]
    },
    'asserts': {
        'options': [
            None,
            'Number of message, warning, regular, corresponding to errors generated by users assertions raised',
            'number', 'errors (asserts)', 'mongodb.asserts', 'line'],
        'lines': [
            ['msg', None, 'incremental', 1, 1],
            ['warning', None, 'incremental', 1, 1],
            ['regular', None, 'incremental', 1, 1],
            ['user', None, 'incremental', 1, 1]
        ]
    },
    'wiredtiger_cache': {
        'options': [None, 'The percentage of the wiredTiger cache that is in use and cache with dirty bytes',
                    'percentage', 'resource utilization', 'mongodb.wiredtiger_cache', 'stacked'],
        'lines': [
            ['wiredTiger_percent_clean', 'inuse', 'absolute', 1, 1000],
            ['wiredTiger_percent_dirty', 'dirty', 'absolute', 1, 1000]
        ]
    },
    'wiredtiger_pages_evicted': {
        'options': [None, 'Pages evicted from the cache',
                    'pages', 'resource utilization', 'mongodb.wiredtiger_pages_evicted', 'stacked'],
        'lines': [
            ['unmodified', None, 'absolute', 1, 1],
            ['modified', None, 'absolute', 1, 1]
        ]
    },
    'dbstats_objects': {
        'options': [None, 'Number of documents in the database among all the collections', 'documents',
                    'storage size metrics', 'mongodb.dbstats_objects', 'stacked'],
        'lines': []
    },
    'tcmalloc_generic': {
        'options': [None, 'Tcmalloc generic metrics', 'MiB', 'tcmalloc', 'mongodb.tcmalloc_generic', 'stacked'],
        'lines': [
            ['current_allocated_bytes', 'allocated', 'absolute', 1, 1 << 20],
            ['heap_size', 'heap_size', 'absolute', 1, 1 << 20]
        ]
    },
    'tcmalloc_metrics': {
        'options': [None, 'Tcmalloc metrics', 'KiB', 'tcmalloc', 'mongodb.tcmalloc_metrics', 'stacked'],
        'lines': [
            ['central_cache_free_bytes', 'central_cache_free', 'absolute', 1, 1024],
            ['current_total_thread_cache_bytes', 'current_total_thread_cache', 'absolute', 1, 1024],
            ['pageheap_free_bytes', 'pageheap_free', 'absolute', 1, 1024],
            ['pageheap_unmapped_bytes', 'pageheap_unmapped', 'absolute', 1, 1024],
            ['thread_cache_free_bytes', 'thread_cache_free', 'absolute', 1, 1024],
            ['transfer_cache_free_bytes', 'transfer_cache_free', 'absolute', 1, 1024]
        ]
    },
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
        ]
    },
    'command_failed_rate': {
        'options': [None, 'Commands failed rate', 'commands/s', 'commands', 'mongodb.command_failed_rate', 'stacked'],
        'lines': [
            ['count_failed', 'count', 'incremental', 1, 1],
            ['createIndexes_failed', 'createIndexes', 'incremental', 1, 1],
            ['delete_failed', 'delete', 'incremental', 1, 1],
            ['eval_failed', 'eval', 'incremental', 1, 1],
            ['findAndModify_failed', 'findAndModify', 'incremental', 1, 1],
            ['insert_failed', 'insert', 'incremental', 1, 1],
            ['update_failed', 'update', 'incremental', 1, 1]
        ]
    },
    'locks_collection': {
        'options': [None, 'Collection lock. Number of times the lock was acquired in the specified mode',
                    'locks', 'locks metrics', 'mongodb.locks_collection', 'stacked'],
        'lines': [
            ['Collection_R', 'shared', 'incremental'],
            ['Collection_W', 'exclusive', 'incremental'],
            ['Collection_r', 'intent_shared', 'incremental'],
            ['Collection_w', 'intent_exclusive', 'incremental']
        ]
    },
    'locks_database': {
        'options': [None, 'Database lock. Number of times the lock was acquired in the specified mode',
                    'locks', 'locks metrics', 'mongodb.locks_database', 'stacked'],
        'lines': [
            ['Database_R', 'shared', 'incremental'],
            ['Database_W', 'exclusive', 'incremental'],
            ['Database_r', 'intent_shared', 'incremental'],
            ['Database_w', 'intent_exclusive', 'incremental']
        ]
    },
    'locks_global': {
        'options': [None, 'Global lock. Number of times the lock was acquired in the specified mode',
                    'locks', 'locks metrics', 'mongodb.locks_global', 'stacked'],
        'lines': [
            ['Global_R', 'shared', 'incremental'],
            ['Global_W', 'exclusive', 'incremental'],
            ['Global_r', 'intent_shared', 'incremental'],
            ['Global_w', 'intent_exclusive', 'incremental']
        ]
    },
    'locks_metadata': {
        'options': [None, 'Metadata lock. Number of times the lock was acquired in the specified mode',
                    'locks', 'locks metrics', 'mongodb.locks_metadata', 'stacked'],
        'lines': [
            ['Metadata_R', 'shared', 'incremental'],
            ['Metadata_w', 'intent_exclusive', 'incremental']
        ]
    },
    'locks_oplog': {
        'options': [None, 'Lock on the oplog. Number of times the lock was acquired in the specified mode',
                    'locks', 'locks metrics', 'mongodb.locks_oplog', 'stacked'],
        'lines': [
            ['oplog_r', 'intent_shared', 'incremental'],
            ['oplog_w', 'intent_exclusive', 'incremental']
        ]
    }
}

DEFAULT_HOST = '127.0.0.1'
DEFAULT_PORT = 27017
DEFAULT_TIMEOUT = 100
DEFAULT_AUTHDB = 'admin'

CONN_PARAM_HOST = 'host'
CONN_PARAM_PORT = 'port'
CONN_PARAM_SERVER_SELECTION_TIMEOUT_MS = 'serverselectiontimeoutms'
CONN_PARAM_SSL_SSL = 'ssl'
CONN_PARAM_SSL_CERT_REQS = 'ssl_cert_reqs'
CONN_PARAM_SSL_CA_CERTS = 'ssl_ca_certs'
CONN_PARAM_SSL_CRL_FILE = 'ssl_crlfile'
CONN_PARAM_SSL_CERT_FILE = 'ssl_certfile'
CONN_PARAM_SSL_KEY_FILE = 'ssl_keyfile'
CONN_PARAM_SSL_PEM_PASSPHRASE = 'ssl_pem_passphrase'


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER[:]
        self.definitions = deepcopy(CHARTS)
        self.authdb = self.configuration.get('authdb', DEFAULT_AUTHDB)
        self.user = self.configuration.get('user')
        self.password = self.configuration.get('pass')
        self.metrics_to_collect = deepcopy(DEFAULT_METRICS)
        self.connection = None
        self.do_replica = None
        self.databases = list()

    def check(self):
        if not PYMONGO:
            self.error('Pymongo package v2.4+ is needed to use mongodb.chart.py')
            return False
        self.connection, server_status, error = self._create_connection()
        if error:
            self.error(error)
            return False

        self.build_metrics_to_collect_(server_status)

        try:
            data = self._get_data()
        except (LookupError, SyntaxError, AttributeError):
            self.error('Type: %s, error: %s' % (str(exc_info()[0]), str(exc_info()[1])))
            return False
        if isinstance(data, dict) and data:
            self._data_from_check = data
            self.create_charts_(server_status)
            return True
        self.error('_get_data() returned no data or type is not <dict>')
        return False

    def build_metrics_to_collect_(self, server_status):

        self.do_replica = 'repl' in server_status
        if 'dur' in server_status:
            self.metrics_to_collect.extend(DUR)
        if 'tcmalloc' in server_status:
            self.metrics_to_collect.extend(TCMALLOC)
        if 'commands' in server_status['metrics']:
            self.metrics_to_collect.extend(COMMANDS)
        if 'wiredTiger' in server_status:
            self.metrics_to_collect.extend(WIREDTIGER)
        has_locks = 'locks' in server_status
        if has_locks and 'Collection' in server_status['locks']:
            self.metrics_to_collect.extend(LOCKS)

    def create_charts_(self, server_status):

        if 'dur' not in server_status:
            self.order.remove('journaling_transactions')
            self.order.remove('journaling_volume')

        if 'backgroundFlushing' not in server_status:
            self.order.remove('background_flush_average')
            self.order.remove('background_flush_last')
            self.order.remove('background_flush_rate')

        if 'wiredTiger' not in server_status:
            self.order.remove('wiredtiger_write')
            self.order.remove('wiredtiger_read')
            self.order.remove('wiredtiger_cache')

        if 'tcmalloc' not in server_status:
            self.order.remove('tcmalloc_generic')
            self.order.remove('tcmalloc_metrics')

        if 'commands' not in server_status['metrics']:
            self.order.remove('command_total_rate')
            self.order.remove('command_failed_rate')

        has_no_locks = 'locks' not in server_status
        if has_no_locks or 'Collection' not in server_status['locks']:
            self.order.remove('locks_collection')
            self.order.remove('locks_database')
            self.order.remove('locks_global')
            self.order.remove('locks_metadata')

        if has_no_locks or 'oplog' not in server_status['locks']:
            self.order.remove('locks_oplog')

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

        if self.do_replica:
            def create_lines(hosts, string):
                lines = list()
                for host in hosts:
                    dim_id = '_'.join([host, string])
                    lines.append([dim_id, host, 'absolute', 1, 1000])
                return lines

            def create_state_lines(states):
                lines = list()
                for state, description in states:
                    dim_id = '_'.join([host, 'state', state])
                    lines.append([dim_id, description, 'absolute', 1, 1])
                return lines

            all_hosts = server_status['repl']['hosts'] + server_status['repl'].get('arbiters', list())
            this_host = server_status['repl']['me']
            other_hosts = [host for host in all_hosts if host != this_host]

            if 'local' in self.databases:
                self.order.append('oplog_window')
                self.definitions['oplog_window'] = {
                    'options': [None, 'Interval of time between the oldest and the latest entries in the oplog',
                                'seconds', 'replication and oplog', 'mongodb.oplog_window', 'line'],
                    'lines': [['timeDiff', 'window', 'absolute', 1, 1000]]}
            # Create "heartbeat delay" chart
            self.order.append('heartbeat_delay')
            self.definitions['heartbeat_delay'] = {
                'options': [
                    None,
                    'Time when last heartbeat was received from the replica set member (lastHeartbeatRecv)',
                    'seconds ago', 'replication and oplog', 'mongodb.replication_heartbeat_delay', 'stacked'],
                'lines': create_lines(other_hosts, 'heartbeat_lag')}
            # Create "optimedate delay" chart
            self.order.append('optimedate_delay')
            self.definitions['optimedate_delay'] = {
                'options': [None, 'Time when last entry from the oplog was applied (optimeDate)',
                            'seconds ago', 'replication and oplog', 'mongodb.replication_optimedate_delay', 'stacked'],
                'lines': create_lines(all_hosts, 'optimedate')}
            # Create "replica set members state" chart
            for host in all_hosts:
                chart_name = '_'.join([host, 'state'])
                self.order.append(chart_name)
                self.definitions[chart_name] = {
                    'options': [None, 'Replica set member (%s) current state' % host, 'state',
                                'replication and oplog', 'mongodb.replication_state', 'line'],
                    'lines': create_state_lines(REPL_SET_STATES)}

    def _get_raw_data(self):
        raw_data = dict()

        raw_data.update(self.get_server_status() or dict())
        raw_data.update(self.get_db_stats() or dict())
        raw_data.update(self.get_repl_set_get_status() or dict())
        raw_data.update(self.get_get_replication_info() or dict())

        return raw_data or None

    def get_server_status(self):
        raw_data = dict()
        try:
            raw_data['serverStatus'] = self.connection.admin.command('serverStatus')
        except PyMongoError:
            return None
        else:
            return raw_data

    def get_db_stats(self):
        if not self.databases:
            return None

        raw_data = dict()
        raw_data['dbStats'] = dict()
        try:
            for dbase in self.databases:
                raw_data['dbStats'][dbase] = self.connection[dbase].command('dbStats')
            return raw_data
        except PyMongoError:
            return None

    def get_repl_set_get_status(self):
        if not self.do_replica:
            return None

        raw_data = dict()
        try:
            raw_data['replSetGetStatus'] = self.connection.admin.command('replSetGetStatus')
            return raw_data
        except PyMongoError:
            return None

    def get_get_replication_info(self):
        if not (self.do_replica and 'local' in self.databases):
            return None

        raw_data = dict()
        raw_data['getReplicationInfo'] = dict()
        try:
            raw_data['getReplicationInfo']['ASCENDING'] = self.connection.local.oplog.rs.find().sort(
                '$natural', ASCENDING).limit(1)[0]
            raw_data['getReplicationInfo']['DESCENDING'] = self.connection.local.oplog.rs.find().sort(
                '$natural', DESCENDING).limit(1)[0]
            return raw_data
        except PyMongoError:
            return None

    def _get_data(self):
        """
        :return: dict
        """
        raw_data = self._get_raw_data()

        if not raw_data:
            return None

        data = dict()
        serverStatus = raw_data['serverStatus']
        dbStats = raw_data.get('dbStats')
        replSetGetStatus = raw_data.get('replSetGetStatus')
        getReplicationInfo = raw_data.get('getReplicationInfo')
        utc_now = datetime.utcnow()

        # serverStatus
        for metric, new_name, func in self.metrics_to_collect:
            value = serverStatus
            for key in metric.split('.'):
                try:
                    value = value[key]
                except KeyError:
                    break

            if not isinstance(value, dict) and key:
                data[new_name or key] = value if not func else func(value)

        if 'mapped' in serverStatus['mem']:
            data['nonmapped'] = data['virtual'] - serverStatus['mem'].get('mappedWithJournal', data['mapped'])

        if data.get('maximum bytes configured'):
            maximum = data['maximum bytes configured']
            data['wiredTiger_percent_clean'] = int(data['bytes currently in the cache'] * 100 / maximum * 1000)
            data['wiredTiger_percent_dirty'] = int(data['tracked dirty bytes in the cache'] * 100 / maximum * 1000)

        # dbStats
        if dbStats:
            for dbase in dbStats:
                for metric in DBSTATS:
                    key = '_'.join([dbase, metric])
                    data[key] = dbStats[dbase][metric]

        # replSetGetStatus
        if replSetGetStatus:
            other_hosts = list()
            members = replSetGetStatus['members']
            unix_epoch = datetime(1970, 1, 1, 0, 0)

            for member in members:
                if not member.get('self'):
                    other_hosts.append(member)

                # Replica set time diff between current time and time when last entry from the oplog was applied
                if member.get('optimeDate', unix_epoch) != unix_epoch:
                    member_optimedate = member['name'] + '_optimedate'
                    delta = utc_now - member['optimeDate']
                    data[member_optimedate] = int(delta_calculation(delta=delta, multiplier=1000))

                # Replica set members state
                member_state = member['name'] + '_state'
                for elem in REPL_SET_STATES:
                    state = elem[0]
                    data.update({'_'.join([member_state, state]): 0})
                data.update({'_'.join([member_state, str(member['state'])]): member['state']})

            # Heartbeat lag calculation
            for other in other_hosts:
                if other['lastHeartbeatRecv'] != unix_epoch:
                    node = other['name'] + '_heartbeat_lag'
                    delta = utc_now - other['lastHeartbeatRecv']
                    data[node] = int(delta_calculation(delta=delta, multiplier=1000))

        if getReplicationInfo:
            first_event = getReplicationInfo['ASCENDING']['ts'].as_datetime()
            last_event = getReplicationInfo['DESCENDING']['ts'].as_datetime()
            data['timeDiff'] = int(delta_calculation(delta=last_event - first_event, multiplier=1000))

        return data

    def build_ssl_connection_params(self):
        conf = self.configuration

        def cert_req(v):
            if v is None:
                return None
            if not v:
                return ssl.CERT_NONE
            return ssl.CERT_REQUIRED

        ssl_params = {
            CONN_PARAM_SSL_SSL: conf.get(CONN_PARAM_SSL_SSL),
            CONN_PARAM_SSL_CERT_REQS: cert_req(conf.get(CONN_PARAM_SSL_CERT_REQS)),
            CONN_PARAM_SSL_CA_CERTS: conf.get(CONN_PARAM_SSL_CA_CERTS),
            CONN_PARAM_SSL_CRL_FILE: conf.get(CONN_PARAM_SSL_CRL_FILE),
            CONN_PARAM_SSL_CERT_FILE: conf.get(CONN_PARAM_SSL_CERT_FILE),
            CONN_PARAM_SSL_KEY_FILE: conf.get(CONN_PARAM_SSL_KEY_FILE),
            CONN_PARAM_SSL_PEM_PASSPHRASE: conf.get(CONN_PARAM_SSL_PEM_PASSPHRASE),
        }

        ssl_params = dict((k, v) for k, v in ssl_params.items() if v is not None)

        return ssl_params

    def build_connection_params(self):
        conf = self.configuration
        params = {
            CONN_PARAM_HOST: conf.get(CONN_PARAM_HOST, DEFAULT_HOST),
            CONN_PARAM_PORT: conf.get(CONN_PARAM_PORT, DEFAULT_PORT),
        }
        if hasattr(MongoClient, 'server_selection_timeout'):
            params[CONN_PARAM_SERVER_SELECTION_TIMEOUT_MS] = conf.get('timeout', DEFAULT_TIMEOUT)

        params.update(self.build_ssl_connection_params())
        return params

    def _create_connection(self):
        params = self.build_connection_params()
        self.debug('creating connection, connection params: {0}'.format(sorted(params)))

        try:
            connection = MongoClient(**params)
            if self.user and self.password:
                self.debug('authenticating, user: {0}, password: {1}'.format(self.user, self.password))
                getattr(connection, self.authdb).authenticate(name=self.user, password=self.password)
            else:
                self.debug('skip authenticating, user and password are not set')
            # elif self.user:
            #     connection.admin.authenticate(name=self.user, mechanism='MONGODB-X509')
            server_status = connection.admin.command('serverStatus')
        except PyMongoError as error:
            return None, None, str(error)
        else:
            try:
                self.databases = connection.database_names()
            except PyMongoError as error:
                self.info('Can\'t collect databases: %s' % str(error))
            return connection, server_status, None


def delta_calculation(delta, multiplier=1):
    if hasattr(delta, 'total_seconds'):
        return delta.total_seconds() * multiplier
    return (delta.microseconds + (delta.seconds + delta.days * 24 * 3600) * 10 ** 6) / 10.0 ** 6 * multiplier
