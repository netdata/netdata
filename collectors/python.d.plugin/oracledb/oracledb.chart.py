# -*- coding: utf-8 -*-
# Description: oracledb netdata python.d module
# Author: ilyam8 (Ilya Mashchenko)
# SPDX-License-Identifier: GPL-3.0-or-later

from copy import deepcopy

from bases.FrameworkServices.SimpleService import SimpleService

try:
    import cx_Oracle

    HAS_ORACLE = True
except ImportError:
    HAS_ORACLE = False

ORDER = [
    'session_count',
    'session_limit_usage',
    'logons',
    'physical_disk_read_write',
    'sorts_on_disk',
    'full_table_scans',
    'database_wait_time_ratio',
    'shared_pool_free_memory',
    'in_memory_sorts_ratio',
    'sql_service_response_time',
    'user_rollbacks',
    'enqueue_timeouts',
    'cache_hit_ratio',
    'global_cache_blocks',
    'activity',
    'wait_time',
    'tablespace_size',
    'tablespace_usage',
    'tablespace_usage_in_percent',
]

CHARTS = {
    'session_count': {
        'options': [None, 'Session Count', 'sessions', 'session activity', 'oracledb.session_count', 'line'],
        'lines': [
            ['session_count', 'total', 'absolute', 1, 1000],
            ['average_active_sessions', 'active', 'absolute', 1, 1000],
        ]
    },
    'session_limit_usage': {
        'options': [None, 'Session Limit Usage', '%', 'session activity', 'oracledb.session_limit_usage', 'area'],
        'lines': [
            ['session_limit_percent', 'usage', 'absolute', 1, 1000],
        ]
    },
    'logons': {
        'options': [None, 'Logons', 'events/s', 'session activity', 'oracledb.logons', 'area'],
        'lines': [
            ['logons_per_sec', 'logons', 'absolute', 1, 1000],
        ]
    },
    'physical_disk_read_write': {
        'options': [None, 'Physical Disk Reads/Writes', 'events/s', 'disk activity',
                    'oracledb.physical_disk_read_writes', 'area'],
        'lines': [
            ['physical_reads_per_sec', 'reads', 'absolute', 1, 1000],
            ['physical_writes_per_sec', 'writes', 'absolute', -1, 1000],
        ]
    },
    'sorts_on_disk': {
        'options': [None, 'Sorts On Disk', 'events/s', 'disk activity', 'oracledb.sorts_on_disks', 'line'],
        'lines': [
            ['disk_sort_per_sec', 'sorts', 'absolute', 1, 1000],
        ]
    },
    'full_table_scans': {
        'options': [None, 'Full Table Scans', 'events/s', 'disk activity', 'oracledb.full_table_scans', 'line'],
        'lines': [
            ['long_table_scans_per_sec', 'full table scans', 'absolute', 1, 1000],
        ]
    },
    'database_wait_time_ratio': {
        'options': [None, 'Database Wait Time Ratio', '%', 'database and buffer activity',
                    'oracledb.database_wait_time_ratio', 'line'],
        'lines': [
            ['database_wait_time_ratio', 'wait time ratio', 'absolute', 1, 1000],
        ]
    },
    'shared_pool_free_memory': {
        'options': [None, 'Shared Pool Free Memory', '%', 'database and buffer activity',
                    'oracledb.shared_pool_free_memory', 'line'],
        'lines': [
            ['shared_pool_free_percent', 'free memory', 'absolute', 1, 1000],
        ]
    },
    'in_memory_sorts_ratio': {
        'options': [None, 'In-Memory Sorts Ratio', '%', 'database and buffer activity',
                    'oracledb.in_memory_sorts_ratio', 'line'],
        'lines': [
            ['memory_sorts_ratio', 'in-memory sorts', 'absolute', 1, 1000],
        ]
    },
    'sql_service_response_time': {
        'options': [None, 'SQL Service Response Time', 'seconds', 'database and buffer activity',
                    'oracledb.sql_service_response_time', 'line'],
        'lines': [
            ['sql_service_response_time', 'time', 'absolute', 1, 1000],
        ]
    },
    'user_rollbacks': {
        'options': [None, 'User Rollbacks', 'events/s', 'database and buffer activity',
                    'oracledb.user_rollbacks', 'line'],
        'lines': [
            ['user_rollbacks_per_sec', 'rollbacks', 'absolute', 1, 1000],
        ]
    },
    'enqueue_timeouts': {
        'options': [None, 'Enqueue Timeouts', 'events/s', 'database and buffer activity',
                    'oracledb.enqueue_timeouts', 'line'],
        'lines': [
            ['enqueue_timeouts_per_sec', 'enqueue timeouts', 'absolute', 1, 1000],
        ]
    },
    'cache_hit_ratio': {
        'options': [None, 'Cache Hit Ratio', '%', 'cache', 'oracledb.cache_hit_ration', 'stacked'],
        'lines': [
            ['buffer_cache_hit_ratio', 'buffer', 'absolute', 1, 1000],
            ['cursor_cache_hit_ratio', 'cursor', 'absolute', 1, 1000],
            ['library_cache_hit_ratio', 'library', 'absolute', 1, 1000],
            ['row_cache_hit_ratio', 'row', 'absolute', 1, 1000],
        ]
    },
    'global_cache_blocks': {
        'options': [None, 'Global Cache Blocks Events', 'events/s', 'cache', 'oracledb.global_cache_blocks', 'area'],
        'lines': [
            ['global_cache_blocks_corrupted', 'corrupted', 'incremental', 1, 1000],
            ['global_cache_blocks_lost', 'lost', 'incremental', 1, 1000],
        ]
    },
    'activity': {
        'options': [None, 'Activities', 'events/s', 'activities', 'oracledb.activity', 'stacked'],
        'lines': [
            ['activity_parse_count_total', 'parse count', 'incremental', 1, 1000],
            ['activity_execute_count', 'execute count', 'incremental', 1, 1000],
            ['activity_user_commits', 'user commits', 'incremental', 1, 1000],
            ['activity_user_rollbacks', 'user rollbacks', 'incremental', 1, 1000],
        ]
    },
    'wait_time': {
        'options': [None, 'Wait Time', 'ms', 'wait time', 'oracledb.wait_time', 'stacked'],
        'lines': [
            ['wait_time_application', 'application', 'absolute', 1, 1000],
            ['wait_time_configuration', 'configuration', 'absolute', 1, 1000],
            ['wait_time_administrative', 'administrative', 'absolute', 1, 1000],
            ['wait_time_concurrency', 'concurrency', 'absolute', 1, 1000],
            ['wait_time_commit', 'commit', 'absolute', 1, 1000],
            ['wait_time_network', 'network', 'absolute', 1, 1000],
            ['wait_time_user_io', 'user I/O', 'absolute', 1, 1000],
            ['wait_time_system_io', 'system I/O', 'absolute', 1, 1000],
            ['wait_time_scheduler', 'scheduler', 'absolute', 1, 1000],
            ['wait_time_other', 'other', 'absolute', 1, 1000],
        ]
    },
    'tablespace_size': {
        'options': [None, 'Size', 'KiB', 'tablespace', 'oracledb.tablespace_size', 'line'],
        'lines': [],
    },
    'tablespace_usage': {
        'options': [None, 'Usage', 'KiB', 'tablespace', 'oracledb.tablespace_usage', 'line'],
        'lines': [],
    },
    'tablespace_usage_in_percent': {
        'options': [None, 'Usage', '%', 'tablespace', 'oracledb.tablespace_usage_in_percent', 'line'],
        'lines': [],
    },
}

