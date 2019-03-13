# -*- coding: utf-8 -*-
# Description: simple ssl expiration check netdata python.d module
# Original Author: Peter Thurner (github.com/p-thurner)
# SPDX-License-Identifier: GPL-3.0-or-later

import datetime
import socket
import ssl

from bases.FrameworkServices.SimpleService import SimpleService


update_every = 60


ORDER = [
    'time_until_expiration',
]

CHARTS = {
    'time_until_expiration': {
        'options': [
            None,
            'Time Until Certificate Expiration',
            'seconds',
            'certificate expiration time',
            'sslcheck.time_until_expiration',
            'line',
        ],
        'lines': [
            ['time'],
        ],
        'variables': [
            ['days_until_expiration_warning'],
            ['days_until_expiration_critical'],
        ],
    },
}


SSL_DATE_FMT = r'%b %d %H:%M:%S %Y %Z'

DEFAULT_PORT = 443
DEFAULT_CONN_TIMEOUT = 3
DEFAULT_DAYS_UNTIL_WARN_LIMIT = 14
DEFAULT_DAYS_UNTIL_CRIT_LIMIT = 7


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.host = configuration.get('host')
        self.port = configuration.get('port', DEFAULT_PORT)
        self.timeout = configuration.get('timeout', DEFAULT_CONN_TIMEOUT)
        self.days_warn = configuration.get('days_until_expiration_warning', DEFAULT_DAYS_UNTIL_WARN_LIMIT)
        self.days_crit = configuration.get('days_until_expiration_critical', DEFAULT_DAYS_UNTIL_CRIT_LIMIT)

    def check(self):
        if not self.host:
            self.error('host parameter is mandatory, but it is not set')
            return False

        self.debug('run check : host {host}:{port}, update every {update}s, timeout {timeout}s'.format(
            host=self.host, port=self.port, update=self.update_every, timeout=self.timeout))

        return bool(self.get_data())

    def get_data(self):
        conn = create_ssl_conn(self.host, self.timeout)

        try:
            conn.connect((self.host, self.port))
        except Exception as error:
            self.error("error on connection to {0}:{1} : {2}".format(self.host, self.port, error))
            return None

        peer_cert = conn.getpeercert()
        conn.close()

        if peer_cert is None:
            self.warning("no certificate was provided by {0}:{1}".format(self.host, self.port))
            return None
        elif not peer_cert:
            self.warning("certificate was provided by {0}:{1}, but not validated".format(self.host, self.port))
            return None

        return {
            'time': cert_expiration_seconds(peer_cert),
            'days_until_expiration_warning': self.days_warn,
            'days_until_expiration_critical': self.days_crit,
        }


def create_ssl_conn(hostname, timeout):
    context = ssl.create_default_context()
    conn = context.wrap_socket(
        socket.socket(socket.AF_INET),
        server_hostname=hostname,
    )
    conn.settimeout(timeout)

    return conn


def cert_expiration_seconds(cert):
    expiration_date = datetime.datetime.strptime(cert['notAfter'], SSL_DATE_FMT)
    current_date = datetime.datetime.utcnow()
    delta = expiration_date - current_date

    return ((delta.days * 86400 + delta.seconds) * 10 ** 6 + delta.microseconds) / 10 ** 6
