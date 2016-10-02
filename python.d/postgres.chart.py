# -*- coding: utf-8 -*-
# Description: example netdata python.d module
# Author: Pawel Krupa (paulfantom)

import psycopg2
from base import SimpleService
from psycopg2.extras import DictCursor

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

ORDER = ["Tuples", "Scans"]
CHARTS = {
    "Tuples": {
        'options': ["tuples", "PostgreSQL tuple access", "Tuples / sec", "tuples", "postgres.tuples", "line"],
        'lines': [
            ["inserted", "inserted", "incremental", 1, 1],
            ["seqread", "seqread", "incremental", 1, 1],
            ["hotupdated", "hotupdated", "incremental", 1, 1],
            ["deleted", "deleted", "incremental", -1, 1],
            ["updated", "updated", "incremental", 1, 1],
            ["idxfetch", "idxfetch", "incremental", 1, 1],
        ]},
    "Scans": {
        'options': ["scans", "PostgreSQL scan types", "Scans / sec", "scans", "postgres.scans", "line"],
        'lines': [
            ["sequential", "sequential", "incremental", 1, 1],
            ["index", "index", "incremental", 1, 1],
        ]}
}


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        super(self.__class__, self).__init__(configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.error(str(configuration))
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
            SELECT  COALESCE(sum(seq_tup_read),0) AS seqread,
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
        data = {k: float(v) for k, v in cursor.fetchone().items()}
        self.connection.commit()
        cursor.close()
        return data
