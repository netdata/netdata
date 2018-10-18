# -*- coding: utf-8 -*-
# Description: example netdata python.d module
# Authors: facetoe, dangtranhoang
# SPDX-License-Identifier: GPL-3.0-or-later

from copy import deepcopy

try:
    import psycopg2
    from psycopg2 import extensions
    from psycopg2.extras import DictCursor
    from psycopg2 import OperationalError
    PSYCOPG2 = True
except ImportError:
    PSYCOPG2 = False

from bases.FrameworkServices.SimpleService import SimpleService

# default module values
update_every = 1
priority = 60000
retries = 60

METRICS = {
    'DATABASE': [
        'connections',
        'xact_commit',
        'xact_rollback',
        'blks_read',
        'blks_hit',
        'tup_returned',
        'tup_fetched',
        'tup_inserted',
        'tup_updated',
        'tup_deleted',
        'conflicts',
        'temp_files',
        'temp_bytes',
        'size'
    ],
    'BACKENDS': [
        'backends_active',
        'backends_idle'
    ],
    'INDEX_STATS': [
        'index_count',
        'index_size'
    ],
    'TABLE_STATS': [
        'table_size',
        'table_count'
    ],
    'WAL': [
        'written_wal',
        'recycled_wal',
        'total_wal'
    ],
    'WAL_WRITES': [
        'wal_writes'
    ],
    'ARCHIVE': [
        'ready_count',
        'done_count',
        'file_count'
    ],
    'BGWRITER': [
        'checkpoint_scheduled',
        'checkpoint_requested',
        'buffers_checkpoint',
        'buffers_clean',
        'maxwritten_clean',
        'buffers_backend',
        'buffers_alloc',
        'buffers_backend_fsync'
    ],
    'LOCKS': [
        'ExclusiveLock',
        'RowShareLock',
        'SIReadLock',
        'ShareUpdateExclusiveLock',
        'AccessExclusiveLock',
        'AccessShareLock',
        'ShareRowExclusiveLock',
        'ShareLock',
        'RowExclusiveLock'
    ],
    'AUTOVACUUM': [
        'analyze',
        'vacuum_analyze',
        'vacuum',
        'vacuum_freeze',
        'brin_summarize'
    ],
    'STANDBY_DELTA': [
        'sent_delta',
        'write_delta',
        'flush_delta',
        'replay_delta'
    ],
    'REPSLOT_FILES': [
        'replslot_wal_keep',
        'replslot_files'
    ]
}

