# -*- coding: utf-8 -*-
# Description: MySQL netdata python.d plugin
# Author: Pawel Krupa (paulfantom)

from base import SimpleService
import msg

# import 3rd party library to handle MySQL communication
try:
    import MySQLdb

    # https://github.com/PyMySQL/mysqlclient-python
    msg.info("using MySQLdb")
except ImportError:
    try:
        import pymysql as MySQLdb

        # https://github.com/PyMySQL/PyMySQL
        msg.info("using pymysql")
    except ImportError:
        msg.error("MySQLdb or PyMySQL module is needed to use mysql.chart.py plugin")
        raise ImportError

# default module values (can be overridden per job in `config`)
update_every = 3
priority = 90000
retries = 7

# default configuration (overridden by python.d.plugin)
config = {
    'local': {
        'user': 'root',
        'password': '',
        'socket': '/var/run/mysqld/mysqld.sock',
        'update_every': update_every,
        'retries': retries,
        'priority': priority
    }
}

# query executed on MySQL server
QUERY = "SHOW GLOBAL STATUS"

ORDER = ['net',
         'queries',
         'handlers',
         'table_locks',
         'join_issues', 'sort_issues',
         'tmp',
         'connections', 'connection_errors',
         'binlog_cache', 'binlog_stmt_cache',
         'threads', 'thread_cache_misses',
         'innodb_io', 'innodb_io_ops', 'innodb_io_pending_ops', 'innodb_log', 'innodb_os_log', 'innodb_os_log_io',
         'innodb_cur_row_lock', 'innodb_rows', 'innodb_buffer_pool_pages', 'innodb_buffer_pool_bytes',
         'innodb_buffer_pool_read_ahead', 'innodb_buffer_pool_reqs', 'innodb_buffer_pool_ops',
         'qcache_ops', 'qcache', 'qcache_freemem', 'qcache_memblocks',
         'key_blocks', 'key_requests', 'key_disk_ops',
         'files', 'files_rate']

