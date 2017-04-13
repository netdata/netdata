# -*- coding: utf-8 -*-
# Description: routes netdata python.d module
# Author: Tristan Keen

import os.path
import re

from base import ExecutableService

# default module values (can be overridden per job in `config`)
update_every = 5
priority = 60000
retries = 60

# charts order (can be overridden if you want less charts, or different order)
ORDER = ['ipv4']
CIDR_MATCHER = re.compile(
    '^(([0-9]{1,3}\.){3}[0-9]{1,3}(\/([0-9]|[1-2][0-9]|3[0-2]))?)$')

# TODO: Host path to proc file under docker?
LINUX_IPV4_ROUTES_PROCFILE = '/proc/net/route'


class Service(ExecutableService):

    def __init__(self, configuration=None, name=None):
        ExecutableService.__init__(
            self, configuration=configuration, name=name)
        self.read_proc = False
        if os.path.isfile(LINUX_IPV4_ROUTES_PROCFILE):
            self.read_proc = True
            self.command = 'false'
        elif os.path.isfile('/sbin/ip'):
            self.command = 'ip -4 route list'
        else:
            self.command = 'netstat -rn4'
        self.order = ORDER
        self.tested_cidrs = self.configuration.get('tested_cidrs', [])
        self.definitions = {
            'ipv4': {
                'options': [None, 'IPv4 Routes', 'routes found', 'ipv4', 'ipv4.routes', 'line'],
                'lines': [
                    ['num_routes', None, 'absolute']
                ]
            }
        }
        for tested_cidr in self.tested_cidrs:
            self.definitions['ipv4']['lines'].append(
                [tested_cidr['name'], None, 'absolute'])

    def _read_proc_file(self):
        """
        Extract route table cidrs from /proc/net/route
        :return: list
        """
        cidrs = []
        with open(LINUX_IPV4_ROUTES_PROCFILE) as file:
            parsed_header = False
            for line in file:
                elems = line.split('\t')
                if not parsed_header:
                    dest_index = elems.index('Destination')
                    mask_index = elems.index('Mask')
                    if dest_index == -1 or mask_index == -1:
                        self.error('Unable to parse {} header:{}'.format(
                            LINUX_IPV4_ROUTES_PROCFILE, line))
                        break
                    parsed_header = True
                else:
                    dest = int(elems[dest_index], 16)
                    prefix_len = bin(int(elems[mask_index], 16)).count('1')
                    cidrs.append('{}.{}.{}.{}/{}'.format(dest & 0xff, (dest >> 8) &
                                                         0xff, (dest >> 16) & 0xff, dest >> 24, prefix_len))
        return cidrs

    def _normalise_command_output(self):
        """
        Reformat data received from route or ip command
        :return: list
        """
        raw = self._get_raw_data()
        cidrs = []
        for line in raw:
            start = line.split(' ')[0]
            if start == 'default':
                cidrs.append('0.0.0.0/0')
            elif CIDR_MATCHER.match(start):
                cidrs.append(start)
        return cidrs

    def _get_data(self):
        """
        Parse routing table to extract metrics
        :return: dict
        """
        if self.read_proc:
            cidrs = self._read_proc_file()
        else:
            cidrs = self._normalise_command_output()
        if len(cidrs) == 0:
            self.error('No routes detected - likely edge case for routes.chart.py')
            return None
        data = {'num_routes': len(cidrs)}
        for tested_cidr in self.tested_cidrs:
            found = 0
            for cidr in cidrs:
                if cidr == tested_cidr['cidr']:
                    found = 1
                    break
            data[tested_cidr['name']] = found
        return data
