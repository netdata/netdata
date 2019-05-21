# -*- coding: utf-8 -*-
# Description: mail log netdata python.d module
# Author: apardyl
# SPDX-License-Identifier: GPL-3.0-or-later

import os
import re
import collections
from bases.FrameworkServices.LogService import LogService

ORDER = ['connections', 'incoming_statuses', 'incoming_codes', 'outgoing_statuses', 'outgoing_codes']

CHARTS = {
    'connections': {
        'options': [None, 'Incoming connections', 'connections/s', 'incoming',
                    'mail_log.active_connections', 'line'],
        'lines': [
            ['mail_connections', 'connections', 'incremental', 1, 1],
        ]
    },

    'incoming_statuses': {
        'options': [None, 'Incoming email statuses', 'messages/s', 'incoming',
                    'mail_log.incoming_statuses', 'stacked'],
        'lines': [
            ['mail_accepted', 'accepted', 'incremental', 1, 1],
            ['mail_greylisted', 'greylisted', 'incremental', 1, 1],
            ['mail_temp_rejected', 'temporarily rejected', 'incremental', 1, 1],
            ['mail_perm_rejected', 'permanently rejected', 'incremental', 1, 1]
        ]
    },

    'incoming_codes': {
        'options': [None, 'Incoming email status codes', 'messages/s', 'incoming',
                    'mail_log.incoming_codes', 'stacked'],
        'lines': [
            ["mail_incoming_codes_2.0.0", '2.0.0', 'incremental', 1, 1]
        ]
    },

    'outgoing_statuses': {
        'options': [None, 'Outgoing email statuses', 'messages/s', 'outgoing',
                    'mail_log.outgoing_statuses', 'stacked'],
        'lines': [
            ['mail_sent', 'sent', 'incremental', 1, 1],
            ['mail_deferred', 'deferred', 'incremental', 1, 1],
            ['mail_bounced', 'bounced', 'incremental', 1, 1]
        ]
    },

    'outgoing_codes': {
        'options': [None, 'Outgoing email status codes', 'messages/s', 'outgoing',
                    'mail_log.outgoing_codes', 'stacked'],
        'lines': [
            ["mail_outgoing_codes_2.0.0", '2.0.0', 'incremental', 1, 1]
        ]
    },
}


class MailStatistics:
    # incoming
    connections = 0
    incoming_codes = {
        "2.0.0": 0
    }
    accepted = 0
    temp_rejected = 0
    perm_rejected = 0
    greylisted = 0
    # outgoing
    outgoing_codes = {
        "2.0.0": 0
    }
    sent = 0
    deferred = 0
    bounced = 0


class MailParser(object):
    """Abstract MailParser class, mail log parsers must implement the _parse_line method"""

    def __init__(self, log_service):
        self.service = log_service

    def _parse_line(self, line):
        raise NotImplementedError

    def parse_line(self, line):
        self._parse_line(line)


