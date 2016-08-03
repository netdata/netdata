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
        'options': ['disks_temp', 'temperature', 'Celsius', 'Disks temperature', 'hddtemp.temp', 'line'],
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
        self.disk_count = 1
        self.exclude = []

    def _get_disk_count(self):
        all_disks = [f for f in os.listdir("/dev") if len(f) == 3 and f.startswith("sd")]
        for disk in self.exclude:
            try:
                all_disks.remove(disk)
            except:
                self.debug("Disk not found")
        return len(all_disks)

    def _check_raw_data(self, data):
        if not data.endswith('|'):
            return False

        if data.count('|') % (5 * self.disk_count) == 0:
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
        try:
            self.exclude = list(self.configuration['exlude'])
        except (KeyError, TypeError) as e:
            self.info("No excluded disks")
            self.debug(str(e))

        try:
            self.disk_count = int(self.configuration['disk_count'])
        except (KeyError, TypeError) as e:
            self.info("Autodetecting number of disks")
            self.disk_count = self._get_disk_count()
            self.debug(str(e))

        data = self._get_data()
        if data is None:
            return False

        for name in data:
            self.definitions[ORDER[0]]['lines'].append([name])

        return True


