# -*- coding: utf-8 -*-
# Description: routes netdata python.d module
# Author: Tristan Keen

from copy import deepcopy
import os.path
import re

from base import ExecutableService

# default module values (can be overridden per job in `config`)
update_every = 5
priority = 60000
retries = 60

# charts order (can be overridden if you want less charts, or different order)
ORDER = ['ipv4']
CHARTS = {
    'ipv4': {
        'options': [None, 'IPv4 Routes', 'routes found', 'ipv4', 'ipv4.routes', 'line'],
        'lines': []
    }}
CIDR_MATCHER = re.compile(
    '^(default|([0-9]{1,3})\.([0-9]{1,3})\.([0-9]{1,3})\.([0-9]{1,3})(?:\/([0-9]|[1-2][0-9]|3[0-2]))?)(?:(?:\s.*)?)$')

# TODO: Host path to proc file under docker?
LINUX_IPV4_ROUTES_PROCFILE = '/proc/net/route'


class Service(ExecutableService):

    def __init__(self, configuration=None, name=None):
        ExecutableService.__init__(
            self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = deepcopy(CHARTS)
        self.tested_cidrs = self.configuration.get('tested_cidrs')

    def check(self):
        if not (self.tested_cidrs and hasattr(self.tested_cidrs, 'keys')):
            self.error('tested_cidrs not defined')
            return False

        if os.path.isfile(LINUX_IPV4_ROUTES_PROCFILE):
            self.command = 'false'
            self.get_routes_as_numeric_cidrs = self._read_proc_file
        elif self.find_binary('ip'):
            self.command = 'ip -4 route list'
            self.get_routes_as_numeric_cidrs = self._parse_cidr_output
        else:
            self.command = 'netstat -rn4'
            # TODO check which sort of netstat output
            self.get_routes_as_numeric_cidrs = self._parse_netstat_netmask_output

        self.numeric_cidrs_to_test = []
        for cidr_name, cidr in self.tested_cidrs.items():
            self.definitions['ipv4']['lines'].append(
                [cidr_name, None, 'absolute'])
            numeric_cidr = self._cidr_to_number(cidr)
            if numeric_cidr < 0:
                self.error('Unparsable CIDR: ' + cidr)
                return False
            self.numeric_cidrs_to_test.append((cidr_name, numeric_cidr))
        return True

    def _read_proc_file(self):
        """
        Extract route table cidrs from /proc/net/route
        :return: set
        """
        routes_as_numeric_cidrs = set()
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
                    reversed_dest = int(elems[dest_index], 16)
                    prefix_len = bin(int(elems[mask_index], 16)).count('1')
                    routes_as_numeric_cidrs.add((reversed_dest << 8) + prefix_len)
        return routes_as_numeric_cidrs

    def _parse_cidr_output(self):
        """
        Run ip or other command that returns cidrs and parse to numeric_cidrs
        :return: set
        """
        raw_output = self._get_raw_data()
        if not raw_output:
            return None
        routes_as_numeric_cidrs = set()
        for line in raw_output:
            numeric_cidr = self._cidr_to_number(line)
            if numeric_cidr >= 0:
                routes_as_numeric_cidrs.add(numeric_cidr)
        return routes_as_numeric_cidrs

    @staticmethod
    def _cidr_to_number(text):
        """
        Convert the CIDR of form A.B.C.D/E to a number formed as DCBAE in base 256,
        i.e. the 32-bit reversed hex Destination from /proc/net/route
        bumped up by 8 bits with the prefix length added.
        """
        m = CIDR_MATCHER.match(text)
        if not m:
            return -1
        if m.group(0) == 'default':
            return 0
        prefix_length = 32 if m.group(6) == None else int(m.group(6))
        return ((((int(m.group(5)) << 8) + int(m.group(4)) << 8) + int(m.group(3)) << 8) + int(m.group(2)) << 8) + prefix_length

    def _get_data(self):
        """
        Read routing table via selected strategy and extract metrics
        :return: dict
        """
        routes = self.get_routes_as_numeric_cidrs()
        if len(routes) == 0:
            self.error('No routes detected - likely bug in routes_v4.chart.py')
            return None
        data = {'num_routes': len(routes)}
        for cidr_name, numeric_cidr in self.numeric_cidrs_to_test:
            data[cidr_name] = 1 if numeric_cidr in routes else 0
        return data
