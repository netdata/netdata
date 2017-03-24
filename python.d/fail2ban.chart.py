# -*- coding: utf-8 -*-
# Description: fail2ban log netdata python.d module
# Author: l2isbad

from base import LogService
from re import compile as r_compile

try:
    from itertools import filterfalse
except ImportError:
    from itertools import ifilterfalse as filterfalse
from os import access as is_accessible, R_OK
from os.path import isdir
from glob import glob
import bisect

priority = 60000
retries = 60
REGEX_JAILS = r_compile(r'\[([A-Za-z-_]+)][^\[\]]*?(?<!# )enabled = true')
REGEX_DATA = r_compile(r'\[(?P<jail>[a-z]+)\] (?P<ban>[A-Z])[a-z]+ (?P<ipaddr>\d{1,3}(?:\.\d{1,3}){3})')
ORDER = ['jails_bans', 'jails_in_jail']


class Service(LogService):
    def __init__(self, configuration=None, name=None):
        LogService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.log_path = self.configuration.get('log_path', '/var/log/fail2ban.log')
        self.conf_path = self.configuration.get('conf_path', '/etc/fail2ban/jail.local')
        self.conf_dir = self.configuration.get('conf_dir', '')
        self.bans = dict()
        try:
            self.exclude = self.configuration['exclude'].split()
        except (KeyError, AttributeError):
            self.exclude = list()

    def _get_data(self):
        """
        Parse new log lines
        :return: dict
        """
        raw = self._get_raw_data()
        if raw is None:
            return None
        elif not raw:
            return self.data

        # Fail2ban logs looks like
        # 2016-12-25 12:36:04,711 fail2ban.actions[2455]: WARNING [ssh] Ban 178.156.32.231
        for row in raw:
            match = REGEX_DATA.search(row)
            if match:
                match_dict = match.groupdict()
                jail, ban, ipaddr = match_dict['jail'], match_dict['ban'], match_dict['ipaddr']
                if jail in self.jails_list:
                    if ban == 'B':
                        self.data[jail] += 1
                        if address_not_in_jail(self.bans[jail], ipaddr, self.data[jail + '_in_jail']):
                           self.data[jail + '_in_jail'] += 1
                    else:
                        if ipaddr in self.bans[jail]:
                            self.bans[jail].remove(ipaddr)
                            self.data[jail + '_in_jail'] -= 1

        return self.data

    def check(self):

        # Check "log_path" is accessible.
        # If NOT STOP plugin
        if not is_accessible(self.log_path, R_OK):
            self.error('Cannot access file %s' % self.log_path)
            return False
        jails_list = list()

        if self.conf_dir:
            dir_jails, error = parse_conf_dir(self.conf_dir)
            jails_list.extend(dir_jails)
            if not dir_jails:
                self.error(error)

        if self.conf_path:
            path_jails, error = parse_conf_path(self.conf_path)
            jails_list.extend(path_jails)
            if not path_jails:
                self.error(error)

        # If for some reason parse failed we still can START with default jails_list.
        self.jails_list = list(set(jails_list) - set(self.exclude)) or ['ssh']

        self.data = dict([(jail, 0) for jail in self.jails_list])
        self.data.update(dict([(jail + '_in_jail', 0) for jail in self.jails_list]))
        self.bans = dict([(jail, list()) for jail in self.jails_list])

        self.create_dimensions()
        self.info('Plugin successfully started. Jails: %s' % self.jails_list)
        return True

    def create_dimensions(self):
        self.definitions = {
            'jails_bans': {'options': [None, "Jails Ban Statistics", "bans/s", 'bans', 'jail.bans', 'line'],
                            'lines': []},
            'jails_in_jail': {'options': [None, "Currently In Jail", "ip addresses", 'in jail', 'jail.in_jail', 'line'],
                            'lines': []},
                           }
        for jail in self.jails_list:
            self.definitions['jails_bans']['lines'].append([jail, jail, 'incremental'])
            self.definitions['jails_in_jail']['lines'].append([jail + '_in_jail', jail, 'absolute'])


def parse_conf_dir(conf_dir):
    if not isdir(conf_dir):
        return list(), '%s is not a directory' % conf_dir

    jail_local = list(filter(lambda local: is_accessible(local, R_OK), glob(conf_dir + '/*.local')))
    jail_conf = list(filter(lambda conf: is_accessible(conf, R_OK), glob(conf_dir + '/*.conf')))

    if not (jail_local or jail_conf):
        return list(), '%s is empty or not readable' % conf_dir

    # According "man jail.conf" files could be *.local AND *.conf
    # *.conf files parsed first. Changes in *.local overrides configuration in *.conf
    if jail_conf:
        jail_local.extend([conf for conf in jail_conf if conf[:-5] not in [local[:-6] for local in jail_local]])
    jails_list = list()
    for conf in jail_local:
        with open(conf, 'rt') as f:
            raw_data = f.read()

        data = ' '.join(raw_data.split())
        jails_list.extend(REGEX_JAILS.findall(data))
    jails_list = list(set(jails_list))

    return jails_list, 'can\'t locate any jails in %s. Default jail is [\'ssh\']' % conf_dir


def parse_conf_path(conf_path):
    if not is_accessible(conf_path, R_OK):
        return list(), '%s is not readable' % conf_path

    with open(conf_path, 'rt') as jails_conf:
        raw_data = jails_conf.read()

    data = raw_data.split()
    jails_list = REGEX_JAILS.findall(' '.join(data))
    return jails_list, 'can\'t locate any jails in %s. Default jail is  [\'ssh\']' % conf_path


def address_not_in_jail(pool, address, pool_size):
    index = bisect.bisect_left(pool, address)
    if index < pool_size:
        if pool[index] == address:
            return False
        else:
            bisect.insort_left(pool, address)
            return True
    else:
        bisect.insort_left(pool, address)
        return True
