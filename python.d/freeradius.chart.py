# -*- coding: utf-8 -*-
# Description: freeradius netdata python.d module
# Author: l2isbad

from base import SimpleService
from re import findall
from subprocess import Popen, PIPE

# default module values (can be overridden per job in `config`)
priority = 60000
retries = 60
update_every = 15

# charts order (can be overridden if you want less charts, or different order)
ORDER = ['authentication', 'accounting', 'proxy-auth', 'proxy-acct']

CHARTS = {
    'authentication': {
        'options': [None, "Authentication", "packets/s", 'Authentication', 'freerad.auth', 'line'],
        'lines': [
            ['access-accepts', None, 'incremental'], ['access-rejects', None, 'incremental'],
            ['auth-dropped-requests', None, 'incremental'], ['auth-duplicate-requests', None, 'incremental'],
            ['auth-invalid-requests', None, 'incremental'], ['auth-malformed-requests', None, 'incremental'],
            ['auth-unknown-types', None, 'incremental']
        ]},
     'accounting': {
        'options': [None, "Accounting", "packets/s", 'Accounting', 'freerad.acct', 'line'],
        'lines': [
            ['accounting-requests', None, 'incremental'], ['accounting-responses', None, 'incremental'],
            ['acct-dropped-requests', None, 'incremental'], ['acct-duplicate-requests', None, 'incremental'],
            ['acct-invalid-requests', None, 'incremental'], ['acct-malformed-requests', None, 'incremental'],
            ['acct-unknown-types', None, 'incremental']
        ]},
    'proxy-auth': {
        'options': [None, "Proxy Authentication", "packets/s", 'Authentication', 'freerad.proxy.auth', 'line'],
        'lines': [
            ['proxy-access-accepts', None, 'incremental'], ['proxy-access-rejects', None, 'incremental'],
            ['proxy-auth-dropped-requests', None, 'incremental'], ['proxy-auth-duplicate-requests', None, 'incremental'],
            ['proxy-auth-invalid-requests', None, 'incremental'], ['proxy-auth-malformed-requests', None, 'incremental'],
            ['proxy-auth-unknown-types', None, 'incremental']
        ]},
     'proxy-acct': {
        'options': [None, "Proxy Accounting", "packets/s", 'Accounting', 'freerad.proxy.acct', 'line'],
        'lines': [
            ['proxy-accounting-requests', None, 'incremental'], ['proxy-accounting-responses', None, 'incremental'],
            ['proxy-acct-dropped-requests', None, 'incremental'], ['proxy-acct-duplicate-requests', None, 'incremental'],
            ['proxy-acct-invalid-requests', None, 'incremental'], ['proxy-acct-malformed-requests', None, 'incremental'],
            ['proxy-acct-unknown-types', None, 'incremental']
        ]}

}


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.host = self.configuration.get('host', 'localhost')
        self.port = self.configuration.get('port', '18121')
        self.secret = self.configuration.get('secret', 'adminsecret')
        self.acct = self.configuration.get('acct', False)
        self.proxy_auth = self.configuration.get('proxy_auth', False)
        self.proxy_acct = self.configuration.get('proxy_acct', False)
        self.echo = self.find_binary('echo')
        self.radclient = self.find_binary('radclient')
        self.sub_echo = [self.echo, 'Message-Authenticator = 0x00, FreeRADIUS-Statistics-Type = 15, Response-Packet-Type = Access-Accept']
        self.sub_radclient = [self.radclient, '-r', '1', '-t', '1', ':'.join([self.host, self.port]), 'status', self.secret]
    
    def check(self):
        if not all([self.echo, self.radclient]):
            self.error('Can\'t locate \'radclient\' binary or binary is not executable by netdata')
            return False
        if self._get_raw_data():
            chart_choice = [True, bool(self.acct), bool(self.proxy_auth), bool(self.proxy_acct)]
            self.order = [chart for chart, choice in zip(ORDER, chart_choice) if choice]
            self.definitions = dict([chart for chart in CHARTS.items() if chart[0] in self.order])
            self.info('Plugin was started succesfully')
            return True
        else:
            self.error('Request returned no data. Is server alive? Used options: host {0}, port {1}, secret {2}'.format(self.host, self.port, self.secret))
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
        'echo "Message-Authenticator = 0x00, FreeRADIUS-Statistics-Type = 15, Response-Packet-Type = Access-Accept" | radclient -t 1 -r 1 host:port status secret'
        :return: str
        """
        try:
            process_echo = Popen(self.sub_echo, stdout=PIPE, stderr=PIPE, shell=False)
            process_rad = Popen(self.sub_radclient, stdin=process_echo.stdout, stdout=PIPE, stderr=PIPE, shell=False)
            process_echo.stdout.close()
            raw_result = process_rad.communicate()[0]
        except Exception:
            return None
        else:
            if process_rad.returncode is 0:
                return raw_result.decode()
            else:
                return None
