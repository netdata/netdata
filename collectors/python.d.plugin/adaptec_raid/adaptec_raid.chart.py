# -*- coding: utf-8 -*-
# Description: adaptec_raid netdata python.d module
# Author: Ilya Mashchenko (l2isbad)
# SPDX-License-Identifier: GPL-3.0-or-later


import re

from copy import deepcopy

from bases.FrameworkServices.ExecutableService import ExecutableService
from bases.collection import find_binary


update_every = 5

ORDER = [
    'ld_status',
    'pd_state',
    'pd_smart_warnings',
    'pd_temperature',
]

CHARTS = {
    'ld_status': {
        'options': [None, 'Status Not OK', 'bool', 'logical devices', 'adapter_raid.ld_status', 'line'],
        'lines': []
    },
    'pd_state': {
        'options': [None, 'State Not OK', 'bool', 'physical devices', 'adapter_raid.pd_state', 'line'],
        'lines': []
    },
    'pd_smart_warnings': {
        'options': [None, 'S.M.A.R.T warnings', 'count', 'physical devices',
                    'adapter_raid.smart_warnings', 'line'],
        'lines': []
    },
    'pd_temperature': {
        'options': [None, 'Temperature', 'celsius', 'physical devices', 'adapter_raid.temperature', 'line'],
        'lines': []
    },
}

BAD_LD_STATUS = (
    'Degraded',
    'Failed',
)

GOOD_PD_STATUS = (
    'Online',
)

RE_LD = re.compile(
    r'Logical device number\s+([0-9]+).*?'
    r'Status of logical device\s+: ([a-zA-Z]+)'
)


def find_lds(d):
    d = ' '.join(v.strip() for v in d)
    return [LD(*v) for v in RE_LD.findall(d)]


def find_pds(d):
    pds = list()
    pd = PD()

    for row in d:
        row = row.strip()
        if row.startswith('Device #'):
            pd = PD()
            pd.id = row.split('#')[-1]
        elif not pd.id:
            continue

        if row.startswith('State'):
            v = row.split()[-1]
            pd.state = v
        elif row.startswith('S.M.A.R.T. warnings'):
            v = row.split()[-1]
            pd.smart_warnings = v
        elif row.startswith('Temperature'):
            v = row.split(':')[-1].split()[0]
            pd.temperature = v
        elif row.startswith('NCQ status'):
            if pd.id and pd.state and pd.smart_warnings:
                pds.append(pd)

    return pds


class LD:
    def __init__(self, ld_id, status):
        self.id = ld_id
        self.status = status

    def data(self):
        return {
            'ld_{0}_status'.format(self.id): int(self.status in BAD_LD_STATUS)
        }


class PD:
    def __init__(self):
        self.id = None
        self.state = None
        self.smart_warnings = None
        self.temperature = None

    def data(self):
        data = {
            'pd_{0}_state'.format(self.id): int(self.state not in GOOD_PD_STATUS),
            'pd_{0}_smart_warnings'.format(self.id): self.smart_warnings,
        }
        if self.temperature and self.temperature.isdigit():
            data['pd_{0}_temperature'.format(self.id)] = self.temperature

        return data


class Arrconf:
    def __init__(self, arrconf):
        self.arrconf = arrconf

    def ld_info(self):
        return [self.arrconf, '-GETCONFIG', '1', 'LD']

    def pd_info(self):
        return [self.arrconf, '-GETCONFIG', '1', 'PD']


# TODO: hardcoded sudo...
class SudoArrconf:
    def __init__(self, arrconf, sudo):
        self.arrconf = Arrconf(arrconf)
        self.sudo = sudo

    def ld_info(self):
        return [self.sudo, '-n'] + self.arrconf.ld_info()

    def pd_info(self):
        return [self.sudo, '-n'] + self.arrconf.pd_info()


class Service(ExecutableService):
    def __init__(self, configuration=None, name=None):
        ExecutableService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = deepcopy(CHARTS)
        self.use_sudo = self.configuration.get('use_sudo', True)
        self.arrconf = None

    def execute(self, command, stderr=False):
        return self._get_raw_data(command=command, stderr=stderr)

    def check(self):
        sudo = find_binary('sudo')
        if self.use_sudo:
            if not sudo:
                self.error('can\'t locate "sudo" binary')
                return False
            err = self.execute([sudo, '-n', '-v'], True)
            if err:
                self.error(' '.join(err))
                return False

        arrconf = find_binary('arrconf')
        if not arrconf:
            self.error('can\'t locate "arrconf" binary')
            return False

        if self.use_sudo:
            self.arrconf = SudoArrconf(arrconf, sudo)
        else:
            self.arrconf = Arrconf(arrconf)

        lds = self.get_lds()
        if not lds:
            return False

        self.debug('discovered logical devices ids: {0}'.format([ld.id for ld in lds]))

        pds = self.get_pds()
        if not pds:
            return False

        self.debug('discovered physical devices ids: {0}'.format([pd.id for pd in pds]))

        self.update_charts(lds, pds)
        return True

    def get_data(self):
        data = dict()

        for ld in self.get_lds():
            data.update(ld.data())

        for pd in self.get_pds():
            data.update(pd.data())

        return data

    def get_lds(self):
        raw_lds = self.execute(self.arrconf.ld_info())
        if not raw_lds:
            return None

        lds = find_lds(raw_lds)
        if not lds:
            self.error('failed to parse "{0}" output'.format(' '.join(self.arrconf.ld_info())))
            self.debug('output: {0}'.format(raw_lds))
            return None
        return lds

    def get_pds(self):
        raw_pds = self.execute(self.arrconf.pd_info())
        if not raw_pds:
            return None

        pds = find_pds(raw_pds)
        if not pds:
            self.error('failed to parse "{0}" output'.format(' '.join(self.arrconf.pd_info())))
            self.debug('output: {0}'.format(raw_pds))
            return None
        return pds

    def update_charts(self, lds, pds):
        charts = self.definitions
        for ld in lds:
            dim = ['ld_{0}_status'.format(ld.id), 'ld {0}'.format(ld.id)]
            charts['ld_status']['lines'].append(dim)

        for pd in pds:
            dim = ['pd_{0}_state'.format(pd.id), 'pd {0}'.format(pd.id)]
            charts['pd_state']['lines'].append(dim)

            dim = ['pd_{0}_smart_warnings'.format(pd.id), 'pd {0}'.format(pd.id)]
            charts['pd_smart_warnings']['lines'].append(dim)

            dim = ['pd_{0}_temperature'.format(pd.id), 'pd {0}'.format(pd.id)]
            charts['pd_temperature']['lines'].append(dim)
