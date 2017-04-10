# -*- coding: utf-8 -*-
# Description: routes netdata python.d module
# Author: Tristan Keen

import os.path

from base import ExecutableService

# default module values (can be overridden per job in `config`)
update_every = 5
priority = 60000
retries = 60

# charts order (can be overridden if you want less charts, or different order)
ORDER = ['routes_v4']


class Service(ExecutableService):

    def __init__(self, configuration=None, name=None):
        ExecutableService.__init__(self, configuration=configuration, name=name)
        if os.path.isfile('/sbin/ip'):
            self.command = '/sbin/ip -4 route list'
        else:
            self.command = '/usr/bin/netstat -rn4'
        self.order = ORDER
        self.named_cidrs = self.configuration.get('named_cidrs')
        self.definitions = {
            'routes_v4': {
                'options': [None, 'IPv4 Routes', 'routes found', 'ipv4', 'routes.routes_v4', 'line'],
                'lines': [
                    ['num_routes', None, 'absolute']
                ]
            }
        }
        for named_cidr in self.named_cidrs:
            self.definitions['routes_v4']['lines'].append([named_cidr['name'], None, 'absolute'])

    def _get_data(self):
        """
        Reformat data received from route command
        :return: dict
        """

        try:
            raw = self._get_raw_data()
            data = {'num_routes': len(raw)}
            for named_cidr in self.named_cidrs:
                data[named_cidr['name']] = 0
                for line in raw:
                    if line.startswith(named_cidr['cidr'] + ' '):
                        data[named_cidr['name']] = 1
            return data
        except (ValueError, AttributeError):
            return None
