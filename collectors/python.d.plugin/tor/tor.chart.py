# -*- coding: utf-8 -*-
# Description: adaptec_raid netdata python.d module
# Author: Federico Ceratto <federico.ceratto@gmail.com>
# Author: Ilya Mashchenko (l2isbad)
# SPDX-License-Identifier: GPL-3.0-or-later


from bases.FrameworkServices.SimpleService import SimpleService

try:
    import stem
    import stem.connection
    import stem.control
    STEM_AVAILABLE = True
except ImportError:
    STEM_AVAILABLE = False


DEF_PORT = 'default'

ORDER = [
    'traffic',
]

CHARTS = {
    'traffic': {
        'options': [None, 'Tor Traffic', 'KiB/s', 'traffic', 'tor.traffic', 'area'],
        'lines': [
            ['read', 'read', 'incremental', 1, 1024],
            ['write', 'write', 'incremental', 1, -1024],
        ]
    }
}


class Service(SimpleService):
    """Provide netdata service for Tor"""
    def __init__(self, configuration=None, name=None):
        super(Service, self).__init__(configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.port = self.configuration.get('control_port', DEF_PORT)
        self.password = self.configuration.get('password')
        self.use_socket = isinstance(self.port, str) and self.port != DEF_PORT and not self.port.isdigit()
        self.conn = None
        self.alive = False

    def check(self):
        if not STEM_AVAILABLE:
            self.error('the stem library is missing')
            return False

        return self.connect()

    def get_data(self):
        if not self.alive and not self.reconnect():
            return None

        data = dict()

        try:
            data['read'] = self.conn.get_info('traffic/read')
            data['write'] = self.conn.get_info('traffic/written')
        except stem.ControllerError as error:
            self.debug(error)
            self.alive = False

        return data or None

    def authenticate(self):
        try:
            self.conn.authenticate(password=self.password)
        except stem.connection.AuthenticationFailure as error:
            self.error('authentication error: {0}'.format(error))
            return False
        return True

    def connect_via_port(self):
        try:
            self.conn = stem.control.Controller.from_port(port=self.port)
        except (stem.SocketError, ValueError) as error:
            self.error(error)

    def connect_via_socket(self):
        try:
            self.conn = stem.control.Controller.from_socket_file(path=self.port)
        except (stem.SocketError, ValueError) as error:
            self.error(error)

    def connect(self):
        if self.conn:
            self.conn.close()
            self.conn = None

        if self.use_socket:
            self.connect_via_socket()
        else:
            self.connect_via_port()

        if self.conn and self.authenticate():
            self.alive = True

        return self.alive

    def reconnect(self):
        return self.connect()
