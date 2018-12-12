# -*- coding: utf-8 -*-
# Description: simple port check netdata python.d module
# Original Author: ccremer (github.com/ccremer)
# SPDX-License-Identifier: GPL-3.0-or-later

import socket

try:
    from time import monotonic as time
except ImportError:
    from time import time

from bases.FrameworkServices.SimpleService import SimpleService

# default module values (can be overridden per job in `config`)
priority = 60000

PORT_LATENCY = 'connect'

PORT_SUCCESS = 'success'
PORT_TIMEOUT = 'timeout'
PORT_FAILED = 'no_connection'

ORDER = ['latency', 'status']

CHARTS = {
    'latency': {
        'options': [None, 'TCP connect latency', 'ms', 'latency', 'portcheck.latency', 'line'],
        'lines': [
            [PORT_LATENCY, 'connect', 'absolute', 100, 1000]
        ]
    },
    'status': {
        'options': [None, 'Portcheck status', 'boolean', 'status', 'portcheck.status', 'line'],
        'lines': [
            [PORT_SUCCESS, 'success', 'absolute'],
            [PORT_TIMEOUT, 'timeout', 'absolute'],
            [PORT_FAILED, 'no connection', 'absolute']
        ]
    }
}


# Not deriving from SocketService, too much is different
class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.host = self.configuration.get('host')
        self.port = self.configuration.get('port')
        self.timeout = self.configuration.get('timeout', 1)

    def check(self):
        """
        Parse configuration, check if configuration is available, and dynamically create chart lines data
        :return: boolean
        """
        if self.host is None or self.port is None:
            self.error('Host or port missing')
            return False
        if not isinstance(self.port, int):
            self.error('"port" is not an integer. Specify a numerical value, not service name.')
            return False

        self.debug('Enabled portcheck: {host}:{port}, update every {update}s, timeout: {timeout}s'.format(
            host=self.host, port=self.port, update=self.update_every, timeout=self.timeout
        ))
        # We will accept any (valid-ish) configuration, even if initial connection fails (a service might be down from
        # the beginning)
        return True

    def _get_data(self):
        """
        Get data from socket
        :return: dict
        """
        data = dict()
        data[PORT_SUCCESS] = 0
        data[PORT_TIMEOUT] = 0
        data[PORT_FAILED] = 0

        success = False
        try:
            for socket_config in socket.getaddrinfo(self.host, self.port, socket.AF_UNSPEC, socket.SOCK_STREAM):
                # use first working socket
                sock = self._create_socket(socket_config)
                if sock is not None:
                    self._connect2socket(data, socket_config, sock)
                    self._disconnect(sock)
                    success = True
                    break
        except socket.gaierror as error:
            self.debug('Failed to connect to "{host}:{port}", error: {error}'.format(
                host=self.host, port=self.port, error=error
            ))

        # We could not connect
        if not success:
            data[PORT_FAILED] = 1

        return data

    def _create_socket(self, socket_config):
        af, sock_type, proto, _, sa = socket_config
        try:
            self.debug('Creating socket to "{address}", port {port}'.format(address=sa[0], port=sa[1]))
            sock = socket.socket(af, sock_type, proto)
            sock.settimeout(self.timeout)
            return sock
        except socket.error as error:
            self.debug('Failed to create socket "{address}", port {port}, error: {error}'.format(
                address=sa[0], port=sa[1], error=error
            ))
            return None

    def _connect2socket(self, data, socket_config, sock):
        """
        Connect to a socket, passing the result of getaddrinfo()
        :return: dict
        """

        af, _, proto, _, sa = socket_config
        port = str(sa[1])
        try:
            self.debug('Connecting socket to "{address}", port {port}'.format(address=sa[0], port=port))
            start = time()
            sock.connect(sa)
            diff = time() - start
            self.debug('Connected to "{address}", port {port}, latency {latency}'.format(
                address=sa[0], port=port, latency=diff
            ))
            # we will set it at least 0.1 ms. 0.0 would mean failed connection (handy for 3rd-party-APIs)
            data[PORT_LATENCY] = max(round(diff * 10000), 0)
            data[PORT_SUCCESS] = 1

        except socket.timeout as error:
            self.debug('Socket timed out on "{address}", port {port}, error: {error}'.format(
                address=sa[0], port=port, error=error
            ))
            data[PORT_TIMEOUT] = 1

        except socket.error as error:
            self.debug('Failed to connect to "{address}", port {port}, error: {error}'.format(
                address=sa[0], port=port, error=error
            ))
            data[PORT_FAILED] = 1

    def _disconnect(self, sock):
        """
        Close socket connection
        :return:
        """
        if sock is not None:
            try:
                self.debug('Closing socket')
                sock.shutdown(2)  # 0 - read, 1 - write, 2 - all
                sock.close()
            except socket.error:
                pass
