# -*- coding: utf-8 -*-
# Description: fail2ban log netdata python.d module
# Author: l2isbad

import bisect

from glob import glob
from re import compile as r_compile
from os import access as is_accessible, R_OK
from os.path import isdir, getsize


from bases.FrameworkServices.LogService import LogService

priority = 60000
retries = 60
REGEX_JAILS = r_compile(r'\[([a-zA-Z0-9_-]+)\][^\[\]]+?enabled\s+= (true|false)')
REGEX_DATA = r_compile(r'\[(?P<jail>[A-Za-z-_0-9]+)\] (?P<action>U|B)[a-z]+ (?P<ipaddr>\d{1,3}(?:\.\d{1,3}){3})')
ORDER = ['jails_bans', 'jails_in_jail']


class Service(LogService):
    """
    fail2ban log class
    Reads logs line by line
    Jail auto detection included
    It produces following charts:
    * Bans per second for every jail
    * Banned IPs for every jail (since the last restart of netdata)
    """
    def __init__(self, configuration=None, name=None):
        LogService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = dict()
        self.log_path = self.configuration.get('log_path', '/var/log/fail2ban.log')
        self.conf_path = self.configuration.get('conf_path', '/etc/fail2ban/jail.local')
        self.conf_dir = self.configuration.get('conf_dir', '/etc/fail2ban/jail.d/')
        self.exclude = self.configuration.get('exclude')

    def _get_data(self):
        """
        Parse new log lines
        :return: dict
        """
        raw = self._get_raw_data()
        if raw is None:
            return None
        elif not raw:
            return self.to_netdata

        # Fail2ban logs looks like
        # 2016-12-25 12:36:04,711 fail2ban.actions[2455]: WARNING [ssh] Ban 178.156.32.231
        for row in raw:
            match = REGEX_DATA.search(row)
            if match:
                match_dict = match.groupdict()
                jail, action, ipaddr = match_dict['jail'], match_dict['action'], match_dict['ipaddr']
                if jail in self.jails_list:
                    if action == 'B':
                        self.to_netdata[jail] += 1
                        if address_not_in_jail(self.banned_ips[jail], ipaddr, self.to_netdata[jail + '_in_jail']):
                            self.to_netdata[jail + '_in_jail'] += 1
                    else:
                        if ipaddr in self.banned_ips[jail]:
                            self.banned_ips[jail].remove(ipaddr)
                            self.to_netdata[jail + '_in_jail'] -= 1

        return self.to_netdata

    def check(self):
        """
        :return: bool

        Check if the "log_path" is not empty and readable
        """

        if not (is_accessible(self.log_path, R_OK) and getsize(self.log_path) != 0):
            self.error('%s is not readable or empty' % self.log_path)
            return False
        self.jails_list, self.to_netdata, self.banned_ips = self.jails_auto_detection_()
        self.definitions = create_definitions_(self.jails_list)
        self.info('Jails: %s' % self.jails_list)
        return True

    def jails_auto_detection_(self):
        """
        return: <tuple>

        * jails_list - list of enabled jails (['ssh', 'apache', ...])
        * to_netdata - dict ({'ssh': 0, 'ssh_in_jail': 0, ...})
        * banned_ips - here will be stored all the banned ips ({'ssh': ['1.2.3.4', '5.6.7.8', ...], ...})
        """
        raw_jails_list = list()
        jails_list = list()

        for raw_jail in parse_configuration_files_(self.conf_path, self.conf_dir, self.error):
            raw_jails_list.extend(raw_jail)

        for jail, status in raw_jails_list:
            if status == 'true' and jail not in jails_list:
                jails_list.append(jail)
            elif status == 'false' and jail in jails_list:
                jails_list.remove(jail)
        # If for some reason parse failed we still can START with default jails_list.
        jails_list = list(set(jails_list) - set(self.exclude.split()
                                                if isinstance(self.exclude, str) else list())) or ['ssh']

        to_netdata = dict([(jail, 0) for jail in jails_list])
        to_netdata.update(dict([(jail + '_in_jail', 0) for jail in jails_list]))
        banned_ips = dict([(jail, list()) for jail in jails_list])

        return jails_list, to_netdata, banned_ips


