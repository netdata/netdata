# -*- coding: utf-8 -*-
# Description: freeradius netdata python.d module
# Author: l2isbad

from re import findall
from subprocess import Popen, PIPE

from bases.collection import find_binary
from bases.FrameworkServices.SimpleService import SimpleService

# default module values (can be overridden per job in `config`)
priority = 60000
retries = 60
update_every = 15

RADIUS_MSG = 'Message-Authenticator = 0x00, FreeRADIUS-Statistics-Type = 15, Response-Packet-Type = Access-Accept'

# charts order (can be overridden if you want less charts, or different order)
ORDER = ['authentication', 'accounting', 'proxy-auth', 'proxy-acct']

CHARTS = {
    'authentication': {
        'options': [None, "Authentication", "packets/s", 'Authentication', 'freerad.auth', 'line'],
        'lines': [
            ['access-accepts', None, 'incremental'],
            ['access-rejects', None, 'incremental'],
            ['auth-dropped-requests', 'dropped-requests', 'incremental'],
            ['auth-duplicate-requests', 'duplicate-requests', 'incremental'],
            ['auth-invalid-requests', 'invalid-requests', 'incremental'],
            ['auth-malformed-requests', 'malformed-requests', 'incremental'],
            ['auth-unknown-types', 'unknown-types', 'incremental']
            ]},
    'accounting': {
        'options': [None, "Accounting", "packets/s", 'Accounting', 'freerad.acct', 'line'],
        'lines': [
            ['accounting-requests', 'requests', 'incremental'],
            ['accounting-responses', 'responses', 'incremental'],
            ['acct-dropped-requests', 'dropped-requests', 'incremental'],
            ['acct-duplicate-requests', 'duplicate-requests', 'incremental'],
            ['acct-invalid-requests', 'invalid-requests', 'incremental'],
            ['acct-malformed-requests', 'malformed-requests', 'incremental'],
            ['acct-unknown-types', 'unknown-types', 'incremental']
        ]},
    'proxy-auth': {
        'options': [None, "Proxy Authentication", "packets/s", 'Authentication', 'freerad.proxy.auth', 'line'],
        'lines': [
            ['proxy-access-accepts', 'access-accepts', 'incremental'],
            ['proxy-access-rejects', 'access-rejects', 'incremental'],
            ['proxy-auth-dropped-requests', 'dropped-requests', 'incremental'],
            ['proxy-auth-duplicate-requests', 'duplicate-requests', 'incremental'],
            ['proxy-auth-invalid-requests', 'invalid-requests', 'incremental'],
            ['proxy-auth-malformed-requests', 'malformed-requests', 'incremental'],
            ['proxy-auth-unknown-types', 'unknown-types', 'incremental']
        ]},
    'proxy-acct': {
        'options': [None, "Proxy Accounting", "packets/s", 'Accounting', 'freerad.proxy.acct', 'line'],
        'lines': [
            ['proxy-accounting-requests', 'requests', 'incremental'],
            ['proxy-accounting-responses', 'responses', 'incremental'],
            ['proxy-acct-dropped-requests', 'dropped-requests', 'incremental'],
            ['proxy-acct-duplicate-requests', 'duplicate-requests', 'incremental'],
            ['proxy-acct-invalid-requests', 'invalid-requests', 'incremental'],
            ['proxy-acct-malformed-requests', 'malformed-requests', 'incremental'],
            ['proxy-acct-unknown-types', 'unknown-types', 'incremental']
        ]}

}


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.definitions = CHARTS
        self.host = self.configuration.get('host', 'localhost')
        self.port = self.configuration.get('port', '18121')
        self.secret = self.configuration.get('secret')
        self.acct = self.configuration.get('acct', False)
        self.proxy_auth = self.configuration.get('proxy_auth', False)
        self.proxy_acct = self.configuration.get('proxy_acct', False)
        chart_choice = [True, bool(self.acct), bool(self.proxy_auth), bool(self.proxy_acct)]
        self.order = [chart for chart, choice in zip(ORDER, chart_choice) if choice]
        self.echo = find_binary('echo')
        self.radclient = find_binary('radclient')
        self.sub_echo = [self.echo, RADIUS_MSG]
        self.sub_radclient = [self.radclient, '-r', '1', '-t', '1',
                              ':'.join([self.host, self.port]), 'status', self.secret]

    def check(self):
        if not all([self.echo, self.radclient]):
            self.error('Can\'t locate "radclient" binary or binary is not executable by netdata')
            return False
        if not self.secret:
            self.error('"secret" not set')

        if self._get_raw_data():
            return True
        self.error('Request returned no data. Is server alive?')
        return False

    def _get_data(self):
        """
        Format data received from shell command
        :return: dict
        """
        result = self._get_raw_data()
        return dict([(elem[0].lower(), int(elem[1])) for elem in findall(r'((?<=-)[AP][a-zA-Z-]+) = (\d+)', result)])
        
    def _get_raw_data(self):
        """
        The following code is equivalent to
        'echo "Message-Authenticator = 0x00, FreeRADIUS-Statistics-Type = 15, Response-Packet-Type = Access-Accept"
         | radclient -t 1 -r 1 host:port status secret'
        :return: str
        """
        try:
            process_echo = Popen(self.sub_echo, stdout=PIPE, stderr=PIPE, shell=False)
            process_rad = Popen(self.sub_radclient, stdin=process_echo.stdout, stdout=PIPE, stderr=PIPE, shell=False)
            process_echo.stdout.close()
            raw_result = process_rad.communicate()[0]
        except OSError:
            return None
        if process_rad.returncode is 0:
            return raw_result.decode()
        return None
