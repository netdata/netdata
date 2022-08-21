# -*- coding: utf-8 -*-
# Description: sensors netdata python.d plugin
# Author: Pawel Krupa (paulfantom)
# SPDX-License-Identifier: GPL-3.0-or-later

from collections import defaultdict

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
        'options': [None, 'Temperature', 'Celsius', 'temperature', 'sensors.temperature', 'line'],
        'lines': [
            [None, None, 'absolute', 1, 1000]
        ]
    },
    'voltage': {
        'options': [None, 'Voltage', 'Volts', 'voltage', 'sensors.voltage', 'line'],
        'lines': [
            [None, None, 'absolute', 1, 1000]
        ]
    },
    'current': {
        'options': [None, 'Current', 'Ampere', 'current', 'sensors.current', 'line'],
        'lines': [
            [None, None, 'absolute', 1, 1000]
        ]
    },
    'power': {
        'options': [None, 'Power', 'Watt', 'power', 'sensors.power', 'line'],
        'lines': [
            [None, None, 'absolute', 1, 1000]
        ]
    },
    'fan': {
        'options': [None, 'Fans speed', 'Rotations/min', 'fans', 'sensors.fan', 'line'],
        'lines': [
            [None, None, 'absolute', 1, 1000]
        ]
    },
    'energy': {
        'options': [None, 'Energy', 'Joule', 'energy', 'sensors.energy', 'line'],
        'lines': [
            [None, None, 'incremental', 1, 1000]
        ]
    },
    'humidity': {
        'options': [None, 'Humidity', 'Percent', 'humidity', 'sensors.humidity', 'line'],
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
    # 7: 'max_main',
    # 16: 'vid',
    # 17: 'intrusion',
    # 18: 'max_other',
    # 24: 'beep_enable'
}


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.order = list()
        self.definitions = dict()
        self.chips = configuration.get('chips')
        self.priority = 60000

    def get_data(self):
        seen, data = dict(), dict()
        try:
            for chip in sensors.ChipIterator():
                chip_name = sensors.chip_snprintf_name(chip)
                seen[chip_name] = defaultdict(list)

                for feat in sensors.FeatureIterator(chip):
                    if feat.type not in TYPE_MAP:
                        continue

                    feat_type = TYPE_MAP[feat.type]
                    feat_name = str(feat.name.decode())
                    feat_label = sensors.get_label(chip, feat)
                    feat_limits = LIMITS.get(feat_type)
                    sub_feat = next(sensors.SubFeatureIterator(chip, feat))  # current value

                    if not sub_feat:
                        continue

                    try:
                        v = sensors.get_value(chip, sub_feat.number)
                    except sensors.SensorsError:
                        continue

                    if v is None:
                        continue

                    seen[chip_name][feat_type].append((feat_name, feat_label))

                    if feat_limits and (v < feat_limits[0] or v > feat_limits[1]):
                        continue

                    data[chip_name + '_' + feat_name] = int(v * 1000)

        except sensors.SensorsError as error:
            self.error(error)
            return None

        self.update_sensors_charts(seen)

        return data or None

    def update_sensors_charts(self, seen):
        for chip_name, feat in seen.items():
            if self.chips and not any([chip_name.startswith(ex) for ex in self.chips]):
                continue

            for feat_type, sub_feat in feat.items():
                if feat_type not in ORDER or feat_type not in CHARTS:
                    continue

                chart_id = '{}_{}'.format(chip_name, feat_type)
                if chart_id in self.charts:
                    continue

                params = [chart_id] + list(CHARTS[feat_type]['options'])
                new_chart = self.charts.add_chart(params)
                new_chart.params['priority'] = self.get_chart_priority(feat_type)

                for name, label in sub_feat:
                    lines = list(CHARTS[feat_type]['lines'][0])
                    lines[0] = chip_name + '_' + name
                    lines[1] = label
                    new_chart.add_dimension(lines)

    def check(self):
        try:
            sensors.init()
        except sensors.SensorsError as error:
            self.error(error)
            return False

        self.priority = self.charts.priority

        return bool(self.get_data() and self.charts)

    def get_chart_priority(self, feat_type):
        for i, v in enumerate(ORDER):
            if v == feat_type:
                return self.priority + i
        return self.priority
