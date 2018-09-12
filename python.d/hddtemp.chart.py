# -*- coding: utf-8 -*-
# Description: hddtemp netdata python.d module
# Author: Pawel Krupa (paulfantom)
# Author: Ilya Mashchenko (l2isbad)
# SPDX-License-Identifier: GPL-3.0+


import re

from copy import deepcopy

from bases.FrameworkServices.SocketService import SocketService


ORDER = ['temperatures']

CHARTS = {
    'temperatures': {
        'options': ['disks_temp', 'Disks Temperatures', 'Celsius', 'temperatures', 'hddtemp.temperatures', 'line'],
        'lines': [
            # lines are created dynamically in `check()` method
        ]}}

RE = re.compile(r'\/dev\/([^|]+)\|([^|]+)\|([0-9]+|SLP|UNK)\|')


class Disk:
    def __init__(self, id_, name, temp):
        self.id = id_.split('/')[-1]
        self.name = name.replace(' ', '_')
        self.temp = temp if temp.isdigit() else 0

    def __repr__(self):
        return self.id


class Service(SocketService):
    def __init__(self, configuration=None, name=None):
        SocketService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = deepcopy(CHARTS)
        self._keep_alive = False
        self.request = ""
        self.host = "127.0.0.1"
        self.port = 7634
        self.do_only = self.configuration.get('devices')

    def get_disks(self):
        r = self._get_raw_data()

        if not r:
            return None

        m = RE.findall(r)

        if not m:
            self.error("received data doesn't have needed records")
            return None

        rv = [Disk(*d) for d in m]
        self.debug('available disks: {0}'.format(rv))

        if self.do_only:
            return [v for v in rv if v.id in self.do_only]
        return rv

    def get_data(self):
        """
        Get data from TCP/IP socket
        :return: dict
        """

        disks = self.get_disks()

        if not disks:
            return None

        return dict((d.id, d.temp) for d in disks)

    def check(self):
        """
        Parse configuration, check if hddtemp is available, and dynamically create chart lines data
        :return: boolean
        """
        self._parse_config()
        disks = self.get_disks()

        if not disks:
            return False

        for d in disks:
            n = d.id if d.id.startswith('sd') else d.name
            dim = [d.id, n]
            self.definitions['temperatures']['lines'].append(dim)

        return True

    @staticmethod
    def _check_raw_data(data):
        return not bool(data)
