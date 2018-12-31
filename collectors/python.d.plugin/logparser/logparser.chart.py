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

METHOD_REGEX = "regexp"
METHOD_STRING = "string"


class Service(LogService):
    def __init__(self, configuration=None, name=None):
        LogService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.dimensions = self.configuration.get('dimensions')
        self.log_path = self.configuration.get('log_path')

        self.matchers = list()  # list of DimensionMatcher's
        self.data = dict()

    def check(self):
        if not self.log_path:
            self.error('no log_path specified')
            return False

        if not self.dimensions:
            self.error('no dimensions specified')
            return False

        if not self.populate_matchers():
            return False

        for matcher in self.matchers:
            dim = [matcher.name, matcher.name, 'incremental']
            self.definitions['jobs']['lines'].append(dim)
            self.data[matcher.name] = 0

        return LogService.check(self)

    def populate_matchers(self):
        for name, pattern in self.dimensions.items():
            try:
                matcher = matcher_factory(pattern)
            except ValueError as re.error:
                self.error("error on creating matchers : {0}".format(re.error))
                return False

            self.matchers.append(DimensionMatcher(name, matcher))

        return True


    def get_data(self):
        lines = self._get_raw_data()

        if not lines:
            return None if lines is None else self.data

        for line in lines:
            for matcher in self.matchers:
                if matcher.match(line):
                    self.data[matcher.name] += 1
                    break

        return self.data
    

class DimensionMatcher:
    def __init__(self, name, matcher):
        self.name = name
        self.matcher = matcher

    def match(self, line):
        return self.matcher.match(line)


class BaseStringMatcher:
    def __init__(self, value):
        self.value = value


class BaseRegexMatcher:
    def __init__(self, value):
        self.value = re.compile(value)


class StringMatcher(BaseStringMatcher):
    def match(self, row):
        return self.value in row


class StringPrefixMatcher(BaseStringMatcher):

    def match(self, row):
        return row.startswith(self.value)


class StringSuffixMatcher(BaseStringMatcher):

    def match(self, row):
        return row.endswith(self.value)


class RegexMatchMatcher(BaseRegexMatcher):

    def match(self, row):
        return bool(self.value.match(row))


class RegexSearchMatcher(BaseRegexMatcher):

    def match(self, row):
        return bool(self.value.search(row))



def regex_matcher_factory(value):
        if value.startswith('^'):
            return RegexMatchMatcher(value)
        return RegexSearchMatcher(value)


def string_matcher_factory(value):
    if value.startswith("^"):
        return StringPrefixMatcher(value)
    elif value.endswith('$'):
        return StringSuffixMatcher(value)
    return StringMatcher(value)


def matcher_factory(raw_value):
    method, value = raw_value.split("=")
    if method == METHOD_REGEX:
        return regex_matcher_factory(value)

    if method == METHOD_STRING:
        return string_matcher_factory(value)

    raise ValueError('unknown search method')


