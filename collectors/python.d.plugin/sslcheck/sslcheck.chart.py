# -*- coding: utf-8 -*-
# Description: simple ssl expiration check netdata python.d module
# Original Author: ccremer (github.com/ccremer)
# SPDX-License-Identifier: GPL-3.0-or-later

import datetime
import socket
import ssl

from bases.FrameworkServices.SimpleService import SimpleService


DAYS_UNTIL_EXPIRATION = 'daysuntilexpiration'

ORDER = ['daysuntilexpiration']

CHARTS = {
    'daysuntilexpiration': {
        'options': [None, 'days until certificate expiration', 'days', 'days until expiration', 'sslcheck.daysuntilexpiration', 'line'],
        'lines': [
            [DAYS_UNTIL_EXPIRATION, 'days until expiration', 'absolute', 10, 0]
        ]
    }
}


# Not deriving from SocketService, too much is different
class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.host = self.configuration.get('host')
        self.port = self.configuration.get('port')
        self.timeout = self.configuration.get('timeout', 3)

    def check(self):
        """
        Parse configuration, check if configuration is available, and dynamically create chart lines data
        :return: boolean
        """
        if self.host is None or self.port is None:
            self.error('Host or port missing')
            return False
        if not isinstance(self.port, int):
            self.error('"port" is not an integer. Specify a numerical value, not service name.')
            return False

        self.debug('Enabled sslcheck: {host}:{port}, update every {update}s, timeout: {timeout}s'.format(
            host=self.host, port=self.port, update=self.update_every, timeout=self.timeout
        ))
        # We will accept any (valid-ish) configuration, even if initial connection fails (a service
        # might be down from the beginning)
        return True

    def _get_data(self):
        """
        Get number of days until the certificate for the configured hostname expires
        :return: dict
        """
        data = dict()
        data[DAYS_UNTIL_EXPIRATION] = self.ssl_valid_time_remaining(self.host, self.port)

        return data

    def ssl_expiry_datetime(self, hostname, port):
        ssl_date_fmt = r'%b %d %H:%M:%S %Y %Z'

        context = ssl.create_default_context()
        conn = context.wrap_socket(
            socket.socket(socket.AF_INET),
            server_hostname=hostname,
        )

        conn.settimeout(self.timeout)

        conn.connect((hostname, port))
        ssl_info = conn.getpeercert()
        # parse the string from the certificate into a Python datetime object
        return datetime.datetime.strptime(ssl_info['notAfter'], ssl_date_fmt)

    def ssl_valid_time_remaining(self, hostname, port):
        """Get the number of days left in a cert's lifetime."""
        expires = self.ssl_expiry_datetime(hostname, port)
        return (expires - datetime.datetime.utcnow()).days
