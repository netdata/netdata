# -*- coding: utf-8 -*-
# Description: spigotmc netdata python.d module
# Author: Austin S. Hemmelgarn (Ferroin)
# SPDX-License-Identifier: GPL-3.0-or-later

import socket
import platform
import re

from bases.FrameworkServices.SimpleService import SimpleService

from third_party import mcrcon

# Update only every 5 seconds because collection takes in excess of
# 100ms sometimes, and most people won't care about second-by-second data.
update_every = 5

PRECISION = 100

COMMAND_TPS = 'tps'
COMMAND_LIST = 'list'
COMMAND_ONLINE = 'online'

ORDER = [
    'tps',
    'users',
]

CHARTS = {
    'tps': {
        'options': [None, 'Spigot Ticks Per Second', 'ticks', 'spigotmc', 'spigotmc.tps', 'line'],
        'lines': [
            ['tps1', '1 Minute Average', 'absolute', 1, PRECISION],
            ['tps5', '5 Minute Average', 'absolute', 1, PRECISION],
            ['tps15', '15 Minute Average', 'absolute', 1, PRECISION]
        ]
    },
    'users': {
        'options': [None, 'Minecraft Users', 'users', 'spigotmc', 'spigotmc.users', 'area'],
        'lines': [
            ['users', 'Users', 'absolute', 1, 1]
        ]
    }
}


_TPS_REGEX = re.compile(
    r'^.*: .*?'            # Message lead-in
    r'(\d{1,2}.\d+), .*?'  # 1-minute TPS value
    r'(\d{1,2}.\d+), .*?'  # 5-minute TPS value
    r'(\d{1,2}\.\d+).*$',  # 15-minute TPS value
    re.X
)
_LIST_REGEX = re.compile(
    # Examples:
    # There are 4 of a max 50 players online: player1, player2, player3, player4
    # §6There are §c4§6 out of maximum §c50§6 players online.
    # §6There are §c3§6/§c1§6 out of maximum §c50§6 players online.
    # §6当前有 §c4§6 个玩家在线,最大在线人数为 §c50§6 个玩家.
    # §c4§6 人のプレイヤーが接続中です。最大接続可能人数\:§c 50
    r'[^§](\d+)(?:.*?(?=/).*?[^§](\d+))?',  # Current user count.
    re.X
)


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.host = self.configuration.get('host', 'localhost')
        self.port = self.configuration.get('port', 25575)
        self.password = self.configuration.get('password', '')
        self.console = mcrcon.MCRcon()
        self.alive = True

    def check(self):
        if platform.system() != 'Linux':
            self.error('Only supported on Linux.')
            return False
        try:
            self.connect()
        except (mcrcon.MCRconException, socket.error) as err:
            self.error('Error connecting.')
            self.error(repr(err))
            return False

        return self._get_data()

    def connect(self):
        self.console.connect(self.host, self.port, self.password)

    def reconnect(self):
        self.error('try reconnect.')
        try:
            try:
                self.console.disconnect()
            except mcrcon.MCRconException:
                pass
            self.console.connect(self.host, self.port, self.password)
            self.alive = True
        except (mcrcon.MCRconException, socket.error) as err:
            self.error('Error connecting.')
            self.error(repr(err))
            return False
        return True

    def is_alive(self):
        if any(
            [
                not self.alive,
                self.console.socket.getsockopt(socket.IPPROTO_TCP, socket.TCP_INFO, 0) != 1
            ]
        ):
            return self.reconnect()
        return True

    def _get_data(self):
        if not self.is_alive():
            return None

        data = {}

        try:
            raw = self.console.command(COMMAND_TPS)
            match = _TPS_REGEX.match(raw)
            if match:
                data['tps1'] = int(float(match.group(1)) * PRECISION)
                data['tps5'] = int(float(match.group(2)) * PRECISION)
                data['tps15'] = int(float(match.group(3)) * PRECISION)
            else:
                self.error('Unable to process TPS values.')
                if not raw:
                    self.error("'{0}' command returned no value, make sure you set correct password".format(COMMAND_TPS))
        except mcrcon.MCRconException:
            self.error('Unable to fetch TPS values.')
        except socket.error:
            self.error('Connection is dead.')
            self.alive = False
            return None

        try:
            raw = self.console.command(COMMAND_LIST)
            match = _LIST_REGEX.search(raw)
            if not match:
                raw = self.console.command(COMMAND_ONLINE)
                match = _LIST_REGEX.search(raw)
            if match:
                users = int(match.group(1))
                hidden_users = match.group(2)
                if hidden_users:
                    hidden_users = int(hidden_users)
                else:
                    hidden_users = 0
                data['users'] = users + hidden_users
            else:
                if not raw:
                    self.error("'{0}' and '{1}' commands returned no value, make sure you set correct password".format(
                        COMMAND_LIST, COMMAND_ONLINE))
                self.error('Unable to process user counts.')
        except mcrcon.MCRconException:
            self.error('Unable to fetch user counts.')
        except socket.error:
            self.error('Connection is dead.')
            self.alive = False
            return None

        return data
