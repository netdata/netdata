# -*- coding: utf-8 -*-
# Description: ntp readlist python.d module
# Author: Sven Mäder (rda)

import os
from base import ExecutableService

NAME = os.path.basename(__file__).replace(".chart.py", "")

# default module values
update_every = 10
priority = 90000
retries = 60

ORDER = ['frequency',
        'offset',
        'rootdelay',
        'rootdisp',
        'sys_jitter',
        'clk_jitter',
        'clk_wander',
        'precision',
        'stratum',
        'tc',
        'mintc']

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

class Service(ExecutableService):
    def __init__(self, configuration=None, name=None):
        ExecutableService.__init__(self, configuration=configuration, name=name)
        self.command = "ntpq -c readlist"
        self.order = ORDER
        self.definitions = CHARTS

    def _get_data(self):
        """
        Format data received from shell command
        :return: dict
        """
        try:
            raw = self._get_raw_data()
            data = []
            for line in raw:
                variables = line.replace('\n','').split(',')
                for variable in variables:
                    if variable != '':
                        data.append(variable)
            stratum = float(data[8].split('=')[1])
            precision = float(data[9].split('=')[1])
            rootdelay = float(data[10].split('=')[1])
            rootdisp = float(data[11].split('=')[1])
            tc = float(data[18].split('=')[1])
            mintc = float(data[19].split('=')[1])
            offset = float(data[20].split('=')[1])
            frequency = float(data[21].split('=')[1])
            sys_jitter = float(data[22].split('=')[1])
            clk_jitter = float(data[23].split('=')[1])
            clk_wander = float(data[24].split('=')[1])

            return {'frequency': frequency * 1000,
                    'offset': offset * 1000000,
                    'rootdelay': rootdelay * 1000,
                    'rootdisp': rootdisp * 1000,
                    'sys_jitter': sys_jitter * 1000000,
                    'clk_jitter': clk_jitter * 1000,
                    'clk_wander': clk_wander * 1000,
                    'precision': precision,
                    'stratum': stratum,
                    'tc': tc,
                    'mintc': mintc}
        except (ValueError, AttributeError):
            return None

