# -*- coding: utf-8 -*-

import psycopg2
from base import SimpleService
from psycopg2.extras import DictCursor

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

ORDER = ["Tuples", "Scans", "BGWriter", "BufferCache"]
CHARTS = {
    "Tuples": {
        'options': ["tuples", "PostgreSQL tuple access", "Tuples / sec", "tuples", "postgres.tuples", "line"],
        'lines': [
            ["inserted", "inserted", "incremental", 1, 1],
            ["seqread", "seqread", "incremental", 1, 1],
            ["hotupdated", "hotupdated", "incremental", 1, 1],
            ["deleted", "deleted", "incremental", 1, 1],
            ["updated", "updated", "incremental", 1, 1],
            ["idxfetch", "idxfetch", "incremental", 1, 1],
        ]},
    "Scans": {
        'options': ["scans", "PostgreSQL scan types", "Scans / sec", "scans", "postgres.scans", "line"],
        'lines': [
            ["sequential", "sequential", "incremental", 1, 1],
            ["index", "index", "incremental", 1, 1],
        ]},
    "BGWriter": {
        'options': ["bgwriter", "BG Writer Activity", "Buffers / sec", "bgwriter", "postgres.bgwriter", "line"],
        'lines': [
            ["buffers_alloc", "buffers_alloc", "incremental", 1, 1],
            ["buffers_clean", "buffers_clean", "incremental", 1, 1],
            ["buffers_checkpoint", "buffers_checkpoint", "incremental", 1, 1],
            ["buffers_backend", "buffers_backend", "incremental", 1, 1],
        ]},
    "BufferCache": {
        'options': ["buffer_cache", "Buffer Cache", "Buffers / sec", "buffer_cache", "postgres.buffer_cache", "line"],
        'lines': [
            ["blks_read", "blks_read", "incremental", 1, 1],
            ["blks_hit", "blks_hit", "incremental", 1, 1],
        ]}
}


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        super(self.__class__, self).__init__(configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.configuration = configuration
        self.connection = None

    def connect(self):
        params = dict(user='postgres',
                      database=None,
                      password=None,
                      host='localhost',
                      port=5432)
        params.update(self.configuration)
        if self.connection is None:
            self.connection = psycopg2.connect(**params)

    def check(self):
        try:
            self.connect()
            return True
        except Exception as e:
            self.error(e)
            return False

    def _get_data(self):
        cursor = self.connection.cursor(cursor_factory=DictCursor)
        cursor.execute("""
            SELECT
                    -- Tuples
                    COALESCE(sum(seq_tup_read),0) AS seqread,
                    COALESCE(sum(idx_tup_fetch),0) AS idxfetch,
                    COALESCE(sum(n_tup_ins),0) AS inserted,
                    COALESCE(sum(n_tup_upd),0) AS updated,
                    COALESCE(sum(n_tup_del),0) AS deleted,
                    COALESCE(sum(n_tup_hot_upd),0) AS hotupdated,

                    -- Scans
                    COALESCE(sum(seq_scan),0) AS sequential,
                    COALESCE(sum(idx_scan),0) AS index
            FROM pg_stat_user_tables
        """)
        graph_data = {k: float(v) for k, v in cursor.fetchone().items()}

        # Pull in BGWriter info
        cursor.execute("""
            SELECT
              buffers_checkpoint,
              buffers_clean,
              buffers_backend,
              buffers_alloc
            FROM
              pg_stat_bgwriter
        """)
        graph_data.update(dict(cursor.fetchone()))

        cursor.execute("""
            SELECT
              sum(blks_read) AS blks_read,
              sum(blks_hit) AS blks_hit
            FROM
              pg_stat_database
            WHERE
              datname = %(database)s
        """, self.configuration)
        graph_data.update({k: float(v) for k, v in cursor.fetchone().items()})

        self.connection.commit()
        cursor.close()
        return graph_data