QUERIES = {
    'WAL': """
SELECT
    count(*) as total_wal,
    count(*) FILTER (WHERE type = 'recycled') AS recycled_wal,
    count(*) FILTER (WHERE type = 'written') AS written_wal
FROM
    (SELECT
        wal.name,
        pg_{0}file_name(
          CASE pg_is_in_recovery()
            WHEN true THEN NULL
            ELSE pg_current_{0}_{1}()
          END ),
        CASE
          WHEN wal.name > pg_{0}file_name(
            CASE pg_is_in_recovery()
              WHEN true THEN NULL
              ELSE pg_current_{0}_{1}()
            END ) THEN 'recycled'
          ELSE 'written'
        END AS type
    FROM pg_catalog.pg_ls_dir('pg_{0}') AS wal(name)
    WHERE name ~ '^[0-9A-F]{{24}}$'
    ORDER BY
        (pg_stat_file('pg_{0}/'||name)).modification,
        wal.name DESC) sub;
""",
    'ARCHIVE': """
SELECT
    CAST(COUNT(*) AS INT) AS file_count,
    CAST(COALESCE(SUM(CAST(archive_file ~ $r$\.ready$$r$ as INT)),0) AS INT) AS ready_count,
    CAST(COALESCE(SUM(CAST(archive_file ~ $r$\.done$$r$ AS INT)),0) AS INT) AS done_count
FROM
    pg_catalog.pg_ls_dir('pg_{0}/archive_status') AS archive_files (archive_file);
""",
    'BACKENDS': """
SELECT
    count(*) - (SELECT  count(*)
                FROM pg_stat_activity
                WHERE state = 'idle')
      AS backends_active,
    (SELECT count(*)
     FROM pg_stat_activity
     WHERE state = 'idle')
      AS backends_idle
FROM pg_stat_activity;
""",
    'TABLE_STATS': """
SELECT
    ((sum(relpages) * 8) * 1024) AS table_size,
    count(1)                     AS table_count
FROM pg_class
WHERE relkind IN ('r', 't');
""",
    'INDEX_STATS': """
SELECT
    ((sum(relpages) * 8) * 1024) AS index_size,
    count(1)                     AS index_count
FROM pg_class
WHERE relkind = 'i';
""",
    'DATABASE': """
SELECT
    datname AS database_name,
    numbackends AS connections,
    xact_commit AS xact_commit,
    xact_rollback AS xact_rollback,
    blks_read AS blks_read,
    blks_hit AS blks_hit,
    tup_returned AS tup_returned,
    tup_fetched AS tup_fetched,
    tup_inserted AS tup_inserted,
    tup_updated AS tup_updated,
    tup_deleted AS tup_deleted,
    conflicts AS conflicts,
    pg_database_size(datname) AS size,
    temp_files AS temp_files,
    temp_bytes AS temp_bytes
FROM pg_stat_database
WHERE datname IN %(databases)s ;
""",
    'BGWRITER': """
SELECT
    checkpoints_timed AS checkpoint_scheduled,
    checkpoints_req AS checkpoint_requested,
    buffers_checkpoint * current_setting('block_size')::numeric buffers_checkpoint,
    buffers_clean * current_setting('block_size')::numeric buffers_clean,
    maxwritten_clean,
    buffers_backend * current_setting('block_size')::numeric buffers_backend,
    buffers_alloc * current_setting('block_size')::numeric buffers_alloc,
    buffers_backend_fsync
FROM pg_stat_bgwriter;
""",
    'LOCKS': """
SELECT
    pg_database.datname as database_name,
    mode,
    count(mode) AS locks_count
FROM pg_locks
INNER JOIN pg_database
    ON pg_database.oid = pg_locks.database
GROUP BY datname, mode
ORDER BY datname, mode;
""",
    'FIND_DATABASES': """
SELECT
    datname
FROM pg_stat_database
WHERE
    has_database_privilege(
      (SELECT current_user), datname, 'connect')
    AND NOT datname ~* '^template\d ';
""",
    'FIND_STANDBY': """
SELECT
    application_name
FROM pg_stat_replication
WHERE application_name IS NOT NULL
GROUP BY application_name;
""",
    'FIND_REPLICATION_SLOT': """
SELECT slot_name
FROM pg_replication_slots;
""",
    'STANDBY_DELTA': """
SELECT
    application_name,
    pg_{0}_{1}_diff(
      CASE pg_is_in_recovery()
        WHEN true THEN pg_last_{0}_receive_{1}()
        ELSE pg_current_{0}_{1}()
      END,
    sent_{1}) AS sent_delta,
    pg_{0}_{1}_diff(
      CASE pg_is_in_recovery()
        WHEN true THEN pg_last_{0}_receive_{1}()
        ELSE pg_current_{0}_{1}()
      END,
    write_{1}) AS write_delta,
    pg_{0}_{1}_diff(
      CASE pg_is_in_recovery()
        WHEN true THEN pg_last_{0}_receive_{1}()
        ELSE pg_current_{0}_{1}()
      END,
    flush_{1}) AS flush_delta,
    pg_{0}_{1}_diff(
      CASE pg_is_in_recovery()
        WHEN true THEN pg_last_{0}_receive_{1}()
        ELSE pg_current_{0}_{1}()
      END,
    replay_{1}) AS replay_delta
FROM pg_stat_replication
WHERE application_name IS NOT NULL;
""",
    'REPSLOT_FILES': """
WITH wal_size AS (
  SELECT
    current_setting('wal_block_size')::INT * setting::INT AS val
  FROM pg_settings
  WHERE name = 'wal_segment_size'
  )
SELECT
    slot_name,
    slot_type,
    replslot_wal_keep,
    count(slot_file) AS replslot_files
FROM
    (SELECT
        slot.slot_name,
        CASE
            WHEN slot_file <> 'state' THEN 1
        END AS slot_file ,
        slot_type,
        COALESCE (
          floor(
            (pg_wal_lsn_diff(pg_current_wal_lsn (),slot.restart_lsn)
             - (pg_walfile_name_offset (restart_lsn)).file_offset) / (s.val)
          ),0) AS replslot_wal_keep
    FROM pg_replication_slots slot
    LEFT JOIN (
        SELECT
            slot2.slot_name,
            pg_ls_dir('pg_replslot/' || slot2.slot_name) AS slot_file
        FROM pg_replication_slots slot2
        ) files (slot_name, slot_file)
        ON slot.slot_name = files.slot_name
    CROSS JOIN wal_size s
    ) AS d
GROUP BY
    slot_name,
    slot_type,
    replslot_wal_keep;
""",
    'IF_SUPERUSER': """
SELECT current_setting('is_superuser') = 'on' AS is_superuser;
""",
    'DETECT_SERVER_VERSION': """
SHOW server_version_num;
""",
    'AUTOVACUUM': """
SELECT
    count(*) FILTER (WHERE query LIKE 'autovacuum: ANALYZE%%') AS analyze,
    count(*) FILTER (WHERE query LIKE 'autovacuum: VACUUM ANALYZE%%') AS vacuum_analyze,
    count(*) FILTER (WHERE query LIKE 'autovacuum: VACUUM%%'
                       AND query NOT LIKE 'autovacuum: VACUUM ANALYZE%%'
                       AND query NOT LIKE '%%to prevent wraparound%%') AS vacuum,
    count(*) FILTER (WHERE query LIKE '%%to prevent wraparound%%') AS vacuum_freeze,
    count(*) FILTER (WHERE query LIKE 'autovacuum: BRIN summarize%%') AS brin_summarize
FROM pg_stat_activity
WHERE query NOT LIKE '%%pg_stat_activity%%';
""",
    'DIFF_LSN': """
SELECT
    pg_{0}_{1}_diff(
      CASE pg_is_in_recovery()
        WHEN true THEN pg_last_{0}_receive_{1}()
        ELSE pg_current_{0}_{1}()
      END,
    '0/0') as wal_writes ;
"""
}


