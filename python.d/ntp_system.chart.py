# -*- coding: utf-8 -*-
# Description: ntp readlist python.d module
# Author: Sven Mäder (rda)

#import os
from base import ExecutableService

#NAME = os.path.basename(__file__).replace(".chart.py", "")

# default module values
update_every = 10
priority = 90000
retries = 60

ORDER = ['ntp.system_frequency',
        'ntp.system_offset',
        'ntp.system_rootdelay',
        'ntp.system_rootdisp',
        'ntp.system_sys_jitter',
        'ntp.system_clk_jitter',
        'ntp.system_clk_wander',
        'ntp.system_precision',
        'ntp.system_stratum',
        'ntp.system_tc',
        'ntp.system_mintc']

CHARTS = {
    'ntp.system_frequency': {
        'options': [None, "frequency offset relative to hardware clock", "ppm", 'system', 'ntp.frequency', 'area'],
        'lines': [
            ['system_frequency', 'frequency', 'absolute', 1, 1000]
        ]},
    'ntp.system_offset': {
        'options': [None, "combined offset of server relative to this host", "ms", 'system', 'ntp.offset', 'area'],
        'lines': [
            ['system_offset', 'offset', 'absolute', 1, 1000000]
        ]},
    'ntp.system_rootdelay': {
        'options': [None, "total roundtrip delay to the primary reference clock", "ms", 'system', 'ntp.rootdelay', 'area'],
        'lines': [
            ['system_rootdelay', 'rootdelay', 'absolute', 1, 1000]
        ]},
    'ntp.system_rootdisp': {
        'options': [None, "total dispersion to the primary reference clock", "ms", 'system', 'ntp.rootdisp', 'area'],
        'lines': [
            ['system_rootdisp', 'rootdisp', 'absolute', 1, 1000]
        ]},
    'ntp.system_sys_jitter': {
        'options': [None, "combined system jitter", "ms", 'system', 'ntp.sys_jitter', 'area'],
        'lines': [
            ['system_sys_jitter', 'sys_jitter', 'absolute', 1, 1000000]
        ]},
    'ntp.system_clk_jitter': {
        'options': [None, "clock jitter", "ms", 'system', 'ntp.clk_jitter', 'area'],
        'lines': [
            ['system_clk_jitter', 'clk_jitter', 'absolute', 1, 1000]
        ]},
    'ntp.system_clk_wander': {
        'options': [None, "clock frequency wander", "ppm", 'system', 'ntp.clk_wander', 'area'],
        'lines': [
            ['system_clk_wander', 'clk_wander', 'absolute', 1, 1000]
        ]},
    'ntp.system_precision': {
        'options': [None, "precision", "log2 s", 'system', 'ntp.precision', 'line'],
        'lines': [
            ['system_precision', 'precision', 'absolute', 1, 1]
        ]},
    'ntp.system_stratum': {
        'options': [None, "stratum (1-15)", "1", 'system', 'ntp.stratum', 'line'],
        'lines': [
            ['system_stratum', 'stratum', 'absolute', 1, 1]
        ]},
    'ntp.system_tc': {
        'options': [None, "time constant and poll exponent (3-17)", "log2 s", 'system', 'ntp.tc', 'line'],
        'lines': [
            ['system_tc', 'tc', 'absolute', 1, 1]
        ]},
    'ntp.system_mintc': {
        'options': [None, "minimum time constant (3-10)", "log2 s", 'system', 'ntp.mintc', 'line'],
        'lines': [
            ['system_mintc', 'mintc', 'absolute', 1, 1]
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

            return {'system_frequency': frequency * 1000,
                    'system_offset': offset * 1000000,
                    'system_rootdelay': rootdelay * 1000,
                    'system_rootdisp': rootdisp * 1000,
                    'system_sys_jitter': sys_jitter * 1000000,
                    'system_clk_jitter': clk_jitter * 1000,
                    'system_clk_wander': clk_wander * 1000,
                    'system_precision': precision,
                    'system_stratum': stratum,
                    'system_tc': tc,
                    'system_mintc': mintc}
        except (ValueError, AttributeError):
            return None

