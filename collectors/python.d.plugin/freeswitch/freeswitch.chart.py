# -*- coding: utf-8 -*-
# Description: nginx netdata python.d module
# Author: Koldo Aingeru Marcos (paulfantom)
# SPDX-License-Identifier: GPL-3.0-or-later


try:
    import ESL
    HAS_ESL = True
except ImportError:
    HAS_ESL = False

import json

from bases.FrameworkServices.SimpleService import SimpleService


DEFAULT_HOST = '127.0.0.1'
DEFAULT_PORT = 8021
DEFAULT_PASSWORD = 'ClueCon'

ORDER = [
    'calls',
    'registrations',
    'gateways',
    'profiles'
]


CHARTS = {
    'calls': {
        'options': [None, 'Active Calls', 'calls', 'calls', 'freeswitch.calls', 'line'],
        'lines': [
            ['calls','calls','absolute']
        ]
    },
    'registrations': {
        'options': [None, 'Registrations', 'registrations', 'registrations', 'freeswitch.registrations', 'line'],
        'lines': [
            ['registrations','registrations','absolute']
        ]
    },
    'gateways': {
        'options': [None, 'Gateways', 'gateways', 'gateways', 'freeswitch.gateways', 'line'],
        'lines': [
            ['UNREGED','UNREGED','absolute'],
            ['TRYING','TRYING','absolute'],
            ['REGISTER','REGISTER','absolute'],
            ['REGED','REGED','absolute'],
            ['UNREGISTER','UNREGISTER','absolute'],
            ['FAILED','FAILED','absolute'],
            ['FAIL_WAIT','FAIL_WAIT','absolute'],
            ['EXPIRED','EXPIRED','absolute'],
            ['NOREG','NOREG','absolute'],
            ['TIMEOUT','TIMEOUT','absolute'],
        ]
    },
    'profiles': {
        'options': [None, 'Active Profiles', 'profiles', 'profiles', 'freeswitch.profiles', 'line'],
        'lines': [
            ['RUNNING','RUNNING','absolute'],
            ['DOWN','DOWN','absolute'],
        ]
    }
}


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)

        self.order = ORDER
        self.definitions = CHARTS

        self.host = configuration.get('host', DEFAULT_HOST)
        self.port = configuration.get('port', DEFAULT_PORT)
        self.password = configuration.get('password', DEFAULT_PASSWORD)


    def check(self):
        if not HAS_ESL:
            self.error("'ESL' package is needed to use freeswitch module, you can install with with pip install python-ESL")
            return False

        try:
            self.fs = ESL.ESLconnection(self.host, self.port, self.password)
            if not self.fs.connected():
                raise ValueError('Not connected')
        except ValueError:
            self.error("Cannot connect to FreeSwitch with provided data, check that the service is running")
            return False
        return True

    def _get_data(self):
        if not self.fs.connected():
            self.fs = ESL.ESLconnection(self.host, self.port, self.password)

        try:
            data = dict()

            data['calls']=self.get_calls()
            data['registrations']=self.get_registrations()

            gateways = self.get_gateways()
            profiles = self.get_profiles()

            for gw_state in gateways:
                data[gw_state]=int(gateways[gw_state])

            for prof_state in profiles:
                data[prof_state]=int(profiles[prof_state])

            return data or None
        except (ValueError, AttributeError):
            return None
    

    def get_calls(self):
        calls = self.fs.api('show calls as json').getBody()
        return int(json.loads(calls)["row_count"])

    def get_registrations(self):
        registrations = self.fs.api('show registrations as json').getBody()
        return int(json.loads(registrations)["row_count"])

    def get_gateways(self):
        gateways_fs = self.fs.api('sofia status gateway').getBody().splitlines()

        gateways = {'UNREGED':0,
                    'TRYING':0,
                    'REGISTER':0,
                    'REGED':0,
                    'UNREGISTER':0,
                    'FAILED':0,
                    'FAIL_WAIT':0,
                    'EXPIRED':0,
                    'NOREG':0,
                    'TIMEOUT':0}

        for gateway in gateways_fs:
            for gw_data in gateway.split():
                try:
                    gateways[gw_data]+=1;
                except KeyError:
                    continue

        return gateways

    def get_profiles(self):
        profiles_fs = self.fs.api('sofia status').getBody().splitlines()

        profiles = {'RUNNING':0,
                    'DOWN':0}

        for profile in profiles_fs:
            for prof_data in profile.split():
                try:
                    profiles[prof_data]+=1;
                except KeyError:
                    continue

        return profiles