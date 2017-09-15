# -*- coding: utf-8 -*-
# Description: ntp python.d module
# Author: Sven Mäder (rda)

from base import SimpleService
from subprocess import Popen, PIPE

# default module values
update_every = 10
priority = 90000
retries = 1

ORDER = [
    'frequency',
    'offset',
    'rootdelay',
    'rootdisp',
    'sys_jitter',
    'clk_jitter',
    'clk_wander',
    'precision',
    'stratum',
    'tc',
    'mintc',
    'peer_stratum',
    'peer_precision',
    'peer_rootdelay',
    'peer_rootdisp',
    'peer_hmode',
    'peer_pmode',
    'peer_hpoll',
    'peer_ppoll',
    'peer_headway',
    'peer_offset',
    'peer_delay',
    'peer_dispersion',
    'peer_jitter',
    'peer_xleave'
]

CHARTS = {
    'frequency': {
        'options': [None, "frequency offset relative to hardware clock", "ppm", 'system', 'ntp.frequency', 'area'],
        'lines': [
            ['frequency', 'frequency', 'absolute', 1, 1000]
        ]},
    'offset': {
        'options': [None, "combined offset of server relative to this host", "ms", 'system', 'ntp.offset', 'area'],
        'lines': [
            ['offset', 'offset', 'absolute', 1, 1000000]
        ]},
    'rootdelay': {
        'options': [None, "total roundtrip delay to the primary reference clock", "ms", 'system', 'ntp.rootdelay', 'area'],
        'lines': [
            ['rootdelay', 'rootdelay', 'absolute', 1, 1000]
        ]},
    'rootdisp': {
        'options': [None, "total dispersion to the primary reference clock", "ms", 'system', 'ntp.rootdisp', 'area'],
        'lines': [
            ['rootdisp', 'rootdisp', 'absolute', 1, 1000]
        ]},
    'sys_jitter': {
        'options': [None, "combined system jitter", "ms", 'system', 'ntp.sys_jitter', 'area'],
        'lines': [
            ['sys_jitter', 'sys_jitter', 'absolute', 1, 1000000]
        ]},
    'clk_jitter': {
        'options': [None, "clock jitter", "ms", 'system', 'ntp.clk_jitter', 'area'],
        'lines': [
            ['clk_jitter', 'clk_jitter', 'absolute', 1, 1000]
        ]},
    'clk_wander': {
        'options': [None, "clock frequency wander", "ppm", 'system', 'ntp.clk_wander', 'area'],
        'lines': [
            ['clk_wander', 'clk_wander', 'absolute', 1, 1000]
        ]},
    'precision': {
        'options': [None, "precision", "log2 s", 'system', 'ntp.precision', 'line'],
        'lines': [
            ['precision', 'precision', 'absolute', 1, 1]
        ]},
    'stratum': {
        'options': [None, "stratum (1-15)", "1", 'system', 'ntp.stratum', 'line'],
        'lines': [
            ['stratum', 'stratum', 'absolute', 1, 1]
        ]},
    'tc': {
        'options': [None, "time constant and poll exponent (3-17)", "log2 s", 'system', 'ntp.tc', 'line'],
        'lines': [
            ['tc', 'tc', 'absolute', 1, 1]
        ]},
    'mintc': {
        'options': [None, "minimum time constant (3-10)", "log2 s", 'system', 'ntp.mintc', 'line'],
        'lines': [
            ['mintc', 'mintc', 'absolute', 1, 1]
        ]}
}

