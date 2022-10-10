# -*- coding: utf-8 -*-
# Description: example netdata python.d module
# Author: Put your name here (your github login)
# SPDX-License-Identifier: GPL-3.0-or-later

try:
    import dbus

    HAS_DBUS = True
except ImportError:
    HAS_DBUS = False

from bases.FrameworkServices.SimpleService import SimpleService

ORDER = [
    'bluetooth',
]

CHARTS = {
    'bluetooth': {
        'options': [None, 'Connected Devices', 'number', 'connected', 'bluetooth.con', 'line'],
        'lines': [
            ['connected', 'Connected', 'absolute', 1, 1]
        ]
    }
}


class Service(SimpleService):
    def __init__(self, configuration=None, name=None, bus=None, manager=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.bus=dbus.SystemBus()
        self.manager=dbus.Interface(self.bus.get_object('org.bluez','/'),'org.freedesktop.DBus.ObjectManager')
    
    def get_connected(self):
        objs=self.manager.GetManagedObjects()
        return [str(props['org.bluez.Device1']['Address']) for path,props in objs.items() if 'org.bluez.Device1' in props and props['org.bluez.Device1']['Connected']]

    def check(self):
        if not HAS_DBUS:
            self.error("Could not find dbus library")
            return False

        return True

    def get_data(self):
        return {'connected': len(self.get_connected())}

