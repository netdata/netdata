# -*- coding: utf-8 -*-
# Description: ntp netdata python.d module
# Author: Sven Mäder (rda0)

import socket
import struct
import re

from itertools import cycle
from bases.FrameworkServices.SocketService import SocketService

# default module values
update_every = 1
priority = 60000
retries = 60

# NTP Control Message Protocol constants
MODE = 6
HEADER_FORMAT = '!BBHHHHH'
HEADER_LEN = 12
OPCODES = {
    'readstat': 1,
    'readvar': 2
}

# Maximal dimension precision
PRECISION = 1000000

# Static charts
ORDER = [
    'sys_offset',
    'sys_jitter',
    'sys_frequency',
    'sys_wander',
    'sys_rootdelay',
    'sys_rootdisp',
    'sys_stratum',
    'sys_tc',
    'sys_precision'
]

CHARTS = {
    'sys_offset': {
        'options': [None, "Combined offset of server relative to this host", "ms", 'system', 'ntp.sys_offset', 'area'],
        'lines': [
            ['offset', 'offset', 'absolute', 1, PRECISION]
        ]},
    'sys_jitter': {
        'options': [None, "Combined system jitter and clock jitter", "ms", 'system', 'ntp.sys_jitter', 'line'],
        'lines': [
            ['sys_jitter', 'system', 'absolute', 1, PRECISION],
            ['clk_jitter', 'clock', 'absolute', 1, PRECISION]
        ]},
    'sys_frequency': {
        'options': [None, "Frequency offset relative to hardware clock", "ppm", 'system', 'ntp.sys_frequency', 'area'],
        'lines': [
            ['frequency', 'frequency', 'absolute', 1, PRECISION]
        ]},
    'sys_wander': {
        'options': [None, "Clock frequency wander", "ppm", 'system', 'ntp.sys_wander', 'area'],
        'lines': [
            ['clk_wander', 'clock', 'absolute', 1, PRECISION]
        ]},
    'sys_rootdelay': {
        'options': [None, "Total roundtrip delay to the primary reference clock", "ms", 'system', 'ntp.sys_rootdelay', 'area'],
        'lines': [
            ['rootdelay', 'delay', 'absolute', 1, PRECISION]
        ]},
    'sys_rootdisp': {
        'options': [None, "Total root dispersion to the primary reference clock", "ms", 'system', 'ntp.sys_rootdisp', 'area'],
        'lines': [
            ['rootdisp', 'dispersion', 'absolute', 1, PRECISION]
        ]},
    'sys_stratum': {
        'options': [None, "Stratum (1-15)", "1", 'system', 'ntp.sys_stratum', 'line'],
        'lines': [
            ['stratum', 'stratum', 'absolute', 1, PRECISION]
        ]},
    'sys_tc': {
        'options': [None, "Time constant and poll exponent (3-17)", "log2 s", 'system', 'ntp.sys_tc', 'line'],
        'lines': [
            ['tc', 'current', 'absolute', 1, PRECISION],
            ['mintc', 'minimum', 'absolute', 1, PRECISION]
        ]},
    'sys_precision': {
        'options': [None, "Precision", "log2 s", 'system', 'ntp.sys_precision', 'line'],
        'lines': [
            ['precision', 'precision', 'absolute', 1, PRECISION]
        ]}
}

# Dynamic charts templates
PEER_PREFIX = 'peer'

PEER_DIMENSIONS = [
    ['offset', 'Filter offset', 'ms'],
    ['delay', 'Filter delay', 'ms'],
    ['dispersion', 'Filter dispersion', 'ms'],
    ['jitter', 'Filter jitter', 'ms'],
    ['xleave', 'Interleave delay', 'ms'],
    ['rootdelay', 'Total roundtrip delay to the primary reference clock', 'ms'],
    ['rootdisp', 'Total root dispersion to the primary reference clock', 'ms'],
    ['stratum', 'Stratum (1-15)', '1'],
    ['hmode', 'Host mode (1-6)', '1'],
    ['pmode', 'Peer mode (1-5)', '1'],
    ['hpoll', 'Host poll exponent', 'log2 s'],
    ['ppoll', 'Peer poll exponent', 'log2 s'],
    ['precision', 'Precision', 'log2 s']
]


