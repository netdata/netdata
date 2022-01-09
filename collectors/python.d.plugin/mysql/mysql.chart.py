# -*- coding: utf-8 -*-
# Description: MySQL netdata python.d module
# Author: Pawel Krupa (paulfantom)
# Author: Ilya Mashchenko (ilyam8)
# SPDX-License-Identifier: GPL-3.0-or-later

from bases.FrameworkServices.MySQLService import MySQLService

# query executed on MySQL server
QUERY_GLOBAL = 'SHOW GLOBAL STATUS;'
QUERY_SLAVE = 'SHOW SLAVE STATUS;'
QUERY_VARIABLES = 'SHOW GLOBAL VARIABLES LIKE \'max_connections\';'
QUERY_USER_STATISTICS = 'SHOW USER_STATISTICS;'

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
    'Innodb_deadlocks',
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
    'Com_delete',
    'Com_insert',
    'Com_select',
    'Com_update',
    'Com_replace'
]

GALERA_STATS = [
    'wsrep_local_recv_queue',
    'wsrep_local_send_queue',
    'wsrep_received',
    'wsrep_replicated',
    'wsrep_received_bytes',
    'wsrep_replicated_bytes',
    'wsrep_local_bf_aborts',
    'wsrep_local_cert_failures',
    'wsrep_flow_control_paused_ns',
    'wsrep_cluster_weight',
    'wsrep_cluster_size',
    'wsrep_cluster_status',
    'wsrep_local_state',
    'wsrep_open_transactions',
    'wsrep_connected',
    'wsrep_ready',
    'wsrep_thread_count'
]


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

USER_STATISTICS = [
    'Select_commands',
    'Update_commands',
    'Other_commands',
    'Cpu_time',
    'Rows_read',
    'Rows_sent',
    'Rows_deleted',
    'Rows_inserted',
    'Rows_updated'
]

VARIABLES = [
    'max_connections'
]

ORDER = [
    'net',
    'queries',
    'queries_type',
    'handlers',
    'table_locks',
    'join_issues',
    'sort_issues',
    'tmp',
    'connections',
    'connections_active',
    'connection_errors',
    'binlog_cache',
    'binlog_stmt_cache',
    'threads',
    'threads_creation_rate',
    'thread_cache_misses',
    'innodb_io',
    'innodb_io_ops',
    'innodb_io_pending_ops',
    'innodb_log',
    'innodb_os_log',
    'innodb_os_log_fsync_writes',
    'innodb_os_log_io',
    'innodb_cur_row_lock',
    'innodb_deadlocks',
    'innodb_rows',
    'innodb_buffer_pool_pages',
    'innodb_buffer_pool_flush_pages_requests',
    'innodb_buffer_pool_bytes',
    'innodb_buffer_pool_read_ahead',
    'innodb_buffer_pool_reqs',
    'innodb_buffer_pool_ops',
    'qcache_ops',
    'qcache',
    'qcache_freemem',
    'qcache_memblocks',
    'key_blocks',
    'key_requests',
    'key_disk_ops',
    'files',
    'files_rate',
    'slave_behind',
    'slave_status',
    'galera_writesets',
    'galera_bytes',
    'galera_queue',
    'galera_conflicts',
    'galera_flow_control',
    'galera_cluster_status',
    'galera_cluster_state',
    'galera_cluster_size',
    'galera_cluster_weight',
    'galera_connected',
    'galera_ready',
    'galera_open_transactions',
    'galera_thread_count',
    'userstats_cpu',
]

