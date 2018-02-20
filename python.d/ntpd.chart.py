# -*- coding: utf-8 -*-
# Description: ntpd netdata python.d module
# Author: Sven Mäder (rda0)
# Author: Ilya Mashchenko (l2isbad)

import struct
import re

from bases.FrameworkServices.SocketService import SocketService

# default module values
update_every = 1
priority = 60000
retries = 60
peer_rescan = 60

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
    'sys_precision',
    'peer_offset',
    'peer_delay',
    'peer_dispersion',
    'peer_jitter',
    'peer_xleave',
    'peer_rootdelay',
    'peer_rootdisp',
    'peer_stratum',
    'peer_hmode',
    'peer_pmode',
    'peer_hpoll',
    'peer_ppoll',
    'peer_precision'
]

CHARTS = {
    'sys_offset': {
        'options': [None, 'Combined offset of server relative to this host', 'ms', 'system', 'ntpd.sys_offset', 'area'],
        'lines': [
            ['offset', 'offset', 'absolute', 1, PRECISION]
        ]},
    'sys_jitter': {
        'options': [None, 'Combined system jitter and clock jitter', 'ms', 'system', 'ntpd.sys_jitter', 'line'],
        'lines': [
            ['sys_jitter', 'system', 'absolute', 1, PRECISION],
            ['clk_jitter', 'clock', 'absolute', 1, PRECISION]
        ]},
    'sys_frequency': {
        'options': [None, 'Frequency offset relative to hardware clock', 'ppm', 'system', 'ntpd.sys_frequency', 'area'],
        'lines': [
            ['frequency', 'frequency', 'absolute', 1, PRECISION]
        ]},
    'sys_wander': {
        'options': [None, 'Clock frequency wander', 'ppm', 'system', 'ntpd.sys_wander', 'area'],
        'lines': [
            ['clk_wander', 'clock', 'absolute', 1, PRECISION]
        ]},
    'sys_rootdelay': {
        'options': [None, 'Total roundtrip delay to the primary reference clock', 'ms', 'system',
                    'ntpd.sys_rootdelay', 'area'],
        'lines': [
            ['rootdelay', 'delay', 'absolute', 1, PRECISION]
        ]},
    'sys_rootdisp': {
        'options': [None, 'Total root dispersion to the primary reference clock', 'ms', 'system',
                    'ntpd.sys_rootdisp', 'area'],
        'lines': [
            ['rootdisp', 'dispersion', 'absolute', 1, PRECISION]
        ]},
    'sys_stratum': {
        'options': [None, 'Stratum (1-15)', '1', 'system', 'ntpd.sys_stratum', 'line'],
        'lines': [
            ['stratum', 'stratum', 'absolute', 1, PRECISION]
        ]},
    'sys_tc': {
        'options': [None, 'Time constant and poll exponent (3-17)', 'log2 s', 'system', 'ntpd.sys_tc', 'line'],
        'lines': [
            ['tc', 'current', 'absolute', 1, PRECISION],
            ['mintc', 'minimum', 'absolute', 1, PRECISION]
        ]},
    'sys_precision': {
        'options': [None, 'Precision', 'log2 s', 'system', 'ntpd.sys_precision', 'line'],
        'lines': [
            ['precision', 'precision', 'absolute', 1, PRECISION]
        ]}
}

