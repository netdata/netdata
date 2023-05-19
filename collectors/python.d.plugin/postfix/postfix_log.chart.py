# -*- coding: utf-8 -*-
# Description: Postfix delivery delay netdata python.d module
# Author: Daniel Morante (tuaris)
# SPDX-License-Identifier: GPL-3.0-or-later

import re
import statistics as stat
import ast

from bases.FrameworkServices.LogService import LogService

DELAY_REGEX = 'delay=([-+]?[0-9]*\.?[0-9]+),'

ORDER = ['emails', 'sent', 'failures', 'delay']

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
    # average delay of emails that were processed, in update window, for the specified relay
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
    # *** the below dictionary will be appended for every delay window during initialization ***
    #
    #'wdelay': {
    #    'options': [None, 'Average Mail Delay in Window', 'seconds', 'delivery', 'postfix.wdelay', 'line'],
    #    'lines': [
    #        ['wdelay', None, 'absolute'],
    #    ]
    #}

}

wdelay_template = """
{
    'options': [None, 'Average Delay in %d minute Window', 'seconds', 'delivery', 'postfix.%s', 'line'],
    'lines': [
        ['%s', None, 'absolute'],
    ]
}
"""



class Service(LogService):
    def __init__(self, configuration=None, name=None):
        LogService.__init__(self, configuration=configuration, name=name)
        self.reSent = re.compile(r'status=sent')
        self.reFailure = re.compile(r'status=deferred \(temporary failure\)')
        self.log_path = self.configuration.get('log_path', '/var/log/mail.log')
        delay_windows = self.configuration.get('delay_window_span', 2400)


        self.relay=self.configuration.get('relay', '*')
        if self.relay == '*':
            self.reRelay = re.compile(r'relay=.+')
        else:
            self.reRelay = re.compile(f'relay={self.relay}')

        filter_relay = self.configuration.get('email_counter_relay', 'pbfilter')
        self.reFilterRelay = re.compile(f'relay={filter_relay}')


        if isinstance(delay_windows, list) is False:
            delay_windows = [delay_windows]

        self.delay_windows = list()
        for d in delay_windows:
            min_delay = int(float(d)/60)
            delay_name = "w%ddelay" % (min_delay)
            CHARTS[delay_name] = ast.literal_eval(wdelay_template % (min_delay, delay_name, delay_name))
            ORDER.append(delay_name)

            self.delay_windows.append({'name' : delay_name, 'window' : float(d)})



        self.data = {
                        'emails' : 0,
                        'sent' : 0,
                        'failures' : 0,
                        'delay' : 0.0
                    }

        self.order = ORDER
        self.definitions = CHARTS
        self.re = DELAY_REGEX


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
                    })
        delays = list()
        wdelays = dict()
        for d in  self.delay_windows:
            wdelays[d['name']] = 0.0


        raw = self._get_raw_data()

        if not raw:
            return None

        for line in raw:
            match = self.re.search(line)
            if match:
                delay = match.group(1)

                # only count the filter relay lines since that appears one per email
                if self.reFilterRelay.search(line):
                    self.data['emails'] += 1

                # only sent and failures, timeouts included, all other statuses excluded
                if not(self.reSent.search(line) or self.reFailure.search(line)):
                    continue

                delay = float(delay)
                if not self.reRelay.search(line):
                    # only stats to the configured relay will be gathered
                    continue

                delays.append(delay)

                # only add datapoint if delay is within delay window
                temp_fail = False
                if self.reFailure.search(line):
                    temp_fail = True

                for d in  self.delay_windows:
                    if temp_fail:
                        if delay <= d['window']:
                            wdelays[d['name']] += delay
                    else:
                        wdelays[d['name']] += delay

                if self.reFilterRelay.search(line) and self.reSent.search(line):
                    self.data['sent'] += 1
                    continue

                if self.reFilterRelay.search(line) and self.reFailure.search(line):
                    self.data['failures'] += 1

            else:
                continue

        # for a small update window there may not be any datapoints available
        if len(delays) <= 0:
            delays.append(0)

        # mitigate divide by zero
        tot_emls = self.data['emails']
        if tot_emls <= 0:
            tot_emls = 1

        mean = stat.mean(delays)
        sd = stat.pstdev(delays)
        lower = mean-2*sd
        upper = mean+2*sd

        # only consider data within 2*SD from the mean
        delay_pop = [ x for x in delays if x >= lower and x<= upper ]

        self.data['delay'] = sum(delay_pop)/tot_emls
        for k in wdelays:
            self.data[k] = wdelays[k]/tot_emls

        return self.data
