# -*- coding: utf-8 -*-
# Description: mongodb netdata python.d module
# Author: l2isbad

from base import SimpleService
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

# charts order (can be overridden if you want less charts, or different order)
ORDER = ['read_operations', 'write_operations', 'active_clients', 'journaling_transactions',
         'journaling_volume', 'background_flush_average', 'background_flush_last', 'background_flush_rate',
         'wiredtiger_read', 'wiredtiger_write', 'cursors', 'connections', 'memory', 'page_faults',
         'queued_requests', 'record_moves', 'wiredtiger_cache', 'wiredtiger_pages_evicted', 'asserts',
         'dbstats_objects', 'tcmalloc_generic', 'tcmalloc_metrics', 'command_total_rate', 'command_failed_rate']

CHARTS = {
    'read_operations': {
        'options': [None, "Received read requests", "requests/s", 'throughput metrics',
                    'mongodb.read_operations', 'line'],
        'lines': [
            ['readWriteOper_query', 'query', 'incremental'],
            ['readWriteOper_getmore', 'getmore', 'incremental']
        ]},
    'write_operations': {
        'options': [None, "Received write requests", "requests/s", 'throughput metrics',
                    'mongodb.write_operations', 'line'],
        'lines': [
            ['readWriteOper_insert', 'insert', 'incremental'],
            ['readWriteOper_update', 'update', 'incremental'],
            ['readWriteOper_delete', 'delete', 'incremental']
        ]},
    'active_clients': {
        'options': [None, "Clients with read or write operations in progress or queued", "clients",
                    'throughput metrics', 'mongodb.active_clients', 'line'],
        'lines': [
            ['activeClients_readers', 'readers', 'absolute'],
            ['activeClients_writers', 'writers', 'absolute']
            ]},
    'journaling_transactions': {
        'options': [None, "Transactions that have been written to the journal", "commits",
                    'database performance', 'mongodb.journaling_transactions', 'line'],
        'lines': [
            ['journalTrans_commits', 'commits', 'absolute']
            ]},
    'journaling_volume': {
        'options': [None, "Volume of data written to the journal", "MB", 'database performance',
                    'mongodb.journaling_volume', 'line'],
        'lines': [
            ['journalTrans_journaled', 'volume', 'absolute', 1, 100]
            ]},
    'background_flush_average': {
        'options': [None, "Average time taken by flushes to execute", "ms", 'database performance',
                    'mongodb.background_flush_average', 'line'],
        'lines': [
            ['background_flush_average', 'time', 'absolute', 1, 100]
            ]},
    'background_flush_last': {
        'options': [None, "Time taken by the last flush operation to execute", "ms", 'database performance',
                    'mongodb.background_flush_last', 'line'],
        'lines': [
            ['background_flush_last', 'time', 'absolute', 1, 100]
            ]},
    'background_flush_rate': {
        'options': [None, "Flushes rate", "flushes", 'database performance', 'mongodb.background_flush_rate', 'line'],
        'lines': [
            ['background_flush_rate', 'flushes', 'incremental', 1, 1]
            ]},
    'wiredtiger_read': {
        'options': [None, "Read tickets in use and remaining", "tickets", 'database performance',
                    'mongodb.wiredtiger_read', 'stacked'],
        'lines': [
            ['wiredTigerRead_available', 'available', 'absolute', 1, 1],
            ['wiredTigerRead_out', 'inuse', 'absolute', 1, 1]
            ]},
    'wiredtiger_write': {
        'options': [None, "Write tickets in use and remaining", "tickets", 'database performance',
                    'mongodb.wiredtiger_write', 'stacked'],
        'lines': [
            ['wiredTigerWrite_available', 'available', 'absolute', 1, 1],
            ['wiredTigerWrite_out', 'inuse', 'absolute', 1, 1]
            ]},
    'cursors': {
        'options': [None, "Currently openned cursors, cursors with timeout disabled and timed out cursors",
                    "cursors", 'database performance', 'mongodb.cursors', 'stacked'],
        'lines': [
            ['cursor_total', 'openned', 'absolute', 1, 1],
            ['cursor_noTimeout', 'notimeout', 'absolute', 1, 1],
            ['cursor_timedOut', 'timedout', 'incremental', 1, 1]
            ]},
    'connections': {
        'options': [None, "Currently connected clients and unused connections", "connections",
                    'resource utilization', 'mongodb.connections', 'stacked'],
        'lines': [
            ['connections_available', 'unused', 'absolute', 1, 1],
            ['connections_current', 'connected', 'absolute', 1, 1]
            ]},
    'memory': {
        'options': [None, "Memory metrics", "MB", 'resource utilization', 'mongodb.memory', 'stacked'],
        'lines': [
            ['memory_virtual', 'virtual', 'absolute', 1, 1],
            ['memory_resident', 'resident', 'absolute', 1, 1],
            ['memory_mapped', 'mapped', 'absolute', 1, 1]
            ]},
    'page_faults': {
        'options': [None, "Number of times MongoDB had to fetch data from disk", "request/s",
                    'resource utilization', 'mongodb.page_faults', 'line'],
        'lines': [
            ['page_faults', 'page_faults', 'incremental', 1, 1]
            ]},
    'queued_requests': {
        'options': [None, "Currently queued read and wrire requests", "requests", 'resource saturation',
                    'mongodb.queued_requests', 'line'],
        'lines': [
            ['currentQueue_readers', 'readers', 'absolute', 1, 1],
            ['currentQueue_writers', 'writers', 'absolute', 1, 1]
            ]},
    'record_moves': {
        'options': [None, "Number of times documents had to be moved on-disk", "number",
                    'resource saturation', 'mongodb.record_moves', 'line'],
        'lines': [
            ['record_moves', 'moves', 'incremental', 1, 1]
            ]},
    'asserts': {
        'options': [None, "Number of message, warning, regular, corresponding to errors generated"
                          " by users assertions raised", "number", 'errors (asserts)', 'mongodb.asserts', 'line'],
        'lines': [
            ['errors_msg', 'msg', 'incremental', 1, 1],
            ['errors_warning', 'warning', 'incremental', 1, 1],
            ['errors_regular', 'regular', 'incremental', 1, 1],
            ['errors_user', 'user', 'incremental', 1, 1]
            ]},
    'wiredtiger_cache': {
        'options': [None, "Amount of space taken by cached data/dirty data in the cache and maximum cache size",
                    "KB", 'resource utilization', 'mongodb.wiredtiger_cache', 'stacked'],
        'lines': [
            ['wiredTiger_bytes_in_cache', 'cached', 'absolute', 1, 1024],
            ['wiredTiger_dirty_in_cache', 'dirty', 'absolute', 1, 1024],
            ['wiredTiger_maximum_in_conf', 'maximum', 'absolute', 1, 1024]
            ]},
    'wiredtiger_pages_evicted': {
        'options': [None, "Pages evicted from the cache",
                    "pages", 'resource utilization', 'mongodb.wiredtiger_pages_evicted', 'stacked'],
        'lines': [
            ['wiredTiger_unmodified_pages_evicted', 'unmodified', 'absolute', 1, 1],
            ['wiredTiger_modified_pages_evicted', 'modified', 'absolute', 1, 1]
            ]},
    'dbstats_objects': {
        'options': [None, "Number of documents in the database among all the collections", "documents",
                    'storage size metrics', 'mongodb.dbstats_objects', 'stacked'],
        'lines': [
            ]},
    'tcmalloc_generic': {
        'options': [None, "Tcmalloc generic metrics", "MB", 'tcmalloc', 'mongodb.tcmalloc_generic', 'stacked'],
        'lines': [
            ['current_allocated_bytes', 'allocated', 'absolute', 1, 1048576],
            ['heap_size', 'heap_size', 'absolute', 1, 1048576]
            ]},
    'tcmalloc_metrics': {
        'options': [None, "Tcmalloc metrics", "KB", 'tcmalloc', 'mongodb.tcmalloc_metrics', 'stacked'],
        'lines': [
            ['central_cache_free_bytes', 'central_cache_free', 'absolute', 1, 1024],
            ['current_total_thread_cache_bytes', 'current_total_thread_cache', 'absolute', 1, 1024],
            ['pageheap_free_bytes', 'pageheap_free', 'absolute', 1, 1024],
            ['pageheap_unmapped_bytes', 'pageheap_unmapped', 'absolute', 1, 1024],
            ['thread_cache_free_bytes', 'thread_cache_free', 'absolute', 1, 1024],
            ['transfer_cache_free_bytes', 'transfer_cache_free', 'absolute', 1, 1024]
            ]},
    'command_total_rate': {
        'options': [None, "Commands total rate", "commands/s", 'commands', 'mongodb.command_total_rate', 'stacked'],
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
        'options': [None, "Commands failed rate", "commands/s", 'commands', 'mongodb.command_failed_rate', 'stacked'],
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

        self._create_charts(server_status)

        return True

    def _create_charts(self, server_status):

        self.order = ORDER[:]
        self.definitions = CHARTS
        self.ss = dict()

        for elem in ['dur', 'backgroundFlushing', 'wiredTiger', 'tcmalloc', 'cursor', 'commands']:
            self.ss[elem] = in_server_status(elem, server_status)

        if not self.ss['dur']:
            self.order.remove('journaling_transactions')
            self.order.remove('journaling_volume')

        if not self.ss['backgroundFlushing']:
            self.order.remove('background_flush_average')
            self.order.remove('background_flush_last')

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

        self.databases = self.connection.database_names()

        for dbase in self.databases:
            self.order.append('_'.join([dbase, 'dbstats']))
            self.definitions['_'.join([dbase, 'dbstats'])] = {
                    'options': [None, "%s: size of all documents, indexes, extents" % dbase, "KB",
                                'storage size metrics', 'mongodb.dbstats', 'line'],
                    'lines': [
                             ['_'.join([dbase, 'dataSize']), 'documents', 'absolute', 1, 1024],
                             ['_'.join([dbase, 'indexSize']), 'indexes', 'absolute', 1, 1024],
                             ['_'.join([dbase, 'storageSize']), 'extents', 'absolute', 1, 1024]
                      ]}
            self.definitions['dbstats_objects']['lines'].append(['_'.join([dbase, 'objects']), dbase, 'absolute'])


    def _get_raw_data(self):
        raw_data = dict()

        try:
            raw_data['serverStatus'] = self.connection.admin.command('serverStatus')
            for dbase in self.databases:
                raw_data[dbase] = self.connection[dbase].command('dbStats')
        except PyMongoError:
                return None
        return raw_data

    def _get_data(self):
        """
        :return: dict
        """
        raw_data = self._get_raw_data()

        if not raw_data:
            return None

        to_netdata = dict()
        server_status = raw_data['serverStatus']

        to_netdata.update(update_dict_key(server_status['opcounters'], 'readWriteOper'))
        to_netdata.update(update_dict_key(server_status['globalLock']['activeClients'], 'activeClients'))
        to_netdata.update(update_dict_key(server_status['connections'], 'connections'))
        to_netdata.update(update_dict_key(server_status['mem'], 'memory'))
        to_netdata.update(update_dict_key(server_status['globalLock']['currentQueue'], 'currentQueue'))
        to_netdata.update(update_dict_key(server_status['asserts'], 'errors'))
        to_netdata['page_faults'] = server_status['extra_info']['page_faults']
        to_netdata['record_moves'] = server_status['metrics']['record']['moves']

        if self.ss['dur']:
            to_netdata['journalTrans_commits'] = server_status['dur']['commits']
            to_netdata['journalTrans_journaled'] = int(server_status['dur']['journaledMB'] * 100)

        if self.ss['backgroundFlushing']:
            to_netdata['background_flush_average'] = int(server_status['backgroundFlushing']['average_ms'] * 100)
            to_netdata['background_flush_last'] = int(server_status['backgroundFlushing']['last_ms'] * 100)
            to_netdata['background_flush_rate'] = server_status['backgroundFlushing']['flushes']

        if self.ss['cursor']:
            to_netdata['cursor_timedOut'] = server_status['metrics']['cursor']['timedOut']
            to_netdata.update(update_dict_key(server_status['metrics']['cursor']['open'], 'cursor'))

        if self.ss['wiredTiger']:
            wired_tiger = server_status['wiredTiger']
            to_netdata.update(update_dict_key(server_status['wiredTiger']['concurrentTransactions']['read'],
                                              'wiredTigerRead'))
            to_netdata.update(update_dict_key(server_status['wiredTiger']['concurrentTransactions']['write'],
                                              'wiredTigerWrite'))
            to_netdata['wiredTiger_bytes_in_cache'] = wired_tiger['cache']['bytes currently in the cache']
            to_netdata['wiredTiger_maximum_in_conf'] = wired_tiger['cache']['maximum bytes configured']
            to_netdata['wiredTiger_dirty_in_cache'] = wired_tiger['cache']['tracked dirty bytes in the cache']
            to_netdata['wiredTiger_unmodified_pages_evicted'] = wired_tiger['cache']['unmodified pages evicted']
            to_netdata['wiredTiger_modified_pages_evicted'] = wired_tiger['cache']['modified pages evicted']

        if self.ss['tcmalloc']:
            to_netdata.update(server_status['tcmalloc']['generic'])
            to_netdata.update(dict([(k, v) for k, v in server_status['tcmalloc']['tcmalloc'].items()
                                    if int_or_float(v)]))

        if self.ss['commands']:
            for elem in ['count', 'createIndexes', 'delete', 'eval', 'findAndModify', 'insert', 'update']:
                to_netdata.update(update_dict_key(server_status['metrics']['commands'][elem], elem))

        for dbase in self.databases:
            dbase_dbstats = raw_data[dbase]
            dbase_dbstats = dict([(k, v) for k, v in dbase_dbstats.items() if int_or_float(v)])
            to_netdata.update(update_dict_key(dbase_dbstats, dbase))

        return to_netdata

    def _create_connection(self):
        conn_vars = {'host': self.host, 'port': self.port}
        if 'server_selection_timeout' in dir(MongoClient):
            conn_vars.update({'serverselectiontimeoutms': self.timeout})
        try:
            connection = MongoClient(**conn_vars)
            if self.user and self.password:
                connection.admin.authenticate(name=self.user, password=self.password)
            server_status = connection.admin.command('serverStatus')
        except PyMongoError as error:
            return None, None, str(error)
        else:
            return connection, server_status, None


def update_dict_key(collection, string):
    return dict([('_'.join([string, k]), int(round(v))) for k, v in collection.items()])


def int_or_float(value):
    return isinstance(value, int) or isinstance(value, float)


def in_server_status(elem, server_status):
    return elem in server_status or elem in server_status['metrics']
