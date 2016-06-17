# Description: MySQL netdata python.d plugin
# Author: Pawel Krupa (paulfantom)

NAME = "mysql.chart.py"
import sys

sys.path.append("./python_modules") #FIXME it is here for debug only

# import 3rd party library to handle MySQL communication
try:
    import MySQLdb
    # https://github.com/PyMySQL/mysqlclient-python
    sys.stderr.write(NAME + ": using MySQLdb\n")
except ImportError:
    try:
        import pymysql as MySQLdb
        # https://github.com/PyMySQL/PyMySQL
        sys.stderr.write(NAME + ": using pymysql\n")
    except ImportError:
        sys.stderr.write(NAME + ": You need to install PyMySQL module to use mysql.chart.py plugin\n")

from base import BaseService

# default configuration (overriden by python.d.plugin)
# FIXME change password
config = [
    {
        'name'     : 'local',
        'user'     : 'root',
        'password' : 'a',
        'socket'   : '/var/run/mysqld/mysqld.sock',
        'update_every' : 3,
        'retries'  : 4
    }
]

# query executed on MySQL server
QUERY = "SHOW GLOBAL STATUS"

# charts order (can be overriden if you want less charts, or different order)
ORDER = ['net', 
         'queries', 
         'handlers', 
         'table_locks', 
         'join_issues', 
         'sort_issues', 
         'tmp', 
         'connections', 
         'binlog_cache', 
         'threads', 
         'thread_cache_misses', 
         'innodb_io', 
         'innodb_io_ops',
         'innodb_io_pending_ops',
         'innodb_log',
         'innodb_os_log',
         'innodb_os_log_io',
         'innodb_cur_row_lock',
         'innodb_rows',
         'innodb_buffer_pool_pages',
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
         'binlog_stmt_cache',
         'connection_errors']

# charts definitions in format:
# CHARTS = {
#    'chart_name_in_netdata': (
#        "parameters defining chart (passed to CHART statement)",
#        [ # dimensions (lines) definitions
#            ("dimension_name", "dimension parameters (passed to DIMENSION statement)", "additional parameter (optional)")
#        ])
#    }

