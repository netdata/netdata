# -*- coding: utf-8 -*-
# Description: sendmail netdata python.d module
# Author: Ben Gräfe (sthinbetween90)
# SPDX-License-Identifier: GPL-3.0-or-later

from bases.FrameworkServices.ExecutableService import ExecutableService

SENDMAIL_COMMAND = 'mailstats -p'

ORDER = [
    'byte_count',
    'message_count',
    'rejected_count',
    'total_message_count',
    'total_byte_count',
    'active_tcp'
]

CHARTS = {
    'byte_count': {
        'options': [None, 'Sendmail Bytecount', 'KiB', 'sendmail', 'sendmail.byteCount', 'stacked'],
        'lines': [
            ['bytes_from_local', None, 'incremental'],
            ['bytes_from_esmtp', None, 'incremental'],
            ['bytes_to_local', None, 'incremental'],
            ['bytes_to_esmtp', None, 'incremental'],
        ]
    },
    'message_count': {
        'options': [None, 'Sendmail Messagecount', 'messages', 'sendmail', 'sendmail.messageCount', 'stacked'],
        'lines': [
            ['messages_from_local', None, 'incremental'],
            ['messages_from_esmtp', None, 'incremental'],
            ['messages_to_local', None, 'incremental'],
            ['messages_to_esmtp', None, 'incremental'],
        ]
    },
    'rejected_count': {
        'options': [None, 'Sendmail Rejected Messages', 'messages', 'sendmail', 'sendmail.rejectedCount', 'stacked'],
        'lines': [
            ['rejected_local', None, 'incremental'],
            ['rejected_esmtp', None, 'incremental'],
            ['discarded_local', None, 'incremental'],
            ['discarded_esmtp', None, 'incremental'],
            ['quarantined_local', None, 'incremental'],
            ['quarantined_esmtp', None, 'incremental'],
        ]
    },
    'total_message_count': {
        'options': [None, 'Sendmail Total Messagecount', 'messages', 'sendmail', 'sendmail.totMessageCount', 'stacked'],
        'lines': [
            ['total_messages_inc', None, 'incremental'],
            ['total messages_out', None, 'incremental'],
            ['total_messages_rej', None, 'incremental'],
            ['total_messages_dis', None, 'incremental'],
            ['total_messages_quar', None, 'incremental'],
        ]
    },
    'total_byte_count': {
        'options': [None, 'Sendmail Total Bytecount', 'KiB', 'sendmail', 'sendmail.totByteCount', 'stacked'],
        'lines': [
            ['total_bytes_in', None, 'incremental'],
            ['total_bytes_out', None, 'incremental'],
        ]
    },
    'active_tcp': {
        'options': [None, 'Active TCP Connections', 'connections', 'sendmail', 'sendmail.activeTCP', 'stacked'],
        'lines': [
            ['tcp_out', None, 'incremental'],
            ['tcp_in', None, 'incremental'],
            ['tcp_err', None, 'incremental'],
        ]
    },
}


class Service(ExecutableService):
    def __init__(self, configuration=None, name=None):
        ExecutableService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.command = SENDMAIL_COMMAND

    def _get_data(self):
        """
        Format data received from shell command
        :return: dict
        """
        try:
            elements = self._get_raw_data() 
            data = dict()

            for line in elements:
                if 'local' in line: 
                    local_line = ' '.join(line.split()).split(' ')
                    if (len(local_line) == 9):
                        data['bytes_from_local'] = local_line[2]
                        data['bytes_to_local'] = local_line[4]
                        data['messages_from_local'] = local_line[1]
                        data['messages_to_local'] = local_line[3]
                        data['rejected_local'] = local_line[5]
                        data['discarded_local'] = local_line[6]
                        data['quarantined_local'] = local_line[7]
            
                if 'esmtp' in line: 
                    esmtp_line = ' '.join(line.split()).split(' ')
                    if (len(esmtp_line) == 9):
                        data['bytes_from_esmtp'] = esmtp_line[2]
                        data['bytes_to_esmtp'] = esmtp_line[4]
                        data['messages_from_esmtp'] = esmtp_line[1]
                        data['messages_to_esmtp'] = esmtp_line[3]
                        data['rejected_esmtp'] = esmtp_line[5]
                        data['discarded_esmtp'] = esmtp_line[6]
                        data['quarantined_esmtp'] = esmtp_line[7]

                if 'T' in line:
                    total_line = ' '.join(line.split()).split(' ')
                    if (len(total_line) == 8):		
                        data['total_messages_inc'] = total_line[3]
                        data['total_messages_out'] = total_line[1]
                        data['total_messages_rej'] = total_line[5]
                        data['total_messages_dis'] = total_line[6]
                        data['total_messages_quar'] = total_line[7]
                        data['total_bytes_in'] = total_line[4]
                        data['total_bytes_out'] = total_line[2]			
                if 'C' in line:
                    tcp_line = ' '.join(line.split()).split(' ')
                    if (len(tcp_line) == 4):
                        data['tcp_in'] = tcp_line[2]
                        data['tcp_out'] = tcp_line[1]
                        data['tcp_err'] = tcp_line[3]
            return(data)
        
        except (ValueError, AttributeError):
            return None
