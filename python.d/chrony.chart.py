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

    def _get_data(self):
        """
        Format data received from shell command
        :return: dict
        """
        try:
            lines = self._get_raw_data()
            if lines is not None:
                chrony_dict = {}
                for line in lines:
                    lparts = line.split(':', 1)
                    value = lparts[1].strip().split(' ')[0]
                    chrony_dict[lparts[0].strip()] = value
                return {'timediff': int(float(chrony_dict['System time']) * 1e9),
                        'lastoffset': int(float(chrony_dict['Last offset']) * 1e9),
                        'rmsoffset': int(float(chrony_dict['RMS offset']) * 1e9),
                        'rootdelay': int(float(chrony_dict['Root delay']) * 1e9),
                        'rootdispersion': int(float(chrony_dict['Root dispersion']) * 1e9),
                        'skew': int(float(chrony_dict['Skew']) * 1e3),
                        'frequency': int(float(chrony_dict['Frequency']) * 1e3),
                        'residualfreq': int(float(chrony_dict['Residual freq']) * 1e3)
                        }
            else:
                self.error("No valid chronyc output")
                return None
        except (ValueError, AttributeError):
            self.error("Chronyc data parser exception")
            return None
