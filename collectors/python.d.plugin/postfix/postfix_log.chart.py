# -*- coding: utf-8 -*-
# Description: Postfix delivery delay netdata python.d module
# Author: Daniel Morante (tuaris)
# SPDX-License-Identifier: GPL-3.0-or-later

import re
import statistics as stat

from bases.FrameworkServices.LogService import LogService

DELAY_REGEX = 'delay=([-+]?[0-9]*\.?[0-9]+),'

ORDER = ['emails', 'sent', 'failures', 'delay']

CHARTS = {
    'emails': {
        'options': [None, 'Emails Processed', 'emails', 'delivery', 'postfix.total_emails', 'line'],
        'lines': [
            ['emails', None, 'absolute'],
        ]
    },
    'sent': {
        'options': [None, 'Emails Sent', 'emails', 'delivery', 'postfix.sent', 'line'],
        'lines': [
            ['sent', None, 'absolute'],
        ]
    },
    'failures': {
        'options': [None, 'Temporary Failures', 'emails', 'delivery', 'postfix.failures', 'line'],
        'lines': [
            ['failures', None, 'absolute'],
        ]
    },
    'delay': {
        'options': [None, 'Average Mail Delay', 'seconds', 'delivery', 'postfix.delay', 'line'],
        'lines': [
            ['seconds', None, 'absolute'],
        ]
    }

}


class Service(LogService):
    def __init__(self, configuration=None, name=None):
        LogService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.re = DELAY_REGEX
        self.reSent = re.compile(r'status=sent')
        self.reFailure = re.compile(r'status=temporary failure')
        self.log_path = self.configuration.get('log_path', '/var/log/maillog')
        self.data = {
                        'emails' : 0,
                        'sent' : 0,
                        'failures' : 0,
                        'delay' : 0.0
                    }

    def check(self):
        if not LogService.check(self):
            return False

        if not self.re:
            self.error("regex not specified")
            return False

        try:
            self.re = re.compile(self.re)
        except re.error as err:
            self.error("regex compile error: ", err)
            return False

        return True

    def get_data(self):
        """
        :return: dict
        """
        raw = self._get_raw_data()
        delays = list()

        if not raw:
            return None

        for line in raw:
            match = self.re.search(line)
            if match:
                self.data['emails'} += 1
                delay = match.group(1)

                if not(self.reSent.search(line) or self.reFailure.search(line)):
                    continue

                delays.append(float(delay))

                if self.reSent.search(line):
                    self.data['sent'] += 1
                    continue

                if self.reFailure.search(line):
                    self.data['failures'] += 1

            else:
                continue

        mean = stat.mean(delays)
        sd = stat.pstdev(delays)
        lower = mean-2*sd
        upper = mean+2*sd

        delay_pop = [ x for x in delays if x >= lower and x<= upper ]

        self.data['delay'] = stat.mean(delay_pop)

        return self.data
