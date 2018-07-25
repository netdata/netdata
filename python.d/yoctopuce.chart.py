# -*- coding: utf-8 -*-
# Description: yoctopuce netdata python.d module
# Author: Austin S. Hemmelgarn (Ferroin)
# SPDX-License-Identifier: GPL-3.0+

# Much of the check code in this module looks rather insane.
# Unfortunately, it's either that, or a few hundred extra lines of code,
# because the Yoctopuce API's for Python are very poorly written.

from copy import copy
from importlib import import_module

from bases.FrameworkServices.SimpleService import SimpleService

try:
    # We import the other stuff we need dynamically below.
    from yoctopuce import yocto_api
    HAVE_YOCTOPUCE = True
except ImportError:
    HAVE_YOCTOPUCE = False


# USB calls produced by this module are somewhat expensive, and while
# the sensor modules technically have 1Hz refresh rates, the actual sensors
# usually take longer than 1s to stabilize on a value.
update_every = 5

# This is the global order, a subset for the actual list of charts we
# can provide is created at runtime.
ORDER = ['light', 'temperature', 'humidity', 'pressure', 'current', 'voltage', 'voc', 'co2']

# Charts are generated dynamically during the check method.
CHARTS = {}

# Map type names to the various other associated names.
NAME_MAP = {
    'LightSensor': {
        'module': 'yocto_lightsensor',
        'class': 'YLightSensor',
        'chart': 'light',
        'longname': 'Ambient Light',
        'context': 'yoctopuce.light',
    },
    'Temperature': {
        'module': 'yocto_temperature',
        'class': 'YTemperature',
        'chart': 'temperature',
        'longname': 'Temperature',
        'context': 'yoctopuce.temperature',
    },
    'Humidity': {
        'module': 'yocto_humidity',
        'class': 'YHumidity',
        'chart': 'humidity',
        'longname': 'Humidity',
        'context': 'yoctopuce.humidity',
    },
    'Pressure': {
        'module': 'yocto_pressure',
        'class': 'YPressure',
        'chart': 'pressure',
        'longname': 'Pressure',
        'context': 'yoctopuce.pressure',
    },
    'Current': {
        'module': 'yocto_current',
        'class': 'YCurrent',
        'chart': 'current',
        'longname': 'Current',
        'context': 'yoctopuce.current'
    },
    'Voltage': {
        'module': 'yocto_voltage',
        'class': 'YVoltage',
        'chart': 'voltage',
        'longname': 'Voltage',
        'context': 'yoctopuce.voltage'
    },
    'Voc': {
        'module': 'yocto_voc',
        'class': 'YVoc',
        'chart': 'voc',
        'longname': 'Volatile Organice Compounds',
        'context': 'yoctopuce.voc'
    },
    'CarbonDioxide': {
        'module': 'yocto_carbondioxide',
        'class': 'YCarbonDioxide',
        'chart': 'co2',
        'longname': 'Carbon Dioxide',
        'context': 'yoctopuce.co2'
    }
}


def get_chart(name, units):
    '''Generate a chart with the given parameters.

       This returns chart definition with no lines.'''
    return {
        'options': [None, NAME_MAP[name]['longname'], units, 'yoctopuce', NAME_MAP[name]['context'], 'line'],
        'lines': []
    }


def get_line(dim, resolution):
    '''Return a line with the given dimension and resolution.'''
    return [
        dim,
        dim,
        'absolute',
        1,
        resolution
    ]


def get_sensor_definition(sensor):
    '''Return a dict with all the info we need about a sensor.'''
    return {
        'sensor': sensor,
        'units': sensor.get_unit(),
        'precision': int(1.0 / sensor.get_resolution()),
        'name': format('yoctopuce.{0}', sensor.get_friendlyName()),
        'id': sensor.get_friendlyName()
    }