QUERY_STATS = {
    QUERIES['DATABASE']: METRICS['DATABASE'],
    QUERIES['BACKENDS']: METRICS['BACKENDS'],
    QUERIES['LOCKS']: METRICS['LOCKS']
}

ORDER = [
    'db_stat_temp_files',
    'db_stat_temp_bytes',
    'db_stat_blks',
    'db_stat_tuple_returned',
    'db_stat_tuple_write',
    'db_stat_transactions',
    'db_stat_connections',
    'database_size',
    'backend_process',
    'index_count',
    'index_size',
    'table_count',
    'table_size',
    'wal',
    'wal_writes',
    'archive_wal',
    'checkpointer',
    'stat_bgwriter_alloc',
    'stat_bgwriter_checkpoint',
    'stat_bgwriter_backend',
    'stat_bgwriter_backend_fsync',
    'stat_bgwriter_bgwriter',
    'stat_bgwriter_maxwritten',
    'replication_slot',
    'standby_delta',
    'autovacuum'
]

CHARTS = {
    'db_stat_transactions': {
        'options': [None, 'Transactions on db', 'transactions/s', 'db statistics', 'postgres.db_stat_transactions',
                    'line'],
        'lines': [
            ['xact_commit', 'committed', 'incremental'],
            ['xact_rollback', 'rolled back', 'incremental']
        ]
    },
    'db_stat_connections': {
        'options': [None, 'Current connections to db', 'count', 'db statistics', 'postgres.db_stat_connections',
                    'line'],
        'lines': [
            ['connections', 'connections', 'absolute']
        ]
    },
    'db_stat_blks': {
        'options': [None, 'Disk blocks reads from db', 'reads/s', 'db statistics', 'postgres.db_stat_blks', 'line'],
        'lines': [
            ['blks_read', 'disk', 'incremental'],
            ['blks_hit', 'cache', 'incremental']
        ]
    },
    'db_stat_tuple_returned': {
        'options': [None, 'Tuples returned from db', 'tuples/s', 'db statistics', 'postgres.db_stat_tuple_returned',
                    'line'],
        'lines': [
            ['tup_returned', 'sequential', 'incremental'],
            ['tup_fetched', 'bitmap', 'incremental']
        ]
    },
    'db_stat_tuple_write': {
        'options': [None, 'Tuples written to db', 'writes/s', 'db statistics', 'postgres.db_stat_tuple_write', 'line'],
        'lines': [
            ['tup_inserted', 'inserted', 'incremental'],
            ['tup_updated', 'updated', 'incremental'],
            ['tup_deleted', 'deleted', 'incremental'],
            ['conflicts', 'conflicts', 'incremental']
        ]
    },
    'db_stat_temp_bytes': {
        'options': [None, 'Temp files written to disk', 'KB/s', 'db statistics', 'postgres.db_stat_temp_bytes',
                    'line'],
        'lines': [
            ['temp_bytes', 'size', 'incremental', 1, 1024]
        ]
    },
    'db_stat_temp_files': {
        'options': [None, 'Temp files written to disk', 'files', 'db statistics', 'postgres.db_stat_temp_files',
                    'line'],
        'lines': [
            ['temp_files', 'files', 'incremental']
        ]
    },
    'database_size': {
        'options': [None, 'Database size', 'MB', 'database size', 'postgres.db_size', 'stacked'],
        'lines': [
        ]
    },
    'backend_process': {
        'options': [None, 'Current Backend Processes', 'processes', 'backend processes', 'postgres.backend_process',
                    'line'],
        'lines': [
            ['backends_active', 'active', 'absolute'],
            ['backends_idle', 'idle', 'absolute']
        ]
    },
    'index_count': {
        'options': [None, 'Total indexes', 'index', 'indexes', 'postgres.index_count', 'line'],
        'lines': [
            ['index_count', 'total', 'absolute']
        ]
    },
    'index_size': {
        'options': [None, 'Indexes size', 'MB', 'indexes', 'postgres.index_size', 'line'],
        'lines': [
            ['index_size', 'size', 'absolute', 1, 1024 * 1024]
        ]
    },
    'table_count': {
        'options': [None, 'Total Tables', 'tables', 'tables', 'postgres.table_count', 'line'],
        'lines': [
            ['table_count', 'total', 'absolute']
        ]
    },
    'table_size': {
        'options': [None, 'Tables size', 'MB', 'tables', 'postgres.table_size', 'line'],
        'lines': [
            ['table_size', 'size', 'absolute', 1, 1024 * 1024]
        ]
    },
    'wal': {
        'options': [None, 'Write-Ahead Logs', 'files', 'wal', 'postgres.wal', 'line'],
        'lines': [
            ['written_wal', 'written', 'absolute'],
            ['recycled_wal', 'recycled', 'absolute'],
            ['total_wal', 'total', 'absolute']
        ]
    },
    'wal_writes': {
        'options': [None, 'Write-Ahead Logs', 'kilobytes/s', 'wal_writes', 'postgres.wal_writes', 'line'],
        'lines': [
            ['wal_writes', 'writes', 'incremental', 1, 1024]
        ]
    },
    'archive_wal': {
        'options': [None, 'Archive Write-Ahead Logs', 'files/s', 'archive wal', 'postgres.archive_wal', 'line'],
        'lines': [
            ['file_count', 'total', 'incremental'],
            ['ready_count', 'ready', 'incremental'],
            ['done_count', 'done', 'incremental']
        ]
    },
    'checkpointer': {
        'options': [None, 'Checkpoints', 'writes', 'checkpointer', 'postgres.checkpointer', 'line'],
        'lines': [
            ['checkpoint_scheduled', 'scheduled', 'incremental'],
            ['checkpoint_requested', 'requested', 'incremental']
        ]
    },
    'stat_bgwriter_alloc': {
        'options': [None, 'Buffers allocated', 'kilobytes/s', 'bgwriter', 'postgres.stat_bgwriter_alloc', 'line'],
        'lines': [
            ['buffers_alloc', 'alloc', 'incremental', 1, 1024]
        ]
    },
    'stat_bgwriter_checkpoint': {
        'options': [None, 'Buffers written during checkpoints', 'kilobytes/s', 'bgwriter',
                    'postgres.stat_bgwriter_checkpoint', 'line'],
        'lines': [
            ['buffers_checkpoint', 'checkpoint', 'incremental', 1, 1024]
        ]
    },
    'stat_bgwriter_backend': {
        'options': [None, 'Buffers written directly by a backend', 'kilobytes/s', 'bgwriter',
                    'postgres.stat_bgwriter_backend', 'line'],
        'lines': [
            ['buffers_backend', 'backend', 'incremental', 1, 1024]
        ]
    },
    'stat_bgwriter_backend_fsync': {
        'options': [None, 'Fsync by backend', 'times', 'bgwriter', 'postgres.stat_bgwriter_backend_fsync', 'line'],
        'lines': [
            ['buffers_backend_fsync', 'backend fsync', 'incremental']
        ]
    },
    'stat_bgwriter_bgwriter': {
        'options': [None, 'Buffers written by the background writer', 'kilobytes/s', 'bgwriter',
                    'postgres.bgwriter_bgwriter', 'line'],
        'lines': [
            ['buffers_clean', 'clean', 'incremental', 1, 1024]
        ]
    },
    'stat_bgwriter_maxwritten': {
        'options': [None, 'Too many buffers written', 'times', 'bgwriter', 'postgres.stat_bgwriter_maxwritten',
                    'line'],
        'lines': [
            ['maxwritten_clean', 'maxwritten', 'incremental']
        ]
    },
    'autovacuum': {
        'options': [None, 'Autovacuum workers', 'workers', 'autovacuum', 'postgres.autovacuum', 'line'],
        'lines': [
            ['analyze', 'analyze', 'absolute'],
            ['vacuum', 'vacuum', 'absolute'],
            ['vacuum_analyze', 'vacuum analyze', 'absolute'],
            ['vacuum_freeze', 'vacuum freeze', 'absolute'],
            ['brin_summarize', 'brin summarize', 'absolute']
        ]
    },
    'standby_delta': {
        'options': [None, 'Standby delta', 'kilobytes', 'replication delta', 'postgres.standby_delta', 'line'],
        'lines': [
            ['sent_delta', 'sent delta', 'absolute', 1, 1024],
            ['write_delta', 'write delta', 'absolute', 1, 1024],
            ['flush_delta', 'flush delta', 'absolute', 1, 1024],
            ['replay_delta', 'replay delta', 'absolute', 1, 1024]
        ]
    },
    'replication_slot': {
        'options': [None, 'Replication slot files', 'files', 'replication slot', 'postgres.replication_slot', 'line'],
        'lines': [
            ['replslot_wal_keep', 'wal keeped', 'absolute'],
            ['replslot_files', 'pg_replslot files', 'absolute']
        ]
    }
}


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER[:]
        self.definitions = deepcopy(CHARTS)
        self.table_stats = configuration.pop('table_stats', False)
        self.index_stats = configuration.pop('index_stats', False)
        self.database_poll = configuration.pop('database_poll', None)
        self.configuration = configuration
        self.connection = False
        self.server_version = None
        self.data = dict()
        self.locks_zeroed = dict()
        self.databases = list()
        self.secondaries = list()
        self.replication_slots = list()
        self.queries = QUERY_STATS.copy()

    def _connect(self):
        params = dict(user='postgres',
                      database=None,
                      password=None,
                      host=None,
                      port=5432)
        params.update(self.configuration)

        if not self.connection:
            try:
                self.connection = psycopg2.connect(**params)
                self.connection.set_isolation_level(extensions.ISOLATION_LEVEL_AUTOCOMMIT)
                self.connection.set_session(readonly=True)
            except OperationalError as error:
                return False, str(error)
        return True, True

    def check(self):
        if not PSYCOPG2:
            self.error('\'python-psycopg2\' module is needed to use postgres.chart.py')
            return False
        result, error = self._connect()
        if not result:
            conf = dict((k, (lambda k, v: v if k != 'password' else '*****')(k, v))
                        for k, v in self.configuration.items())
            self.error('Failed to connect to %s. Error: %s' % (str(conf), error))
            return False
        try:
            cursor = self.connection.cursor()
            self.databases = discover_databases_(cursor, QUERIES['FIND_DATABASES'])
            is_superuser = check_if_superuser_(cursor, QUERIES['IF_SUPERUSER'])
            self.secondaries = discover_secondaries_(cursor, QUERIES['FIND_STANDBY'])
            self.server_version = detect_server_version(cursor, QUERIES['DETECT_SERVER_VERSION'])
            if self.server_version >= 94000:
                self.replication_slots = discover_replication_slots_(cursor, QUERIES['FIND_REPLICATION_SLOT'])
            cursor.close()

            if self.database_poll and isinstance(self.database_poll, str):
                self.databases = [dbase for dbase in self.databases if dbase in self.database_poll.split()] \
                                 or self.databases

            self.locks_zeroed = populate_lock_types(self.databases)
            self.add_additional_queries_(is_superuser)
            self.create_dynamic_charts_()
            return True
        except Exception as error:
            self.error(str(error))
            return False

    def add_additional_queries_(self, is_superuser):

        if self.server_version >= 100000:
            wal = 'wal'
            lsn = 'lsn'
        else:
            wal = 'xlog'
            lsn = 'location'
        self.queries[QUERIES['BGWRITER']] = METRICS['BGWRITER']
        self.queries[QUERIES['DIFF_LSN'].format(wal, lsn)] = METRICS['WAL_WRITES']
        self.queries[QUERIES['STANDBY_DELTA'].format(wal, lsn)] = METRICS['STANDBY_DELTA']

        if self.index_stats:
            self.queries[QUERIES['INDEX_STATS']] = METRICS['INDEX_STATS']
        if self.table_stats:
            self.queries[QUERIES['TABLE_STATS']] = METRICS['TABLE_STATS']
        if is_superuser:
            self.queries[QUERIES['ARCHIVE'].format(wal)] = METRICS['ARCHIVE']
            if self.server_version >= 90400:
                self.queries[QUERIES['WAL'].format(wal, lsn)] = METRICS['WAL']
            if self.server_version >= 100000:
                self.queries[QUERIES['REPSLOT_FILES']] = METRICS['REPSLOT_FILES']
        if self.server_version >= 90400:
            self.queries[QUERIES['AUTOVACUUM']] = METRICS['AUTOVACUUM']

    def create_dynamic_charts_(self):

        for database_name in self.databases[::-1]:
            self.definitions['database_size']['lines'].append(
                [database_name + '_size', database_name, 'absolute', 1, 1024 * 1024])
            for chart_name in [name for name in self.order if name.startswith('db_stat')]:
                    add_database_stat_chart_(order=self.order, definitions=self.definitions,
                                             name=chart_name, database_name=database_name)

            add_database_lock_chart_(order=self.order, definitions=self.definitions, database_name=database_name)

        for application_name in self.secondaries[::-1]:
            add_replication_delta_chart_(
                order=self.order,
                definitions=self.definitions,
                name='standby_delta',
                application_name=application_name)

        for slot_name in self.replication_slots[::-1]:
            add_replication_slot_chart_(
                order=self.order,
                definitions=self.definitions,
                name='replication_slot',
                slot_name=slot_name)

    def _get_data(self):
        result, error = self._connect()
        if result:
            cursor = self.connection.cursor(cursor_factory=DictCursor)
            try:
                self.data.update(self.locks_zeroed)
                for query, metrics in self.queries.items():
                    self.query_stats_(cursor, query, metrics)

            except OperationalError:
                self.connection = False
                cursor.close()
                return None
            else:
                cursor.close()
                return self.data
        else:
            return None

    def query_stats_(self, cursor, query, metrics):
        cursor.execute(query, dict(databases=tuple(self.databases)))
        for row in cursor:
            for metric in metrics:
                if 'database_name' in row:
                    dimension_id = '_'.join([row['database_name'], metric])
                elif 'application_name' in row:
                    dimension_id = '_'.join([row['application_name'], metric])
                elif 'slot_name' in row:
                    dimension_id = '_'.join([row['slot_name'], metric])
                else:
                    dimension_id = metric
                if metric in row:
                    if row[metric] is not None:
                        self.data[dimension_id] = int(row[metric])
                elif 'locks_count' in row:
                    self.data[dimension_id] = row['locks_count'] if metric == row['mode'] else 0