CHARTS = {
    'net': {
        'options': [None, 'mysql Bandwidth', 'kilobits/s', 'bandwidth', 'mysql.net', 'area'],
        'lines': [
            ["Bytes_received", "in", "incremental", 8, 1024],
            ["Bytes_sent", "out", "incremental", -8, 1024]
        ]},
    'queries': {
        'options': [None, 'mysql Queries', 'queries/s', 'queries', 'mysql.queries', 'line'],
        'lines': [
            ["Queries", "queries"],
            ["Questions", "questions"],
            ["Slow_queries", "slow_queries"]
        ]},
    'handlers': {
        'options': [None, 'mysql Handlers', 'handlers/s', 'handlers', 'mysql.handlers', 'line'],
        'lines': [
            ["Handler_commit", "commit"],
            ["Handler_delete", "delete"],
            ["Handler_prepare", "prepare"],
            ["Handler_read_first", "read_first"],
            ["Handler_read_key", "read_key"],
            ["Handler_read_next", "read_next"],
            ["Handler_read_prev", "read_prev"],
            ["Handler_read_rnd", "read_rnd"],
            ["Handler_read_rnd_next", "read_rnd_next"],
            ["Handler_rollback", "rollback"],
            ["Handler_savepoint", "savepoint"],
            ["Handler_savepoint_rollback", "savepoint_rollback"],
            ["Handler_update", "update"],
            ["Handler_write", "write"]
        ]},
    'table_locks': {
        'options': [None, 'mysql Tables Locks', 'locks/s', 'locks', 'mysql.table_locks', 'line'],
        'lines': [
            ["Table_locks_immediate", "immediate"],
            ["Table_locks_waited", "waited", "incremental", -1, 1]
        ]},
    'join_issues': {
        'options': [None, 'mysql Select Join Issues', 'joins/s', 'issues', 'mysql.join_issues', 'line'],
        'lines': [
            ["Select_full_join", "full_join"],
            ["Select_full_range_join", "full_range_join"],
            ["Select_range", "range"],
            ["Select_range_check", "range_check"],
            ["Select_scan", "scan"]
        ]},
    'sort_issues': {
        'options': [None, 'mysql Sort Issues', 'issues/s', 'issues', 'mysql.sort.issues', 'line'],
        'lines': [
            ["Sort_merge_passes", "merge_passes"],
            ["Sort_range", "range"],
            ["Sort_scan", "scan"]
        ]},
    'tmp': {
        'options': [None, 'mysql Tmp Operations', 'counter', 'temporaries', 'mysql.tmp', 'line'],
        'lines': [
            ["Created_tmp_disk_tables", "disk_tables"],
            ["Created_tmp_files", "files"],
            ["Created_tmp_tables", "tables"]
        ]},
    'connections': {
        'options': [None, 'mysql Connections', 'connections/s', 'connections', 'mysql.connections', 'line'],
        'lines': [
            ["Connections", "all"],
            ["Aborted_connects", "aborted"]
        ]},
    'binlog_cache': {
        'options': [None, 'mysql Binlog Cache', 'transactions/s', 'binlog', 'mysql.binlog_cache', 'line'],
        'lines': [
            ["Binlog_cache_disk_use", "disk"],
            ["Binlog_cache_use", "all"]
        ]},
    'threads': {
        'options': [None, 'mysql Threads', 'threads', 'threads', 'mysql.threads', 'line'],
        'lines': [
            ["Threads_connected", "connected", "absolute"],
            ["Threads_created", "created"],
            ["Threads_cached", "cached", "absolute", -1, 1],
            ["Threads_running", "running", "absolute"],
        ]},
    'thread_cache_misses': {
        'options': [None, 'mysql Threads Cache Misses', 'misses', 'threads', 'mysql.thread_cache_misses', 'area'],
        'lines': [
            ["Thread_cache_misses", "misses", "misses", "absolute", 1, 100]
        ]},
    'innodb_io': {
        'options': [None, 'mysql InnoDB I/O Bandwidth', 'kilobytes/s', 'innodb', 'mysql.innodb_io', 'area'],
        'lines': [
            ["Innodb_data_read", "read", "incremental", 1, 1024],
            ["Innodb_data_written", "write", "incremental", -1, 1024]
        ]},
    'innodb_io_ops': {
        'options': [None, 'mysql InnoDB I/O Operations', 'operations/s', 'innodb', 'mysql.innodb_io_ops', 'line'],
        'lines': [
            ["Innodb_data_reads", "reads"],
            ["Innodb_data_writes", "writes", "incremental", -1, 1],
            ["Innodb_data_fsyncs", "fsyncs"]
        ]},
    'innodb_io_pending_ops': {
        'options': [None, 'mysql InnoDB Pending I/O Operations', 'operations', 'innodb', 'mysql.innodb_io_pending_ops', 'line'],
        'lines': [
            ["Innodb_data_pending_reads", "reads", "absolute"],
            ["Innodb_data_pending_writes", "writes", "absolute", -1, 1],
            ["Innodb_data_pending_fsyncs", "fsyncs"]
        ]},
    'innodb_log': {
        'options': [None, 'mysql InnoDB Log Operations', 'operations/s', 'innodb', 'mysql.innodb_log', 'line'],
        'lines': [
            ["Innodb_log_waits", "waits"],
            ["Innodb_log_write_requests", "write_requests", "incremental", -1, 1],
            ["Innodb_log_writes", "incremental", -1, 1],
        ]},
    'innodb_os_log': {
        'options': [None, 'mysql InnoDB OS Log Operations', 'operations', 'innodb', 'mysql.innodb_os_log', 'line'],
        'lines': [
            ["Innodb_os_log_fsyncs", "fsyncs"],
            ["Innodb_os_log_pending_fsyncs", "pending_fsyncs", "absolute"],
            ["Innodb_os_log_pending_writes", "pending_writes", "absolute", -1, 1],
        ]},
    'innodb_os_log_io': {
        'options': [None, 'mysql InnoDB OS Log Bandwidth', 'kilobytes/s', 'innodb', 'mysql.innodb_os_log_io', 'area'],
        'lines': [
            ["Innodb_os_log_written", "write", "incremental", -1, 1024],
        ]},
    'innodb_cur_row_lock': {
        'options': [None, 'mysql InnoDB Current Row Locks', 'operations', 'innodb', 'mysql.innodb_cur_row_lock', 'area'],
        'lines': [
            ["Innodb_row_lock_current_waits", "current_waits", "absolute"]
        ]},
    'innodb_rows': {
        'options': [None, 'mysql InnoDB Row Operations', 'operations/s', 'innodb', 'mysql.innodb_rows', 'area'],
        'lines': [
            ["Innodb_rows_inserted", "read"],
            ["Innodb_rows_read", "deleted", "incremental", -1, 1],
            ["Innodb_rows_updated", "inserted", "incremental", 1, 1],
            ["Innodb_rows_deleted", "updated", "incremental", -1, 1],
        ]},
    'innodb_buffer_pool_pages': {
        'options': [None, 'mysql InnoDB Buffer Pool Pages', 'pages', 'innodb', 'mysql.innodb_buffer_pool_pages', 'line'],
        'lines': [
            ["Innodb_buffer_pool_pages_data", "data", "absolute"],
            ["Innodb_buffer_pool_pages_dirty", "dirty", "absolute", -1, 1],
            ["Innodb_buffer_pool_pages_free", "free", "absolute"],
            ["Innodb_buffer_pool_pages_flushed", "flushed", "incremental", -1, 1],
            ["Innodb_buffer_pool_pages_misc", "misc", "absolute", -1, 1],
            ["Innodb_buffer_pool_pages_total", "total", "absolute"]
        ]},
    'innodb_buffer_pool_bytes': {
        'options': [None, 'mysql InnoDB Buffer Pool Bytes', 'MB', 'innodb', 'mysql.innodb_buffer_pool_bytes', 'area'],
        'lines': [
            ["Innodb_buffer_pool_bytes_data", "data", "absolute"],
            ["Innodb_buffer_pool_bytes_dirty", "dirty", "absolute", -1, 1]
        ]},
    'innodb_buffer_pool_read_ahead': {
        'options': [None, 'mysql InnoDB Buffer Pool Read Ahead', 'operations/s', 'innodb', 'mysql.innodb_buffer_pool_read_ahead', 'area'],
        'lines': [
            ["Innodb_buffer_pool_read_ahead", "all"],
            ["Innodb_buffer_pool_read_ahead_evicted", "evicted", "incremental", -1, 1],
            ["Innodb_buffer_pool_read_ahead_rnd", "random"]
        ]},
    'innodb_buffer_pool_reqs': {
        'options': [None, 'mysql InnoDB Buffer Pool Requests', 'requests/s', 'innodb', 'mysql.innodb_buffer_pool_reqs', 'area'],
        'lines': [
            ["Innodb_buffer_pool_read_requests", "reads"],
            ["Innodb_buffer_pool_write_requests", "writes", "incremental", -1, 1]
        ]},
    'innodb_buffer_pool_ops': {
        'options': [None, 'mysql InnoDB Buffer Pool Operations', 'operations/s', 'innodb', 'mysql.innodb_buffer_pool_ops', 'area'],
        'lines': [
            ["Innodb_buffer_pool_reads", "disk reads"],
            ["Innodb_buffer_pool_wait_free", "wait free", "incremental", -1, 1]
        ]},
    'qcache_ops': {
        'options': [None, 'mysql QCache Operations', 'queries/s', 'qcache', 'mysql.qcache_ops', 'line'],
        'lines': [
            ["Qcache_hits", "hits"],
            ["Qcache_lowmem_prunes", "lowmem prunes", "incremental", -1, 1],
            ["Qcache_inserts", "inserts"],
            ["Qcache_not_cached", "not cached", "incremental", -1, 1]
        ]},
    'qcache': {
        'options': [None, 'mysql QCache Queries in Cache', 'queries', 'qcache', 'mysql.qcache', 'line'],
        'lines': [
            ["Qcache_queries_in_cache", "queries", "absolute"]
        ]},
    'qcache_freemem': {
        'options': [None, 'mysql QCache Free Memory', 'MB', 'qcache', 'mysql.qcache_freemem', 'area'],
        'lines': [
            ["Qcache_free_memory", "free", "absolute"]
        ]},
    'qcache_memblocks': {
        'options': [None, 'mysql QCache Memory Blocks', 'blocks', 'qcache', 'mysql.qcache_memblocks', 'line'],
        'lines': [
            ["Qcache_free_blocks", "free", "absolute"],
            ["Qcache_total_blocks", "total", "absolute"]
        ]},
    'key_blocks': {
        'options': [None, 'mysql MyISAM Key Cache Blocks', 'blocks', 'myisam', 'mysql.key_blocks', 'line'],
        'lines': [
            ["Key_blocks_unused", "unused", "absolute"],
            ["Key_blocks_used", "used", "absolute", -1, 1],
            ["Key_blocks_not_flushed", "not flushed", "absolute"]
        ]},
    'key_requests': {
        'options': [None, 'mysql MyISAM Key Cache Requests', 'requests/s', 'myisam', 'mysql.key_requests', 'area'],
        'lines': [
            ["Key_read_requests", "reads"],
            ["Key_write_requests", "writes", "incremental", -1, 1]
        ]},
    'key_disk_ops': {
        'options': [None, 'mysql MyISAM Key Cache Disk Operations', 'operations/s', 'myisam', 'mysql.key_disk_ops', 'area'],
        'lines': [
            ["Key_reads", "reads"],
            ["Key_writes", "writes", "incremental", -1, 1]
        ]},
    'files': {
        'options': [None, 'mysql Open Files', 'files', 'files', 'mysql.files', 'line'],
        'lines': [
            ["Open_files", "files", "absolute"]
        ]},
    'files_rate': {
        'options': [None, 'mysql Opened Files Rate', 'files/s', 'files', 'mysql.files_rate', 'line'],
        'lines': [
            ["Opened_files", "files"]
        ]},
    'binlog_stmt_cache': {
        'options': [None, 'mysql Binlog Statement Cache', 'statements/s', 'binlog', 'mysql.binlog_stmt_cache', 'line'],
        'lines': [
            ["Binlog_stmt_cache_disk_use", "disk"],
            ["Binlog_stmt_cache_use", "all"]
        ]},
    'connection_errors': {
        'options': [None, 'mysql Connection Errors', 'connections/s', 'connections', 'mysql.connection_errors', 'line'],
        'lines': [
            ["Connection_errors_accept", "accept"],
            ["Connection_errors_internal", "internal"],
            ["Connection_errors_max_connections", "max"],
            ["Connection_errors_peer_address", "peer_addr"],
            ["Connection_errors_select", "select"],
            ["Connection_errors_tcpwrap", "tcpwrap"]
        ]}

}


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self._parse_config(configuration)
        self.order = ORDER
        self.definitions = CHARTS
        self.connection = None

    def _parse_config(self, configuration):
        """
        Parse configuration to collect data from MySQL server
        :param configuration: dict
        :return: dict
        """
        if self.name is None:
            self.name = 'local'
        if 'user' not in configuration:
            self.configuration['user'] = 'root'
        if 'password' not in configuration:
            self.configuration['password'] = ''
        if 'my.cnf' in configuration:
            self.configuration['socket'] = ''
            self.configuration['host'] = ''
            self.configuration['port'] = 0
        elif 'socket' in configuration:
            self.configuration['my.cnf'] = ''
            self.configuration['host'] = ''
            self.configuration['port'] = 0
        elif 'host' in configuration:
            self.configuration['my.cnf'] = ''
            self.configuration['socket'] = ''
            if 'port' in configuration:
                self.configuration['port'] = int(configuration['port'])
            else:
                self.configuration['port'] = 3306

    def _connect(self):
        """
        Try to connect to MySQL server
        """
        try:
            self.connection = MySQLdb.connect(user=self.configuration['user'],
                                              passwd=self.configuration['password'],
                                              read_default_file=self.configuration['my.cnf'],
                                              unix_socket=self.configuration['socket'],
                                              host=self.configuration['host'],
                                              port=self.configuration['port'],
                                              connect_timeout=self.update_every)
        except Exception as e:
            self.error("problem connecting to server:", e)
            raise RuntimeError

    def _format_data(self):
        """
        Get raw data from MySQL server
        :return: dict
        """
        if self.connection is None:
            try:
                self._connect()
            except RuntimeError:
                return None
        try:
            cursor = self.connection.cursor()
            cursor.execute(QUERY)
            raw_data = cursor.fetchall()
        except Exception as e:
            self.error("cannot execute query.", e)
            self.connection.close()
            self.connection = None
            return None

        return dict(raw_data)

    def check(self):
        """
        Check if service is able to connect to server
        :return: boolean
        """
        try:
            self.connection = self._connect()
            return True
        except RuntimeError:
            self.connection = None
            return False
