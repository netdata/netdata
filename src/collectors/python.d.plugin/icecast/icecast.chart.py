# -*- coding: utf-8 -*-
# Description: icecast netdata python.d module
# Author: Ilya Mashchenko (ilyam8)
# SPDX-License-Identifier: GPL-3.0-or-later

import json

from bases.FrameworkServices.UrlService import UrlService

ORDER = [
    'listeners',
]

CHARTS = {
    'listeners': {
        'options': [None, 'Number Of Listeners', 'listeners', 'listeners', 'icecast.listeners', 'line'],
        'lines': [
        ]
    }
}


class Source:
    def __init__(self, idx, data):
        self.name = 'source_{0}'.format(idx)
        self.is_active = data.get('stream_start') and data.get('server_name')
        self.listeners = data['listeners']


class Service(UrlService):
    def __init__(self, configuration=None, name=None):
        UrlService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.url = self.configuration.get('url')
        self._manager = self._build_manager()

    def check(self):
        """
        Add active sources to the "listeners" chart
        :return: bool
        """
        sources = self.get_sources()
        if not sources:
            return None

        active_sources = 0
        for idx, raw_source in enumerate(sources):
            if Source(idx, raw_source).is_active:
                active_sources += 1
                dim_id = 'source_{0}'.format(idx)
                dim = 'source {0}'.format(idx)
                self.definitions['listeners']['lines'].append([dim_id, dim])

        return bool(active_sources)

    def _get_data(self):
        """
        Get number of listeners for every source
        :return: dict
        """
        sources = self.get_sources()
        if not sources:
            return None

        data = dict()

        for idx, raw_source in enumerate(sources):
            source = Source(idx, raw_source)
            data[source.name] = source.listeners

        return data

    def get_sources(self):
        """
        Format data received from http request and return list of sources
        :return: list
        """

        raw_data = self._get_raw_data()
        if not raw_data:
            return None

        try:
            data = json.loads(raw_data)
        except ValueError as error:
            self.error('JSON decode error:', error)
            return None

        sources = data['icestats'].get('source')
        if not sources:
            return None

        return sources if isinstance(sources, list) else [sources]
