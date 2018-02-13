# -*- coding: utf-8 -*-
# Description:
# Author: Pawel Krupa (paulfantom)

import socket

from bases.FrameworkServices.SimpleService import SimpleService


class SocketService(SimpleService):
    def __init__(self, configuration=None, name=None):
        self._sock = None
        self._keep_alive = False
        self.host = 'localhost'
        self.port = None
        self.unix_socket = None
        self.dgram_socket = False
        self.request = ''
        self.__socket_config = None
        self.__empty_request = "".encode()
        SimpleService.__init__(self, configuration=configuration, name=name)

    def _socket_error(self, message=None):
        if self.unix_socket is not None:
            self.error('unix socket "{socket}": {message}'.format(socket=self.unix_socket,
                                                                  message=message))
        else:
            if self.__socket_config is not None:
                af, sock_type, proto, canon_name, sa = self.__socket_config
                self.error('socket to "{address}" port {port}: {message}'.format(address=sa[0],
                                                                                 port=sa[1],
                                                                                 message=message))
            else:
                self.error('unknown socket: {0}'.format(message))

    def _connect2socket(self, res=None):
        """
        Connect to a socket, passing the result of getaddrinfo()
        :return: boolean
        """
        if res is None:
            res = self.__socket_config
            if res is None:
                self.error("Cannot create socket to 'None':")
                return False

        af, sock_type, proto, canon_name, sa = res
        try:
            self.debug('Creating socket to "{address}", port {port}'.format(address=sa[0], port=sa[1]))
            self._sock = socket.socket(af, sock_type, proto)
        except socket.error as error:
            self.error('Failed to create socket "{address}", port {port}, error: {error}'.format(address=sa[0],
                                                                                                 port=sa[1],
                                                                                                 error=error))
            self._sock = None
            self.__socket_config = None
            return False

        try:
            self.debug('connecting socket to "{address}", port {port}'.format(address=sa[0], port=sa[1]))
            self._sock.connect(sa)
        except socket.error as error:
            self.error('Failed to connect to "{address}", port {port}, error: {error}'.format(address=sa[0],
                                                                                              port=sa[1],
                                                                                              error=error))
            self._disconnect()
            self.__socket_config = None
            return False

        self.debug('connected to "{address}", port {port}'.format(address=sa[0], port=sa[1]))
        self.__socket_config = res
        return True

    def _connect2unixsocket(self):
        """
        Connect to a unix socket, given its filename
        :return: boolean
        """
        if self.unix_socket is None:
            self.error("cannot connect to unix socket 'None'")
            return False

        try:
            self.debug('attempting DGRAM unix socket "{0}"'.format(self.unix_socket))
            self._sock = socket.socket(socket.AF_UNIX, socket.SOCK_DGRAM)
            self._sock.connect(self.unix_socket)
            self.debug('connected DGRAM unix socket "{0}"'.format(self.unix_socket))
            return True
        except socket.error as error:
            self.debug('Failed to connect DGRAM unix socket "{socket}": {error}'.format(socket=self.unix_socket,
                                                                                        error=error))

        try:
            self.debug('attempting STREAM unix socket "{0}"'.format(self.unix_socket))
            self._sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
            self._sock.connect(self.unix_socket)
            self.debug('connected STREAM unix socket "{0}"'.format(self.unix_socket))
            return True
        except socket.error as error:
            self.debug('Failed to connect STREAM unix socket "{socket}": {error}'.format(socket=self.unix_socket,
                                                                                         error=error))
            self._sock = None
            return False

    def _connect(self):
        """
        Recreate socket and connect to it since sockets cannot be reused after closing
        Available configurations are IPv6, IPv4 or UNIX socket
        :return:
        """
        try:
            if self.unix_socket is not None:
                self._connect2unixsocket()

            else:
                if self.__socket_config is not None:
                    self._connect2socket()
                else:
                    if self.dgram_socket:
                        sock_type = socket.SOCK_DGRAM
                    else:
                        sock_type = socket.SOCK_STREAM
                    for res in socket.getaddrinfo(self.host, self.port, socket.AF_UNSPEC, sock_type):
                        if self._connect2socket(res):
                            break

        except Exception:
            self._sock = None
            self.__socket_config = None

        if self._sock is not None:
            self._sock.setblocking(0)
            self._sock.settimeout(5)
            self.debug('set socket timeout to: {0}'.format(self._sock.gettimeout()))

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
            self._sock = None

    def _send(self):
        """
        Send request.
        :return: boolean
        """
        # Send request if it is needed
        if self.request != self.__empty_request:
            try:
                self.debug('sending request: {0}'.format(self.request))
                self._sock.send(self.request)
            except Exception as error:
                self._socket_error('error sending request: {0}'.format(error))
                self._disconnect()
                return False
        return True

    def _receive(self):
        """
        Receive data from socket
        :return: str
        """
        data = ""
        while True:
            self.debug('receiving response')
            try:
                buf = self._sock.recv(4096)
            except Exception as error:
                self._socket_error('failed to receive response: {0}'.format(error))
                self._disconnect()
                break

            if buf is None or len(buf) == 0:  # handle server disconnect
                if data == "":
                    self._socket_error('unexpectedly disconnected')
                else:
                    self.debug('server closed the connection')
                self._disconnect()
                break

            self.debug('received data')
            data += buf.decode('utf-8', 'ignore')
            if self._check_raw_data(data):
                break

        self.debug('final response: {0}'.format(data))
        return data

    def _get_raw_data(self):
        """
        Get raw data with low-level "socket" module.
        :return: str
        """
        if self._sock is None:
            self._connect()
            if self._sock is None:
                return None

        # Send request if it is needed
        if not self._send():
            return None

        data = self._receive()

        if not self._keep_alive:
            self._disconnect()

        return data

    @staticmethod
    def _check_raw_data(data):
        """
        Check if all data has been gathered from socket
        :param data: str
        :return: boolean
        """
        return bool(data)

    def _parse_config(self):
        """
        Parse configuration data
        :return: boolean
        """
        try:
            self.unix_socket = str(self.configuration['socket'])
        except (KeyError, TypeError):
            self.debug('No unix socket specified. Trying TCP/IP socket.')
            self.unix_socket = None
            try:
                self.host = str(self.configuration['host'])
            except (KeyError, TypeError):
                self.debug('No host specified. Using: "{0}"'.format(self.host))
            try:
                self.port = int(self.configuration['port'])
            except (KeyError, TypeError):
                self.debug('No port specified. Using: "{0}"'.format(self.port))

        try:
            self.request = str(self.configuration['request'])
        except (KeyError, TypeError):
            self.debug('No request specified. Using: "{0}"'.format(self.request))

        self.request = self.request.encode()

    def check(self):
        self._parse_config()
        return SimpleService.check(self)