CHARTS = {
    'net': {
        'options': [None, 'Bandwidth', 'kilobits/s', 'bandwidth', 'mysql.net', 'area'],
        'lines': [
            ['Bytes_received', 'in', 'incremental', 8, 1000],
            ['Bytes_sent', 'out', 'incremental', -8, 1000]
        ]
    },
    'queries': {
        'options': [None, 'Queries', 'queries/s', 'queries', 'mysql.queries', 'line'],
        'lines': [
            ['Queries', 'queries', 'incremental'],
            ['Questions', 'questions', 'incremental'],
            ['Slow_queries', 'slow_queries', 'incremental']
        ]
    },
    'queries_type': {
        'options': [None, 'Query Type', 'queries/s', 'query_types', 'mysql.queries_type', 'stacked'],
        'lines': [
            ['Com_select', 'select', 'incremental'],
            ['Com_delete', 'delete', 'incremental'],
            ['Com_update', 'update', 'incremental'],
            ['Com_insert', 'insert', 'incremental'],
            ['Qcache_hits', 'cache_hits', 'incremental'],
            ['Com_replace', 'replace', 'incremental']
        ]
    },
    'handlers': {
        'options': [None, 'Handlers', 'handlers/s', 'handlers', 'mysql.handlers', 'line'],
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
        ]
    },
    'table_locks': {
        'options': [None, 'Tables Locks', 'locks/s', 'locks', 'mysql.table_locks', 'line'],
        'lines': [
            ['Table_locks_immediate', 'immediate', 'incremental'],
            ['Table_locks_waited', 'waited', 'incremental', -1, 1]
        ]
    },
    'join_issues': {
        'options': [None, 'Select Join Issues', 'joins/s', 'issues', 'mysql.join_issues', 'line'],
        'lines': [
            ['Select_full_join', 'full_join', 'incremental'],
            ['Select_full_range_join', 'full_range_join', 'incremental'],
            ['Select_range', 'range', 'incremental'],
            ['Select_range_check', 'range_check', 'incremental'],
            ['Select_scan', 'scan', 'incremental']
        ]
    },
    'sort_issues': {
        'options': [None, 'Sort Issues', 'issues/s', 'issues', 'mysql.sort_issues', 'line'],
        'lines': [
            ['Sort_merge_passes', 'merge_passes', 'incremental'],
            ['Sort_range', 'range', 'incremental'],
            ['Sort_scan', 'scan', 'incremental']
        ]
    },
    'tmp': {
        'options': [None, 'Tmp Operations', 'counter', 'temporaries', 'mysql.tmp', 'line'],
        'lines': [
            ['Created_tmp_disk_tables', 'disk_tables', 'incremental'],
            ['Created_tmp_files', 'files', 'incremental'],
            ['Created_tmp_tables', 'tables', 'incremental']
        ]
    },
    'connections': {
        'options': [None, 'Connections', 'connections/s', 'connections', 'mysql.connections', 'line'],
        'lines': [
            ['Connections', 'all', 'incremental'],
            ['Aborted_connects', 'aborted', 'incremental']
        ]
    },
    'connections_active': {
        'options': [None, 'Connections Active', 'connections', 'connections', 'mysql.connections_active', 'line'],
        'lines': [
            ['Threads_connected', 'active', 'absolute'],
            ['max_connections', 'limit', 'absolute'],
            ['Max_used_connections', 'max_active', 'absolute']
        ]
    },
    'binlog_cache': {
        'options': [None, 'Binlog Cache', 'transactions/s', 'binlog', 'mysql.binlog_cache', 'line'],
        'lines': [
            ['Binlog_cache_disk_use', 'disk', 'incremental'],
            ['Binlog_cache_use', 'all', 'incremental']
        ]
    },
    'threads': {
        'options': [None, 'Threads', 'threads', 'threads', 'mysql.threads', 'line'],
        'lines': [
            ['Threads_connected', 'connected', 'absolute'],
            ['Threads_cached', 'cached', 'absolute', -1, 1],
            ['Threads_running', 'running', 'absolute'],
        ]
    },
    'threads_creation_rate': {
        'options': [None, 'Threads Creation Rate', 'threads/s', 'threads', 'mysql.threads_creation_rate', 'line'],
        'lines': [
            ['Threads_created', 'created', 'incremental'],
        ]
    },
    'thread_cache_misses': {
        'options': [None, 'mysql Threads Cache Misses', 'misses', 'threads', 'mysql.thread_cache_misses', 'area'],
        'lines': [
            ['Thread_cache_misses', 'misses', 'absolute', 1, 100]
        ]
    },
    'innodb_io': {
        'options': [None, 'InnoDB I/O Bandwidth', 'KiB/s', 'innodb', 'mysql.innodb_io', 'area'],
        'lines': [
            ['Innodb_data_read', 'read', 'incremental', 1, 1024],
            ['Innodb_data_written', 'write', 'incremental', -1, 1024]
        ]
    },
    'innodb_io_ops': {
        'options': [None, 'InnoDB I/O Operations', 'operations/s', 'innodb', 'mysql.innodb_io_ops', 'line'],
        'lines': [
            ['Innodb_data_reads', 'reads', 'incremental'],
            ['Innodb_data_writes', 'writes', 'incremental', -1, 1],
            ['Innodb_data_fsyncs', 'fsyncs', 'incremental']
        ]
    },
    'innodb_io_pending_ops': {
        'options': [None, 'InnoDB Pending I/O Operations', 'operations', 'innodb',
                    'mysql.innodb_io_pending_ops', 'line'],
        'lines': [
            ['Innodb_data_pending_reads', 'reads', 'absolute'],
            ['Innodb_data_pending_writes', 'writes', 'absolute', -1, 1],
            ['Innodb_data_pending_fsyncs', 'fsyncs', 'absolute']
        ]
    },
    'innodb_log': {
        'options': [None, 'InnoDB Log Operations', 'operations/s', 'innodb', 'mysql.innodb_log', 'line'],
        'lines': [
            ['Innodb_log_waits', 'waits', 'incremental'],
            ['Innodb_log_write_requests', 'write_requests', 'incremental', -1, 1],
            ['Innodb_log_writes', 'writes', 'incremental', -1, 1],
        ]
    },
    'innodb_os_log': {
        'options': [None, 'InnoDB OS Log Pending Operations', 'operations', 'innodb', 'mysql.innodb_os_log', 'line'],
        'lines': [
            ['Innodb_os_log_pending_fsyncs', 'fsyncs', 'absolute'],
            ['Innodb_os_log_pending_writes', 'writes', 'absolute', -1, 1],
        ]
    },
    'innodb_os_log_fsync_writes': {
        'options': [None, 'InnoDB OS Log Operations', 'operations/s', 'innodb', 'mysql.innodb_os_log_fsyncs', 'line'],
        'lines': [
            ['Innodb_os_log_fsyncs', 'fsyncs', 'incremental'],
        ]
    },
    'innodb_os_log_io': {
        'options': [None, 'InnoDB OS Log Bandwidth', 'KiB/s', 'innodb', 'mysql.innodb_os_log_io', 'area'],
        'lines': [
            ['Innodb_os_log_written', 'write', 'incremental', -1, 1024],
        ]
    },
    'innodb_cur_row_lock': {
        'options': [None, 'InnoDB Current Row Locks', 'operations', 'innodb',
                    'mysql.innodb_cur_row_lock', 'area'],
        'lines': [
            ['Innodb_row_lock_current_waits', 'current_waits', 'absolute']
        ]
    },
    'innodb_deadlocks': {
        'options': [None, 'InnoDB Deadlocks', 'operations/s', 'innodb',
                    'mysql.innodb_deadlocks', 'area'],
        'lines': [
            ['Innodb_deadlocks', 'deadlocks', 'incremental']
        ]
    },
    'innodb_rows': {
        'options': [None, 'InnoDB Row Operations', 'operations/s', 'innodb', 'mysql.innodb_rows', 'area'],
        'lines': [
            ['Innodb_rows_inserted', 'inserted', 'incremental'],
            ['Innodb_rows_read', 'read', 'incremental', 1, 1],
            ['Innodb_rows_updated', 'updated', 'incremental', 1, 1],
            ['Innodb_rows_deleted', 'deleted', 'incremental', -1, 1],
        ]
    },
    'innodb_buffer_pool_pages': {
        'options': [None, 'InnoDB Buffer Pool Pages', 'pages', 'innodb',
                    'mysql.innodb_buffer_pool_pages', 'line'],
        'lines': [
            ['Innodb_buffer_pool_pages_data', 'data', 'absolute'],
            ['Innodb_buffer_pool_pages_dirty', 'dirty', 'absolute', -1, 1],
            ['Innodb_buffer_pool_pages_free', 'free', 'absolute'],
            ['Innodb_buffer_pool_pages_misc', 'misc', 'absolute', -1, 1],
            ['Innodb_buffer_pool_pages_total', 'total', 'absolute']
        ]
    },
    'innodb_buffer_pool_flush_pages_requests': {
        'options': [None, 'InnoDB Buffer Pool Flush Pages Requests', 'requests/s', 'innodb',
                    'mysql.innodb_buffer_pool_pages_flushed', 'line'],
        'lines': [
            ['Innodb_buffer_pool_pages_flushed', 'flush pages', 'incremental'],
        ]
    },
    'innodb_buffer_pool_bytes': {
        'options': [None, 'InnoDB Buffer Pool Bytes', 'MiB', 'innodb', 'mysql.innodb_buffer_pool_bytes', 'area'],
        'lines': [
            ['Innodb_buffer_pool_bytes_data', 'data', 'absolute', 1, 1024 * 1024],
            ['Innodb_buffer_pool_bytes_dirty', 'dirty', 'absolute', -1, 1024 * 1024]
        ]
    },
    'innodb_buffer_pool_read_ahead': {
        'options': [None, 'mysql InnoDB Buffer Pool Read Ahead', 'operations/s', 'innodb',
                    'mysql.innodb_buffer_pool_read_ahead', 'area'],
        'lines': [
            ['Innodb_buffer_pool_read_ahead', 'all', 'incremental'],
            ['Innodb_buffer_pool_read_ahead_evicted', 'evicted', 'incremental', -1, 1],
            ['Innodb_buffer_pool_read_ahead_rnd', 'random', 'incremental']
        ]
    },
    'innodb_buffer_pool_reqs': {
        'options': [None, 'InnoDB Buffer Pool Requests', 'requests/s', 'innodb',
                    'mysql.innodb_buffer_pool_reqs', 'area'],
        'lines': [
            ['Innodb_buffer_pool_read_requests', 'reads', 'incremental'],
            ['Innodb_buffer_pool_write_requests', 'writes', 'incremental', -1, 1]
        ]
    },
    'innodb_buffer_pool_ops': {
        'options': [None, 'InnoDB Buffer Pool Operations', 'operations/s', 'innodb',
                    'mysql.innodb_buffer_pool_ops', 'area'],
        'lines': [
            ['Innodb_buffer_pool_reads', 'disk reads', 'incremental'],
            ['Innodb_buffer_pool_wait_free', 'wait free', 'incremental', -1, 1]
        ]
    },
    'qcache_ops': {
        'options': [None, 'QCache Operations', 'queries/s', 'qcache', 'mysql.qcache_ops', 'line'],
        'lines': [
            ['Qcache_hits', 'hits', 'incremental'],
            ['Qcache_lowmem_prunes', 'lowmem prunes', 'incremental', -1, 1],
            ['Qcache_inserts', 'inserts', 'incremental'],
            ['Qcache_not_cached', 'not cached', 'incremental', -1, 1]
        ]
    },
    'qcache': {
        'options': [None, 'QCache Queries in Cache', 'queries', 'qcache', 'mysql.qcache', 'line'],
        'lines': [
            ['Qcache_queries_in_cache', 'queries', 'absolute']
        ]
    },
    'qcache_freemem': {
        'options': [None, 'QCache Free Memory', 'MiB', 'qcache', 'mysql.qcache_freemem', 'area'],
        'lines': [
            ['Qcache_free_memory', 'free', 'absolute', 1, 1024 * 1024]
        ]
    },
    'qcache_memblocks': {
        'options': [None, 'QCache Memory Blocks', 'blocks', 'qcache', 'mysql.qcache_memblocks', 'line'],
        'lines': [
            ['Qcache_free_blocks', 'free', 'absolute'],
            ['Qcache_total_blocks', 'total', 'absolute']
        ]
    },
    'key_blocks': {
        'options': [None, 'MyISAM Key Cache Blocks', 'blocks', 'myisam', 'mysql.key_blocks', 'line'],
        'lines': [
            ['Key_blocks_unused', 'unused', 'absolute'],
            ['Key_blocks_used', 'used', 'absolute', -1, 1],
            ['Key_blocks_not_flushed', 'not flushed', 'absolute']
        ]
    },
    'key_requests': {
        'options': [None, 'MyISAM Key Cache Requests', 'requests/s', 'myisam', 'mysql.key_requests', 'area'],
        'lines': [
            ['Key_read_requests', 'reads', 'incremental'],
            ['Key_write_requests', 'writes', 'incremental', -1, 1]
        ]
    },
    'key_disk_ops': {
        'options': [None, 'MyISAM Key Cache Disk Operations', 'operations/s',
                    'myisam', 'mysql.key_disk_ops', 'area'],
        'lines': [
            ['Key_reads', 'reads', 'incremental'],
            ['Key_writes', 'writes', 'incremental', -1, 1]
        ]
    },
    'files': {
        'options': [None, 'Open Files', 'files', 'files', 'mysql.files', 'line'],
        'lines': [
            ['Open_files', 'files', 'absolute']
        ]
    },
    'files_rate': {
        'options': [None, 'Opened Files Rate', 'files/s', 'files', 'mysql.files_rate', 'line'],
        'lines': [
            ['Opened_files', 'files', 'incremental']
        ]
    },
    'binlog_stmt_cache': {
        'options': [None, 'Binlog Statement Cache', 'statements/s', 'binlog',
                    'mysql.binlog_stmt_cache', 'line'],
        'lines': [
            ['Binlog_stmt_cache_disk_use', 'disk', 'incremental'],
            ['Binlog_stmt_cache_use', 'all', 'incremental']
        ]
    },
    'connection_errors': {
        'options': [None, 'Connection Errors', 'connections/s', 'connections',
                    'mysql.connection_errors', 'line'],
        'lines': [
            ['Connection_errors_accept', 'accept', 'incremental'],
            ['Connection_errors_internal', 'internal', 'incremental'],
            ['Connection_errors_max_connections', 'max', 'incremental'],
            ['Connection_errors_peer_address', 'peer_addr', 'incremental'],
            ['Connection_errors_select', 'select', 'incremental'],
            ['Connection_errors_tcpwrap', 'tcpwrap', 'incremental']
        ]
    },
    'slave_behind': {
        'options': [None, 'Slave Behind Seconds', 'seconds', 'slave', 'mysql.slave_behind', 'line'],
        'lines': [
            ['Seconds_Behind_Master', 'seconds', 'absolute']
        ]
    },
    'slave_status': {
        'options': [None, 'Slave Status', 'status', 'slave', 'mysql.slave_status', 'line'],
        'lines': [
            ['Slave_SQL_Running', 'sql_running', 'absolute'],
            ['Slave_IO_Running', 'io_running', 'absolute']
        ]
    },
    'galera_writesets': {
        'options': [None, 'Replicated Writesets', 'writesets/s', 'galera', 'mysql.galera_writesets', 'line'],
        'lines': [
            ['wsrep_received', 'rx', 'incremental'],
            ['wsrep_replicated', 'tx', 'incremental', -1, 1],
        ]
    },
    'galera_bytes': {
        'options': [None, 'Replicated Bytes', 'KiB/s', 'galera', 'mysql.galera_bytes', 'area'],
        'lines': [
            ['wsrep_received_bytes', 'rx', 'incremental', 1, 1024],
            ['wsrep_replicated_bytes', 'tx', 'incremental', -1, 1024],
        ]
    },
    'galera_queue': {
        'options': [None, 'Galera Queue', 'writesets', 'galera', 'mysql.galera_queue', 'line'],
        'lines': [
            ['wsrep_local_recv_queue', 'rx', 'absolute'],
            ['wsrep_local_send_queue', 'tx', 'absolute', -1, 1],
        ]
    },
    'galera_conflicts': {
        'options': [None, 'Replication Conflicts', 'transactions', 'galera', 'mysql.galera_conflicts', 'area'],
        'lines': [
            ['wsrep_local_bf_aborts', 'bf_aborts', 'incremental'],
            ['wsrep_local_cert_failures', 'cert_fails', 'incremental', -1, 1],
        ]
    },
    'galera_flow_control': {
        'options': [None, 'Flow Control', 'millisec', 'galera', 'mysql.galera_flow_control', 'area'],
        'lines': [
            ['wsrep_flow_control_paused_ns', 'paused', 'incremental', 1, 1000000],
        ]
    },
    'galera_cluster_status': {
        'options': [None, 'Cluster Component Status', 'status', 'galera', 'mysql.galera_cluster_status', 'line'],
        'lines': [
            ['wsrep_cluster_status', 'status', 'absolute'],
        ]
    },
    'galera_cluster_state': {
        'options': [None, 'Cluster Component State', 'state', 'galera', 'mysql.galera_cluster_state', 'line'],
        'lines': [
            ['wsrep_local_state', 'state', 'absolute'],
        ]
    },
    'galera_cluster_size': {
        'options': [None, 'Number of Nodes in the Cluster', 'num', 'galera', 'mysql.galera_cluster_size', 'line'],
        'lines': [
            ['wsrep_cluster_size', 'nodes', 'absolute'],
        ]
    },
    'galera_cluster_weight': {
        'options': [None, 'The Total Weight of the Current Members in the Cluster', 'weight', 'galera',
                    'mysql.galera_cluster_weight', 'line'],
        'lines': [
            ['wsrep_cluster_weight', 'weight', 'absolute'],
        ]
    },
    'galera_connected': {
        'options': [None, 'Whether the Node is Connected to the Cluster', 'boolean', 'galera',
                    'mysql.galera_connected', 'line'],
        'lines': [
            ['wsrep_connected', 'connected', 'absolute'],
        ]
    },
    'galera_ready': {
        'options': [None, 'Whether the Node is Ready to Accept Queries', 'boolean', 'galera',
                    'mysql.galera_ready', 'line'],
        'lines': [
            ['wsrep_ready', 'ready', 'absolute'],
        ]
    },
    'galera_open_transactions': {
        'options': [None, 'Open Transactions', 'num', 'galera', 'mysql.galera_open_transactions', 'line'],
        'lines': [
            ['wsrep_open_transactions', 'open transactions', 'absolute'],
        ]
    },
    'galera_thread_count': {
        'options': [None, 'Total Number of WSRep (applier/rollbacker) Threads', 'num', 'galera',
                    'mysql.galera_thread_count', 'line'],
        'lines': [
            ['wsrep_thread_count', 'threads', 'absolute'],
        ]
    },
    'userstats_cpu': {
        'options': [None, 'Users CPU time', 'percentage', 'userstats', 'mysql.userstats_cpu', 'stacked'],
        'lines': []
    }
}


