# -*- coding: utf-8 -*-
# Description: example netdata python.d module
# Author: Pawel Krupa (paulfantom)

import psycopg2
from base import SimpleService
from psycopg2.extras import DictCursor
from psycopg2 import extensions
from psycopg2 import extras

NAME = "test"

# default module values
update_every = 1
priority = 90000
retries = 60

# Default Config options.
# {
#    'database': None,
#    'user': 'postgres',
#    'password': None,
#    'host': 'localhost',
#    'port': 5432
# }

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

TABLE_SIZE_ON_DISK = """
SELECT ((sum(relpages)* 8) * 1024) AS size_relations FROM pg_class WHERE relkind IN ('r', 't');
"""

TABLE_COUNT = """
SELECT count(1) as relations FROM pg_class WHERE relkind IN ('r', 't');
"""

INDEX_SIZE_ON_DISK = """
SELECT ((sum(relpages)* 8) * 1024) AS size_indexes FROM pg_class WHERE relkind = 'i';
"""

INDEX_COUNT = """
SELECT count(1) as indexes FROM pg_class WHERE relkind = 'i';
"""

DATABASE = """
SELECT
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
FROM pg_stat_database;
"""

STATIO = """
SELECT
    sum(heap_blks_read) AS heap_blocks_read,
    sum(heap_blks_hit) AS heap_blocks_hit,
    sum(idx_blks_read) AS index_blocks_read,
    sum(idx_blks_hit) AS index_blocks_hit,
    sum(toast_blks_read) AS toast_blocks_read,
    sum(toast_blks_hit) AS toast_blocks_hit,
    sum(tidx_blks_read) AS toastindex_blocks_read,
    sum(tidx_blks_hit) AS toastindex_blocks_hit
FROM
    pg_statio_all_tables
WHERE
    schemaname <> 'pg_catalog';
"""
BGWRITER = 'SELECT * FROM pg_stat_bgwriter;'
LOCKS = 'SELECT mode, count(mode) AS count FROM pg_locks GROUP BY mode ORDER BY mode;'
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
        ('x' || lpad(split_part(sent_location,   '/', 1), 8, '0'))::bit(32)::bigint AS sent_xlog,
        ('x' || lpad(split_part(replay_location, '/', 1), 8, '0'))::bit(32)::bigint AS replay_xlog,
        ('x' || lpad(split_part(sent_location,   '/', 2), 8, '0'))::bit(32)::bigint AS sent_offset,
        ('x' || lpad(split_part(replay_location, '/', 2), 8, '0'))::bit(32)::bigint AS replay_offset
    FROM pg_stat_replication
) AS s;
"""

LOCK_MAP = {'AccessExclusiveLock': 'lock_access_exclusive',
            'AccessShareLock': 'lock_access_share',
            'ExclusiveLock': 'lock_exclusive',
            'RowExclusiveLock': 'lock_row_exclusive',
            'RowShareLock': 'lock_row_share',
            'ShareUpdateExclusiveLock': 'lock_update_exclusive_lock',
            'ShareLock': 'lock_share',
            'ShareRowExclusiveLock': 'lock_share_row_exclusive',
            'SIReadLock': 'lock_si_read'}

ORDER = ['db_stat_transactions', 'db_stat_tuple_read', 'db_stat_tuple_returned', 'db_stat_tuple_write', 'backend_process', 'index_count', 'index_size', 'table_count', 'table_size', 'locks', 'wal', 'operations_heap', 'operations_index', 'operations_toast', 'operations_toast_index', 'background_writer']

CHARTS = {
    'db_stat_transactions': {
        'options': [None, 'Transactions', 'Count', 'Database Stat', 'postgres.db_stat_transactions', 'line'],
        'lines': [
            ['db_stat_xact_commit', 'Committed', 'absolute'],
            ['db_stat_xact_rollback', 'Rolled Back', 'absolute']
        ]},
    'db_stat_tuple_read': {
        'options': [None, 'Tuple read', 'Count', 'Database Stat', 'postgres.db_stat_tuple_read', 'line'],
        'lines': [
            ['db_stat_blks_read', 'Disk', 'absolute'],
            ['db_stat_blks_hit', 'Cache', 'absolute']
        ]},
    'db_stat_tuple_returned': {
        'options': [None, 'Tuple returned', 'Count', 'Database Stat', 'postgres.db_stat_tuple_returned', 'line'],
        'lines': [
            ['db_stat_tup_returned', 'Sequential', 'absolute'],
            ['db_stat_tup_fetched', 'Bitmap', 'absolute']
        ]},
    'db_stat_tuple_write': {
        'options': [None, 'Tuple write', 'Count', 'Database Stat', 'postgres.db_stat_tuple_write', 'line'],
        'lines': [
            ['db_stat_tup_inserted', 'Inserted', 'absolute'],
            ['db_stat_tup_updated', 'Updated', 'absolute'],
            ['db_stat_tup_deleted', 'Deleted', 'absolute'],
            ['db_stat_conflicts', 'Conflicts', 'absolute']
        ]},
    'backend_process': {
        'options': [None, 'Backend processes', 'Count', 'Backend processes', 'postgres.backend_process', 'line'],
        'lines': [
            ['backend_process_active', 'Active', 'absolute'],
            ['backend_process_idle', 'Idle', 'absolute']
        ]},
    'index_count': {
        'options': [None, 'Total index', 'Count', 'Index', 'postgres.index_count', 'line'],
        'lines': [
            ['index_count', 'Total index', 'absolute']
        ]},
    'index_size': {
        'options': [None, 'Index size', 'MB', 'Index', 'postgres.index_size', 'line'],
        'lines': [
            ['index_size', 'Size', 'absolute', 1, 1024*1024]
        ]},
    'table_count': {
        'options': [None, 'Total table', 'Count', 'Table', 'postgres.table_count', 'line'],
        'lines': [
            ['table_count', 'Total table', 'absolute']
        ]},
    'table_size': {
        'options': [None, 'Table size', 'MB', 'Table', 'postgres.table_size', 'line'],
        'lines': [
            ['table_size', 'Size', 'absolute', 1, 1024*1024]
        ]},
    'locks': {
        'options': [None, 'Table size', 'Count', 'Locks', 'postgres.locks', 'line'],
        'lines': [
            ['lock_access_exclusive', 'Access Exclusive', 'absolute'],
            ['lock_access_share', 'Access Share', 'absolute'],
            ['lock_exclusive', 'Exclusive', 'absolute'],
            ['lock_row_exclusive', 'Row Exclusive', 'absolute'],
            ['lock_row_share', 'Row Share', 'absolute'],
            ['lock_update_exclusive_lock', 'Update Exclusive Lock', 'absolute'],
            ['lock_share', 'Share', 'absolute'],
            ['lock_share_row_exclusive', 'Share Row Exclusive', 'absolute'],
            ['lock_si_read', 'SI Read', 'absolute']
        ]},
    'wal': {
        'options': [None, 'WAL stats', 'Files', 'WAL', 'postgres.wal', 'line'],
        'lines': [
            ['wal_total', 'Total', 'absolute'],
            ['wal_ready', 'Ready', 'absolute'],
            ['wal_done', 'Done', 'absolute']
        ]},
    'operations_heap': {
        'options': [None, 'Heap', 'iops', 'IO Operations', 'postgres.operations_heap', 'line'],
        'lines': [
            ['operations_heap_blocks_read', 'Read', 'absolute'],
            ['operations_heap_blocks_hit', 'Hit', 'absolute']
        ]},
    'operations_index': {
        'options': [None, 'Index', 'iops', 'IO Operations', 'postgres.operations_index', 'line'],
        'lines': [
            ['operations_index_blocks_read', 'Read', 'absolute'],
            ['operations_index_blocks_hit', 'Hit', 'absolute']
        ]},
    'operations_toast': {
        'options': [None, 'Toast', 'iops', 'IO Operations', 'postgres.operations_toast', 'line'],
        'lines': [
            ['operations_toast_blocks_read', 'Read', 'absolute'],
            ['operations_toast_blocks_hit', 'Hit', 'absolute']
        ]},
    'operations_toast_index': {
        'options': [None, 'Toast index', 'iops', 'IO Operations', 'postgres.operations_toast_index', 'line'],
        'lines': [
            ['operations_toastindex_blocks_read', 'Read', 'absolute'],
            ['operations_toastindex_blocks_hit', 'Hit', 'absolute']
        ]},
    'background_writer': {
        'options': [None, 'Checkpoints', 'Count', 'Background Writer', 'postgres.background_writer', 'line'],
        'lines': [
            ['background_writer_scheduled', 'Scheduled', 'absolute'],
            ['background_writer_requested', 'Requested', 'absolute']
        ]}
}

class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        super(self.__class__, self).__init__(configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.configuration = configuration
        self.connection = None
        self.data = {}
        self.old_data = {}

    def connect(self):
        params = dict(user='postgres',
                      database=None,
                      password=None,
                      host='localhost',
                      port=5432)
        params.update(self.configuration)
        self.connection = psycopg2.connect(**params)
        self.connection.set_isolation_level(extensions.ISOLATION_LEVEL_AUTOCOMMIT)

    def check(self):
        try:
            self.connect()
            return True
        except Exception as e:
            self.error(e)
            return False

    def _get_data(self):
        self.connect()

        cursor = self.connection.cursor(cursor_factory=DictCursor)
        self.add_stats(cursor)

        cursor.close()
        self.connection.close()

        return self.data;

    def add_stats(self, cursor):
        self.add_database_stats(cursor)
        self.add_backend_stats(cursor)
        self.add_index_stats(cursor)
        self.add_table_stats(cursor)
        self.add_lock_stats(cursor)
        self.add_statio_stats(cursor)
        self.add_bgwriter_stats(cursor)

        #self.add_replication_stats(cursor)

        # add_wal_metrics needs superuser to get directory listings
        #if self.config.get('superuser', True):
        #self.add_wal_stats(cursor)

    def add_database_stats(self, cursor):
        cursor.execute(DATABASE)
        temp = cursor.fetchone()

        self.add_derive_value('db_stat_xact_commit', int(temp.get('xact_commit', 0)))
        self.add_derive_value('db_stat_xact_rollback', int(temp.get('xact_rollback', 0)))
        self.add_derive_value('db_stat_blks_read', int(temp.get('blks_read', 0)))
        self.add_derive_value('db_stat_blks_hit', int(temp.get('blks_hit', 0)))
        self.add_derive_value('db_stat_tup_returned', int(temp.get('tup_returned', 0)))
        self.add_derive_value('db_stat_tup_fetched', int(temp.get('tup_fetched', 0)))
        self.add_derive_value('db_stat_tup_inserted', int(temp.get('tup_inserted', 0)))
        self.add_derive_value('db_stat_tup_updated', int(temp.get('tup_updated', 0)))
        self.add_derive_value('db_stat_tup_deleted', int(temp.get('tup_deleted', 0)))
        self.add_derive_value('db_stat_conflicts', int(temp.get('conflicts', 0)))

    def add_backend_stats(self, cursor):
        cursor.execute(BACKENDS)
        temp = cursor.fetchone()

        self.data['backend_process_active'] = int(temp.get('backends_active', 0))
        self.data['backend_process_idle'] = int(temp.get('backends_idle', 0))

    def add_index_stats(self, cursor):
        cursor.execute(INDEX_COUNT)
        temp = cursor.fetchone()
        self.data['index_count'] = int(temp.get('indexes', 0))

        cursor.execute(INDEX_SIZE_ON_DISK)
        temp = cursor.fetchone()
        self.data['index_size'] = int(temp.get('size_indexes', 0))

    def add_table_stats(self, cursor):
        cursor.execute(TABLE_COUNT)
        temp = cursor.fetchone()
        self.data['table_count'] = int(temp.get('relations', 0))

        cursor.execute(TABLE_SIZE_ON_DISK)
        temp = cursor.fetchone()
        self.data['table_size'] = int(temp.get('size_relations', 0))

    def add_lock_stats(self, cursor):
        cursor.execute(LOCKS)
        temp = cursor.fetchall()
        for key in LOCK_MAP:
            found = False
            for row in temp:
                if row['mode'] == key:
                    found = True
                    self.data[LOCK_MAP[key]] = int(row['count'])

            if not found:
                    self.data[LOCK_MAP[key]] = 0

    def add_wal_stats(self, cursor):
        cursor.execute(ARCHIVE)
        temp = cursor.fetchone()
        self.add_derive_value('wal_total', int(temp.get('file_count', 0)))
        self.add_derive_value('wal_ready', int(temp.get('ready_count', 0)))
        self.add_derive_value('wal_done', int(temp.get('done_count', 0)))

    def add_statio_stats(self, cursor):
        cursor.execute(STATIO)
        temp = cursor.fetchone()
        self.add_derive_value('operations_heap_blocks_read', int(temp.get('heap_blocks_read', 0)))
        self.add_derive_value('operations_heap_blocks_hit', int(temp.get('heap_blocks_hit', 0)))
        self.add_derive_value('operations_index_blocks_read', int(temp.get('index_blocks_read', 0)))
        self.add_derive_value('operations_index_blocks_hit', int(temp.get('index_blocks_hit', 0)))
        self.add_derive_value('operations_toast_blocks_read', int(temp.get('toast_blocks_read', 0)))
        self.add_derive_value('operations_toast_blocks_hit', int(temp.get('toast_blocks_hit', 0)))
        self.add_derive_value('operations_toastindex_blocks_read', int(temp.get('toastindex_blocks_read', 0)))
        self.add_derive_value('operations_toastindex_blocks_hit', int(temp.get('toastindex_blocks_hit', 0)))

    def add_bgwriter_stats(self, cursor):
        cursor.execute(BGWRITER)
        temp = cursor.fetchone()

        self.add_derive_value('background_writer_scheduled', temp.get('checkpoints_timed', 0))
        self.add_derive_value('background_writer_requested', temp.get('checkpoints_requests', 0))

    def add_derive_value(self, key, value):
        if key not in self.old_data.keys():
            self.data[key] = 0
        else:
            self.data[key] = value - self.old_data[key]

        self.old_data[key] = value


'''
    def add_replication_stats(self, cursor):
        cursor.execute(REPLICATION)
        temp = cursor.fetchall()
        for row in temp:
            self.add_gauge_value('Replication/%s' % row.get('client_addr', 'Unknown'),
                                 'byte_lag',
                                 int(row.get('byte_lag', 0)))
'''
