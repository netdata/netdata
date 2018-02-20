# -*- coding: utf-8 -*-
# Description: http check netdata python.d module
# Original Author: ccremer (github.com/ccremer)

import urllib3
import time
import re

from bases.FrameworkServices.UrlService import UrlService

try:
    urllib3.disable_warnings()
except AttributeError:
    pass

# default module values (can be overridden per job in `config`)
# update_every = 2
priority = 60000
retries = 60

HTTP_RESPONSE_TIME = 'http_responsetime'
HTTP_ERROR = 'http_error'

ORDER = ['responsetime', 'error']

CHARTS = {
    'responsetime': {
        'options': [None, 'HTTP response time', 'ms', 'response time', 'httpcheck.responsetime', 'line'],
        'lines': [
            [HTTP_RESPONSE_TIME, 'response time', 'absolute', 100, 1000]
        ]},
    'error': {
        'options': [None, 'HTTP check error code', 'code', 'error', 'httpcheck.error', 'line'],
        'lines': [
            [HTTP_ERROR, 'error', 'absolute']
        ]}
}

CONTENT_MISMATCH = 1
STATUS_NOT_ACCEPTED = 2
CONNECTION_TIMED_OUT = 3
CONNECTION_FAILED = 4


class Service(UrlService):
    def __init__(self, configuration=None, name=None):
        UrlService.__init__(self, configuration=configuration, name=name)
        self.url = self.configuration.get('url', None)
        self.regex = re.compile(self.configuration.get('regex', '.*'))
        self.status_codes_accepted = self.configuration.get('status_accepted', [200])
        self.follow_redirect = self.configuration.get('redirect', False)
        self.order = ORDER
        self.definitions = CHARTS
        self._manager = None

    def check(self):
        """
        Format configuration data and try to connect to server
        :return: boolean
        """
        if not (self.url and isinstance(self.url, str)):
            self.error('URL is not defined or type is not <str>')
            return False

        self._manager = self._build_manager()
        if not self._manager:
            return False

        self.info('Enabled {url} with (redirect={redirect}, status={accepted}, interval={update}, timeout={timeout}, '
                  'regex={regex})'
                  .format(url=self.url, redirect=self.follow_redirect, accepted=self.status_codes_accepted,
                          update=self.update_every, timeout=self.request_timeout, regex=self.regex.pattern))
        return True

    def _get_data(self):
        """
        Format data received from http request
        :return: dict
        """
        data = dict()
        data[HTTP_RESPONSE_TIME] = 0
        data[HTTP_ERROR] = 0
        url = self.url
        try:
            start = time.time()
            response = self._manager.request(
                method='GET', url=url, timeout=self.request_timeout, retries=1, headers=self._manager.headers,
                redirect=self.follow_redirect
            )
            diff = time.time() - start
            data[HTTP_RESPONSE_TIME] = max(round(diff * 10000), 1)
            self.debug('Url: {url}. Host responded with status code {code} in {diff} ms'.format(
                url=url, code=response.status, diff=diff
            ))

            if response.status in self.status_codes_accepted:
                content = response.data
                self.debug('Content: \n\n{content}\n'.format(content=content))
                if self.regex.search(content) is None:
                    self.debug('No match for regex \'{regex}\' found'.format(regex=self.regex.pattern))
                    data[HTTP_ERROR] = CONTENT_MISMATCH
            else:
                data[HTTP_ERROR] = STATUS_NOT_ACCEPTED

        except urllib3.exceptions.TimeoutError:
            self.debug("Connection timed out: {url}".format(url=url))
            data[HTTP_ERROR] = CONNECTION_TIMED_OUT

        except urllib3.exceptions.HTTPError:
            self.debug("Connection timed out: {url}".format(url=url))
            data[HTTP_ERROR] = CONNECTION_FAILED

        except (TypeError, AttributeError) as error:
            self.error('Url: {url}. Error: {error}'.format(url=url, error=error))
            return None

        return data
