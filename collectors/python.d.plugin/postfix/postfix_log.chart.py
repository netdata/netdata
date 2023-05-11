# -*- coding: utf-8 -*-
# Description: Postfix delivery delay netdata python.d module
# Author: Daniel Morante (tuaris)
# SPDX-License-Identifier: GPL-3.0-or-later

import re
import statistics as stat

from bases.FrameworkServices.LogService import LogService

DELAY_REGEX = 'delay=([-+]?[0-9]*\.?[0-9]+),'

ORDER = ['emails', 'sent', 'failures', 'delay', 'wdelay']

CHARTS = {
    # number of emails that came thru in update window
    'emails': {
        'options': [None, 'Emails Processed', 'emails', 'delivery', 'postfix.total_emails', 'line'],
        'lines': [
            ['emails', None, 'absolute'],
        ]
    },
    # number of emails successfully sent in update window
    'sent': {
        'options': [None, 'Emails Sent', 'emails', 'delivery', 'postfix.sent', 'line'],
        'lines': [
            ['sent', None, 'absolute'],
        ]
    },
    # number of emails that errored out with status 'temporary failure' in update window
    'failures': {
        'options': [None, 'Temporary Failures', 'emails', 'delivery', 'postfix.failures', 'line'],
        'lines': [
            ['failures', None, 'absolute'],
        ]
    },
    # average delay of emails that were processed, in update window
    # all emails processed with a status in the the list ['sent', 'temporary failure']
    # to eliminate the skew caused by outliers on the higher end, this metric only considers the
    #   datapoints which are within two standard deviations from the mean
    'delay': {
        'options': [None, 'Average Mail Delay', 'seconds', 'delivery', 'postfix.delay', 'line'],
        'lines': [
            ['delay', None, 'absolute'],
        ]
    },
    # average delay of emails that were processed, in update window
    # all emails processed with a status in the the list ['sent', 'temporary failure']
    # only consider datapoints whose delay is within a delay window specified
    # within the config file - default window is <= 40min
    #   this is to ensure very old temporary failures do not skew the delay calculation
    'wdelay': {
        'options': [None, 'Average Mail Delay in Window', 'seconds', 'delivery', 'postfix.wdelay', 'line'],
        'lines': [
            ['wdelay', None, 'absolute'],
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
        self.reFailure = re.compile(r'status=deferred \(temporary failure\)')
        self.log_path = self.configuration.get('log_path', '/var/log/mail.log')
        self.delay_window = float(self.configuration.get('delay_window_span', 2400))
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
        self.data = dict({
                        'emails' : 0,
                        'sent' : 0,
                        'failures' : 0,
                        'delay' : 0.0,
                        'wdelay' : 0.0
                    })
        delays = list()
        wdelays = list()

        raw = self._get_raw_data()

        if not raw:
            return None

        for line in raw:
            match = self.re.search(line)
            if match:
                self.data['emails'] += 1
                delay = match.group(1)

                # only sent and failures, timeouts and all other statuses excluded
                if not(self.reSent.search(line) or self.reFailure.search(line)):
                    continue

                delay = float(delay)

                delays.append(delay)

                # only add datapoint if delay is within delay window
                if delay <= self.delay_window:
                    wdelays.append(delay)

                if self.reSent.search(line):
                    self.data['sent'] += 1
                    continue

                if self.reFailure.search(line):
                    self.data['failures'] += 1

            else:
                continue

        # for a small update window there may not be any datapoints available
        if len(delays) <= 0:
            delays.append(0)
        if len(wdelays) <=0:
            wdelays.append(0)

        mean = stat.mean(delays)
        sd = stat.pstdev(delays)
        lower = mean-2*sd
        upper = mean+2*sd

        # only consider data within 2*SD from the mean
        delay_pop = [ x for x in delays if x >= lower and x<= upper ]

        self.data['delay'] = stat.mean(delay_pop)
        self.data['wdelay'] = stat.mean(wdelays)

        return self.data
