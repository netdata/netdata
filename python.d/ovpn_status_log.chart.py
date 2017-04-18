# -*- coding: utf-8 -*-
# Description: openvpn status log netdata python.d module
# Author: l2isbad

from re import compile as r_compile
from collections import defaultdict
from base import SimpleService

priority = 60000
retries = 60
update_every = 10

ORDER = ['users', 'traffic']
CHARTS = {
    'users': {
        'options': [None, 'OpenVPN Active Users', 'active users', 'users', 'openvpn_status.users', 'line'],
        'lines': [
            ['users', None, 'absolute'],
        ]},
    'traffic': {
        'options': [None, 'OpenVPN Traffic', 'KB/s', 'traffic', 'openvpn_status.traffic', 'area'],
        'lines': [
            ['bytes_in', 'in', 'incremental', 1, 1 << 10], ['bytes_out', 'out', 'incremental', 1, -1 << 10]
        ]},

}


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.log_path = self.configuration.get('log_path')
        self.regex = dict(tls=r_compile(r'\d{1,3}(?:\.\d{1,3}){3}(?::\d+)? (?P<bytes_in>\d+) (?P<bytes_out>\d+)'),
                          static_key=r_compile(r'TCP/[A-Z]+ (?P<direction>(?:read|write)) bytes,(?P<bytes>\d+)'))
        self.to_netdata = dict(bytes_in=0, bytes_out=0)

    def check(self):
        if not (self.log_path and isinstance(self.log_path, str)):
            self.error('\'log_path\' is not defined')
            return False

        data = False
        for method in (self._get_data_tls, self._get_data_static_key):
            data = method()
            if data:
                self._get_data = method
                self._data_from_check = data
                break

        if data:
            return True
        self.error('Make sure that the openvpn status log file exists and netdata has permission to read it')
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

    def _get_data_static_key(self):
        """
        Parse openvpn-status log file.
        """

        raw_data = self._get_raw_data()
        if not raw_data:
            return None

        data = defaultdict(lambda: 0)

        for row in raw_data:
            match = self.regex['static_key'].search(row)
            if match:
                match = match.groupdict()
                if match['direction'] == 'read':
                    data['bytes_in'] += int(match['bytes'])
                else:
                    data['bytes_out'] += int(match['bytes'])

        return data or None

    def _get_data_tls(self):
        """
        Parse openvpn-status log file.
        """

        raw_data = self._get_raw_data()
        if not raw_data:
            return None

        data = defaultdict(lambda: 0)
        for row in raw_data:
            row = ' '.join(row.split(',')) if ',' in row else ' '.join(row.split())
            match = self.regex['tls'].search(row)
            if match:
                match = match.groupdict()
                data['users'] += 1
                data['bytes_in'] += int(match['bytes_in'])
                data['bytes_out'] += int(match['bytes_out'])

        return data or None

