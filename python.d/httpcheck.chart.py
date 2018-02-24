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

# Latency
HTTP_RESPONSE_TIME = 'response_time'

# Error dimensions
HTTP_SUCCESS = 'success'
HTTP_UNEXPECTED_CONTENT = 'unexpected_content'
HTTP_UNEXPECTED_STATUS = 'unexpected_status'
HTTP_TIMEOUT = 'timeout'
HTTP_FAILED = 'failed'

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
            [HTTP_SUCCESS, 'success', 'absolute'],
            [HTTP_UNEXPECTED_CONTENT, 'unexpected content', 'absolute'],
            [HTTP_UNEXPECTED_STATUS, 'unexpected status', 'absolute'],
            [HTTP_TIMEOUT, 'timeout', 'absolute'],
            [HTTP_FAILED, 'failed', 'absolute']
        ]}
}


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
        data[HTTP_SUCCESS] = 0
        data[HTTP_UNEXPECTED_CONTENT] = 0
        data[HTTP_UNEXPECTED_STATUS] = 0
        data[HTTP_TIMEOUT] = 0
        data[HTTP_FAILED] = 0
        url = self.url
        try:
            retr = 1 if self.follow_redirect else False
            start = time.time()
            response = self._manager.request(
                method='GET', url=url, timeout=self.request_timeout, retries=retr, headers=self._manager.headers,
                redirect=self.follow_redirect
            )
            diff = time.time() - start
            data[HTTP_RESPONSE_TIME] = max(round(diff * 10000), 1)
            self.debug('Url: {url}. Host responded with status code {code} in {diff} s'.format(
                url=url, code=response.status, diff=diff
            ))

            if response.status in self.status_codes_accepted:
                content = response.data
                self.debug('Content: \n\n{content}\n'.format(content=content))
                if self.regex.search(content) is None:
                    self.debug('No match for regex \'{regex}\' found'.format(regex=self.regex.pattern))
                    data[HTTP_UNEXPECTED_CONTENT] = 1
                else:
                    data[HTTP_SUCCESS] = 1
            else:
                data[HTTP_UNEXPECTED_STATUS] = 1

        except urllib3.exceptions.NewConnectionError as error:
            self.debug("Connection failed: {url}. Error: {error}".format(url=url, error=error))
            data[HTTP_FAILED] = 1

        except (urllib3.exceptions.TimeoutError, urllib3.exceptions.PoolError) as error:
            self.debug("Connection timed out: {url}. Error: {error}".format(url=url, error=error))
            data[HTTP_TIMEOUT] = 1

        except urllib3.exceptions.HTTPError as error:
            self.debug("Connection failed: {url}. Error: {error}".format(url=url, error=error))
            data[HTTP_FAILED] = 1

        except (TypeError, AttributeError) as error:
            self.error('Url: {url}. Error: {error}'.format(url=url, error=error))
            return None

        return data
