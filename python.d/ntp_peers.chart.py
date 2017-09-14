# -*- coding: utf-8 -*-
# Description: ntp readvar python.d module
# Author: Sven Mäder (rda)

import os
from base import SimpleService
from subprocess import Popen, PIPE

NAME = os.path.basename(__file__).replace(".chart.py", "")

# default module values
update_every = 10
priority = 90000
retries = 1

ORDER = ['peer_stratum',
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
        'peer_xleave']

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
            peer = self.get_parsed_variables(associd)
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
            reply = Popen([self.ntpq, '-c', command], stdout=PIPE, stderr=PIPE, shell=False)
        except OSError:
            self.error('OSError')
            return None

        raw_data = reply.communicate()[0]

        if not raw_data:
            self.error('No raw_data, possibly ntp was restarted')
            return None

        return raw_data.decode()

    def _get_data(self):
        to_netdata = {}
        if len(self.associd_list) != 0:
            for associd in self.associd_list:
                variables = self.get_parsed_variables(associd)
                peer = self.peer_dict[associd]
                stratum = float(variables[10][0].split('=')[1])
                precision = float(variables[11][0].split('=')[1])
                rootdelay = float(variables[12][0].split('=')[1])
                rootdisp = float(variables[13][0].split('=')[1])
                hmode = float(variables[21][0].split('=')[1])
                pmode = float(variables[22][0].split('=')[1])
                hpoll = float(variables[23][0].split('=')[1])
                ppoll = float(variables[24][0].split('=')[1])
                headway = float(variables[25][0].split('=')[1])
                offset = float(variables[28][0].split('=')[1])
                delay = float(variables[29][0].split('=')[1])
                dispersion = float(variables[30][0].split('=')[1])
                jitter = float(variables[31][0].split('=')[1])
                xleave = float(variables[32][0].split('=')[1])
                to_netdata[''.join([peer, '_stratum'])] = stratum
                to_netdata[''.join([peer, '_precision'])] = precision
                to_netdata[''.join([peer, '_rootdelay'])] = rootdelay * 1000
                to_netdata[''.join([peer, '_rootdisp'])] = rootdisp * 1000
                to_netdata[''.join([peer, '_hmode'])] = hmode
                to_netdata[''.join([peer, '_pmode'])] = pmode
                to_netdata[''.join([peer, '_hpoll'])] = hpoll
                to_netdata[''.join([peer, '_ppoll'])] = ppoll
                to_netdata[''.join([peer, '_headway'])] = headway
                to_netdata[''.join([peer, '_offset'])] = offset * 1000
                to_netdata[''.join([peer, '_delay'])] = delay * 1000
                to_netdata[''.join([peer, '_dispersion'])] = dispersion * 1000
                to_netdata[''.join([peer, '_jitter'])] = jitter * 1000
                to_netdata[''.join([peer, '_xleave'])] = xleave * 1000

        return to_netdata

    def get_parsed_variables(self, assoc_id):
        raw = self._get_raw_data('readvar %s' % assoc_id)
        data = []
        lines = raw.split('\n')
        for line in lines:
            variables = line.replace('\n','').split(',')
            for variable in variables:
                if variable != '':
                    data.append(' '.join(variable.split()).split(' '))
        return data

    def create_charts(self):
        self.order = []

        #self.definitions = {chart: values for chart, values in CHARTS.items() if chart in self.order}
        self.definitions = {}

        self.order = ORDER[:]

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