CHARTS = {
    'net' : (
        "'' 'mysql Bandwidth' 'kilobits/s' bandwidth mysql.net area",
        [
            ("Bytes_received", "in incremental 8 1024"),
            ("Bytes_sent",     "out incremental -8 1024")
        ]),
    'queries' : (
        "'' 'mysql Queries' 'queries/s' queries mysql.queries line",
        [
            ("Queries",      "queries incremental 1 1"),
            ("Questions",    "questions incremental 1 1"),
            ("Slow_queries", "slow_queries incremental -1 1")
        ]),
    'handlers' : (
        "'' 'mysql Handlers' 'handlers/s' handlers mysql.handlers line",
        [
            ("Handler_commit", "commit incremental 1 1"),
            ("Handler_delete", "delete incremental 1 1"),
            ("Handler_prepare", "prepare incremental 1 1"),
            ("Handler_read_first", "read_first incremental 1 1"),
            ("Handler_read_key", "read_key incremental 1 1"),
            ("Handler_read_next", "read_next incremental 1 1"),
            ("Handler_read_prev", "read_prev incremental 1 1"),
            ("Handler_read_rnd", "read_rnd incremental 1 1"),
            ("Handler_read_rnd_next", "read_rnd_next incremental 1 1"),
            ("Handler_rollback", "rollback incremental 1 1"),
            ("Handler_savepoint", "savepoint incremental 1 1"),
            ("Handler_savepoint_rollback", "savepoint_rollback incremental 1 1"),
            ("Handler_update", "update incremental 1 1"),
            ("Handler_write", "write incremental 1 1")
        ]),
    'table_locks' : (
        "'' 'mysql Tables Locks' 'locks/s' locks mysql.table_locks line",
        [
            ("Table_locks_immediate", "immediate incremental 1 1"),
            ("Table_locks_waited", "waited incremental -1 1")
        ]),
    'join_issues' : (
        "'' 'mysql Select Join Issues' 'joins/s' issues mysql.join_issues line",
        [
            ("Select_full_join", "full_join incremental 1 1"),
            ("Select_full_range_join", "full_range_join incremental 1 1"),
            ("Select_range", "range incremental 1 1"),
            ("Select_range_check", "range_check incremental 1 1"),
            ("Select_scan", "scan incremental 1 1"),
        ]),
    'sort_issues' : (
        "'' 'mysql Sort Issues' 'issues/s' issues mysql.sort.issues line",
        [
            ("Sort_merge_passes", "merge_passes incremental 1 1"),
            ("Sort_range", "range incremental 1 1"),
            ("Sort_scan", "scan incremental 1 1"),
        ]),
    'tmp' : (
        "'' 'mysql Tmp Operations' 'counter' temporaries mysql.tmp line",
        [
            ("Created_tmp_disk_tables", "disk_tables incremental 1 1"),
            ("Created_tmp_files", "files incremental 1 1"),
            ("Created_tmp_tables", "tables incremental 1 1"),
        ]),
    'connections' : (
        "'' 'mysql Connections' 'connections/s' connections mysql.connections line",
        [
            ("Connections", "all incremental 1 1"),
            ("Aborted_connects", "aborted incremental 1 1"),
        ]),
    'binlog_cache' : (
        "'' 'mysql Binlog Cache' 'transactions/s' binlog mysql.binlog_cache line",
        [
            ("Binlog_cache_disk_use", "disk incremental 1 1"),
            ("Binlog_cache_use", "all incremental 1 1"),
        ]),
    'threads' : (
        "'' 'mysql Threads' 'threads' threads mysql.threads line",
        [
            ("Threads_connected", "connected absolute 1 1"),
            ("Threads_created", "created incremental 1 1"),
            ("Threads_cached", "cached absolute -1 1"),
            ("Threads_running", "running absolute 1 1"),
        ]),
    'thread_cache_misses' : (
        "'' 'mysql Threads Cache Misses' 'misses' threads mysql.thread_cache_misses area",
        [
            ("Thread_cache_misses", "misses misses absolute 1 100"),
        ]),
    'innodb_io' : (
        "'' 'mysql InnoDB I/O Bandwidth' 'kilobytes/s' innodb mysql.innodb_io area",
        [
            ("Innodb_data_read", "read incremental 1 1024"),
            ("Innodb_data_written", "write incremental -1 1024"),
        ]),
    'innodb_io_ops' : (
        "'' 'mysql InnoDB I/O Operations' 'operations/s' innodb mysql.innodb_io_ops line",
        [
            ("Innodb_data_reads", "reads incremental 1 1"),
            ("Innodb_data_writes", "writes incremental -1 1"),
            ("Innodb_data_fsyncs", "fsyncs incremental 1 1"),
        ]),
    'innodb_io_pending_ops' : (
        "'' 'mysql InnoDB Pending I/O Operations' 'operations' innodb mysql.innodb_io_pending_ops line",
        [
            ("Innodb_data_pending_reads", "reads absolute 1 1"),
            ("Innodb_data_pending_writes", "writes absolute -1 1"),
            ("Innodb_data_pending_fsyncs", "fsyncs absolute 1 1"),
        ]),
    'innodb_log' : (
        "'' 'mysql InnoDB Log Operations' 'operations/s' innodb mysql.innodb_log line",
        [
            ("Innodb_log_waits", "waits incremental 1 1"),
            ("Innodb_log_write_requests", "write_requests incremental -1 1"),
            ("Innodb_log_writes", "incremental -1 1"),
        ]),
    'innodb_os_log' : (
        "'' 'mysql InnoDB OS Log Operations' 'operations' innodb mysql.innodb_os_log line",
        [
            ("Innodb_os_log_fsyncs", "fsyncs incremental 1 1"),
            ("Innodb_os_log_pending_fsyncs", "pending_fsyncs absolute 1 1"),
            ("Innodb_os_log_pending_writes", "pending_writes absolute -1 1"),
        ]),
    'innodb_os_log_io' : (
        "'' 'mysql InnoDB OS Log Bandwidth' 'kilobytes/s' innodb mysql.innodb_os_log_io area",
        [
            ("Innodb_os_log_written", "write incremental -1 1024"),
        ]),
    'innodb_cur_row_lock' : (
        "'' 'mysql InnoDB Current Row Locks' 'operations' innodb mysql.innodb_cur_row_lock area",
        [
            ("Innodb_row_lock_current_waits", "current_waits absolute 1 1"),
        ]),
    'innodb_rows' : (
        "'' 'mysql InnoDB Row Operations' 'operations/s' innodb mysql.innodb_rows area",
        [
            ("Innodb_rows_inserted", "read incremental 1 1"),
            ("Innodb_rows_read", "deleted incremental -1 1"),
            ("Innodb_rows_updated", "inserted incremental 1 1"),
            ("Innodb_rows_deleted", "updated incremental -1 1"),
        ]),
    'innodb_buffer_pool_pages' : (
        "'' 'mysql InnoDB Buffer Pool Pages' 'pages' innodb mysql.innodb_buffer_pool_pages line",
        [
            ("Innodb_buffer_pool_pages_data", "data absolute 1 1"),
            ("Innodb_buffer_pool_pages_dirty", "dirty absolute -1 1"),
            ("Innodb_buffer_pool_pages_free", "free absolute 1 1"),
            ("Innodb_buffer_pool_pages_flushed", "flushed incremental -1 1"),
            ("Innodb_buffer_pool_pages_misc", "misc absolute -1 1"),
            ("Innodb_buffer_pool_pages_total", "total absolute 1 1"),
        ]),
    'innodb_buffer_pool_bytes' : (
        "'' 'mysql InnoDB Buffer Pool Bytes' 'MB' innodb mysql.innodb_buffer_pool_bytes area",
        [
            ("Innodb_buffer_pool_bytes_data", "data absolute 1"),
            ("Innodb_buffer_pool_bytes_dirty", "dirty absolute -1"),
        ]),
    'innodb_buffer_pool_read_ahead' : (
        "'' 'mysql InnoDB Buffer Pool Read Ahead' 'operations/s' innodb mysql.innodb_buffer_pool_read_ahead area",
        [
            ("Innodb_buffer_pool_read_ahead", "all incremental 1 1"),
            ("Innodb_buffer_pool_read_ahead_evicted", "evicted incremental -1 1"),
            ("Innodb_buffer_pool_read_ahead_rnd", "random incremental 1 1"),
        ]),
    'innodb_buffer_pool_reqs' : (
        "'' 'mysql InnoDB Buffer Pool Requests' 'requests/s' innodb mysql.innodb_buffer_pool_reqs area",
        [
            ("Innodb_buffer_pool_read_requests", "reads incremental 1 1"),
            ("Innodb_buffer_pool_write_requests", "writes incremental -1 1"),
        ]),
    'innodb_buffer_pool_ops' : (
        "'' 'mysql InnoDB Buffer Pool Operations' 'operations/s' innodb mysql.innodb_buffer_pool_ops area",
        [
            ("Innodb_buffer_pool_reads", "'disk reads' incremental 1 1"),
            ("Innodb_buffer_pool_wait_free", "'wait free' incremental -1 1"),
        ]),
    'qcache_ops' : (
        "'' 'mysql QCache Operations' 'queries/s' qcache mysql.qcache_ops line",
        [
            ("Qcache_hits", "hits incremental 1 1"),
            ("Qcache_lowmem_prunes", "'lowmem prunes' incremental -1 1"),
            ("Qcache_inserts", "inserts incremental 1 1"),
            ("Qcache_not_cached", "'not cached' incremental -1 1"),
        ]),
    'qcache' : (
        "'' 'mysql QCache Queries in Cache' 'queries' qcache mysql.qcache line",
        [
            ("Qcache_queries_in_cache", "queries absolute 1 1"),
        ]),
    'qcache_freemem' : (
        "'' 'mysql QCache Free Memory' 'MB' qcache mysql.qcache_freemem area",
        [
            ("Qcache_free_memory", "free absolute 1"),
        ]),
    'qcache_memblocks' : (
        "'' 'mysql QCache Memory Blocks' 'blocks' qcache mysql.qcache_memblocks line",
        [
            ("Qcache_free_blocks", "free absolute 1"),
            ("Qcache_total_blocks", "total absolute 1 1"),
        ]),
    'key_blocks' : (
        "'' 'mysql MyISAM Key Cache Blocks' 'blocks' myisam mysql.key_blocks line",
        [
            ("Key_blocks_unused", "unused absolute 1 1"),
            ("Key_blocks_used", "used absolute -1 1"),
            ("Key_blocks_not_flushed", "'not flushed' absolute 1 1"),
        ]),
    'key_requests' : (
        "'' 'mysql MyISAM Key Cache Requests' 'requests/s' myisam mysql.key_requests area",
        [
            ("Key_read_requests", "reads incremental 1 1"),
            ("Key_write_requests", "writes incremental -1 1"),
        ]),
    'key_disk_ops' : (
        "'' 'mysql MyISAM Key Cache Disk Operations' 'operations/s' myisam mysql.key_disk_ops area",
        [
            ("Key_reads", "reads incremental 1 1"),
            ("Key_writes", "writes incremental -1 1"),
        ]),
    'files' : (
        "'' 'mysql Open Files' 'files' files mysql.files line",
        [
            ("Open_files", "files absolute 1 1"),
        ]),
    'files_rate' : (
        "'' 'mysql Opened Files Rate' 'files/s' files mysql.files_rate line",
        [
            ("Opened_files", "files incremental 1 1"),
        ]),
    'binlog_stmt_cache' : (
        "'' 'mysql Binlog Statement Cache' 'statements/s' binlog mysql.binlog_stmt_cache line",
        [
            ("Binlog_stmt_cache_disk_use", "disk incremental 1 1"),
            ("Binlog_stmt_cache_use", "all incremental 1 1"),
        ]),
    'connection_errors' : (
        "'' 'mysql Connection Errors' 'connections/s' connections mysql.connection_errors line",
        [
            ("Connection_errors_accept", "accept incremental 1 1"),
            ("Connection_errors_internal", "internal incremental 1 1"),
            ("Connection_errors_max_connections", "max incremental 1 1"),
            ("Connection_errors_peer_address", "peer_addr incremental 1 1"),
            ("Connection_errors_select", "select incremental 1 1"),
            ("Connection_errors_tcpwrap", "tcpwrap incremental 1 1")
        ])
}


