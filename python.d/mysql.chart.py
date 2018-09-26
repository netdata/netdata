# -*- coding: utf-8 -*-
# Description: MySQL netdata python.d module
# Author: Pawel Krupa (paulfantom)
# Author: Ilya Mashchenko (l2isbad)
# SPDX-License-Identifier: GPL-3.0+

from bases.FrameworkServices.MySQLService import MySQLService

# default module values (can be overridden per job in `config`)
# update_every = 3
priority = 60000
retries = 60

# query executed on MySQL server
QUERY_GLOBAL = 'SHOW GLOBAL STATUS;'
QUERY_SLAVE = 'SHOW SLAVE STATUS;'
QUERY_VARIABLES = 'SHOW GLOBAL VARIABLES LIKE \'max_connections\';'

GLOBAL_STATS = [
 'Bytes_received',
 'Bytes_sent',
 'Queries',
 'Questions',
 'Slow_queries',
 'Handler_commit',
 'Handler_delete',
 'Handler_prepare',
 'Handler_read_first',
 'Handler_read_key',
 'Handler_read_next',
 'Handler_read_prev',
 'Handler_read_rnd',
 'Handler_read_rnd_next',
 'Handler_rollback',
 'Handler_savepoint',
 'Handler_savepoint_rollback',
 'Handler_update',
 'Handler_write',
 'Table_locks_immediate',
 'Table_locks_waited',
 'Select_full_join',
 'Select_full_range_join',
 'Select_range',
 'Select_range_check',
 'Select_scan',
 'Sort_merge_passes',
 'Sort_range',
 'Sort_scan',
 'Created_tmp_disk_tables',
 'Created_tmp_files',
 'Created_tmp_tables',
 'Connections',
 'Aborted_connects',
 'Max_used_connections',
 'Binlog_cache_disk_use',
 'Binlog_cache_use',
 'Threads_connected',
 'Threads_created',
 'Threads_cached',
 'Threads_running',
 'Thread_cache_misses',
 'Innodb_data_read',
 'Innodb_data_written',
 'Innodb_data_reads',
 'Innodb_data_writes',
 'Innodb_data_fsyncs',
 'Innodb_data_pending_reads',
 'Innodb_data_pending_writes',
 'Innodb_data_pending_fsyncs',
 'Innodb_log_waits',
 'Innodb_log_write_requests',
 'Innodb_log_writes',
 'Innodb_os_log_fsyncs',
 'Innodb_os_log_pending_fsyncs',
 'Innodb_os_log_pending_writes',
 'Innodb_os_log_written',
 'Innodb_row_lock_current_waits',
 'Innodb_rows_inserted',
 'Innodb_rows_read',
 'Innodb_rows_updated',
 'Innodb_rows_deleted',
 'Innodb_buffer_pool_pages_data',
 'Innodb_buffer_pool_pages_dirty',
 'Innodb_buffer_pool_pages_free',
 'Innodb_buffer_pool_pages_flushed',
 'Innodb_buffer_pool_pages_misc',
 'Innodb_buffer_pool_pages_total',
 'Innodb_buffer_pool_bytes_data',
 'Innodb_buffer_pool_bytes_dirty',
 'Innodb_buffer_pool_read_ahead',
 'Innodb_buffer_pool_read_ahead_evicted',
 'Innodb_buffer_pool_read_ahead_rnd',
 'Innodb_buffer_pool_read_requests',
 'Innodb_buffer_pool_write_requests',
 'Innodb_buffer_pool_reads',
 'Innodb_buffer_pool_wait_free',
 'Qcache_hits',
 'Qcache_lowmem_prunes',
 'Qcache_inserts',
 'Qcache_not_cached',
 'Qcache_queries_in_cache',
 'Qcache_free_memory',
 'Qcache_free_blocks',
 'Qcache_total_blocks',
 'Key_blocks_unused',
 'Key_blocks_used',
 'Key_blocks_not_flushed',
 'Key_read_requests',
 'Key_write_requests',
 'Key_reads',
 'Key_writes',
 'Open_files',
 'Opened_files',
 'Binlog_stmt_cache_disk_use',
 'Binlog_stmt_cache_use',
 'Connection_errors_accept',
 'Connection_errors_internal',
 'Connection_errors_max_connections',
 'Connection_errors_peer_address',
 'Connection_errors_select',
 'Connection_errors_tcpwrap',
 'wsrep_local_recv_queue',
 'wsrep_local_send_queue',
 'wsrep_received',
 'wsrep_replicated',
 'wsrep_received_bytes',
 'wsrep_replicated_bytes',
 'wsrep_local_bf_aborts',
 'wsrep_local_cert_failures',
 'wsrep_flow_control_paused_ns']

