# -*- coding: utf-8 -*-
# Description: tomcat netdata python.d module
# Author: Pawel Krupa (paulfantom)

from base import UrlService
import xml.etree.ElementTree as ET  # phone home...

# default module values (can be overridden per job in `config`)
# update_every = 2
priority = 60000
retries = 60

# charts order (can be overridden if you want less charts, or different order)
ORDER = ['accesses', 'volume', 'threads', 'jvm']

CHARTS = {
    'accesses': {
        'options': [None, "tomcat requests", "requests/s", "statistics", "tomcat.accesses", "area"],
        'lines': [
            ["accesses"]
        ]},
    'volume': {
        'options': [None, "tomcat volume", "KB/s", "volume", "tomcat.volume", "area"],
        'lines': [
            ["volume", None, 'incremental']
        ]},
    'threads': {
        'options': [None, "tomcat threads", "current threads", "statistics", "tomcat.threads", "line"],
        'lines': [
            ["current", None, "absolute"],
            ["busy", None, "absolute"]
        ]},
    'jvm': {
        'options': [None, "JVM Free Memory", "MB", "statistics", "tomcat.jvm", "area"],
        'lines': [
            ["jvm", None, "absolute"]
        ]}
}


class Service(UrlService):
    def __init__(self, configuration=None, name=None):
        UrlService.__init__(self, configuration=configuration, name=name)
        if len(self.url) == 0:
            self.url = "http://localhost:8080/manager/status?XML=true"
        self.order = ORDER
        self.definitions = CHARTS
        # get port from url
        self.port = 0
        for i in self.url.split('/'):
            try:
                int(i[-1])
                self.port = i.split(':')[-1]
                break
            except:
                pass
        if self.port == 0:
            self.port = 80

    def _get_data(self):
        """
        Format data received from http request
        :return: dict
        """
        try:
            raw = self._get_raw_data()
            data = ET.fromstring(raw)
            memory = data.find('./jvm/memory')
            threads = data.find("./connector[@name='\"http-bio-" + str(self.port) + "\"']/threadInfo")
            requests = data.find("./connector[@name='\"http-bio-" + str(self.port) + "\"']/requestInfo")

            return {'accesses': requests.attrib['requestCount'],
                    'volume': requests.attrib['bytesSent'],
                    'current': threads.attrib['currentThreadCount'],
                    'busy': threads.attrib['currentThreadsBusy'],
                    'jvm': memory.attrib['free']}
        except (ValueError, AttributeError):
            return None
        except Exception as e:
            self.debug(str(e))
