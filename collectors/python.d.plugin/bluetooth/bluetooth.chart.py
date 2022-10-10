# -*- coding: utf-8 -*-
# Description: bluetooth netdata python.d module
# Author: Dim-P
# SPDX-License-Identifier: GPL-3.0-or-later

try:
    import dbus

    HAS_DBUS = True
except ImportError:
    HAS_DBUS = False

from bases.FrameworkServices.SimpleService import SimpleService

ORDER = [
    'status',
]

CHARTS = {
    'status': {
        'options': [None, 'Device Status', 'devices', 'Device Status', 'bluetooth.device_status', 'line'],
        'lines': [
            ['connected', 'connected', 'absolute', 1, 1],
            ['paired', 'paired', 'absolute', 1, 1],
            ['blocked', 'blocked', 'absolute', 1, 1],
        ]
    }
}


class Service(SimpleService):
    def __init__(self, configuration=None, name=None, bus=None, manager=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS        

    def check(self):
        if not HAS_DBUS:
            self.error("Could not find dbus-python")
            return False
        
        self.bus=dbus.SystemBus()
        self.manager=dbus.Interface(self.bus.get_object('org.bluez','/'),'org.freedesktop.DBus.ObjectManager')

        return True

    def get_data(self):
        connected=0
        paired=0
        blocked=0
        objs=self.manager.GetManagedObjects()
        for path,props in objs.items():
            if 'org.bluez.Device1' in props:
                if props['org.bluez.Device1']['Connected']:
                    connected += 1
                if props['org.bluez.Device1']['Paired']:
                    paired += 1
                if props['org.bluez.Device1']['Blocked']:
                    blocked += 1
                # print(props)
        return {'connected':connected, 
                'paired':   paired,
                'blocked':  blocked}