PEER_CHARTS = {
    'peer_offset': {
        'options': [None, 'Filter offset', 'ms', 'peers', 'ntpd.peer_offset', 'line'],
        'lines': [
        ]},
    'peer_delay': {
        'options': [None, 'Filter delay', 'ms', 'peers', 'ntpd.peer_delay', 'line'],
        'lines': [
        ]},
    'peer_dispersion': {
        'options': [None, 'Filter dispersion', 'ms', 'peers', 'ntpd.peer_dispersion', 'line'],
        'lines': [
        ]},
    'peer_jitter': {
        'options': [None, 'Filter jitter', 'ms', 'peers', 'ntpd.peer_jitter', 'line'],
        'lines': [
        ]},
    'peer_xleave': {
        'options': [None, 'Interleave delay', 'ms', 'peers', 'ntpd.peer_xleave', 'line'],
        'lines': [
        ]},
    'peer_rootdelay': {
        'options': [None, 'Total roundtrip delay to the primary reference clock', 'ms', 'peers',
                    'ntpd.peer_rootdelay', 'line'],
        'lines': [
        ]},
    'peer_rootdisp': {
        'options': [None, 'Total root dispersion to the primary reference clock', 'ms', 'peers',
                    'ntpd.peer_rootdisp', 'line'],
        'lines': [
        ]},
    'peer_stratum': {
        'options': [None, 'Stratum (1-15)', '1', 'peers', 'ntpd.peer_stratum', 'line'],
        'lines': [
        ]},
    'peer_hmode': {
        'options': [None, 'Host mode (1-6)', '1', 'peers', 'ntpd.peer_hmode', 'line'],
        'lines': [
        ]},
    'peer_pmode': {
        'options': [None, 'Peer mode (1-5)', '1', 'peers', 'ntpd.peer_pmode', 'line'],
        'lines': [
        ]},
    'peer_hpoll': {
        'options': [None, 'Host poll exponent', 'log2 s', 'peers', 'ntpd.peer_hpoll', 'line'],
        'lines': [
        ]},
    'peer_ppoll': {
        'options': [None, 'Peer poll exponent', 'log2 s', 'peers', 'ntpd.peer_ppoll', 'line'],
        'lines': [
        ]},
    'peer_precision': {
        'options': [None, 'Precision', 'log2 s', 'peers', 'ntpd.peer_precision', 'line'],
        'lines': [
        ]}
}


class Base:
    regex = re.compile(r'([a-z_]+)=((?:-)?[0-9]+(?:\.[0-9]+)?)')

    @staticmethod
    def get_header(associd=0, operation='readvar'):
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
        version = 2
        sequence = 1
        status = 0
        offset = 0
        count = 0
        header = struct.pack(HEADER_FORMAT, (version << 3 | MODE), OPCODES[operation],
                             sequence, status, associd, offset, count)
        return header


class System(Base):
    def __init__(self):
        self.request = self.get_header()

    def get_data(self, raw):
        """
        Extracts key=value pairs with float/integer from ntp response packet data.
        """
        data = dict()
        for key, value in self.regex.findall(raw):
            data[key] = float(value) * PRECISION
        return data


class Peer(Base):
    def __init__(self, idx, name):
        self.id = idx
        self.real_name = name
        self.name = name.replace('.', '_')
        self.request = self.get_header(self.id)

    def get_data(self, raw):
        """
        Extracts key=value pairs with float/integer from ntp response packet data.
        """
        data = dict()
        for key, value in self.regex.findall(raw):
            dimension = '_'.join([self.name, key])
            data[dimension] = float(value) * PRECISION
        return data


