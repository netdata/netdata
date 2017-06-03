# -*- coding: utf-8 -*-
# Description: smart netdata python.d module
#

from subprocess import Popen, PIPE
import re
import os
import stat
import msg

from base import SimpleService

# default module values (can be overridden per job in `config`)
update_every = 5
priority = 60000
retries = 5
command = None

# charts order (can be overridden if you want less charts, or different order)
ORDER = ["attr1", "attr4", "attr5", "attr7", "attr9", "attr12", "attr193", "attr194", "attr197", "attr198", "attr200"]

SMARTCTL_PATHS = [
    "/bin/smartctl",
    "/sbin/smartctl",
    "/usr/bin/smartctl",
    "/usr/sbin/smartctl",
]

SMART_ATTRIBUTES = {
   "1": "Read Error Rate",
   "2": "Throughput Performance",
   "3": "Spin-Up Time",
   "4": "Start/Stop Count",
   "5": "Reallocated Sectors Count",
   "6": "Read Channel Margin",
   "7": "Seek Error Rate",
   "8": "Seek Time Performance",
   "9": "Power-On Hours Count",
   "10": "Spin-up Retries",
   "11": "Calibration Retries",
   "12": "Power Cycle Count",
   "13": "Soft Read Error Rate",
   "100": "Erase/Program Cycles",
   "103": "Translation Table Rebuild",
   "108": "Unknown (108)",
   "170": "Reserved Block Count",
   "171": "Program Fail Count",
   "172": "Erase Fail Count",
   "173": "Wear Leveller Worst Case Erase Count",
   "174": "Unexpected Power Loss",
   "175": "Program Fail Count",
   "176": "Erase Fail Count",
   "177": "Wear Leveling Count",
   "178": "Used Reserved Block Count",
   "179": "Used Reserved Block Count",
   "180": "Unused Reserved Block Count",
   "181": "Program Fail Count",
   "182": "Erase Fail Count",
   "183": "SATA Downshifts",
   "184": "End-to-End error",
   "185": "Head Stability",
   "186": "Induced Op-Vibration Detection",
   "187": "Reported Uncorrectable Errors",
   "188": "Command Timeout",
   "189": "High Fly Writes",
   "190": "Temperature",
   "191": "G-Sense Errors",
   "192": "Power-Off Retract Cycles",
   "193": "Load/Unload Cycles",
   "194": "Temperature",
   "195": "Hardware ECC Recovered",
   "196": "Reallocation Events",
   "197": "Current Pending Sectors",
   "198": "Off-line Uncorrectable",
   "199": "UDMA CRC Error Rate",
   "200": "Write Error Rate",
   "201": "Soft Read Errors",
   "202": "Data Address Mark Errors",
   "203": "Run Out Cancel",
   "204": "Soft ECC Corrections",
   "205": "Thermal Asperity Rate",
   "206": "Flying Height",
   "207": "Spin High Current",
   "209": "Offline Seek Performance",
   "220": "Disk Shift",
   "221": "G-Sense Error Rate",
   "222": "Loaded Hours",
   "223": "Load/Unload Retries",
   "224": "Load Friction",
   "225": "Load/Unload Cycles",
   "226": "Load-in Time",
   "227": "Torque Amplification Count",
   "228": "Power-Off Retracts",
   "230": "GMR Head Amplitude",
   "231": "Temperature",
   "232": "Available Reserved Space",
   "233": "Media Wearout Indicator",
   "240": "Head Flying Hours",
   "241": "Total LBAs Written",
   "242": "Total LBAs Read",
   "250": "Read Error Retry Rate"
}

class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.regex = re.compile(r'(\d+)[a-zA-Z-_\s]+0x.*?(?:Always|Offline)[\s-]+(\d+)')
        self.command = None

    def _get_disks(self):
        dList, lErr = Popen([self.command, '--scan-open'], stdout=PIPE, stderr=PIPE).communicate()

        ret = []

        for line in dList.decode(encoding='utf-8').split('\n'):
            deviceName = line.split(' ')[0].replace('/dev/','')

            if deviceName and deviceName != "#":
                ret.append(deviceName)

        return ret

    def _get_disk_lines(self, attrid):
        ret = []

        for disk in self.disks:
            ret.append([
                attrid + "_" + disk, disk, "absolute"
            ]);

        return ret

    def _get_raw_data(self, disk):
        try:
            raw_out, raw_err = Popen([self.command, '-a', '/dev/' + disk], stdout=PIPE, stderr=PIPE).communicate()
        except Exception:
            return None

        return raw_out.decode(encoding='utf-8')

    def _get_data(self):
        ret = dict()

        for disk in self.disks:
            data = self._get_raw_data(disk)
            if data:
                ret.update({
                    '_'.join([attrid, disk]): value for attrid, value in self.regex.findall(data)
                })

        return ret or None

    def check(self):
        if not self.command:
            for path in SMARTCTL_PATHS:
                if os.path.isfile(path):
                    self.command = path
                    break

        if not self.command:
            self.error("Can't locate smartctl binary")
            return False

        if os.stat(self.command).st_mode & stat.S_ISUID == 0:
            self.error("%s needs to have SUID set" % self.command)
            return False

        self.disks = self._get_disks()
        self.definitions = {}

        for attrid, desc in SMART_ATTRIBUTES.items():
            self.definitions["attr" + attrid] = {
                "options": [None, desc, "value", desc, "smart.attr" + attrid, "line"],
                "lines": self._get_disk_lines(attrid)
            }

        return bool(self._get_data())
