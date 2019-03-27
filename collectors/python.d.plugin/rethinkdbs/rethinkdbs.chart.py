# -*- coding: utf-8 -*-
# Description: rethinkdb netdata python.d module
# Author: Ilya Mashchenko (ilyam8)
# SPDX-License-Identifier: GPL-3.0-or-later

try:
    import rethinkdb as rdb
    HAS_RETHINKDB = True
except ImportError:
    HAS_RETHINKDB = False

from bases.FrameworkServices.SimpleService import SimpleService

ORDER = [
    'cluster_connected_servers',
    'cluster_clients_active',
    'cluster_queries',
    'cluster_documents',
]


def cluster_charts():
    return {
        'cluster_connected_servers': {
            'options': [None, 'Connected Servers', 'servers', 'cluster', 'rethinkdb.cluster_connected_servers',
                        'stacked'],
            'lines': [
                ['cluster_servers_connected', 'connected'],
                ['cluster_servers_missing', 'missing'],
            ]
        },
        'cluster_clients_active': {
            'options': [None, 'Active Clients', 'clients', 'cluster', 'rethinkdb.cluster_clients_active',
                        'line'],
            'lines': [
                ['cluster_clients_active', 'active'],
            ]
        },
        'cluster_queries': {
            'options': [None, 'Queries', 'queries/s', 'cluster', 'rethinkdb.cluster_queries', 'line'],
            'lines': [
                ['cluster_queries_per_sec', 'queries'],
            ]
        },
        'cluster_documents': {
            'options': [None, 'Documents', 'documents/s', 'cluster', 'rethinkdb.cluster_documents', 'line'],
            'lines': [
                ['cluster_read_docs_per_sec', 'reads'],
                ['cluster_written_docs_per_sec', 'writes'],
            ]
        },
    }


def server_charts(n):
    o = [
        '{0}_client_connections'.format(n),
        '{0}_clients_active'.format(n),
        '{0}_queries'.format(n),
        '{0}_documents'.format(n),
    ]
    f = 'server {0}'.format(n)

    c = {
        o[0]: {
            'options': [None, 'Client Connections', 'connections', f, 'rethinkdb.client_connections', 'line'],
            'lines': [
                ['{0}_client_connections'.format(n), 'connections'],
            ]
        },
        o[1]: {
            'options': [None, 'Active Clients', 'clients', f, 'rethinkdb.clients_active', 'line'],
            'lines': [
                ['{0}_clients_active'.format(n), 'active'],
            ]
        },
        o[2]: {
            'options': [None, 'Queries', 'queries/s', f, 'rethinkdb.queries', 'line'],
            'lines': [
                ['{0}_queries_total'.format(n), 'queries', 'incremental'],
            ]
        },
        o[3]: {
            'options': [None, 'Documents', 'documents/s', f, 'rethinkdb.documents', 'line'],
            'lines': [
                ['{0}_read_docs_total'.format(n), 'reads', 'incremental'],
                ['{0}_written_docs_total'.format(n), 'writes', 'incremental'],
            ]
        },
    }

    return o, c


class Cluster:
    def __init__(self, raw):
        self.raw = raw

    def data(self):
        qe = self.raw['query_engine']

        return {
            'cluster_clients_active': qe['clients_active'],
            'cluster_queries_per_sec': qe['queries_per_sec'],
            'cluster_read_docs_per_sec': qe['read_docs_per_sec'],
            'cluster_written_docs_per_sec': qe['written_docs_per_sec'],
            'cluster_servers_connected': 0,
            'cluster_servers_missing': 0,
        }


class Server:
    def __init__(self, raw):
        self.name = raw['server']
        self.raw = raw

    def error(self):
        return self.raw.get('error')

    def data(self):
        qe = self.raw['query_engine']

        d = {
            'client_connections': qe['client_connections'],
            'clients_active': qe['clients_active'],
            'queries_total': qe['queries_total'],
            'read_docs_total': qe['read_docs_total'],
            'written_docs_total': qe['written_docs_total'],
        }

        return dict(('{0}_{1}'.format(self.name, k), d[k]) for k in d)


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.order = list(ORDER)
        self.definitions = cluster_charts()
        self.host = self.configuration.get('host', '127.0.0.1')
        self.port = self.configuration.get('port', 28015)
        self.user = self.configuration.get('user', 'admin')
        self.password = self.configuration.get('password')
        self.timeout = self.configuration.get('timeout', 2)
        self.conn = None
        self.alive = True

    def check(self):
        if not HAS_RETHINKDB:
            self.error('"rethinkdb" module is needed to use rethinkdbs.py')
            return False

        if not self.connect():
            return None

        stats = self.get_stats()

        if not stats:
            return None

        for v in stats[1:]:
            if get_id(v) == 'server':
                o, c = server_charts(v['server'])
                self.order.extend(o)
                self.definitions.update(c)

        return True

    def get_data(self):
        if not self.is_alive():
            return None

        stats = self.get_stats()

        if not stats:
            return None

        data = dict()

        # cluster
        data.update(Cluster(stats[0]).data())

        # servers
        for v in stats[1:]:
            if get_id(v) != 'server':
                continue

            s = Server(v)

            if s.error():
                data['cluster_servers_missing'] += 1
            else:
                data['cluster_servers_connected'] += 1
                data.update(s.data())

        return data

    def get_stats(self):
        try:
            return list(rdb.db('rethinkdb').table('stats').run(self.conn).items)
        except rdb.errors.ReqlError:
            self.alive = False
            return None

    def connect(self):
        try:
            self.conn = rdb.connect(
                host=self.host,
                port=self.port,
                user=self.user,
                password=self.password,
                timeout=self.timeout,
            )
            self.alive = True
            return True
        except rdb.errors.ReqlError as error:
            self.error('Connection to {0}:{1} failed: {2}'.format(self.host, self.port, error))
            return False

    def reconnect(self):
        # The connection is already closed after rdb.errors.ReqlError,
        #  so we do not need to call conn.close()
        if self.connect():
            return True
        return False

    def is_alive(self):
        if not self.alive:
            return self.reconnect()
        return True


def get_id(v):
    return v['id'][0]