class Peer(object):
    """
    Class to hold peer data required in _get_data
    """
    def __init__(self, peer_id, name, request):
        self.id = peer_id
        self.name = name
        self.request = request


class Service(SocketService):
    def __init__(self, configuration=None, name=None):
        SocketService.__init__(self, configuration=configuration, name=name)
        self.port = 'ntp'
        self.dgram_socket = True
        self.request_systemvars = None
        self.peers = None
        self.peer_error = 0
        self.regex_srcadr = re.compile(r'srcadr=([A-Za-z0-9.-]+)')
        self.regex_data = re.compile(r'([a-z_]+)=([0-9-]+(?:\.[0-9]+)?)(?=,)')
        self.order = None
        self.definitions = None
        self.peer_names = self.configuration.get('peer_names', True)
        peer_filter_start = r'^((0\.0\.0\.0)|('
        peer_filter_end = r'))$'
        peer_filter_default = r'127\..*'
        peer_filter_custom = str(self.configuration.get('peer_filter', peer_filter_default))

        try:
            self.regex_peer_filter = re.compile(peer_filter_start + peer_filter_custom + peer_filter_end)
        except re.error as error:
            self.error('Pattern compile error: %s, Using defaults.' % str(error))
            self.regex_peer_filter = re.compile(peer_filter_start + peer_filter_default + peer_filter_end)

    def create_charts(self):
        """
        Creates the charts dynamically.
        Checks ntp for available peers.
        Adds all peers whith valid data.
        """
        # Create systemvars charts
        self.order = list(ORDER)
        self.definitions = dict(CHARTS)

        # Get peer ids
        self.request = self.get_header(0, 'readstat')
        peer_ids = self.get_peer_ids(self._get_raw_data(raw=True))

        # Get peers
        peers = self.get_peers(peer_ids)

        # Create peer charts
        if peers:
            charts = dict()

            for dimension in PEER_DIMENSIONS:
                chart_id = '_'.join([PEER_PREFIX, dimension[0]])
                context = '.'.join(['ntp', chart_id])
                title = dimension[1]
                units = dimension[2]
                lines = list()

                for peer in peers:
                    unique_dimension_id = '_'.join([peer.name, dimension[0]])
                    line = [unique_dimension_id, peer.name, 'absolute', 1, PRECISION]
                    lines.append(line)
                charts[chart_id] = dict()
                charts[chart_id]['options'] = [None, title, units, 'peers', context, 'line']
                charts[chart_id]['lines'] = lines

            self.order += ['_'.join([PEER_PREFIX, d[0]]) for d in PEER_DIMENSIONS]
            self.definitions.update(charts)
            self.peers = cycle(peers)
        else:
            self.peers = None

    def get_peers(self, peer_ids):
        """
        Figures out the possible local domain name.
        Queries each peer once to get data for charts.
        Replace the peer srcadr with the possible hostname.
        Returns all peers whith valid data.
        """
        if peer_ids:
            peer_ids.sort()

        domain = None

        # Get the local domain name
        if self.peer_names:
            try:
                hostname = socket.gethostname()
                fqdn = socket.getfqdn()
                if fqdn.startswith(hostname):
                    domain = fqdn[len(hostname):]
            except socket.error:
                self.error('Error getting local domain')

        peers = list()

        # Get peer data
        for peer_id in peer_ids:
            request = self.get_header(peer_id, 'readvar')
            self.request = request
            raw = self._get_raw_data()
            if not raw:
                continue

            data = self.get_data_from_raw(raw)
            if not data:
                continue

            match_srcadr = self.regex_srcadr.search(raw)
            if not match_srcadr:
                continue

            name = match_srcadr.group(1)
            match_peer_filter = self.regex_peer_filter.search(name)
            if match_peer_filter:
                continue

            if domain:
                try:
                    name = socket.gethostbyaddr(name)[0]

                    match_peer_filter = self.regex_peer_filter.search(name)
                    if match_peer_filter:
                        continue

                    if len(name) > len(domain) and name.endswith(domain):
                        name = name[:-len(domain)]
                except (IndexError, socket.error):
                    self.error('Failed to reverse lookup address')

            name = name.replace('.', '-')

            peers.append(Peer(peer_id, name, request))

        return peers

    def check(self):
        """
        Checks if we can get valid systemvars.
        If not, returns None to disable module.
        """
        self._parse_config()

        self.request_systemvars = self.get_header(0, 'readvar')
        self.request = self.request_systemvars
        raw_systemvars = self._get_raw_data()

        if not self.get_data_from_raw(raw_systemvars):
            return None

        self.create_charts()

        return True

    def _get_data(self):
        """
        Gets systemvars data on each update.
        Gets peervars data for only one peer on each update.
        Total amount of _get_raw_data invocations per update = 2
        """
        data = dict()

        self.request = self.request_systemvars
        raw_systemvars = self._get_raw_data()
        data.update(self.get_data_from_raw(raw_systemvars))

        if self.peers:
            peer = next(self.peers)
            self.request = peer.request
            raw_peervars = self._get_raw_data()
            data.update(self.get_data_from_raw(raw_peervars, peer))

        if not data:
            self.error("No data received")
            return None

        return data

    def get_data_from_raw(self, raw, peer=None):
        """
        Extracts key=value pairs with float/integer from ntp response packet data.
        """
        data = dict()
        try:
            data_list = self.regex_data.findall(raw)

            for data_point in data_list:
                key, value = data_point
                if peer:
                    dimension = '_'.join([peer.name, key])
                else:
                    dimension = key
                data[dimension] = int(float(value) * PRECISION)
        except (ValueError, AttributeError, TypeError):
            self.error("Invalid data received")
            return None

        # If peer returns no valid data, probably due to ntpd restart,
        # then wait 10 seconds and re-initialize the peers and charts
        if not data and peer:
            self.error('Peer error: No data received')
            self.peer_error += 1

            if (self.peer_error * self.update_every) > 10:
                self.error('Peer error count exceeded, re-creating charts.')
                self.create_charts()
                self.peer_error = 0
                self.create()

        return data

    def get_header(self, associd=0, operation='readvar'):
        """
        Constructs the NTP Control Message header:
         0                   1                   2                   3
         0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
        +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
        |LI |  VN |Mode |R|E|M| OpCode  |       Sequence Number         |
        +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
        |            Status             |       Association ID          |
        +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
        |            Offset             |            Count              |
        +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
        """
        try:
            opcode = OPCODES[operation]
        except KeyError:
            self.error('Invalid operation: {0}'.format(operation))
            return None
        version = 2
        sequence = 1
        status = 0
        offset = 0
        count = 0
        try:
            header = struct.pack(HEADER_FORMAT, (version << 3 | MODE), opcode,
                                 sequence, status, associd, offset, count)
            return header
        except struct.error:
            self.error('error packing header: {0}'.format(struct.error))
            return None

    def get_peer_ids(self, res):
        """
        Unpack the NTP Control Message header
        Get data length from header
        Get list of association ids returned in the readstat response
        """
        try:
            count = struct.unpack(HEADER_FORMAT, res[:HEADER_LEN])[6]
        except struct.error:
            self.error('error unpacking header: {0}'.format(struct.error))
            return list()
        if not count:
            self.debug('empty data field in NTP control packet')
            return list()

        data_end = HEADER_LEN + count
        data = res[HEADER_LEN:data_end]
        data_format = ''.join(['!', 'H' * int(count / 2)])

        try:
            return list(struct.unpack(data_format, data))[::2]
        except struct.error:
            self.error('error unpacking data: {0}'.format(struct.error))
            return list()
