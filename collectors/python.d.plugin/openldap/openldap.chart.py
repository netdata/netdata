# -*- coding: utf-8 -*-
# Description: openldap netdata python.d module
# Author: Manolis Kartsonakis (ekartsonakis)
# SPDX-License-Identifier: GPL-3.0+

try:
    import ldap

    HAS_LDAP = True
except ImportError:
    HAS_LDAP = False

from bases.FrameworkServices.SimpleService import SimpleService

DEFAULT_SERVER = 'localhost'
DEFAULT_PORT = '389'
DEFAULT_TLS = False
DEFAULT_CERT_CHECK = True
DEFAULT_TIMEOUT = 1

ORDER = [
    'total_connections',
    'bytes_sent',
    'operations',
    'referrals_sent',
    'entries_sent',
    'ldap_operations',
    'waiters'
]

CHARTS = {
    'total_connections': {
        'options': [None, 'Total Connections', 'connections/s', 'ldap', 'openldap.total_connections', 'line'],
        'lines': [
            ['total_connections', 'connections', 'incremental']
        ]
    },
    'bytes_sent': {
        'options': [None, 'Traffic', 'KiB/s', 'ldap', 'openldap.traffic_stats', 'line'],
        'lines': [
            ['bytes_sent', 'sent', 'incremental', 1, 1024]
        ]
    },
    'operations': {
        'options': [None, 'Operations Status', 'ops/s', 'ldap', 'openldap.operations_status', 'line'],
        'lines': [
            ['completed_operations', 'completed', 'incremental'],
            ['initiated_operations', 'initiated', 'incremental']
        ]
    },
    'referrals_sent': {
        'options': [None, 'Referrals', 'referals/s', 'ldap', 'openldap.referrals', 'line'],
        'lines': [
            ['referrals_sent', 'sent', 'incremental']
        ]
    },
    'entries_sent': {
        'options': [None, 'Entries', 'entries/s', 'ldap', 'openldap.entries', 'line'],
        'lines': [
            ['entries_sent', 'sent', 'incremental']
        ]
    },
    'ldap_operations': {
        'options': [None, 'Operations', 'ops/s', 'ldap', 'openldap.ldap_operations', 'line'],
        'lines': [
            ['bind_operations', 'bind', 'incremental'],
            ['search_operations', 'search', 'incremental'],
            ['unbind_operations', 'unbind', 'incremental'],
            ['add_operations', 'add', 'incremental'],
            ['delete_operations', 'delete', 'incremental'],
            ['modify_operations', 'modify', 'incremental'],
            ['compare_operations', 'compare', 'incremental']
        ]
    },
    'waiters': {
        'options': [None, 'Waiters', 'waiters/s', 'ldap', 'openldap.waiters', 'line'],
        'lines': [
            ['write_waiters', 'write', 'incremental'],
            ['read_waiters', 'read', 'incremental']
        ]
    },
}

# Stuff to gather - make tuples of DN dn and attrib to get
SEARCH_LIST = {
    'total_connections': (
        'cn=Total,cn=Connections,cn=Monitor', 'monitorCounter',
    ),
    'bytes_sent': (
        'cn=Bytes,cn=Statistics,cn=Monitor', 'monitorCounter',
    ),
    'completed_operations': (
        'cn=Operations,cn=Monitor', 'monitorOpCompleted',
    ),
    'initiated_operations': (
        'cn=Operations,cn=Monitor', 'monitorOpInitiated',
    ),
    'referrals_sent': (
        'cn=Referrals,cn=Statistics,cn=Monitor', 'monitorCounter',
    ),
    'entries_sent': (
        'cn=Entries,cn=Statistics,cn=Monitor', 'monitorCounter',
    ),
    'bind_operations': (
        'cn=Bind,cn=Operations,cn=Monitor', 'monitorOpCompleted',
    ),
    'unbind_operations': (
        'cn=Unbind,cn=Operations,cn=Monitor', 'monitorOpCompleted',
    ),
    'add_operations': (
        'cn=Add,cn=Operations,cn=Monitor', 'monitorOpInitiated',
    ),
    'delete_operations': (
        'cn=Delete,cn=Operations,cn=Monitor', 'monitorOpCompleted',
    ),
    'modify_operations': (
        'cn=Modify,cn=Operations,cn=Monitor', 'monitorOpCompleted',
    ),
    'compare_operations': (
        'cn=Compare,cn=Operations,cn=Monitor', 'monitorOpCompleted',
    ),
    'search_operations': (
        'cn=Search,cn=Operations,cn=Monitor', 'monitorOpCompleted',
    ),
    'write_waiters': (
        'cn=Write,cn=Waiters,cn=Monitor', 'monitorCounter',
    ),
    'read_waiters': (
        'cn=Read,cn=Waiters,cn=Monitor', 'monitorCounter',
    ),
}


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.server = configuration.get('server', DEFAULT_SERVER)
        self.port = configuration.get('port', DEFAULT_PORT)
        self.username = configuration.get('username')
        self.password = configuration.get('password')
        self.timeout = configuration.get('timeout', DEFAULT_TIMEOUT)
        self.use_tls = configuration.get('use_tls', DEFAULT_TLS)
        self.cert_check = configuration.get('cert_check', DEFAULT_CERT_CHECK)
        self.alive = False
        self.conn = None

    def disconnect(self):
        if self.conn:
            self.conn.unbind()
            self.conn = None
            self.alive = False

    def connect(self):
        try:
            if self.use_tls:
                self.conn = ldap.initialize('ldaps://%s:%s' % (self.server, self.port))
            else:
                self.conn = ldap.initialize('ldap://%s:%s' % (self.server, self.port))
            self.conn.set_option(ldap.OPT_NETWORK_TIMEOUT, self.timeout)
            if self.use_tls and not self.cert_check:
                self.conn.set_option(ldap.OPT_X_TLS_REQUIRE_CERT, ldap.OPT_X_TLS_NEVER)
            if self.username and self.password:
                self.conn.simple_bind(self.username, self.password)
        except ldap.LDAPError as error:
            self.error(error)
            return False

        self.alive = True
        return True

    def reconnect(self):
        self.disconnect()
        return self.connect()

    def check(self):
        if not HAS_LDAP:
            self.error("'python-ldap' package is needed")
            return None

        return self.connect() and self.get_data()

    def get_data(self):
        if not self.alive and not self.reconnect():
            return None

        data = dict()
        for key in SEARCH_LIST:
            dn = SEARCH_LIST[key][0]
            attr = SEARCH_LIST[key][1]
            try:
                num = self.conn.search(dn, ldap.SCOPE_BASE, 'objectClass=*', [attr, ])
                result_type, result_data = self.conn.result(num, 1)
            except ldap.LDAPError as error:
                self.error("Empty result. Check bind username/password. Message: ", error)
                self.alive = False
                return None

            try:
                if result_type == 101:
                    val = int(result_data[0][1].values()[0][0])
            except (ValueError, IndexError) as error:
                self.debug(error)
                continue

            data[key] = val

        return data
