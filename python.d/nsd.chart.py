# -*- coding: utf-8 -*-
# Description: NSD `nsd-control stats_noreset` netdata python.d module
# Author: <383c57 at gmail.com>

import re

from bases.FrameworkServices.ExecutableService import ExecutableService

# default module values (can be overridden per job in `config`)
priority = 60000
retries = 5
update_every = 30

# charts order (can be overridden if you want less charts, or different order)
ORDER = ['queries', 'zones', 'protocol', 'type', 'transfer', 'rcode']

CHARTS = {
    'queries': {
        'options': [
            None, "queries", 'queries/s', 'queries', 'nsd.queries', 'line'],
        'lines': [
            ['num_queries', 'queries', 'incremental'],]},
    'zones': {
        'options': [
            None, "zones", 'zones', 'zones', 'nsd.zones', 'stacked'],
        'lines': [
            ['zone_master', 'master', 'absolute'],
            ['zone_slave', 'slave', 'absolute'],]},
    'protocol': {
        'options': [
            None, "protocol", 'queries/s', 'protocol', 'nsd.protocols', 'stacked'],
        'lines': [
            ['num_udp', 'udp', 'incremental'],
            ['num_udp6', 'udp6', 'incremental'],
            ['num_tcp', 'tcp', 'incremental'],
            ['num_tcp6', 'tcp6', 'incremental'],]},
    'type': {
        'options': [
            None, "query type", 'queries/s', 'query type', 'nsd.type', 'stacked'],
        'lines': [
            ['num_type_A', 'A', 'incremental'],
            ['num_type_NS', 'NS', 'incremental'],
            ['num_type_CNAME', 'CNAME', 'incremental'],
            ['num_type_SOA', 'SOA', 'incremental'],
            ['num_type_PTR', 'PTR', 'incremental'],
            ['num_type_HINFO', 'HINFO', 'incremental'],
            ['num_type_MX', 'MX', 'incremental'],
            ['num_type_NAPTR', 'NAPTR', 'incremental'],
            ['num_type_TXT', 'TXT', 'incremental'],
            ['num_type_AAAA', 'AAAA', 'incremental'],
            ['num_type_SRV', 'SRV', 'incremental'],
            ['num_type_TYPE255', 'ANY', 'incremental'],]},
    'transfer': {
        'options': [
            None, "transfer", 'queries/s', 'transfer', 'nsd.transfer', 'stacked'],
        'lines': [
            ['num_opcode_NOTIFY', 'NOTIFY', 'incremental'],
            ['num_type_TYPE252', 'AXFR', 'incremental'],]},
    'rcode': {
        'options': [
            None, "return code", 'queries/s', 'return code', 'nsd.rcode', 'stacked'],
        'lines': [
            ['num_rcode_NOERROR', 'NOERROR', 'incremental'],
            ['num_rcode_FORMERR', 'FORMERR', 'incremental'],
            ['num_rcode_SERVFAIL', 'SERVFAIL', 'incremental'],
            ['num_rcode_NXDOMAIN', 'NXDOMAIN', 'incremental'],
            ['num_rcode_NOTIMP', 'NOTIMP', 'incremental'],
            ['num_rcode_REFUSED', 'REFUSED', 'incremental'],
            ['num_rcode_YXDOMAIN', 'YXDOMAIN', 'incremental'],]}
}


class Service(ExecutableService):
    def __init__(self, configuration=None, name=None):
        ExecutableService.__init__(
            self, configuration=configuration, name=name)
        self.command = "nsd-control stats_noreset"
        self.order = ORDER
        self.definitions = CHARTS
        self.regex = re.compile(r'([A-Za-z0-9.]+)=(\d+)')

    def _get_data(self):
        lines = self._get_raw_data()
        if not lines:
            return None

        r = self.regex
        stats = dict((k.replace('.', '_'), int(v))
                     for k, v in r.findall(''.join(lines)))
        stats.setdefault('num_opcode_NOTIFY', 0)
        stats.setdefault('num_type_TYPE252', 0)
        stats.setdefault('num_type_TYPE255', 0)
        return stats
