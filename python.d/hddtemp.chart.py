# -*- coding: utf-8 -*-
# Description: hddtemp netdata python.d module
# Author: Pawel Krupa (paulfantom)

import os
from base import SocketService

# default module values (can be overridden per job in `config`)
#update_every = 2
priority = 60000
retries = 60

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
        'options': ['disks_temp', 'Disks Temperatures', 'Celsius', 'temperatures', 'hddtemp.temperatures', 'line'],
        'lines': [
            # lines are created dynamically in `check()` method
        ]}
}


class Service(SocketService):
    def __init__(self, configuration=None, name=None):
        SocketService.__init__(self, configuration=configuration, name=name)
        self._keep_alive = False
        self.request = ""
        self.host = "127.0.0.1"
        self.port = 7634
        self.order = ORDER
        self.definitions = CHARTS
        self.disks = []

    def _get_disks(self):
        try:
            disks = self.configuration['devices']
            self.info("Using configured disks" + str(disks))
        except (KeyError, TypeError) as e:
            self.info("Autodetecting disks")
            return ["/dev/" + f for f in os.listdir("/dev") if len(f) == 3 and f.startswith("sd")]

        ret = []
        for disk in disks:
            if not disk.startswith('/dev/'):
                disk = "/dev/" + disk
            ret.append(disk)
        if len(ret) == 0:
            self.error("Provided disks cannot be found in /dev directory.")
        return ret

    def _check_raw_data(self, data):
        if not data.endswith('|'):
            return False

        if all(disk in data for disk in self.disks):
            return True

        return False

    def _get_data(self):
        """
        Get data from TCP/IP socket
        :return: dict
        """
        try:
            raw = self._get_raw_data().split("|")[:-1]
        except AttributeError:
            self.error("no data received")
            return None
        data = {}
        for i in range(len(raw) // 5):
            if not raw[i*5+1] in self.disks:
                continue
            try:
                val = int(raw[i*5+3])
            except ValueError:
                val = 0
            data[raw[i*5+1].replace("/dev/", "")] = val

        if len(data) == 0:
            self.error("received data doesn't have needed records")
            return None
        else:
            return data

    def check(self):
        """
        Parse configuration, check if hddtemp is available, and dynamically create chart lines data
        :return: boolean
        """
        self._parse_config()
        self.disks = self._get_disks()

        data = self._get_data()
        if data is None:
            return False

        for name in data:
            self.definitions[ORDER[0]]['lines'].append([name])

        return True


