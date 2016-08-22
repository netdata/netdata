# -*- coding: utf-8 -*-
# Description: dovecot netdata python.d module
# Author: Pawel Krupa (paulfantom)

from base import SocketService

# default module values (can be overridden per job in `config`)
# update_every = 2
priority = 60000
retries = 60

# charts order (can be overridden if you want less charts, or different order)
ORDER = ['sessions', 'commands',
         'faults',
         'context_switches',
         'disk', 'bytes', 'syscalls',
         'lookup', 'cache',
         'auth', 'auth_cache']

CHARTS = {
    'sessions': {
        'options': [None, "logins and sessions", 'number', 'IMAP', 'dovecot.sessions', 'line'],
        'lines': [
            ['num_logins', 'logins', 'absolute'],
            ['num_connected_sessions', 'active sessions', 'absolute']
        ]},
    'commands': {
        'options': [None, "commands", "commands", 'IMAP', 'dovecot.commands', 'line'],
        'lines': [
            ['num_cmds', 'commands', 'absolute']
        ]},
    'faults': {
        'options': [None, "faults", "faults", 'Faults', 'dovecot.faults', 'line'],
        'lines': [
            ['min_faults', 'minor', 'absolute'],
            ['maj_faults', 'major', 'absolute']
        ]},
    'context_switches': {
        'options': [None, "context switches", '', 'Context Switches', 'dovecot.context_switches', 'line'],
        'lines': [
            ['vol_cs', 'volountary', 'absolute'],
            ['invol_cs', 'involountary', 'absolute']
        ]},
    'disk': {
        'options': [None, "disk", 'bytes/s', 'Reads and Writes', 'dovecot.disk', 'line'],
        'lines': [
            ['disk_input', 'read', 'incremental'],
            ['disk_output', 'write', 'incremental']
        ]},
    'bytes': {
        'options': [None, "bytes", 'bytes/s', 'Reads and Writes', 'dovecot.bytes', 'line'],
        'lines': [
            ['read_bytes', 'read', 'incremental'],
            ['write_bytes', 'write', 'incremental']
        ]},
    'syscalls': {
        'options': [None, "number of syscalls", 'syscalls/s', 'Reads and Writes', 'dovecot.syscalls', 'line'],
        'lines': [
            ['read_count', 'read', 'incremental'],
            ['write_count', 'write', 'incremental']
        ]},
    'lookup': {
        'options': [None, "lookups", 'number/s', 'Mail', 'dovecot.lookup', 'line'],
        'lines': [
            ['mail_lookup_path', 'path', 'incremental'],
            ['mail_lookup_attr', 'attr', 'incremental']
        ]},
    'cache': {
        'options': [None, "hits", 'hits/s', 'Mail', 'dovecot.cache', 'line'],
        'lines': [
            ['mail_cache_hits', 'hits', 'incremental']
        ]},
    'auth': {
        'options': [None, "attempts", 'attempts', 'Authentication', 'dovecot.auth', 'stacked'],
        'lines': [
            ['auth_successes', 'success', 'absolute'],
            ['auth_failures', 'failure', 'absolute']
        ]},
    'auth_cache': {
        'options': [None, "cache", 'number', 'Authentication', 'dovecot.auth_cache', 'stacked'],
        'lines': [
            ['auth_cache_hits', 'hit', 'absolute'],
            ['auth_cache_misses', 'miss', 'absolute']
        ]}
}


class Service(SocketService):
    def __init__(self, configuration=None, name=None):
        SocketService.__init__(self, configuration=configuration, name=name)
        self.request = "EXPORT\tglobal\r\n"
        self.host = None  # localhost
        self.port = None  # 24242
        # self._keep_alive = True
        self.unix_socket = "/var/run/dovecot/stats"
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

        data = raw.split('\n')[:2]
        desc = data[0].split('\t')
        vals = data[1].split('\t')
        # ret = dict(zip(desc, vals))
        ret = {}
        for i in range(len(desc)):
            try:
                #d = str(desc[i])
                #if d in ('user_cpu', 'sys_cpu', 'clock_time'):
                #    val = float(vals[i])
                #else:
                #    val = int(vals[i])
                #ret[d] = val
                ret[str(desc[i])] = int(vals[i])
            except ValueError:
                pass
        if len(ret) == 0:
            return None
        return ret
