try:
    import board
    import busio
    import adafruit_am2320
    HAS_STUFF = True
except ImportError:
    HAS_STUFF = False


#from bases.FrameworkServices.SimpleService import SimpleService
from base import SimpleService


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
        if not HAS_STUFF:
            self.error("following libraries are needed to get module working: board, busio, adafruit_am2320")
            return False

        try:
            i2c = busio.I2C(board.SCL, board.SDA)
            self.am = adafruit_am2320.AM2320(i2c)
        except Exception as error:
            self.error("error on creating I2C shared bus : {0}".format(error))
            return False

        return True

    def get_data(self):
        return {
            'temperature': self.am.temperature,
            'humidity': self.am.relative_humidity,
        }