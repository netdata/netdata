# -*- coding: utf-8 -*-
# Description: smart netdata python.d module
# Author: l2isbad, vorph1

import os
import re

from collections import namedtuple
from time import time

from bases.collection import read_last_line
from bases.FrameworkServices.SimpleService import SimpleService

# charts order (can be overridden if you want less charts, or different order)
# ORDER = ['1', '4', '5', '7', '9', '12', '193', '194', '197', '198', '200']
ORDER = ['194']

SMART_ATTR = {
    '1': 'Read Error Rate',
    '2': 'Throughput Performance',
    '3': 'Spin-Up Time',
    '4': 'Start/Stop Count',
    '5': 'Reallocated Sectors Count',
    '6': 'Read Channel Margin',
    '7': 'Seek Error Rate',
    '8': 'Seek Time Performance',
    '9': 'Power-On Hours Count',
    '10': 'Spin-up Retries',
    '11': 'Calibration Retries',
    '12': 'Power Cycle Count',
    '13': 'Soft Read Error Rate',
    '100': 'Erase/Program Cycles',
    '103': 'Translation Table Rebuild',
    '108': 'Unknown (108)',
    '170': 'Reserved Block Count',
    '171': 'Program Fail Count',
    '172': 'Erase Fail Count',
    '173': 'Wear Leveller Worst Case Erase Count',
    '174': 'Unexpected Power Loss',
    '175': 'Program Fail Count',
    '176': 'Erase Fail Count',
    '177': 'Wear Leveling Count',
    '178': 'Used Reserved Block Count',
    '179': 'Used Reserved Block Count',
    '180': 'Unused Reserved Block Count',
    '181': 'Program Fail Count',
    '182': 'Erase Fail Count',
    '183': 'SATA Downshifts',
    '184': 'End-to-End error',
    '185': 'Head Stability',
    '186': 'Induced Op-Vibration Detection',
    '187': 'Reported Uncorrectable Errors',
    '188': 'Command Timeout',
    '189': 'High Fly Writes',
    '190': 'Temperature',
    '191': 'G-Sense Errors',
    '192': 'Power-Off Retract Cycles',
    '193': 'Load/Unload Cycles',
    '194': 'Temperature',
    '195': 'Hardware ECC Recovered',
    '196': 'Reallocation Events',
    '197': 'Current Pending Sectors',
    '198': 'Off-line Uncorrectable',
    '199': 'UDMA CRC Error Rate',
    '200': 'Write Error Rate',
    '201': 'Soft Read Errors',
    '202': 'Data Address Mark Errors',
    '203': 'Run Out Cancel',
    '204': 'Soft ECC Corrections',
    '205': 'Thermal Asperity Rate',
    '206': 'Flying Height',
    '207': 'Spin High Current',
    '209': 'Offline Seek Performance',
    '220': 'Disk Shift',
    '221': 'G-Sense Error Rate',
    '222': 'Loaded Hours',
    '223': 'Load/Unload Retries',
    '224': 'Load Friction',
    '225': 'Load/Unload Cycles',
    '226': 'Load-in Time',
    '227': 'Torque Amplification Count',
    '228': 'Power-Off Retracts',
    '230': 'GMR Head Amplitude',
    '231': 'Temperature',
    '232': 'Available Reserved Space',
    '233': 'Media Wearout Indicator',
    '240': 'Head Flying Hours',
    '241': 'Total LBAs Written',
    '242': 'Total LBAs Read',
    '250': 'Read Error Retry Rate'
}

LIMIT = namedtuple('LIMIT', ['min', 'max'])

LIMITS = {
    '194': LIMIT(0, 200)
}

RESCAN_INTERVAL = 60

REGEX = re.compile(
    '(\d+);'  # attribute
    '(\d+);'  # normalized value
    '(\d+)',  # raw value
    re.X
)


def chart_template(attr, raw):
    chart_name = 'attr_id' + attr
    title = SMART_ATTR[attr]
    units = 'raw' if raw else 'normalized'

    return {
        chart_name: {
            'options': [None, title, units, title.lower(), 'smartd_log.' + chart_name, 'line'],
            'lines': []
            }
    }


def handle_os_error(method):
    def on_call(*args):
        try:
            return method(*args)
        except OSError:
            return None
    return on_call


class SmartAttribute(object):
    def __init__(self, idx, normalized, raw):
        self.id = idx
        self.normalized = normalized
        self._raw = raw

    @property
    def raw(self):
        if self.id in LIMITS:
            limit = LIMITS[self.id]
            if limit.min <= int(self._raw) <= limit.max:
                return self._raw
            return None
        return self._raw

    @raw.setter
    def raw(self, value):
        self._raw = value


