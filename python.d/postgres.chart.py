# -*- coding: utf-8 -*-
# Description: example netdata python.d module
# Authors: facetoe, dangtranhoang

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

METRICS = dict(
    DATABASE=['connections',
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
              'size'],
    BACKENDS=['backends_active',
              'backends_idle'],
    INDEX_STATS=['index_count',
                 'index_size'],
    TABLE_STATS=['table_size',
                 'table_count'],
    ARCHIVE=['ready_count',
             'done_count',
             'file_count'],
    BGWRITER=['writer_scheduled',
              'writer_requested'],
    LOCKS=['ExclusiveLock',
           'RowShareLock',
           'SIReadLock',
           'ShareUpdateExclusiveLock',
           'AccessExclusiveLock',
           'AccessShareLock',
           'ShareRowExclusiveLock',
           'ShareLock',
           'RowExclusiveLock']
)

QUERIES = dict(
    ARCHIVE="""
SELECT
    CAST(COUNT(*) AS INT) AS file_count,
    CAST(COALESCE(SUM(CAST(archive_file ~ $r$\.ready$$r$ as INT)), 0) AS INT) AS ready_count,
    CAST(COALESCE(SUM(CAST(archive_file ~ $r$\.done$$r$ AS INT)), 0) AS INT) AS done_count
FROM
    pg_catalog.pg_ls_dir('{0}/archive_status') AS archive_files (archive_file);
""",
    BACKENDS="""
SELECT
    count(*) - (SELECT count(*) FROM pg_stat_activity WHERE state = 'idle') AS backends_active,
    (SELECT count(*) FROM pg_stat_activity WHERE state = 'idle' ) AS backends_idle
FROM  pg_stat_activity;
""",
    TABLE_STATS="""
SELECT
  ((sum(relpages) * 8) * 1024) AS table_size,
  count(1)                     AS table_count
FROM pg_class
WHERE relkind IN ('r', 't');
""",
    INDEX_STATS="""
SELECT
  ((sum(relpages) * 8) * 1024) AS index_size,
  count(1)                     AS index_count
FROM pg_class
WHERE relkind = 'i';""",
    DATABASE="""
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
  sum(conflicts) AS conflicts,
  pg_database_size(datname) AS size
FROM pg_stat_database
WHERE datname IN %(databases)s
GROUP BY datname;
""",
    BGWRITER="""
SELECT
  checkpoints_timed AS writer_scheduled,
  checkpoints_req AS writer_requested
FROM pg_stat_bgwriter;""",
   LOCKS="""
SELECT
  pg_database.datname as database_name,
  mode,
  count(mode) AS locks_count
FROM pg_locks
  INNER JOIN pg_database ON pg_database.oid = pg_locks.database
GROUP BY datname, mode
ORDER BY datname, mode;
""",
    FIND_DATABASES="""
SELECT datname
FROM pg_stat_database
WHERE has_database_privilege((SELECT current_user), datname, 'connect')
AND NOT datname ~* '^template\d+';
""",
    IF_SUPERUSER="""
SELECT current_setting('is_superuser') = 'on' AS is_superuser;
    """,
    DETECT_SERVER_VERSION="""
SHOW server_version_num;
    """
)


QUERY_STATS = {
    QUERIES['DATABASE']: METRICS['DATABASE'],
    QUERIES['BACKENDS']: METRICS['BACKENDS'],
    QUERIES['LOCKS']: METRICS['LOCKS']
}

ORDER = ['db_stat_transactions', 'db_stat_tuple_read', 'db_stat_tuple_returned', 'db_stat_tuple_write', 'database_size',
         'backend_process', 'index_count', 'index_size', 'table_count', 'table_size', 'wal', 'background_writer']

