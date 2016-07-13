# -*- coding: utf-8 -*-
# Description: hddtemp netdata python.d module
# Author: Pawel Krupa (paulfantom)

from base import SocketService

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


class Service(SocketService):
    def __init__(self, configuration=None, name=None):
        SocketService.__init__(self, configuration=configuration, name=name)
        self.request = ""
        self.host = "127.0.0.1"
        self.port = 7634
        self.order = ORDER
        self.definitions = CHARTS

    def _get_data(self):
        """
        Get data from TCP/IP socket
        :return: dict
        """
        try:
            raw = self._get_raw_data().split("|")[:-1]
        except AttributeError:
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
        """
        Parse configuration, check if hddtemp is available, and dynamically create chart lines data
        :return: boolean
        """
        self._parse_config()
        data = self._get_data()
        if data is None:
            self.error("No data received")
            return False

        for name in data:
            self.definitions[ORDER[0]]['lines'].append([name])

        return True


