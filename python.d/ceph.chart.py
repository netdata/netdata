# -*- coding: utf-8 -*-
# Description: ceph netdata python.d module
# Author: Luis Eduardo (lets00)

import rados
import json
from bases.FrameworkServices.SimpleService import SimpleService

ORDER = ['general', 'pool_usage', 'pool_objects', 'osd_usage']

CHARTS = {
    'general': {
        'options': [None, 'Ceph General Options', 'usage', 'general', 'ceph.general', 'line'],
        'lines': []
    },
    'pool_usage': {
        'options': [None, 'Ceph Pools', 'usage', 'pool usage', 'ceph.pool_usage', 'line'],
        'lines': []
    },
    'pool_objects': {
        'options': [None, 'Ceph Pools', 'objects', 'number of objects', 'ceph.pool_objects', 'line'],
        'lines': []
    },
    'osd_usage': {
        'options': [None, 'Ceph OSDs', 'usage', 'osd usage', 'ceph.osd_usage', 'line'],
        'lines': []
    }
}

PRESENTATION_MAP = {
    'KB': 1,
    'MB': 1024,
    'GB': 1024**2,
    'TB': 1024**3
}


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.config_file = self.configuration.get('config_file')
        self.keyring_file = self.configuration.get('keyring_file')
        self.presentation_data = self.configuration.get('presentation_data')
        self.presentation_value = None
        self.cluster = rados.Rados(conffile=self.config_file,
                                   conf=dict(keyring=self.keyring_file))

    def check(self):
        """
        Checks module
        :return:
        """
        if not (self.config_file and self.keyring_file):
            self.error('config_file and/or keyring_file is not defined')
            return False

        # Set default presentation_data to GB if it does not defined and declare
        # presentation_value with PRESENTATION_MAP value.
        if not self.presentation_data:
            self.presentation_data = 'GB'
        self.presentation_value = PRESENTATION_MAP[self.presentation_data]
        try:
            self.cluster.connect()
        except Exception as error:
            self.error(error)
            return False
        self.create_definitions()
        return True

    def create_definitions(self):
        """
        Create dynamically charts options, add presentation data/value
        set on YAML file
        :return: None
        """
        # Options
        self.definitions['general']['options'] = [None, 'Ceph General Options',
                                                  'usage ({})'.format(self.presentation_data),
                                                  'general', 'ceph.general', 'areastack']
        self.definitions['pool_usage']['options'] = [None, 'Ceph Pools',
                                                     'usage ({})'.format(self.presentation_data),
                                                     'pool usage', 'ceph.pool_usage', 'line']
        self.definitions['osd_usage']['options'] = [None, 'Ceph OSDs',
                                                     'usage ({})'.format(self.presentation_data),
                                                     'osd usage', 'ceph.osd_usage', 'line']

        # Lines
        self.definitions['general']['lines'].append(['general_total', 'Total',
                                                'absolute', 1, self.presentation_value])
        self.definitions['general']['lines'].append(['general_usage', 'Usage',
                                                'absolute', 1, self.presentation_value])
        self.definitions['general']['lines'].append(['general_available', 'Available',
                                                'absolute', 1, self.presentation_value])

        for pool in self.cluster.list_pools():
            self.definitions['pool_usage']['lines'].append([pool, pool,
                                                            'absolute', 1, self.presentation_value])
            self.definitions['pool_objects']['lines'].append(["obj_{}".format(pool), pool,
                                                              'absolute'])

        for osd in self._get_osds_df()['nodes']:
            self.definitions['osd_usage']['lines'].append([osd['name'], osd['name'],
                                                           'absolute', 1, self.presentation_value])

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
        info = self.cluster.get_cluster_stats()
        # Values are get in bytes, convert it
        return {'general_total': float(info['kb']),
                'general_usage': float(info['kb_used']),
                'general_available': float(info['kb_avail'])
                }

    def _get_pool_usage(self):
        """
        Process raw data into pool usage dict information
        :return: A pool dict with pool name's key and usage bytes' value
        """
        pool_usage = {}
        for df in self._get_df()['pools']:
            pool_usage[df['name']] = df['stats']['bytes_used']
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
        # TODO: FIX ME
        """
        Process raw data into osd dict information to get osd usage
        :return: A osd dict with osd name's key and usage bytes' value
        """
        osd_usage = {}
        for osd in self._get_osds_df()['nodes']:
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

    def _get_osds_df(self):
        """
        Get ceph osd df output
        :return: ceph osd df --format json
        """
        return json.loads(self.cluster.mon_command(json.dumps({
            'prefix': 'osd df',
            'format': 'json'
        }), '')[1])
