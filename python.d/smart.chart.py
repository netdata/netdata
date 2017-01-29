# -*- coding: utf-8 -*-
# Description: smart netdata python.d module
#

from subprocess import Popen, PIPE
from base import SimpleService

# default module values (can be overridden per job in `config`)
update_every = 5
priority = 60000
retries = 5

# charts order (can be overridden if you want less charts, or different order)
ORDER = ["attr1", "attr4", "attr5", "attr7", "attr9", "attr12", "attr193", "attr194", "attr197", "attr198", "attr200"]

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
        self.disks = self._get_disks()

        self.definitions = {}

        for attrid, desc in SMART_ATTRIBUTES.items():
            self.definitions["attr" + attrid] = {
                "options": [None, desc, "value", desc, "smart.attr" + attrid, "line"],
                "lines": self._get_disk_lines(attrid)
            }

    def _get_disks(self):
        dList, lErr = Popen(['smartctl', '--scan-open'], stdout=PIPE, stderr=PIPE).communicate()

        ret = []

        for line in dList.split(b'\n'):
            deviceName = line.decode().split(' ')[0].replace('/dev/','')

            if deviceName:
                ret.append(deviceName)

        return ret

    def _get_disk_lines(self, attrid):
        ret = []

        for disk in self.disks:
            ret.append([
                attrid + "_" + disk, disk, "absolute"
            ]);

        return ret

    def _get_raw_data_for_disk(self, disk):
        ret = []

        dDetails, dErr = Popen(['smartctl', '-a', '/dev/' + disk], stdout=PIPE, stderr=PIPE).communicate()

        attributesSection = False

        for line in dDetails.split(b'\n'):
            if attributesSection:
                if line:
                    ret.append(line.decode().split())
                else:
                    break

            if line.decode().startswith('ID#'):
                attributesSection = True

        return ret

    def _get_data(self):
        data = {}

        for disk in self.disks:
            for line in self._get_raw_data_for_disk(disk):
                data[line[0] + "_" + disk] = line[9]

        return data
