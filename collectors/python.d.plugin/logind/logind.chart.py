# -*- coding: utf-8 -*-
# Description: logind netdata python.d module
# Author: Austin S. Hemmelgarn (Ferroin)
# SPDX-License-Identifier: GPL-3.0-or-later

from bases.FrameworkServices.SimpleService import SimpleService

try:
    import dbus
    HAVE_DBUS = True
except ImportError:
    HAVE_DBUS = False

priority = 59999
disabled_by_default = True

BUS_NAME = 'org.freedesktop.login1'
OBJ_PATH = '/org/freedesktop/login1'
MGR_IFACE = 'org.freedesktop.login1.Manager'
PROP_IFACE = 'org.freedesktop.DBus.Properties'
SESSION_IFACE = 'org.freedesktop.login1.Session'
USER_IFACE = 'org.freedesktop.login1.User'

GUI_SESSION_TYPES = [
    'x11',
    'mir',
    'wayland'
]

ORDER = [
    'session_types',
    'session_states',
    'users',
]

CHARTS = {
    'session_types': {
        'options': [None, 'Logind Session By Type', 'session_types', 'session_types', 'logind.session_types', 'stacked'],
        'lines': [
            ['sessions_graphical', 'Graphical', 'absolute', 1, 1],
            ['sessions_console', 'Console', 'absolute', 1, 1],
            ['sessions_remote', 'Remote', 'absolute', 1, 1],
            ['sessions_other', 'Other', 'absolute', 1, 1]
        ]
    },
    'session_states': {
        'options': [None, 'Logind Sessions By State', 'session_states', 'session_states', 'logind.session_states', 'stacked'],
        'lines': [
            ['sessions_online', 'Online', 'absolute', 1, 1],
            ['sessions_active', 'Active', 'absolute', 1, 1],
            ['sessions_closing', 'Closing', 'absolute', 1, 1]
        ]
    },
    'users': {
        'options': [None, 'Logind Users', 'users', 'users', 'logind.users', 'stacked'],
        'lines': [
            ['users_offline', 'Offline', 'absolute', 1, 1],
            ['users_lingering', 'Lingering', 'absolute', 1, 1],
            ['users_online', 'Online', 'absolute', 1, 1],
            ['users_active', 'Active', 'absolute', 1, 1],
            ['users_closing', 'Closing', 'absolute', 1, 1]
        ]
    },
}


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.bus_name = configuration.get('bus_name', BUS_NAME)
        self.obj_path = configuration.get('object_path', OBJ_PATH)
        self.bus = dbus.SystemBus()
        self.mgr = None
        self.iface = None

        try:
            self.mgr = self.bus.get_object(self.bus_name, self.obj_path)
            self.iface = dbus.Interface(self.mgr, MGR_IFACE)
        except dbus.DBusException:
            pass

    def check(self):
        if not HAVE_DBUS:
            self.error('dbus-python is not available')
            return False

        if self.mgr is None:
            self.error('Unable to connect to ' + self.bus_name + ' service')
            return False

        if self.iface is None:
            self.error('Failed to construct interface')

        return self._get_data()

    def _get_data(self):
        try:
            sessions = self.iface.ListSessions()
            users = self.iface.ListUsers()
        except dbus.DBusException:
            return None

        ret = {
            'sessions_graphical': 0,
            'sessions_console': 0,
            'sessions_remote': 0,
            'sessions_other': 0,
            'sessions_online': 0,
            'sessions_active': 0,
            'sessions_closing': 0,
            'users_offline': 0,
            'users_lingering': 0,
            'users_online': 0,
            'users_active': 0,
            'users_closing': 0,
        }

        session_paths = {s[4] for s in sessions}
        user_paths = {u[2] for u in users}

        for p in session_paths:
            o = self.bus.get_object(self.bus_name, p)
            i = dbus.Interface(o, PROP_IFACE)
            props = i.GetAll(SESSION_IFACE)

            if bool(props['Remote']):
                ret['sessions_remote'] += 1
            elif str(props['Type']) in GUI_SESSION_TYPES:
                ret['sessions_graphical'] += 1
            elif str(props['Type']) == 'tty':
                ret['sessions_console'] += 1
            elif str(props['Type']) == 'unspecified':
                ret['sessions_other'] += 1
            else:
                self.error('Unknown session type ' + str(props['Type']) + ' for session ' + str(p))
                ret['sessions_other'] += 1

            if str(props['State']) == 'online':
                ret['sessions_online'] += 1
            elif str(props['State']) == 'active':
                ret['sessions_active'] += 1
            elif str(props['State']) == 'closing':
                ret['sessions_closing'] += 1
            else:
                self.error('Unknown session state ' + str(props['State']) + ' for session ' + str(p))

        for p in user_paths:
            o = self.bus.get_object(self.bus_name, p)
            i = dbus.Interface(o, PROP_IFACE)
            state = i.Get(USER_IFACE, 'State')

            if str(state) == 'offline':
                ret['users_offline'] += 1
            elif str(state) == 'lingering':
                ret['users_lingering'] += 1
            elif str(state) == 'online':
                ret['users_online'] += 1
            elif str(state) == 'active':
                ret['users_active'] += 1
            elif str(state) == 'closing':
                ret['users_closing'] += 1
            else:
                self.error('Unknown user state ' + str(state) + ' for user ' + str(p))

        return ret
