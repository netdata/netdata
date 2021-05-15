# -*- coding: utf-8 -*-
# Description: watchtower netdata python.d module
# Author: tknobi
# SPDX-License-Identifier: GPL-3.0-or-later

from bases.FrameworkServices.UrlService import UrlService

ORDER = [
    'containers',
    'scans'
]

CHARTS = {
    'containers': {
        'options': [None, 'Watchtower Containers', 'containers', None, 'watchtower.containers', 'line'],
        'lines': [
            ['watchtower_containers_failed', 'failed'],
            ['watchtower_containers_scanned', 'scanned'],
            ['watchtower_containers_updated', 'updated']
        ]},
    'scans': {
        'options': [None, 'Watchtower Scans', 'scans', None, 'watchtower.scans', 'line'],
        'lines': [
            ['watchtower_scans_skipped', 'skipped'],
            ['watchtower_scans_total', 'total'],
        ]}
}


class Service(UrlService):
    def __init__(self, configuration=None, name=None):
        UrlService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.url = self.configuration.get('url', 'http://127.0.0.1:8080') + '/v1/metrics'
        self.api_token = self.configuration.get('api_token', '')
        self.header = {'Authorization': 'Bearer ' + self.api_token}

    def check(self):
        if not (self.api_token and isinstance(self.api_token, str)):
            self.error('API_TOKEN is not defined or type is not <str>')
            return False
        return UrlService.check(self)

    def _get_data(self):
        raw_data = self._get_raw_data()
        results = {}
        for line in raw_data.split('\n'):
            if line.startswith('#') or ' ' not in line:
                continue
            key, value = line.split(' ')
            results[key] = value
        return results