class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.ntpq = self.find_binary('ntpq')
        self.associd_list = list()
        self.peer_dict = dict()

    def check(self):
        # Cant start without 'ntpq' command
        if not self.ntpq:
            self.error('Can\'t locate \'ntpq\' binary or binary is not executable by netdata')
            return False

        # If command is present and we can execute it we need to make sure..
        # 1. STDOUT is not empty
        reply = self._get_raw_data('associations')
        if not reply:
            self.error('No output from \'ntpq\' (not enough privileges?)')
            return False

        # 2. STDOUT has some peers
        try:
            associations = reply.split('\n')[4:]
        except:
            self.error('Cant parse output...')
            return False

        # Parse reachable peer association ids
        for association in associations:
            if association != '':
		fields = ' '.join(association.split()).split(' ')
		if fields[4] == 'yes':
		    self.associd_list.append(fields[1])

        # Make sure we have reachable peers
        if len(self.associd_list) == 0:
            self.error('Cant parse output...')
            return False

        for associd in self.associd_list:
            peer = self.get_variables(associd)
            if len(peer) == 0:
                self.error('Cant parse output...')
                return False
            self.peer_dict[associd] = peer[5][0].split('=')[1].replace('.','-')

        # We are about to start!
        self.create_charts()

        self.info('Plugin was started successfully')
        return True

    def _get_raw_data(self, command):
        try:
            proc = Popen([self.ntpq, '-c', command], stdout=PIPE, stderr=PIPE, shell=False)
        except OSError:
            self.error('OSError')
            return None

        outs, errs = proc.communicate()

        if not outs:
            self.error(''.join(['No data (possibly ntp was restarted), message was: ', errs]))
            return None

        return outs.decode()

    def parse_variables(self, raw):
        data = []
        lines = raw.split('\n')
        for line in lines:
            variables = line.replace('\n','').split(',')
            for variable in variables:
                if variable != '':
                    data.append(' '.join(variable.split()).split(' '))
        return data

    def get_variables(self, assoc_id):
        raw = self._get_raw_data('readlist %s' % assoc_id)

        if not raw:
            if assoc_id == 0:
                self.error(''.join(['No data for system, assoc_id: 0']))
            else:
                self.error(''.join(['No data for peer: ', self.peer_dict[assoc_id], ', assoc_id: ', assoc_id]))
            return None

        data = self.parse_variables(raw)

        return data

    def _get_data(self):
        to_netdata = {}

        sys_vars = self.get_variables(0)
        stratum = float(sys_vars[8][0].split('=')[1])
        precision = float(sys_vars[9][0].split('=')[1])
        rootdelay = float(sys_vars[10][0].split('=')[1])
        rootdisp = float(sys_vars[11][0].split('=')[1])
        tc = float(sys_vars[18][0].split('=')[1])
        mintc = float(sys_vars[19][0].split('=')[1])
        offset = float(sys_vars[20][0].split('=')[1])
        frequency = float(sys_vars[21][0].split('=')[1])
        sys_jitter = float(sys_vars[22][0].split('=')[1])
        clk_jitter = float(sys_vars[23][0].split('=')[1])
        clk_wander = float(sys_vars[24][0].split('=')[1])
        to_netdata['frequency'] = frequency * 1000
        to_netdata['offset'] = offset * 1000000
        to_netdata['rootdelay'] = rootdelay * 1000
        to_netdata['rootdisp'] = rootdisp * 1000
        to_netdata['sys_jitter'] = sys_jitter * 1000000
        to_netdata['clk_jitter'] = clk_jitter * 1000
        to_netdata['clk_wander'] = clk_wander * 1000
        to_netdata['precision'] = precision
        to_netdata['stratum'] = stratum
        to_netdata['tc'] = tc
        to_netdata['mintc'] = mintc

        if len(self.associd_list) != 0:
            for associd in self.associd_list:
                peer = self.peer_dict[associd]
                peer_vars = self.get_variables(associd)
                if (peer_vars != None):
                    peer_stratum = float(peer_vars[10][0].split('=')[1])
                    peer_precision = float(peer_vars[11][0].split('=')[1])
                    peer_rootdelay = float(peer_vars[12][0].split('=')[1])
                    peer_rootdisp = float(peer_vars[13][0].split('=')[1])
                    peer_hmode = float(peer_vars[21][0].split('=')[1])
                    peer_pmode = float(peer_vars[22][0].split('=')[1])
                    peer_hpoll = float(peer_vars[23][0].split('=')[1])
                    peer_ppoll = float(peer_vars[24][0].split('=')[1])
                    peer_headway = float(peer_vars[25][0].split('=')[1])
                    peer_offset = float(peer_vars[28][0].split('=')[1])
                    peer_delay = float(peer_vars[29][0].split('=')[1])
                    peer_dispersion = float(peer_vars[30][0].split('=')[1])
                    peer_jitter = float(peer_vars[31][0].split('=')[1])
                    peer_xleave = float(peer_vars[32][0].split('=')[1])
                    to_netdata[''.join([peer, '_stratum'])] = peer_stratum
                    to_netdata[''.join([peer, '_precision'])] = peer_precision
                    to_netdata[''.join([peer, '_rootdelay'])] = peer_rootdelay * 1000
                    to_netdata[''.join([peer, '_rootdisp'])] = peer_rootdisp * 1000
                    to_netdata[''.join([peer, '_hmode'])] = peer_hmode
                    to_netdata[''.join([peer, '_pmode'])] = peer_pmode
                    to_netdata[''.join([peer, '_hpoll'])] = peer_hpoll
                    to_netdata[''.join([peer, '_ppoll'])] = peer_ppoll
                    to_netdata[''.join([peer, '_headway'])] = peer_headway
                    to_netdata[''.join([peer, '_offset'])] = peer_offset * 1000
                    to_netdata[''.join([peer, '_delay'])] = peer_delay * 1000
                    to_netdata[''.join([peer, '_dispersion'])] = peer_dispersion * 1000
                    to_netdata[''.join([peer, '_jitter'])] = peer_jitter * 1000
                    to_netdata[''.join([peer, '_xleave'])] = peer_xleave * 1000

        return to_netdata

    def create_charts(self):
        self.order = ORDER[:]

        # Create static charts
        self.definitions = CHARTS

        lines_stratum = []
        lines_precision = []
        lines_rootdelay = []
        lines_rootdisp = []
        lines_hmode = []
        lines_pmode = []
        lines_hpoll = []
        lines_ppoll = []
        lines_headway = []
        lines_offset = []
        lines_delay = []
        lines_dispersion = []
        lines_jitter = []
        lines_xleave = []
 
        # Create dynamic peer charts
        if len(self.associd_list) != 0:
            for associd in self.associd_list:
                peer = self.peer_dict[associd]
                lines_stratum.append([''.join([peer, '_stratum']), None, 'absolute', 1, 1])
                lines_precision.append([''.join([peer, '_precision']), None, 'absolute', 1, 1])
                lines_rootdelay.append([''.join([peer, '_rootdelay']), None, 'absolute', 1, 1000])
                lines_rootdisp.append([''.join([peer, '_rootdisp']), None, 'absolute', 1, 1000])
                lines_hmode.append([''.join([peer, '_hmode']), None, 'absolute', 1, 1])
                lines_pmode.append([''.join([peer, '_pmode']), None, 'absolute', 1, 1])
                lines_hpoll.append([''.join([peer, '_hpoll']), None, 'absolute', 1, 1])
                lines_ppoll.append([''.join([peer, '_ppoll']), None, 'absolute', 1, 1])
                lines_headway.append([''.join([peer, '_headway']), None, 'absolute', 1, 1000])
                lines_offset.append([''.join([peer, '_offset']), None, 'absolute', 1, 1000])
                lines_delay.append([''.join([peer, '_delay']), None, 'absolute', 1, 1000])
                lines_dispersion.append([''.join([peer, '_dispersion']), None, 'absolute', 1, 1000])
                lines_jitter.append([''.join([peer, '_jitter']), None, 'absolute', 1, 1000])
                lines_xleave.append([''.join([peer, '_xleave']), None, 'absolute', 1, 1000])

        self.definitions.update({'peer_stratum': {
            'options': [None, 'stratum (0-15)', "1", 'peer', 'stratum', 'line'],
            'lines': lines_stratum}})
        self.definitions.update({'peer_precision': {
            'options': [None, 'precision', "log2 s", 'peer', 'precision', 'line'],
            'lines': lines_precision}})
        self.definitions.update({'peer_rootdelay': {
            'options': [None, 'total roundtrip delay to the primary reference clock', "ms", 'peer', 'rootdelay', 'line'],
            'lines': lines_rootdelay}})
        self.definitions.update({'peer_rootdisp': {
            'options': [None, 'total root dispersion to the primary reference clock', "ms", 'peer', 'rootdisp', 'line'],
            'lines': lines_rootdisp}})
        self.definitions.update({'peer_hmode': {
            'options': [None, 'host mode(1-6)', "1", 'peer', 'hmode', 'line'],
            'lines': lines_hmode}})
        self.definitions.update({'peer_pmode': {
            'options': [None, 'peer mode (1-5)', "1", 'peer', 'pmode', 'line'],
            'lines': lines_pmode}})
        self.definitions.update({'peer_hpoll': {
            'options': [None, 'host poll exponent (3-17)', "log2 s", 'peer', 'hpoll', 'line'],
            'lines': lines_hpoll}})
        self.definitions.update({'peer_ppoll': {
            'options': [None, 'peer poll exponent (3-17)', "log2 s", 'peer', 'ppoll', 'line'],
            'lines': lines_ppoll}})
        self.definitions.update({'peer_headway': {
            'options': [None, 'headway', "1", 'peer', 'headway', 'line'],
            'lines': lines_headway}})
        self.definitions.update({'peer_offset': {
            'options': [None, 'filter offset', "ms", 'peer', 'offset', 'line'],
            'lines': lines_offset}})
        self.definitions.update({'peer_delay': {
            'options': [None, 'filter delay', "ms", 'peer', 'delay', 'line'],
            'lines': lines_delay}})
        self.definitions.update({'peer_dispersion': {
            'options': [None, 'filter dispersion', "ms", 'peer', 'dispersion', 'line'],
            'lines': lines_dispersion}})
        self.definitions.update({'peer_jitter': {
            'options': [None, 'filter jitter', "ms", 'peer', 'jitter', 'line'],
            'lines': lines_jitter}})
        self.definitions.update({'peer_xleave': {
            'options': [None, 'interleave delay', "ms", 'peer', 'xleave', 'line'],
            'lines': lines_xleave}})
