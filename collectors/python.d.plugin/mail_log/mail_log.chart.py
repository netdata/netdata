# -*- coding: utf-8 -*-
# Description: mail log netdata python.d module
# Author: apardyl
# SPDX-License-Identifier: GPL-3.0-or-later

import os
import re
from bases.FrameworkServices.LogService import LogService

ORDER = ['incoming_statuses', 'incoming_codes', 'outgoing_statuses', 'outgoing_codes']

CHARTS = {
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


class MailParser:
    """Abstract MailParser class, parsers must implement the _parse_line method"""
    def __init__(self, log_service):
        self.charts = log_service.charts
        self.stats = log_service.stats

    def increment_incoming_code(self, code):
        if code not in self.stats.incoming_codes:
            self.charts["incoming_codes"].add_dimension(['mail_incoming_codes_' + code, code, 'incremental', 1, 1])
            self.stats.incoming_codes[code] = 0
        self.stats.incoming_codes[code] += 1

    def increment_outgoing_code(self, code):
        if code not in self.stats.outgoing_codes:
            self.charts["outgoing_codes"].add_dimension(['mail_outgoing_codes_' + code, code, 'incremental', 1, 1])
            self.stats.outgoing_codes[code] = 0
        self.stats.outgoing_codes[code] += 1

    def _parse_line(self, line):
        raise NotImplementedError

    def parse_line(self, line):
        self._parse_line(line)


class PostfixParser(MailParser):
    """MailParser for Postfix"""
    POSTFIX_RE = re.compile(r'postfix\/([a-z]+)\[\d*\]:')
    ENQUEUED_RE = re.compile(r'from=<.*\(queue active\)')
    REJECT_RE = re.compile(r'reject: [^:]+: (\d{3} )?(\d+\.\d+\.\d+)')
    STATUS_RE = re.compile(r'status=(\w+)')
    DSN_RE = re.compile(r'dsn=(\d+\.\d+\.\d+)')

    def __init__(self, log_service):
        super().__init__(log_service)
        self.greylist_status = log_service.configuration.get("greylist_status", "Try again later")

    def parse_qmgr(self, line):
        if self.ENQUEUED_RE.search(line):
            self.stats.accepted += 1
            self.increment_incoming_code("2.0.0")

    def parse_smtpd_and_cleanup(self, line):
        reject = self.REJECT_RE.search(line)
        if reject:
            code = reject.group(2)
            self.increment_incoming_code(code)
            if self.greylist_status and self.greylist_status in line:
                self.stats.greylisted += 1
            if code.startswith("4"):
                self.stats.temp_rejected += 1
            elif code.startswith("5"):
                self.stats.perm_rejected += 1

    def parse_smtp(self, line):
        status = self.STATUS_RE.search(line)
        if status:
            code = status.group(1)
            if code == "sent":
                self.stats.sent += 1
            elif code == "deferred":
                self.stats.deferred += 1
            elif code == "bounced":
                self.stats.bounced += 1
        dsn = self.DSN_RE.search(line)
        if dsn:
            self.increment_outgoing_code(dsn.group(1))

    def _parse_line(self, line):
        match = self.POSTFIX_RE.search(line)
        if not match:
            return
        service = match.group(1)
        if service == "qmgr":
            self.parse_qmgr(line)
        elif service == "smtpd" or service == "cleanup":
            self.parse_smtpd_and_cleanup(line)
        elif service == "smtp":
            self.parse_smtp(line)


class Service(LogService):
    def __init__(self, configuration=None, name=None):
        LogService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.stats = MailStatistics()
        self.parsers = [PostfixParser(self)]

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
        for parser in self.parsers:
            parser.parse_line(line)

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
