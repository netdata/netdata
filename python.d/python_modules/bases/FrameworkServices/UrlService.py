# -*- coding: utf-8 -*-
# Description:
# Author: Pawel Krupa (paulfantom)
# Author: Ilya Mashchenko (l2isbad)

import urllib3

from bases.FrameworkServices.SimpleService import SimpleService

try:
    urllib3.disable_warnings()
except AttributeError:
    pass


class UrlService(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.url = self.configuration.get('url')
        self.user = self.configuration.get('user')
        self.password = self.configuration.get('pass')
        self.proxy_user = self.configuration.get('proxy_user')
        self.proxy_password = self.configuration.get('proxy_pass')
        self.proxy_url = self.configuration.get('proxy_url')
        self.header = self.configuration.get('header')
        self.request_timeout = self.configuration.get('timeout', 1)
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
        try:
            return manager(**params)
        except (urllib3.exceptions.ProxySchemeUnknown, TypeError) as error:
            self.error('build_manager() error:', str(error))
            return None

    def _get_raw_data(self, url=None, manager=None):
        """
        Get raw data from http request
        :return: str
        """
        try:
            url = url or self.url
            manager = manager or self._manager
            response = manager.request(method='GET',
                                       url=url,
                                       timeout=self.request_timeout,
                                       retries=1,
                                       headers=manager.headers)
        except (urllib3.exceptions.HTTPError, TypeError, AttributeError) as error:
            self.error('Url: {url}. Error: {error}'.format(url=url, error=error))
            return None
        if response.status == 200:
            return response.data.decode()
        self.debug('Url: {url}. Http response status code: {code}'.format(url=url, code=response.status))
        return None

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
