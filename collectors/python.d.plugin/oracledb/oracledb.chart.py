# -*- coding: utf-8 -*-
# Description: example netdata python.d module
# Author: ilyam8 (Ilya Mashchenko)
# SPDX-License-Identifier: GPL-3.0-or-later

from bases.FrameworkServices.SimpleService import SimpleService

try:
    import cx_Oracle
    HAS_ORACLE = True
except ImportError:
    HAS_ORACLE = False


ORDER = [
    'processes',
    'total_sessions',
    'sessions',
    'activity',
    'wait_time',
]

CHARTS = {
    'processes': {
        'options': [None, 'Processes', 'amount', 'processes', 'oracledb.processes', 'line'],
        'lines': [
            ['processes'],
        ]
    },
    'total_sessions': {
        'options': [None, 'Total Sessions', 'amount', 'sessions', 'oracledb.sessions', 'line'],
        'lines': [
            ['sessions_total', 'sessions'],
        ]
    },
    'sessions': {
        'options': [None, 'Sessions', 'amount', 'sessions', 'oracledb.sessions', 'line'],
        'lines': [
            ['sessions_active', 'active'],
            ['sessions_inactive', 'inactive'],
        ]
    },
    'activity': {
        'options': [None, 'Activities Rate', 'activities', 'activities', 'oracledb.activity', 'stacked'],
        'lines': [
            ['activity_parse_count_total', 'parse count (total)', 'incremental'],
            ['activity_execute_count', 'execute count', 'incremental'],
            ['activity_user_commits', 'user commits', 'incremental'],
            ['activity_user_rollbacks', 'user rollbacks', 'incremental'],
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
    }
}


CX_CONNECT_STRING = "{0}/{1}@//{2}/{3}"

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
QUERY_SESSION = '''
SELECT
  status,
  type
FROM
  v$session
GROUP BY
  status,
  type
'''
QUERY_PROCESSES_COUNT = '''
SELECT
  COUNT(*)
FROM
  v$process
'''
QUERY_PROCESS = '''
SELECT
  program,
  pga_used_mem,
  pga_alloc_mem,
  pga_freeable_mem,
  pga_max_mem
FROM
  gv$process
'''
QUERY_SYSTEM = '''
SELECT
  metric_name,
  value,
  begin_time
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

PROCESS_METRICS = [
    'pga_used_memory',
    'pga_allocated_memory',
    'pga_freeable_memory',
    'pga_maximum_memory',
]


# def handle_oracle_error(method):
#     def on_call(*args, **kwargs):
#         self = args[0]
#         try:
#             return method(*args, **kwargs)
#         except cx_Oracle.Error as error:
#             self.error(error)
#             return None
#     return on_call


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.user = configuration.get('user', 'system')
        self.password = configuration.get('password', 'oraclepass')
        self.server = configuration.get('server', 'localhost:1521')
        self.service = configuration.get('service', 'XE')
        self.alive = False
        self.conn = None

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
            return False

        if not all([
            self.user,
            self.password,
            self.server,
            self.service,
        ]):
            return False

        return bool(self.get_data())

    def get_data(self):
        if not self.alive and not self.reconnect():
            return None

        return None

    def get_system_metrics(self):

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
         ['VM out bytes Per Sec', 0],
         ['Buffer Cache Hit Ratio', 100],
         ['Total PGA Used by SQL Workareas', 0],
         ['User Transaction Per Sec', 0],
         ['Physical Reads Per Sec', 0],
         ['Physical Reads Per Txn', 0],
         ['Physical Writes Per Sec', 0],
         ['Physical Writes Per Txn', 0],
         ['Physical Reads Direct Per Sec', 0],
         ['Physical Reads Direct Per Txn', 0],
         ['Redo Generated Per Sec', Decimal('18.6542305129913')],
         ['Redo Generated Per Txn', 280],
         ['Logons Per Sec', Decimal('0.0666222518321119')],
         ['Logons Per Txn', 1],
         ['User Calls Per Sec', Decimal('0.133244503664224')],
         ['User Calls Per Txn', 2],
         ['Logical Reads Per Sec', Decimal('2.33177881412392')],
         ['Logical Reads Per Txn', 35],
         ['Redo Writes Per Sec', Decimal('0.133244503664224')],
         ['Redo Writes Per Txn', 2],
         ['Total Table Scans Per Sec', Decimal('0.0666222518321119')],
         ['Total Table Scans Per Txn', 1],
         ['Full Index Scans Per Sec', 0],
         ['Full Index Scans Per Txn', 0],
         ['Execute Without Parse Ratio', 0],
         ['Soft Parse Ratio', 100],
         ['Host CPU Utilization (%)', Decimal('0.14194464158978')],
         ['DB Block Gets Per Sec', 0],
         ['DB Block Gets Per Txn', 0],
         ['Consistent Read Gets Per Sec', Decimal('2.33177881412392')],
         ['Consistent Read Gets Per Txn', 35],
         ['DB Block Changes Per Sec', 0],
         ['DB Block Changes Per Txn', 0],
         ['Consistent Read Changes Per Sec', 0],
         ['Consistent Read Changes Per Txn', 0],
         ['Database CPU Time Ratio', 0],
         ['Library Cache Hit Ratio', 100],
         ['Shared Pool Free %', Decimal('7.82380268491548')],
         ['Executions Per Txn', 9],
         ['Executions Per Sec', Decimal('0.599600266489007')],
         ['Txns Per Logon', 0],
         ['Database Time Per Sec', 0],
         ['Average Active Sessions', 0],
         ['Host CPU Usage Per Sec', Decimal('0.133244503664224')],
         ['Cell Physical IO Interconnect Bytes', 1131520],
         ['Temp Space Used', 0],
         ['Total PGA Allocated', 200657920],
         ['Memory Sorts Ratio', 100]]
        """

        metrics = list()
        with self.conn.cursor() as cursor:
            cursor.execute(QUERY_SYSTEM)
            for row in cursor.fetchall():
                metrics.append([row[0], row[1]])
        return metrics

    def get_process_metrics(self):
        """
        :return:

        [['PSEUDO', 'pga_used_memory', 0],
         ['PSEUDO', 'pga_allocated_memory', 0],
         ['PSEUDO', 'pga_freeable_memory', 0],
         ['PSEUDO', 'pga_maximum_memory', 0],
         ['oracle@localhost.localdomain (PMON)', 'pga_used_memory', 1793827],
         ['oracle@localhost.localdomain (PMON)', 'pga_allocated_memory', 1888651],
         ['oracle@localhost.localdomain (PMON)', 'pga_freeable_memory', 0],
         ['oracle@localhost.localdomain (PMON)', 'pga_maximum_memory', 1888651],
         ...
         ...
        """

        metrics = list()
        with self.conn.cursor() as cursor:
            cursor.execute(QUERY_PROCESS)
            for row in cursor.fetchall():
                for i, name in enumerate(PROCESS_METRICS, 1):
                    metrics.append([row[0], name, row[i]])
        return metrics

    def get_tablespace_metrics(self):
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
                    offline = 1
                    used = 0
                else:
                    offline = 0
                    used = float(used_bytes)
                if max_bytes is None:
                    size = 0
                else:
                    size = float(max_bytes)
                if used_percent is None:
                    in_use = 0
                else:
                    in_use = float(used_percent)
                metrics.append(
                    [
                        tablespace_name,
                        used,
                        size,
                        in_use,
                        offline,
                    ]
                )
        return metrics

    def get_sessions_metrics(self):
        with self.conn.cursor() as cursor:
            cursor.execute(QUERY_SESSION)
            total, active, inactive = 0, 0, 0
            for status, _ in cursor.fetchall():
                total += 1
                active += status == 'ACTIVE'
                inactive += status == 'INACTIVE'
        return [
            ['total', total],
            ['active', active],
            ['inactive', inactive],
        ]

    def get_wait_time_metrics(self):
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
            for wait_class, value in cursor.fetchall():
                metrics.append([wait_class, value])
        return metrics

    def get_processes_count(self):
        with self.conn.cursor() as cursor:
            cursor.execute(QUERY_PROCESSES_COUNT)
            return cursor.fetchone()[0]  # 53

    def get_activities_count(self):
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
