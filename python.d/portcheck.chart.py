# -*- coding: utf-8 -*-
# Description: simple port check netdata python.d module
# Original Author: ccremer (github.com/ccremer)

import socket
import time
from bases.FrameworkServices.SimpleService import SimpleService

# default module values (can be overridden per job in `config`)
priority = 60000
retries = 60

ORDER = ['latency', 'error']

CHARTS = {
    'latency': {
        'options': [None, 'TCP connect latency', 'ms', 'latency', 'portcheck.latency', 'line'],
        'lines': [
            ['tcp_connect_time', 'connect', 'absolute', 100, 1000]
        ]},
    'error': {
        'options': [None, 'Portcheck error code', 'code', 'error', 'portcheck.error', 'line'],
        'lines': [
            ['tcp_error', 'error', 'absolute']
        ]}
}

SOCKET_FAILED = -1
CONNECTION_FAILED = -2
CONNECTION_TIMED_OUT = -3


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.chart_name = ""
        self._sock = None
        self.__socket_config = None
        self.host = self.configuration.get('host', None)
        self.port = self.configuration.get('port', None)
        self.update_every = self.configuration.get('update every', 3)
        self.timeout = self.configuration.get('timeout', 5)

    def check(self):
        """
        Parse configuration, check if configuration is available, and dynamically create chart lines data
        :return: boolean
        """
        if self.host is None or self.port is None:
            self.error("Host and/or port missing")
            return False
        else:
            self.debug("Enabled portcheck: {host}:{port}, update every {update}s, timeout: {timeout}s".format(
                host=self.host, port=self.port, update=self.update_every, timeout=self.timeout
            ))
            # We will accept any configuration, even if initial connection fails (a service might be down from
            # the beginning)
            return True

    def _get_data(self):
        """
        Get data from socket
        :return: dict
        """
        data = dict()

        for res in socket.getaddrinfo(self.host, self.port, socket.AF_UNSPEC, socket.SOCK_STREAM):
            # use first working socket
            if self._create_socket(res):
                self._connect2socket(data)
                self._disconnect()
                return data

        # We could not connect
        data['tcp_connect_time'] = 0
        data['tcp_error'] = SOCKET_FAILED
        return data

    def _create_socket(self, res=None):
        af, sock_type, proto, canon_name, sa = res
        try:
            self.debug('Creating socket to "{address}", port {port}'.format(address=sa[0], port=sa[1]))
            self._sock = socket.socket(af, sock_type, proto)
            self.__socket_config = res
            self._sock.settimeout(self.timeout)
            return True
        except socket.error as error:
            self.error('Failed to create socket "{address}", port {port}, error: {error}'.format(
                address=sa[0], port=sa[1], error=error
            ))
            return False

    def _connect2socket(self, data):
        """
        Connect to a socket, passing the result of getaddrinfo()
        :return: dict
        """
        data['tcp_connect_time'] = 0
        data['tcp_error'] = 0

        af, sock_type, proto, canon_name, sa = self.__socket_config
        try:
            self.debug('connecting socket to "{address}", port {port}'.format(address=sa[0], port=sa[1]))
            start = time.time()
            self._sock.connect(sa)
            diff = time.time() - start
            self.debug('connected to "{address}", port {port}, latency {latency}'.format(
                address=sa[0], port=sa[1], latency=diff
            ))
            # we will set it at least 0.1 ms. 0.0 would mean failed connection (handy for 3rd-party-APIs)
            data['tcp_connect_time'] = max(round(diff * 10000), 1)

        except socket.timeout as error:
            self.error('Socket timed out on "{address}", port {port}, error: {error}'.format(
                address=sa[0], port=sa[1], error=error
            ))
            data['tcp_error'] = CONNECTION_TIMED_OUT

        except socket.error as error:
            self.error('Failed to connect to "{address}", port {port}, error: {error}'.format(
                address=sa[0], port=sa[1], error=error
            ))
            data['tcp_error'] = CONNECTION_FAILED

    def _disconnect(self):
        """
        Close socket connection
        :return:
        """
        if self._sock is not None:
            try:
                self.debug('closing socket')
                self._sock.shutdown(2)  # 0 - read, 1 - write, 2 - all
                self._sock.close()
            except Exception:
                pass