CX_CONNECT_STRING = "{0}/{1}@//{2}/{3}"

QUERY_SYSTEM = '''
SELECT
  metric_name,
  value
FROM
  gv$sysmetric
ORDER BY
  begin_time
'''
QUERY_TABLESPACE = '''
SELECT
  m.tablespace_name,
  m.used_space * t.block_size AS used_bytes,
  m.tablespace_size * t.block_size AS max_bytes,
  m.used_percent
FROM
  dba_tablespace_usage_metrics m
  JOIN dba_tablespaces t ON m.tablespace_name = t.tablespace_name
'''
QUERY_ACTIVITIES_COUNT = '''
SELECT
  name,
  value
FROM
  v$sysstat
WHERE
  name IN (
    'parse count (total)',
    'execute count',
    'user commits',
    'user rollbacks'
  )
'''
QUERY_WAIT_TIME = '''
SELECT
  n.wait_class,
  round(m.time_waited / m.INTSIZE_CSEC, 3)
FROM
  v$waitclassmetric m,
  v$system_wait_class n
WHERE
  m.wait_class_id = n.wait_class_id
  AND n.wait_class != 'Idle'
'''
# QUERY_SESSION_COUNT = '''
# SELECT
#   status,
#   type
# FROM
#   v$session
# GROUP BY
#   status,
#   type
# '''
# QUERY_PROCESSES_COUNT = '''
# SELECT
#   COUNT(*)
# FROM
#   v$process
# '''
# QUERY_PROCESS = '''
# SELECT
#   program,
#   pga_used_mem,
#   pga_alloc_mem,
#   pga_freeable_mem,
#   pga_max_mem
# FROM
#   gv$process
# '''

