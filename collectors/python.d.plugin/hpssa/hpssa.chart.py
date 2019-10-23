# -*- coding: utf-8 -*-
# Description: hpssa netdata python.d module
# Author: Peter Gnodde (gnoddep)
# SPDX-License-Identifier: GPL-3.0-or-later

import os
import re

from bases.FrameworkServices.ExecutableService import ExecutableService
from bases.collection import find_binary
from copy import deepcopy

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

_adapter_regex = re.compile(r'^(?P<adapter_type>.+) in Slot (?P<slot>\d+)')
_ignored_sections_regex = re.compile(
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
_mirror_group_regex = re.compile(r'^Mirror Group \d+:$')
_array_regex = re.compile(r'^Array: (?P<id>[A-Z]+)$')
_drive_regex = re.compile(
    r'''
    ^
        Logical[ ]Drive:[ ](?P<logical_drive_id>\d+)
        | physicaldrive[ ](?P<fqn>[^:]+:\d+:\d+)
    $
    ''',
    re.X
)
_key_value_regex = re.compile(r'^(?P<key>[^:]+): ?(?P<value>.*)$')
_status_complete_regex = re.compile(r'^(?P<status>[^,]+), (?P<percentage>[0-9.]+)% complete$')
_error_match = re.compile(r'Error:')


class HPSSAException(Exception):
    pass


class HPSSA(object):
    def __init__(self, lines):
        self._lines = [line.strip() for line in lines if line.strip()]
        self._current_line = 0
        self.adapters = []
        self._parse()

    def __iter__(self):
        return self

    def __next__(self):
        if self._current_line == len(self._lines):
            raise StopIteration

        line = self._lines[self._current_line]
        self._current_line += 1

        return line

    def next(self):
        """
        This is for Python 2.7 compatibility
        """
        return self.__next__()

    def _rewind(self):
        self._current_line = max(self._current_line - 1, 0)

    def _parse(self):
        for line in self:
            match = _adapter_regex.match(line)
            if match:
                self.adapters.append(self._parse_adapter(**match.groupdict()))

    def _parse_adapter(self, slot, adapter_type):
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
            if _error_match.match(line):
                raise HPSSAException('Error: {}'.format(line))
            elif _array_regex.match(line):
                self._parse_array(adapter)
            elif line == 'Unassigned':
                self._parse_unassigned_physical_drives(adapter)
            elif _ignored_sections_regex.match(line):
                self._parse_ignored_section()
            else:
                match = _key_value_regex.match(line)
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

    def _parse_array(self, adapter):
        for line in self:
            if _adapter_regex.match(line) or _array_regex.match(line) or _ignored_sections_regex.match(line):
                self._rewind()
                break

            match = _drive_regex.match(line)
            if match:
                data = match.groupdict()
                if data['logical_drive_id']:
                    self._parse_logical_drive(adapter, int(data['logical_drive_id']))
                else:
                    self._parse_physical_drive(adapter, data['fqn'])
            elif not _key_value_regex.match(line):
                self._rewind()
                break

    def _parse_unassigned_physical_drives(self, adapter):
        for line in self:
            match = _drive_regex.match(line)
            if match:
                self._parse_physical_drive(adapter, match.group('fqn'))
            else:
                self._rewind()
                break

    def _parse_logical_drive(self, adapter, logical_drive_id):
        ld = {
            'id': logical_drive_id,
            'status': None,
            'status_complete': None,
        }

        for line in self:
            if _mirror_group_regex.match(line):
                self._parse_ignored_section()
            elif _adapter_regex.match(line) \
                    or _array_regex.match(line) \
                    or _drive_regex.match(line) \
                    or _ignored_sections_regex.match(line):
                self._rewind()
                break
            else:
                match = _key_value_regex.match(line)
                if match:
                    key, value = match.group('key', 'value')
                    if key == 'Status':
                        status_match = _status_complete_regex.match(value)
                        if status_match:
                            ld['status'] = status_match.group('status') == 'OK'
                            ld['status_complete'] = float(status_match.group('percentage')) / 100
                        else:
                            ld['status'] = value == 'OK'
                else:
                    self._rewind()
                    break

        adapter['logical_drives'].append(ld)

    def _parse_physical_drive(self, adapter, fqn):
        pd = {
            'fqn': fqn,
            'status': None,
            'temperature': None,
        }

        for line in self:
            if _adapter_regex.match(line) or \
                    _array_regex.match(line) or \
                    _drive_regex.search(line) or \
                    _ignored_sections_regex.match(line):
                self._rewind()
                break

            match = _key_value_regex.match(line)
            if match:
                key, value = match.group('key', 'value')
                if key == 'Status':
                    pd['status'] = value == 'OK'
                elif key == 'Current Temperature (C)':
                    pd['temperature'] = int(value)
            else:
                self._rewind()
                break

        adapter['physical_drives'].append(pd)

    def _parse_ignored_section(self):
        for line in self:
            if _adapter_regex.match(line) \
                    or _array_regex.match(line) \
                    or _drive_regex.match(line) \
                    or _ignored_sections_regex.match(line) \
                    or not _key_value_regex.match(line):
                self._rewind()
                break


class Service(ExecutableService):
    def __init__(self, configuration=None, name=None):
        super(Service, self).__init__(configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = deepcopy(CHARTS)
        self.ssacli_path = self.configuration.get('ssacli_path', 'ssacli')
        self.use_sudo = self.configuration.get('use_sudo', True)
        self.cmd = []

    def _get_adapters(self):
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

            err = self._get_raw_data(command=[sudo, '-n', '-v'], stderr=True)
            if err:
                self.error('Could not run sudo: {}'.format(' '.join(err)))
                return False

            self.cmd = [sudo, '-n']

        self.cmd.extend([self.ssacli_path, 'ctrl', 'all', 'show', 'config', 'detail'])
        self.info('Command: {}'.format(self.cmd))

        adapters = self._get_adapters()

        self.info('Discovered adapters: {}'.format([adapter['type'] for adapter in adapters]))
        if not adapters:
            self.error('No adapters discovered')
            return False

        return True

    def get_data(self):
        netdata = {}

        for adapter in self._get_adapters():
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
