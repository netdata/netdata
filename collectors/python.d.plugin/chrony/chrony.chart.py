# -*- coding: utf-8 -*-
# Description: chrony netdata python.d module
# Author: Dominik Schloesser (domschl)
# SPDX-License-Identifier: GPL-3.0-or-later

from bases.FrameworkServices.ExecutableService import ExecutableService

# default module values (can be overridden per job in `config`)
update_every = 5
priority = 60000

# charts order (can be overridden if you want less charts, or different order)
ORDER = ['system', 'offsets', 'stratum', 'root', 'frequency', 'residualfreq', 'skew']

CHARTS = {
    'system': {
        'options': [None, 'Chrony System Time Deltas', 'microseconds', 'system', 'chrony.system', 'area'],
        'lines': [
            ['timediff', 'system time', 'absolute', 1, 1000]
        ]
    },
    'offsets': {
        'options': [None, 'Chrony System Time Offsets', 'microseconds', 'system', 'chrony.offsets', 'area'],
        'lines': [
            ['lastoffset', 'last offset', 'absolute', 1, 1000],
            ['rmsoffset', 'RMS offset', 'absolute', 1, 1000]
        ]
    },
    'stratum': {
        'options': [None, 'Chrony Stratum', 'stratum', 'root', 'chrony.stratum', 'line'],
        'lines': [
            ['stratum', None, 'absolute', 1, 1]
        ]
    },
    'root': {
        'options': [None, 'Chrony Root Delays', 'milliseconds', 'root', 'chrony.root', 'line'],
        'lines': [
            ['rootdelay', 'delay', 'absolute', 1, 1000000],
            ['rootdispersion', 'dispersion', 'absolute', 1, 1000000]
        ]
    },
    'frequency': {
        'options': [None, 'Chrony Frequency', 'ppm', 'frequencies', 'chrony.frequency', 'area'],
        'lines': [
            ['frequency', None, 'absolute', 1, 1000]
        ]
    },
    'residualfreq': {
        'options': [None, 'Chrony Residual frequency', 'ppm', 'frequencies', 'chrony.residualfreq', 'area'],
        'lines': [
            ['residualfreq', 'residual frequency', 'absolute', 1, 1000]
        ]
    },
    'skew': {
        'options': [None, 'Chrony Skew, error bound on frequency', 'ppm', 'frequencies', 'chrony.skew', 'area'],
        'lines': [
            ['skew', None, 'absolute', 1, 1000]
        ]
    }
}

CHRONY = [
    ('Frequency', 'frequency', 1e3),
    ('Last offset', 'lastoffset', 1e9),
    ('RMS offset', 'rmsoffset', 1e9),
    ('Residual freq', 'residualfreq', 1e3),
    ('Root delay', 'rootdelay', 1e9),
    ('Root dispersion', 'rootdispersion', 1e9),
    ('Skew', 'skew', 1e3),
    ('Stratum', 'stratum', 1),
    ('System time', 'timediff', 1e9)
]


class Service(ExecutableService):
    def __init__(self, configuration=None, name=None):
        ExecutableService.__init__(
            self, configuration=configuration, name=name)
        self.command = 'chronyc -n tracking'
        self.order = ORDER
        self.definitions = CHARTS

    def _get_data(self):
        """
        Format data received from shell command
        :return: dict
        """
        raw_data = self._get_raw_data()
        if not raw_data:
            return None

        raw_data = (line.split(':', 1) for line in raw_data)
        parsed, data = dict(), dict()

        for line in raw_data:
            try:
                key, value = (l.strip() for l in line)
            except ValueError:
                continue
            if value:
                parsed[key] = value.split()[0]

        for key, dim_id, multiplier in CHRONY:
            try:
                data[dim_id] = int(float(parsed[key]) * multiplier)
            except (KeyError, ValueError):
                continue

        return data or None
