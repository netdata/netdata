# -*- coding: utf-8 -*-
# Description: isc dhcpd lease netdata python.d module
# Author: l2isbad

from time import mktime, strptime, gmtime, time
from os import stat, access, R_OK
from os.path import isfile
try:
    from ipaddress import ip_network, ip_address
    HAVE_IPADDRESS = True
except ImportError:
    HAVE_IPADDRESS = False
try:
    from itertools import filterfalse
except ImportError:
    from itertools import ifilterfalse as filterfalse

from bases.FrameworkServices.SimpleService import SimpleService

priority = 60000
retries = 60
update_every = 5

ORDER = ['pools_utilization', 'pools_active_leases', 'leases_total', 'parse_time', 'leases_size']

CHARTS = {
    'pools_utilization': {
        'options': [None, 'Pools Utilization', 'used in percent', 'utilization',
                    'isc_dhcpd.utilization', 'line'],
        'lines': []},
    'pools_active_leases': {
        'options': [None, 'Active Leases', 'leases per pool', 'active leases',
                    'isc_dhcpd.active_leases', 'line'],
        'lines': []},
    'leases_total': {
        'options': [None, 'Total All Pools', 'number', 'active leases',
                    'isc_dhcpd.leases_total', 'line'],
        'lines': [['leases_total', 'leases', 'absolute']]},
    'parse_time': {
        'options': [None, 'Parse Time', 'ms', 'parse stats',
                    'isc_dhcpd.parse_time', 'line'],
        'lines': [['parse_time', 'time', 'absolute']]},
    'leases_size': {
        'options': [None, 'Dhcpd Leases File Size', 'kilobytes',
                    'parse stats', 'isc_dhcpd.leases_size', 'line'],
        'lines': [['leases_size', 'size', 'absolute', 1, 1024]]}}


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.leases_path = self.configuration.get('leases_path', '/var/lib/dhcp/dhcpd.leases')
        self.order = ORDER
        self.definitions = CHARTS
        self.pools = dict()

        # Will work only with 'default' db-time-format (weekday year/month/day hour:minute:second)
        # TODO: update algorithm to parse correctly 'local' db-time-format
        # (epoch <seconds-since-epoch>; # <day-name> <month-name> <day-number> <hours>:<minutes>:<seconds> <year>)
        # Also only ipv4 supported

    def check(self):
        if not HAVE_IPADDRESS:
            self.error('\'python-ipaddress\' module is needed')
            return False
        if not (isfile(self.leases_path) and access(self.leases_path, R_OK)):
            self.error('Make sure leases_path is correct and leases log file is readable by netdata')
            return False
        if not self.configuration.get('pools'):
            self.error('Pools are not defined')
            return False
        if not isinstance(self.configuration['pools'], dict):
            self.error('Invalid \'pools\' format')
            return False

        for pool in self.configuration['pools']:
            try:
                net = ip_network(u'%s' % self.configuration['pools'][pool])
                self.pools[pool] = dict(net=net, num_hosts=net.num_addresses - 2)
            except ValueError as error:
                self.error('%s removed, error: %s' % (self.configuration['pools'][pool], error))

        if not self.pools:
            return False
        self.create_charts()
        return True

    def _get_raw_data(self):
        """
        Parses log file
        :return: tuple(
                       [ipaddress, lease end time, ...],
                       time to parse leases file
                      )
        """
        try:
            with open(self.leases_path) as leases:
                time_start = time()
                part1 = filterfalse(find_lease, leases)
                part2 = filterfalse(find_ends, leases)
                result = dict(zip(part1, part2))
                time_end = time()
                file_parse_time = round((time_end - time_start) * 1000)
                return result, file_parse_time
        except (OSError, IOError) as error:
            self.error("Failed to parse leases file:", str(error))
            return None

    def _get_data(self):
        """
        :return: dict
        """
        raw_data = self._get_raw_data()
        if not raw_data:
            return None

        raw_leases, parse_time = raw_data[0], raw_data[1]

        # Result: {ipaddress: end lease time, ...}
        active_leases, to_netdata = list(), dict()
        current_time = mktime(gmtime())

        for ip, lease_end_time in raw_leases.items():
            # Result: [active binding, active binding....]. (Expire time (ends date;) - current time > 0)
            if binding_active(lease_end_time=lease_end_time[7:-2],
                              current_time=current_time):
                active_leases.append(ip_address(u'%s' % ip[6:-3]))

        for pool in self.pools:
            dim_id = pool.replace('.', '_')
            pool_leases_count = len([ip for ip in active_leases if ip in self.pools[pool]['net']])
            to_netdata[dim_id + '_active_leases'] = pool_leases_count
            to_netdata[dim_id + '_utilization'] = float(pool_leases_count) / self.pools[pool]['num_hosts'] * 10000

        to_netdata['leases_total'] = len(active_leases)
        to_netdata['leases_size'] = stat(self.leases_path)[6]
        to_netdata['parse_time'] = parse_time
        return to_netdata

    def create_charts(self):
        for pool in self.pools:
            dim, dim_id = pool, pool.replace('.', '_')
            self.definitions['pools_utilization']['lines'].append([dim_id + '_utilization',
                                                                   dim, 'absolute', 1, 100])
            self.definitions['pools_active_leases']['lines'].append([dim_id + '_active_leases',
                                                                     dim, 'absolute'])


def binding_active(lease_end_time, current_time):
    return mktime(strptime(lease_end_time, '%w %Y/%m/%d %H:%M:%S')) - current_time > 0


def find_lease(value):
    return value[0:3] != 'lea'


def find_ends(value):
    return value[2:6] != 'ends'
