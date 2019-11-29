# -*- coding: utf-8 -*-
# Description: monit netdata python.d module
# Author: Evgeniy K. (n0guest)
# SPDX-License-Identifier: GPL-3.0-or-later

import xml.etree.ElementTree as ET
from collections import namedtuple

from bases.FrameworkServices.UrlService import UrlService

MonitType = namedtuple('MonitType', ('index', 'name'))

# see enum Service_Type from monit.h (https://bitbucket.org/tildeslash/monit/src/master/src/monit.h)
# typedef enum {
#         Service_Filesystem = 0,
#         Service_Directory,
#         Service_File,
#         Service_Process,
#         Service_Host,
#         Service_System,
#         Service_Fifo,
#         Service_Program,
#         Service_Net,
#         Service_Last = Service_Net
# } __attribute__((__packed__)) Service_Type;

TYPE_FILESYSTEM = MonitType(0, 'filesystem')
TYPE_DIRECTORY = MonitType(1, 'directory')
TYPE_FILE = MonitType(2, 'file')
TYPE_PROCESS = MonitType(3, 'process')
TYPE_HOST = MonitType(4, 'host')
TYPE_SYSTEM = MonitType(5, 'system')
TYPE_FIFO = MonitType(6, 'fifo')
TYPE_PROGRAM = MonitType(7, 'program')
TYPE_NET = MonitType(8, 'net')

TYPES = (
    TYPE_FILESYSTEM,
    TYPE_DIRECTORY,
    TYPE_FILE,
    TYPE_PROCESS,
    TYPE_HOST,
    TYPE_SYSTEM,
    TYPE_FIFO,
    TYPE_PROGRAM,
    TYPE_NET,
)

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
        'options': ['hosts latency', 'Hosts latency', 'milliseconds', 'network', 'monit.host_latency', 'line'],
        'lines': []
    },
    'net': {
        'options': ['interfaces', 'Network interfaces and addresses', 'interfaces', 'network',
                    'monit.networks', 'line'],
        'lines': []
    },
}


class BaseMonitService(object):
    def __init__(self, typ, name, status, monitor):
        self.type = typ
        self.name = name
        self.status = status
        self.monitor = monitor

    def __repr__(self):
        return 'MonitService({0}:{1})'.format(self.type.name, self.name)

    def __eq__(self, other):
        if not isinstance(other, BaseMonitService):
            return False
        return self.type == other.type and self.name == other.name

    def __ne__(self, other):
        return not self == other

    def __hash__(self):
        return hash(repr(self))

    def is_running(self):
        return self.status == '0' and self.monitor == '1'

    def key(self):
        return '{0}_{1}'.format(self.type.name, self.name)

    def data(self):
        return {self.key(): int(self.is_running())}


class ProcessMonitService(BaseMonitService):
    def __init__(self, typ, name, status, monitor):
        super(ProcessMonitService, self).__init__(typ, name, status, monitor)
        self.uptime = None
        self.threads = None
        self.children = None

    def __eq__(self, other):
        return super(ProcessMonitService, self).__eq__(other)

    def __ne__(self, other):
        return super(ProcessMonitService, self).__ne__(other)

    def __hash__(self):
        return super(ProcessMonitService, self).__hash__()

    def uptime_key(self):
        return 'process_uptime_{0}'.format(self.name)

    def threads_key(self):
        return 'process_threads_{0}'.format(self.name)

    def children_key(self):
        return 'process_children_{0}'.format(self.name)

    def data(self):
        base_data = super(ProcessMonitService, self).data()
        # skipping bugged metrics with negative uptime (monit before v5.16)
        uptime = self.uptime if self.uptime and int(self.uptime) >= 0 else None
        data = {
            self.uptime_key(): uptime,
            self.threads_key(): self.threads,
            self.children_key(): self.children,
        }
        data.update(base_data)

        return data


class HostMonitService(BaseMonitService):
    def __init__(self, typ, name, status, monitor):
        super(HostMonitService, self).__init__(typ, name, status, monitor)
        self.latency = None

    def __eq__(self, other):
        return super(HostMonitService, self).__eq__(other)

    def __ne__(self, other):
        return super(HostMonitService, self).__ne__(other)

    def __hash__(self):
        return super(HostMonitService, self).__hash__()

    def latency_key(self):
        return 'host_latency_{0}'.format(self.name)

    def data(self):
        base_data = super(HostMonitService, self).data()
        latency = float(self.latency) * 1000000 if self.latency else None
        data = {self.latency_key(): latency}
        data.update(base_data)

        return data


