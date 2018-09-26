# -*- coding: utf-8 -*-
# Description: spigotmc netdata python.d module
# Author: Austin S. Hemmelgarn (Ferroin)
# SPDX-License-Identifier: GPL-3.0+

import socket
import platform

from bases.FrameworkServices.SimpleService import SimpleService

from third_party import mcrcon

# Update only every 5 seconds because collection takes in excess of
# 100ms sometimes, and mos tpeople won't care about second-by-second data.
update_every = 5

PRECISION = 100

ORDER = ['tps', 'users']

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
        return True

    def connect(self):
        self.console.connect(self.host, self.port, self.password)

    def reconnect(self):
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
        if (not self.alive) or \
           self.console.socket.getsockopt(socket.IPPROTO_TCP, socket.TCP_INFO, 0) != 1:
            return self.reconnect()
        return True

    def _get_data(self):
        if not self.is_alive():
            return None
        data = {}
        try:
            raw = self.console.command('tps')
            # The above command returns a string that looks like this:
            # '§6TPS from last 1m, 5m, 15m: §a19.99, §a19.99, §a19.99\n'
            # The values we care about are the three numbers after the :
            tmp = raw.split(':')[1].split(',')
            data['tps1'] = float(tmp[0].lstrip(u' §a*')) * PRECISION
            data['tps5'] = float(tmp[1].lstrip(u' §a*')) * PRECISION
            data['tps15'] = float(tmp[2].lstrip(u' §a*').rstrip()) * PRECISION
        except mcrcon.MCRconException:
            self.error('Unable to fetch TPS values.')
        except socket.error:
            self.error('Connection is dead.')
            self.alive = False
            return None
        except (TypeError, LookupError):
            self.error('Unable to process TPS values.')
        try:
            raw = self.console.command('list')
            # The above command returns a string that looks like this:
            # 'There are 0/20 players online:'
            # We care about the first number here.
            data['users'] = int(raw.split()[2].split('/')[0])
        except mcrcon.MCRconException:
            self.error('Unable to fetch user counts.')
        except socket.error:
            self.error('Connection is dead.')
            self.alive = False
            return None
        except (TypeError, LookupError):
            self.error('Unable to process user counts.')
        return data
