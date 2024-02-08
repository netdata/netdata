# _*_ coding: utf-8 _*_
# Description: AM2320 netdata module
# Author: tommybuck
# SPDX-License-Identifier: GPL-3.0-or-Later

try:
    import board
    import busio
    import adafruit_am2320

    HAS_AM2320 = True
except ImportError:
    HAS_AM2320 = False

from bases.FrameworkServices.SimpleService import SimpleService

ORDER = [
    'temperature',
    'humidity',
]

CHARTS = {
    'temperature': {
        'options': [None, 'Temperature', 'celsius', 'temperature', 'am2320.temperature', 'line'],
        'lines': [
            ['temperature']
        ]
    },
    'humidity': {
        'options': [None, 'Relative Humidity', 'percentage', 'humidity', 'am2320.humidity', 'line'],
        'lines': [
            ['humidity']
        ]
    }
}


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.am = None

    def check(self):
        if not HAS_AM2320:
            self.error("Could not find the adafruit-circuitpython-am2320 package.")
            return False

        try:
            i2c = busio.I2C(board.SCL, board.SDA)
            self.am = adafruit_am2320.AM2320(i2c)
        except ValueError as error:
            self.error("error on creating I2C shared bus : {0}".format(error))
            return False

        return True

    def get_data(self):
        try:
            return {
                'temperature': self.am.temperature,
                'humidity': self.am.relative_humidity,
            }

        except (OSError, RuntimeError) as error:
            self.error(error)
            return None
