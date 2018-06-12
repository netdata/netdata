# -*- coding: utf-8 -*-
# Description: logind netdata python.d module
# Author: Austin S. Hemmelgarn (Ferroin)
# SPDX-License-Identifier: GPL-3.0+

import os
import sys

from bases.FrameworkServices.ExecutableService import ExecutableService

PRIORITY = 40000

ORDER = ['sessions', 'users', 'seats']

CHARTS = {
    'sessions': {
        'options': [None, 'Logind Sessions', 'sessions', 'sessions', 'logind.sessions', 'stacked'],
        'lines': [
            ['sessions_graphical', 'Graphical', 'absolute', 1, 1],
            ['sessions_console', 'Console', 'absolute', 1, 1],
            ['sessions_remote', 'Remote', 'absolute', 1, 1]
        ]
    },
    'users': {
        'options': [None, 'Logind Users', 'users', 'users', 'logind.users', 'stacked'],
        'lines': [
            ['users_graphical', 'Graphical', 'absolute', 1, 1],
            ['users_console', 'Console', 'absolute', 1, 1],
            ['users_remote', 'Remote', 'absolute', 1, 1]
        ]
    },
    'seats': {
        'options': [None, 'Logind Seats', 'seats', 'seats', 'logind.seats', 'line'],
        'lines': [
            ['seats', 'Active Seats', 'absolute', 1, 1]
        ]
    }
}


class Service(ExecutableService):
    def __init__(self, configuration=None, name=None):
        ExecutableService.__init__(self, configuration=configuration, name=name)
        self.command = 'loginctl list-sessions --no-legend'
        self.order = ORDER
        self.definitions = CHARTS

    def _get_data(self):
        ret = {
            'sessions_graphical': 0,
            'sessions_console': 0,
            'sessions_remote': 0,
        }
        users = {
            'graphical': list(),
            'console': list(),
            'remote': list()
        }
        seats = list()
        data = self._get_raw_data()

        for item in data:
            fields = item.split()
            if len(fields) == 3:
                users['remote'].append(fields[2])
                ret['sessions_remote'] += 1
            elif len(fields) == 4:
                users['graphical'].append(fields[2])
                ret['sessions_graphical'] += 1
                seats.append(fields[3])
            elif len(fields) == 5:
                users['console'].append(fields[2])
                ret['sessions_console'] += 1
                seats.append(fields[3])

        ret['users_graphical'] = len(set(users['graphical']))
        ret['users_console'] = len(set(users['console']))
        ret['users_remote'] = len(set(users['remote']))
        ret['seats'] = len(set(seats))

        return ret
