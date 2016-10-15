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
#    'database': postgres,
#    'user': 'postgres',
#    'password': None,
#    'host': 'localhost',
#    'port': 5432
# }

CHARTS = {
    "Tuples": {
        'options': ["tuples", "PostgreSQL tuple access", "Tuples / sec", "tuples", "postgres.tuples", "line"],
        'lines': [
            ["tup_inserted", "tup_inserted", "incremental", 1, 1],
            ["tup_inserted", "tup_inserted", "incremental", 1, 1],
            ["tup_fetched", "tup_fetched", "incremental", 1, 1],
            ["tup_updated", "tup_updated", "incremental", 1, 1],
            ["tup_deleted", "tup_deleted", "incremental", 1, 1],
        ]},
    "Transactions": {
        'options': ["transactions", "Transactions", "transactions / sec", "transactions", "postgres.transactions", "line"],
        'lines': [
            ["xact_commit", "xact_commit", "incremental", 1, 1],
            ["xact_rollback", "xact_rollback", "incremental", 1, 1],
        ]},
    "BlockAccess": {
        'options': ["block_access", "block_access", "Block / sec ", "block_access", "postgres.block_access", "line"],
        'lines': [
            ["blks_read", "blks_read", "incremental", 1, 1],
            ["blks_hit", "blks_hit", "incremental", 1, 1],
        ]},
    "Checkpoints": {
        'options': ["checkpoints", "Checkpoints", "Checkpoints", "checkpoints", "postgres.checkpoints", "line"],
        'lines': [
            ["bg_checkpoint_time", "bg_checkpoint_time", "absolute", 1, 1],
            ["bg_checkpoint_requested", "bg_checkpoint_requested", "absolute", 1, 1],
        ]},
    "Buffers": {
        'options': ["buffers", "buffers", "Buffer/ sec", "buffers", "postgres.buffers", "line"],
        'lines': [
            ["buffers_written", "buffers_written", "incremental", 1, 1],
            ["buffers_allocated", "buffers_allocated", "incremental", 1, 1],
        ]},
}
ORDER = ["Tuples", "Transactions", "BlockAccess", "Checkpoints", "Buffers"]


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        super(self.__class__, self).__init__(configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.configuration = configuration
        self.connection = None

    def connect(self):
        params = dict(user='postgres',
                      database='postgres',
                      password=None,
                      host=None,
                      port=5432)
        params.update(self.configuration)
        if self.connection is None:
            self.connection = psycopg2.connect(**params)
            self.connection.set_session(readonly=True)

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
              pg_stat_database.*,
              pg_stat_get_bgwriter_timed_checkpoints()     AS bg_checkpoint_time,
              pg_stat_get_bgwriter_requested_checkpoints() AS bg_checkpoint_requested,
              pg_stat_get_buf_written_backend()            AS buffers_written,
              pg_stat_get_buf_alloc()                      AS buffers_allocated
            FROM pg_stat_database
            WHERE datname = %(database)s
        """, self.configuration)
        graph_data = dict(cursor.fetchone())
        self.connection.commit()
        cursor.close()
        return graph_data
