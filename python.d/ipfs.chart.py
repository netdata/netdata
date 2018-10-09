# -*- coding: utf-8 -*-
# Description: IPFS netdata python.d module
# Authors: davidak
# SPDX-License-Identifier: GPL-3.0-or-later

import json

from bases.FrameworkServices.UrlService import UrlService

# default module values (can be overridden per job in `config`)
# update_every = 2
priority = 60000
retries = 60

# default job configuration (overridden by python.d.plugin)
# config = {'local': {
#     'update_every': update_every,
#     'retries': retries,
#     'priority': priority,
#     'url': 'http://localhost:5001'
# }}

# charts order (can be overridden if you want less charts, or different order)
ORDER = ['bandwidth', 'peers', 'repo_size', 'repo_objects']

CHARTS = {
    'bandwidth': {
        'options': [None, 'IPFS Bandwidth', 'kbits/s', 'Bandwidth', 'ipfs.bandwidth', 'line'],
        'lines': [
            ['in', None, 'absolute', 8, 1000],
            ['out', None, 'absolute', -8, 1000]
        ]
    },
    'peers': {
        'options': [None, 'IPFS Peers', 'peers', 'Peers', 'ipfs.peers', 'line'],
        'lines': [
            ['peers', None, 'absolute']
        ]
    },
    'repo_size': {
        'options': [None, 'IPFS Repo Size', 'GB', 'Size', 'ipfs.repo_size', 'area'],
        'lines': [
            ['avail', None, 'absolute', 1, 1e9],
            ['size', None, 'absolute', 1, 1e9],
        ]
    },
    'repo_objects': {
        'options': [None, 'IPFS Repo Objects', 'objects', 'Objects', 'ipfs.repo_objects', 'line'],
        'lines': [
            ['objects', None, 'absolute', 1, 1],
            ['pinned', None, 'absolute', 1, 1],
            ['recursive_pins', None, 'absolute', 1, 1]
        ]
    }
}

SI_zeroes = {
    'k': 3,
    'm': 6,
    'g': 9,
    't': 12,
    'p': 15,
    'e': 18,
    'z': 21,
    'y': 24
}


class Service(UrlService):
    def __init__(self, configuration=None, name=None):
        UrlService.__init__(self, configuration=configuration, name=name)
        self.baseurl = self.configuration.get('url', 'http://localhost:5001')
        self.order = ORDER
        self.definitions = CHARTS
        self.__storage_max = None
        self.do_pinapi = self.configuration.get('pinapi')

    def _get_json(self, sub_url):
        """
        :return: json decoding of the specified url
        """
        self.url = self.baseurl + sub_url
        try:
            return json.loads(self._get_raw_data())
        except (TypeError, ValueError):
            return dict()

    @staticmethod
    def _recursive_pins(keys):
        return sum(1 for k in keys if keys[k]['Type'] == b'recursive')

    @staticmethod
    def _dehumanize(store_max):
        # convert from '10Gb' to 10000000000
        if not isinstance(store_max, int):
            store_max = store_max.lower()
            if store_max.endswith('b'):
                val, units = store_max[:-2], store_max[-2]
                if units in SI_zeroes:
                    val += '0'*SI_zeroes[units]
                store_max = val
            try:
                store_max = int(store_max)
            except (TypeError, ValueError):
                store_max = None
        return store_max

    def _storagemax(self, store_cfg):
        if self.__storage_max is None:
            self.__storage_max = self._dehumanize(store_cfg)
        return self.__storage_max

    def _get_data(self):
        """
        Get data from API
        :return: dict
        """
        # suburl : List of (result-key, original-key, transform-func)
        cfg = {
            '/api/v0/stats/bw':
                [('in', 'RateIn', int), ('out', 'RateOut', int)],
            '/api/v0/swarm/peers':
                [('peers', 'Peers', len)],
            '/api/v0/stats/repo':
                [('size', 'RepoSize', int), ('objects', 'NumObjects', int), ('avail', 'StorageMax', self._storagemax)],
        }
        if self.do_pinapi:
                cfg.update({
                    '/api/v0/pin/ls':
                        [('pinned', 'Keys', len), ('recursive_pins', 'Keys', self._recursive_pins)]
                })
        r = dict()
        for suburl in cfg:
            in_json = self._get_json(suburl)
            for new_key, orig_key, xmute in cfg[suburl]:
                try:
                    r[new_key] = xmute(in_json[orig_key])
                except Exception:
                    continue
        return r or None
