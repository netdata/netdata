# -*- coding: utf-8 -*-
# Description: monit netdata python.d module
# Author: Evgeniy K. (n0guest)
# SPDX-License-Identifier: GPL-3.0+

import xml.etree.ElementTree as ET
from bases.FrameworkServices.UrlService import UrlService

# default module values (can be overridden per job in `config`)
# update_every = 2
priority = 60000
retries = 60

# see enum State_Type from monit.h (https://bitbucket.org/tildeslash/monit/src/master/src/monit.h)
MONIT_SERVICE_NAMES = ['Filesystem', 'Directory', 'File', 'Process', 'Host', 'System', 'Fifo', 'Program', 'Net']
DEFAULT_SERVICES_IDS = [0, 1, 2, 3, 4, 6, 7, 8]

# charts order (can be overridden if you want less charts, or different order)
ORDER = [
    'filesystem',
    'directory',
    'file',
    'process',
    'process_uptime',
    'process_threads',
    'process_children',
    'host',
    'host_latency',
    'system',
    'fifo',
    'program',
    'net'
]
CHARTS = {
    'filesystem': {
        'options': ['filesystems', 'Filesystems', 'filesystems', 'filesystem', 'monit.filesystems', 'line'],
        'lines': []
    },
    'directory': {
        'options': ['directories', 'Directories', 'directories', 'filesystem', 'monit.directories', 'line'],
        'lines': []
    },
    'file': {
        'options': ['files', 'Files', 'files', 'filesystem', 'monit.files', 'line'],
        'lines': []
    },
    'fifo': {
        'options': ['fifos', 'Pipes (fifo)', 'pipes', 'filesystem', 'monit.fifos', 'line'],
        'lines': []
    },
    'program': {
        'options': ['programs', 'Programs statuses', 'programs', 'applications', 'monit.programs', 'line'],
        'lines': []
    },
    'process': {
        'options': ['processes', 'Processes statuses', 'processes', 'applications', 'monit.services', 'line'],
        'lines': []
    },
    'process_uptime': {
        'options': ['processes uptime', 'Processes uptime', 'seconds', 'applications',
                    'monit.process_uptime', 'line', 'hidden'],
        'lines': []
    },
    'process_threads': {
        'options': ['processes threads', 'Processes threads', 'threads', 'applications',
                    'monit.process_threads', 'line'],
        'lines': []
    },
    'process_children': {
        'options': ['processes childrens', 'Child processes', 'childrens', 'applications',
                    'monit.process_childrens', 'line'],
        'lines': []
    },
    'host': {
        'options': ['hosts', 'Hosts', 'hosts', 'network', 'monit.hosts', 'line'],
        'lines': []
    },
    'host_latency': {
        'options': ['hosts latency', 'Hosts latency', 'milliseconds/s', 'network', 'monit.host_latency', 'line'],
        'lines': []
    },
    'net': {
        'options': ['interfaces', 'Network interfaces and addresses', 'interfaces', 'network',
                    'monit.networks', 'line'],
        'lines': []
    },
}


class Service(UrlService):
    def __init__(self, configuration=None, name=None):
        UrlService.__init__(self, configuration=configuration, name=name)
        base_url = self.configuration.get('url', 'http://localhost:2812')
        self.url = '{0}/_status?format=xml&level=full'.format(base_url)
        self.order = ORDER
        self.definitions = CHARTS

    def parse(self, data):
        try:
            xml = ET.fromstring(data)
        except ET.ParseError:
            self.error("URL {0} didn't return a vaild XML page. Please check your settings.".format(self.url))
            return None
        return xml

    def check(self):
        self._manager = self._build_manager()
        raw_data = self._get_raw_data()
        if not raw_data:
            return None
        return bool(self.parse(raw_data))

    def _get_data(self):
        raw_data = self._get_raw_data()
        if not raw_data:
            return None
        xml = self.parse(raw_data)
        if not xml:
            return None

        data = {}
        for service_id in DEFAULT_SERVICES_IDS:
            service_category = MONIT_SERVICE_NAMES[service_id].lower()
            if service_category == 'system':
                self.debug("Skipping service from 'System' category, because it's useless in graphs")
                continue

            xpath_query = "./service[@type='{0}']".format(service_id)
            self.debug('Searching for {0} as {1}'.format(service_category, xpath_query))
            for service_node in xml.findall(xpath_query):

                service_name = service_node.find('name').text
                service_status = service_node.find('status').text
                service_monitoring = service_node.find('monitor').text
                self.debug('=> found {0} with type={1}, status={2}, monitoring={3}'.format(service_name,
                           service_id, service_status, service_monitoring))

                dimension_key = service_category + '_' + service_name
                if dimension_key not in self.charts[service_category]:
                    self.charts[service_category].add_dimension([dimension_key, service_name, 'absolute'])
                data[dimension_key] = 1 if service_status == '0' and service_monitoring == '1' else 0

                if service_category == 'process':
                    for subnode in ('uptime', 'threads', 'children'):
                        subnode_value = service_node.find(subnode)
                        if subnode_value is None:
                            continue
                        if subnode == 'uptime' and int(subnode_value.text) < 0:
                            self.debug('Skipping bugged metrics with negative uptime (monit before v5.16')
                            continue
                        dimension_key = 'process_{0}_{1}'.format(subnode, service_name)
                        if dimension_key not in self.charts['process_' + subnode]:
                            self.charts['process_' + subnode].add_dimension([dimension_key, service_name, 'absolute'])
                        data[dimension_key] = int(subnode_value.text)

                if service_category == 'host':
                    subnode_value = service_node.find('./icmp/responsetime')
                    if subnode_value is None:
                        continue
                    dimension_key = 'host_latency_{0}'.format(service_name)
                    if dimension_key not in self.charts['host_latency']:
                        self.charts['host_latency'].add_dimension([dimension_key, service_name,
                                                                   'absolute', 1000, 1000000])
                    data[dimension_key] = float(subnode_value.text) * 1000000

        return data or None
