# -*- coding: utf-8 -*-
# Description: nvidia-smi netdata python.d module
# Original Author: Steven Noonan (tycho)
# Author: Ilya Mashchenko (ilyam8)
# User Memory Stat Author: Guido Scatena (scatenag)

import datetime
import re

from bases.FrameworkServices.ExecutableService import ExecutableService
from bases.collection import find_binary

disabled_by_default = True

ETHTOOL = 'ethtool'

RX_BW = 'rx_bw'
TX_BW = 'tx_bw'

ORDER = [
    RX_BW,
    TX_BW,
]

RE_BATTERY = re.compile(
    r'BBU Capacity Info for Adapter: ([0-9]+) Relative State of Charge: ([0-9]+) % Cycle Count: ([0-9]+)'
)

# def ethernet_charts(device):
#     order = [
#         'bytes_received',
#     ]

#     charts = {
#         'bytes_received': {
#             'options': [None, 'RX Bytes', 'Bytes', "{0}".format(device.id), 'ethtool.bytes_received', 'line'],
#             'lines': [
#                 ['dev_{0}_bytes_received'.format(device.id), 'received', 'absolute', 1, 1],
#             ]
#         },
#     }

#     return order, charts

class Ethtool :
    def __init__(self, num, raw):
        self.raw = raw
        self.num = num

    def data(self):
        data = {
            'bytes_received' : 0,
            'bytes_lost' :0,
        }
        return dict(('gpu{0}_{1}'.format(self.num, k), v) for k, v in data.items())


def device_charts(devs):
    order = list()
    charts = dict()

    for d in devs:
        order.append('dev_{0}_bytes_received'.format(d.id))
        charts.update(
            {
                'dev_{0}_bytes_received'.format(d.id): {
                    'options': [None, 'Bytes received', 'absolute', 'network_device',
                                'ethtool.dev_bytes_received', 'line'],
                    'lines': [
                        ['dev_{0}_bytes_received'.format(d.id), 'device {0}'.format(d.id)],
                    ]
                }
            }
        )

    for d in devs:
        order.append('dev_{0}_bytes_lost'.format(d.id))
        charts.update(
            {
                'dev_{0}_bytes_lost'.format(d.id): {
                    'options': [None, 'Bytes lost', 'bytes lost', 'network_device', 'ethtool.dev_bytes_lost', 'line'],
                    'lines': [
                        ['dev_{0}_bytes_lost'.format(d.id), 'adapter {0}'.format(d.id)],
                    ]
                }
            }
        )

    return order, charts


class Metrics:
    def __init__(self, device_id, bytes_received, rx_discards_phy):
        self.id = device_id
        self.bytes_received = bytes_received
        self.rx_discards_phy = rx_discards_phy

    def data(self):
        return {
            'dev_{0}_bytes_received'.format(self.id): self.bytes_received,
            'dev_{0}_bytes_lost'.format(self.id): self.rx_discards_phy,
        }

def format_data(raw_data, name):
    rx_bytes_received = 0
    rx_discards_phy = 0
    for line in raw_data :
        if 'rx_bytes_phy' in line.split(':')[0] :
            rx_bytes_received = line.split(':')[1]
        if 'rx_discards_phy' in line.split(':')[0] :
            rx_discards_phy = line.split(':')[1]
    return Metrics(name, rx_bytes_received, rx_discards_phy)


class Service(ExecutableService):
    def __init__(self, configuration=None, name=None):
        ExecutableService.__init__(self, configuration=configuration, name=name)
        self.order = list()
        self.definitions = dict()
        self.s = find_binary('sudo')
        self.ethtool = find_binary('ethtool')
        self.ethtool_info =  [self.ethtool, '-S']
        self.devices = self.find_devices()
        
    def find_devices(self):
        #TODO implement method to find all devices
        # for the moment hardcoded
        ifconfig = find_binary('ifconfig')
        command = [ifconfig, '-s']
        raw_data = self._get_raw_data(command=command)
        device_names = []
        for line in raw_data:
            name = line.split()[0]
            if name == 'Iface':
                continue
            elif name == "ens1f0":
            # else :
                device_names.append(name)
        return device_names

    def check(self):
        self.debug('------ check my ethtool service ')
        if not self.devices:
            self.error('Not devices found "{0}" output'.format(' '.join(self.ethtool_info)))
            return False

        data = []
        for d in self.devices:
            command = self.ethtool_info.copy()
            command.append(d)
            raw_data = self._get_raw_data(command=command)
            data.append(format_data(raw_data, d))
            break # WIP

        o, c = device_charts(data)
        self.order.extend(o)
        self.definitions.update(c)
        return True

    def get_ethtool_data(self):
        data = dict()

        for d in self.devices:
            command = self.ethtool_info.copy()
            command.append(d)
            start = datetime.datetime.now()
            raw_data = self._get_raw_data(command=command)
            end = datetime.datetime.now()
            self.debug('------ Get raw data at {0} finish at {1} elapsed {2}'.format(start, end, end - start))
            data.update(format_data(raw_data, d).data())

        return data

    def get_data(self):
        data = dict()
        data.update(self.get_ethtool_data())
        return data or None