def slave_status_chart_template(channel_name):
    order = [
        'slave_behind_{0}'.format(channel_name),
        'slave_status_{0}'.format(channel_name)
    ]

    charts = {
        order[0]: {
            'options': [None, 'Slave Behind Seconds Channel {0}'.format(channel_name),
                        'seconds', 'slave', 'mysql.slave_behind', 'line'],
            'lines': [
                ['Seconds_Behind_Master_{0}'.format(channel_name), 'seconds', 'absolute']
            ]
        },
        order[1]: {
            'options': [None, 'Slave Status Channel {0}'.format(channel_name),
                        'status', 'slave', 'mysql.slave_status', 'line'],
            'lines': [
                ['Slave_SQL_Running_{0}'.format(channel_name), 'sql_running', 'absolute'],
                ['Slave_IO_Running_{0}'.format(channel_name), 'io_running', 'absolute']
            ]
        },
    }

    return order, charts


def userstats_chart_template(name):
    order = [
        'userstats_rows_{0}'.format(name),
        'userstats_commands_{0}'.format(name)
    ]
    family = 'userstats {0}'.format(name)

    charts = {
        order[0]: {
            'options': [None, 'Rows Operations', 'operations/s', family, 'mysql.userstats_rows', 'stacked'],
            'lines': [
                ['userstats_{0}_Rows_read'.format(name), 'read', 'incremental'],
                ['userstats_{0}_Rows_send'.format(name), 'send', 'incremental'],
                ['userstats_{0}_Rows_updated'.format(name), 'updated', 'incremental'],
                ['userstats_{0}_Rows_inserted'.format(name), 'inserted', 'incremental'],
                ['userstats_{0}_Rows_deleted'.format(name), 'deleted', 'incremental']
            ]
        },
        order[1]: {
            'options': [None, 'Commands', 'commands/s', family, 'mysql.userstats_commands', 'stacked'],
            'lines': [
                ['userstats_{0}_Select_commands'.format(name), 'select', 'incremental'],
                ['userstats_{0}_Update_commands'.format(name), 'update', 'incremental'],
                ['userstats_{0}_Other_commands'.format(name), 'other', 'incremental']
            ]
        }
    }

    return order, charts


