# -*- coding: utf-8 -*-
# Description: openvpn status log netdata python.d module
# Author: l2isbad

from base import SimpleService
from re import compile, findall, search, subn
priority = 60000
retries = 60
update_every = 10

ORDER = ['users', 'traffic']
CHARTS = {
    'users': {
        'options': [None, 'OpenVPN active users', 'active users', 'Users', 'openvpn_status.users', 'line'],
        'lines': [
            ["users", None, "absolute"],
        ]},
    'traffic': {
        'options': [None, 'OpenVPN traffic', 'kilobit/s', 'Traffic', 'openvpn_status.traffic', 'area'],
        'lines': [
            ["in", None, "incremental", 8, 1000], ["out", None, "incremental", 8, -1000]
        ]},

}

class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.log_path = self.configuration.get('log_path')
        self.regex_data_inter = compile(r'(?<=Since ).*?(?=.ROUTING)')
        self.regex_data_final = compile(r'\d{1,3}(?:\.\d{1,3}){3}[:0-9,. ]*')
        self.regex_users = compile(r'\d{1,3}(?:\.\d{1,3}){3}:\d+')
        self.regex_traffic = compile(r'(?<=(?:,| ))\d+(?=(?:,| ))')

    def check(self):
        if not self._get_raw_data():
            self.error('Make sure that the openvpn status log file exists and netdata has permission to read it')
            return False
        else:
            self.info('Plugin was started succesfully')
            return True

    def _get_raw_data(self):
        """
        Open log file
        :return: str
        """
        try:
            with open(self.log_path, 'rt') as log:
                result = log.read()
        except Exception:
            return None
        else:
            return result

    def _get_data(self):
        """
        Parse openvpn-status log file.
        Current regex version is ok for status-version 1, 2 and 3. Both users and bytes in/out are collecting.
        """

        raw_data = self._get_raw_data()
        try:
            data_inter = self.regex_data_inter.search(' '.join(raw_data.splitlines())).group()
        except AttributeError:
            data_inter = ''

        data_final = ' '.join(self.regex_data_final.findall(data_inter))
        users = self.regex_users.subn('', data_final)[1]
        traffic = self.regex_traffic.findall(data_final)

        bytes_in = sum([int(traffic[i]) for i in range(len(traffic)) if (i + 1) % 2 is 1])
        bytes_out = sum([int(traffic[i]) for i in range(len(traffic)) if (i + 1) % 2 is 0])

        return {'users': users, 'in': bytes_in, 'out': bytes_out}
