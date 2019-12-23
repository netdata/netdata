# -*- coding: utf-8 -*-
# Description:
# Author: Pawel Krupa (paulfantom)
# Author: Ilya Mashchenko (ilyam8)
# SPDX-License-Identifier: GPL-3.0-or-later

import urllib3

from distutils.version import StrictVersion as version

from bases.FrameworkServices.SimpleService import SimpleService

try:
    urllib3.disable_warnings()
except AttributeError:
    pass

# https://github.com/urllib3/urllib3/blob/master/CHANGES.rst#19-2014-07-04
# New retry logic and urllib3.util.retry.Retry configuration object. (Issue https://github.com/urllib3/urllib3/pull/326)
URLLIB3_MIN_REQUIRED_VERSION = '1.9'
URLLIB3_VERSION = urllib3.__version__
URLLIB3 = 'urllib3'


def version_check():
    if version(URLLIB3_VERSION) >= version(URLLIB3_MIN_REQUIRED_VERSION):
        return

    err = '{0} version: {1}, minimum required version: {2}, please upgrade'.format(
        URLLIB3,
        URLLIB3_VERSION,
        URLLIB3_MIN_REQUIRED_VERSION,
    )
    raise Exception(err)


class UrlService(SimpleService):
    def __init__(self, configuration=None, name=None):
        version_check()
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.debug("{0} version: {1}".format(URLLIB3, URLLIB3_VERSION))
        self.url = self.configuration.get('url')
        self.user = self.configuration.get('user')
        self.password = self.configuration.get('pass')
        self.proxy_user = self.configuration.get('proxy_user')
        self.proxy_password = self.configuration.get('proxy_pass')
        self.proxy_url = self.configuration.get('proxy_url')
        self.method = self.configuration.get('method', 'GET')
        self.header = self.configuration.get('header')
        self.request_timeout = self.configuration.get('timeout', 1)
        self.respect_retry_after_header = self.configuration.get('respect_retry_after_header')
        self.tls_verify = self.configuration.get('tls_verify')
        self.tls_ca_file = self.configuration.get('tls_ca_file')
        self.tls_key_file = self.configuration.get('tls_key_file')
        self.tls_cert_file = self.configuration.get('tls_cert_file')
        self._manager = None

    def __make_headers(self, **header_kw):
        user = header_kw.get('user') or self.user
        password = header_kw.get('pass') or self.password
        proxy_user = header_kw.get('proxy_user') or self.proxy_user
        proxy_password = header_kw.get('proxy_pass') or self.proxy_password
        custom_header = header_kw.get('header') or self.header
        header_params = dict(keep_alive=True)
        proxy_header_params = dict()
        if user and password:
            header_params['basic_auth'] = '{user}:{password}'.format(user=user,
                                                                     password=password)
        if proxy_user and proxy_password:
            proxy_header_params['proxy_basic_auth'] = '{user}:{password}'.format(user=proxy_user,
                                                                                 password=proxy_password)
        try:
            header, proxy_header = urllib3.make_headers(**header_params), urllib3.make_headers(**proxy_header_params)
        except TypeError as error:
            self.error('build_header() error: {error}'.format(error=error))
            return None, None
        else:
            header.update(custom_header or dict())
            return header, proxy_header

    def _build_manager(self, **header_kw):
        header, proxy_header = self.__make_headers(**header_kw)
        if header is None or proxy_header is None:
            return None
        proxy_url = header_kw.get('proxy_url') or self.proxy_url
        if proxy_url:
            manager = urllib3.ProxyManager
            params = dict(proxy_url=proxy_url, headers=header, proxy_headers=proxy_header)
        else:
            manager = urllib3.PoolManager
            params = dict(headers=header)
        tls_cert_file = self.tls_cert_file
        if tls_cert_file:
            params['cert_file'] = tls_cert_file
            # NOTE: key_file is useless without cert_file, but
            #       cert_file may include the key as well.
            tls_key_file = self.tls_key_file
            if tls_key_file:
                params['key_file'] = tls_key_file
        tls_ca_file = self.tls_ca_file
        if tls_ca_file:
            params['ca_certs'] = tls_ca_file
        try:
            url = header_kw.get('url') or self.url
            is_https = url.startswith('https')
            if skip_tls_verify(is_https, self.tls_verify, tls_ca_file):
                params['ca_certs'] = None
                params['cert_reqs'] = 'CERT_NONE'
                if is_https:
                    params['assert_hostname'] = False
            return manager(**params)
        except (urllib3.exceptions.ProxySchemeUnknown, TypeError) as error:
            self.error('build_manager() error:', str(error))
            return None

    def _get_raw_data(self, url=None, manager=None, **kwargs):
        """
        Get raw data from http request
        :return: str
        """
        try:
            status, data = self._get_raw_data_with_status(url, manager, **kwargs)
        except Exception as error:
            self.error('Url: {url}. Error: {error}'.format(url=url or self.url, error=error))
            return None

        if status == 200:
            return data
        else:
            self.debug('Url: {url}. Http response status code: {code}'.format(url=url or self.url, code=status))
            return None

    def _get_raw_data_with_status(self, url=None, manager=None, retries=1, redirect=True, **kwargs):
        """
        Get status and response body content from http request. Does not catch exceptions
        :return: int, str
        """
        url = url or self.url
        manager = manager or self._manager
        retry = urllib3.Retry(retries)
        if hasattr(retry, 'respect_retry_after_header'):
            retry.respect_retry_after_header = bool(self.respect_retry_after_header)

        response = manager.request(
            method=self.method,
            url=url,
            timeout=self.request_timeout,
            retries=retry,
            headers=manager.headers,
            redirect=redirect,
            **kwargs
        )
        if isinstance(response.data, str):
            return response.status, response.data
        return response.status, response.data.decode(errors='ignore')

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

        try:
            data = self._get_data()
        except Exception as error:
            self.error('_get_data() failed. Url: {url}. Error: {error}'.format(url=self.url, error=error))
            return False

        if isinstance(data, dict) and data:
            return True
        self.error('_get_data() returned no data or type is not <dict>')
        return False


def skip_tls_verify(is_https, tls_verify, tls_ca_file):
    # default 'tls_verify' value is None
    # logic is:
    #   - never skip if there is 'tls_ca_file' file
    #   - skip by default for https
    #   - do not skip by default for http
    if tls_ca_file:
        return False
    if is_https and not tls_verify:
        return True
    return tls_verify is False
