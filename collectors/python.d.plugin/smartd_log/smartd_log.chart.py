# -*- coding: utf-8 -*-
# Description: smart netdata python.d module
# Author: ilyam8, vorph1
# SPDX-License-Identifier: GPL-3.0-or-later

import os
import re

from copy import deepcopy
from time import time

from bases.collection import read_last_line
from bases.FrameworkServices.SimpleService import SimpleService


INCREMENTAL = 'incremental'
ABSOLUTE = 'absolute'

ATA = 'ata'
SCSI = 'scsi'
CSV = '.csv'

DEF_RESCAN_INTERVAL = 60
DEF_AGE = 30
DEF_PATH = '/var/log/smartd'

ATTR1 = '1'
ATTR2 = '2'
ATTR3 = '3'
ATTR4 = '4'
ATTR5 = '5'
ATTR7 = '7'
ATTR8 = '8'
ATTR9 = '9'
ATTR10 = '10'
ATTR11 = '11'
ATTR12 = '12'
ATTR13 = '13'
ATTR170 = '170'
ATTR171 = '171'
ATTR172 = '172'
ATTR173 = '173'
ATTR174 = '174'
ATTR180 = '180'
ATTR183 = '183'
ATTR190 = '190'
ATTR194 = '194'
ATTR196 = '196'
ATTR197 = '197'
ATTR198 = '198'
ATTR199 = '199'
ATTR202 = '202'
ATTR206 = '206'
ATTR_READ_ERR_COR = 'read-total-err-corrected'
ATTR_READ_ERR_UNC = 'read-total-unc-errors'
ATTR_WRITE_ERR_COR = 'write-total-err-corrected'
ATTR_WRITE_ERR_UNC = 'write-total-unc-errors'
ATTR_VERIFY_ERR_COR = 'verify-total-err-corrected'
ATTR_VERIFY_ERR_UNC = 'verify-total-unc-errors'
ATTR_TEMPERATURE = 'temperature'


RE_ATA = re.compile(
    '(\d+);'  # attribute
    '(\d+);'  # normalized value
    '(\d+)',  # raw value
    re.X
)

RE_SCSI = re.compile(
    '([a-z-]+);'  # attribute
    '([0-9.]+)',  # raw value
    re.X
)

ORDER = [
    # errors
    'read_error_rate',
    'seek_error_rate',
    'soft_read_error_rate',
    'write_error_rate',
    'read_total_err_corrected',
    'read_total_unc_errors',
    'write_total_err_corrected',
    'write_total_unc_errors',
    'verify_total_err_corrected',
    'verify_total_unc_errors',
    # external failure
    'sata_interface_downshift',
    'udma_crc_error_count',
    # performance
    'throughput_performance',
    'seek_time_performance',
    # power
    'start_stop_count',
    'power_on_hours_count',
    'power_cycle_count',
    'unexpected_power_loss',
    # spin
    'spin_up_time',
    'spin_up_retries',
    'calibration_retries',
    # temperature
    'airflow_temperature_celsius',
    'temperature_celsius',
    # wear
    'reallocated_sectors_count',
    'reserved_block_count',
    'program_fail_count',
    'erase_fail_count',
    'wear_leveller_worst_case_erase_count',
    'unused_reserved_nand_blocks',
    'reallocation_event_count',
    'current_pending_sector_count',
    'offline_uncorrectable_sector_count',
    'percent_lifetime_used',
]

