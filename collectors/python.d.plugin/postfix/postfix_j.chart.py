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
    'qmeandelay',
]

CHARTS = {
    'qactiveemails': {
        'options': [None, 'Postfix Active Queue Emails', 'emails', 'queue', 'postfix.qactiveemails', 'line'],
        'lines': [
            ['active_emails', None, 'absolute']
        ]
    },
    'qtemporaryfails': {
        'options': [None, 'Postfix Deferred Queue Failures', 'emails', 'queue', 'postfix.qtemporaryfails', 'line'],
        'lines': [
            ['temporary_failures', None, 'absolute']
        ]
    },
    'qmeandelay': {
        'options': [None, 'Postfix Queue Mean Delay', 'seconds', 'queue', 'postfix.qmeandelay', 'line'],
        'lines': [
            ['mean_delay', None, 'absolute']
        ]
    }
}


class Service(ExecutableService):
    def __init__(self, configuration=None, name=None):
        ExecutableService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.command = POSTQUEUE_COMMAND
        self.data = { 
                        'active_emails' : 0,
                        'temp_fail' : 0,
                        'total_delay' : [ ]
                    }

    def _get_data(self):
        """
        Format data received from shell command
        :return: dict
        """


        try:

	        raw = self._get_raw_data()
	
	        if not raw:
	            return None
	
	        epoch_now = calendar.timegm(time.gmtime())
	
	        for line in raw:
	            jdata = json.load(self._get_raw_data(raw))
	
	            if not jdata:
	                continue
	
	            if jdata['queue_name'] != 'active' \
	                and not ( jdata['queue_name'] == 'deferred' \
	                          jdata['delay_reason'] == 'temporary_failure') :
	'
	                # for now only collecting stats for active messages 
	                #   and temporary failures
	                continue
	        
	            
	            self.data['total_delay'].append(epoch_now - jdata['arrival_time'])
	
	            if jdata['queue_name'] == 'active':
	                self.data['active_emails'] += 1
	                continue
	
	            if jdata['delay_reason'] == 'temporary failure':
	                self.data['temp_fail'] += 1
	
	
            mean = stat.mean(self.data['total_delay'])
            sd = stat.pstdev(self.data['total_delay'])
            
            lower = mean - 2*sd
            upper = mean + 2*sd
            
            final_fail = [ x for x in self.data['temp_fail'] if x >= lower and x <= upper ]
            
            return { 
                'active_emails' : self.data['active_emails'],
                'temporary_failures' : self.data['temp_fail'],
                'mean_delay' : stat.mean(final_fail)
            }

        except (ValueError, AttributeError):
            return None

