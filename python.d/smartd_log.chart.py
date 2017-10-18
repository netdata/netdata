# -*- coding: utf-8 -*-
# Description: smart netdata python.d module
# Author: l2isbad, vorph1

import os
from re import compile as r_compile

from bases.collection import read_last_line
from bases.FrameworkServices.SimpleService import SimpleService

# charts order (can be overridden if you want less charts, or different order)
ORDER = ['1', '4', '5', '7', '9', '12', '193', '194', '197', '198', '200']

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


class Disk:
    def __init__(self, name, path):
        self.name = name
        self.path = path
        self.status = True


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.regex = r_compile(r'(\d+);(\d+);(\d+)')
        self.log_path = self.configuration.get('log_path', '/var/log/smartd')
        self.raw_values = self.configuration.get('raw_values')
        self.attr = self.configuration.get('smart_attributes', [])
        self.exclude_disks = self.configuration.get('exclude_disks', str()).split()
        self.order = list()
        self.definitions = dict()
        self.disks = list()

        for path_to_disk in find_disks_in_log_path(self.log_path):
            disk_name = os.path.basename(path_to_disk).split('.')[-3]
            for pattern in self.exclude_disks:
                if pattern in disk_name:
                    break
            else:
                self.disks.append(Disk(name=disk_name, path=path_to_disk))

    def check(self):
        if not self.disks:
            self.error('Can\'t locate any smartd log files in {0}'.format(self.log_path))
            return False

        self.create_charts()
        return True

    def get_data(self):
        data = dict()
        for disk in self.disks:
            if not disk.status:
                continue

            try:
                last_line = read_last_line(disk.path)
            except OSError:
                disk.status = False
                continue

            result = self.regex.findall(last_line)
            if not result:
                continue
            for a, n, r in result:
                data.update({'_'.join([disk.name, a]): r if self.raw_values else n})

        return data or None

    def create_charts(self):

        def create_lines(attr_id):
            result = list()
            for disk in self.disks:
                result.append(['_'.join([disk.name, attr_id]), disk.name, 'absolute'])
            return result

        try:
            order = [attr for attr in self.attr.split() if attr in SMART_ATTR.keys()] or ORDER
        except AttributeError:
            order = ORDER

        self.order = [''.join(['attr_id', i]) for i in order]
        units = 'raw' if self.raw_values else 'normalized'

        for k, v in dict([(k, v) for k, v in SMART_ATTR.items() if k in order]).items():
            self.definitions[''.join(['attr_id', k])] =\
                {'options': [None, v, units, v.lower(), 'smartd.attr_id' + k, 'line'],
                 'lines': create_lines(k)}


def find_disks_in_log_path(log_path):
    if not os.path.isdir(log_path):
        raise StopIteration
    for f in os.listdir(log_path):
        f = os.path.join(log_path, f)
        if all([os.path.isfile(f),
                os.access(f, os.R_OK),
                f.endswith('.csv'),
                os.path.getsize(f)]):
            yield f
