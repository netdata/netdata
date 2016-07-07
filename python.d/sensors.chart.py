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
        'options': [None, ' temperature', 'Celsius', 'temperature', 'sensors.temp', 'line'],
        'lines': [
            [None, None, 'absolute', 1, 1000]
        ]},
    'voltage': {
        'options': [None, ' voltage', 'Volts', 'voltage', 'sensors.volt', 'line'],
        'lines': [
            [None, None, 'absolute', 1, 1000]
        ]},
    'current': {
        'options': [None, ' current', 'Ampere', 'current', 'sensors.curr', 'line'],
        'lines': [
            [None, None, 'absolute', 1, 1000]
        ]},
    'power': {
        'options': [None, ' power', 'Watt', 'power', 'sensors.power', 'line'],
        'lines': [
            [None, None, 'absolute', 1, 1000000]
        ]},
    'fan': {
        'options': [None, ' fans speed', 'Rotations/min', 'fans', 'sensors.fans', 'line'],
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


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.order = []
        self.definitions = {}

    def _get_data(self):
        data = {}
        try:
            for chip in sensors.iter_detected_chips():
                prefix = '_'.join(str(chip.path.decode()).split('/')[3:])
                lines = {}
                for feature in chip:
                    data[prefix + "_" + str(feature.name.decode())] = feature.get_value() * 1000
        except Exception as e:
            self.error(e)
            return None

        if len(data) == 0:
            return None
        return data

    def _create_definitions(self):
        for type in ORDER:
            for chip in sensors.iter_detected_chips():
                prefix = '_'.join(str(chip.path.decode()).split('/')[3:])
                name = ""
                lines = []
                for feature in chip:
                    if feature.get_value() != 0:
                        continue
                    if sensors.TYPE_DICT[feature.type] == type:
                        name = str(chip.prefix.decode()) + "_" + sensors.TYPE_DICT[feature.type]
                        if name not in self.order:
                            options = list(CHARTS[type]['options'])
                            options[1] = str(chip.prefix) + options[1]
                            self.definitions[name] = {'options': options}
                            self.definitions[name]['lines'] = []
                            self.order.append(name)
                        line = list(CHARTS[type]['lines'][0])
                        line[0] = prefix + "_" + str(feature.name.decode())
                        line[1] = str(feature.label)
                        self.definitions[name]['lines'].append(line)

    def check(self):
        try:
            sensors.init()
        except Exception as e:
            self.error(e)
            return False
        try:
            self._create_definitions()
        except:
            return False
        return True