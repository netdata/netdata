# -*- coding: utf-8 -*-
# Description: freeradius netdata python.d module
# Author: ilyam8
# SPDX-License-Identifier: GPL-3.0-or-later

import re
from subprocess import Popen, PIPE

from bases.FrameworkServices.SimpleService import SimpleService
from bases.collection import find_binary

update_every = 15

PARSER = re.compile(r'((?<=-)[AP][a-zA-Z-]+) = (\d+)')

RADIUS_MSG = 'Message-Authenticator = 0x00, FreeRADIUS-Statistics-Type = 15, Response-Packet-Type = Access-Accept'

RADCLIENT_RETRIES = 1
RADCLIENT_TIMEOUT = 1

DEFAULT_HOST = 'localhost'
DEFAULT_PORT = 18121
DEFAULT_DO_ACCT = False
DEFAULT_DO_PROXY_AUTH = False
DEFAULT_DO_PROXY_ACCT = False

ORDER = [
    'authentication',
    'accounting',
    'proxy-auth',
    'proxy-acct',
]

CHARTS = {
    'authentication': {
        'options': [None, 'Authentication', 'packets/s', 'authentication', 'freerad.auth', 'line'],
        'lines': [
            ['access-accepts', None, 'incremental'],
            ['access-rejects', None, 'incremental'],
            ['auth-dropped-requests', 'dropped-requests', 'incremental'],
            ['auth-duplicate-requests', 'duplicate-requests', 'incremental'],
            ['auth-invalid-requests', 'invalid-requests', 'incremental'],
            ['auth-malformed-requests', 'malformed-requests', 'incremental'],
            ['auth-unknown-types', 'unknown-types', 'incremental']
        ]
    },
    'accounting': {
        'options': [None, 'Accounting', 'packets/s', 'accounting', 'freerad.acct', 'line'],
        'lines': [
            ['accounting-requests', 'requests', 'incremental'],
            ['accounting-responses', 'responses', 'incremental'],
            ['acct-dropped-requests', 'dropped-requests', 'incremental'],
            ['acct-duplicate-requests', 'duplicate-requests', 'incremental'],
            ['acct-invalid-requests', 'invalid-requests', 'incremental'],
            ['acct-malformed-requests', 'malformed-requests', 'incremental'],
            ['acct-unknown-types', 'unknown-types', 'incremental']
        ]
    },
    'proxy-auth': {
        'options': [None, 'Proxy Authentication', 'packets/s', 'authentication', 'freerad.proxy.auth', 'line'],
        'lines': [
            ['proxy-access-accepts', 'access-accepts', 'incremental'],
            ['proxy-access-rejects', 'access-rejects', 'incremental'],
            ['proxy-auth-dropped-requests', 'dropped-requests', 'incremental'],
            ['proxy-auth-duplicate-requests', 'duplicate-requests', 'incremental'],
            ['proxy-auth-invalid-requests', 'invalid-requests', 'incremental'],
            ['proxy-auth-malformed-requests', 'malformed-requests', 'incremental'],
            ['proxy-auth-unknown-types', 'unknown-types', 'incremental']
        ]
    },
    'proxy-acct': {
        'options': [None, 'Proxy Accounting', 'packets/s', 'accounting', 'freerad.proxy.acct', 'line'],
        'lines': [
            ['proxy-accounting-requests', 'requests', 'incremental'],
            ['proxy-accounting-responses', 'responses', 'incremental'],
            ['proxy-acct-dropped-requests', 'dropped-requests', 'incremental'],
            ['proxy-acct-duplicate-requests', 'duplicate-requests', 'incremental'],
            ['proxy-acct-invalid-requests', 'invalid-requests', 'incremental'],
            ['proxy-acct-malformed-requests', 'malformed-requests', 'incremental'],
            ['proxy-acct-unknown-types', 'unknown-types', 'incremental']
        ]
    }
}


def radclient_status(radclient, retries, timeout, host, port, secret):
    # radclient -r 1 -t 1 -x 127.0.0.1:18121 status secret

    return '{radclient} -r {num_retries} -t {timeout} -x {host}:{port} status {secret}'.format(
        radclient=radclient,
        num_retries=retries,
        timeout=timeout,
        host=host,
        port=port,
        secret=secret,
    ).split()


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.host = self.configuration.get('host', DEFAULT_HOST)
        self.port = self.configuration.get('port', DEFAULT_PORT)
        self.secret = self.configuration.get('secret')
        self.do_acct = self.configuration.get('acct', DEFAULT_DO_ACCT)
        self.do_proxy_auth = self.configuration.get('proxy_auth', DEFAULT_DO_PROXY_AUTH)
        self.do_proxy_acct = self.configuration.get('proxy_acct', DEFAULT_DO_PROXY_ACCT)
        self.echo = find_binary('echo')
        self.radclient = find_binary('radclient')
        self.sub_echo = [self.echo, RADIUS_MSG]
        self.sub_radclient = radclient_status(
            self.radclient, RADCLIENT_RETRIES, RADCLIENT_TIMEOUT, self.host, self.port, self.secret,
        )

    def check(self):
        if not self.radclient:
            self.error("Can't locate 'radclient' binary or binary is not executable by netdata user")
            return False

        if not self.echo:
            self.error("Can't locate 'echo' binary or binary is not executable by netdata user")
            return None

        if not self.secret:
            self.error("'secret' isn't set")
            return None

        if not self.get_raw_data():
            self.error('Request returned no data. Is server alive?')
            return False

        if not self.do_acct:
            self.order.remove('accounting')

        if not self.do_proxy_auth:
            self.order.remove('proxy-auth')

        if not self.do_proxy_acct:
            self.order.remove('proxy-acct')

        return True

    def get_data(self):
        """
        Format data received from shell command
        :return: dict
        """
        result = self.get_raw_data()

        if not result:
            return None

        return dict(
            (key.lower(), value) for key, value in PARSER.findall(result)
        )

    def get_raw_data(self):
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
