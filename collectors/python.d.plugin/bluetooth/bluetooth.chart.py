# -*- coding: utf-8 -*-
# Description: example netdata python.d module
# Author: Put your name here (your github login)
# SPDX-License-Identifier: GPL-3.0-or-later

try:
    import csv
    import btmgmt

    HAS_BTMGMT = True
except ImportError:
    HAS_BTMGMT = False

from bases.FrameworkServices.SimpleService import SimpleService

ORDER = [
    'bluetooth',
]

CHARTS = {
    'bluetooth': {
        'options': [None, 'Connected Devices', 'number', 'connected', 'bluetooth.con', 'line'],
        'lines': [
            ['connected']
        ]
    }
}


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        # self.random = SystemRandom()
        # self.num_lines = self.configuration.get('num_lines', 4)
        # self.lower = self.configuration.get('lower', 0)
        # self.upper = self.configuration.get('upper', 100)

    def check(self):
        if not HAS_BTMGMT:
            self.error("Could not find btmgmt library")
            return False

        response = btmgmt.command_str("con")
        if response[0] != 0:
            self.error("Could not use btmgmt with correct privileges")
            return False

        return True

    def get_data(self):
        response = btmgmt.command_str("con")

        lines = response[1].splitlines()
        reader = csv.reader(lines, delimiter=' ')

        # print("lines:", response[1])

        return {'connected': int(len(lines))}

