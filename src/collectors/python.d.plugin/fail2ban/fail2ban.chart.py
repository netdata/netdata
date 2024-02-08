# -*- coding: utf-8 -*-
# Description: fail2ban log netdata python.d module
# Author: ilyam8
# SPDX-License-Identifier: GPL-3.0-or-later

import os
import re
from collections import defaultdict
from glob import glob

from bases.FrameworkServices.LogService import LogService

ORDER = [
    'jails_failed_attempts',
    'jails_bans',
    'jails_banned_ips',
]


def charts(jails):
    """
    Chart definitions creating
    """

    ch = {
        ORDER[0]: {
            'options': [None, 'Failed attempts', 'attempts/s', 'failed attempts', 'fail2ban.failed_attempts', 'line'],
            'lines': []
        },
        ORDER[1]: {
            'options': [None, 'Bans', 'bans/s', 'bans', 'fail2ban.bans', 'line'],
            'lines': []
        },
        ORDER[2]: {
            'options': [None, 'Banned IP addresses (since the last restart of netdata)', 'ips', 'banned ips',
                        'fail2ban.banned_ips', 'line'],
            'lines': []
        },
    }
    for jail in jails:
        dim = ['{0}_failed_attempts'.format(jail), jail, 'incremental']
        ch[ORDER[0]]['lines'].append(dim)

        dim = [jail, jail, 'incremental']
        ch[ORDER[1]]['lines'].append(dim)

        dim = ['{0}_in_jail'.format(jail), jail, 'absolute']
        ch[ORDER[2]]['lines'].append(dim)

    return ch


RE_JAILS = re.compile(r'\[([a-zA-Z0-9_-]+)\][^\[\]]+?enabled\s+= +(true|yes|false|no)')

ACTION_BAN = 'Ban'
ACTION_UNBAN = 'Unban'
ACTION_RESTORE_BAN = 'Restore Ban'
ACTION_FOUND = 'Found'

# Example:
# 2018-09-12 11:45:58,727 fail2ban.actions[25029]: WARNING [ssh] Found 203.0.113.1
# 2018-09-12 11:45:58,727 fail2ban.actions[25029]: WARNING [ssh] Ban 203.0.113.1
# 2018-09-12 11:45:58,727 fail2ban.actions[25029]: WARNING [ssh] Restore Ban 203.0.113.1
# 2018-09-12 11:45:53,715 fail2ban.actions[25029]: WARNING [ssh] Unban 203.0.113.1
RE_DATA = re.compile(
    r'\[(?P<jail>[A-Za-z-_0-9]+)\] (?P<action>{0}|{1}|{2}|{3}) (?P<ip>[a-f0-9.:]+)'.format(
        ACTION_BAN, ACTION_UNBAN, ACTION_RESTORE_BAN, ACTION_FOUND
    )
)

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
            self.data['{0}_failed_attempts'.format(jail)] = 0
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

            if action == ACTION_FOUND:
                self.data['{0}_failed_attempts'.format(jail)] += 1
            elif action in (ACTION_BAN, ACTION_RESTORE_BAN):
                self.data[jail] += 1
                if ip not in self.banned_ips[jail]:
                    self.banned_ips[jail].add(ip)
                    self.data['{0}_in_jail'.format(jail)] += 1
            elif action == ACTION_UNBAN:
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

            if status in ('true', 'yes') and name not in active_jails:
                active_jails.append(name)
            elif status in ('false', 'no') and name in active_jails:
                active_jails.remove(name)

        return active_jails or DEFAULT_JAILS
