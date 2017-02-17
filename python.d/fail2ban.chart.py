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
from os.path import isdir
from glob import glob

priority = 60000
retries = 60
REGEX = compile(r'\[([A-Za-z-]+)][^\[\]]*? enabled = true')
ORDER = ['jails_group']


class Service(LogService):
    def __init__(self, configuration=None, name=None):
        LogService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.log_path = self.configuration.get('log_path', '/var/log/fail2ban.log')
        self.conf_path = self.configuration.get('conf_path', '/etc/fail2ban/jail.local')
        self.conf_dir = self.configuration.get('conf_dir', '')
        try:
            self.exclude = self.configuration['exclude'].split()
        except (KeyError, AttributeError):
            self.exclude = []

    def _get_data(self):
        """
        Parse new log lines
        :return: dict
        """
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
        data = dict(
            zip(
                self.jails_list,
                [len(list(filterfalse(lambda line: (jail + '] Ban') not in line, raw))) for jail in self.jails_list]
            ))

        for jail in data:
            self.data[jail] += data[jail]

        return self.data

    def check(self):

        # Check "log_path" is accessible.
        # If NOT STOP plugin
        if not is_accessible(self.log_path, R_OK):
            self.error('Cannot access file %s' % self.log_path)
            return False
        if not isdir(self.conf_dir):
            self.conf_dir = None

        # If "conf_dir" not specified (or not a dir) plugin will use "conf_path"
        if not self.conf_dir:
            if is_accessible(self.conf_path, R_OK):
                with open(self.conf_path, 'rt') as jails_conf:
                    jails_list = REGEX.findall(' '.join(jails_conf.read().split()))
                self.jails_list = jails_list
            else:
                self.jails_list = list()
                self.error('Cannot access jail configuration file %s.' % self.conf_path)
        # If "conf_dir" is specified and "conf_dir" is dir plugin will use "conf_dir"
        else:
            dot_local = glob(self.conf_dir + '/*.local')  # *.local jail configurations files
            dot_conf = glob(self.conf_dir + '/*.conf')  # *.conf jail configuration files

            if not any([dot_local, dot_conf]):
                self.error('%s is empty or not readable' % self.conf_dir)
            # According "man jail.conf" files could be *.local AND *.conf
            # *.conf files parsed first. Changes in *.local overrides configuration in *.conf
            if dot_conf:
                dot_local.extend([conf for conf in dot_conf if conf[:-5] not in [local[:-6] for local in dot_local]])
            # Make sure all files are readable
            dot_local = [conf for conf in dot_local if is_accessible(conf, R_OK)]
            if dot_local:
                enabled_jails = list()
                for jail_conf in dot_local:
                    with open(jail_conf, 'rt') as conf:
                        enabled_jails.extend(REGEX.findall(' '.join(conf.read().split())))
                self.jails_list = list(set(enabled_jails))
            else:
                self.jails_list = list()
                self.error('Files in %s not readable' % self.conf_dir)

        # If for some reason parse failed we still can START with default jails_list.
        self.jails_list = list(set(self.jails_list) - set(self.exclude)) or ['ssh']
        self.data = dict([(jail, 0) for jail in self.jails_list])
        self.create_dimensions()
        self.info('Plugin successfully started. Jails: %s' % self.jails_list)
        return True

    def create_dimensions(self):
        self.definitions = {
            'jails_group': {'options': [None, "Jails ban statistics", "bans/s", 'Jails', 'jail.ban', 'line'],
                            'lines': []}}
        for jail in self.jails_list:
            self.definitions['jails_group']['lines'].append([jail, jail, 'incremental'])
