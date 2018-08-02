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
MONIT_SERVICE_NAMES = [ 'Filesystem', 'Directory', 'File', 'Process', 'Host', 'System', 'Fifo', 'Program', 'Net' ]
DEFAULT_SERVICES_IDS = [ 0, 1, 2, 3, 4, 6, 7, 8 ]

# charts order (can be overridden if you want less charts, or different order)
ORDER = [ 'filesystem', 'directory', 'file', 'process', 'process_uptime', 'process_threads', 'process_children', 'host', 'host_latency', 'system', 'fifo', 'program', 'net' ]
CHARTS = {
    # id: {
    #     'options': [name, title, units, family, context, charttype],
    #     'lines': [
    #         [unique_dimension_name, name, algorithm, multiplier, divisor]
    #     ]}
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
        'options': ['processes uptime', 'Processes uptime', 'seconds', 'applications', 'monit.process_uptime', 'line', 'hidden'],
        'lines': []
    },
    'process_threads': {
        'options': ['processes threads', 'Processes threads', 'threads', 'applications', 'monit.process_threads', 'line'],
        'lines': []
    },
    'process_children': {
        'options': ['processes childrens', 'Child processes', 'childrens', 'applications', 'monit.process_childrens', 'line'],
        'lines': []
    },
    'host': {
        'options': ['hosts', 'Hosts', 'hosts', 'network', 'monit.hosts', 'line'],
        'lines': []
    },
    'host_latency': {
        'options': ['hosts latency', 'Hosts latency', 'milliseconds/s', 'network', 'monit.hosts', 'line'],
        'lines': []
    },
    'net': {
        'options': ['interfaces', 'Network interfaces and addresses', 'interfaces', 'network', 'monit.networks', 'line'],
        'lines': []
    },
}

class Service(UrlService):
    def __init__(self, configuration=None, name=None):
        UrlService.__init__(self, configuration=configuration, name=name)
        self.url = self.configuration.get('url', "http://127.0.0.1:2812/_status?format=xml&level=full")
        self.order = ORDER
        self.definitions = CHARTS

    def check(self):
        self._manager = self._build_manager()
        raw_data = self._get_raw_data()
        if not raw_data:
            return None
        try:
            xml = ET.fromstring(raw_data)
            return True
        except ET.ParseError:
            self.debug('%s is not a vaild XML page. Please add "_status?format=xml&level=full" to monit URL.' % self.url)
            return None

    def _get_data(self):
        raw_data = self._get_raw_data()
        if not raw_data:
            return None

        try:
            xml = ET.fromstring(raw_data)
        except ET.ParseError:
            self.debug('%s is not a vaild XML page. Please add "_status?format=xml&level=full" to monit URL.' % self.url)
            return None

        data = {}
        for svc_id in DEFAULT_SERVICES_IDS:
            svc_category = MONIT_SERVICE_NAMES[svc_id].lower()
            if svc_category == 'system':
                self.debug("Skipping service from 'System' category, because it's useless in graphs")
                continue

            xpath_query = "./service[@type='%d']" % (svc_id)
            self.debug("Searching for %s as %s" % (svc_category, xpath_query))
            for svc in xml.findall(xpath_query):
                svc_name = svc.find('name').text
                svc_status = svc.find('status').text
                svc_monitor = svc.find('monitor').text
                self.debug('=> found %s with type=%s, status=%s, monitoring=%s' % (svc_name, svc_id, svc_status, svc_monitor))

                dimension_key = svc_category + '_' + svc_name
                if dimension_key not in self.charts[svc_category]:
                    self.charts[svc_category].add_dimension([dimension_key, svc_name, 'absolute'])
                data[dimension_key] = 1 if svc_status == "0" and svc_monitor == "1" else 0

                if svc_category == 'process':
                    for node in ('uptime', 'threads', 'children'):
                        node_value = svc.find(node)
                        if node_value != None:
                            if node == 'uptime' and int(node_value.text) < 0:
                                self.debug('Skipping bugged metrics with negative uptime')
                                continue
                            dimension_key = 'process_%s_%s' % (node, svc_name)
                            if dimension_key not in self.charts['process_' + node]:
                                self.charts['process_' + node].add_dimension([dimension_key, svc_name, 'absolute'])
                            data[dimension_key] = int(node_value.text)

                if svc_category == 'host':
                    node_value = svc.find('./icmp/responsetime')
                    if node_value != None:
                        dimension_key = 'host_latency_%s' % (svc_name)
                        if dimension_key not in self.charts['host_latency']:
                            self.charts['host_latency'].add_dimension([dimension_key, svc_name, 'absolute', 1000, 1000000])
                        data[dimension_key] = float(node_value.text) * 1000000

        return data or None
