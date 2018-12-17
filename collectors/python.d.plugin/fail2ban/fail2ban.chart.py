# -*- coding: utf-8 -*-
# Description: fail2ban log netdata python.d module
# Author: l2isbad
# SPDX-License-Identifier: GPL-3.0-or-later

import re
import os

from collections import defaultdict
from glob import glob

from bases.FrameworkServices.LogService import LogService


ORDER = [
    'jails_bans',
    'jails_in_jail',
]


def charts(jails):
    """
    Chart definitions creating
    """

    ch = {
        ORDER[0]: {
                'options': [None, 'Jails Ban Rate', 'bans/s', 'bans', 'jail.bans', 'line'],
                'lines': []
        },
        ORDER[1]: {
                'options': [None, 'Banned IPs (since the last restart of netdata)', 'IPs', 'in jail',
                            'jail.in_jail', 'line'],
                'lines': []
        },
    }
    for jail in jails:
        dim = [
            jail,
            jail,
            'incremental',
        ]
        ch[ORDER[0]]['lines'].append(dim)

        dim = [
            '{0}_in_jail'.format(jail),
            jail,
            'absolute',
        ]
        ch[ORDER[1]]['lines'].append(dim)

    return ch


RE_JAILS = re.compile(r'\[([a-zA-Z0-9_-]+)\][^\[\]]+?enabled\s+= (true|false)')

# Example:
# 2018-09-12 11:45:53,715 fail2ban.actions[25029]: WARNING [ssh] Unban 195.201.88.33
# 2018-09-12 11:45:58,727 fail2ban.actions[25029]: WARNING [ssh] Ban 217.59.246.27
# 2018-09-12 11:45:58,727 fail2ban.actions[25029]: WARNING [ssh] Restore Ban 217.59.246.27
RE_DATA = re.compile(r'\[(?P<jail>[A-Za-z-_0-9]+)\] (?P<action>Unban|Ban|Restore Ban) (?P<ip>[a-f0-9.:]+)')

DEFAULT_JAILS = [
    'ssh',
]


class Service(LogService):
    def __init__(self, configuration=None, name=None):
        LogService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = dict()
        self.log_path = self.configuration.get('log_path', '/var/log/fail2ban.log')
        self.conf_path = self.configuration.get('conf_path', '/etc/fail2ban/jail.local')
        self.conf_dir = self.configuration.get('conf_dir', '/etc/fail2ban/jail.d/')
        self.exclude = self.configuration.get('exclude', str())
        self.monitoring_jails = list()
        self.banned_ips = defaultdict(set)
        self.data = dict()

    def check(self):
        """
        :return: bool
        """
        if not self.conf_path.endswith(('.conf', '.local')):
            self.error('{0} is a wrong conf path name, must be *.conf or *.local'.format(self.conf_path))
            return False

        if not os.access(self.log_path, os.R_OK):
            self.error('{0} is not readable'.format(self.log_path))
            return False

        if os.path.getsize(self.log_path) == 0:
            self.error('{0} is empty'.format(self.log_path))
            return False

        self.monitoring_jails = self.jails_auto_detection()
        for jail in self.monitoring_jails:
            self.data[jail] = 0
            self.data['{0}_in_jail'.format(jail)] = 0

        self.definitions = charts(self.monitoring_jails)
        self.info('monitoring jails: {0}'.format(self.monitoring_jails))

        return True

    def get_data(self):
        """
        :return: dict
        """
        raw = self._get_raw_data()

        if not raw:
            return None if raw is None else self.data

        for row in raw:
            match = RE_DATA.search(row)

            if not match:
                continue

            match = match.groupdict()

            if match['jail'] not in self.monitoring_jails:
                continue

            jail, action, ip = match['jail'], match['action'], match['ip']

            if action == 'Ban' or action == 'Restore Ban':
                self.data[jail] += 1
                if ip not in self.banned_ips[jail]:
                    self.banned_ips[jail].add(ip)
                    self.data['{0}_in_jail'.format(jail)] += 1
            else:
                if ip in self.banned_ips[jail]:
                    self.banned_ips[jail].remove(ip)
                    self.data['{0}_in_jail'.format(jail)] -= 1

        return self.data

    def get_files_from_dir(self, dir_path, suffix):
        """
        :return: list
        """
        if not os.path.isdir(dir_path):
            self.error('{0} is not a directory'.format(dir_path))
            return list()

        return glob('{0}/*.{1}'.format(self.conf_dir, suffix))

    def get_jails_from_file(self, file_path):
        """
        :return: list
        """
        if not os.access(file_path, os.R_OK):
            self.error('{0} is not readable or not exist'.format(file_path))
            return list()

        with open(file_path, 'rt') as f:
            lines = f.readlines()
            raw = ' '.join(line for line in lines if line.startswith(('[', 'enabled')))

        match = RE_JAILS.findall(raw)
        # Result: [('ssh', 'true'), ('dropbear', 'true'), ('pam-generic', 'true'), ...]

        if not match:
            self.debug('{0} parse failed'.format(file_path))
            return list()

        return match

    def jails_auto_detection(self):
        """
        :return: list

        Parses jail configuration files. Returns list of enabled jails.
        According man jail.conf parse order must be
        * jail.conf
        * jail.d/*.conf (in alphabetical order)
        * jail.local
        * jail.d/*.local (in alphabetical order)
        """
        jails_files, all_jails, active_jails = list(), list(), list()

        jails_files.append('{0}.conf'.format(self.conf_path.rsplit('.')[0]))
        jails_files.extend(self.get_files_from_dir(self.conf_dir, 'conf'))
        jails_files.append('{0}.local'.format(self.conf_path.rsplit('.')[0]))
        jails_files.extend(self.get_files_from_dir(self.conf_dir, 'local'))

        self.debug('config files to parse: {0}'.format(jails_files))

        for f in jails_files:
            all_jails.extend(self.get_jails_from_file(f))

        exclude = self.exclude.split()

        for name, status in all_jails:
            if name in exclude:
                continue

            if status == 'true' and name not in active_jails:
                active_jails.append(name)
            elif status == 'false' and name in active_jails:
                active_jails.remove(name)

        return active_jails or DEFAULT_JAILS
