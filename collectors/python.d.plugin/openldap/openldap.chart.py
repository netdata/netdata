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

# default module values (can be overridden per job in `config`)
update_every = 10
priority = 60000
retries = 60

ORDER = [
    'total_connections', 'bytes_sent',
    'operations', 'referrals_sent',
    'entries_sent', 'ldap_operations', 'waiters'
]

CHARTS = {
  
     'total_connections': {
        'options': [None, 'Total Connections', 'connections', 'ldap', 'monitorCounter', 'line'],
        'lines': [
            ['total_connections', None, 'incremental']
        ]
    },
  
    'bytes_sent': {
        'options': [None, 'Bytes Statistics', 'Bytes', 'ldap', 'monitorCounter', 'line'],
        'lines': [
            ['bytes_sent', None, 'incremental']
        ]
    },
    'operations': {
        'options': [None, 'Operations', 'ops', 'ldap', 'monitorCounter', 'line'],
        'lines': [
            ['completed_operations', None, 'incremental'],
            ['initiated_operations', None, 'incremental']
        ]
    },
    'referrals_sent': {
        'options': [None, 'Referrals Statistics', 'referalls_sent', 'ldap', 'monitorCounter', 'line'],
        'lines': [
            ['referrals_sent', None, 'incremental']
        ]
    },
    'entries_sent': {
        'options': [None, 'Entries Statistics', 'entries_sent', 'ldap', 'monitorCounter', 'line'],
        'lines': [
            ['entries_sent', None, 'incremental']
        ]
    },
    'ldap_operations': {
        'options': [None, 'Operations', 'bind_operations', 'ldap', 'monitorCounter', 'line'],
        'lines': [
            ['bind_operations', None, 'incremental'],
            ['search_operations', None, 'incremental'],
            ['unbind_operations', None, 'incremental'],
            ['add_operations', None, 'incremental'],
            ['delete_operations', None, 'incremental'],
            ['modify_operations', None, 'incremental'],
            ['compare_operations', None, 'incremental']
        ]
    },
    'waiters': {
        'options': [None, 'Waiters', 'amount', 'ldap', 'monitorCounter', 'line'],
        'lines': [
            ['write_waiters', None, 'incremental'],
            ['read_waiters', None, 'incremental']
        ]
    },
}

# Stuff to gather - make tuples of DN dn and attrib to get
searchlist = {
    'total_connections':('cn=Total,cn=Connections,cn=Monitor', 'monitorCounter'),
    'bytes_sent': ('cn=Bytes,cn=Statistics,cn=Monitor', 'monitorCounter'),
    'completed_operations': ('cn=Operations,cn=Monitor', 'monitorOpCompleted'),
    'initiated_operations': ('cn=Operations,cn=Monitor', 'monitorOpInitiated'),
    'referrals_sent': ('cn=Referrals,cn=Statistics,cn=Monitor', 'monitorCounter'),
    'entries_sent': ('cn=Entries,cn=Statistics,cn=Monitor', 'monitorCounter'),
    'bind_operations': ('cn=Bind,cn=Operations,cn=Monitor', 'monitorOpCompleted'),
    'unbind_operations': ('cn=Unbind,cn=Operations,cn=Monitor', 'monitorOpCompleted'),
    'add_operations': ('cn=Add,cn=Operations,cn=Monitor', 'monitorOpInitiated'),
    'delete_operations':  ('cn=Delete,cn=Operations,cn=Monitor', 'monitorOpCompleted'),
    'modify_operations': ('cn=Modify,cn=Operations,cn=Monitor', 'monitorOpCompleted'),
    'compare_operations': ('cn=Compare,cn=Operations,cn=Monitor', 'monitorOpCompleted'),
    'search_operations': ('cn=Search,cn=Operations,cn=Monitor', 'monitorOpCompleted'),
    'write_waiters': ('cn=Write,cn=Waiters,cn=Monitor', 'monitorCounter'),
    'read_waiters': ('cn=Read,cn=Waiters,cn=Monitor', 'monitorCounter'),
}

class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS

        self.server = configuration.get('server')
        self.port = configuration.get('port')
        self.username = configuration.get('username')
        self.password = configuration.get('password')
        
    def connect(self):
        try: 
            print self.server
            self.conn = ldap.initialize('ldap://%s:%s' % ( self.server, self.port))
            self.conn.simple_bind(self.username, self.password)
            return True
        except:
            return False

    def check(self):
        if not HAS_LDAP: 
            self.error('\'ldap\' python module is needed to use openldap.chart.py') 
            return False

        self.connect()
        if self.conn:
            return True
        else:
            self.error('Could not establish ldap connection. Check your openldap.conf')
            return False

    def get_data(self):
        data = dict()
        for key in searchlist.keys():
            bind = searchlist[key][0]
            attr = searchlist[key][1]
            num = self.conn.search(bind, ldap.SCOPE_BASE, 'objectClass=*', [attr,])
            try:
                result_type, result_data = self.conn.result(num, 1)
                if result_type == 101:
                    val = int(result_data[0][1].values()[0][0])
                    data[key] = val
            except Exception as e:
                return repr(e)
        return data