CHARTS = {
    'read_error_rate': {
        'options': [None, 'Read Error Rate', 'value', 'errors', 'smartd_log.read_error_rate', 'line'],
        'lines': [],
        'attrs': [ATTR1],
        'algo': ABSOLUTE,
    },
    'seek_error_rate': {
        'options': [None, 'Seek Error Rate', 'value', 'errors', 'smartd_log.seek_error_rate', 'line'],
        'lines': [],
        'attrs': [ATTR7],
        'algo': ABSOLUTE,
    },
    'soft_read_error_rate': {
        'options': [None, 'Soft Read Error Rate', 'errors', 'errors', 'smartd_log.soft_read_error_rate', 'line'],
        'lines': [],
        'attrs': [ATTR13],
        'algo': INCREMENTAL,
    },
    'write_error_rate': {
        'options': [None, 'Write Error Rate', 'value', 'errors', 'smartd_log.write_error_rate', 'line'],
        'lines': [],
        'attrs': [ATTR206],
        'algo': ABSOLUTE,
    },
    'read_total_err_corrected': {
        'options': [None, 'Read Error Corrected', 'errors', 'errors', 'smartd_log.read_total_err_corrected', 'line'],
        'lines': [],
        'attrs': [ATTR_READ_ERR_COR],
        'algo': INCREMENTAL,
    },
    'read_total_unc_errors': {
        'options': [None, 'Read Error Uncorrected', 'errors', 'errors', 'smartd_log.read_total_unc_errors', 'line'],
        'lines': [],
        'attrs': [ATTR_READ_ERR_UNC],
        'algo': INCREMENTAL,
    },
    'write_total_err_corrected': {
        'options': [None, 'Write Error Corrected', 'errors', 'errors', 'smartd_log.read_total_err_corrected', 'line'],
        'lines': [],
        'attrs': [ATTR_WRITE_ERR_COR],
        'algo': INCREMENTAL,
    },
    'write_total_unc_errors': {
        'options': [None, 'Write Error Uncorrected', 'errors', 'errors', 'smartd_log.write_total_unc_errors', 'line'],
        'lines': [],
        'attrs': [ATTR_WRITE_ERR_UNC],
        'algo': INCREMENTAL,
    },
    'verify_total_err_corrected': {
        'options': [None, 'Verify Error Corrected', 'errors', 'errors', 'smartd_log.verify_total_err_corrected',
                    'line'],
        'lines': [],
        'attrs': [ATTR_VERIFY_ERR_COR],
        'algo': INCREMENTAL,
    },
    'verify_total_unc_errors': {
        'options': [None, 'Verify Error Uncorrected', 'errors', 'errors', 'smartd_log.verify_total_unc_errors', 'line'],
        'lines': [],
        'attrs': [ATTR_VERIFY_ERR_UNC],
        'algo': INCREMENTAL,
    },
    'sata_interface_downshift': {
        'options': [None, 'SATA Interface Downshift', 'events', 'external failure',
                    'smartd_log.sata_interface_downshift', 'line'],
        'lines': [],
        'attrs': [ATTR183],
        'algo': INCREMENTAL,
    },
    'udma_crc_error_count': {
        'options': [None, 'UDMA CRC Error Count', 'errors', 'external failure', 'smartd_log.udma_crc_error_count',
                    'line'],
        'lines': [],
        'attrs': [ATTR199],
        'algo': INCREMENTAL,
    },
    'throughput_performance': {
        'options': [None, 'Throughput Performance', 'value', 'performance', 'smartd_log.throughput_performance',
                    'line'],
        'lines': [],
        'attrs': [ATTR2],
        'algo': ABSOLUTE,
    },
    'seek_time_performance': {
        'options': [None, 'Seek Time Performance', 'value', 'performance', 'smartd_log.seek_time_performance', 'line'],
        'lines': [],
        'attrs': [ATTR8],
        'algo': ABSOLUTE,
    },
    'start_stop_count': {
        'options': [None, 'Start/Stop Count', 'events', 'power', 'smartd_log.start_stop_count', 'line'],
        'lines': [],
        'attrs': [ATTR4],
        'algo': ABSOLUTE,
    },
    'power_on_hours_count': {
        'options': [None, 'Power-On Hours Count', 'hours', 'power', 'smartd_log.power_on_hours_count', 'line'],
        'lines': [],
        'attrs': [ATTR9],
        'algo': ABSOLUTE,
    },
    'power_cycle_count': {
        'options': [None, 'Power Cycle Count', 'events', 'power', 'smartd_log.power_cycle_count', 'line'],
        'lines': [],
        'attrs': [ATTR12],
        'algo': ABSOLUTE,
    },
    'unexpected_power_loss': {
        'options': [None, 'Unexpected Power Loss', 'events', 'power', 'smartd_log.unexpected_power_loss', 'line'],
        'lines': [],
        'attrs': [ATTR174],
        'algo': ABSOLUTE,
    },
    'spin_up_time': {
        'options': [None, 'Spin-Up Time', 'ms', 'spin', 'smartd_log.spin_up_time', 'line'],
        'lines': [],
        'attrs': [ATTR3],
        'algo': ABSOLUTE,
    },
    'spin_up_retries': {
        'options': [None, 'Spin-up Retries', 'retries', 'spin', 'smartd_log.spin_up_retries', 'line'],
        'lines': [],
        'attrs': [ATTR10],
        'algo': INCREMENTAL,
    },
    'calibration_retries': {
        'options': [None, 'Calibration Retries', 'retries', 'spin', 'smartd_log.calibration_retries', 'line'],
        'lines': [],
        'attrs': [ATTR11],
        'algo': INCREMENTAL,
    },
    'airflow_temperature_celsius': {
        'options': [None, 'Airflow Temperature Celsius', 'celsius', 'temperature',
                    'smartd_log.airflow_temperature_celsius', 'line'],
        'lines': [],
        'attrs': [ATTR190],
        'algo': ABSOLUTE,
    },
    'temperature_celsius': {
        'options': [None, 'Temperature', 'celsius', 'temperature', 'smartd_log.temperature_celsius', 'line'],
        'lines': [],
        'attrs': [ATTR194, ATTR_TEMPERATURE],
        'algo': ABSOLUTE,
    },
    'reallocated_sectors_count': {
        'options': [None, 'Reallocated Sectors Count', 'sectors', 'wear', 'smartd_log.reallocated_sectors_count',
                    'line'],
        'lines': [],
        'attrs': [ATTR5],
        'algo': INCREMENTAL,
    },
    'reserved_block_count': {
        'options': [None, 'Reserved Block Count', 'percentage', 'wear', 'smartd_log.reserved_block_count', 'line'],
        'lines': [],
        'attrs': [ATTR170],
        'algo': ABSOLUTE,
    },
    'program_fail_count': {
        'options': [None, 'Program Fail Count', 'errors', 'wear', 'smartd_log.program_fail_count', 'line'],
        'lines': [],
        'attrs': [ATTR171],
        'algo': INCREMENTAL,
    },
    'erase_fail_count': {
        'options': [None, 'Erase Fail Count', 'failures', 'wear', 'smartd_log.erase_fail_count', 'line'],
        'lines': [],
        'attrs': [ATTR172],
        'algo': INCREMENTAL,
    },
    'wear_leveller_worst_case_erase_count': {
        'options': [None, 'Wear Leveller Worst Case Erase Count', 'erases', 'wear',
                    'smartd_log.wear_leveller_worst_case_erase_count', 'line'],
        'lines': [],
        'attrs': [ATTR173],
        'algo': ABSOLUTE,
    },
    'unused_reserved_nand_blocks': {
        'options': [None, 'Unused Reserved NAND Blocks', 'blocks', 'wear', 'smartd_log.unused_reserved_nand_blocks',
                    'line'],
        'lines': [],
        'attrs': [ATTR180],
        'algo': ABSOLUTE,
    },
    'reallocation_event_count': {
        'options': [None, 'Reallocation Event Count', 'events', 'wear', 'smartd_log.reallocation_event_count', 'line'],
        'lines': [],
        'attrs': [ATTR196],
        'algo': INCREMENTAL,
    },
    'current_pending_sector_count': {
        'options': [None, 'Current Pending Sector Count', 'sectors', 'wear', 'smartd_log.current_pending_sector_count',
                    'line'],
        'lines': [],
        'attrs': [ATTR197],
        'algo': ABSOLUTE,
    },
    'offline_uncorrectable_sector_count': {
        'options': [None, 'Offline Uncorrectable Sector Count', 'sectors', 'wear',
                    'smartd_log.offline_uncorrectable_sector_count', 'line'],
        'lines': [],
        'attrs': [ATTR198],
        'algo': ABSOLUTE,

    },
    'percent_lifetime_used': {
        'options': [None, 'Percent Lifetime Used', 'percentage', 'wear', 'smartd_log.percent_lifetime_used', 'line'],
        'lines': [],
        'attrs': [ATTR202],
        'algo': ABSOLUTE,
    }
}

