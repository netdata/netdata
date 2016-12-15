# -*- coding: utf-8 -*-
# Description: example netdata python.d module
# Authors: facetoe, dangtranhoang

import re
from copy import deepcopy

import psycopg2
from psycopg2 import extensions
from psycopg2.extras import DictCursor

from base import SimpleService

# default module values
update_every = 1
priority = 90000
retries = 60

ARCHIVE = """
SELECT
    CAST(COUNT(*) AS INT) AS file_count,
    CAST(COALESCE(SUM(CAST(archive_file ~ $r$\.ready$$r$ as INT)), 0) AS INT) AS ready_count,
    CAST(COALESCE(SUM(CAST(archive_file ~ $r$\.done$$r$ AS INT)), 0) AS INT) AS done_count
FROM
    pg_catalog.pg_ls_dir('pg_xlog/archive_status') AS archive_files (archive_file);
"""

BACKENDS = """
SELECT
    count(*) - (SELECT count(*) FROM pg_stat_activity WHERE state = 'idle') AS backends_active,
    (SELECT count(*) FROM pg_stat_activity WHERE state = 'idle' ) AS backends_idle
FROM
    pg_stat_activity;
"""

TABLE_STATS = """
SELECT
  ((sum(relpages) * 8) * 1024) AS size_relations,
  count(1)                     AS relations
FROM pg_class
WHERE relkind IN ('r', 't');
"""

INDEX_STATS = """
SELECT
  ((sum(relpages) * 8) * 1024) AS size_indexes,
  count(1)                     AS indexes
FROM pg_class
WHERE relkind = 'i';"""

DATABASE = """
SELECT
  datname AS database_name,
  sum(numbackends) AS connections,
  sum(xact_commit) AS xact_commit,
  sum(xact_rollback) AS xact_rollback,
  sum(blks_read) AS blks_read,
  sum(blks_hit) AS blks_hit,
  sum(tup_returned) AS tup_returned,
  sum(tup_fetched) AS tup_fetched,
  sum(tup_inserted) AS tup_inserted,
  sum(tup_updated) AS tup_updated,
  sum(tup_deleted) AS tup_deleted,
  sum(conflicts) AS conflicts
FROM pg_stat_database
WHERE NOT datname ~* '^template\d+'
GROUP BY database_name;
"""

BGWRITER = 'SELECT * FROM pg_stat_bgwriter;'
DATABASE_LOCKS = """
SELECT
  pg_database.datname as database_name,
  mode,
  count(mode) AS count
FROM pg_locks
  INNER JOIN pg_database ON pg_database.oid = pg_locks.database
GROUP BY datname, mode
ORDER BY datname, mode;
"""
REPLICATION = """
SELECT
    client_hostname,
    client_addr,
    state,
    sent_offset - (
        replay_offset - (sent_xlog - replay_xlog) * 255 * 16 ^ 6 ) AS byte_lag
FROM (
    SELECT
        client_addr, client_hostname, state,
        ('x' || lpad(split_part(sent_location::text,   '/', 1), 8, '0'))::bit(32)::bigint AS sent_xlog,
        ('x' || lpad(split_part(replay_location::text, '/', 1), 8, '0'))::bit(32)::bigint AS replay_xlog,
        ('x' || lpad(split_part(sent_location::text,   '/', 2), 8, '0'))::bit(32)::bigint AS sent_offset,
        ('x' || lpad(split_part(replay_location::text, '/', 2), 8, '0'))::bit(32)::bigint AS replay_offset
    FROM pg_stat_replication
) AS s;
"""

LOCK_TYPES = [
    'ExclusiveLock',
    'RowShareLock',
    'SIReadLock',
    'ShareUpdateExclusiveLock',
    'AccessExclusiveLock',
    'AccessShareLock',
    'ShareRowExclusiveLock',
    'ShareLock',
    'RowExclusiveLock'
]

ORDER = ['db_stat_transactions', 'db_stat_tuple_read', 'db_stat_tuple_returned', 'db_stat_tuple_write',
         'backend_process', 'index_count', 'index_size', 'table_count', 'table_size', 'wal', 'background_writer']