# PROCESS_METRICS = [
#     'pga_used_memory',
#     'pga_allocated_memory',
#     'pga_freeable_memory',
#     'pga_maximum_memory',
# ]


SYS_METRICS = {
    'Average Active Sessions': 'average_active_sessions',
    'Session Count': 'session_count',
    'Session Limit %': 'session_limit_percent',
    'Logons Per Sec': 'logons_per_sec',
    'Physical Reads Per Sec': 'physical_reads_per_sec',
    'Physical Writes Per Sec': 'physical_writes_per_sec',
    'Disk Sort Per Sec': 'disk_sort_per_sec',
    'Long Table Scans Per Sec': 'long_table_scans_per_sec',
    'Database Wait Time Ratio': 'database_wait_time_ratio',
    'Shared Pool Free %': 'shared_pool_free_percent',
    'Memory Sorts Ratio': 'memory_sorts_ratio',
    'SQL Service Response Time': 'sql_service_response_time',
    'User Rollbacks Per Sec': 'user_rollbacks_per_sec',
    'Enqueue Timeouts Per Sec': 'enqueue_timeouts_per_sec',
    'Buffer Cache Hit Ratio': 'buffer_cache_hit_ratio',
    'Cursor Cache Hit Ratio': 'cursor_cache_hit_ratio',
    'Library Cache Hit Ratio': 'library_cache_hit_ratio',
    'Row Cache Hit Ratio': 'row_cache_hit_ratio',
    'Global Cache Blocks Corrupted': 'global_cache_blocks_corrupted',
    'Global Cache Blocks Lost': 'global_cache_blocks_lost',
}


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = deepcopy(CHARTS)
        self.user = configuration.get('user')
        self.password = configuration.get('password')
        self.server = configuration.get('server')
        self.service = configuration.get('service')
        self.alive = False
        self.conn = None
        self.active_tablespaces = set()

    def connect(self):
        if self.conn:
            self.conn.close()
            self.conn = None

        try:
            self.conn = cx_Oracle.connect(
                CX_CONNECT_STRING.format(
                    self.user,
                    self.password,
                    self.server,
                    self.service,
                ))
        except cx_Oracle.DatabaseError as error:
            self.error(error)
            return False

        self.alive = True
        return True

    def reconnect(self):
        return self.connect()

    def check(self):
        if not HAS_ORACLE:
            self.error("'cx_Oracle' package is needed to use oracledb module")
            return False

        if not all([
            self.user,
            self.password,
            self.server,
            self.service,
        ]):
            self.error("one of these parameters is not specified: user, password, server, service")
            return False

        if not self.connect():
            return False

        return bool(self.get_data())

    def get_data(self):
        if not self.alive and not self.reconnect():
            return None

        data = dict()

        # SYSTEM
        try:
            rv = self.gather_system_metrics()
        except cx_Oracle.Error as error:
            self.error(error)
            self.alive = False
            return None
        else:
            for name, value in rv:
                if name not in SYS_METRICS:
                    continue
                data[SYS_METRICS[name]] = int(float(value) * 1000)

        # ACTIVITIES COUNT
        try:
            rv = self.gather_activities_count()
        except cx_Oracle.Error as error:
            self.error(error)
            self.alive = False
            return None
        else:
            for name, amount in rv:
                cleaned = name.replace(' ', '_').replace('(', '').replace(')', '')
                new_name = 'activity_{0}'.format(cleaned)
                data[new_name] = int(float(amount) * 1000)

        # WAIT TIME
        try:
            rv = self.gather_wait_time_metrics()
        except cx_Oracle.Error as error:
            self.error(error)
            self.alive = False
            return None
        else:
            for name, amount in rv:
                cleaned = name.replace(' ', '_').replace('/', '').lower()
                new_name = 'wait_time_{0}'.format(cleaned)
                data[new_name] = amount

        # TABLESPACE
        try:
            rv = self.gather_tablespace_metrics()
        except cx_Oracle.Error as error:
            self.error(error)
            self.alive = False
            return None
        else:
            for name, offline, size, used, used_in_percent in rv:
                # TODO: skip offline?
                if not (not offline and self.charts):
                    continue
                # TODO: remove inactive?
                if name not in self.active_tablespaces:
                    self.active_tablespaces.add(name)
                    self.add_tablespace_to_charts(name)
                data['{0}_tablespace_size'.format(name)] = int(size * 1000)
                data['{0}_tablespace_used'.format(name)] = int(used * 1000)
                data['{0}_tablespace_used_in_percent'.format(name)] = int(used_in_percent * 1000)

        return data or None

    def gather_system_metrics(self):

        """
        :return:

        [['Buffer Cache Hit Ratio', 100],
         ['Memory Sorts Ratio', 100],
         ['Redo Allocation Hit Ratio', 100],
         ['User Transaction Per Sec', 0],
         ['Physical Reads Per Sec', 0],
         ['Physical Reads Per Txn', 0],
         ['Physical Writes Per Sec', 0],
         ['Physical Writes Per Txn', 0],
         ['Physical Reads Direct Per Sec', 0],
         ['Physical Reads Direct Per Txn', 0],
         ['Physical Writes Direct Per Sec', 0],
         ['Physical Writes Direct Per Txn', 0],
         ['Physical Reads Direct Lobs Per Sec', 0],
         ['Physical Reads Direct Lobs Per Txn', 0],
         ['Physical Writes Direct Lobs Per Sec', 0],
         ['Physical Writes Direct Lobs  Per Txn', 0],
         ['Redo Generated Per Sec', Decimal('4.66666666666667')],
         ['Redo Generated Per Txn', 280],
         ['Logons Per Sec', Decimal('0.0166666666666667')],
         ['Logons Per Txn', 1],
         ['Open Cursors Per Sec', 0.35],
         ['Open Cursors Per Txn', 21],
         ['User Commits Per Sec', 0],
         ['User Commits Percentage', 0],
         ['User Rollbacks Per Sec', 0],
         ['User Rollbacks Percentage', 0],
         ['User Calls Per Sec', Decimal('0.0333333333333333')],
         ['User Calls Per Txn', 2],
         ['Recursive Calls Per Sec', 14.15],
         ['Recursive Calls Per Txn', 849],
         ['Logical Reads Per Sec', Decimal('0.683333333333333')],
         ['Logical Reads Per Txn', 41],
         ['DBWR Checkpoints Per Sec', 0],
         ['Background Checkpoints Per Sec', 0],
         ['Redo Writes Per Sec', Decimal('0.0333333333333333')],
         ['Redo Writes Per Txn', 2],
         ['Long Table Scans Per Sec', 0],
         ['Long Table Scans Per Txn', 0],
         ['Total Table Scans Per Sec', Decimal('0.0166666666666667')],
         ['Total Table Scans Per Txn', 1],
         ['Full Index Scans Per Sec', 0],
         ['Full Index Scans Per Txn', 0],
         ['Total Index Scans Per Sec', Decimal('0.216666666666667')],
         ['Total Index Scans Per Txn', 13],
         ['Total Parse Count Per Sec', 0.35],
         ['Total Parse Count Per Txn', 21],
         ['Hard Parse Count Per Sec', 0],
         ['Hard Parse Count Per Txn', 0],
         ['Parse Failure Count Per Sec', 0],
         ['Parse Failure Count Per Txn', 0],
         ['Cursor Cache Hit Ratio', Decimal('52.3809523809524')],
         ['Disk Sort Per Sec', 0],
         ['Disk Sort Per Txn', 0],
         ['Rows Per Sort', 8.6],
         ['Execute Without Parse Ratio', Decimal('27.5862068965517')],
         ['Soft Parse Ratio', 100],
         ['User Calls Ratio', Decimal('0.235017626321974')],
         ['Host CPU Utilization (%)', Decimal('0.124311845142959')],
         ['Network Traffic Volume Per Sec', 0],
         ['Enqueue Timeouts Per Sec', 0],
         ['Enqueue Timeouts Per Txn', 0],
         ['Enqueue Waits Per Sec', 0],
         ['Enqueue Waits Per Txn', 0],
         ['Enqueue Deadlocks Per Sec', 0],
         ['Enqueue Deadlocks Per Txn', 0],
         ['Enqueue Requests Per Sec', Decimal('216.683333333333')],
         ['Enqueue Requests Per Txn', 13001],
         ['DB Block Gets Per Sec', 0],
         ['DB Block Gets Per Txn', 0],
         ['Consistent Read Gets Per Sec', Decimal('0.683333333333333')],
         ['Consistent Read Gets Per Txn', 41],
         ['DB Block Changes Per Sec', 0],
         ['DB Block Changes Per Txn', 0],
         ['Consistent Read Changes Per Sec', 0],
         ['Consistent Read Changes Per Txn', 0],
         ['CPU Usage Per Sec', 0],
         ['CPU Usage Per Txn', 0],
         ['CR Blocks Created Per Sec', 0],
         ['CR Blocks Created Per Txn', 0],
         ['CR Undo Records Applied Per Sec', 0],
         ['CR Undo Records Applied Per Txn', 0],
         ['User Rollback UndoRec Applied Per Sec', 0],
         ['User Rollback Undo Records Applied Per Txn', 0],
         ['Leaf Node Splits Per Sec', 0],
         ['Leaf Node Splits Per Txn', 0],
         ['Branch Node Splits Per Sec', 0],
         ['Branch Node Splits Per Txn', 0],
         ['PX downgraded 1 to 25% Per Sec', 0],
         ['PX downgraded 25 to 50% Per Sec', 0],
         ['PX downgraded 50 to 75% Per Sec', 0],
         ['PX downgraded 75 to 99% Per Sec', 0],
         ['PX downgraded to serial Per Sec', 0],
         ['Physical Read Total IO Requests Per Sec', Decimal('2.16666666666667')],
         ['Physical Read Total Bytes Per Sec', Decimal('35498.6666666667')],
         ['GC CR Block Received Per Second', 0],
         ['GC CR Block Received Per Txn', 0],
         ['GC Current Block Received Per Second', 0],
         ['GC Current Block Received Per Txn', 0],
         ['Global Cache Average CR Get Time', 0],
         ['Global Cache Average Current Get Time', 0],
         ['Physical Write Total IO Requests Per Sec', Decimal('0.966666666666667')],
         ['Global Cache Blocks Corrupted', 0],
         ['Global Cache Blocks Lost', 0],
         ['Current Logons Count', 49],
         ['Current Open Cursors Count', 64],
         ['User Limit %', Decimal('0.00000114087015416959')],
         ['SQL Service Response Time', 0],
         ['Database Wait Time Ratio', 0],
         ['Database CPU Time Ratio', 0],
         ['Response Time Per Txn', 0],
         ['Row Cache Hit Ratio', 100],
         ['Row Cache Miss Ratio', 0],
         ['Library Cache Hit Ratio', 100],
         ['Library Cache Miss Ratio', 0],
         ['Shared Pool Free %', Decimal('7.82380268491548')],
         ['PGA Cache Hit %', Decimal('98.0399767109115')],
         ['Process Limit %', Decimal('17.6666666666667')],
         ['Session Limit %', Decimal('15.2542372881356')],
         ['Executions Per Txn', 29],
         ['Executions Per Sec', Decimal('0.483333333333333')],
         ['Txns Per Logon', 0],
         ['Database Time Per Sec', 0],
         ['Physical Write Total Bytes Per Sec', 15308.8],
         ['Physical Read IO Requests Per Sec', 0],
         ['Physical Read Bytes Per Sec', 0],
         ['Physical Write IO Requests Per Sec', 0],
         ['Physical Write Bytes Per Sec', 0],
         ['DB Block Changes Per User Call', 0],
         ['DB Block Gets Per User Call', 0],
         ['Executions Per User Call', 14.5],
         ['Logical Reads Per User Call', 20.5],
         ['Total Sorts Per User Call', 2.5],
         ['Total Table Scans Per User Call', 0.5],
         ['Current OS Load', 0.0390625],
         ['Streams Pool Usage Percentage', 0],
         ['PQ QC Session Count', 0],
         ['PQ Slave Session Count', 0],
         ['Queries parallelized Per Sec', 0],
         ['DML statements parallelized Per Sec', 0],
         ['DDL statements parallelized Per Sec', 0],
         ['PX operations not downgraded Per Sec', 0],
         ['Session Count', 72],
         ['Average Synchronous Single-Block Read Latency', 0],
         ['I/O Megabytes per Second', 0.05],
         ['I/O Requests per Second', Decimal('3.13333333333333')],
         ['Average Active Sessions', 0],
         ['Active Serial Sessions', 1],
         ['Active Parallel Sessions', 0],
         ['Captured user calls', 0],
         ['Replayed user calls', 0],
         ['Workload Capture and Replay status', 0],
         ['Background CPU Usage Per Sec', Decimal('1.22578833333333')],
         ['Background Time Per Sec', 0.0147551],
         ['Host CPU Usage Per Sec', Decimal('0.116666666666667')],
         ['Cell Physical IO Interconnect Bytes', 3048448],
         ['Temp Space Used', 0],
         ['Total PGA Allocated', 200657920],
         ['Total PGA Used by SQL Workareas', 0],
         ['Run Queue Per Sec', 0],
         ['VM in bytes Per Sec', 0],
         ['VM out bytes Per Sec', 0]]
        """

        metrics = list()
        with self.conn.cursor() as cursor:
            cursor.execute(QUERY_SYSTEM)
            for metric_name, value in cursor.fetchall():
                metrics.append([metric_name, value])
        return metrics

    def gather_tablespace_metrics(self):
        """
        :return:

        [['SYSTEM', 874250240.0, 3233169408.0, 27.040038107400033, 0],
         ['SYSAUX', 498860032.0, 3233169408.0, 15.429443033997678, 0],
         ['TEMP', 0.0, 3233177600.0, 0.0, 0],
         ['USERS', 1048576.0, 3233169408.0, 0.03243182981397305, 0]]
        """
        metrics = list()
        with self.conn.cursor() as cursor:
            cursor.execute(QUERY_TABLESPACE)
            for tablespace_name, used_bytes, max_bytes, used_percent in cursor.fetchall():
                if used_bytes is None:
                    offline = True
                    used = 0
                else:
                    offline = False
                    used = float(used_bytes)
                if max_bytes is None:
                    size = 0
                else:
                    size = float(max_bytes)
                if used_percent is None:
                    used_percent = 0
                else:
                    used_percent = float(used_percent)
                metrics.append(
                    [
                        tablespace_name,
                        offline,
                        size,
                        used,
                        used_percent,
                    ]
                )
        return metrics

    def gather_wait_time_metrics(self):
        """
        :return:

        [['Other', 0],
         ['Application', 0],
         ['Configuration', 0],
         ['Administrative', 0],
         ['Concurrency', 0],
         ['Commit', 0],
         ['Network', 0],
         ['User I/O', 0],
         ['System I/O', 0.002],
         ['Scheduler', 0]]
        """
        metrics = list()
        with self.conn.cursor() as cursor:
            cursor.execute(QUERY_WAIT_TIME)
            for wait_class_name, value in cursor.fetchall():
                metrics.append([wait_class_name, value])
        return metrics

    def gather_activities_count(self):
        """
        :return:

        [('user commits', 9104),
         ('user rollbacks', 17),
         ('parse count (total)', 483695),
         ('execute count', 2020356)]
        """
        with self.conn.cursor() as cursor:
            cursor.execute(QUERY_ACTIVITIES_COUNT)
            return cursor.fetchall()

    # def gather_process_metrics(self):
    #     """
    #     :return:
    #
    #     [['PSEUDO', 'pga_used_memory', 0],
    #      ['PSEUDO', 'pga_allocated_memory', 0],
    #      ['PSEUDO', 'pga_freeable_memory', 0],
    #      ['PSEUDO', 'pga_maximum_memory', 0],
    #      ['oracle@localhost.localdomain (PMON)', 'pga_used_memory', 1793827],
    #      ['oracle@localhost.localdomain (PMON)', 'pga_allocated_memory', 1888651],
    #      ['oracle@localhost.localdomain (PMON)', 'pga_freeable_memory', 0],
    #      ['oracle@localhost.localdomain (PMON)', 'pga_maximum_memory', 1888651],
    #      ...
    #      ...
    #     """
    #
    #     metrics = list()
    #     with self.conn.cursor() as cursor:
    #         cursor.execute(QUERY_PROCESS)
    #         for row in cursor.fetchall():
    #             for i, name in enumerate(PROCESS_METRICS, 1):
    #                 metrics.append([row[0], name, row[i]])
    #     return metrics

    # def gather_processes_count(self):
    #     with self.conn.cursor() as cursor:
    #         cursor.execute(QUERY_PROCESSES_COUNT)
    #         return cursor.fetchone()[0]  # 53

    # def gather_sessions_count(self):
    #     with self.conn.cursor() as cursor:
    #         cursor.execute(QUERY_SESSION_COUNT)
    #         total, active, inactive = 0, 0, 0
    #         for status, _ in cursor.fetchall():
    #             total += 1
    #             active += status == 'ACTIVE'
    #             inactive += status == 'INACTIVE'
    #     return [total, active, inactive]

    def add_tablespace_to_charts(self, name):
        self.charts['tablespace_size'].add_dimension(
            [
                '{0}_tablespace_size'.format(name),
                name,
                'absolute',
                1,
                1024 * 1000,
            ])
        self.charts['tablespace_usage'].add_dimension(
            [
                '{0}_tablespace_used'.format(name),
                name,
                'absolute',
                1,
                1024 * 1000,
            ])
        self.charts['tablespace_usage_in_percent'].add_dimension(
            [
                '{0}_tablespace_used_in_percent'.format(name),
                name,
                'absolute',
                1,
                1000,
            ])