def slave_seconds(value):
    try:
        return int(value)
    except (TypeError, ValueError):
        return -1


def slave_running(value):
    return 1 if value == 'Yes' else -1


SLAVE_STATS = [
    ('Seconds_Behind_Master', slave_seconds),
    ('Slave_SQL_Running', slave_running),
    ('Slave_IO_Running', slave_running)
]

VARIABLES = [
 'max_connections']

ORDER = ['net',
         'queries',
         'handlers',
         'table_locks',
         'join_issues', 'sort_issues',
         'tmp',
         'connections', 'connections_active', 'connection_errors',
         'binlog_cache', 'binlog_stmt_cache',
         'threads', 'thread_cache_misses',
         'innodb_io', 'innodb_io_ops', 'innodb_io_pending_ops', 'innodb_log', 'innodb_os_log', 'innodb_os_log_io',
         'innodb_cur_row_lock', 'innodb_rows', 'innodb_buffer_pool_pages', 'innodb_buffer_pool_bytes',
         'innodb_buffer_pool_read_ahead', 'innodb_buffer_pool_reqs', 'innodb_buffer_pool_ops',
         'qcache_ops', 'qcache', 'qcache_freemem', 'qcache_memblocks',
         'key_blocks', 'key_requests', 'key_disk_ops',
         'files', 'files_rate', 'slave_behind', 'slave_status',
         'galera_writesets', 'galera_bytes', 'galera_queue', 'galera_conflicts', 'galera_flow_control']

