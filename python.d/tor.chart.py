# -*- coding: utf-8 -*-
# Description: Tor Netdata python.d plugin
# Author: Federico Ceratto <federico.ceratto@gmail.com>
# © 2017 - released under GPLv3, see LICENSE file
#
# Create a configuration file at /etc/netdata/tor.conf
#
# local:
#     connect_method: 'port'
#     password: 'REPLACEME'
#     port: 9051
#     socket_path: '/tmp/REPLACEME'
#
# You can use "port" or "socket" as "connect_method"
# The "port" and "socket_path" params will be used accodingly

from bases.FrameworkServices.SocketService import SocketService

try:
    import stem
    import stem.connection
    import stem.control
    ControllerError = stem.ControllerError
    STEM_AVAILABLE = True
except ImportError:
    ControllerError = Exception
    STEM_AVAILABLE = False

NAME = "Tor"

# default module values
# update_every = 10
priority = 90000
retries = 60

ORDER = ['traffic']
CHARTS = {
    'traffic': {
        'options': [None, 'Tor traffic', 'bytes/s', 'Tor traffic', 'tor.traffic', 'line'],
        'lines': [
            ['read', 'read', 'incremental'],
            ['write', 'write', 'incremental'],
        ]
    }
}


class Service(SocketService):
    """Provide netdata service for Tor"""
    def __init__(self, configuration=None, name=None):
        SocketService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self._connect_method = self.configuration.get('connect_method', 'port')
        self._password = self.configuration.get('password', None)
        self._port = self.configuration.get('port', 9051)
        self._socket_path = self.configuration.get('socket_path', '')

    def connect_to_daemon(self):
        """Connect to the local tor daemon and authenticate
        """
        if self._connect_method == 'port':
            self._controller = stem.control.Controller.from_port(port=self._port)
        else:
            self._controller = stem.control.Controller.from_socket_file(path=self._socket_path)

        try:
            self._controller.authenticate(password=self._password)
        except stem.connection.AuthenticationFailure as e:
            self.error('Authentication failed ({})'.format(e))

    def check(self):
        """
        Parse configuration, check if Tor is available
        :return: boolean
        """
        self._parse_config()
        if not STEM_AVAILABLE:
            self.error("The stem library is missing")
            return False

        self.connect_to_daemon()
        return True

    def get_metric(self, name):
        try:
            return self._controller.get_info(name)
        except ControllerError:
            # Try to recreate the controller and authenticate - just once
            self.connect_to_daemon()
            return self._controller.get_info(name)

    def _get_data(self):
        """Get host metrics
        :return: dict
        """
        tr = self.get_metric('traffic/read')
        tw = self.get_metric('traffic/written')
        data = {
            'read': int(tr),
            'write': int(tw),
        }
        return data
