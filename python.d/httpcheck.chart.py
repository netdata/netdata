# -*- coding: utf-8 -*-
# Description: http check netdata python.d module
# Original Author: ccremer (github.com/ccremer)

import urllib3
import re

try:
    from time import monotonic as time
except ImportError:
    from time import time

from bases.FrameworkServices.UrlService import UrlService

# default module values (can be overridden per job in `config`)
update_every = 3
priority = 60000
retries = 60

# Response
HTTP_RESPONSE_TIME = 'time'
HTTP_RESPONSE_LENGTH = 'length'

# Status dimensions
HTTP_SUCCESS = 'success'
HTTP_BAD_CONTENT = 'bad_content'
HTTP_BAD_STATUS = 'bad_status'
HTTP_TIMEOUT = 'timeout'
HTTP_NO_CONNECTION = 'no_connection'

ORDER = ['response_time', 'response_length', 'status']

CHARTS = {
    'response_time': {
        'options': [None, 'HTTP response time', 'ms', 'response', 'httpcheck.responsetime', 'line'],
        'lines': [
            [HTTP_RESPONSE_TIME, 'time', 'absolute', 100, 1000]
        ]},
    'response_length': {
        'options': [None, 'HTTP response body length', 'characters', 'response', 'httpcheck.responselength', 'line'],
        'lines': [
            [HTTP_RESPONSE_LENGTH, 'length', 'absolute']
        ]},
    'status': {
        'options': [None, 'HTTP status', 'boolean', 'status', 'httpcheck.status', 'line'],
        'lines': [
            [HTTP_SUCCESS, 'success', 'absolute'],
            [HTTP_BAD_CONTENT, 'bad content', 'absolute'],
            [HTTP_BAD_STATUS, 'bad status', 'absolute'],
            [HTTP_TIMEOUT, 'timeout', 'absolute'],
            [HTTP_NO_CONNECTION, 'no connection', 'absolute']
        ]}
}


class Service(UrlService):
    def __init__(self, configuration=None, name=None):
        UrlService.__init__(self, configuration=configuration, name=name)
        pattern = self.configuration.get('regex')
        self.regex = re.compile(pattern) if pattern else None
        self.status_codes_accepted = self.configuration.get('status_accepted', [200])
        self.follow_redirect = self.configuration.get('redirect', True)
        self.order = ORDER
        self.definitions = CHARTS

    def _get_data(self):
        """
        Format data received from http request
        :return: dict
        """
        data = dict()
        data[HTTP_SUCCESS] = 0
        data[HTTP_BAD_CONTENT] = 0
        data[HTTP_BAD_STATUS] = 0
        data[HTTP_TIMEOUT] = 0
        data[HTTP_NO_CONNECTION] = 0
        url = self.url
        try:
            start = time()
            status, content = self._get_raw_data_with_status(retries=1 if self.follow_redirect else False,
                                                             redirect=self.follow_redirect)
            diff = time() - start
            data[HTTP_RESPONSE_TIME] = max(round(diff * 10000), 0)
            self.debug('Url: {url}. Host responded with status code {code} in {diff} s'.format(
                url=url, code=status, diff=diff
            ))
            self.process_response(content, data, status)

        except urllib3.exceptions.NewConnectionError as error:
            self.debug("Connection failed: {url}. Error: {error}".format(url=url, error=error))
            data[HTTP_NO_CONNECTION] = 1

        except (urllib3.exceptions.TimeoutError, urllib3.exceptions.PoolError) as error:
            self.debug("Connection timed out: {url}. Error: {error}".format(url=url, error=error))
            data[HTTP_TIMEOUT] = 1

        except urllib3.exceptions.HTTPError as error:
            self.debug("Connection failed: {url}. Error: {error}".format(url=url, error=error))
            data[HTTP_NO_CONNECTION] = 1

        except (TypeError, AttributeError) as error:
            self.error('Url: {url}. Error: {error}'.format(url=url, error=error))
            return None

        return data

    def process_response(self, content, data, status):
        data[HTTP_RESPONSE_LENGTH] = len(content)
        self.debug('Content: \n\n{content}\n'.format(content=content))
        if status in self.status_codes_accepted:
            if self.regex and self.regex.search(content) is None:
                self.debug("No match for regex '{regex}' found".format(regex=self.regex.pattern))
                data[HTTP_BAD_CONTENT] = 1
            else:
                data[HTTP_SUCCESS] = 1
        else:
            data[HTTP_BAD_STATUS] = 1