CHARTS = {
    'net': {
        'options': [None, 'mysql Bandwidth', 'kilobits/s', 'bandwidth', 'mysql.net', 'area'],
        'lines': [
            ['Bytes_received', 'in', 'incremental', 8, 1024],
            ['Bytes_sent', 'out', 'incremental', -8, 1024]
        ]},
    'queries': {
        'options': [None, 'mysql Queries', 'queries/s', 'queries', 'mysql.queries', 'line'],
        'lines': [
            ['Queries', 'queries', 'incremental'],
            ['Questions', 'questions', 'incremental'],
            ['Slow_queries', 'slow_queries', 'incremental']
        ]},
    'handlers': {
        'options': [None, 'mysql Handlers', 'handlers/s', 'handlers', 'mysql.handlers', 'line'],
        'lines': [
            ['Handler_commit', 'commit', 'incremental'],
            ['Handler_delete', 'delete', 'incremental'],
            ['Handler_prepare', 'prepare', 'incremental'],
            ['Handler_read_first', 'read_first', 'incremental'],
            ['Handler_read_key', 'read_key', 'incremental'],
            ['Handler_read_next', 'read_next', 'incremental'],
            ['Handler_read_prev', 'read_prev', 'incremental'],
            ['Handler_read_rnd', 'read_rnd', 'incremental'],
            ['Handler_read_rnd_next', 'read_rnd_next', 'incremental'],
            ['Handler_rollback', 'rollback', 'incremental'],
            ['Handler_savepoint', 'savepoint', 'incremental'],
            ['Handler_savepoint_rollback', 'savepoint_rollback', 'incremental'],
            ['Handler_update', 'update', 'incremental'],
            ['Handler_write', 'write', 'incremental']
        ]},
    'table_locks': {
        'options': [None, 'mysql Tables Locks', 'locks/s', 'locks', 'mysql.table_locks', 'line'],
        'lines': [
            ['Table_locks_immediate', 'immediate', 'incremental'],
            ['Table_locks_waited', 'waited', 'incremental', -1, 1]
        ]},
    'join_issues': {
        'options': [None, 'mysql Select Join Issues', 'joins/s', 'issues', 'mysql.join_issues', 'line'],
        'lines': [
            ['Select_full_join', 'full_join', 'incremental'],
            ['Select_full_range_join', 'full_range_join', 'incremental'],
            ['Select_range', 'range', 'incremental'],
            ['Select_range_check', 'range_check', 'incremental'],
            ['Select_scan', 'scan', 'incremental']
        ]},
    'sort_issues': {
        'options': [None, 'mysql Sort Issues', 'issues/s', 'issues', 'mysql.sort_issues', 'line'],
        'lines': [
            ['Sort_merge_passes', 'merge_passes', 'incremental'],
            ['Sort_range', 'range', 'incremental'],
            ['Sort_scan', 'scan', 'incremental']
        ]},
    'tmp': {
        'options': [None, 'mysql Tmp Operations', 'counter', 'temporaries', 'mysql.tmp', 'line'],
        'lines': [
            ['Created_tmp_disk_tables', 'disk_tables', 'incremental'],
            ['Created_tmp_files', 'files', 'incremental'],
            ['Created_tmp_tables', 'tables', 'incremental']
        ]},
    'connections': {
        'options': [None, 'mysql Connections', 'connections/s', 'connections', 'mysql.connections', 'line'],
        'lines': [
            ['Connections', 'all', 'incremental'],
            ['Aborted_connects', 'aborted', 'incremental']
        ]},
    'connections_active': {
        'options': [None, 'mysql Connections Active', 'connections', 'connections', 'mysql.connections_active', 'line'],
        'lines': [
            ['Threads_connected', 'active', 'absolute'],
            ['max_connections', 'limit', 'absolute'],
            ['Max_used_connections', 'max_active', 'absolute']
        ]},
    'binlog_cache': {
        'options': [None, 'mysql Binlog Cache', 'transactions/s', 'binlog', 'mysql.binlog_cache', 'line'],
        'lines': [
            ['Binlog_cache_disk_use', 'disk', 'incremental'],
            ['Binlog_cache_use', 'all', 'incremental']
        ]},
    'threads': {
        'options': [None, 'mysql Threads', 'threads', 'threads', 'mysql.threads', 'line'],
        'lines': [
            ['Threads_connected', 'connected', 'absolute'],
            ['Threads_created', 'created', 'incremental'],
            ['Threads_cached', 'cached', 'absolute', -1, 1],
            ['Threads_running', 'running', 'absolute'],
        ]},
    'thread_cache_misses': {
        'options': [None, 'mysql Threads Cache Misses', 'misses', 'threads', 'mysql.thread_cache_misses', 'area'],
        'lines': [
            ['Thread_cache_misses', 'misses', 'absolute', 1, 100]
        ]},
    'innodb_io': {
        'options': [None, 'mysql InnoDB I/O Bandwidth', 'kilobytes/s', 'innodb', 'mysql.innodb_io', 'area'],
        'lines': [
            ['Innodb_data_read', 'read', 'incremental', 1, 1024],
            ['Innodb_data_written', 'write', 'incremental', -1, 1024]
        ]},
    'innodb_io_ops': {
        'options': [None, 'mysql InnoDB I/O Operations', 'operations/s', 'innodb', 'mysql.innodb_io_ops', 'line'],
        'lines': [
            ['Innodb_data_reads', 'reads', 'incremental'],
            ['Innodb_data_writes', 'writes', 'incremental', -1, 1],
            ['Innodb_data_fsyncs', 'fsyncs', 'incremental']
        ]},
    'innodb_io_pending_ops': {
        'options': [None, 'mysql InnoDB Pending I/O Operations', 'operations', 'innodb',
                    'mysql.innodb_io_pending_ops', 'line'],
        'lines': [
            ['Innodb_data_pending_reads', 'reads', 'absolute'],
            ['Innodb_data_pending_writes', 'writes', 'absolute', -1, 1],
            ['Innodb_data_pending_fsyncs', 'fsyncs', 'absolute']
        ]},
    'innodb_log': {
        'options': [None, 'mysql InnoDB Log Operations', 'operations/s', 'innodb', 'mysql.innodb_log', 'line'],
        'lines': [
            ['Innodb_log_waits', 'waits', 'incremental'],
            ['Innodb_log_write_requests', 'write_requests', 'incremental', -1, 1],
            ['Innodb_log_writes', 'writes', 'incremental', -1, 1],
        ]},
    'innodb_os_log': {
        'options': [None, 'mysql InnoDB OS Log Operations', 'operations', 'innodb', 'mysql.innodb_os_log', 'line'],
        'lines': [
            ['Innodb_os_log_fsyncs', 'fsyncs', 'incremental'],
            ['Innodb_os_log_pending_fsyncs', 'pending_fsyncs', 'absolute'],
            ['Innodb_os_log_pending_writes', 'pending_writes', 'absolute', -1, 1],
        ]},
    'innodb_os_log_io': {
        'options': [None, 'mysql InnoDB OS Log Bandwidth', 'kilobytes/s', 'innodb', 'mysql.innodb_os_log_io', 'area'],
        'lines': [
            ['Innodb_os_log_written', 'write', 'incremental', -1, 1024],
        ]},
    'innodb_cur_row_lock': {
        'options': [None, 'mysql InnoDB Current Row Locks', 'operations', 'innodb',
                    'mysql.innodb_cur_row_lock', 'area'],
        'lines': [
            ['Innodb_row_lock_current_waits', 'current_waits', 'absolute']
        ]},
    'innodb_rows': {
        'options': [None, 'mysql InnoDB Row Operations', 'operations/s', 'innodb', 'mysql.innodb_rows', 'area'],
        'lines': [
            ['Innodb_rows_inserted', 'inserted', 'incremental'],
            ['Innodb_rows_read', 'read', 'incremental', 1, 1],
            ['Innodb_rows_updated', 'updated', 'incremental', 1, 1],
            ['Innodb_rows_deleted', 'deleted', 'incremental', -1, 1],
        ]},
    'innodb_buffer_pool_pages': {
        'options': [None, 'mysql InnoDB Buffer Pool Pages', 'pages', 'innodb',
                    'mysql.innodb_buffer_pool_pages', 'line'],
        'lines': [
            ['Innodb_buffer_pool_pages_data', 'data', 'absolute'],
            ['Innodb_buffer_pool_pages_dirty', 'dirty', 'absolute', -1, 1],
            ['Innodb_buffer_pool_pages_free', 'free', 'absolute'],
            ['Innodb_buffer_pool_pages_flushed', 'flushed', 'incremental', -1, 1],
            ['Innodb_buffer_pool_pages_misc', 'misc', 'absolute', -1, 1],
            ['Innodb_buffer_pool_pages_total', 'total', 'absolute']
        ]},
    'innodb_buffer_pool_bytes': {
        'options': [None, 'mysql InnoDB Buffer Pool Bytes', 'MB', 'innodb', 'mysql.innodb_buffer_pool_bytes', 'area'],
        'lines': [
            ['Innodb_buffer_pool_bytes_data', 'data', 'absolute', 1, 1024 * 1024],
            ['Innodb_buffer_pool_bytes_dirty', 'dirty', 'absolute', -1, 1024 * 1024]
        ]},
    'innodb_buffer_pool_read_ahead': {
        'options': [None, 'mysql InnoDB Buffer Pool Read Ahead', 'operations/s', 'innodb',
                    'mysql.innodb_buffer_pool_read_ahead', 'area'],
        'lines': [
            ['Innodb_buffer_pool_read_ahead', 'all', 'incremental'],
            ['Innodb_buffer_pool_read_ahead_evicted', 'evicted', 'incremental', -1, 1],
            ['Innodb_buffer_pool_read_ahead_rnd', 'random', 'incremental']
        ]},
    'innodb_buffer_pool_reqs': {
        'options': [None, 'mysql InnoDB Buffer Pool Requests', 'requests/s', 'innodb',
                    'mysql.innodb_buffer_pool_reqs', 'area'],
        'lines': [
            ['Innodb_buffer_pool_read_requests', 'reads', 'incremental'],
            ['Innodb_buffer_pool_write_requests', 'writes', 'incremental', -1, 1]
        ]},
    'innodb_buffer_pool_ops': {
        'options': [None, 'mysql InnoDB Buffer Pool Operations', 'operations/s', 'innodb',
                    'mysql.innodb_buffer_pool_ops', 'area'],
        'lines': [
            ['Innodb_buffer_pool_reads', 'disk reads', 'incremental'],
            ['Innodb_buffer_pool_wait_free', 'wait free', 'incremental', -1, 1]
        ]},
    'qcache_ops': {
        'options': [None, 'mysql QCache Operations', 'queries/s', 'qcache', 'mysql.qcache_ops', 'line'],
        'lines': [
            ['Qcache_hits', 'hits', 'incremental'],
            ['Qcache_lowmem_prunes', 'lowmem prunes', 'incremental', -1, 1],
            ['Qcache_inserts', 'inserts', 'incremental'],
            ['Qcache_not_cached', 'not cached', 'incremental', -1, 1]
        ]},
    'qcache': {
        'options': [None, 'mysql QCache Queries in Cache', 'queries', 'qcache', 'mysql.qcache', 'line'],
        'lines': [
            ['Qcache_queries_in_cache', 'queries', 'absolute']
        ]},
    'qcache_freemem': {
        'options': [None, 'mysql QCache Free Memory', 'MB', 'qcache', 'mysql.qcache_freemem', 'area'],
        'lines': [
            ['Qcache_free_memory', 'free', 'absolute', 1, 1024 * 1024]
        ]},
    'qcache_memblocks': {
        'options': [None, 'mysql QCache Memory Blocks', 'blocks', 'qcache', 'mysql.qcache_memblocks', 'line'],
        'lines': [
            ['Qcache_free_blocks', 'free', 'absolute'],
            ['Qcache_total_blocks', 'total', 'absolute']
        ]},
    'key_blocks': {
        'options': [None, 'mysql MyISAM Key Cache Blocks', 'blocks', 'myisam', 'mysql.key_blocks', 'line'],
        'lines': [
            ['Key_blocks_unused', 'unused', 'absolute'],
            ['Key_blocks_used', 'used', 'absolute', -1, 1],
            ['Key_blocks_not_flushed', 'not flushed', 'absolute']
        ]},
    'key_requests': {
        'options': [None, 'mysql MyISAM Key Cache Requests', 'requests/s', 'myisam', 'mysql.key_requests', 'area'],
        'lines': [
            ['Key_read_requests', 'reads', 'incremental'],
            ['Key_write_requests', 'writes', 'incremental', -1, 1]
        ]},
    'key_disk_ops': {
        'options': [None, 'mysql MyISAM Key Cache Disk Operations', 'operations/s',
                    'myisam', 'mysql.key_disk_ops', 'area'],
        'lines': [
            ['Key_reads', 'reads', 'incremental'],
            ['Key_writes', 'writes', 'incremental', -1, 1]
        ]},
    'files': {
        'options': [None, 'mysql Open Files', 'files', 'files', 'mysql.files', 'line'],
        'lines': [
            ['Open_files', 'files', 'absolute']
        ]},
    'files_rate': {
        'options': [None, 'mysql Opened Files Rate', 'files/s', 'files', 'mysql.files_rate', 'line'],
        'lines': [
            ['Opened_files', 'files', 'incremental']
        ]},
    'binlog_stmt_cache': {
        'options': [None, 'mysql Binlog Statement Cache', 'statements/s', 'binlog',
                    'mysql.binlog_stmt_cache', 'line'],
        'lines': [
            ['Binlog_stmt_cache_disk_use', 'disk', 'incremental'],
            ['Binlog_stmt_cache_use', 'all', 'incremental']
        ]},
    'connection_errors': {
        'options': [None, 'mysql Connection Errors', 'connections/s', 'connections',
                    'mysql.connection_errors', 'line'],
        'lines': [
            ['Connection_errors_accept', 'accept', 'incremental'],
            ['Connection_errors_internal', 'internal', 'incremental'],
            ['Connection_errors_max_connections', 'max', 'incremental'],
            ['Connection_errors_peer_address', 'peer_addr', 'incremental'],
            ['Connection_errors_select', 'select', 'incremental'],
            ['Connection_errors_tcpwrap', 'tcpwrap', 'incremental']
        ]},
    'slave_behind': {
        'options': [None, 'Slave Behind Seconds', 'seconds', 'slave', 'mysql.slave_behind', 'line'],
        'lines': [
            ['Seconds_Behind_Master', 'seconds', 'absolute']
        ]},
    'slave_status': {
        'options': [None, 'Slave Status', 'status', 'slave', 'mysql.slave_status', 'line'],
        'lines': [
            ['Slave_SQL_Running', 'sql_running', 'absolute'],
            ['Slave_IO_Running', 'io_running', 'absolute']
        ]},
    'galera_writesets': {
        'options': [None, 'Replicated writesets', 'writesets/s', 'galera', 'mysql.galera_writesets', 'line'],
        'lines': [
            ['wsrep_received', 'rx', 'incremental'],
            ['wsrep_replicated', 'tx', 'incremental', -1, 1],
        ]},
    'galera_bytes': {
        'options': [None, 'Replicated bytes', 'KB/s', 'galera', 'mysql.galera_bytes', 'area'],
        'lines': [
            ['wsrep_received_bytes', 'rx', 'incremental', 1, 1024],
            ['wsrep_replicated_bytes', 'tx', 'incremental', -1, 1024],
        ]},
    'galera_queue': {
        'options': [None, 'Galera queue', 'writesets', 'galera', 'mysql.galera_queue', 'line'],
        'lines': [
            ['wsrep_local_recv_queue', 'rx', 'absolute'],
            ['wsrep_local_send_queue', 'tx', 'absolute', -1, 1],
        ]},
    'galera_conflicts': {
        'options': [None, 'Replication conflicts', 'transactions', 'galera', 'mysql.galera_conflicts', 'area'],
        'lines': [
            ['wsrep_local_bf_aborts', 'bf_aborts', 'incremental'],
            ['wsrep_local_cert_failures', 'cert_fails', 'incremental', -1, 1],
        ]},
    'galera_flow_control': {
        'options': [None, 'Flow control', 'millisec', 'galera', 'mysql.galera_flow_control', 'area'],
        'lines': [
            ['wsrep_flow_control_paused_ns', 'paused', 'incremental', 1, 1000000],
        ]}
}


