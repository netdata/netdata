# -*- coding: utf-8 -*-
# Description: squid netdata python.d module
# Author: Pawel Krupa (paulfantom)

from base import SocketService
import select

# default module values (can be overridden per job in `config`)
# update_every = 2
priority = 60000
retries = 5

# charts order (can be overridden if you want less charts, or different order)
ORDER = ['clients_net', 'clients_requests', 'servers_net', 'servers_requests']

CHARTS = {
    'clients_net': {
        'options': [None, "Squid Client Bandwidth", "kilobits/s", "clients", "squid.clients.net", "area"],
        'lines': [
            ["client_http_kbytes_in", "in", "incremental", 8, 1],
            ["client_http_kbytes_out", "out", "incremental", -8, 1],
            ["client_http_hit_kbytes_out", "hits", "incremental", -8, 1]
        ]},
    'clients_requests': {
        'options': [None, "Squid Client Requests", "requests/s", "clients", "squid.clients.requests", 'line'],
        'lines': [
            ["client_http_requests", "requests", "incremental"],
            ["client_http_hits", "hits", "incremental"],
            ["client_http_errors", "errors", "incremental", -1, 1]
        ]},
    'servers_net': {
        'options': [None, "Squid Server Bandwidth", "kilobits/s", "servers", "squid.servers.net", "area"],
        'lines': [
            ["server_all_kbytes_in", "in", "incremental", 8, 1],
            ["server_all_kbytes_out", "out", "incremental", -8, 1]
        ]},
    'servers_requests': {
        'options': [None, "Squid Server Requests", "requests/s", "servers", "squid.servers.requests", 'line'],
        'lines': [
            ["server_all_requests", "requests", "incremental"],
            ["server_all_errors", "errors", "incremental", -1, 1]
        ]}
}


class Service(SocketService):
    def __init__(self, configuration=None, name=None):
        SocketService.__init__(self, configuration=configuration, name=name)
        self.request = ""
        self.host = "localhost"
        self.port = 3128
        self.order = ORDER
        self.definitions = CHARTS

    def _get_data(self):
        """
        Get data via http request
        :return: dict
        """
        data = {}
        try:
            raw = self._get_raw_data().split('\r\n')[-1]
            if raw.startswith('<'):
                self.error("invalid data received")
                return None
            for row in raw.split('\n'):
                if row.startswith(("client", "server.all")):
                    tmp = row.split("=")
                    data[tmp[0].replace('.', '_').strip(' ')] = int(tmp[1])
        except (ValueError, AttributeError, TypeError):
            self.error("invalid data received")
            return None

        if len(data) == 0:
            self.error("no data received")
            return None
        else:
            return data

    def _more_data_available(self):
        try:
            ready_to_read, _, in_error = select.select([self._sock, ], [], [], 0.05)
        except Exception as e:
            self.debug("select returned exception", str(e))
        if len(ready_to_read) > 0:
            return True
        else:
            return False

    def check(self):
        """
        Parse essential configuration, autodetect squid configuration (if needed), and check if data is available
        :return: boolean
        """
        self._parse_config()
        # format request
        req = self.request.decode()
        if not req.startswith("GET"):
            req = "GET " + req
        if not req.endswith(" HTTP/1.0\r\n\r\n"):
            req += " HTTP/1.0\r\n\r\n"
        self.request = req.encode()
        if self._get_data() is not None:
            return True
        else:
            return False