class DiskLogFile:
    def __init__(self, path):
        self.path = path
        self.size = os.path.getsize(path)

    @handle_os_error
    def is_changed(self):
        new_size = os.path.getsize(self.path)
        old_size, self.size = self.size, new_size

        return new_size != old_size and new_size

    @staticmethod
    @handle_os_error
    def is_valid(log_file, exclude):
        return all([log_file.endswith('.csv'),
                    not [p for p in exclude if p in log_file],
                    os.access(log_file, os.R_OK),
                    os.path.getsize(log_file)])


class Disk:
    def __init__(self, full_path, age):
        self.log_file = DiskLogFile(full_path)
        self.name = os.path.basename(full_path).split('.')[-3]
        self.age = int(age)
        self.status = True
        self.attributes = dict()

        self.get_attributes()

    def __eq__(self, other):
        if isinstance(other, Disk):
            return self.name == other.name
        return self.name == other

    @handle_os_error
    def is_active(self):
        return (time() - os.path.getmtime(self.log_file.path)) / 60 < self.age

    @handle_os_error
    def get_attributes(self):
        last_line = read_last_line(self.log_file.path)
        self.attributes = dict((attr, SmartAttribute(attr, normalized, raw)) for attr, normalized, raw
                               in REGEX.findall(last_line))
        return True

    def data(self, raw=None):
        data = dict()
        for attr in self.attributes.values():
            value = attr.raw if raw else attr.normalized
            if value is None:
                continue
            key = '_'.join([self.name, attr.id])
            data[key] = value
        return data


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.log_path = self.configuration.get('log_path', '/var/log/smartd')
        self.raw = self.configuration.get('raw_values', True)
        self.exclude = self.configuration.get('exclude_disks', str()).split()
        self.age = self.configuration.get('age', 60)

        self.runs = 0
        self.disks = list()
        self.order = list()
        self.definitions = dict()

    def check(self):
        self.disks = self.scan()

        if not self.disks:
            return None

        user_defined_sa = self.configuration.get('smart_attributes')

        if user_defined_sa:
            order = user_defined_sa.split() or ORDER
        else:
            order = ORDER

        self.create_charts(order)

        return True

    def get_data(self):
        self.runs += 1

        if self.runs % RESCAN_INTERVAL == 0:
            self.cleanup_and_rescan()

        data = dict()

        for disk in self.disks:

            if not disk.status:
                continue

            changed = disk.log_file.is_changed()

            # True = changed, False = unchanged, None = Exception
            if changed is None:
                disk.status = False
                continue

            if changed:
                success = disk.get_attributes()
                if not success:
                    disk.status = False
                    continue

            data.update(disk.data(self.raw))

        return data or None

    def create_charts(self, order):
        for attr in order:
            chart_id = 'attr_id' + attr
            chart = chart_template(attr, self.raw)

            for disk in self.disks:
                if attr not in disk.attributes:
                    self.debug("'{disk}' has no attribute '{attr_id}'".format(disk=disk.name,
                                                                              attr_id=attr))
                    continue

                if self.raw and disk.attributes[attr].raw is None:
                    self.debug("'{disk}' attribute '{attr_id}' value not in {limits}".format(disk=disk.name,
                                                                                             attr_id=attr,
                                                                                             limits=LIMITS[attr]))
                    continue
                chart[chart_id]['lines'].append(['_'.join([disk.name, attr]), disk.name])

            self.order.append(chart_id)
            self.definitions.update(chart)

    def scan(self, only_new=None):
        new_disks = list()
        for f in os.listdir(self.log_path):
            full_path = os.path.join(self.log_path, f)

            if DiskLogFile.is_valid(full_path, self.exclude):
                disk = Disk(full_path, self.age)

                active = disk.is_active()
                if active is None:
                    continue

                if active:
                    if not only_new:
                        new_disks.append(disk)
                    else:
                        if disk not in self.disks:
                            new_disks.append(disk)
                else:
                    if not only_new:
                        self.debug("'{disk}' not updated in the last {age} minutes, "
                                   "skipping it.".format(disk=disk.name, age=self.age))
        return new_disks

    def cleanup_and_rescan(self):
        self.cleanup()
        new_disks = self.scan(only_new=True)

        for disk in new_disks:
            valid = False

            for chart in self.charts:
                idx = chart.id[7:]

                if idx in disk.attributes:
                    valid = True
                    dimension_id = '_'.join([disk.name, idx])

                    if dimension_id in chart:
                        chart.hide_dimension(dimension_id=dimension_id, reverse=True)
                    else:
                        chart.add_dimension([dimension_id, disk.name])
            if valid:
                self.disks.append(disk)

    def cleanup(self):
        for disk in self.disks:

            if not disk.is_active():
                disk.status = False

            if not disk.status:
                for chart in self.charts:
                    dimension_id = '_'.join([disk.name, chart.id[7:]])
                    chart.hide_dimension(dimension_id=dimension_id)

        self.disks = [disk for disk in self.disks if disk.status]
