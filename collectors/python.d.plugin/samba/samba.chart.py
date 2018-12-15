# -*- coding: utf-8 -*-
# Description:  samba netdata python.d module
# Author: Christopher Cox <chris_cox@endlessnow.com>
# SPDX-License-Identifier: GPL-3.0-or-later
#
# The netdata user needs to be able to be able to sudo the smbstatus program
# without password:
# netdata ALL=(ALL)       NOPASSWD: /usr/bin/smbstatus -P
#
# This makes calls to smbstatus -P
#
# This just looks at a couple of values out of syscall, and some from smb2.
#
# The Lesser Ops chart is merely a display of current counter values.  They
# didn't seem to change much to me.  However, if you notice something changing
# a lot there, bring one or more out into its own chart and make it incremental
# (like find and notify... good examples).

import re

from bases.collection import find_binary
from bases.FrameworkServices.ExecutableService import ExecutableService


disabled_by_default = True

update_every = 5

ORDER = [
    'syscall_rw',
    'smb2_rw',
    'smb2_create_close',
    'smb2_info',
    'smb2_find',
    'smb2_notify',
    'smb2_sm_count'
]

CHARTS = {
    'syscall_rw': {
        'options': [None, 'R/Ws', 'KiB/s', 'syscall', 'syscall.rw', 'area'],
        'lines': [
            ['syscall_sendfile_bytes', 'sendfile', 'incremental', 1, 1024],
            ['syscall_recvfile_bytes', 'recvfile', 'incremental', -1, 1024]
        ]
    },
    'smb2_rw': {
        'options': [None, 'R/Ws', 'KiB/s', 'smb2', 'smb2.rw', 'area'],
        'lines': [
            ['smb2_read_outbytes', 'readout', 'incremental', 1, 1024],
            ['smb2_write_inbytes', 'writein', 'incremental', -1, 1024],
            ['smb2_read_inbytes', 'readin', 'incremental', 1, 1024],
            ['smb2_write_outbytes', 'writeout', 'incremental', -1, 1024]
        ]
    },
    'smb2_create_close': {
        'options': [None, 'Create/Close', 'operations/s', 'smb2', 'smb2.create_close', 'line'],
        'lines': [
            ['smb2_create_count', 'create', 'incremental', 1, 1],
            ['smb2_close_count', 'close', 'incremental', -1, 1]
        ]
    },
    'smb2_info': {
        'options': [None, 'Info', 'operations/s', 'smb2', 'smb2.get_set_info', 'line'],
        'lines': [
            ['smb2_getinfo_count', 'getinfo', 'incremental', 1, 1],
            ['smb2_setinfo_count', 'setinfo', 'incremental', -1, 1]
        ]
    },
    'smb2_find': {
        'options': [None, 'Find', 'operations/s', 'smb2', 'smb2.find', 'line'],
        'lines': [
            ['smb2_find_count', 'find', 'incremental', 1, 1]
        ]
    },
    'smb2_notify': {
        'options': [None, 'Notify', 'operations/s', 'smb2', 'smb2.notify', 'line'],
        'lines': [
            ['smb2_notify_count', 'notify', 'incremental', 1, 1]
        ]
    },
    'smb2_sm_count': {
        'options': [None, 'Lesser Ops', 'count', 'smb2', 'smb2.sm_counters', 'stacked'],
        'lines': [
            ['smb2_tcon_count', 'tcon', 'absolute', 1, 1],
            ['smb2_negprot_count', 'negprot', 'absolute', 1, 1],
            ['smb2_tdis_count', 'tdis', 'absolute', 1, 1],
            ['smb2_cancel_count', 'cancel', 'absolute', 1, 1],
            ['smb2_logoff_count', 'logoff', 'absolute', 1, 1],
            ['smb2_flush_count', 'flush', 'absolute', 1, 1],
            ['smb2_lock_count', 'lock', 'absolute', 1, 1],
            ['smb2_keepalive_count', 'keepalive', 'absolute', 1, 1],
            ['smb2_break_count', 'break', 'absolute', 1, 1],
            ['smb2_sessetup_count', 'sessetup', 'absolute', 1, 1]
        ]
    }
}


class Service(ExecutableService):
    def __init__(self, configuration=None, name=None):
        ExecutableService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.rgx_smb2 = re.compile(r'(smb2_[^:]+|syscall_.*file_bytes):\s+(\d+)')

    def check(self):
        sudo_binary, smbstatus_binary = find_binary('sudo'), find_binary('smbstatus')

        if not (sudo_binary and smbstatus_binary):
            self.error("Can\'t locate 'sudo' or 'smbstatus' binary")
            return False

        self.command = [sudo_binary, '-v']
        err = self._get_raw_data(stderr=True)
        if err:
            self.error(''.join(err))
            return False

        self.command = ' '.join([sudo_binary, '-n', smbstatus_binary, '-P'])

        return ExecutableService.check(self)

    def _get_data(self):
        """
        Format data received from shell command
        :return: dict
        """
        raw_data = self._get_raw_data()
        if not raw_data:
            return None

        parsed = self.rgx_smb2.findall(' '.join(raw_data))

        return dict(parsed) or None
