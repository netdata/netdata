# -*- coding: utf-8 -*-
# Description: sensors netdata python.d plugin
# Author: Pawel Krupa (paulfantom)
# SPDX-License-Identifier: GPL-3.0-or-later

from bases.FrameworkServices.SimpleService import SimpleService
from third_party import lm_sensors as sensors

ORDER = [
    'temperature',
    'fan',
    'voltage',
    'current',
    'power',
    'energy',
    'humidity',
]

# This is a prototype of chart definition which is used to dynamically create self.definitions
CHARTS = {
    'temperature': {
        'options': [None, ' temperature', 'Celsius', 'temperature', 'sensors.temperature', 'line'],
        'lines': [
            [None, None, 'absolute', 1, 1000]
        ]
    },
    'voltage': {
        'options': [None, ' voltage', 'Volts', 'voltage', 'sensors.voltage', 'line'],
        'lines': [
            [None, None, 'absolute', 1, 1000]
        ]
    },
    'current': {
        'options': [None, ' current', 'Ampere', 'current', 'sensors.current', 'line'],
        'lines': [
            [None, None, 'absolute', 1, 1000]
        ]
    },
    'power': {
        'options': [None, ' power', 'Watt', 'power', 'sensors.power', 'line'],
        'lines': [
            [None, None, 'absolute', 1, 1000]
        ]
    },
    'fan': {
        'options': [None, ' fans speed', 'Rotations/min', 'fans', 'sensors.fan', 'line'],
        'lines': [
            [None, None, 'absolute', 1, 1000]
        ]
    },
    'energy': {
        'options': [None, ' energy', 'Joule', 'energy', 'sensors.energy', 'line'],
        'lines': [
            [None, None, 'incremental', 1, 1000]
        ]
    },
    'humidity': {
        'options': [None, ' humidity', 'Percent', 'humidity', 'sensors.humidity', 'line'],
        'lines': [
            [None, None, 'absolute', 1, 1000]
        ]
    }
}

LIMITS = {
    'temperature': [-127, 1000],
    'voltage': [-127, 127],
    'current': [-127, 127],
    'fan': [0, 65535]
}

TYPE_MAP = {
    0: 'voltage',
    1: 'fan',
    2: 'temperature',
    3: 'power',
    4: 'energy',
    5: 'current',
    6: 'humidity',
    7: 'max_main',
    16: 'vid',
    17: 'intrusion',
    18: 'max_other',
    24: 'beep_enable'
}


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.order = list()
        self.definitions = dict()
        self.chips = configuration.get('chips')

    def get_data(self):
        data = dict()
        try:
            for chip in sensors.ChipIterator():
                prefix = sensors.chip_snprintf_name(chip)
                for feature in sensors.FeatureIterator(chip):
                    sfi = sensors.SubFeatureIterator(chip, feature)
                    val = None
                    for sf in sfi:
                        try:
                            val = sensors.get_value(chip, sf.number)
                            break
                        except sensors.SensorsError:
                            continue
                    if val is None:
                        continue
                    type_name = TYPE_MAP[feature.type]
                    if type_name in LIMITS:
                        limit = LIMITS[type_name]
                        if val < limit[0] or val > limit[1]:
                            continue
                    data[prefix + '_' + str(feature.name.decode())] = int(val * 1000)
        except sensors.SensorsError as error:
            self.error(error)
            return None

        return data or None

    def create_definitions(self):
        for sensor in ORDER:
            for chip in sensors.ChipIterator():
                chip_name = sensors.chip_snprintf_name(chip)
                if self.chips and not any([chip_name.startswith(ex) for ex in self.chips]):
                    continue
                for feature in sensors.FeatureIterator(chip):
                    sfi = sensors.SubFeatureIterator(chip, feature)
                    vals = list()
                    for sf in sfi:
                        try:
                            vals.append(sensors.get_value(chip, sf.number))
                        except sensors.SensorsError as error:
                            self.error('{0}: {1}'.format(sf.name, error))
                            continue
                    if not vals or (vals[0] == 0 and feature.type != 1):
                        continue
                    if TYPE_MAP[feature.type] == sensor:
                        # create chart
                        name = chip_name + '_' + TYPE_MAP[feature.type]
                        if name not in self.order:
                            self.order.append(name)
                            chart_def = list(CHARTS[sensor]['options'])
                            chart_def[1] = chip_name + chart_def[1]
                            self.definitions[name] = {'options': chart_def}
                            self.definitions[name]['lines'] = []
                        line = list(CHARTS[sensor]['lines'][0])
                        line[0] = chip_name + '_' + str(feature.name.decode())
                        line[1] = sensors.get_label(chip, feature)
                        self.definitions[name]['lines'].append(line)

    def check(self):
        try:
            sensors.init()
        except sensors.SensorsError as error:
            self.error(error)
            return False

        self.create_definitions()

        return bool(self.get_data())
