# -*- coding: utf-8 -*-
# Description: hpssa netdata python.d module
# Author: Peter Gnodde (gnoddep)
# SPDX-License-Identifier: GPL-3.0-or-later

import os
import re
from copy import deepcopy

from bases.FrameworkServices.ExecutableService import ExecutableService
from bases.collection import find_binary

disabled_by_default = True
update_every = 5

ORDER = [
    'ctrl_status',
    'ctrl_temperature',
    'ld_status',
    'pd_status',
    'pd_temperature',
]

CHARTS = {
    'ctrl_status': {
        'options': [
            None,
            'Status 1 is OK, Status 0 is not OK',
            'Status',
            'Controller',
            'hpssa.ctrl_status',
            'line'
        ],
        'lines': []
    },
    'ctrl_temperature': {
        'options': [
            None,
            'Temperature',
            'Celsius',
            'Controller',
            'hpssa.ctrl_temperature',
            'line'
        ],
        'lines': []
    },
    'ld_status': {
        'options': [
            None,
            'Status 1 is OK, Status 0 is not OK',
            'Status',
            'Logical drives',
            'hpssa.ld_status',
            'line'
        ],
        'lines': []
    },
    'pd_status': {
        'options': [
            None,
            'Status 1 is OK, Status 0 is not OK',
            'Status',
            'Physical drives',
            'hpssa.pd_status',
            'line'
        ],
        'lines': []
    },
    'pd_temperature': {
        'options': [
            None,
            'Temperature',
            'Celsius',
            'Physical drives',
            'hpssa.pd_temperature',
            'line'
        ],
        'lines': []
    }
}

adapter_regex = re.compile(r'^(?P<adapter_type>.+) in Slot (?P<slot>\d+)')
ignored_sections_regex = re.compile(
    r'''
    ^
        Physical[ ]Drives
        | None[ ]attached
        | (?:Expander|Enclosure|SEP|Port[ ]Name:)[ ].+
        | .+[ ]at[ ]Port[ ]\S+,[ ]Box[ ]\d+,[ ].+
        | Mirror[ ]Group[ ]\d+:
    $
    ''',
    re.X
)
mirror_group_regex = re.compile(r'^Mirror Group \d+:$')
array_regex = re.compile(r'^Array: (?P<id>[A-Z]+)$')
drive_regex = re.compile(
    r'''
    ^
        Logical[ ]Drive:[ ](?P<logical_drive_id>\d+)
        | physicaldrive[ ](?P<fqn>[^:]+:\d+:\d+)
    $
    ''',
    re.X
)
key_value_regex = re.compile(r'^(?P<key>[^:]+): ?(?P<value>.*)$')
ld_status_regex = re.compile(r'^Status: (?P<status>[^,]+)(?:, (?P<percentage>[0-9.]+)% complete)?$')
error_match = re.compile(r'Error:')


class HPSSAException(Exception):
    pass


class HPSSA(object):
    def __init__(self, lines):
        self.lines = [line.strip() for line in lines if line.strip()]
        self.current_line = 0
        self.adapters = []
        self.parse()

    def __iter__(self):
        return self

    def __next__(self):
        if self.current_line == len(self.lines):
            raise StopIteration

        line = self.lines[self.current_line]
        self.current_line += 1

        return line

    def next(self):
        """
        This is for Python 2.7 compatibility
        """
        return self.__next__()

    def rewind(self):
        self.current_line = max(self.current_line - 1, 0)

    @staticmethod
    def match_any(line, *regexes):
        return any([regex.match(line) for regex in regexes])

    def parse(self):
        for line in self:
            match = adapter_regex.match(line)
            if match:
                self.adapters.append(self.parse_adapter(**match.groupdict()))

    def parse_adapter(self, slot, adapter_type):
        adapter = {
            'slot': int(slot),
            'type': adapter_type,

            'controller': {
                'status': None,
                'temperature': None,
            },
            'cache': {
                'present': False,
                'status': None,
                'temperature': None,
            },
            'battery': {
                'status': None,
                'count': 0,
            },

            'logical_drives': [],
            'physical_drives': [],
        }

        for line in self:
            if error_match.match(line):
                raise HPSSAException('Error: {}'.format(line))
            elif adapter_regex.match(line):
                self.rewind()
                break
            elif array_regex.match(line):
                self.parse_array(adapter)
            elif line == 'Unassigned':
                self.parse_unassigned_physical_drives(adapter)
            elif ignored_sections_regex.match(line):
                self.parse_ignored_section()
            else:
                match = key_value_regex.match(line)
                if match:
                    key, value = match.group('key', 'value')
                    if key == 'Controller Status':
                        adapter['controller']['status'] = value == 'OK'
                    elif key == 'Controller Temperature (C)':
                        adapter['controller']['temperature'] = int(value)
                    elif key == 'Cache Board Present':
                        adapter['cache']['present'] = value == 'True'
                    elif key == 'Cache Status':
                        adapter['cache']['status'] = value == 'OK'
                    elif key == 'Cache Module Temperature (C)':
                        adapter['cache']['temperature'] = int(value)
                    elif key == 'Battery/Capacitor Count':
                        adapter['battery']['count'] = int(value)
                    elif key == 'Battery/Capacitor Status':
                        adapter['battery']['status'] = value == 'OK'
                else:
                    raise HPSSAException('Cannot parse line: {}'.format(line))

        return adapter

    def parse_array(self, adapter):
        for line in self:
            if HPSSA.match_any(line, adapter_regex, array_regex, ignored_sections_regex):
                self.rewind()
                break

            match = drive_regex.match(line)
            if match:
                data = match.groupdict()
                if data['logical_drive_id']:
                    self.parse_logical_drive(adapter, int(data['logical_drive_id']))
                else:
                    self.parse_physical_drive(adapter, data['fqn'])
            elif not key_value_regex.match(line):
                self.rewind()
                break

    def parse_unassigned_physical_drives(self, adapter):
        for line in self:
            match = drive_regex.match(line)
            if match:
                self.parse_physical_drive(adapter, match.group('fqn'))
            else:
                self.rewind()
                break

    def parse_logical_drive(self, adapter, logical_drive_id):
        ld = {
            'id': logical_drive_id,
            'status': None,
            'status_complete': None,
        }

        for line in self:
            if mirror_group_regex.match(line):
                self.parse_ignored_section()
                continue

            match = ld_status_regex.match(line)
            if match:
                ld['status'] = match.group('status') == 'OK'

                if match.group('percentage'):
                    ld['status_complete'] = float(match.group('percentage')) / 100
            elif HPSSA.match_any(line, adapter_regex, array_regex, drive_regex, ignored_sections_regex) \
                    or not key_value_regex.match(line):
                self.rewind()
                break

        adapter['logical_drives'].append(ld)

    def parse_physical_drive(self, adapter, fqn):
        pd = {
            'fqn': fqn,
            'status': None,
            'temperature': None,
        }

        for line in self:
            if HPSSA.match_any(line, adapter_regex, array_regex, drive_regex, ignored_sections_regex):
                self.rewind()
                break

            match = key_value_regex.match(line)
            if match:
                key, value = match.group('key', 'value')
                if key == 'Status':
                    pd['status'] = value == 'OK'
                elif key == 'Current Temperature (C)':
                    pd['temperature'] = int(value)
            else:
                self.rewind()
                break

        adapter['physical_drives'].append(pd)

    def parse_ignored_section(self):
        for line in self:
            if HPSSA.match_any(line, adapter_regex, array_regex, drive_regex, ignored_sections_regex) \
                    or not key_value_regex.match(line):
                self.rewind()
                break


