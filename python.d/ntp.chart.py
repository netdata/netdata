# -*- coding: utf-8 -*-
# Description: ntp python.d module
# Author: Sven Mäder (rda)

from base import SimpleService
import re
import ntpq

# default module values
update_every = 10
priority = 90000
retries = 1

ORDER = [
    'frequency',
    'offset',
    'rootdelay',
    'rootdisp',
    'sys_jitter',
    'clk_jitter',
    'clk_wander',
    'precision',
    'stratum',
    'tc',
    'mintc'
]

CHARTS = {
    'frequency': {
        'options': [None, "frequency offset relative to hardware clock", "ppm", 'system', 'ntp.frequency', 'area'],
        'lines': [
            ['frequency', 'frequency', 'absolute', 1, 1000]
        ]},
    'offset': {
        'options': [None, "combined offset of server relative to this host", "ms", 'system', 'ntp.offset', 'area'],
        'lines': [
            ['offset', 'offset', 'absolute', 1, 1000000]
        ]},
    'rootdelay': {
        'options': [None, "total roundtrip delay to the primary reference clock", "ms", 'system', 'ntp.rootdelay', 'area'],
        'lines': [
            ['rootdelay', 'rootdelay', 'absolute', 1, 1000]
        ]},
    'rootdisp': {
        'options': [None, "total dispersion to the primary reference clock", "ms", 'system', 'ntp.rootdisp', 'area'],
        'lines': [
            ['rootdisp', 'rootdisp', 'absolute', 1, 1000]
        ]},
    'sys_jitter': {
        'options': [None, "combined system jitter", "ms", 'system', 'ntp.sys_jitter', 'area'],
        'lines': [
            ['sys_jitter', 'sys_jitter', 'absolute', 1, 1000000]
        ]},
    'clk_jitter': {
        'options': [None, "clock jitter", "ms", 'system', 'ntp.clk_jitter', 'area'],
        'lines': [
            ['clk_jitter', 'clk_jitter', 'absolute', 1, 1000]
        ]},
    'clk_wander': {
        'options': [None, "clock frequency wander", "ppm", 'system', 'ntp.clk_wander', 'area'],
        'lines': [
            ['clk_wander', 'clk_wander', 'absolute', 1, 1000]
        ]},
    'precision': {
        'options': [None, "precision", "log2 s", 'system', 'ntp.precision', 'line'],
        'lines': [
            ['precision', 'precision', 'absolute', 1, 1]
        ]},
    'stratum': {
        'options': [None, "stratum (1-15)", "1", 'system', 'ntp.stratum', 'line'],
        'lines': [
            ['stratum', 'stratum', 'absolute', 1, 1]
        ]},
    'tc': {
        'options': [None, "time constant and poll exponent (3-17)", "log2 s", 'system', 'ntp.tc', 'line'],
        'lines': [
            ['tc', 'tc', 'absolute', 1, 1]
        ]},
    'mintc': {
        'options': [None, "minimum time constant (3-10)", "log2 s", 'system', 'ntp.mintc', 'line'],
        'lines': [
            ['mintc', 'mintc', 'absolute', 1, 1]
        ]}
}

class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.sock, self.sockaddr = ntpq.get_socket('127.0.0.1')
        self.rgx_sys = re.compile(
                r'.*stratum=(?P<stratum>[0-9.-]+)'
                r'.*precision=(?P<precision>[0-9.-]+)'
                r'.*rootdelay=(?P<rootdelay>[0-9.-]+)'
                r'.*rootdisp=(?P<rootdisp>[0-9.-]+)'
                r'.*tc=(?P<tc>[0-9.-]+)'
                r'.*mintc=(?P<mintc>[0-9.-]+)'
                r'.*offset=(?P<offset>[0-9.-]+)'
                r'.*frequency=(?P<frequency>[0-9.-]+)'
                r'.*sys_jitter=(?P<sys_jitter>[0-9.-]+)'
                r'.*clk_jitter=(?P<clk_jitter>[0-9.-]+)'
                r'.*clk_wander=(?P<clk_wander>[0-9.-]+)')

    def check(self):
        self.create_charts()

        self.info('Plugin was started successfully')
        return True

    def _get_raw_data(self):
        try:
            data = ntpq.ntp_control_request(self.sock, self.sockaddr, op="readvar")
        except ntpq.NTPException:
            self.error('NTPException')
            return None

        if not data:
            self.error(''.join(['No data, message was: ', errs]))
            return None

        return data

    def get_variables(self):
        raw = self._get_raw_data()
        data = ' '.join(raw.split('\r\n'))

        if not raw:
            self.error(''.join(['No data for system, assoc_id: 0']))
            return None

        match = self.rgx_sys.search(data)

        if match != None:
            data = match.groupdict()
        else:
            return None

        return data

    def _get_data(self):
        to_netdata = {}

        sys_vars = self.get_variables()
        if (sys_vars != None):
            to_netdata['frequency'] = float(sys_vars['frequency']) * 1000
            to_netdata['offset'] = float(sys_vars['offset']) * 1000000
            to_netdata['rootdelay'] = float(sys_vars['rootdelay']) * 1000
            to_netdata['rootdisp'] = float(sys_vars['rootdisp']) * 1000
            to_netdata['sys_jitter'] = float(sys_vars['sys_jitter']) * 1000000
            to_netdata['clk_jitter'] = float(sys_vars['clk_jitter']) * 1000
            to_netdata['clk_wander'] = float(sys_vars['clk_wander']) * 1000
            to_netdata['precision'] = float(sys_vars['precision'])
            to_netdata['stratum'] = float(sys_vars['stratum'])
            to_netdata['tc'] = float(sys_vars['tc'])
            to_netdata['mintc'] = float(sys_vars['mintc'])

        return to_netdata

    def create_charts(self):
        self.order = ORDER[:]

        # Create static charts
        self.definitions = CHARTS
