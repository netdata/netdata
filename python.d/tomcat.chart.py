# -*- coding: utf-8 -*-
# Description: tomcat netdata python.d module
# Author: Pawel Krupa (paulfantom)

from base import UrlService
from re import compile

try:
    from urlparse import urlparse
except ImportError:
    from urllib.parse import urlparse

# default module values (can be overridden per job in `config`)
# update_every = 2
priority = 60000
retries = 60

# charts order (can be overridden if you want less charts, or different order)
ORDER = ['accesses', 'volume', 'threads', 'jvm']

CHARTS = {
    'accesses': {
        'options': [None, "Requests", "requests/s", "statistics", "tomcat.accesses", "area"],
        'lines': [
            ["requestCount", 'accesses', 'incremental']
        ]},
    'volume': {
        'options': [None, "Volume", "KB/s", "volume", "tomcat.volume", "area"],
        'lines': [
            ["bytesSent", 'volume', 'incremental', 1, 1024]
        ]},
    'threads': {
        'options': [None, "Threads", "current threads", "statistics", "tomcat.threads", "line"],
        'lines': [
            ["currentThreadCount", 'current', "absolute"],
            ["currentThreadsBusy", 'busy', "absolute"]
        ]},
    'jvm': {
        'options': [None, "JVM Free Memory", "MB", "statistics", "tomcat.jvm", "area"],
        'lines': [
            ["free", None, "absolute", 1, 1048576]
        ]}
}


class Service(UrlService):
    def __init__(self, configuration=None, name=None):
        UrlService.__init__(self, configuration=configuration, name=name)
        self.url = self.configuration.get('url', "http://127.0.0.1:8080/manager/status?XML=true")
        self.order = ORDER
        self.definitions = CHARTS

    def check(self):
        netloc = urlparse(self.url).netloc.rpartition(':')
        if netloc[1] == ':': port = netloc[2]
        else: port = 80
        
        self.regex_jvm = compile(r'<jvm>.*?</jvm>')
        self.regex_connector = compile(r'[a-z-]+%s.*?/connector' % port)
        self.regex = compile(r'([\w]+)=\\?[\'\"](\d+)\\?[\'\"]')
        
        return UrlService.check(self)

    def _get_data(self):
        """
        Format data received from http request
        :return: dict
        """
        data = self._get_raw_data()
        if data:
            jvm = self.regex_jvm.findall(data) or ['']
            connector = self.regex_connector.findall(data) or ['']
            data = dict(self.regex.findall(''.join([jvm[0], connector[0]])))
        
        return data or None

