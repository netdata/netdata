# -*- coding: utf-8 -*-
# Description: sensors netdata python.d plugin
# Author: Pawel Krupa (paulfantom)

from base import SimpleService
import lm_sensors as sensors

# default module values (can be overridden per job in `config`)
# update_every = 2

ORDER = ['temperature', 'fan', 'voltage', 'current', 'power', 'energy', 'humidity']

# This is a prototype of chart definition which is used to dynamically create self.definitions
CHARTS = {
    'temperature': {
        'options': [None, ' temperature', 'Celsius', 'temperature', 'sensors.temperature', 'line'],
        'lines': [
            [None, None, 'absolute', 1, 1000]
        ]},
    'voltage': {
        'options': [None, ' voltage', 'Volts', 'voltage', 'sensors.voltage', 'line'],
        'lines': [
            [None, None, 'absolute', 1, 1000]
        ]},
    'current': {
        'options': [None, ' current', 'Ampere', 'current', 'sensors.current', 'line'],
        'lines': [
            [None, None, 'absolute', 1, 1000]
        ]},
    'power': {
        'options': [None, ' power', 'Watt', 'power', 'sensors.power', 'line'],
        'lines': [
            [None, None, 'absolute', 1, 1000000]
        ]},
    'fan': {
        'options': [None, ' fans speed', 'Rotations/min', 'fans', 'sensors.fan', 'line'],
        'lines': [
            [None, None, 'absolute', 1, 1000]
        ]},
    'energy': {
        'options': [None, ' energy', 'Joule', 'energy', 'sensors.energy', 'areastack'],
        'lines': [
            [None, None, 'incremental', 1, 1000000]
        ]},
    'humidity': {
        'options': [None, ' humidity', 'Percent', 'humidity', 'sensors.humidity', 'line'],
        'lines': [
            [None, None, 'absolute', 1, 1000]
        ]}
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
        self.order = []
        self.definitions = {}
        self.chips = []

    def _get_data(self):
        data = {}
        try:
            for chip in sensors.ChipIterator():
                prefix = sensors.chip_snprintf_name(chip)
                for feature in sensors.FeatureIterator(chip):
                    sfi = sensors.SubFeatureIterator(chip, feature)
                    for sf in sfi:
                        val = sensors.get_value(chip, sf.number)
                        break
                    data[prefix + "_" + str(feature.name.decode())] = int(val * 1000)
        except Exception as e:
            self.error(e)
            return None

        if len(data) == 0:
            return None
        return data

    def _create_definitions(self):
        prev_chip = ""
        for type in ORDER:
            for chip in sensors.ChipIterator():
                chip_name = sensors.chip_snprintf_name(chip)
                if len(self.chips) != 0 and not any([chip_name.startswith(ex) for ex in self.chips]):
                    continue
                for feature in sensors.FeatureIterator(chip):
                    sfi = sensors.SubFeatureIterator(chip, feature)
                    vals = [sensors.get_value(chip, sf.number) for sf in sfi]
                    if vals[0] == 0:
                        continue
                    if TYPE_MAP[feature.type] == type:
                        # create chart
                        if chip_name != prev_chip:
                            name = chip_name + "_" + TYPE_MAP[feature.type]
                            if name not in self.order:
                                self.order.append(name)
                                chart_def = list(CHARTS[type]['options'])
                                chart_def[1] = chip_name + chart_def[1]
                                self.definitions[name] = {'options': chart_def}
                                self.definitions[name]['lines'] = []
                        line = list(CHARTS[type]['lines'][0])
                        line[0] = chip_name + "_" + str(feature.name.decode())
                        line[1] = sensors.get_label(chip, feature)
                        self.definitions[name]['lines'].append(line)
                prev_chip = chip_name

    def check(self):
        try:
            sensors.init()
        except Exception as e:
            self.error(e)
            return False

        try:
            self._create_definitions()
        except Exception as e:
            self.error(e)
            return False
        return True
