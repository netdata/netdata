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
        'lines': []
    },
    'error': {
        'options': [None, 'Portcheck error code', 'code', 'error', 'portcheck.error', 'line'],
        'lines': [

        ]}
}

# The higher the error code, the "more severe" it is.
# We use steps of 5, which allows future fine-grained error codes, should we need it (fill the blanks where
# appropriate).
CONNECTION_FAILED = 5
CONNECTION_TIMED_OUT = 10
SOCKET_FAILED = 15

TCP_DIMENSION_CONNECT_PREFIX = 'tcp_connect_'
TCP_DIMENSION_ERROR_PREFIX = 'tcp_error_'


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.chart_name = ""
        self.host = self.configuration.get('host', None)
        self.ports = self.configuration.get('ports', None)
        self.timeout = self.configuration.get('timeout', 1)

    def check(self):
        """
        Parse configuration, check if configuration is available, and dynamically create chart lines data
        :return: boolean
        """
        if self.host is None or self.ports is None:
            self.error("Host and/or ports missing")
            return False
        if not isinstance(self.ports, list):
            self.error('"ports" is not defined as a list. Specify a PyYaml compatible list.')
            return False

        for port in self.ports:
            if not isinstance(port, int):
                self.error("{port} is not an integer. Disabling plugin.".format(port=port))
                return False
            self.debug("Enabled portcheck: {host}:{port}, update every {update}s, timeout: {timeout}s".format(
                host=self.host, port=port, update=self.update_every, timeout=self.timeout
            ))
            self.definitions['latency']['lines'].append(
                [TCP_DIMENSION_CONNECT_PREFIX + str(port), port, 'absolute', 100, 1000])
            self.definitions['error']['lines'].append(
                [TCP_DIMENSION_ERROR_PREFIX + str(port), port, 'absolute'])
        # We will accept any (valid-ish) configuration, even if initial connection fails (a service might be down from
        # the beginning)
        return True

    def _get_data(self):
        """
        Get data from socket
        :return: dict
        """
        data = dict()

        for port in self.ports:
            success = False
            try:
                for socket_config in socket.getaddrinfo(self.host, port, socket.AF_UNSPEC, socket.SOCK_STREAM):
                    # use first working socket
                    sock = self._create_socket(socket_config)
                    if sock is not None:
                        self._connect2socket(data, socket_config, sock)
                        self._disconnect(sock)
                        success = True
                        break
            except socket.gaierror:
                success = False
                pass

            # We could not connect
            if not success:
                data[TCP_DIMENSION_CONNECT_PREFIX + str(port)] = 0
                data[TCP_DIMENSION_ERROR_PREFIX + str(port)] = SOCKET_FAILED

        return data

    def _create_socket(self, socket_config):
        af, sock_type, proto, canon_name, sa = socket_config
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

        af, sock_type, proto, canon_name, sa = socket_config
        port = str(sa[1])
        data[TCP_DIMENSION_CONNECT_PREFIX + port] = 0
        data[TCP_DIMENSION_ERROR_PREFIX + port] = 0
        try:
            self.debug('Connecting socket to "{address}", port {port}'.format(address=sa[0], port=port))
            start = time.time()
            sock.connect(sa)
            diff = time.time() - start
            self.debug('Connected to "{address}", port {port}, latency {latency}'.format(
                address=sa[0], port=port, latency=diff
            ))
            # we will set it at least 0.1 ms. 0.0 would mean failed connection (handy for 3rd-party-APIs)
            data[TCP_DIMENSION_CONNECT_PREFIX + port] = max(round(diff * 10000), 1)

        except socket.timeout as error:
            self.debug('Socket timed out on "{address}", port {port}, error: {error}'.format(
                address=sa[0], port=port, error=error
            ))
            data[TCP_DIMENSION_ERROR_PREFIX + port] = CONNECTION_TIMED_OUT

        except socket.error as error:
            self.debug('Failed to connect to "{address}", port {port}, error: {error}'.format(
                address=sa[0], port=port, error=error
            ))
            data[TCP_DIMENSION_ERROR_PREFIX + port] = CONNECTION_FAILED

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
            except Exception:
                pass
