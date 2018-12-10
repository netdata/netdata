# -*- coding: utf-8 -*-
# Description: parsing applications log files
# Author: Hamed Beiranvand (hamedbrd)

import re
import sys
from bases.FrameworkServices.LogService import LogService


ORDER = ['jobs']

CHARTS = {
    'jobs': {
        'options': [None, 'Parse Log Line', 'jobs/s', 'Log Parser', 'logparser.jobs', 'line'],
        'lines': [
        ]
    }
}

METHOD_REGEX = "regex"
METHOD_STRING = "string"


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
            item_val = self.dimensions[item]
            method, value = item_val.split("=")

            if method == METHOD_REGEX:
                 self.data[item] = self.regex_matcher_factory(value,item,raw)

            if method == METHOD_STRING:
                 self.data[item] = self.string_matcher_factory(value,item,raw)

        return self.data
            
        
    def regex_matcher_factory(self,value,item,data):
        count = 0
        for line in data:
            if value.startswith('^'):
                    regex = re.compile(value)
                    count += bool(regex.match(line))
            else:
                    regex = re.compile(value)
                    count += bool(regex.search(line))
        return count
    
    def string_matcher_factory(self,value,item,data):
         count = 0
         for line in data:
            if value.startswith("^"):
                count += regex.startswith(line)
            elif value.endswith('$'):
                count += regex.endswith(line)
            else:
                regex = re.compile(value)
                count += bool(regex.search(line))

         return count
