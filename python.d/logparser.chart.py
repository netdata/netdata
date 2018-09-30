# -*- coding: utf-8 -*-
# Description: parsing applications log files
# Author: Hamed Beiranvand (hamedbrd)

import re

from bases.FrameworkServices.LogService import LogService


ORDER = ['jobs']

CHARTS = {
    'jobs': {
        'options': [None, 'Parse Log Line', 'jobs/s', 'Log Parser', 'logparser.jobs', 'line'],
        'lines': [
        ]
    }
}


class Service(LogService):
    def __init__(self, configuration=None, name=None):
        LogService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.dimensions = self.configuration.get('dimensions')
        self.log_path = self.configuration.get('log_path')
        self.data = {}
    def check(self):
        if not LogService.check(self):
            return False
        try:
            for item in self.dimensions:
                 CHARTS['jobs']['lines'].append([item,item,'incremental'])
                 self.data[item] = 0
                 re.compile(self.dimensions[item])
        except re.error as err:
            self.error("regex compile error: ", err)
            return False
        return True

    def get_data(self):
        raw = self._get_raw_data()
        if not raw:
            return None if raw is None else self.data
        for item in self.dimensions:

            for line in raw:
                regex = re.compile(self.dimensions[item])
                self.data[item] += bool(regex.search(line))

        return self.data