CHARTS = {
    'db_stat_transactions': {
        'options': [None, 'Transactions on db', 'transactions/s', 'db statistics', 'postgres.db_stat_transactions', 'line'],
        'lines': [
            ['db_stat_xact_commit',   'committed',   'incremental'],
            ['db_stat_xact_rollback', 'rolled back', 'incremental']
        ]},
    'db_stat_connections': {
        'options': [None, 'Current connections to db', 'count', 'db statistics', 'postgres.db_stat_connections', 'line'],
        'lines': [
            ['db_stat_connections', 'connections', 'absolute']
        ]},
    'db_stat_tuple_read': {
        'options': [None, 'Tuple reads from db', 'reads/s', 'db statistics', 'postgres.db_stat_tuple_read', 'line'],
        'lines': [
            ['db_stat_blks_read', 'disk',  'incremental'],
            ['db_stat_blks_hit',  'cache', 'incremental']
        ]},
    'db_stat_tuple_returned': {
        'options': [None, 'Tuples returned from db', 'tuples/s', 'db statistics', 'postgres.db_stat_tuple_returned', 'line'],
        'lines': [
            ['db_stat_tup_returned', 'sequential', 'incremental'],
            ['db_stat_tup_fetched',  'bitmap',     'incremental']
        ]},
    'db_stat_tuple_write': {
        'options': [None, 'Tuples written to db', 'writes/s', 'db statistics', 'postgres.db_stat_tuple_write', 'line'],
        'lines': [
            ['db_stat_tup_inserted', 'inserted',  'incremental'],
            ['db_stat_tup_updated',  'updated',   'incremental'],
            ['db_stat_tup_deleted',  'deleted',   'incremental'],
            ['db_stat_conflicts',    'conflicts', 'incremental']
        ]},
    'backend_process': {
        'options': [None, 'Current Backend Processes', 'processes', 'backend processes', 'postgres.backend_process', 'line'],
        'lines': [
            ['backend_process_active', 'active', 'absolute'],
            ['backend_process_idle',   'idle',   'absolute']
        ]},
    'index_count': {
        'options': [None, 'Total indexes', 'index', 'indexes', 'postgres.index_count', 'line'],
        'lines': [
            ['index_count', 'total', 'absolute']
        ]},
    'index_size': {
        'options': [None, 'Indexes size', 'MB', 'indexes', 'postgres.index_size', 'line'],
        'lines': [
            ['index_size', 'size', 'absolute', 1, 1024 * 1024]
        ]},
    'table_count': {
        'options': [None, 'Total Tables', 'tables', 'tables', 'postgres.table_count', 'line'],
        'lines': [
            ['table_count', 'total', 'absolute']
        ]},
    'table_size': {
        'options': [None, 'Tables size', 'MB', 'tables', 'postgres.table_size', 'line'],
        'lines': [
            ['table_size', 'size', 'absolute', 1, 1024 * 1024]
        ]},
    'wal': {
        'options': [None, 'Write-Ahead Logging Statistics', 'files/s', 'write ahead log', 'postgres.wal', 'line'],
        'lines': [
            ['wal_total', 'total', 'incremental'],
            ['wal_ready', 'ready', 'incremental'],
            ['wal_done',  'done',  'incremental']
        ]},
    'background_writer': {
        'options': [None, 'Checkpoints', 'writes/s', 'background writer', 'postgres.background_writer', 'line'],
        'lines': [
            ['background_writer_scheduled', 'scheduled', 'incremental'],
            ['background_writer_requested', 'requested', 'incremental']
        ]}
}


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        super(self.__class__, self).__init__(configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.table_stats = configuration.pop('table_stats', True)
        self.index_stats = configuration.pop('index_stats', True)
        self.configuration = configuration
        self.connection = None
        self.is_superuser = False
        self.data = {}
        self.databases = set()

    def _connect(self):
        params = dict(user='postgres',
                      database=None,
                      password=None,
                      host='localhost',
                      port=5432)
        params.update(self.configuration)

        if not self.connection:
            self.connection = psycopg2.connect(**params)
            self.connection.set_isolation_level(extensions.ISOLATION_LEVEL_AUTOCOMMIT)
            self.connection.set_session(readonly=True)

    def check(self):
        try:
            self._connect()
            cursor = self.connection.cursor()
            self._discover_databases(cursor)
            self._check_if_superuser(cursor)
            cursor.close()

            self._create_definitions()
            return True
        except Exception as e:
            self.error(str(e))
            return False

    def _discover_databases(self, cursor):
        cursor.execute("""
            SELECT datname
            FROM pg_stat_database
            WHERE NOT datname ~* '^template\d+'
        """)
        self.databases = set(r[0] for r in cursor)

    def _check_if_superuser(self, cursor):
        cursor.execute("""
            SELECT current_setting('is_superuser') = 'on' AS is_superuser;
        """)
        self.is_superuser = cursor.fetchone()[0]

    def _create_definitions(self):
        for database_name in self.databases:
            for chart_template_name in list(CHARTS):
                if chart_template_name.startswith('db_stat'):
                    self._add_database_stat_chart(chart_template_name, database_name)
            self._add_database_lock_chart(database_name)

    def _add_database_stat_chart(self, chart_template_name, database_name):
        chart_template = CHARTS[chart_template_name]
        chart_name = "{0}_{1}".format(database_name, chart_template_name)
        if chart_name not in self.order:
            self.order.insert(0, chart_name)
            name, title, units, family, context, chart_type = chart_template['options']
            self.definitions[chart_name] = {
                'options': [
                    name,
                    title + ': ' + database_name,
                    units,
                    'db ' + database_name,
                    context,
                    chart_type
                ]
            }

            self.definitions[chart_name]['lines'] = []
            for line in deepcopy(chart_template['lines']):
                line[0] = "{0}_{1}".format(database_name, line[0])
                self.definitions[chart_name]['lines'].append(line)

    def _add_database_lock_chart(self, database_name):
        chart_name = "{0}_locks".format(database_name)
        if chart_name not in self.order:
            self.order.insert(-1, chart_name)
            self.definitions[chart_name] = dict(
                options=
                [
                    None,
                    'Locks on db: ' + database_name,
                    'locks',
                    'db ' + database_name,
                    'postgres.db_locks',
                    'line'
                ],
                lines=[]
            )

            for lock_type in LOCK_TYPES:
                lock_id = "{0}_{1}".format(database_name, lock_type)
                label = re.sub("([a-z])([A-Z])", "\g<1> \g<2>", lock_type)
                self.definitions[chart_name]['lines'].append([lock_id, label, 'absolute'])

    def _get_data(self):
        self._connect()

        cursor = self.connection.cursor(cursor_factory=DictCursor)
        self.add_stats(cursor)

        cursor.close()
        return self.data

    def add_stats(self, cursor):
        self.add_database_stats(cursor)
        self.add_backend_stats(cursor)
        if self.index_stats:
            self.add_index_stats(cursor)
        if self.table_stats:
            self.add_table_stats(cursor)
        self.add_lock_stats(cursor)
        self.add_bgwriter_stats(cursor)

        # self.add_replication_stats(cursor)

        if self.is_superuser:
            self.add_wal_stats(cursor)

    def add_database_stats(self, cursor):
        cursor.execute(DATABASE)
        for row in cursor:
            database_name = row.get('database_name')
            self.data["{0}_{1}".format(database_name, 'db_stat_xact_commit')]   = int(row.get('xact_commit',   0))
            self.data["{0}_{1}".format(database_name, 'db_stat_xact_rollback')] = int(row.get('xact_rollback', 0))
            self.data["{0}_{1}".format(database_name, 'db_stat_blks_read')]     = int(row.get('blks_read',     0))
            self.data["{0}_{1}".format(database_name, 'db_stat_blks_hit')]      = int(row.get('blks_hit',      0))
            self.data["{0}_{1}".format(database_name, 'db_stat_tup_returned')]  = int(row.get('tup_returned',  0))
            self.data["{0}_{1}".format(database_name, 'db_stat_tup_fetched')]   = int(row.get('tup_fetched',   0))
            self.data["{0}_{1}".format(database_name, 'db_stat_tup_inserted')]  = int(row.get('tup_inserted',  0))
            self.data["{0}_{1}".format(database_name, 'db_stat_tup_updated')]   = int(row.get('tup_updated',   0))
            self.data["{0}_{1}".format(database_name, 'db_stat_tup_deleted')]   = int(row.get('tup_deleted',   0))
            self.data["{0}_{1}".format(database_name, 'db_stat_conflicts')]     = int(row.get('conflicts',     0))
            self.data["{0}_{1}".format(database_name, 'db_stat_connections')]   = int(row.get('connections',   0))

    def add_backend_stats(self, cursor):
        cursor.execute(BACKENDS)
        temp = cursor.fetchone()

        self.data['backend_process_active'] = int(temp.get('backends_active', 0))
        self.data['backend_process_idle']   = int(temp.get('backends_idle',   0))

    def add_index_stats(self, cursor):
        cursor.execute(INDEX_STATS)
        temp = cursor.fetchone()
        self.data['index_count'] = int(temp.get('indexes',      0))
        self.data['index_size']  = int(temp.get('size_indexes', 0))

    def add_table_stats(self, cursor):
        cursor.execute(TABLE_STATS)
        temp = cursor.fetchone()
        self.data['table_count'] = int(temp.get('relations',      0))
        self.data['table_size']  = int(temp.get('size_relations', 0))

    def add_lock_stats(self, cursor):
        cursor.execute(DATABASE_LOCKS)

        # zero out all current lock values
        for database_name in self.databases:
            for lock_type in LOCK_TYPES:
                self.data["{0}_{1}".format(database_name, lock_type)] = 0

        # populate those that have current locks
        for row in cursor:
            database_name, lock_type, lock_count = row
            self.data["{0}_{1}".format(database_name, lock_type)] = lock_count

    def add_wal_stats(self, cursor):
        cursor.execute(ARCHIVE)
        temp = cursor.fetchone()
        self.data['wal_total'] = int(temp.get('file_count',  0))
        self.data['wal_ready'] = int(temp.get('ready_count', 0))
        self.data['wal_done']  = int(temp.get('done_count',  0))

    def add_bgwriter_stats(self, cursor):
        cursor.execute(BGWRITER)
        temp = cursor.fetchone()
        self.data['background_writer_scheduled'] = temp.get('checkpoints_timed',    0)
        self.data['background_writer_requested'] = temp.get('checkpoints_requests', 0)

'''
    def add_replication_stats(self, cursor):
        cursor.execute(REPLICATION)
        temp = cursor.fetchall()
        for row in temp:
            self.add_gauge_value('Replication/%s' % row.get('client_addr', 'Unknown'),
                                 'byte_lag',
                                 int(row.get('byte_lag', 0)))
'''
