# -*- coding: utf-8 -*-
# Description: isc dhcpd lease netdata python.d module
# Author: l2isbad

from base import SimpleService
from time import mktime, strptime, gmtime, time
from os import stat
try:
    from ipaddress import IPv4Address as ipaddress
    from ipaddress import ip_network
    have_ipaddress = True
except ImportError:
    have_ipaddress = False
try:
    from itertools import filterfalse as filterfalse
except ImportError:
    from itertools import ifilterfalse as filterfalse


priority = 60000
retries = 60
update_every = 60

class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.leases_path = self.configuration.get('leases_path', '/var/lib/dhcp/dhcpd.leases')
        self.pools = self.configuration.get('pools')

        # Will work only with 'default' db-time-format (weekday year/month/day hour:minute:second)
        # TODO: update algorithm to parse correctly 'local' db-time-format
        # (epoch <seconds-since-epoch>; # <day-name> <month-name> <day-number> <hours>:<minutes>:<seconds> <year>)
        # Also only ipv4 supported

    def check(self):
        if not self._get_raw_data():
            self.error('Make sure leases_path is correct and leases log file is readable by netdata')
            return False
        elif not have_ipaddress:
            self.error('No ipaddress module. Please install (py2-ipaddress in case of python2)')
            return False
        else:
            try:
                self.pools = self.pools.split()
                if not [ip_network(return_utf(pool)) for pool in self.pools]:
                    self.error('Pools list is empty')
                    return False
            except (ValueError, IndexError, AttributeError, SyntaxError) as e:
                self.error('Pools configurations is incorrect', str(e))
                return False

            # Creating static charts
            self.order = ['parse_time', 'leases_size', 'utilization', 'total']
            self.definitions = {'utilization':
                                    {'options':
                                         [None, 'Pools utilization', 'used %', 'utilization', 'isc_dhcpd.util', 'line'],
                                     'lines': []},
                                 'total':
                                   {'options':
                                        [None, 'Total all pools', 'leases', 'utilization', 'isc_dhcpd.total', 'line'],
                                    'lines': [['total', 'leases', 'absolute']]},
                               'parse_time':
                                   {'options':
                                        [None, 'Parse time', 'ms', 'parse stats', 'isc_dhcpd.parse', 'line'],
                                    'lines': [['ptime', 'time', 'absolute']]},
                               'leases_size':
                                   {'options':
                                        [None, 'dhcpd.leases file size', 'kilobytes', 'parse stats', 'isc_dhcpd.lsize', 'line'],
                                    'lines': [['lsize', 'size', 'absolute']]}}
            # Creating dynamic charts
            for pool in self.pools:
                self.definitions['utilization']['lines'].append([''.join(['ut_', pool]), pool, 'absolute'])
                self.order.append(''.join(['leases_', pool]))
                self.definitions[''.join(['leases_', pool])] = \
                    {'options': [None, 'Active leases', 'leases', 'pools', 'isc_dhcpd.lease', 'area'],
                     'lines': [[''.join(['le_', pool]), pool, 'absolute']]}

            self.info('Plugin was started succesfully')
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
            with open(self.leases_path, 'rt') as dhcp_leases:
                time_start = time()
                part1 = filterfalse(find_lease, dhcp_leases)
                part2 = filterfalse(find_ends, dhcp_leases)
                raw_result = dict(zip(part1, part2))
                time_end = time()
                file_parse_time = round((time_end - time_start) * 1000)
        except Exception as e:
            self.error("Failed to parse leases file:", str(e))
            return None
        else:
            result = (raw_result, file_parse_time)
            return result

    def _get_data(self):
        """
        :return: dict
        """
        raw_leases = self._get_raw_data()
        if not raw_leases:
            return None

        # Result: {ipaddress: end lease time, ...}
        all_leases = dict([(k[6:len(k)-3], v[7:len(v)-2]) for k, v in raw_leases[0].items()])

        # Result: [active binding, active binding....]. (Expire time (ends date;) - current time > 0)
        active_leases = [k for k, v in all_leases.items() if is_binding_active(all_leases[k])]

        # Result: {pool: number of active bindings in pool, ...}
        pools_count = dict([(pool, len([lease for lease in active_leases if is_address_in(lease, pool)]))
                       for pool in self.pools])

        # Result: {pool: number of host ip addresses in pool, ...}
        pools_max = dict([(pool, (2 ** (32 - int(pool.split('/')[1])) - 2))
                     for pool in self.pools])

        # Result: {pool: % utilization, ....} (percent)
        pools_util = dict([(pool, int(round(float(pools_count[pool]) / pools_max[pool] * 100, 0)))
                      for pool in self.pools])

        # Bulding dicts to send to netdata
        final_count = dict([(''.join(['le_', k]), v) for k, v in pools_count.items()])
        final_util = dict([(''.join(['ut_', k]), v) for k, v in pools_util.items()])

        to_netdata = {'total': len(active_leases)}
        to_netdata.update({'lsize': int(stat(self.leases_path)[6] / 1024)})
        to_netdata.update({'ptime': int(raw_leases[1])})
        to_netdata.update(final_util)
        to_netdata.update(final_count)

        return to_netdata


def is_binding_active(binding):
    return mktime(strptime(binding, '%w %Y/%m/%d %H:%M:%S')) - mktime(gmtime()) > 0


def is_address_in(address, pool):
    return ipaddress(return_utf(address)) in ip_network(return_utf(pool))


def find_lease(value):
    return value[0:3] != 'lea'


def find_ends(value):
    return value[2:6] != 'ends'


def return_utf(s):
    # python2 returns "<type 'str'>" for simple strings
    # python3 returns "<class 'str'>" for unicode strings
    if str(type(s)) == "<type 'str'>":
        return unicode(s, 'utf-8')
    return s
