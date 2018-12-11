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


WAL = 'WAL'
ARCHIVE = 'ARCHIVE'
BACKENDS = 'BACKENDS'
TABLE_STATS = 'TABLE_STATS'
INDEX_STATS = 'INDEX_STATS'
DATABASE = 'DATABASE'
BGWRITER = 'BGWRITER'
LOCKS = 'LOCKS'
DATABASES = 'DATABASES'
STANDBY = 'STANDBY'
REPLICATION_SLOT = 'REPLICATION_SLOT'
STANDBY_DELTA = 'STANDBY_DELTA'
REPSLOT_FILES = 'REPSLOT_FILES'
IF_SUPERUSER = 'IF_SUPERUSER'
SERVER_VERSION = 'SERVER_VERSION'
AUTOVACUUM = 'AUTOVACUUM'
DIFF_LSN = 'DIFF_LSN'
WAL_WRITES = 'WAL_WRITES'

METRICS = {
    DATABASE: [
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
    BACKENDS: [
        'backends_active',
        'backends_idle'
    ],
    INDEX_STATS: [
        'index_count',
        'index_size'
    ],
    TABLE_STATS: [
        'table_size',
        'table_count'
    ],
    WAL: [
        'written_wal',
        'recycled_wal',
        'total_wal'
    ],
    WAL_WRITES: [
        'wal_writes'
    ],
    ARCHIVE: [
        'ready_count',
        'done_count',
        'file_count'
    ],
    BGWRITER: [
        'checkpoint_scheduled',
        'checkpoint_requested',
        'buffers_checkpoint',
        'buffers_clean',
        'maxwritten_clean',
        'buffers_backend',
        'buffers_alloc',
        'buffers_backend_fsync'
    ],
    LOCKS: [
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
    AUTOVACUUM: [
        'analyze',
        'vacuum_analyze',
        'vacuum',
        'vacuum_freeze',
        'brin_summarize'
    ],
    STANDBY_DELTA: [
        'sent_delta',
        'write_delta',
        'flush_delta',
        'replay_delta'
    ],
    REPSLOT_FILES: [
        'replslot_wal_keep',
        'replslot_files'
    ]
}

NO_VERSION = 0
DEFAULT = 'DEFAULT'
V96 = 'V96'
V10 = 'V10'
V11 = 'V11'


QUERY_WAL = {
    DEFAULT: """
SELECT
    count(*) as total_wal,
    count(*) FILTER (WHERE type = 'recycled') AS recycled_wal,
    count(*) FILTER (WHERE type = 'written') AS written_wal
FROM
    (SELECT
        wal.name,
        pg_walfile_name(
          CASE pg_is_in_recovery()
            WHEN true THEN NULL
            ELSE pg_current_wal_lsn()
          END ),
        CASE
          WHEN wal.name > pg_walfile_name(
            CASE pg_is_in_recovery()
              WHEN true THEN NULL
              ELSE pg_current_wal_lsn()
            END ) THEN 'recycled'
          ELSE 'written'
        END AS type
    FROM pg_catalog.pg_ls_dir('pg_wal') AS wal(name)
    WHERE name ~ '^[0-9A-F]{{24}}$'
    ORDER BY
        (pg_stat_file('pg_wal/'||name)).modification,
        wal.name DESC) sub;
""",
    V96: """
SELECT
    count(*) as total_wal,
    count(*) FILTER (WHERE type = 'recycled') AS recycled_wal,
    count(*) FILTER (WHERE type = 'written') AS written_wal
FROM
    (SELECT
        wal.name,
        pg_xlogfile_name(
          CASE pg_is_in_recovery()
            WHEN true THEN NULL
            ELSE pg_current_xlog_location()
          END ),
        CASE
          WHEN wal.name > pg_xlogfile_name(
            CASE pg_is_in_recovery()
              WHEN true THEN NULL
              ELSE pg_current_xlog_location()
            END ) THEN 'recycled'
          ELSE 'written'
        END AS type
    FROM pg_catalog.pg_ls_dir('pg_xlog') AS wal(name)
    WHERE name ~ '^[0-9A-F]{{24}}$'
    ORDER BY
        (pg_stat_file('pg_xlog/'||name)).modification,
        wal.name DESC) sub;
""",
}

QUERY_ARCHIVE = {
    DEFAULT: """
SELECT
    CAST(COUNT(*) AS INT) AS file_count,
    CAST(COALESCE(SUM(CAST(archive_file ~ $r$\.ready$$r$ as INT)),0) AS INT) AS ready_count,
    CAST(COALESCE(SUM(CAST(archive_file ~ $r$\.done$$r$ AS INT)),0) AS INT) AS done_count
FROM
    pg_catalog.pg_ls_dir('pg_wal/archive_status') AS archive_files (archive_file);
""",
    V96: """
SELECT
    CAST(COUNT(*) AS INT) AS file_count,
    CAST(COALESCE(SUM(CAST(archive_file ~ $r$\.ready$$r$ as INT)),0) AS INT) AS ready_count,
    CAST(COALESCE(SUM(CAST(archive_file ~ $r$\.done$$r$ AS INT)),0) AS INT) AS done_count
FROM
    pg_catalog.pg_ls_dir('pg_xlog/archive_status') AS archive_files (archive_file);

""",
}

QUERY_BACKEND = {
    DEFAULT: """
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
}

QUERY_TABLE_STATS = {
    DEFAULT: """
SELECT
    ((sum(relpages) * 8) * 1024) AS table_size,
    count(1)                     AS table_count
FROM pg_class
WHERE relkind IN ('r', 't');
""",
}

QUERY_INDEX_STATS = {
    DEFAULT: """
SELECT
    ((sum(relpages) * 8) * 1024) AS index_size,
    count(1)                     AS index_count
FROM pg_class
WHERE relkind = 'i';
""",
}

QUERY_DATABASE = {
    DEFAULT: """
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
}

QUERY_BGWRITER = {
    DEFAULT: """
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
}

QUERY_LOCKS = {
    DEFAULT: """
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
}

QUERY_DATABASES = {
    DEFAULT: """
SELECT
    datname
FROM pg_stat_database
WHERE
    has_database_privilege(
      (SELECT current_user), datname, 'connect')
    AND NOT datname ~* '^template\d ';
""",
}

QUERY_STANDBY = {
    DEFAULT: """
SELECT
    application_name
FROM pg_stat_replication
WHERE application_name IS NOT NULL
GROUP BY application_name;
""",
}

QUERY_REPLICATION_SLOT = {
    DEFAULT: """
SELECT slot_name
FROM pg_replication_slots;
"""
}

QUERY_STANDBY_DELTA = {
    DEFAULT: """
SELECT
    application_name,
    pg_wal_lsn_diff(
      CASE pg_is_in_recovery()
        WHEN true THEN pg_last_wal_receive_lsn()
        ELSE pg_current_wal_lsn()
      END,
    sent_lsn) AS sent_delta,
    pg_wal_lsn_diff(
      CASE pg_is_in_recovery()
        WHEN true THEN pg_last_wal_receive_lsn()
        ELSE pg_current_wal_lsn()
      END,
    write_lsn) AS write_delta,
    pg_wal_lsn_diff(
      CASE pg_is_in_recovery()
        WHEN true THEN pg_last_wal_receive_lsn()
        ELSE pg_current_wal_lsn()
      END,
    flush_lsn) AS flush_delta,
    pg_wal_lsn_diff(
      CASE pg_is_in_recovery()
        WHEN true THEN pg_last_wal_receive_lsn()
        ELSE pg_current_wal_lsn()
      END,
    replay_lsn) AS replay_delta
FROM pg_stat_replication
WHERE application_name IS NOT NULL;
""",
    V96: """
SELECT
    application_name,
    pg_xlog_location_diff(
      CASE pg_is_in_recovery()
        WHEN true THEN pg_last_xlog_receive_location()
        ELSE pg_current_xlog_location()
      END,
    sent_location) AS sent_delta,
    pg_xlog_location_diff(
      CASE pg_is_in_recovery()
        WHEN true THEN pg_last_xlog_receive_location()
        ELSE pg_current_xlog_location()
      END,
    write_location) AS write_delta,
    pg_xlog_location_diff(
      CASE pg_is_in_recovery()
        WHEN true THEN pg_last_xlog_receive_location()
        ELSE pg_current_xlog_location()
      END,
    flush_location) AS flush_delta,
    pg_xlog_location_diff(
      CASE pg_is_in_recovery()
        WHEN true THEN pg_last_xlog_receive_location()
        ELSE pg_current_xlog_location()
      END,
    replay_location) AS replay_delta
FROM pg_stat_replication
WHERE application_name IS NOT NULL;
""",
}

QUERY_REPSLOT_FILES = {
    DEFAULT: """
WITH wal_size AS (
  SELECT
    setting::int AS val
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
    V10: """
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
}

QUERY_SUPERUSER = {
    DEFAULT: """
SELECT current_setting('is_superuser') = 'on' AS is_superuser;
""",
}

QUERY_SHOW_VERSION = {
    DEFAULT: """
SHOW server_version_num;
""",
}

QUERY_AUTOVACUUM = {
    DEFAULT: """
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
}

QUERY_DIFF_LSN = {
    DEFAULT: """
SELECT
    pg_wal_lsn_diff(
      CASE pg_is_in_recovery()
        WHEN true THEN pg_last_wal_receive_lsn()
        ELSE pg_current_wal_lsn()
      END,
    '0/0') as wal_writes ;
""",
    V96: """
SELECT
    pg_xlog_location_diff(
      CASE pg_is_in_recovery()
        WHEN true THEN pg_last_xlog_receive_location()
        ELSE pg_current_xlog_location()
      END,
    '0/0') as wal_writes ;
""",
}


def query_factory(name, version=NO_VERSION):
    if name == BACKENDS:
        return QUERY_BACKEND[DEFAULT]
    elif name == TABLE_STATS:
        return QUERY_TABLE_STATS[DEFAULT]
    elif name == INDEX_STATS:
        return QUERY_INDEX_STATS[DEFAULT]
    elif name == DATABASE:
        return QUERY_DATABASE[DEFAULT]
    elif name == BGWRITER:
        return QUERY_BGWRITER[DEFAULT]
    elif name == LOCKS:
        return QUERY_LOCKS[DEFAULT]
    elif name == DATABASES:
        return QUERY_DATABASES[DEFAULT]
    elif name == STANDBY:
        return QUERY_STANDBY[DEFAULT]
    elif name == REPLICATION_SLOT:
        return QUERY_REPLICATION_SLOT[DEFAULT]
    elif name == IF_SUPERUSER:
        return QUERY_SUPERUSER[DEFAULT]
    elif name == SERVER_VERSION:
        return QUERY_SHOW_VERSION[DEFAULT]
    elif name == AUTOVACUUM:
        return QUERY_AUTOVACUUM[DEFAULT]
    elif name == WAL:
        if version < 100000:
            return QUERY_WAL[V96]
        return QUERY_WAL[DEFAULT]
    elif name == ARCHIVE:
        if version < 100000:
            return QUERY_ARCHIVE[V96]
        return QUERY_ARCHIVE[DEFAULT]
    elif name == STANDBY_DELTA:
        if version < 100000:
            return QUERY_STANDBY_DELTA[V96]
        return QUERY_STANDBY_DELTA[DEFAULT]
    elif name == REPSLOT_FILES:
        if version < 110000:
            return QUERY_REPSLOT_FILES[V10]
        return QUERY_REPSLOT_FILES[DEFAULT]
    elif name == DIFF_LSN:
        if version < 100000:
            return QUERY_DIFF_LSN[V96]
        return QUERY_DIFF_LSN[DEFAULT]

    raise ValueError('unknown query')


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
        self.order = list(ORDER)
        self.definitions = deepcopy(CHARTS)

        self.do_table_stats = configuration.pop('table_stats', False)
        self.do_index_stats = configuration.pop('index_stats', False)
        self.databases_to_poll = configuration.pop('database_poll', None)
        self.configuration = configuration

        self.conn = None
        self.server_version = None
        self.is_superuser = False
        self.alive = False

        self.databases = list()
        self.secondaries = list()
        self.replication_slots = list()

        self.queries = dict()

        self.data = dict()

    def reconnect(self):
        return self.connect()

    def connect(self):
        if self.conn:
            self.conn.close()
            self.conn = None

        try:
            params = dict(user='postgres',
                          database=None,
                          password=None,
                          host=None,
                          port=5432)
            params.update(self.configuration)

            self.conn = psycopg2.connect(**params)
            self.conn.set_isolation_level(extensions.ISOLATION_LEVEL_AUTOCOMMIT)
            self.conn.set_session(readonly=True)
        except OperationalError as error:
            self.error(error)
            self.alive = False
        else:
            self.alive = True

        return self.alive

    def check(self):
        if not PSYCOPG2:
            self.error("'python-psycopg2' package is needed to use postgres module")
            return False

        if not self.connect():
            self.error('failed to connect to {0}'.format(hide_password(self.configuration)))
            return False

        try:
            self.check_queries()
        except Exception as error:
            self.error(error)
            return False

        self.populate_queries()
        self.create_dynamic_charts()

        return True

    def get_data(self):
        if not self.alive and not self.reconnect():
            return None

        try:
            cursor = self.conn.cursor(cursor_factory=DictCursor)

            self.data.update(zero_lock_types(self.databases))

            for query, metrics in self.queries.items():
                self.query_stats(cursor, query, metrics)

        except OperationalError:
            self.alive = False
            return None

        cursor.close()

        return self.data

    def query_stats(self, cursor, query, metrics):
        cursor.execute(query, dict(databases=tuple(self.databases)))

        for row in cursor:
            for metric in metrics:
                #  databases
                if 'database_name' in row:
                    dimension_id = '_'.join([row['database_name'], metric])
                #  secondaries
                elif 'application_name' in row:
                    dimension_id = '_'.join([row['application_name'], metric])
                # replication slots
                elif 'slot_name' in row:
                    dimension_id = '_'.join([row['slot_name'], metric])
                #  other
                else:
                    dimension_id = metric

                if metric in row:
                    if row[metric] is not None:
                        self.data[dimension_id] = int(row[metric])
                elif 'locks_count' in row:
                    if metric == row['mode']:
                        self.data[dimension_id] = row['locks_count']

    def check_queries(self):
        cursor = self.conn.cursor()

        self.server_version = detect_server_version(cursor, query_factory(SERVER_VERSION))
        self.debug('server version: {0}'.format(self.server_version))

        self.is_superuser = check_if_superuser(cursor, query_factory(IF_SUPERUSER))
        self.debug('superuser: {0}'.format(self.is_superuser))

        self.databases = discover(cursor, query_factory(DATABASES))
        self.debug('discovered databases {0}'.format(self.databases))
        if self.databases_to_poll:
            to_poll = self.databases_to_poll.split()
            self.databases = [db for db in self.databases if db in to_poll] or self.databases

        self.secondaries = discover(cursor, query_factory(STANDBY))
        self.debug('discovered secondaries: {0}'.format(self.secondaries))

        if self.server_version >= 94000:
            self.replication_slots = discover(cursor, query_factory(REPLICATION_SLOT))
            self.debug('discovered replication slots: {0}'.format(self.replication_slots))

        cursor.close()

    def populate_queries(self):
        self.queries[query_factory(DATABASE)] = METRICS[DATABASE]
        self.queries[query_factory(BACKENDS)] = METRICS[BACKENDS]
        self.queries[query_factory(LOCKS)] = METRICS[LOCKS]
        self.queries[query_factory(BGWRITER)] = METRICS[BGWRITER]
        self.queries[query_factory(DIFF_LSN, self.server_version)] = METRICS[WAL_WRITES]
        self.queries[query_factory(STANDBY_DELTA, self.server_version)] = METRICS[STANDBY_DELTA]

        if self.do_index_stats:
            self.queries[query_factory(INDEX_STATS)] = METRICS[INDEX_STATS]
        if self.do_table_stats:
            self.queries[query_factory(TABLE_STATS)] = METRICS[TABLE_STATS]

        if self.is_superuser:
            self.queries[query_factory(ARCHIVE, self.server_version)] = METRICS[ARCHIVE]

            if self.server_version >= 90400:
                self.queries[query_factory(WAL, self.server_version)] = METRICS[WAL]

            if self.server_version >= 100000:
                self.queries[query_factory(REPSLOT_FILES, self.server_version)] = METRICS[REPSLOT_FILES]

        if self.server_version >= 90400:
            self.queries[query_factory(AUTOVACUUM)] = METRICS[AUTOVACUUM]

    def create_dynamic_charts(self):
        for database_name in self.databases[::-1]:
            dim = [
                database_name + '_size',
                database_name,
                'absolute',
                1,
                1024 * 1024,
            ]
            self.definitions['database_size']['lines'].append(dim)
            for chart_name in [name for name in self.order if name.startswith('db_stat')]:
                    add_database_stat_chart(
                        order=self.order,
                        definitions=self.definitions,
                        name=chart_name,
                        database_name=database_name,
                    )
            add_database_lock_chart(
                order=self.order,
                definitions=self.definitions,
                database_name=database_name,
            )

        for application_name in self.secondaries[::-1]:
            add_replication_delta_chart(
                order=self.order,
                definitions=self.definitions,
                name='standby_delta',
                application_name=application_name,
            )

        for slot_name in self.replication_slots[::-1]:
            add_replication_slot_chart(
                order=self.order,
                definitions=self.definitions,
                name='replication_slot',
                slot_name=slot_name,
            )


def discover(cursor, query):
    cursor.execute(query)
    result = list()
    for v in [value[0] for value in cursor]:
        if v not in result:
            result.append(v)
    return result


def check_if_superuser(cursor, query):
    cursor.execute(query)
    return cursor.fetchone()[0]


def detect_server_version(cursor, query):
    cursor.execute(query)
    return int(cursor.fetchone()[0])


def zero_lock_types(databases):
    result = dict()
    for database in databases:
        for lock_type in METRICS['LOCKS']:
            key = '_'.join([database, lock_type])
            result[key] = 0

    return result


def hide_password(config):
    return dict((k, v if k != 'password' else '*****') for k, v in config.items())


def add_database_lock_chart(order, definitions, database_name):
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


def add_database_stat_chart(order, definitions, name, database_name):
    def create_lines(database, lines):
        result = list()
        for line in lines:
            new_line = ['_'.join([database, line[0]])] + line[1:]
            result.append(new_line)
        return result

    chart_template = CHARTS[name]
    chart_name = '_'.join([database_name, name])
    order.insert(0, chart_name)
    name, title, units, _, context, chart_type = chart_template['options']
    definitions[chart_name] = {
               'options': [name, title + ': ' + database_name,  units, 'db ' + database_name, context,  chart_type],
               'lines': create_lines(database_name, chart_template['lines'])}


def add_replication_delta_chart(order, definitions, name, application_name):
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


def add_replication_slot_chart(order, definitions, name, slot_name):
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