def create_definitions_(jails_list):
    """
    Chart definitions creating
    """

    definitions = {
        'jails_bans': {'options': [None, 'Jails Ban Statistics', 'bans/s', 'bans', 'jail.bans', 'line'],
                       'lines': []},
        'jails_in_jail': {'options': [None, 'Banned IPs (since the last restart of netdata)', 'IPs',
                                      'in jail', 'jail.in_jail', 'line'],
                          'lines': []}}
    for jail in jails_list:
        definitions['jails_bans']['lines'].append([jail, jail, 'incremental'])
        definitions['jails_in_jail']['lines'].append([jail + '_in_jail', jail, 'absolute'])

    return definitions


def parse_configuration_files_(jails_conf_path, jails_conf_dir, print_error):
    """
    :param jails_conf_path: <str>
    :param jails_conf_dir: <str>
    :param print_error: <function>
    :return: <tuple>

    Uses "find_jails_in_files" function to find all jails in the "jails_conf_dir" directory
    and in the "jails_conf_path"

    All files must endswith ".local" or ".conf"
    Return order is important.
    According man jail.conf it should be
    * jail.conf
    * jail.d/*.conf (in alphabetical order)
    * jail.local
    * jail.d/*.local (in alphabetical order)
    """
    path_conf, path_local, dir_conf, dir_local = list(), list(), list(), list()

    # Parse files in the directory
    if not (isinstance(jails_conf_dir, str) and isdir(jails_conf_dir)):
        print_error('%s is not a directory' % jails_conf_dir)
    else:
        dir_conf = list(filter(lambda conf: is_accessible(conf, R_OK), glob(jails_conf_dir + '/*.conf')))
        dir_local = list(filter(lambda local: is_accessible(local, R_OK), glob(jails_conf_dir + '/*.local')))
        if not (dir_conf or dir_local):
            print_error('%s is empty or not readable' % jails_conf_dir)
        else:
            dir_conf, dir_local = (find_jails_in_files(dir_conf, print_error),
                                   find_jails_in_files(dir_local, print_error))

    # Parse .conf and .local files
    if isinstance(jails_conf_path, str) and jails_conf_path.endswith(('.local', '.conf')):
        path_conf, path_local = (find_jails_in_files([jails_conf_path.split('.')[0] + '.conf'], print_error),
                                 find_jails_in_files([jails_conf_path.split('.')[0] + '.local'], print_error))

    return path_conf, dir_conf, path_local, dir_local


def find_jails_in_files(list_of_files, print_error):
    """
    :param list_of_files: <list>
    :param print_error: <function>
    :return: <list>

    Open a file and parse it to find all (enabled and disabled) jails
    The output is a list of tuples:
    [('ssh', 'true'), ('apache', 'false'), ...]
    """
    jails_list = list()
    for conf in list_of_files:
        if is_accessible(conf, R_OK):
            with open(conf, 'rt') as f:
                raw_data = f.readlines()
            data = ' '.join(line for line in raw_data if line.startswith(('[', 'enabled')))
            jails_list.extend(REGEX_JAILS.findall(data))
        else:
            print_error('%s is not readable or not exist' % conf)
    return jails_list


def address_not_in_jail(pool, address, pool_size):
    """
    :param pool: <list>
    :param address: <str>
    :param pool_size: <int>
    :return: bool

    Checks if the address is in the pool.
    If not address will be added
    """
    index = bisect.bisect_left(pool, address)
    if index < pool_size:
        if pool[index] == address:
            return False
        bisect.insort_left(pool, address)
        return True
    else:
        bisect.insort_left(pool, address)
        return True
