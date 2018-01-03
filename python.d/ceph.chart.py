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
            ['general_available', 'avail', 'absolute', 1, 1024]
            ['general_usage', 'used', 'absolute', 1, 1024],
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
        self.cluster = rados.Rados(conffile=self.config_file,
                                   conf=dict(keyring=self.keyring_file))

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
            self.cluster.connect()
        except Exception as error:
            self.error(error)
            return False
        self.create_definitions()
        return True

    def create_definitions(self):
        """
        Create dynamically charts options
        set on YAML file
        :return: None
        """
        # Lines
        for pool in self._get_df()['pools']:
            self.definitions['pool_usage']['lines'].append([pool['name'],
                                                            pool['name'],
                                                            'absolute'])
            self.definitions['pool_objects']['lines'].append(["obj_{}".format(pool['name']),
                                                              pool['name'],
                                                              'absolute'])
        for osd in self._get_osd_df()['nodes']:
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
            data.update(self._get_pool_usage())
            data.update(self._get_pool_objects())
            data.update(self._get_osd_usage())
            return data
        except (ValueError, AttributeError):
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

    def _get_pool_usage(self):
        """
        Process raw data into pool usage dict information
        :return: A pool dict with pool name's key and usage bytes' value
        """
        pool_usage = {}
        for df in self._get_df()['pools']:
            pool_usage[df['name']] = df['stats']['kb_used']
        return pool_usage

    def _get_pool_objects(self):
        """
        Process raw data into pool usage dict information
        :return: A pool dict with pool name's key and object numbers
        """
        pool_objects = {}
        for df in self._get_df()['pools']:
            pool_objects["obj_{}".format(df['name'])] = df['stats']['objects']
        return pool_objects

    def _get_osd_usage(self):
        """
        Process raw data into osd dict information to get osd usage
        :return: A osd dict with osd name's key and usage bytes' value
        """
        osd_usage = {}
        for osd in self._get_osd_df()['nodes']:
            osd_usage[osd['name']] = float(osd['kb_used'])
        return osd_usage

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
