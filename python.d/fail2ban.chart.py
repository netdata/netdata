# -*- coding: utf-8 -*-
# Description: fail2ban log netdata python.d module
# Author: l2isbad

from base import LogService
from re import compile
try:
    from itertools import filterfalse
except ImportError:
    from itertools import ifilterfalse as filterfalse
from os import access as is_accessible, R_OK

priority = 60000
retries = 60
regex = compile(r'([A-Za-z-]+\]) enabled = ([a-z]+)')

ORDER = ['jails_group']


class Service(LogService):
    def __init__(self, configuration=None, name=None):
        LogService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.log_path = self.configuration.get('log_path', '/var/log/fail2ban.log')
        self.conf_path = self.configuration.get('conf_path', '/etc/fail2ban/jail.local')
        self.default_jails = ['ssh']
        try:
            self.exclude = self.configuration['exclude'].split()
        except (KeyError, AttributeError):
            self.exclude = []
        

    def _get_data(self):
        """
        Parse new log lines
        :return: dict
        """

        # If _get_raw_data returns empty list (no new lines in log file) we will send to Netdata this
        self.data = {jail: 0 for jail in self.jails_list}
        
        try:
            raw = self._get_raw_data()
            if raw is None:
                return None
            elif not raw:
                return self.data
        except (ValueError, AttributeError):
            return None

        # Fail2ban logs looks like
        # 2016-12-25 12:36:04,711 fail2ban.actions[2455]: WARNING [ssh] Ban 178.156.32.231
        self.data = dict(
            zip(
                self.jails_list,
                [len(list(filterfalse(lambda line: (jail + '] Ban') not in line, raw))) for jail in self.jails_list]
            ))

        return self.data

    def check(self):
            
        # Check "log_path" is accessible.
        # If NOT STOP plugin
        if not is_accessible(self.log_path, R_OK):
            self.error('Cannot access file %s' % (self.log_path))
            return False

        # Check "conf_path" is accessible.
        # If "conf_path" is accesible try to parse it to find enabled jails
        if is_accessible(self.conf_path, R_OK):
            with open(self.conf_path, 'rt') as jails_conf:
                jails_list = regex.findall(' '.join(jails_conf.read().split()))
            self.jails_list = [jail[:-1] for jail, status in jails_list if status == 'true']
        else:
            self.jails_list = []
            self.error('Cannot access jail.local file %s.' % (self.conf_path))
        
        # If for some reason parse failed we still can START with default jails_list.
        self.jails_list = [jail for jail in self.jails_list if jail not in self.exclude]\
                                              if self.jails_list else self.default_jails
        self.create_dimensions()
        self.info('Plugin succefully started. Jails: %s' % (self.jails_list))
        return True

    def create_dimensions(self):
        self.definitions = {'jails_group':
                                {'options':
                                     [None, "Jails ban statistics", "bans/s", 'Jails', 'jail.ban', 'line'], 'lines': []}}
        for jail in self.jails_list:
            self.definitions['jails_group']['lines'].append([jail, jail, 'absolute'])