# https://dev.mysql.com/doc/refman/8.0/en/replication-channels.html
DEFAULT_REPL_CHANNEL = ''


# Write Set REPlication
# https://galeracluster.com/library/documentation/galera-status-variables.html
# https://www.percona.com/doc/percona-xtradb-cluster/LATEST/wsrep-status-index.html
class WSRepDataConverter:
    unknown_value = -1

    def convert(self, key, value):
        if key == 'wsrep_connected':
            return self.convert_connected(value)
        elif key == 'wsrep_ready':
            return self.convert_ready(value)
        elif key == 'wsrep_cluster_status':
            return self.convert_cluster_status(value)
        return value

    def convert_connected(self, value):
        # https://www.percona.com/doc/percona-xtradb-cluster/LATEST/wsrep-status-index.html#wsrep_connected
        if value == 'OFF':
            return 0
        if value == 'ON':
            return 1
        return self.unknown_value

    def convert_ready(self, value):
        # https://www.percona.com/doc/percona-xtradb-cluster/LATEST/wsrep-status-index.html#wsrep_ready
        if value == 'OFF':
            return 0
        if value == 'ON':
            return 1
        return self.unknown_value

    def convert_cluster_status(self, value):
        # https://www.percona.com/doc/percona-xtradb-cluster/LATEST/wsrep-status-index.html#wsrep_cluster_status
        # https://github.com/codership/wsrep-API/blob/eab2d5d5a31672c0b7d116ef1629ff18392fd7d0/wsrep_api.h
        # typedef enum wsrep_view_status {
        #     WSREP_VIEW_PRIMARY,      //!< primary group configuration (quorum present)
        #     WSREP_VIEW_NON_PRIMARY,  //!< non-primary group configuration (quorum lost)
        #     WSREP_VIEW_DISCONNECTED, //!< not connected to group, retrying.
        #     WSREP_VIEW_MAX
        # } wsrep_view_status_t;
        value = value.lower()
        if value == 'primary':
            return 0
        elif value == 'non-primary':
            return 1
        elif value == 'disconnected':
            return 2
        return self.unknown_value