class Service(MySQLService):
    def __init__(self, configuration=None, name=None):
        MySQLService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.queries = dict(global_status=QUERY_GLOBAL, slave_status=QUERY_SLAVE, variables=QUERY_VARIABLES)

    def _get_data(self):

        raw_data = self._get_raw_data(description=True)

        if not raw_data:
            return None

        to_netdata = dict()

        if 'global_status' in raw_data:
            global_status = dict(raw_data['global_status'][0])
            for key in GLOBAL_STATS:
                if key in global_status:
                    to_netdata[key] = global_status[key]
            if 'Threads_created' in to_netdata and 'Connections' in to_netdata:
                to_netdata['Thread_cache_misses'] = round(int(to_netdata['Threads_created'])
                                                          / float(to_netdata['Connections']) * 10000)

        if 'slave_status' in raw_data:
            if raw_data['slave_status'][0]:
                slave_raw_data = dict(zip([e[0] for e in raw_data['slave_status'][1]], raw_data['slave_status'][0][0]))
                for key, func in SLAVE_STATS:
                    if key in slave_raw_data:
                        to_netdata[key] = func(slave_raw_data[key])
            else:
                self.queries.pop('slave_status')

        if 'variables' in raw_data:
            variables = dict(raw_data['variables'][0])
            for key in VARIABLES:
                if key in variables:
                    to_netdata[key] = variables[key]

        return to_netdata or None