def discover_databases_(cursor, query):
    cursor.execute(query)
    result = list()
    for db in [database[0] for database in cursor]:
        if db not in result:
            result.append(db)
    return result


def discover_secondaries_(cursor, query):
    cursor.execute(query)
    result = list()
    for sc in [standby[0] for standby in cursor]:
        if sc not in result:
            result.append(sc)
    return result


def discover_replication_slots_(cursor, query):
    cursor.execute(query)
    result = list()
    for slot in [replication_slot[0] for replication_slot in cursor]:
        if slot not in result:
            result.append(slot)
    return result


def check_if_superuser_(cursor, query):
    cursor.execute(query)
    return cursor.fetchone()[0]


def detect_server_version(cursor, query):
    cursor.execute(query)
    return int(cursor.fetchone()[0])


def populate_lock_types(databases):
    result = dict()
    for database in databases:
        for lock_type in METRICS['LOCKS']:
            key = '_'.join([database, lock_type])
            result[key] = 0

    return result


def add_database_lock_chart_(order, definitions, database_name):
    def create_lines(database):
        result = list()
        for lock_type in METRICS['LOCKS']:
            dimension_id = '_'.join([database, lock_type])
            result.append([dimension_id, lock_type, 'absolute'])
        return result

    chart_name = database_name + '_locks'
    order.insert(-1, chart_name)
    definitions[chart_name] = {
            'options':
            [None, 'Locks on db: ' + database_name, 'locks', 'db ' + database_name, 'postgres.db_locks', 'line'],
            'lines': create_lines(database_name)
            }


