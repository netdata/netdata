# -*- coding: utf-8 -*-
# Description: dns_query_time netdata python.d module
# Author: ilyam8
# SPDX-License-Identifier: GPL-3.0-or-later

from random import choice
from socket import getaddrinfo, gaierror
from threading import Thread

try:
    import dns.message
    import dns.query
    import dns.name

    DNS_PYTHON = True
except ImportError:
    DNS_PYTHON = False

try:
    from queue import Queue
except ImportError:
    from Queue import Queue

from bases.FrameworkServices.SimpleService import SimpleService

update_every = 5


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.order = list()
        self.definitions = dict()
        self.timeout = self.configuration.get('response_timeout', 4)
        self.aggregate = self.configuration.get('aggregate', True)
        self.domains = self.configuration.get('domains')
        self.server_list = self.configuration.get('dns_servers')

    def check(self):
        if not DNS_PYTHON:
            self.error("'python-dnspython' package is needed to use dns_query_time.chart.py")
            return False

        self.timeout = self.timeout if isinstance(self.timeout, int) else 4

        if not all([self.domains, self.server_list,
                    isinstance(self.server_list, str), isinstance(self.domains, str)]):
            self.error("server_list and domain_list can't be empty")
            return False
        else:
            self.domains, self.server_list = self.domains.split(), self.server_list.split()

        for ns in self.server_list:
            if not check_ns(ns):
                self.info('Bad NS: %s' % ns)
                self.server_list.remove(ns)
                if not self.server_list:
                    return False

        data = self._get_data(timeout=1)

        down_servers = [s for s in data if data[s] == -100]
        for down in down_servers:
            down = down[3:].replace('_', '.')
            self.info('Removed due to non response %s' % down)
            self.server_list.remove(down)
            if not self.server_list:
                return False

        self.order, self.definitions = create_charts(aggregate=self.aggregate, server_list=self.server_list)
        return True

    def _get_data(self, timeout=None):
        return dns_request(self.server_list, timeout or self.timeout, self.domains)


def dns_request(server_list, timeout, domains):
    threads = list()
    que = Queue()
    result = dict()

    def dns_req(ns, t, q):
        domain = dns.name.from_text(choice(domains))
        request = dns.message.make_query(domain, dns.rdatatype.A)

        try:
            resp = dns.query.udp(request, ns, timeout=t)
            if (resp.rcode() == dns.rcode.NOERROR and resp.answer):
                query_time = resp.time * 1000
            else:
                query_time = -100
        except dns.exception.Timeout:
            query_time = -100
        finally:
            q.put({'_'.join(['ns', ns.replace('.', '_')]): query_time})

    for server in server_list:
        th = Thread(target=dns_req, args=(server, timeout, que))
        th.start()
        threads.append(th)

    for th in threads:
        th.join()
        result.update(que.get())

    return result


def check_ns(ns):
    try:
        return getaddrinfo(ns, 'domain')[0][4][0]
    except gaierror:
        return False


def create_charts(aggregate, server_list):
    if aggregate:
        order = ['dns_group']
        definitions = {
            'dns_group': {
                'options': [None, 'DNS Response Time', 'ms', 'name servers', 'dns_query_time.response_time', 'line'],
                'lines': []
            }
        }
        for ns in server_list:
            dim = [
                '_'.join(['ns', ns.replace('.', '_')]),
                ns,
                'absolute',
            ]
            definitions['dns_group']['lines'].append(dim)

        return order, definitions
    else:
        order = [''.join(['dns_', ns.replace('.', '_')]) for ns in server_list]
        definitions = dict()

        for ns in server_list:
            definitions[''.join(['dns_', ns.replace('.', '_')])] = {
                'options': [None, 'DNS Response Time', 'ms', ns, 'dns_query_time.response_time', 'area'],
                'lines': [
                    [
                        '_'.join(['ns', ns.replace('.', '_')]),
                        ns,
                        'absolute',
                    ]
                ]
            }
        return order, definitions
