# -*- coding: utf-8 -*-
# Description: mdstat netdata python.d module
# Author: l2isbad

from base import SimpleService
from re import compile
priority = 60000
retries = 60
update_every = 1


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.order = ['agr_health']
        self.definitions = {'agr_health':
                                {'options':
                                     [None, 'Faulty devices in MD', 'failed disks', 'health', 'md.health', 'line'],
                                 'lines': []}}
        self.proc_mdstat = '/proc/mdstat'
        self.regex_disks = compile(r'((?<=\ )[a-zA-Z_0-9]+(?= : active)).*?((?<= \[)[0-9]+)/([0-9]+(?=\] ))')
        self.regex_status = compile(r'([a-zA-Z_0-9]+)( : active)[^:]*?([a-z]+) = ([0-9.]+(?=%))')

    def check(self):
        raw_data = self._get_raw_data()
        if not raw_data:
            self.error('Cant read mdstat data from %s' % (self.proc_mdstat))
            return False
        else:
            md_list = [md[0] for md in self.regex_disks.findall(raw_data)]
            for md in md_list:
                self.order.append(md)
                self.order.append(''.join([md, '_status']))
                self.definitions['agr_health']['lines'].append([''.join([md, '_health']), md, 'absolute'])
                self.definitions[md] = {'options':
                                            [None, 'MD disks stats', 'disks', md, 'md.stats', 'stacked'],
                                        'lines': [[''.join([md, '_total']), 'total', 'absolute'],
                                                  [''.join([md, '_inuse']), 'inuse', 'absolute']]}
                self.definitions[''.join([md, '_status'])] = {'options':
                                            [None, 'MD current status', 'percent', md, 'md.status', 'line'],
                                        'lines': [[''.join([md, '_resync']), 'resync', 'absolute'],
                                                  [''.join([md, '_recovery']), 'recovery', 'absolute'],
                                                  [''.join([md, '_check']), 'check', 'absolute']]}
            self.info('Plugin was started successfully. MDs to monitor %s' % (md_list))

            return True

    def _get_raw_data(self):
        """
        Read data from /proc/mdstat
        :return: str
        """
        try:
            with open(self.proc_mdstat, 'rt') as proc_mdstat:
                raw_result = proc_mdstat.read()
        except Exception:
            return None
        else:
            raw_result = ' '.join(raw_result.split())
            return raw_result

    def _get_data(self):
        """
        Parse data from _get_raw_data()
        :return: dict
        """
        raw_mdstat = self._get_raw_data()
        mdstat_disks = self.regex_disks.findall(raw_mdstat)
        mdstat_status = self.regex_status.findall(raw_mdstat)
        to_netdata = {}

        for md in mdstat_disks:
            to_netdata[''.join([md[0], '_total'])] = int(md[1])
            to_netdata[''.join([md[0], '_inuse'])] = int(md[2])
            to_netdata[''.join([md[0], '_health'])] = int(md[1]) - int(md[2])
            to_netdata[''.join([md[0], '_check'])] = 0
            to_netdata[''.join([md[0], '_resync'])] = 0
            to_netdata[''.join([md[0], '_recovery'])] = 0
        
        for md in mdstat_status:
            to_netdata[''.join([md[0], '_' + md[2]])] = round(float(md[3]))

        return to_netdata