class Service(ExecutableService):
    def __init__(self, configuration=None, name=None):
        super(Service, self).__init__(configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = deepcopy(CHARTS)
        self.ssacli_path = self.configuration.get('ssacli_path', 'ssacli')
        self.use_sudo = self.configuration.get('use_sudo', True)
        self.cmd = []

    def get_adapters(self):
        try:
            adapters = HPSSA(self._get_raw_data(command=self.cmd)).adapters
            if not adapters:
                # If no adapters are returned, run the command again but capture stderr
                err = self._get_raw_data(command=self.cmd, stderr=True)
                if err:
                    raise HPSSAException('Error executing cmd {}: {}'.format(' '.join(self.cmd), '\n'.join(err)))
            return adapters
        except HPSSAException as ex:
            self.error(ex)
            return []

    def check(self):
        if not os.path.isfile(self.ssacli_path):
            ssacli_path = find_binary(self.ssacli_path)
            if ssacli_path:
                self.ssacli_path = ssacli_path
            else:
                self.error('Cannot locate "{}" binary'.format(self.ssacli_path))
                return False

        if self.use_sudo:
            sudo = find_binary('sudo')
            if not sudo:
                self.error('Cannot locate "{}" binary'.format('sudo'))
                return False

            allowed = self._get_raw_data(command=[sudo, '-n', '-l', self.ssacli_path])
            if not allowed or allowed[0].strip() != os.path.realpath(self.ssacli_path):
                self.error('Not allowed to run sudo for command {}'.format(self.ssacli_path))
                return False

            self.cmd = [sudo, '-n']

        self.cmd.extend([self.ssacli_path, 'ctrl', 'all', 'show', 'config', 'detail'])
        self.info('Command: {}'.format(self.cmd))

        adapters = self.get_adapters()

        self.info('Discovered adapters: {}'.format([adapter['type'] for adapter in adapters]))
        if not adapters:
            self.error('No adapters discovered')
            return False

        return True

    def get_data(self):
        netdata = {}

        for adapter in self.get_adapters():
            status_key = '{}_status'.format(adapter['slot'])
            temperature_key = '{}_temperature'.format(adapter['slot'])
            ld_key = 'ld_{}_'.format(adapter['slot'])

            data = {
                'ctrl_status': {
                    'ctrl_' + status_key: adapter['controller']['status'],
                    'cache_' + status_key: adapter['cache']['present'] and adapter['cache']['status'],
                    'battery_' + status_key:
                        adapter['battery']['status'] if adapter['battery']['count'] > 0 else None
                },

                'ctrl_temperature': {
                    'ctrl_' + temperature_key: adapter['controller']['temperature'],
                    'cache_' + temperature_key: adapter['cache']['temperature'],
                },

                'ld_status': {
                    ld_key + '{}_status'.format(ld['id']): ld['status'] for ld in adapter['logical_drives']
                },

                'pd_status': {},
                'pd_temperature': {},
            }

            for pd in adapter['physical_drives']:
                pd_key = 'pd_{}_{}'.format(adapter['slot'], pd['fqn'])
                data['pd_status'][pd_key + '_status'] = pd['status']
                data['pd_temperature'][pd_key + '_temperature'] = pd['temperature']

            for chart, dimension_data in data.items():
                for dimension_id, value in dimension_data.items():
                    if value is None:
                        continue

                    if dimension_id not in self.charts[chart]:
                        self.charts[chart].add_dimension([dimension_id])

                    netdata[dimension_id] = value

        return netdata
