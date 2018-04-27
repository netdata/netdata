# -*- coding: utf-8 -*-
# Description: spigotmc netdata python.d module
# Author: Austin S. Hemmelgarn (Ferroin)

import socket

from bases.FrameworkServices.SimpleService import SimpleService

from third_party import mcrcon

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

    def check(self):
        '''Check plugin configuration validity.

           This really just makes sure we can connect and authenticate
           correctly.'''
        try:
            self.console.connect(self.host, self.port, self.password)
        except (mcrcon.MCRconException, socket.error):
            return False
        return True

    def _get_data(self):
        data = {}
        try:
            raw = self.console.command('tps')
            # The above command returns a string that looks like this:
            # '§6TPS from last 1m, 5m, 15m: §a19.99, §a19.99, §a19.99\n'
            # The values we care about are the three numbers after the :
            tmp = raw.split(':')[1].split(',')
            data['tps1'] = float(tmp[0].lstrip()[2:]) * PRECISION
            data['tps5'] = float(tmp[1].lstrip()[2:]) * PRECISION
            data['tps15'] = float(tmp[2].lstrip().rstrip()[2:]) * PRECISION
        except (mcrcon.MCRconException, socket.error):
            data['tps1'] = None
            data['tps5'] = None
            data['tps15'] = None
            self.error('Unable to fetch TPS values.')
        try:
            raw = self.console.command('list')
            # The above command returns a string that looks like this:
            # 'There are 0/20 players online:'
            # We care about the first number here.
            data['users'] = int(raw.split()[2].split('/')[0])
        except (mcrcon.MCRconException, socket.error):
            data['users'] = None
            self.error('Unable to fetch user counts.')
        return data