# NOTE: 'parse_temp' decodes ATA 194 raw value. Not heavily tested. Written by @Ferroin
# C code:
# https://github.com/smartmontools/smartmontools/blob/master/smartmontools/atacmds.cpp#L2051
#
# Calling 'parse_temp' on the raw value will return a 4-tuple, containing
#  * temperature
#  * minimum
#  * maximum
#  * over-temperature count
# substituting None for values it can't decode.
#
# Example:
# >>> parse_temp(42952491042)
# >>> (34, 10, 43, None)
#
#
# def check_temp_word(i):
#     if i <= 0x7F:
#         return 0x11
#     elif i <= 0xFF:
#         return 0x01
#     elif 0xFF80 <= i:
#         return 0x10
#     return 0x00
#
#
# def check_temp_range(t, b0, b1):
#     if b0 > b1:
#         t0, t1 = b1, b0
#     else:
#         t0, t1 = b0, b1
#
#     if all([
#         -60 <= t0,
#         t0 <= t,
#         t <= t1,
#         t1 <= 120,
#         not (t0 == -1 and t1 <= 0)
#     ]):
#         return t0, t1
#     return None, None
#
#
# def parse_temp(raw):
#     byte = list()
#     word = list()
#     for i in range(0, 6):
#         byte.append(0xFF & (raw >> (i * 8)))
#     for i in range(0, 3):
#         word.append(0xFFFF & (raw >> (i * 16)))
#
#     ctwd = check_temp_word(word[0])
#
#     if not word[2]:
#         if ctwd and not word[1]:
#             # byte[0] is temp, no other data
#             return byte[0], None, None, None
#
#         if ctwd and all(check_temp_range(byte[0], byte[2], byte[3])):
#             # byte[0] is temp, byte[2] is max or min, byte[3] is min or max
#             trange = check_temp_range(byte[0], byte[2], byte[3])
#             return byte[0], trange[0], trange[1], None
#
#         if ctwd and all(check_temp_range(byte[0], byte[1], byte[2])):
#             # byte[0] is temp, byte[1] is max or min, byte[2] is min or max
#             trange = check_temp_range(byte[0], byte[1], byte[2])
#             return byte[0], trange[0], trange[1], None
#
#         return None, None, None, None
#
#     if ctwd:
#         if all(
#                 [
#                     ctwd & check_temp_word(word[1]) & check_temp_word(word[2]) != 0x00,
#                     all(check_temp_range(byte[0], byte[2], byte[4])),
#                 ]
#         ):
#             # byte[0] is temp, byte[2] is max or min, byte[4] is min or max
#             trange = check_temp_range(byte[0], byte[2], byte[4])
#             return byte[0], trange[0], trange[1], None
#         else:
#             trange = check_temp_range(byte[0], byte[2], byte[3])
#             if word[2] < 0x7FFF and all(trange) and trange[1] >= 40:
#                 # byte[0] is temp, byte[2] is max or min, byte[3] is min or max, word[2] is overtemp count
#                 return byte[0], trange[0], trange[1], word[2]
#     # no data
#     return None, None, None, None


