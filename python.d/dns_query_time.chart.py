try:
    from time import monotonic as time
except ImportError:
    from time import time
try:
    import dns.message, dns.query, dns.name
    DNS_PYTHON = True
except ImportError:
    DNS_PYTHON = False
try:
    from queue import Queue
except ImportError:
    from Queue import Queue
from random import choice
from threading import Thread
from socket import gethostbyname, gaierror
from base import SimpleService


# default module values (can be overridden per job in `config`)
update_every = 5
priority = 60000
retries = 60


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
            self.error('\'python-dnspython\' package is needed to use dns_query_time.chart.py')
            return False

        self.timeout = self.timeout if isinstance(self.timeout, int) else 4
        self.update_every = self.timeout + 1 if self.update_every <= self.timeout else self.update_every

        if not all([self.domains, self.server_list,
                    isinstance(self.server_list, str), isinstance(self.domains, str)]):
            self.error('server_list and domain_list can\'t be empty')
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

        down_servers = [s[2:] for s in data if data[s] == -100]
        if down_servers:
            self.info('Removed due to non response %s' % down_servers)
            self.server_list = [s for s in self.server_list if s not in down_servers]
        if self.server_list:
            self._data_from_check = data
            self.order, self.definitions = create_charts(aggregate=self.aggregate, server_list=self.server_list)
            self.info(str({'domains': len(self.domains), 'servers': self.server_list}))
            return True
        else:
            return False

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
            dns_start = time()
            dns.query.udp(request, ns, timeout=t)
            dns_end = time()
            query_time = round((dns_end - dns_start) * 1000)
            q.put({''.join(['ns', ns]): query_time})
        except dns.exception.Timeout:
            q.put({''.join(['ns', ns]): -100})

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
        return gethostbyname(ns)
    except gaierror:
        return False


def create_charts(aggregate, server_list):
    if aggregate:
        order = ['dns_group']
        definitions = {'dns_group': {'options': [None, "DNS Response Time", "ms", 'name servers',
                                                 'resp.time', 'line'], 'lines': []}}
        for ns in server_list:
            definitions['dns_group']['lines'].append([''.join(['ns', ns]), ns, 'absolute'])

        return order, definitions
    else:
        order = [''.join(['dns_', ns]) for ns in server_list]
        definitions = dict()
        for ns in server_list:
            definitions[''.join(['dns_', ns])] = {'options': [None, "DNS Response Time", "ms", ns,
                                                              'resp.time', 'area'],
                                                  'lines': [[''.join(['ns', ns]), ns, 'absolute']]}
        return order, definitions
