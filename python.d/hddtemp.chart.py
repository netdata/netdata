# -*- coding: utf-8 -*-
# Description: hddtemp netdata python.d plugin
# Author: Pawel Krupa (paulfantom)

from base import SimpleService
import socket

# default module values (can be overridden per job in `config`)
#update_every = 2
priority = 60000
retries = 5

# default job configuration (overridden by python.d.plugin)
# config = {'local': {
#             'update_every': update_every,
#             'retries': retries,
#             'priority': priority,
#             'host': 'localhost',
#             'port': 7634
#          }}

ORDER = ['temperatures']

CHARTS = {
    'temperatures': {
        'options': ['disks_temp', 'temperature', 'Celsius', 'Disks temperature', 'hddtemp.temp', 'line'],
        'lines': [
            # lines are created dynamically in `check()` method
        ]}
}


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        self.host = "localhost"
        self.port = 7634
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS

    def _get_data(self):
        """
        Get data from TCP/IP socket
        :return: dict
        """
        try:
            s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
            s.connect((self.host, self.port))
            raw = s.recv(4096).split("|")[:-1]
            s.close()
        except Exception:
            return None

        data = {}
        for i in range(len(raw) // 5):
            try:
                val = int(raw[i*5+3])
            except ValueError:
                val = 0
            data[raw[i*5+1].replace("/dev/", "")] = val
        return data

    def check(self):
        if self.name is not None or self.name != str(None):
            self.name = ""
        else:
            self.name = str(self.name)
        try:
            self.host = str(self.configuration['host'])
        except (KeyError, TypeError):
            self.error("No host specified. Using: '" + self.host + "'")
        try:
            self.port = int(self.configuration['port'])
        except (KeyError, TypeError):
            self.error("No port specified. Using: '" + str(self.port) + "'")

        data = self._get_data()
        if data is None:
            self.error("No data received")
            return False

        for name in data:
            self.definitions[ORDER[0]]['lines'].append([name])

        return True