CHARTED_ATTRS = dict((attr, k) for k, v in CHARTS.items() for attr in v['attrs'])


class BaseAtaSmartAttribute:
    def __init__(self, name, normalized_value, raw_value):
        self.name = name
        self.normalized_value = normalized_value
        self.raw_value = raw_value

    def value(self):
        raise NotImplementedError


class AtaRaw(BaseAtaSmartAttribute):
    def value(self):
        return self.raw_value


class AtaNormalized(BaseAtaSmartAttribute):
    def value(self):
        return self.normalized_value


class Ata3(BaseAtaSmartAttribute):
    def value(self):
        value = int(self.raw_value)
        # https://github.com/netdata/netdata/issues/5919
        #
        # 3;151;38684000679;
        # 423 (Average 447)
        # 38684000679 & 0xFFF -> 423
        # (38684000679 & 0xFFF0000) >> 16 -> 447
        if value > 1e6:
            return value & 0xFFF
        return value


class Ata9(BaseAtaSmartAttribute):
    def value(self):
        value = int(self.raw_value)
        if value > 1e6:
            return value & 0xFFFF
        return value


class Ata190(BaseAtaSmartAttribute):
    def value(self):
        return 100 - int(self.normalized_value)


class Ata194(BaseAtaSmartAttribute):
    # https://github.com/netdata/netdata/issues/3041
    # https://github.com/netdata/netdata/issues/5919
    #
    # The low byte is the current temperature, the third lowest is the maximum, and the fifth lowest is the minimum
    def value(self):
        value = int(self.raw_value)
        if value > 1e6:
            return value & 0xFF
        return min(int(self.normalized_value), int(self.raw_value))