wsrep_converter = WSRepDataConverter()


class Service(MySQLService):
    def __init__(self, configuration=None, name=None):
        MySQLService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.queries = dict(
            global_status=QUERY_GLOBAL,
            slave_status=QUERY_SLAVE,
            variables=QUERY_VARIABLES,
            user_statistics=QUERY_USER_STATISTICS,
        )
        self.repl_channels = [DEFAULT_REPL_CHANNEL]

    def _get_data(self):

        raw_data = self._get_raw_data(description=True)

        if not raw_data:
            return None

        data = dict()

        if 'global_status' in raw_data:
            global_status = self.get_global_status(raw_data['global_status'])
            if global_status:
                data.update(global_status)

        if 'slave_status' in raw_data:
            status = self.get_slave_status(raw_data['slave_status'])
            if status:
                data.update(status)

        if 'user_statistics' in raw_data:
            if raw_data['user_statistics'][0]:
                data.update(self.get_userstats(raw_data))
            else:
                self.queries.pop('user_statistics')

        if 'variables' in raw_data:
            variables = dict(raw_data['variables'][0])
            for key in VARIABLES:
                if key in variables:
                    data[key] = variables[key]

        return data or None

    @staticmethod
    def convert_wsrep(key, value):
        return wsrep_converter.convert(key, value)

    def get_global_status(self, raw_global_status):
        # (
        #     (
        #         ('Aborted_clients', '18'),
        #         ('Aborted_connects', '33'),
        #         ('Access_denied_errors', '80'),
        #         ('Acl_column_grants', '0'),
        #         ('Acl_database_grants', '0'),
        #         ('Acl_function_grants', '0'),
        #         ('wsrep_ready', 'OFF'),
        #         ('wsrep_rollbacker_thread_count', '0'),
        #         ('wsrep_thread_count', '0')
        #     ),
        #     (
        #         ('Variable_name', 253, 60, 64, 64, 0, 0),
        #         ('Value', 253, 48, 2048, 2048, 0, 0),
        #     )
        # )
        rows = raw_global_status[0]
        if not rows:
            return

        global_status = dict(rows)
        data = dict()

        for key in GLOBAL_STATS:
            if key not in global_status:
                continue
            value = global_status[key]
            data[key] = value

        for key in GALERA_STATS:
            if key not in global_status:
                continue
            value = global_status[key]
            value = self.convert_wsrep(key, value)
            data[key] = value

        if 'Threads_created' in data and 'Connections' in data:
            data['Thread_cache_misses'] = round(int(data['Threads_created']) / float(data['Connections']) * 10000)
        return data

    def get_slave_status(self, slave_status_data):
        rows, description = slave_status_data[0], slave_status_data[1]
        description_keys = [v[0] for v in description]
        if not rows:
            return

        data = dict()
        for row in rows:
            slave_data = dict(zip(description_keys, row))
            channel_name = slave_data.get('Channel_Name', DEFAULT_REPL_CHANNEL)

            if channel_name not in self.repl_channels and len(self.charts) > 0:
                self.add_repl_channel_charts(channel_name)
                self.repl_channels.append(channel_name)

            for key, func in SLAVE_STATS:
                if key not in slave_data:
                    continue

                value = slave_data[key]
                if channel_name:
                    key = '{0}_{1}'.format(key, channel_name)
                data[key] = func(value)

        return data

    def add_repl_channel_charts(self, name):
        self.add_new_charts(slave_status_chart_template, name)

    def get_userstats(self, raw_data):
        # (
        #     (
        #         ('netdata', 1L, 0L, 60L, 0.15842499999999984, 0.15767439999999996, 5206L, 963957L, 0L, 0L,
        #          61L, 0L, 0L, 0L, 0L, 0L, 62L, 0L, 0L, 0L, 0L, 0L, 0L, 0L, 0L),
        #     ),
        #     (
        #         ('User', 253, 7, 128, 128, 0, 0),
        #         ('Total_connections', 3, 2, 11, 11, 0, 0),
        #         ('Concurrent_connections', 3, 1, 11, 11, 0, 0),
        #         ('Connected_time', 3, 2, 11, 11, 0, 0),
        #         ('Busy_time', 5, 20, 21, 21, 31, 0),
        #         ('Cpu_time', 5, 20, 21, 21, 31, 0),
        #         ('Bytes_received', 8, 4, 21, 21, 0, 0),
        #         ('Bytes_sent', 8, 6, 21, 21, 0, 0),
        #         ('Binlog_bytes_written', 8, 1, 21, 21, 0, 0),
        #         ('Rows_read', 8, 1, 21, 21, 0, 0),
        #         ('Rows_sent', 8, 2, 21, 21, 0, 0),
        #         ('Rows_deleted', 8, 1, 21, 21, 0, 0),
        #         ('Rows_inserted', 8, 1, 21, 21, 0, 0),
        #         ('Rows_updated', 8, 1, 21, 21, 0, 0),
        #         ('Select_commands', 8, 2, 21, 21, 0, 0),
        #         ('Update_commands', 8, 1, 21, 21, 0, 0),
        #         ('Other_commands', 8, 2, 21, 21, 0, 0),
        #         ('Commit_transactions', 8, 1, 21, 21, 0, 0),
        #         ('Rollback_transactions', 8, 1, 21, 21, 0, 0),
        #         ('Denied_connections', 8, 1, 21, 21, 0, 0),
        #         ('Lost_connections', 8, 1, 21, 21, 0, 0),
        #         ('Access_denied', 8, 1, 21, 21, 0, 0),
        #         ('Empty_queries', 8, 2, 21, 21, 0, 0),
        #         ('Total_ssl_connections', 8, 1, 21, 21, 0, 0),
        #         ('Max_statement_time_exceeded', 8, 1, 21, 21, 0, 0)
        #     )
        # )
        data = dict()
        userstats_vars = [e[0] for e in raw_data['user_statistics'][1]]
        for i, _ in enumerate(raw_data['user_statistics'][0]):
            user_name = raw_data['user_statistics'][0][i][0]
            userstats = dict(zip(userstats_vars, raw_data['user_statistics'][0][i]))

            if len(self.charts) > 0:
                if ('userstats_{0}_Cpu_time'.format(user_name)) not in self.charts['userstats_cpu']:
                    self.add_userstats_dimensions(user_name)
                    self.create_new_userstats_charts(user_name)

            for key in USER_STATISTICS:
                if key in userstats:
                    data['userstats_{0}_{1}'.format(user_name, key)] = userstats[key]

        return data

    def add_userstats_dimensions(self, name):
        self.charts['userstats_cpu'].add_dimension(['userstats_{0}_Cpu_time'.format(name), name, 'incremental', 100, 1])

    def create_new_userstats_charts(self, tube):
        self.add_new_charts(userstats_chart_template, tube)

    def add_new_charts(self, template, *params):
        order, charts = template(*params)

        for chart_name in order:
            params = [chart_name] + charts[chart_name]['options']
            dimensions = charts[chart_name]['lines']

            new_chart = self.charts.add_chart(params)
            for dimension in dimensions:
                new_chart.add_dimension(dimension)
