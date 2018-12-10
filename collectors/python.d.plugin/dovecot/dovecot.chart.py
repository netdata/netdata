# -*- coding: utf-8 -*-
# Description: dovecot netdata python.d module
# Author: Pawel Krupa (paulfantom)
# SPDX-License-Identifier: GPL-3.0-or-later

from bases.FrameworkServices.SocketService import SocketService

# default module values (can be overridden per job in `config`)
# update_every = 2
priority = 60000

# charts order (can be overridden if you want less charts, or different order)
ORDER = [
    'sessions',
    'logins',
    'commands',
    'faults',
    'context_switches',
    'io',
    'net',
    'syscalls',
    'lookup',
    'cache',
    'auth',
    'auth_cache'
]

CHARTS = {
    'sessions': {
        'options': [None, 'Dovecot Active Sessions', 'number', 'sessions', 'dovecot.sessions', 'line'],
        'lines': [
            ['num_connected_sessions', 'active sessions', 'absolute']
        ]
    },
    'logins': {
        'options': [None, 'Dovecot Logins', 'number', 'logins', 'dovecot.logins', 'line'],
        'lines': [
            ['num_logins', 'logins', 'absolute']
        ]
    },
    'commands': {
        'options': [None, 'Dovecot Commands', 'commands', 'commands', 'dovecot.commands', 'line'],
        'lines': [
            ['num_cmds', 'commands', 'absolute']
        ]
    },
    'faults': {
        'options': [None, 'Dovecot Page Faults', 'faults', 'page faults', 'dovecot.faults', 'line'],
        'lines': [
            ['min_faults', 'minor', 'absolute'],
            ['maj_faults', 'major', 'absolute']
        ]
    },
    'context_switches': {
        'options': [None, 'Dovecot Context Switches', '', 'context switches', 'dovecot.context_switches', 'line'],
        'lines': [
            ['vol_cs', 'voluntary', 'absolute'],
            ['invol_cs', 'involuntary', 'absolute']
        ]
    },
    'io': {
        'options': [None, 'Dovecot Disk I/O', 'kilobytes/s', 'disk', 'dovecot.io', 'area'],
        'lines': [
            ['disk_input', 'read', 'incremental', 1, 1024],
            ['disk_output', 'write', 'incremental', -1, 1024]
        ]
    },
    'net': {
        'options': [None, 'Dovecot Network Bandwidth', 'kilobits/s', 'network', 'dovecot.net', 'area'],
        'lines': [
            ['read_bytes', 'read', 'incremental', 8, 1024],
            ['write_bytes', 'write', 'incremental', -8, 1024]
        ]
    },
    'syscalls': {
        'options': [None, 'Dovecot Number of SysCalls', 'syscalls/s', 'system', 'dovecot.syscalls', 'line'],
        'lines': [
            ['read_count', 'read', 'incremental'],
            ['write_count', 'write', 'incremental']
        ]
    },
    'lookup': {
        'options': [None, 'Dovecot Lookups', 'number/s', 'lookups', 'dovecot.lookup', 'stacked'],
        'lines': [
            ['mail_lookup_path', 'path', 'incremental'],
            ['mail_lookup_attr', 'attr', 'incremental']
        ]
    },
    'cache': {
        'options': [None, 'Dovecot Cache Hits', 'hits/s', 'cache', 'dovecot.cache', 'line'],
        'lines': [
            ['mail_cache_hits', 'hits', 'incremental']
        ]
    },
    'auth': {
        'options': [None, 'Dovecot Authentications', 'attempts', 'logins', 'dovecot.auth', 'stacked'],
        'lines': [
            ['auth_successes', 'ok', 'absolute'],
            ['auth_failures', 'failed', 'absolute']
        ]
    },
    'auth_cache': {
        'options': [None, 'Dovecot Authentication Cache', 'number', 'cache', 'dovecot.auth_cache', 'stacked'],
        'lines': [
            ['auth_cache_hits', 'hit', 'absolute'],
            ['auth_cache_misses', 'miss', 'absolute']
        ]
    }
}


class Service(SocketService):
    def __init__(self, configuration=None, name=None):
        SocketService.__init__(self, configuration=configuration, name=name)
        self.request = 'EXPORT\tglobal\r\n'
        self.host = None  # localhost
        self.port = None  # 24242
        # self._keep_alive = True
        self.unix_socket = '/var/run/dovecot/stats'
        self.order = ORDER
        self.definitions = CHARTS

    def _get_data(self):
        """
        Format data received from socket
        :return: dict
        """
        try:
            raw = self._get_raw_data()
        except (ValueError, AttributeError):
            return None

        if raw is None:
            self.debug('dovecot returned no data')
            return None

        data = raw.split('\n')[:2]
        desc = data[0].split('\t')
        vals = data[1].split('\t')
        ret = dict()
        for i, _ in enumerate(desc):
            try:
                ret[str(desc[i])] = int(vals[i])
            except ValueError:
                continue
        return ret or None
