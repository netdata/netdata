# -*- coding: utf-8 -*-
# Description: nginx log netdata python.d module
# Author: Pawel Krupa (paulfantom)

from base import LogService
import re

priority = 60000
retries = 60
# update_every = 3

ORDER = ['codes']
CHARTS = {
    'codes': {
        'options': [None, 'nginx status codes', 'requests/s', 'requests', 'nginx_log.codes', 'stacked'],
        'lines': [
            ["2xx", None, "incremental"],
            ["5xx", None, "incremental"],
            ["3xx", None, "incremental"],
            ["4xx", None, "incremental"],
            ["1xx", None, "incremental"],
            ["other", None, "incremental"]
        ]}
}


class Service(LogService):
    def __init__(self, configuration=None, name=None):
        LogService.__init__(self, configuration=configuration, name=name)
        if len(self.log_path) == 0:
            self.log_path = "/var/log/nginx/access.log"
        self.order = ORDER
        self.definitions = CHARTS
        pattern = r'" ([0-9]{3}) ?'
        #pattern = r'(?:" )([0-9][0-9][0-9]) ?'
        self.regex = re.compile(pattern)

        self.data = {
            '1xx': 0,
            '2xx': 0,
            '3xx': 0,
            '4xx': 0,
            '5xx': 0,
            'other': 0
        }

    def _get_data(self):
        """
        Parse new log lines
        :return: dict
        """
        try:
            raw = self._get_raw_data()
            if raw is None:
                return None
            elif not raw:
                return self.data
        except (ValueError, AttributeError):
            return None

        regex = self.regex
        for line in raw:
            code = regex.search(line)
            try:
                beginning = code.group(1)[0]
            except AttributeError:
                continue

            if beginning == '2':
                self.data["2xx"] += 1
            elif beginning == '3':
                self.data["3xx"] += 1
            elif beginning == '4':
                self.data["4xx"] += 1
            elif beginning == '5':
                self.data["5xx"] += 1
            elif beginning == '1':
                self.data["1xx"] += 1
            else:
                self.data["other"] += 1

        return self.data
