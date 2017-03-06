# -*- coding: utf-8 -*-
# Description: NSD `nsd-control stats_noreset` netdata python.d module
# Author: <383c57 at gmail.com>


from base import ExecutableService
import re

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
            ['num.queries', 'queries', 'incremental'],]},
    'zones': {
        'options': [
            None, "zones", 'zones', 'zones', 'nsd.zones', 'stacked'],
        'lines': [
            ['zone.master', 'master', 'absolute'],
            ['zone.slave', 'slave', 'absolute'],]},
    'protocol': {
        'options': [
            None, "protocol", 'queries/s', 'protocol', 'nsd.protocols', 'stacked'],
        'lines': [
            ['num.udp', 'udp', 'incremental'],
            ['num.udp6', 'udp6', 'incremental'],
            ['num.tcp', 'tcp', 'incremental'],
            ['num.tcp6', 'tcp6', 'incremental'],]},
    'type': {
        'options': [
            None, "query type", 'queries/s', 'query type', 'nsd.type', 'stacked'],
        'lines': [
            ['num.type.A', 'A', 'incremental'],
            ['num.type.NS', 'NS', 'incremental'],
            ['num.type.CNAME', 'CNAME', 'incremental'],
            ['num.type.SOA', 'SOA', 'incremental'],
            ['num.type.PTR', 'PTR', 'incremental'],
            ['num.type,HINFO', 'HINFO', 'incremental'],
            ['num.type.MX', 'MX', 'incremental'],
            ['num.type.NAPTR', 'NAPTR', 'incremental'],
            ['num.type.TXT', 'TXT', 'incremental'],
            ['num.type.AAAA', 'AAAA', 'incremental'],
            ['num.type.SRV', 'SRV', 'incremental'],
            ['num.type.TYPE255', 'ANY', 'incremental'],]},
    'transfer': {
        'options': [
            None, "transfer", 'queries/s', 'transfer', 'nsd.transfer', 'stacked'],
        'lines': [
            ['num.opcode.NOTIFY', 'NOTIFY', 'incremental'],
            ['num.type.TYPE252', 'AXFR', 'incremental'],]},
    'rcode': {
        'options': [
            None, "return code", 'queries/s', 'return code', 'nsd.rcode', 'stacked'],
        'lines': [
            ['num.rcode.NOERROR', 'NOERROR', 'incremental'],
            ['num.rcode.FORMERR', 'FORMERR', 'incremental'],
            ['num.rcode.SERVFAIL', 'SERVFAIL', 'incremental'],
            ['num.rcode.NXDOMAIN', 'NXDOMAIN', 'incremental'],
            ['num.rcode.NOTIMP', 'NOTIMP', 'incremental'],
            ['num.rcode.REFUSED', 'REFUSED', 'incremental'],
            ['num.rcode.YXDOMAIN', 'YXDOMAIN', 'incremental'],]}
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
        try:
            lines = self._get_raw_data()
            r = self.regex
            stats = {k: int(v) for k, v in r.findall(''.join(lines))}
            stats.setdefault('num.opcode.NOTIFY', 0)
            stats.setdefault('num.type.TYPE252', 0)
            stats.setdefault('num.type.TYPE255', 0)
            return stats
        except Exception:
            return None
