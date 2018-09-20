# -*- coding: utf-8 -*-
# Description: ceph netdata python.d module
# Author: Luis Eduardo (lets00)
# SPDX-License-Identifier: GPL-3.0+

try:
    import rados
    CEPH = True
except ImportError:
    CEPH = False

import os
import json
from bases.FrameworkServices.SimpleService import SimpleService

# default module values (can be overridden per job in `config`)
update_every = 10
priority = 60000
retries = 60

ORDER = ['general_usage', 'general_objects', 'general_bytes', 'general_operations',
         'general_latency', 'pool_usage', 'pool_objects', 'pool_read_bytes',
         'pool_write_bytes', 'pool_read_operations', 'pool_write_operations', 'osd_usage',
         'osd_apply_latency', 'osd_commit_latency']

CHARTS = {
    'general_usage': {
        'options': [None, 'Ceph General Space', 'KB', 'general', 'ceph.general_usage', 'stacked'],
        'lines': [
            ['general_available', 'avail', 'absolute'],
            ['general_usage', 'used', 'absolute']
        ]
    },
    'general_objects': {
        'options': [None, 'Ceph General Objects', 'objects', 'general', 'ceph.general_objects', 'area'],
        'lines': [
            ['general_objects', 'cluster', 'absolute']
        ]
    },
    'general_bytes': {
        'options': [None, 'Ceph General Read/Write Data/s', 'KB', 'general', 'ceph.general_bytes',
                    'area'],
        'lines': [
            ['general_read_bytes', 'read', 'absolute', 1, 1024],
            ['general_write_bytes', 'write', 'absolute', -1, 1024]
        ]
    },
    'general_operations': {
        'options': [None, 'Ceph General Read/Write Operations/s', 'operations', 'general', 'ceph.general_operations',
                    'area'],
        'lines': [
            ['general_read_operations', 'read', 'absolute', 1],
            ['general_write_operations', 'write', 'absolute', -1]
        ]
    },
    'general_latency': {
        'options': [None, 'Ceph General Apply/Commit latency', 'milliseconds', 'general', 'ceph.general_latency',
                    'area'],
        'lines': [
            ['general_apply_latency', 'apply', 'absolute'],
            ['general_commit_latency', 'commit', 'absolute']
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
    'pool_read_bytes': {
        'options': [None, 'Ceph Read Pool Data/s', 'KB', 'pool', 'ceph.pool_read_bytes', 'area'],
        'lines': []
    },
    'pool_write_bytes': {
        'options': [None, 'Ceph Write Pool Data/s', 'KB', 'pool', 'ceph.pool_write_bytes', 'area'],
        'lines': []
    },
    'pool_read_operations': {
        'options': [None, 'Ceph Read Pool Operations/s', 'operations', 'pool', 'ceph.pool_read_operations', 'area'],
        'lines': []
    },
    'pool_write_operations': {
        'options': [None, 'Ceph Write Pool Operations/s', 'operations', 'pool', 'ceph.pool_write_operations', 'area'],
        'lines': []
    },
    'osd_usage': {
        'options': [None, 'Ceph OSDs', 'KB', 'osd', 'ceph.osd_usage', 'line'],
        'lines': []
    },
    'osd_apply_latency': {
        'options': [None, 'Ceph OSDs apply latency', 'milliseconds', 'osd', 'ceph.apply_latency', 'line'],
        'lines': []
    },
    'osd_commit_latency': {
        'options': [None, 'Ceph OSDs commit latency', 'milliseconds', 'osd', 'ceph.commit_latency', 'line'],
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

        # Verify files and permissions
        if not (os.access(self.config_file, os.F_OK)):
            self.error('{0} does not exist'.format(self.config_file))
            return False
        if not (os.access(self.keyring_file, os.F_OK)):
            self.error('{0} does not exist'.format(self.keyring_file))
            return False
        if not (os.access(self.config_file, os.R_OK)):
            self.error('Ceph plugin does not read {0}, define read permission.'.format(self.config_file))
            return False
        if not (os.access(self.keyring_file, os.R_OK)):
            self.error('Ceph plugin does not read {0}, define read permission.'.format(self.keyring_file))
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
        # Pool lines
        for pool in sorted(self._get_df()['pools']):
            self.definitions['pool_usage']['lines'].append([pool['name'],
                                                            pool['name'],
                                                            'absolute'])
            self.definitions['pool_objects']['lines'].append(["obj_{0}".format(pool['name']),
                                                              pool['name'],
                                                              'absolute'])
            self.definitions['pool_read_bytes']['lines'].append(['read_{0}'.format(pool['name']),
                                                                pool['name'],
                                                                'absolute', 1, 1024])
            self.definitions['pool_write_bytes']['lines'].append(['write_{0}'.format(pool['name']),
                                                                 pool['name'],
                                                                 'absolute', 1, 1024])
            self.definitions['pool_read_operations']['lines'].append(['read_operations_{0}'.format(pool['name']),
                                                                pool['name'],
                                                                'absolute'])
            self.definitions['pool_write_operations']['lines'].append(['write_operations_{0}'.format(pool['name']),
                                                                 pool['name'],
                                                                 'absolute'])

        # OSD lines
        for osd in sorted(self._get_osd_df()['nodes']):
            self.definitions['osd_usage']['lines'].append([osd['name'],
                                                           osd['name'],
                                                           'absolute'])
            self.definitions['osd_apply_latency']['lines'].append(['apply_latency_{0}'.format(osd['name']),
                                                                   osd['name'],
                                                                   'absolute'])
            self.definitions['osd_commit_latency']['lines'].append(['commit_latency_{0}'.format(osd['name']),
                                                                    osd['name'],
                                                                    'absolute'])

    def get_data(self):
        """
        Catch all ceph data
        :return: dict
        """
        try:
            data = {}
            df = self._get_df()
            osd_df = self._get_osd_df()
            osd_perf = self._get_osd_perf()
            pool_stats = self._get_osd_pool_stats()
            data.update(self._get_general(osd_perf, pool_stats))
            for pool in df['pools']:
                data.update(self._get_pool_usage(pool))
                data.update(self._get_pool_objects(pool))
            for pool_io in pool_stats:
                data.update(self._get_pool_rw(pool_io))
            for osd in osd_df['nodes']:
                data.update(self._get_osd_usage(osd))
            for osd_apply_commit in osd_perf['osd_perf_infos']:
                data.update(self._get_osd_latency(osd_apply_commit))
            return data
        except (ValueError, AttributeError) as error:
            self.error(error)
            return None

    def _get_general(self, osd_perf, pool_stats):
        """
        Get ceph's general usage
        :return: dict
        """
        status = self.cluster.get_cluster_stats()
        read_bytes_sec = 0
        write_bytes_sec = 0
        read_op_per_sec = 0
        write_op_per_sec = 0
        apply_latency = 0
        commit_latency = 0

        for pool_rw_io_b in pool_stats:
            read_bytes_sec += pool_rw_io_b['client_io_rate'].get('read_bytes_sec', 0)
            write_bytes_sec += pool_rw_io_b['client_io_rate'].get('write_bytes_sec', 0)
            read_op_per_sec += pool_rw_io_b['client_io_rate'].get('read_op_per_sec', 0)
            write_op_per_sec += pool_rw_io_b['client_io_rate'].get('write_op_per_sec', 0)
        for perf in osd_perf['osd_perf_infos']:
            apply_latency += perf['perf_stats']['apply_latency_ms']
            commit_latency += perf['perf_stats']['commit_latency_ms']

        return {'general_usage': int(status['kb_used']),
                'general_available': int(status['kb_avail']),
                'general_objects': int(status['num_objects']),
                'general_read_bytes': read_bytes_sec,
                'general_write_bytes': write_bytes_sec,
                'general_read_operations': read_op_per_sec,
                'general_write_operations': write_op_per_sec,
                'general_apply_latency': apply_latency,
                'general_commit_latency': commit_latency
                }

    @staticmethod
    def _get_pool_usage(pool):
        """
        Process raw data into pool usage dict information
        :return: A pool dict with pool name's key and usage bytes' value
        """
        return {pool['name']: pool['stats']['kb_used']}

    @staticmethod
    def _get_pool_objects(pool):
        """
        Process raw data into pool usage dict information
        :return: A pool dict with pool name's key and object numbers
        """
        return {'obj_{0}'.format(pool['name']): pool['stats']['objects']}

    @staticmethod
    def _get_pool_rw(pool):
        """
        Get read/write kb and operations in a pool
        :return: A pool dict with both read/write bytes and operations.
        """
        return {'read_{0}'.format(pool['pool_name']): int(pool['client_io_rate'].get('read_bytes_sec', 0)),
                'write_{0}'.format(pool['pool_name']): int(pool['client_io_rate'].get('write_bytes_sec', 0)),
                'read_operations_{0}'.format(pool['pool_name']): int(pool['client_io_rate'].get('read_op_per_sec', 0)),
                'write_operations_{0}'.format(pool['pool_name']): int(pool['client_io_rate'].get('write_op_per_sec', 0))
                }

    @staticmethod
    def _get_osd_usage(osd):
        """
        Process raw data into osd dict information to get osd usage
        :return: A osd dict with osd name's key and usage bytes' value
        """
        return {osd['name']: float(osd['kb_used'])}

    @staticmethod
    def _get_osd_latency(osd):
        """
        Get ceph osd apply and commit latency
        :return: A osd dict with osd name's key with both apply and commit latency values
        """
        return {'apply_latency_osd.{0}'.format(osd['id']): osd['perf_stats']['apply_latency_ms'],
                'commit_latency_osd.{0}'.format(osd['id']): osd['perf_stats']['commit_latency_ms']}

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

    def _get_osd_perf(self):
        """
        Get ceph osd performance
        :return: ceph osd perf --format json
        """
        return json.loads(self.cluster.mon_command(json.dumps({
            'prefix': 'osd perf',
            'format': 'json'
        }), '')[1])

    def _get_osd_pool_stats(self):
        """
        Get ceph osd pool status.
        This command is used to get information about both
        read/write operation and bytes per second on each pool
        :return: ceph osd pool stats --format json
        """
        return json.loads(self.cluster.mon_command(json.dumps({
            'prefix': 'osd pool stats',
            'format': 'json'
        }), '')[1])
