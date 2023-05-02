# -*- coding: utf-8 -*-
# Description: postfix netdata python.d module
# Author: Pawel Krupa (paulfantom)
# SPDX-License-Identifier: GPL-3.0-or-later

import json
import time
import calendar
import statistics as stat

from bases.FrameworkServices.ExecutableService import ExecutableService

POSTQUEUE_COMMAND = 'postqueue -j'

ORDER = [
    'qactiveemails',
    'qtemporaryfails',
    'qdelay',
    'qwdelay',
]

CHARTS = {
    # count of emails in the active queue
    'qactiveemails': {
        'options': [None, 'Active Queue Emails', 'emails', 'queue', 'postfix.qactiveemails', 'line'],
        'lines': [
            ['active_emails', None, 'absolute']
        ]
    },
    # count of emails in the deferred queue with delay_reson = 'temporary failure'
    'qtemporaryfails': {
        'options': [None, 'Deferred Queue Failures', 'emails', 'queue', 'postfix.qtemporaryfails', 'line'],
        'lines': [
            ['temporary_failures', None, 'absolute']
        ]
    },
    # average delay of all messages either in the active queue or in the deferred queue with status 'temporary failure'
    # only those datapoints within 2*SD from the mean are considered to avoid skewing the distribution 
    #   by outliers ( all messages that have been in the queue for a considerable amount of time )
    'qdelay': {
        'options': [None, 'Queue Mean Delay', 'seconds', 'queue', 'postfix.qdelay', 'line'],
        'lines': [
            ['delay', None, 'absolute']
        ]
    },
    # average delay of all messages either in the active queue or in the deferred queue with status 'temporary failure'
    # only those messages which have a delay within a specified time window, messages failed a long time ago are not considered
    #   window can be specified in the config file, default 40min
    'qwdelay': {
        'options': [None, 'Queue Mean Delay in Window', 'seconds', 'queue', 'postfix.qwdelay', 'line'],
        'lines': [
            ['wdelay', None, 'absolute']
        ]
    }
}


class Service(ExecutableService):
    def __init__(self, configuration=None, name=None):
        ExecutableService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.command = POSTQUEUE_COMMAND
        self.delay_window = float(self.configuration.get('delay_window_span', 2400))
        self.data = { 
                        'active_emails' : 0,
                        'temp_fail' : 0,
                        'delay' : 0.0,
                        'wdelay' : 0.0
                    }

    def _get_data(self):
        """
        Format data received from shell command
        :return: dict
        """

        self.data = dict({
                        'active_emails' : 0,
                        'temp_fail' : 0,
                        'total_delay' : 0.0
                    })

        delays = list()
        wdelays = list()

        try:

            raw = self._get_raw_data()

	
            if not raw:
                return None
	
            epoch_now = calendar.timegm(time.gmtime())
	
            for line in raw:
                jdata = json.loads(line)

                if not jdata:
                    continue
	
                if jdata['queue_name'] != 'active' \
                    and not ( jdata['queue_name'] == 'deferred' \
                            and jdata['recipients'][0]['delay_reason'] == 'temporary failure' ) :
	            # for now only collecting stats for active messages 
	            #   and temporary failures
                    continue

	            
                delay = epoch_now - jdata['arrival_time']
                delays.append(delay)
                if delay <= self.delay_window:
                    wdelays.append(delay)

                if jdata['queue_name'] == 'active':
                    self.data['active_emails'] += 1
                    continue
	
                if 'delay_reason' in jdata['recipients'][0] and jdata['recipients'][0]['delay_reason'] == 'temporary failure':
                    self.data['temp_fail'] += 1
	

        except (ValueError, AttributeError):
            return None
	
        # ensure atleast one datapoint
        if len(delays) <=0:
            delays.append(0)
        if len(wdelays) <=0:
            wdelays.append(0)

        mean = stat.mean(delays)
        sd = stat.pstdev(delays)
        
        lower = mean - 2*sd
        upper = mean + 2*sd
            
        # only data 2*SD from mean
        final_fail = [ x for x in delays if x >= lower and x <= upper ]
            
        return dict({ 
            'active_emails' : self.data['active_emails'],
            'temporary_failures' : self.data['temp_fail'],
            'delay' : stat.mean(final_fail),
            'wdelay' : stat.mean(wdelays)
        })

