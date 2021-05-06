# -*- coding: utf-8 -*-
# Description: Parse iperf3 server log
# Author: Leo Sartre
# SPDX-License-Identifier: GPL-3.0-or-later

from bases.FrameworkServices.LogService import LogService

import re

NETDATA_UPDATE_EVERY=1
priority = 90000

ORDER = [
    "Bandwidth"
]

CHARTS = {
    "Bandwidth": {
        "options": [None, "Bandwidth", "bits/sec", "IPERF", "iperf3.bandwith", "line"],
        "lines": [
            # lines are created dynamically in `check()`
        ]
    }
}

RE_PORT = re.compile(r'Server listening on (\d+)')
RE_BDWTH = re.compile(r'(\d+\.?\d*)-(\d+\.?\d*).*?(\d+\.?\d*) ([KMGT]?bits)/sec')

class Service(LogService):
    def __init__(self, configuration=None, name=None):
        LogService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.log_path = self.configuration.get('path', "/tmp/iperfs.log")
        self.line_name = None
        self.bdwth_data = []

    def get_iperf_port_no(self):
        lines = self._get_raw_data()
        if not lines:
            return None

        for l in reversed(lines):
            port = RE_PORT.findall(l)
            if port:
                break # stops as soon as the port is found

        if port:
            return port[0]

        return None

    def check(self):
        """
        Get the port number on which the server listen and create the line
        """
        if not LogService.check(self):
            return False

        if not self.get_iperf_port_no():
            return False

        self.line_name = "current_bandwidth"
        self.definitions['Bandwidth']['lines'].append([self.line_name])

        return True

    def get_data(self):
        # parse new lines and append data to the bdwth_data list
        lines = self._get_raw_data()
        for l in lines:
            t = RE_BDWTH.findall(l)
            v = 0
            if t:
                start, stop, value, unit = t[0]
                # ignore last iperf line that is garbage
                if start == stop:
                    break
                v = float(value)
                if unit == "bits":
                    v *= 1
                elif unit == 'Kbits':
                    v *= 1e3
                elif unit == 'Mbits':
                    v *= 1e6
                elif unit == 'Gbits':
                    v *= 1e9
                elif unit == 'Tbits':
                    v *= 1e12
                else:
                    self.warning("Unexpected unit found: {}!".format(unit))
                    continue

                self.bdwth_data.append(int(v))

        # dequeue the bdwth_data list
        if self.bdwth_data:
            value = self.bdwth_data.pop(0)
            self.info("Returning {}".format(value))
            return {self.line_name: value}
        else:
            return {self.line_name: 0}