class Service(SocketService):
    def __init__(self, configuration=None, name=None):
        SocketService.__init__(self, configuration=configuration, name=name)
        self.order = list(ORDER)
        self.definitions = dict(CHARTS)

        self.port = 'ntp'
        self.dgram_socket = True
        self.system = System()
        self.peers = dict()
        self.request = str()
        self.retries = 0
        self.show_peers = self.configuration.get('show_peers', False)

    def check(self):
        """
        Checks if we can get valid systemvars.
        If not, returns None to disable module.
        """
        self._parse_config()

        if self.show_peers:
            self.definitions.update(PEER_CHARTS)

            peer_filter = self.configuration.get('peer_filter', r'127\..*')
            try:
                self.peer_filter = re.compile(r'^((0\.0\.0\.0)|({0}))$'.format(peer_filter))
            except re.error as error:
                self.error('Compile pattern error (peer_filter) : {0}'.format(error))
                return None

            try:
                self.peer_rescan = int(self.configuration.get('peer_rescan', peer_rescan))
                if self.peer_rescan <= 0:
                    raise ValueError('int > 0 expected: {0}'.format(self.peer_rescan))
            except ValueError as error:
                self.error('Value error (peer_rescan) : {0}'.format(error))
                return None

        self.request = self.system.request
        raw_systemvars = self._get_raw_data()

        if not self.system.get_data(raw_systemvars):
            return None

        return True

    def get_data(self):
        """
        Gets systemvars data on each update.
        Gets peervars data for all peers on each update.
        """
        data = dict()

        self.request = self.system.request
        raw = self._get_raw_data()
        if not raw:
            return None

        data.update(self.system.get_data(raw))

        if not self.show_peers:
            return data

        if self.runs_counter == 1 or self.runs_counter % self.peer_rescan or self.retries > 8:
            self.find_new_peers()

        for peer in self.peers.values():
            self.request = peer.request
            peer_data = peer.get_data(self._get_raw_data())
            if peer_data:
                data.update(peer_data)
            else:
                self.retries += 1

        return data

    def find_new_peers(self):
        new_peers = dict((p.real_name, p) for p in self.get_peers())
        if new_peers:

            peers_to_remove = set(self.peers) - set(new_peers)
            peers_to_add = set(new_peers) - set(self.peers)

            for peer_name in peers_to_remove:
                self.hide_old_peer_from_charts(self.peers[peer_name])
                del self.peers[peer_name]

            for peer_name in peers_to_add:
                self.add_new_peer_to_charts(new_peers[peer_name])

            self.peers.update(new_peers)
            self.retries = 0

    def add_new_peer_to_charts(self, peer):
        for chart_id in set(self.charts.charts) & set(PEER_CHARTS):
            dim_id = peer.name + chart_id[4:]
            if dim_id not in self.charts[chart_id]:
                self.charts[chart_id].add_dimension([dim_id, peer.real_name, 'absolute', 1, PRECISION])
            else:
                self.charts[chart_id].hide_dimension(dim_id, reverse=True)

    def hide_old_peer_from_charts(self, peer):
        for chart_id in set(self.charts.charts) & set(PEER_CHARTS):
            dim_id = peer.name + chart_id[4:]
            self.charts[chart_id].hide_dimension(dim_id)

    def get_peers(self):
        self.request = Base.get_header(operation='readstat')

        raw_data = self._get_raw_data(raw=True)
        if not raw_data:
            return list()

        peer_ids = self.get_peer_ids(raw_data)
        if not peer_ids:
            return list()

        new_peers = list()
        for peer_id in peer_ids:
            self.request = Base.get_header(peer_id)
            raw_peer_data = self._get_raw_data()
            if not raw_peer_data:
                continue
            srcadr = re.search(r'(srcadr)=([^,]+)', raw_peer_data)
            if not srcadr:
                continue
            srcadr = srcadr.group(2)
            if self.peer_filter.search(srcadr):
                continue

            new_peer = Peer(idx=peer_id, name=srcadr)
            new_peers.append(new_peer)
        return new_peers

    def get_peer_ids(self, res):
        """
        Unpack the NTP Control Message header
        Get data length from header
        Get list of association ids returned in the readstat response
        """

        try:
            count = struct.unpack(HEADER_FORMAT, res[:HEADER_LEN])[6]
        except struct.error as error:
            self.error('error unpacking header: {0}'.format(error))
            return None
        if not count:
            self.error('empty data field in NTP control packet')
            return None

        data_end = HEADER_LEN + count
        data = res[HEADER_LEN:data_end]
        data_format = ''.join(['!', 'H' * int(count / 2)])
        try:
            peer_ids = list(struct.unpack(data_format, data))[::2]
        except struct.error as error:
            self.error('error unpacking data: {0}'.format(error))
            return None
        return peer_ids