def add_database_stat_chart_(order, definitions, name, database_name):
    def create_lines(database, lines):
        result = list()
        for line in lines:
            new_line = ['_'.join([database, line[0]])] + line[1:]
            result.append(new_line)
        return result

    chart_template = CHARTS[name]
    chart_name = '_'.join([database_name, name])
    order.insert(0, chart_name)
    name, title, units, family, context, chart_type = chart_template['options']
    definitions[chart_name] = {
               'options': [name, title + ': ' + database_name,  units, 'db ' + database_name, context,  chart_type],
               'lines': create_lines(database_name, chart_template['lines'])}


def add_replication_delta_chart_(order, definitions, name, application_name):
    def create_lines(standby, lines):
        result = list()
        for line in lines:
            new_line = ['_'.join([standby, line[0]])] + line[1:]
            result.append(new_line)
        return result

    chart_template = CHARTS[name]
    chart_name = '_'.join([application_name, name])
    position = order.index('database_size')
    order.insert(position, chart_name)
    name, title, units, family, context, chart_type = chart_template['options']
    definitions[chart_name] = {
               'options': [name, title + ': ' + application_name,  units, 'replication delta', context,  chart_type],
               'lines': create_lines(application_name, chart_template['lines'])}


def add_replication_slot_chart_(order, definitions, name, slot_name):
    def create_lines(slot, lines):
        result = list()
        for line in lines:
            new_line = ['_'.join([slot, line[0]])] + line[1:]
            result.append(new_line)
        return result

    chart_template = CHARTS[name]
    chart_name = '_'.join([slot_name, name])
    position = order.index('database_size')
    order.insert(position, chart_name)
    name, title, units, family, context, chart_type = chart_template['options']
    definitions[chart_name] = {
               'options': [name, title + ': ' + slot_name,  units, 'replication slot files', context,  chart_type],
               'lines': create_lines(slot_name, chart_template['lines'])}
