# -*- coding: utf-8 -*-
# Description: tomcat netdata python.d module
# Author: Pawel Krupa (paulfantom)
# Author: Wei He (Wing924)
# SPDX-License-Identifier: GPL-3.0-or-later

import xml.etree.ElementTree as ET

from bases.FrameworkServices.UrlService import UrlService

# default module values (can be overridden per job in `config`)
# update_every = 2
priority = 60000
retries = 60

# charts order (can be overridden if you want less charts, or different order)
ORDER = ['accesses', 'bandwidth', 'processing_time', 'threads', 'jvm', 'jvm_eden', 'jvm_survivor', 'jvm_tenured']

CHARTS = {
    'accesses': {
        'options': [None, 'Requests', 'requests/s', 'statistics', 'tomcat.accesses', 'area'],
        'lines': [
            ['requestCount', 'accesses', 'incremental'],
            ['errorCount', 'errors', 'incremental'],
        ]
    },
    'bandwidth': {
        'options': [None, 'Bandwidth', 'KB/s', 'statistics', 'tomcat.bandwidth', 'area'],
        'lines': [
            ['bytesSent', 'sent', 'incremental', 1, 1024],
            ['bytesReceived', 'received', 'incremental', 1, 1024],
        ]
    },
    'processing_time': {
        'options': [None, 'processing time', 'seconds', 'statistics', 'tomcat.processing_time', 'area'],
        'lines': [
            ['processingTime', 'processing time', 'incremental', 1, 1000]
        ]
    },
    'threads': {
        'options': [None, 'Threads', 'current threads', 'statistics', 'tomcat.threads', 'area'],
        'lines': [
            ['currentThreadCount', 'current', 'absolute'],
            ['currentThreadsBusy', 'busy', 'absolute']
        ]
    },
    'jvm': {
        'options': [None, 'JVM Memory Pool Usage', 'MB', 'memory', 'tomcat.jvm', 'stacked'],
        'lines': [
            ['free', 'free', 'absolute', 1, 1048576],
            ['eden_used', 'eden', 'absolute', 1, 1048576],
            ['survivor_used', 'survivor', 'absolute', 1, 1048576],
            ['tenured_used', 'tenured', 'absolute', 1, 1048576],
            ['code_cache_used', 'code cache', 'absolute', 1, 1048576],
            ['compressed_used', 'compressed', 'absolute', 1, 1048576],
            ['metaspace_used', 'metaspace', 'absolute', 1, 1048576],
        ]
    },
    'jvm_eden': {
        'options': [None, 'Eden Memory Usage', 'MB', 'memory', 'tomcat.jvm_eden', 'area'],
        'lines': [
            ['eden_used', 'used', 'absolute', 1, 1048576],
            ['eden_committed', 'committed', 'absolute', 1, 1048576],
            ['eden_max', 'max', 'absolute', 1, 1048576]
        ]
    },
    'jvm_survivor': {
        'options': [None, 'Survivor Memory Usage', 'MB', 'memory', 'tomcat.jvm_survivor', 'area'],
        'lines': [
            ['survivor_used', 'used', 'absolute', 1, 1048576],
            ['survivor_committed', 'committed', 'absolute', 1, 1048576],
            ['survivor_max', 'max', 'absolute', 1, 1048576]
        ]
    },
    'jvm_tenured': {
        'options': [None, 'Tenured Memory Usage', 'MB', 'memory', 'tomcat.jvm_tenured', 'area'],
        'lines': [
            ['tenured_used', 'used', 'absolute', 1, 1048576],
            ['tenured_committed', 'committed', 'absolute', 1, 1048576],
            ['tenured_max', 'max', 'absolute', 1, 1048576]
        ]
    }
}


class Service(UrlService):
    def __init__(self, configuration=None, name=None):
        UrlService.__init__(self, configuration=configuration, name=name)
        self.url = self.configuration.get('url', 'http://127.0.0.1:8080/manager/status?XML=true')
        self.connector_name = self.configuration.get('connector_name', None)
        self.order = ORDER
        self.definitions = CHARTS

    def _get_data(self):
        """
        Format data received from http request
        :return: dict
        """
        data = None
        raw_data = self._get_raw_data()
        if raw_data:
            try:
                xml = ET.fromstring(raw_data)
            except ET.ParseError:
                self.debug('%s is not a vaild XML page. Please add "?XML=true" to tomcat status page.' % self.url)
                return None
            data = {}

            jvm = xml.find('jvm')

            connector = None
            if self.connector_name:
                for conn in xml.findall('connector'):
                    if self.connector_name in conn.get('name'):
                        connector = conn
                        break
            else:
                connector = xml.find('connector')

            memory = jvm.find('memory')
            data['free'] = memory.get('free')
            data['total'] = memory.get('total')

            for pool in jvm.findall('memorypool'):
                name = pool.get('name')
                if 'Eden Space' in name:
                    data['eden_used'] = pool.get('usageUsed')
                    data['eden_committed'] = pool.get('usageCommitted')
                    data['eden_max'] = pool.get('usageMax')
                elif 'Survivor Space' in name:
                    data['survivor_used'] = pool.get('usageUsed')
                    data['survivor_committed'] = pool.get('usageCommitted')
                    data['survivor_max'] = pool.get('usageMax')
                elif 'Tenured Gen' in name or 'Old Gen' in name:
                    data['tenured_used'] = pool.get('usageUsed')
                    data['tenured_committed'] = pool.get('usageCommitted')
                    data['tenured_max'] = pool.get('usageMax')
                elif name == 'Code Cache':
                    data['code_cache_used'] = pool.get('usageUsed')
                    data['code_cache_committed'] = pool.get('usageCommitted')
                    data['code_cache_max'] = pool.get('usageMax')
                elif name == 'Compressed':
                    data['compressed_used'] = pool.get('usageUsed')
                    data['compressed_committed'] = pool.get('usageCommitted')
                    data['compressed_max'] = pool.get('usageMax')
                elif name == 'Metaspace':
                    data['metaspace_used'] = pool.get('usageUsed')
                    data['metaspace_committed'] = pool.get('usageCommitted')
                    data['metaspace_max'] = pool.get('usageMax')

            if connector:
                thread_info = connector.find('threadInfo')
                data['currentThreadsBusy'] = thread_info.get('currentThreadsBusy')
                data['currentThreadCount'] = thread_info.get('currentThreadCount')

                request_info = connector.find('requestInfo')
                data['processingTime'] = request_info.get('processingTime')
                data['requestCount'] = request_info.get('requestCount')
                data['errorCount'] = request_info.get('errorCount')
                data['bytesReceived'] = request_info.get('bytesReceived')
                data['bytesSent'] = request_info.get('bytesSent')

        return data or None
