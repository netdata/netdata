# -*- coding: utf-8 -*-
# Description: smart netdata python.d module
# Author: l2isbad, vorph1

from re import compile as r_compile
from os import listdir, access, R_OK
from os.path import isfile, join, getsize, basename, isdir
try:
    from queue import Queue
except ImportError:
    from Queue import Queue
from threading import Thread
from base import SimpleService
from collections import namedtuple

# default module values (can be overridden per job in `config`)
update_every = 5
priority = 60000

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

NAMED_DISKS = namedtuple('disks', ['name', 'size', 'number'])


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.regex = r_compile(r'(\d+);(\d+);(\d+)')
        self.log_path = self.configuration.get('log_path', '/var/log/smartd')
        self.raw_values = self.configuration.get('raw_values')
        self.attr = self.configuration.get('smart_attributes', [])
        self.previous_data = dict()

    def check(self):
        # Can\'t start without smartd readable diks log files
        disks = find_disks_in_log_path(self.log_path)
        if not disks:
            self.error('Can\'t locate any smartd log files in %s' % self.log_path)
            return False

        # List of namedtuples to track smartd log file size
        self.disks = [NAMED_DISKS(name=disks[i], size=0, number=i) for i in range(len(disks))]

        if self._get_data():
            self.create_charts()
            return True
        else:
            self.error('Can\'t collect any data. Sorry.')
            return False

    def _get_raw_data(self, queue, disk):
        # The idea is to open a file.
        # Jump to the end.
        # Seek backward until '\n' symbol appears
        # If '\n' is found or it's the beginning of the file
        # readline()! (last or first line)
        with open(disk, 'rb') as f:
            f.seek(-2, 2)
            while f.read(1) != b'\n':
                f.seek(-2, 1)
                if f.tell() == 0:
                    break
            result = f.readline()

        result = result.decode()
        result = self.regex.findall(result)

        queue.put([basename(disk), result])

    def _get_data(self):
        threads, result = list(), list()
        queue = Queue()
        to_netdata = dict()

        # If the size has not changed there is no reason to poll log files.
        disks = [disk for disk in self.disks if self.size_changed(disk)]
        if disks:
            for disk in disks:
                th = Thread(target=self._get_raw_data, args=(queue, disk.name))
                th.start()
                threads.append(th)

            for thread in threads:
                thread.join()
                result.append(queue.get())
        else:
            # Data from last real poll
            return self.previous_data or None

        for elem in result:
            for a, n, r in elem[1]:
                to_netdata.update({'_'.join([elem[0], a]): r if self.raw_values else n})

        self.previous_data.update(to_netdata)

        return to_netdata or None

    def size_changed(self, disk):
        # We are not interested in log files:
        # 1. zero size
        # 2. size is not changed since last poll
        try:
            size = getsize(disk.name)
            if  size != disk.size and size:
                self.disks[disk.number] = disk._replace(size=size)
                return True
            else:
                return False
        except OSError:
            # Remove unreadable/nonexisting log files from list of disks and previous_data
            self.disks.remove(disk)
            self.previous_data = dict([(k, v) for k, v in self.previous_data.items() if basename(disk.name) not in k])
            return False

    def create_charts(self):

        def create_lines(attrid):
            result = list()
            for disk in self.disks:
                name = basename(disk.name)
                result.append(['_'.join([name, attrid]), name[:name.index('.')], 'absolute'])
            return result

        # Use configured attributes, if present. If something goes wrong we don't care.
        order = ORDER
        try:
            order = [attr for attr in self.attr.split() if attr in SMART_ATTR.keys()] or ORDER
        except Exception:
            pass
        self.order = [''.join(['attrid', i]) for i in order]
        self.definitions = dict()
        units = 'raw' if self.raw_values else 'normalized'

        for k, v in dict([(k, v) for k, v in SMART_ATTR.items() if k in ORDER]).items():
            self.definitions.update({''.join(['attrid', k]): {
                                      'options': [None, v, units, v.lower(), 'smartd.attrid' + k, 'line'],
                                       'lines': create_lines(k)}})

def find_disks_in_log_path(log_path):
    # smartd log file is OK if:
    # 1. it is a file
    # 2. file name endswith with 'csv'
    # 3. file is readable
    if not isdir(log_path): return None
    return [join(log_path, f) for f in listdir(log_path)
            if all([isfile(join(log_path, f)), f.endswith('.csv'), access(join(log_path, f), R_OK)])]