class Service(UrlService):
    def __init__(self, configuration=None, name=None):
        UrlService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        base_url = self.configuration.get('url', "http://localhost:2812")
        self.url = '{0}/_status?format=xml&level=full'.format(base_url)
        self.active_services = list()

    def parse(self, raw):
        try:
            root = ET.fromstring(raw)
        except ET.ParseError:
            self.error("URL {0} didn't return a valid XML page. Please check your settings.".format(self.url))
            return None
        return root

    def _get_data(self):
        raw = self._get_raw_data()
        if not raw:
            return None

        root = self.parse(raw)
        if root is None:
            return None

        services = self.get_services(root)
        if not services:
            return None

        if len(self.charts) > 0:
            self.update_charts(services)

        data = dict()

        for svc in services:
            data.update(svc.data())

        return data

    def get_services(self, root):
        services = list()

        for typ in TYPES:
            if typ == TYPE_SYSTEM:
                self.debug("skipping service from '{0}' category, it's useless in graphs".format(TYPE_SYSTEM.name))
                continue

            xpath_query = "./service[@type='{0}']".format(typ.index)
            self.debug('Searching for {0} as {1}'.format(typ.name, xpath_query))

            for svc_root in root.findall(xpath_query):
                svc = create_service(svc_root, typ)
                self.debug('=> found {0} with type={1}, status={2}, monitoring={3}'.format(
                    svc.name, svc.type.name, svc.status, svc.monitor))

                services.append(svc)

        return services

    def update_charts(self, services):
        remove = [svc for svc in self.active_services if svc not in services]
        add = [svc for svc in services if svc not in self.active_services]

        self.remove_services_from_charts(remove)
        self.add_services_to_charts(add)

        self.active_services = services

    def add_services_to_charts(self, services):
        for svc in services:
            if svc.type == TYPE_HOST:
                self.charts['host_latency'].add_dimension([svc.latency_key(), svc.name, 'absolute', 1000, 1000000])
            if svc.type == TYPE_PROCESS:
                self.charts['process_uptime'].add_dimension([svc.uptime_key(), svc.name])
                self.charts['process_threads'].add_dimension([svc.threads_key(), svc.name])
                self.charts['process_children'].add_dimension([svc.children_key(), svc.name])
            self.charts[svc.type.name].add_dimension([svc.key(), svc.name])

    def remove_services_from_charts(self, services):
        for svc in services:
            if svc.type == TYPE_HOST:
                self.charts['host_latency'].del_dimension(svc.latency_key(), False)
            if svc.type == TYPE_PROCESS:
                self.charts['process_uptime'].del_dimension(svc.uptime_key(), False)
                self.charts['process_threads'].del_dimension(svc.threads_key(), False)
                self.charts['process_children'].del_dimension(svc.children_key(), False)
            self.charts[svc.type.name].del_dimension(svc.key(), False)


def create_service(root, typ):
    if typ == TYPE_HOST:
        return create_host_service(root)
    elif typ == TYPE_PROCESS:
        return create_process_service(root)
    return create_base_service(root, typ)


def create_host_service(root):
    svc = HostMonitService(
        TYPE_HOST,
        root.find('name').text,
        root.find('status').text,
        root.find('monitor').text,
    )

    latency = root.find('./icmp/responsetime')
    if latency is not None:
        svc.latency = latency.text

    return svc


def create_process_service(root):
    svc = ProcessMonitService(
        TYPE_PROCESS,
        root.find('name').text,
        root.find('status').text,
        root.find('monitor').text,
    )

    uptime = root.find('uptime')
    if uptime is not None:
        svc.uptime = uptime.text

    threads = root.find('threads')
    if threads is not None:
        svc.threads = threads.text

    children = root.find('children')
    if children is not None:
        svc.children = children.text

    return svc


def create_base_service(root, typ):
    return BaseMonitService(
        typ,
        root.find('name').text,
        root.find('status').text,
        root.find('monitor').text,
    )
