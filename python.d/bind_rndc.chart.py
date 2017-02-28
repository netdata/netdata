# -*- coding: utf-8 -*-
# Description: bind rndc netdata python.d module
# Author: l2isbad

from base import SimpleService
from re import compile, findall
from os.path import getsize, split
from os import access as is_accessible, R_OK
from subprocess import Popen

priority = 60000
retries = 60
update_every = 30

NMS = ['requests', 'responses', 'success', 'auth_answer', 'nonauth_answer', 'nxrrset', 'failure',
       'nxdomain', 'recursion', 'duplicate', 'rejections']
QUERIES = ['RESERVED0', 'A', 'NS', 'CNAME', 'SOA', 'PTR', 'MX', 'TXT', 'X25', 'AAAA', 'SRV', 'NAPTR',
           'A6', 'DS', 'RRSIG', 'DNSKEY', 'SPF', 'ANY', 'DLV']


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.named_stats_path = self.configuration.get('named_stats_path', '/var/log/bind/named.stats')
        self.regex_values = compile(r'([0-9]+) ([^\n]+)')
        # self.options = ['Incoming Requests', 'Incoming Queries', 'Outgoing Queries',
        # 'Name Server Statistics', 'Zone Maintenance Statistics', 'Resolver Statistics',
        # 'Cache DB RRsets', 'Socket I/O Statistics']
        self.options = ['Name Server Statistics', 'Incoming Queries', 'Outgoing Queries']
        self.regex_options = [r'(%s(?= \+\+)) \+\+([^\+]+)' % option for option in self.options]
        self.rndc = self.find_binary('rndc')

    def check(self):
        # We cant start without 'rndc' command
        if not self.rndc:
            self.error('Can\'t locate \'rndc\' binary or binary is not executable by netdata')
            return False

        # We cant start if stats file is not exist or not readable by netdata user
        if not is_accessible(self.named_stats_path, R_OK):
            self.error('Cannot access file %s' % self.named_stats_path)
            return False

        size_before = getsize(self.named_stats_path)
        run_rndc = Popen([self.rndc, 'stats'], shell=False)
        run_rndc.wait()
        size_after = getsize(self.named_stats_path)

        # We cant start if netdata user has no permissions to run 'rndc stats'
        if not run_rndc.returncode:
            # 'rndc' was found, stats file is exist and readable and we can run 'rndc stats'. Lets go!
            self.create_charts()

            # BIND APPEND dump on every run 'rndc stats'
            # that is why stats file size can be VERY large if update_interval too small
            dump_size_24hr = round(86400 / self.update_every * (int(size_after) - int(size_before)) / 1048576, 3)

            # If update_every too small we should WARN user
            if self.update_every < 30:
                self.info('Update_every %s is NOT recommended for use. Increase the value to > 30' % self.update_every)

            self.info('With current update_interval it will be + %s MB every 24hr. '
                      'Don\'t forget to create logrotate conf file for %s' % (dump_size_24hr, self.named_stats_path))

            self.info('Plugin was started successfully.')

            return True
        else:
            self.error('Not enough permissions to run "%s stats"' % self.rndc)
            return False

    def _get_raw_data(self):
        """
        Run 'rndc stats' and read last dump from named.stats
        :return: tuple(
                       file.read() obj,
                       named.stats file size
                      )
        """
        try:
            current_size = getsize(self.named_stats_path)
        except OSError:
            return None, None

        run_rndc = Popen([self.rndc, 'stats'], shell=False)
        run_rndc.wait()

        if run_rndc.returncode:     
            return None, None

        try:
            with open(self.named_stats_path) as bind_rndc:
                bind_rndc.seek(current_size)
                result = bind_rndc.read()
        except OSError:
            return None, None
        else:
            return result, current_size

    def _get_data(self):
        """
        Parse data from _get_raw_data()
        :return: dict
        """

        raw_data, size = self._get_raw_data()

        if raw_data is None:
            return None

        rndc_stats = dict()

        # Result: dict.
        # topic = Cache DB RRsets; body = A 178303 NS 86790 ... ; desc = A; value = 178303
        # {'Cache DB RRsets': [('A', 178303), ('NS', 286790), ...],
        # {Incoming Queries': [('RESERVED0', 8), ('A', 4557317680), ...],
        # ......
        for regex in self.regex_options:
            rndc_stats.update(dict([(topic, [(desc, int(value)) for value, desc in self.regex_values.findall(body)])
                               for topic, body in findall(regex, raw_data)]))

        nms = dict(rndc_stats.get('Name Server Statistics', []))

        inc_queries = dict([('i' + k, 0) for k in QUERIES])
        inc_queries.update(dict([('i' + k, v) for k, v in rndc_stats.get('Incoming Queries', [])]))
        out_queries = dict([('o' + k, 0) for k in QUERIES])
        out_queries.update(dict([('o' + k, v) for k, v in rndc_stats.get('Outgoing Queries', [])]))

        to_netdata = dict()
        to_netdata['requests'] = sum([v for k, v in nms.items() if 'request' in k and 'received' in k])
        to_netdata['responses'] = sum([v for k, v in nms.items() if 'responses' in k and 'sent' in k])
        to_netdata['success'] = nms.get('queries resulted in successful answer', 0)
        to_netdata['auth_answer'] = nms.get('queries resulted in authoritative answer', 0)
        to_netdata['nonauth_answer'] = nms.get('queries resulted in non authoritative answer', 0)
        to_netdata['nxrrset'] = nms.get('queries resulted in nxrrset', 0)
        to_netdata['failure'] = sum([nms.get('queries resulted in SERVFAIL', 0), nms.get('other query failures', 0)])
        to_netdata['nxdomain'] = nms.get('queries resulted in NXDOMAIN', 0)
        to_netdata['recursion'] = nms.get('queries caused recursion', 0)
        to_netdata['duplicate'] = nms.get('duplicate queries received', 0)
        to_netdata['rejections'] = nms.get('recursive queries rejected', 0)
        to_netdata['stats_size'] = size

        to_netdata.update(inc_queries)
        to_netdata.update(out_queries)
        return to_netdata

    def create_charts(self):
        self.order = ['stats_size', 'bind_stats', 'incoming_q', 'outgoing_q']
        self.definitions = {
            'bind_stats': {
                'options': [None, 'Name Server Statistics', 'stats', 'name server statistics', 'bind_rndc.stats', 'line'],
                'lines': [
                         ]},
            'incoming_q': {
                'options': [None, 'Incoming queries', 'queries','incoming queries', 'bind_rndc.incq', 'line'],
                'lines': [
                        ]},
            'outgoing_q': {
                'options': [None, 'Outgoing queries', 'queries','outgoing queries', 'bind_rndc.outq', 'line'],
                'lines': [
                        ]},
            'stats_size': {
                'options': [None, '%s file size' % split(self.named_stats_path)[1].capitalize(), 'megabytes',
                            '%s size' % split(self.named_stats_path)[1], 'bind_rndc.size', 'line'],
                'lines': [
                         ["stats_size", None, "absolute", 1, 1048576]
                        ]}
                     }
        for elem in QUERIES:
            self.definitions['incoming_q']['lines'].append(['i' + elem, elem, 'incremental'])
            self.definitions['outgoing_q']['lines'].append(['o' + elem, elem, 'incremental'])

        for elem in NMS:
            self.definitions['bind_stats']['lines'].append([elem, None, 'incremental'])
