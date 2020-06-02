# -*- coding: utf-8 -*-
# Description: isc dhcpd lease netdata python.d module
# Author: ilyam8
# SPDX-License-Identifier: GPL-3.0-or-later

import os
import re
import time

try:
    import ipaddress

    HAVE_IP_ADDRESS = True
except ImportError:
    HAVE_IP_ADDRESS = False

from collections import defaultdict
from copy import deepcopy

from bases.FrameworkServices.SimpleService import SimpleService

ORDER = [
    'pools_utilization',
    'pools_active_leases',
    'leases_total',
]

CHARTS = {
    'pools_utilization': {
        'options': [None, 'Pools Utilization', 'percentage', 'utilization', 'isc_dhcpd.utilization', 'line'],
        'lines': []
    },
    'pools_active_leases': {
        'options': [None, 'Active Leases Per Pool', 'leases', 'active leases', 'isc_dhcpd.active_leases', 'line'],
        'lines': []
    },
    'leases_total': {
        'options': [None, 'All Active Leases', 'leases', 'active leases', 'isc_dhcpd.leases_total', 'line'],
        'lines': [
            ['leases_total', 'leases', 'absolute']
        ],
        'variables': [
            ['leases_size']
        ]
    }
}


class DhcpdLeasesFile:
    def __init__(self, path):
        self.path = path
        self.mod_time = 0
        self.size = 0

    def is_valid(self):
        return os.path.isfile(self.path) and os.access(self.path, os.R_OK)

    def is_changed(self):
        mod_time = os.path.getmtime(self.path)
        if mod_time != self.mod_time:
            self.mod_time = mod_time
            self.size = int(os.path.getsize(self.path) / 1024)
            return True
        return False

    def get_data(self):
        try:
            with open(self.path) as leases:
                result = defaultdict(dict)
                for row in leases:
                    row = row.strip()
                    if row.startswith('lease'):
                        address = row[6:-2]
                    elif row.startswith('iaaddr'):
                        address = row[7:-2]
                    elif row.startswith('ends'):
                        result[address]['ends'] = row[5:-1]
                    elif row.startswith('binding state'):
                        result[address]['state'] = row[14:-1]
                return dict((k, v) for k, v in result.items() if len(v) == 2)
        except (OSError, IOError):
            return None


class Pool:
    def __init__(self, name, network):
        self.id = re.sub(r'[:/.-]+', '_', name)
        self.name = name

        self.networks = list()
        for network_item in network.split(" "):
            range_ip = network_item.split("-")
            if len(range_ip) == 1:
                self.networks.append({
                    'type': 'network',
                    'list': ipaddress.ip_network(address=u'%s' % network_item)
                })
            elif len(range_ip) == 2:
                self.networks.append({
                    'type': 'range',
                    'list': list(
                                ipaddress.summarize_address_range(
                                    ipaddress.ip_address(u'%s' % range_ip[0]), 
                                    ipaddress.ip_address(u'%s' % range_ip[1])
                                )
                            )
                })
            else:
                raise ValueError('Network syntaxis error!')

    def _get_networks_by_type(self, type_network):
        filter_network = filter(lambda network: network.get("type") == type_network, self.networks)
        data_return = list()
        if type_network == "range":
            for network in filter_network:
                data_return = data_return + network.get('list')
            
        elif type_network == "network":
            data_return = list( network.get('list') for network in filter_network )
        
        elif type_network == "all":
            data_return += self._get_networks_by_type("network")
            data_return += self._get_networks_by_type("range")

        return data_return

    def num_hosts(self):
        data_return = 0
        data_return += sum([network.num_addresses - 2 for network in self._get_networks_by_type("network")])
        data_return += sum([network.num_addresses for network in self._get_networks_by_type("range")])
        return data_return

    def __contains__(self, item):
        for network in self._get_networks_by_type("all"):
            if item.address in network:
                return True

        return False

class Lease:
    def __init__(self, address, ends, state):
        self.address = ipaddress.ip_address(address=u'%s' % address)
        self.ends = ends
        self.state = state

    def is_active(self, current_time):
        # lease_end_time might be epoch
        if self.ends.startswith('epoch'):
            epoch = int(self.ends.split()[1].replace(';', ''))
            return epoch - current_time > 0
        # max. int for lease-time causes lease to expire in year 2038.
        # dhcpd puts 'never' in the ends section of active lease
        elif self.ends == 'never':
            return True
        return time.mktime(time.strptime(self.ends, '%w %Y/%m/%d %H:%M:%S')) - current_time > 0

    def is_valid(self):
        return self.state == 'active'


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = deepcopy(CHARTS)
        lease_path = self.configuration.get('leases_path', '/var/lib/dhcp/dhcpd.leases')
        self.dhcpd_leases = DhcpdLeasesFile(path=lease_path)
        self.pools = list()
        self.data = dict()

        # Will work only with 'default' db-time-format (weekday year/month/day hour:minute:second)
        # TODO: update algorithm to parse correctly 'local' db-time-format

    def check(self):
        if not HAVE_IP_ADDRESS:
            self.error("'python-ipaddress' package is needed")
            return False

        if not self.dhcpd_leases.is_valid():
            self.error("Make sure '{path}' is exist and readable by netdata".format(path=self.dhcpd_leases.path))
            return False

        pools = self.configuration.get('pools')
        if not pools:
            self.error('Pools are not defined')
            return False

        for pool in pools:
            try:
                new_pool = Pool(name=pool, network=pools[pool])
            except ValueError as error:
                self.error("'{pool}' was removed, error: {error}".format(pool=pools[pool], error=error))
            else:
                self.pools.append(new_pool)

        self.create_charts()
        return bool(self.pools)

    def get_data(self):
        """
        :return: dict
        """
        if not self.dhcpd_leases.is_changed():
            return self.data

        raw_leases = self.dhcpd_leases.get_data()
        if not raw_leases:
            self.data = dict()
            return None

        active_leases = list()
        current_time = time.mktime(time.gmtime())

        for address in raw_leases:
            try:
                new_lease = Lease(address, **raw_leases[address])
            except ValueError:
                continue
            else:
                if new_lease.is_active(current_time) and new_lease.is_valid():
                    active_leases.append(new_lease)

        for pool in self.pools:
            count = len([ip for ip in active_leases if ip in pool])
            self.data[pool.id + '_active_leases'] = count
            self.data[pool.id + '_utilization'] = float(count) / pool.num_hosts() * 10000

        self.data['leases_size'] = self.dhcpd_leases.size
        self.data['leases_total'] = len(active_leases)

        return self.data

    def create_charts(self):
        for pool in self.pools:
            dim = [
                pool.id + '_utilization',
                pool.name,
                'absolute',
                1,
                100,
            ]
            self.definitions['pools_utilization']['lines'].append(dim)

            dim = [
                pool.id + '_active_leases',
                pool.name,
            ]
            self.definitions['pools_active_leases']['lines'].append(dim)
