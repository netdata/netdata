# -*- coding: utf-8 -*-
# Description: 1-wire temperature monitor netdata python.d module
# Author: Diomidis Spinellis <http://www.spinellis.gr>
# SPDX-License-Identifier: GPL-3.0-or-later

import os
import re
from bases.FrameworkServices.SimpleService import SimpleService

# default module values (can be overridden per job in `config`)
update_every = 5

# Location where 1-Wire devices can be found
W1_DIR = '/sys/bus/w1/devices/'

# Lines matching the following regular expression contain a temperature value
RE_TEMP = re.compile(r' t=(\d+)')

ORDER = [
    'temp',
]

CHARTS = {
    'temp': {
        'options': [None, '1-Wire Temperature Sensor', 'Celsius', 'Temperature', 'w1sensor.temp', 'line'],
        'lines': []
    }
}

# Known and supported family members
# Based on linux/drivers/w1/w1_family.h and w1/slaves/w1_therm.c
THERM_FAMILY = {
    '10': 'W1_THERM_DS18S20',
    '22': 'W1_THERM_DS1822',
    '28': 'W1_THERM_DS18B20',
    '3b': 'W1_THERM_DS1825',
    '42': 'W1_THERM_DS28EA00',
}


class Service(SimpleService):
    """Provide netdata service for 1-Wire sensors"""
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.probes = []

    def check(self):
        """Auto-detect available 1-Wire sensors, setting line definitions
        and probes to be monitored."""
        try:
            file_names = os.listdir(W1_DIR)
        except OSError as err:
            self.error(err)
            return False

        lines = []
        for file_name in file_names:
            if file_name[2] != '-':
                continue
            if not file_name[0:2] in THERM_FAMILY:
                continue

            self.probes.append(file_name)
            identifier = file_name[3:]
            name = identifier
            config_name = self.configuration.get('name_' + identifier)
            if config_name:
                name = config_name
            lines.append(['w1sensor_temp_' + identifier, name, 'absolute',
                          1, 10])
        self.definitions['temp']['lines'] = lines
        return len(self.probes) > 0

    def get_data(self):
        """Return data read from sensors."""
        data = dict()

        for file_name in self.probes:
            file_path = W1_DIR + file_name + '/w1_slave'
            identifier = file_name[3:]
            try:
                with open(file_path, 'r') as device_file:
                    for line in device_file:
                        matched = RE_TEMP.search(line)
                        if matched:
                            # Round to one decimal digit to filter-out noise
                            value = round(int(matched.group(1)) / 1000., 1)
                            value = int(value * 10)
                            data['w1sensor_temp_' + identifier] = value
            except (OSError, IOError) as err:
                self.error(err)
                continue
        return data or None