class BaseSCSISmartAttribute:
    def __init__(self, name, raw_value):
        self.name = name
        self.raw_value = raw_value

    def value(self):
        raise NotImplementedError


class SCSIRaw(BaseSCSISmartAttribute):
    def value(self):
        return self.raw_value


def ata_attribute_factory(value):
    name = value[0]

    if name == ATTR3:
        return Ata3(*value)
    elif name == ATTR9:
        return Ata9(*value)
    elif name == ATTR190:
        return Ata190(*value)
    elif name == ATTR194:
        return Ata194(*value)
    elif name in [
        ATTR1,
        ATTR7,
        ATTR202,
        ATTR206,
    ]:
        return AtaNormalized(*value)

    return AtaRaw(*value)


def scsi_attribute_factory(value):
    return SCSIRaw(*value)


def attribute_factory(value):
    name = value[0]
    if name.isdigit():
        return ata_attribute_factory(value)
    return scsi_attribute_factory(value)


def handle_error(*errors):
    def on_method(method):
        def on_call(*args):
            try:
                return method(*args)
            except errors:
                return None
        return on_call
    return on_method


class DiskLogFile:
    def __init__(self, full_path):
        self.path = full_path
        self.size = os.path.getsize(full_path)

    @handle_error(OSError)
    def is_changed(self):
        return self.size != os.path.getsize(self.path)

    @handle_error(OSError)
    def is_active(self, current_time, limit):
        return (current_time - os.path.getmtime(self.path)) / 60 < limit

    @handle_error(OSError)
    def read(self):
        self.size = os.path.getsize(self.path)
        return read_last_line(self.path)


class BaseDisk:
    def __init__(self, name, log_file):
        self.raw_name = name
        self.name = re.sub(r'_+', '_', name)
        self.log_file = log_file
        self.attrs = list()
        self.alive = True
        self.charted = False

    def __eq__(self, other):
        if isinstance(other, BaseDisk):
            return self.raw_name == other.raw_name
        return self.raw_name == other

    def __ne__(self, other):
        return not self == other

    def __hash__(self):
        return hash(repr(self))

    def parser(self, data):
        raise NotImplementedError

    @handle_error(TypeError)
    def populate_attrs(self):
        self.attrs = list()
        line = self.log_file.read()
        for value in self.parser(line):
            self.attrs.append(attribute_factory(value))

        return len(self.attrs)

    def data(self):
        data = dict()
        for attr in self.attrs:
            data['{0}_{1}'.format(self.name, attr.name)] = attr.value()
        return data


class ATADisk(BaseDisk):
    def parser(self, data):
        return RE_ATA.findall(data)


