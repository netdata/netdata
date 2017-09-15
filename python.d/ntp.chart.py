# -*- coding: utf-8 -*-
# Description: ntp python.d module
# Author: Sven Mäder (rda)

from base import SimpleService
from subprocess import Popen, PIPE
import re

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
        self.rgx_sys = re.compile(
                r'.*stratum=(?P<stratum>[0-9.-]+)'
                r'.*precision=(?P<precision>[0-9.-]+)'
                r'.*rootdelay=(?P<rootdelay>[0-9.-]+)'
                r'.*rootdisp=(?P<rootdisp>[0-9.-]+)'
                r'.*tc=(?P<tc>[0-9.-]+)'
                r'.*mintc=(?P<mintc>[0-9.-]+)'
                r'.*offset=(?P<offset>[0-9.-]+)'
                r'.*frequency=(?P<frequency>[0-9.-]+)'
                r'.*sys_jitter=(?P<sys_jitter>[0-9.-]+)'
                r'.*clk_jitter=(?P<clk_jitter>[0-9.-]+)'
                r'.*clk_wander=(?P<clk_wander>[0-9.-]+)')
        self.rgx_peer = re.compile(
                r'.*associd=(?P<associd>[0-9.-]+)'
                r'.*srcadr=(?P<srcadr>[a-z0-9.-]+)'
                r'.*stratum=(?P<stratum>[0-9.-]+)'
                r'.*precision=(?P<precision>[0-9.-]+)'
                r'.*rootdelay=(?P<rootdelay>[0-9.-]+)'
                r'.*rootdisp=(?P<rootdisp>[0-9.-]+)'
                r'.*hmode=(?P<hmode>[0-9.-]+)'
                r'.*pmode=(?P<pmode>[0-9.-]+)'
                r'.*hpoll=(?P<hpoll>[0-9.-]+)'
                r'.*ppoll=(?P<ppoll>[0-9.-]+)'
                r'.*headway=(?P<headway>[0-9.-]+)'
                r'.*offset=(?P<offset>[0-9.-]+)'
                r'.*delay=(?P<delay>[0-9.-]+)'
                r'.*dispersion=(?P<dispersion>[0-9.-]+)'
                r'.*jitter=(?P<jitter>[0-9.-]+)'
                r'.*xleave=(?P<xleave>[0-9.-]+)')

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
            self.peer_dict[associd] = '-'.join(peer['srcadr'].split('.'))

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

    def get_variables(self, assoc_id):
        raw = self._get_raw_data('readlist %s' % assoc_id)

        if not raw:
            if assoc_id == 0:
                self.error(''.join(['No data for system, assoc_id: 0']))
            else:
                self.error(''.join(['No data for peer: ', self.peer_dict[assoc_id], ', assoc_id: ', assoc_id]))
            return None

        if assoc_id == 0:
            match = self.rgx_sys.search(' '.join(raw.split('\n')))
        else:
            match = self.rgx_peer.search(' '.join(raw.split('\n')))

        if match != None:
            data = match.groupdict()
        else:
            return None

        return data

    def _get_data(self):
        to_netdata = {}

        sys_vars = self.get_variables(0)
        if (sys_vars != None):
            to_netdata['frequency'] = float(sys_vars['frequency']) * 1000
            to_netdata['offset'] = float(sys_vars['offset']) * 1000000
            to_netdata['rootdelay'] = float(sys_vars['rootdelay']) * 1000
            to_netdata['rootdisp'] = float(sys_vars['rootdisp']) * 1000
            to_netdata['sys_jitter'] = float(sys_vars['sys_jitter']) * 1000000
            to_netdata['clk_jitter'] = float(sys_vars['clk_jitter']) * 1000
            to_netdata['clk_wander'] = float(sys_vars['clk_wander']) * 1000
            to_netdata['precision'] = float(sys_vars['precision'])
            to_netdata['stratum'] = float(sys_vars['stratum'])
            to_netdata['tc'] = float(sys_vars['tc'])
            to_netdata['mintc'] = float(sys_vars['mintc'])

        if len(self.associd_list) != 0:
            for associd in self.associd_list:
                peer = self.peer_dict[associd]
                peer_vars = self.get_variables(associd)
                if (peer_vars != None):
                    to_netdata[''.join([peer, '_stratum'])] = float(peer_vars['stratum'])
                    to_netdata[''.join([peer, '_precision'])] = float(peer_vars['precision'])
                    to_netdata[''.join([peer, '_rootdelay'])] = float(peer_vars['rootdelay']) * 1000
                    to_netdata[''.join([peer, '_rootdisp'])] = float(peer_vars['rootdisp']) * 1000
                    to_netdata[''.join([peer, '_hmode'])] = float(peer_vars['hmode'])
                    to_netdata[''.join([peer, '_pmode'])] = float(peer_vars['pmode'])
                    to_netdata[''.join([peer, '_hpoll'])] = float(peer_vars['hpoll'])
                    to_netdata[''.join([peer, '_ppoll'])] = float(peer_vars['ppoll'])
                    to_netdata[''.join([peer, '_headway'])] = float(peer_vars['headway'])
                    to_netdata[''.join([peer, '_offset'])] = float(peer_vars['offset']) * 1000
                    to_netdata[''.join([peer, '_delay'])] = float(peer_vars['delay']) * 1000
                    to_netdata[''.join([peer, '_dispersion'])] = float(peer_vars['dispersion']) * 1000
                    to_netdata[''.join([peer, '_jitter'])] = float(peer_vars['jitter']) * 1000
                    to_netdata[''.join([peer, '_xleave'])] = float(peer_vars['xleave']) * 1000

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
