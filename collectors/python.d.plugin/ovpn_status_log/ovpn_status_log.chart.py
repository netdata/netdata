# -*- coding: utf-8 -*-
# Description: openvpn status log netdata python.d module
# Author: ilyam8
# SPDX-License-Identifier: GPL-3.0-or-later

import re

from bases.FrameworkServices.SimpleService import SimpleService

update_every = 10

ORDER = [
    'users',
    'traffic',
]

CHARTS = {
    'users': {
        'options': [None, 'OpenVPN Active Users', 'active users', 'users', 'openvpn_status.users', 'line'],
        'lines': [
            ['users', None, 'absolute'],
        ]
    },
    'traffic': {
        'options': [None, 'OpenVPN Traffic', 'KiB/s', 'traffic', 'openvpn_status.traffic', 'area'],
        'lines': [
            ['bytes_in', 'in', 'incremental', 1, 1 << 10],
            ['bytes_out', 'out', 'incremental', -1, 1 << 10]
        ]
    }
}

TLS_REGEX = re.compile(
    r'(?:[0-9a-f]+:[0-9a-f:]+|(?:\d{1,3}(?:\.\d{1,3}){3}(?::\d+)?)) (?P<bytes_in>\d+) (?P<bytes_out>\d+)'
)
STATIC_KEY_REGEX = re.compile(
    r'TCP/[A-Z]+ (?P<direction>(?:read|write)) bytes,(?P<bytes>\d+)'
)


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.log_path = self.configuration.get('log_path')
        self.regex = {
            'tls': TLS_REGEX,
            'static_key': STATIC_KEY_REGEX
        }

    def check(self):
        if not (self.log_path and isinstance(self.log_path, str)):
            self.error("'log_path' is not defined")
            return False

        data = self._get_raw_data()
        if not data:
            self.error('Make sure that the openvpn status log file exists and netdata has permission to read it')
            return None

        found = None
        for row in data:
            if 'ROUTING' in row:
                self.get_data = self.get_data_tls
                found = True
                break
            elif 'STATISTICS' in row:
                self.get_data = self.get_data_static_key
                found = True
                break
        if found:
            return True
        self.error('Failed to parse ovpenvpn log file')
        return False

    def _get_raw_data(self):
        """
        Open log file
        :return: str
        """

        try:
            with open(self.log_path) as log:
                raw_data = log.readlines() or None
        except OSError:
            return None
        else:
            return raw_data

    def get_data_static_key(self):
        """
        Parse openvpn-status log file.
        """

        raw_data = self._get_raw_data()
        if not raw_data:
            return None

        data = dict(bytes_in=0, bytes_out=0)

        for row in raw_data:
            match = self.regex['static_key'].search(row)
            if match:
                match = match.groupdict()
                if match['direction'] == 'read':
                    data['bytes_in'] += int(match['bytes'])
                else:
                    data['bytes_out'] += int(match['bytes'])

        return data or None

    def get_data_tls(self):
        """
        Parse openvpn-status log file.
        """

        raw_data = self._get_raw_data()
        if not raw_data:
            return None

        data = dict(users=0, bytes_in=0, bytes_out=0)
        for row in raw_data:
            columns = row.split(',') if ',' in row else row.split()
            if 'UNDEF' in columns:
                # see https://openvpn.net/archive/openvpn-users/2004-08/msg00116.html
                continue

            match = self.regex['tls'].search(' '.join(columns))
            if match:
                match = match.groupdict()
                data['users'] += 1
                data['bytes_in'] += int(match['bytes_in'])
                data['bytes_out'] += int(match['bytes_out'])

        return data or None
