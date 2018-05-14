# -*- coding: utf-8 -*-
# Description:  netdata python.d module for monitoring Neo4j
# Author: Jerome Baton, @wadael , copied from the example of Pawel Krupa (paulfantom)

from bases.FrameworkServices.SimpleService import SimpleService

try:
    from neo4j.v1 import GraphDatabase
    NEO4JDRIVER = True
except ImportError:
    NEO4JDRIVER = False


ORDER = ['monnodes','monlabels','monrels']

CHARTS = {
    'monnodes': {
        'options': [None, 'Nodes count', 'Nodes', 'Node count', 'neo4j', 'line'],
        'lines': [
             ['id', 'name', 'absolute', 1, 1, 'hidden']
        ]
    },
    'monrels': {
        'options': [None, 'Relations count', 'Nodes', 'Most present relations', 'neo4j', 'line'],
        'lines': [
             ['id', 'name', 'absolute', 1, 1, 'hidden']
        ]
    },
    'monlabels': {
        'options': [None, 'Relations count', 'Nodes', 'Most present labels', 'neo4j', 'line'],
        'lines': [
            ['id', 'name', 'absolute', 1, 1, 'hidden']
        ]
    }


}

update_every = 10
priority = 99000
retries = 10


class Service(SimpleService):

    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.neosession = None
        self.alive = True

    def __exit__(self, exc_type, exc_value, traceback):
       if NEO4JDRIVER:
            self.neosession.close()
            self.neodriver.close()

    def check(self):
        if not NEO4JDRIVER:
            self.error("the 'neo4j-driver' module is needed to use neo.chart.py. See the script python-modules-installer.sh in /usr/libexec/netdata/python.d")
            return False

        self.connect()
        return True

    def get_data_from_cypher_query(self, chartName, data, cypherQuery):
        try:
            with self.neosession.begin_transaction() as tx:
                for neorecord in tx.run(cypherQuery):
                    if neorecord["label"] not in self.charts[chartName]:
                        self.charts[chartName].add_dimension([ neorecord["label"] ])
                    data[ neorecord["label"] ]  = neorecord["cnt"]
                    tx.close()
        except Exception:
            return None

    def get_data(self):

        """
        :return: dict
        """
        if not self.is_alive():
            return None

        if not NEO4JDRIVER:
            # self.error('neo4j-driver not available. Please install.')
            return None

        data = dict()

        # nodes (total)
        self.get_data_from_cypher_query( 'monnodes',data, "MATCH (n) RETURN 'Total' AS label, count(n) AS cnt")

        # relations (top 5)
        self.get_data_from_cypher_query( 'monrels',data,  "MATCH (n)-[r]-(m) RETURN type(r) as label, count(r) AS cnt ORDER BY cnt DESC LIMIT 5")

        # labels (top 5)
        self.get_data_from_cypher_query( 'monlabels',data,  "MATCH (a) WITH DISTINCT LABELS(a) AS temp, COUNT(a) AS tempCnt UNWIND temp AS label RETURN label, SUM(tempCnt) AS cnt ORDER BY cnt DESC LIMIT 5")
        return data

    def connect(self):
        host = self.configuration.get('host')
        boltport = self.configuration.get('port','7687')
        uri = "bolt://" + host + ":" + boltport
        self.neodriver = GraphDatabase.driver(uri, auth=(self.configuration.get('user'), self.configuration.get('pwd') ))

        if self.neosession == None:
            self.neosession = self.neodriver.session()

    def reconnect(self):
        try:
            self.neosession = self.neodriver.session()
            self.alive = True
            return True
        except Exception:
            return False

    def is_alive(self):
        if not self.alive:
            return self.reconnect()
        return True