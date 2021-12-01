# Minecraft Remote Console module.
#
# Copyright (C) 2015 Barnaby Gale
#
# SPDX-License-Identifier: MIT

import socket
import select
import struct
import time


class MCRconException(Exception):
    pass


class MCRcon(object):
    socket = None

    def connect(self, host, port, password):
        if self.socket is not None:
            raise MCRconException("Already connected")
        self.socket = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        self.socket.settimeout(0.9)
        self.socket.connect((host, port))
        self.send(3, password)

    def disconnect(self):
        if self.socket is None:
            raise MCRconException("Already disconnected")
        self.socket.close()
        self.socket = None

    def read(self, length):
        data = b""
        while len(data) < length:
            data += self.socket.recv(length - len(data))
        return data

    def send(self, out_type, out_data):
        if self.socket is None:
            raise MCRconException("Must connect before sending data")

        # Send a request packet
        out_payload = struct.pack('<ii', 0, out_type) + out_data.encode('utf8') + b'\x00\x00'
        out_length = struct.pack('<i', len(out_payload))
        self.socket.send(out_length + out_payload)

        # Read response packets
        in_data = ""
        while True:
            # Read a packet
            in_length, = struct.unpack('<i', self.read(4))
            in_payload = self.read(in_length)
            in_id = struct.unpack('<ii', in_payload[:8])
            in_data_partial, in_padding = in_payload[8:-2], in_payload[-2:]

            # Sanity checks
            if in_padding != b'\x00\x00':
                raise MCRconException("Incorrect padding")
            if in_id == -1:
                raise MCRconException("Login failed")

            # Record the response
            in_data += in_data_partial.decode('utf8')

            # If there's nothing more to receive, return the response
            if len(select.select([self.socket], [], [], 0)[0]) == 0:
                return in_data

    def command(self, command):
        result = self.send(2, command)
        time.sleep(0.003) # MC-72390 workaround
        return result