class PostfixParser(MailParser):
    """MailParser for Postfix"""
    POSTFIX_RE = re.compile(r'postfix(\/\w+)?\/([a-z]+)\[\d*\]: (\w+)')
    QUEUE_RE = re.compile(r'from=<.*\(queue active\)')
    REJECT_RE = re.compile(r'reject: [^:]+: (\d{3} )?(\d+\.\d+\.\d+)')
    STATUS_RE = re.compile(r'dsn=(\d+\.\d+\.\d+).*status=(\w+)')

    def __init__(self, log_service):
        super(PostfixParser, self).__init__(log_service)
        self.greylist_status = log_service.configuration.get("greylist_status", "Try again later")
        # Since postfix does not log messages passing milters (only rejected ones) we store last 50 ids of messages
        # received by the `cleanup` process and count them as accepted when queued for delivery for the first time.
        self.queue_ids = collections.OrderedDict()

    def parse_qmgr(self, line, msg_id):
        """Parses queue manager log"""
        if self.QUEUE_RE.search(line) and msg_id in self.queue_ids.keys():
            self.queue_ids.pop(msg_id)
            self.service.stats.accepted += 1
            self.service.increment_incoming_code("2.0.0")

    def parse_smtpd_and_cleanup(self, line, first, service):
        """Parses smtpd and cleanup logs"""
        if service == "smtpd" and first == "connect":
            self.service.stats.connections += 1
            return
        reject = self.REJECT_RE.search(line)
        if reject:
            code = reject.group(2)
            self.service.increment_incoming_code(code)
            if self.greylist_status and self.greylist_status in line:
                self.service.stats.greylisted += 1
            if code.startswith("4"):
                self.service.stats.temp_rejected += 1
            elif code.startswith("5"):
                self.service.stats.perm_rejected += 1
            return
        if service == "cleanup":
            self.queue_ids[first] = None
            if len(self.queue_ids) > 50:
                self.queue_ids.popitem(False)

    def parse_smtp(self, line):
        """Parses smtp client log"""
        status = self.STATUS_RE.search(line)
        if status:
            self.service.increment_outgoing_code(status.group(1))
            stat = status.group(2)
            if stat == "sent":
                self.service.stats.sent += 1
            elif stat == "deferred":
                self.service.stats.deferred += 1
            elif stat == "bounced":
                self.service.stats.bounced += 1

    def _parse_line(self, line):
        match = self.POSTFIX_RE.search(line)
        if not match:
            return
        service = match.group(2)
        first = match.group(3)
        if service == "qmgr":
            self.parse_qmgr(line, first)
        elif service == "smtpd" or service == "cleanup":
            self.parse_smtpd_and_cleanup(line, first, service)
        elif service == "smtp":
            self.parse_smtp(line)


class Service(LogService):
    def __init__(self, configuration=None, name=None):
        LogService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.stats = MailStatistics()
        self.parser = {
            "postfix": PostfixParser
        }.get(configuration.get("type", "postfix"))(self)

    def check(self):
        """
        :return: bool

        1. "log_path" is specified in the module configuration file
        2. "log_path" must must exist and be readable by netdata user
        """
        if not self.log_path:
            self.error('log path is not specified')
            return False

        if not (self._find_recent_log_file() and os.access(self.log_path, os.R_OK)):
            self.error('{log_file} not readable or does not exist'.format(log_file=self.log_path))
            return False
        return True

    def parse_line(self, line):
        self.parser.parse_line(line)

    def _get_data(self):
        """
        Parses new log lines
        :return: dict OR None
        None if _get_raw_data method fails or parsing error encountered.
        In all other cases - dict.
        """
        try:
            for line in self._get_raw_data():
                self.parse_line(line)
            data = {
                "mail_connections": self.stats.connections,
                "mail_accepted": self.stats.accepted,
                "mail_greylisted": self.stats.greylisted,
                "mail_temp_rejected": self.stats.temp_rejected,
                "mail_perm_rejected": self.stats.perm_rejected,
                "mail_sent": self.stats.sent,
                "mail_deferred": self.stats.deferred,
                "mail_bounced": self.stats.bounced
            }

            for code, count in self.stats.incoming_codes.items():
                data["mail_incoming_codes_" + code] = count
            for code, count in self.stats.outgoing_codes.items():
                data["mail_outgoing_codes_" + code] = count

            return data
        except (ValueError, AttributeError) as ex:
            self.error(ex)
            return None

    def increment_incoming_code(self, code):
        """Increments number of incoming messages per status code, adds a new line to chart if required"""
        if code not in self.stats.incoming_codes:
            self.charts["incoming_codes"].add_dimension(['mail_incoming_codes_' + code, code, 'incremental', 1, 1])
            self.stats.incoming_codes[code] = 0
        self.stats.incoming_codes[code] += 1

    def increment_outgoing_code(self, code):
        """Increments number of outgoing messages per status code, adds a new line to chart if required"""
        if code not in self.stats.outgoing_codes:
            self.charts["outgoing_codes"].add_dimension(['mail_outgoing_codes_' + code, code, 'incremental', 1, 1])
            self.stats.outgoing_codes[code] = 0
        self.stats.outgoing_codes[code] += 1
