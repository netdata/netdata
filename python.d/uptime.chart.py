# -*- coding: utf-8 -*-
# Description: uptime netdata python.d module
# Author: l2isbad

from base import SimpleService
from re import compile
priority = 60000
retries = 60
update_every = 1

ORDER = ['uptime']

CHARTS = {'uptime': {
            'options': [None, "System uptime", "uptime", 'uptime', 'uptime.days', 'stacked'],
            'lines': [
                ['minutes', None, 'absolute'], ['hours', None, 'absolute'],
                ['days', None, 'absolute']
                    ]}
        }


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)        
        self.order = ORDER
        self.definitions = CHARTS
        self._orig_name = ''

    def check(self):
        raw_data = self._get_raw_data()

        if raw_data:
            self._orig_name = self.chart_name
            return True
        else:
            return False

    def _get_raw_data(self):
        """
        Read data from /proc/uptime
        :return: float
        """
        try:
            with open('/proc/uptime', 'rt') as uptime:
                uptime = float(uptime.read().partition(' ')[0])
        except Exception:
            return None
        else:
            return uptime

    def _get_data(self):
        """
        Parse data from _get_raw_data()
        :return: dict
        """
        uptime = self._get_raw_data()

        days = str((uptime / 86400)).partition('.')[0]
        var1 = uptime % 86400
        hours = str((var1 / 3600)).partition('.')[0]
        minutes = str((var1 % 3600 / 60)).partition('.')[0]
        to_netdata = {'days': int(days), 'hours': int(hours), 'minutes': int(minutes)} 

        return to_netdata

    def create(self):
        self.chart_name = "system"
        status = SimpleService.create(self)
        self.chart_name = self._orig_name
        return status

    def update(self, interval):
        self.chart_name = "system"
        status = SimpleService.update(self, interval=interval)
        self.chart_name = self._orig_name
        return status