class SCSIDisk(BaseDisk):
    def parser(self, data):
        return RE_SCSI.findall(data)


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = deepcopy(CHARTS)
        self.log_path = configuration.get('log_path', DEF_PATH)
        self.age = configuration.get('age', DEF_AGE)
        self.exclude = configuration.get('exclude_disks', str()).split()
        self.disks = list()
        self.runs = 0

    def check(self):
        return self.scan() > 0

    def get_data(self):
        self.runs += 1

        if self.runs % DEF_RESCAN_INTERVAL == 0:
            self.cleanup()
            self.scan()

        data = dict()

        for disk in self.disks:
            if not disk.alive:
                continue

            if not disk.charted:
                self.add_disk_to_charts(disk)

            changed = disk.log_file.is_changed()

            if changed is None:
                disk.alive = False
                continue

            if changed and disk.populate_attrs() is None:
                disk.alive = False
                continue

            data.update(disk.data())

        return data

    def cleanup(self):
        current_time = time()
        for disk in self.disks[:]:
            if any(
                [
                    not disk.alive,
                    not disk.log_file.is_active(current_time, self.age),
                ]
            ):
                self.disks.remove(disk.raw_name)
                self.remove_disk_from_charts(disk)

    def scan(self):
        self.debug('scanning {0}'.format(self.log_path))
        current_time = time()

        for full_name in os.listdir(self.log_path):
            disk = self.create_disk_from_file(full_name, current_time)
            if not disk:
                continue
            self.disks.append(disk)

        return len(self.disks)

    def create_disk_from_file(self, full_name,  current_time):
        if not full_name.endswith(CSV):
            self.debug('skipping {0}: not a csv file'.format(full_name))
            return None

        name = os.path.basename(full_name).split('.')[-3]
        path = os.path.join(self.log_path, full_name)

        if name in self.disks:
            self.debug('skipping {0}: already in disks'.format(full_name))
            return None

        if [p for p in self.exclude if p in name]:
            self.debug('skipping {0}: filtered by `exclude` option'.format(full_name))
            return None

        if not os.access(path, os.R_OK):
            self.debug('skipping {0}: not readable'.format(full_name))
            return None

        if os.path.getsize(path) == 0:
            self.debug('skipping {0}: zero size'.format(full_name))
            return None

        if (current_time - os.path.getmtime(path)) / 60 > self.age:
            self.debug('skipping {0}: haven\'t been updated for last {1} minutes'.format(full_name, self.age))
            return None

        if ATA in full_name:
            disk = ATADisk(name, DiskLogFile(path))
        elif SCSI in full_name:
            disk = SCSIDisk(name, DiskLogFile(path))
        else:
            self.debug('skipping {0}: unknown type'.format(full_name))
            return None

        disk.populate_attrs()
        if not disk.attrs:
            self.error('skipping {0}: parsing failed'.format(full_name))
            return None

        self.debug('added {0}'.format(full_name))
        return disk

    def add_disk_to_charts(self, disk):
        if len(self.charts) == 0 or disk.charted:
            return
        disk.charted = True

        for attr in disk.attrs:
            chart_id = CHARTED_ATTRS.get(attr.name)

            if not chart_id or chart_id not in self.charts:
                continue

            chart = self.charts[chart_id]
            dim = [
                '{0}_{1}'.format(disk.name, attr.name),
                disk.name,
                CHARTS[chart_id]['algo'],
            ]

            if dim[0] in self.charts[chart_id].dimensions:
                chart.hide_dimension(dim[0], reverse=True)
            else:
                chart.add_dimension(dim)

    def remove_disk_from_charts(self, disk):
        if len(self.charts) == 0 or not disk.charted:
            return

        for attr in disk.attrs:
            chart_id = CHARTED_ATTRS.get(attr.name)

            if not chart_id or chart_id not in self.charts:
                continue

            self.charts[chart_id].del_dimension('{0}_{1}'.format(disk.name, attr.name))
