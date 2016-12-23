# -*- coding: utf-8 -*-
# Description: isc dhcpd lease netdata python.d module
# Author: l2isbad

from base import SimpleService
from re import compile
from time import mktime, strptime, gmtime, time
try:
    from ipaddress import IPv4Address as ipaddress
    from ipaddress import ip_network
    have_ipaddress = True
except ImportError:
    have_ipaddress = False

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
        self.regex = compile(r'\d+(?:\.\d+){3}')

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
                if not [ip_network(pool) for pool in self.pools]:
                    self.error('Pools list is empty')
                    return False
            except (ValueError, IndexError, AttributeError, SyntaxError):
                self.error('Pools configurations is incorrect')
                return False
            
            # Creating dynamic charts
            self.order = ['parse_time', 'utilization']
            self.definitions = {'utilization':
                                    {'options':
                                         [None, 'Pools utilization', 'used %', 'Utulization', 'isc_dhcpd.util', 'line'],
                                     'lines': []},
                               'parse_time':
                                   {'options':
                                        [None, 'Parse time', 'ms', 'Parse statistics', 'isc_dhcpd.parse', 'line'],
                                    'lines': [['ptime', 'time', 'absolute']]}}
            for pool in self.pools:
                self.definitions['utilization']['lines'].append([''.join(['ut_', pool]), pool, 'absolute'])
                self.order.append(''.join(['leases_', pool]))
                self.definitions[''.join(['leases_', pool])] = \
                    {'options': [None, 'Active leases', 'leases', 'Leases', 'isc_dhcpd.lease', 'area'], 
                     'lines': [[''.join(['le_', pool]), pool, 'absolute']]}

            self.info('Plugin was started succesfully')
            return True

    def _get_raw_data(self):
        """
        Parses log file
        :return: tuple(
                       [ipaddress, lease end time, ...],
                       length of list,
                       time to parse leases file
                      )
        """
        try:
            with open(self.leases_path, 'rt') as dhcp_leases:
                raw_result = []

                time_start = time()
                for line in dhcp_leases:
                    if line[0:3] == 'lea':
                        raw_result.append(self.regex.search(line).group())
                    elif line[2:6] == 'ends':
                        raw_result.append(line[7:28])
                    else:
                        continue
                time_end = time()
                file_parse_time = round((time_end - time_start) * 1000)

        except Exception:
            return None

        else:
            raw_result_length = len(raw_result)
            result = (raw_result, raw_result_length, file_parse_time)
            return result

    def _get_data(self):
        """
        :return: dict
        """
        raw_leases = self._get_raw_data()
        
        if not raw_leases:
            return None

        # Result: {ipaddress: end lease time, ...}
        all_leases = dict(zip([raw_leases[0][_] for _ in range(0, raw_leases[1], 2)],
                              [raw_leases[0][_] for _ in range(1, raw_leases[1], 2)]))

        # Result: [active binding, active binding....]. (Expire time (ends date;) - current time > 0)
        active_leases = [k for k, v in all_leases.items() if is_bind_active(all_leases[k])]

        # Result: {pool: number of active bindings in pool, ...}
        pools_count = {pool: len([lease for lease in active_leases if is_address_in(lease, pool)])
                       for pool in self.pools}

        # Result: {pool: number of host ip addresses in pool, ...}
        pools_max = {pool: (2 ** (32 - int(pool.split('/')[1])) - 2)
                     for pool in self.pools}

        # Result: {pool: % utilization, ....} (percent)
        pools_util = {pool:int(round(float(pools_count[pool]) / pools_max[pool] * 100, 0))
                      for pool in self.pools}

        # Bulding dicts to send to netdata
        final_count = {''.join(['le_', k]): v for k, v in pools_count.items()}
        final_util = {''.join(['ut_', k]): v for k, v in pools_util.items()}

        to_netdata = {'ptime': int(raw_leases[2])}
        to_netdata.update(final_util)
        to_netdata.update(final_count)
 
        return to_netdata
    
def is_bind_active(binding):
    return mktime(strptime(binding, '%w %Y/%m/%d %H:%M:%S')) - mktime(gmtime()) > 0

def is_address_in(address, pool):
    return ipaddress(address) in ip_network(pool)
