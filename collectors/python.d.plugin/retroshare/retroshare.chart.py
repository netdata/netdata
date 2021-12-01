# -*- coding: utf-8 -*-
# Description: RetroShare netdata python.d module
# Authors: sehraf
# SPDX-License-Identifier: GPL-3.0-or-later

import json

from bases.FrameworkServices.UrlService import UrlService

ORDER = [
    'bandwidth',
    'peers',
    'dht',
]

CHARTS = {
    'bandwidth': {
        'options': [None, 'RetroShare Bandwidth', 'kilobits/s', 'RetroShare', 'retroshare.bandwidth', 'area'],
        'lines': [
            ['bandwidth_up_kb', 'Upload'],
            ['bandwidth_down_kb', 'Download']
        ]
    },
    'peers': {
        'options': [None, 'RetroShare Peers', 'peers', 'RetroShare', 'retroshare.peers', 'line'],
        'lines': [
            ['peers_all', 'All friends'],
            ['peers_connected', 'Connected friends']
        ]
    },
    'dht': {
        'options': [None, 'Retroshare DHT', 'peers', 'RetroShare', 'retroshare.dht', 'line'],
        'lines': [
            ['dht_size_all', 'DHT nodes estimated'],
            ['dht_size_rs', 'RS nodes estimated']
        ]
    }
}


class Service(UrlService):
    def __init__(self, configuration=None, name=None):
        UrlService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.baseurl = self.configuration.get('url', 'http://localhost:9090')

    def _get_stats(self):
        """
        Format data received from http request
        :return: dict
        """
        try:
            raw = self._get_raw_data()
            parsed = json.loads(raw)
            if str(parsed['returncode']) != 'ok':
                return None
        except (TypeError, ValueError):
            return None

        return parsed['data'][0]

    def _get_data(self):
        """
        Get data from API
        :return: dict
        """
        self.url = self.baseurl + '/api/v2/stats'
        data = self._get_stats()
        if data is None:
            return None

        data['bandwidth_up_kb'] = data['bandwidth_up_kb'] * -1
        if data['dht_active'] is False:
            data['dht_size_all'] = None
            data['dht_size_rs'] = None

        return data