class Service(BaseService):
    def __init__(self,configuration=None,update_every=3,priority=90000,retries=2):
        super().__init__(configuration,update_every,priority,retries)
        self.configuration = self._parse_config(configuration)
        self.connection = None
        self.defs = {}

    def _parse_config(self,configuration):
        # parse configuration to collect data from mysql server
        if 'name' not in configuration:
            configuration['name'] = 'local'
        if 'user' not in configuration:
            configuration['user'] = 'root'
        if 'password' not in configuration:
            configuration['password'] = ''
        if 'my.cnf' in configuration:
            configuration['socket'] = ''
            configuration['host'] = ''
            configuration['port'] = 0
        elif 'socket' in configuration:
            configuration['my.cnf'] = ''
            configuration['host'] = ''
            configuration['port'] = 0
        elif 'host' in configuration:
            configuration['my.cnf'] = ''
            configuration['socket'] = ''
            if 'port' in configuration:
                configuration['port'] = int(configuration['port'])
            else:
                configuration['port'] = 3306

        return configuration

    def _connect(self):
        try:
            self.connection = MySQLdb.connect(user=self.configuration['user'],
                                              passwd=self.configuration['password'],
                                              read_default_file=self.configuration['my.cnf'],
                                              unix_socket=self.configuration['socket'],
                                              host=self.configuration['host'],
                                              port=self.configuration['port'],
                                              connect_timeout=self.configuration['update_every'])
        except Exception as e:
            self.error(NAME + " has problem connecting to server:", e)
            raise RuntimeError #stop creating module, need to catch it in supervisor
    
    def _get_data(self):
        if self.connection is None:
            self._connect()
        try:
            with self.connection.cursor() as cursor:
                cursor.execute(QUERY)
                raw_data = cursor.fetchall()
        except Exception as e:
            self.error(NAME + ": cannot execute query.", e)
            self.connection.close()
            self.connection = None
            return None
        
        return dict(raw_data)

    def check(self):
        try:
            self.connection = self._connect()
            return True
        except RuntimeError:
            self.connection = None
            return False

    def create(self):
        for name in ORDER:
            print(name)
            self.defs[name] = []
            for line in CHARTS[name][1]:
                self.defs[name].append(line[0])
   
        idx = 0
        data = self._get_data()
        for name in ORDER:
            header = "CHART mysql_" + \
                     str(self.configuration['name']) + "." + \
                     name + " " + \
                     CHARTS[name][0] + " " + \
                     str(self.priority + idx) + " " + \
                     str(self.update_every)
            content = ""
            # check if server has this datapoint
            for line in CHARTS[name][1]:
                if line[0] in data:
                     content += "DIMENSION " + line[0] + " " + line[1] + "\n"
            if len(content) > 0:
                print(header)
                print(content)
                idx += 1
    
        if idx == 0:
            return False
        return True

    def update(self,interval):
        data = self._get_data()
        if data is None:
            return False
        try:
            data['Thread cache misses'] = int( int(data['Threads_created']) * 10000 / int(data['Connections']))
        except Exception:
            pass
        for chart, dimensions in self.defs.items():
            header = "BEGIN mysql_" + str(self.configuration['name']) + "." + chart + " " + str(interval) + '\n'
            lines = ""
            for d in dimensions:
                try:
                    lines += "SET " + d + " = " + data[d] + '\n'
                except KeyError:
                    pass
            if len(lines) > 0:
                print(header + lines + "END")
        
        return True

#FIXME debug only:
if __name__ == "__main__":
    my = Service(config[0])
    my.check()
    my.create()
    my.update(1)
