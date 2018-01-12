# -*- coding: utf-8 -*-
# Description: ceph netdata python.d module
# Author: Luis Eduardo (lets00)

try:
    import rados
    CEPH = True
except ImportError:
    CEPH = False

import json
from bases.FrameworkServices.SimpleService import SimpleService

# default module values (can be overridden per job in `config`)
update_every = 10
priority = 60000
retries = 60

ORDER = ['general', 'pool_usage', 'pool_objects', 'osd_usage']

CHARTS = {
    'general': {
        'options': [None, 'Ceph General Options', 'KB', 'general', 'ceph.general', 'stacked'],
        'lines': [
            ['general_available', 'avail', 'absolute', 1, 1024],
            ['general_usage', 'used', 'absolute', 1, 1024]
        ]
    },
    'pool_usage': {
        'options': [None, 'Ceph Pools', 'KB', 'pool', 'ceph.pool_usage', 'line'],
        'lines': []
    },
    'pool_objects': {
        'options': [None, 'Ceph Pools', 'objects', 'pool', 'ceph.pool_objects', 'line'],
        'lines': []
    },
    'osd_usage': {
        'options': [None, 'Ceph OSDs', 'KB', 'osd', 'ceph.osd_usage', 'line'],
        'lines': []
    }
}


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.config_file = self.configuration.get('config_file')
        self.keyring_file = self.configuration.get('keyring_file')

    def check(self):
        """
        Checks module
        :return:
        """
        if not CEPH:
            self.error('rados module is needed to use ceph.chart.py')
            return False
        if not (self.config_file and self.keyring_file):
            self.error('config_file and/or keyring_file is not defined')
            return False
        try:
            self.cluster = rados.Rados(conffile=self.config_file,
                                       conf=dict(keyring=self.keyring_file))
            self.cluster.connect()
        except rados.Error as error:
            self.error(error)
            return False
        self.create_definitions()
        return True

    def create_definitions(self):
        """
        Create dynamically charts options
        :return: None
        """
        # Lines
        for pool in sorted(self._get_df()['pools']):
            self.definitions['pool_usage']['lines'].append([pool['name'],
                                                            pool['name'],
                                                            'absolute'])
            self.definitions['pool_objects']['lines'].append(["obj_{0}".format(pool['name']),
                                                              pool['name'],
                                                              'absolute'])
        for osd in sorted(self._get_osd_df()['nodes']):
            self.definitions['osd_usage']['lines'].append([osd['name'],
                                                           osd['name'],
                                                           'absolute'])

    def get_data(self):
        """
        Catch all ceph data
        :return: dict
        """
        try:
            data = {}
            data.update(self._get_general())
            for pool in self._get_df()['pools']:
                data.update(self._get_pool_usage(pool))
                data.update(self._get_pool_objects(pool))
            for osd in self._get_osd_df()['nodes']:
                data.update(self._get_osd_usage(osd))
            return data
        except (ValueError, AttributeError) as error:
            self.error(error)
            return None

    def _get_general(self):
        """
        Get ceph's general usage
        :return: dict
        """
        info = self._get_df()['stats']
        return {'general_usage': info['total_used_bytes'],
                'general_available': info['total_avail_bytes']
                }

    def _get_pool_usage(self, pool):
        """
        Process raw data into pool usage dict information
        :return: A pool dict with pool name's key and usage bytes' value
        """
        return {pool['name']: pool['stats']['kb_used']}

    def _get_pool_objects(self, pool):
        """
        Process raw data into pool usage dict information
        :return: A pool dict with pool name's key and object numbers
        """
        return {'obj_{0}'.format(pool['name']): pool['stats']['objects']}

    def _get_osd_usage(self, osd):
        """
        Process raw data into osd dict information to get osd usage
        :return: A osd dict with osd name's key and usage bytes' value
        """
        return {osd['name']: float(osd['kb_used'])}

    def _get_df(self):
        """
        Get ceph df output
        :return: ceph df --format json
        """
        return json.loads(self.cluster.mon_command(json.dumps({
            'prefix': 'df',
            'format': 'json'
        }), '')[1])

    def _get_osd_df(self):
        """
        Get ceph osd df output
        :return: ceph osd df --format json
        """
        return json.loads(self.cluster.mon_command(json.dumps({
            'prefix': 'osd df',
            'format': 'json'
        }), '')[1])