CHARTS = {
    'db_stat_transactions': {
        'options': [None, 'Transactions on db', 'transactions/s', 'db statistics', 'postgres.db_stat_transactions',
                    'line'],
        'lines': [
            ['xact_commit', 'committed', 'incremental'],
            ['xact_rollback', 'rolled back', 'incremental']
        ]},
    'db_stat_connections': {
        'options': [None, 'Current connections to db', 'count', 'db statistics', 'postgres.db_stat_connections',
                    'line'],
        'lines': [
            ['connections', 'connections', 'absolute']
        ]},
    'db_stat_tuple_read': {
        'options': [None, 'Tuple reads from db', 'reads/s', 'db statistics', 'postgres.db_stat_tuple_read', 'line'],
        'lines': [
            ['blks_read', 'disk', 'incremental'],
            ['blks_hit', 'cache', 'incremental']
        ]},
    'db_stat_tuple_returned': {
        'options': [None, 'Tuples returned from db', 'tuples/s', 'db statistics', 'postgres.db_stat_tuple_returned',
                    'line'],
        'lines': [
            ['tup_returned', 'sequential', 'incremental'],
            ['tup_fetched', 'bitmap', 'incremental']
        ]},
    'db_stat_tuple_write': {
        'options': [None, 'Tuples written to db', 'writes/s', 'db statistics', 'postgres.db_stat_tuple_write', 'line'],
        'lines': [
            ['tup_inserted', 'inserted', 'incremental'],
            ['tup_updated', 'updated', 'incremental'],
            ['tup_deleted', 'deleted', 'incremental'],
            ['conflicts', 'conflicts', 'incremental']
        ]},
    'database_size': {
        'options': [None, 'Database size', 'MB', 'database size', 'postgres.db_size', 'stacked'],
        'lines': [
        ]},
    'backend_process': {
        'options': [None, 'Current Backend Processes', 'processes', 'backend processes', 'postgres.backend_process',
                    'line'],
        'lines': [
            ['backends_active', 'active', 'absolute'],
            ['backends_idle', 'idle', 'absolute']
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
            ['file_count', 'total', 'incremental'],
            ['ready_count', 'ready', 'incremental'],
            ['done_count', 'done', 'incremental']
        ]},
    'background_writer': {
        'options': [None, 'Checkpoints', 'writes/s', 'background writer', 'postgres.background_writer', 'line'],
        'lines': [
            ['writer_scheduled', 'scheduled', 'incremental'],
            ['writer_requested', 'requested', 'incremental']
        ]}
}


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        super(self.__class__, self).__init__(configuration=configuration, name=name)
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
            self.server_version = detect_server_version(cursor, QUERIES['DETECT_SERVER_VERSION'])
            cursor.close()

            if self.database_poll and isinstance(self.database_poll, str):
                self.databases = [dbase for dbase in self.databases if dbase in self.database_poll.split()]\
                                 or self.databases

            self.locks_zeroed = populate_lock_types(self.databases)
            self.add_additional_queries_(is_superuser)
            self.create_dynamic_charts_()
            return True
        except Exception as error:
            self.error(str(error))
            return False

    def add_additional_queries_(self, is_superuser):
        if self.index_stats:
            self.queries[QUERIES['INDEX_STATS']] = METRICS['INDEX_STATS']
        if self.table_stats:
            self.queries[QUERIES['TABLE_STATS']] = METRICS['TABLE_STATS']
        if is_superuser:
            self.queries[QUERIES['BGWRITER']] = METRICS['BGWRITER']
            if self.server_version >= 100000:
                wal_dir_name = 'pg_wal'
            else:
                wal_dir_name = 'pg_xlog'
            self.queries[QUERIES['ARCHIVE'].format(wal_dir_name)] = METRICS['ARCHIVE']

    def create_dynamic_charts_(self):

        for database_name in self.databases[::-1]:
            self.definitions['database_size']['lines'].append([database_name + '_size',
                                                               database_name, 'absolute', 1, 1024 * 1024])
            for chart_name in [name for name in CHARTS if name.startswith('db_stat')]:
                    add_database_stat_chart_(order=self.order, definitions=self.definitions,
                                             name=chart_name, database_name=database_name)

            add_database_lock_chart_(order=self.order, definitions=self.definitions, database_name=database_name)

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
                dimension_id = '_'.join([row['database_name'], metric]) if 'database_name' in row else metric
                if metric in row:
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
