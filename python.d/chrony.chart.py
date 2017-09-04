# -*- coding: utf-8 -*-
# Description: chrony netdata python.d module
# Author: Dominik Schloesser (domschl)

from base import ExecutableService

# default module values (can be overridden per job in `config`)
# update_every = 10
priority = 60000
retries = 10

# charts order (can be overridden if you want less charts, or different order)
ORDER = ['timediff', 'lastoffset', 'rmsoffset', 'rootdelay',
         'rootdispersion', 'skew', 'frequency', 'residualfreq']

CHARTS = {
    # id: {
    #     'options': [name, title, units, family, context, charttype],
    #     'lines': [
    #         [unique_dimension_name, name, algorithm, multiplier, divisor]
    #     ]}
    'timediff': {
        'options': [None, "Difference system time to NTP", "us", 'chrony', 'chrony.timediff', 'line'],
        'lines': [
            ['timediff', None, 'absolute', 1, 1000]
        ]},
    'lastoffset': {
        'options': [None, "Last offset", "us", 'chrony', 'chrony.lastoffset', 'line'],
        'lines': [
            ['lastoffset', None, 'absolute', 1, 1000]
        ]},
    'rmsoffset': {
        'options': [None, "RMS offset", "us", 'chrony', 'chrony.rmsoffset', 'line'],
        'lines': [
            ['rmsoffset', None, 'absolute', 1, 1000]
        ]},
    'rootdelay': {
        'options': [None, "Root delay", "us", 'chrony', 'chrony.rootdelay', 'line'],
        'lines': [
            ['rootdelay', None, 'absolute', 1, 1000]
        ]},
    'rootdispersion': {
        'options': [None, "Root dispersion", "us", 'chrony', 'chrony.rootdispersion', 'line'],
        'lines': [
            ['rootdispersion', None, 'absolute', 1, 1000]
        ]},
    'skew': {
        'options': [None, "Skew, error bound on frequency", "ppm", 'chrony', 'chrony.skew', 'line'],
        'lines': [
            ['skew', None, 'absolute', 1, 1000]
        ]},
    'frequency': {
        'options': [None, "Frequency", "ppm", 'chrony', 'chrony.frequency', 'line'],
        'lines': [
            ['frequency', None, 'absolute', 1, 1000]
        ]},
    'residualfreq': {
        'options': [None, "Residual frequency", "ppm", 'chrony', 'chrony.residualfreq', 'line'],
        'lines': [
            ['residualfreq', None, 'absolute', 1, 1000]
        ]}
}


class Service(ExecutableService):
    def __init__(self, configuration=None, name=None):
        ExecutableService.__init__(
            self, configuration=configuration, name=name)
        self.command = "chronyc -n tracking"
        self.order = ORDER
        self.definitions = CHARTS

    CHRONY = [('Frequency', 'frequency', 1e3),
              ('Last offset', 'lastoffset', 1e9),
              ('RMS offset', 'rmsoffset', 1e9),
              ('Residual freq', 'residualfreq', 1e3),
              ('Root delay', 'rootdelay', 1e9),
              ('Root dispersion', 'rootdispersion', 1e9),
              ('Skew', 'skew', 1e3),
              ('System time', 'timediff', 1e9)]

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
            if len(value) > 0:
                parsed[key] = value.split()[0]

        for key, dim_id, multiplier in self.CHRONY:
            try:
                data[dim_id] = int(float(parsed[key]) * multiplier)
            except (KeyError, ValueError):
                continue

        return data or None