def get_sensor_class(name):
    '''Get the class for a particular sensor type.'''
    module = import_module(format('.{0}', NAME_MAP[item]['module']), 'yoctopuce')
    return getattr(module, NAME_MAP[item]['class'])


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.order = list()
        self.definitions = dict()
        self.hub = self.configuration.get('hub', '127.0.0.1')
        self.scan = self.configuration.get('scan', True)
        self.sensors = list()

    def __scan_sensors(self):
        '''This scans for sensors of all supported types.

           This isn't really all that complex, it just looks that way
           because accessing attributes with runtime computed names in
           python is a pain in the arse.  For each sensor type it does
           the following:

           - Find and load the required yoctopuce module, and get a
             class reference for the sensor type from it.
           - Call the appropriate method of that class to get a reference
             to the first sensor in the discovery order and add that to
             our sensor list.  If this finds nothing, skip to the
             next type.
           - Loop over the resultant objects to get the full list (they
             work like a C-style linked list).
           - Build up a chart out of the sensors we found.

           Returns a 2-tuple consisting of the chart definitions and
           the chart order.
           '''
        charts = dict()
        order = copy(ORDER)
        for item in [x['chart'] for x in NAME_MAP]:
            sensors = list()
            try:
                base = get_sensor_class(item)
                sensor = base.FirstSensor()
                entry = get_sensor_definition(sensor)
                if entry:
                    sensors.append(entry)
                else:
                    continue
            except yocto_api.YAPI_Exception:
                order.remove(item)
                continue
            while True:
                sensor = sensors[-1]['sensor'].nextSensor()
                if not sensor:
                    break
                entry = get_sensor_definition(sensor)
                if entry:
                    sensors.append(entry)
                else:
                    continue
            # This conditional _should_ never evaluate to true, but it's
            # better to be on the safe size.
            if not sensors:
                order.remove(item)
                continue
            self.sensors.extend(sensors)
            charts[item] = get_chart(item, sensors[0]['unit'])
            for entry in sensors:
                chart[item]['lines'].append(get_line(entry['name'], entry['precision']))
        return (charts, order)

    def __find_sensors(self):
        '''Search for a group of sensors based on our config.

           Just like __scan_sensors, this isn't really complex, it just
           looks that way.'''
        charts = dict()
        order = copy(ORDER)
        search = self.config.get('search', dict())
        for item in [x['chart'] for x in NAME_MAP]:
            sensors = list()
            base = get_sensor_class(item)
            for param in [x for x in search.get(item, list()) if 'id' in x]:
                try:
                    sensor = base.FindSensor(param['id'])
                    entry = get_sensor_definition(sensor)
                    if entry:
                        if param.get('name'):
                            entry['name'] = param['name']
                        sensors.append(entry)
                    else:
                        continue
            if not sensors:
                order.remove(item)
                continue
            self.sensors.extend(sensors)
            charts[item] = get_chart(item, sensors[0]['unit'])
            for entry in sensors:
                chart[item]['lines'].append(get_line(entry['name'], entry['precision']))
        return (charts, order)

    def check(self):
        if not HAVE_YOCTOPUCE:
            self.error('Unable to load Yoctopuce library.')
            return False

        try:
            yapi.RegisterHub(self.hub)
            yapi.UpdateDeviceList()
        except yocto_api.YAPI_Exception:
            self.error('Unable to enumerate devices.')
            return False

        if self.scan:
            chartinfo = self.__scan_sensors()
        else:
            chartinfo = self.__find_sensors()

        if not any(self.sensors.values()):
            self.error('No devices found')
            return False

        self.definitions.update(chartinfo[0])
        self.order.extend(chartinfo[1])
        return True

    def _get_data(self):
        ret = dict()
        for sensor in self.sensors:
            try:
                if not sensor['sensor'].isOnline():
                    # Skip offline sensors
                    continue
                ret[sensor['name']] = sensor['sensor'].get_currentValue() * sensor['precision']
            except yocto_api.YAPI_Exception:
                self.warning(format('Error accessing sensor {0}', sensor['id']))
                continue
        return ret
