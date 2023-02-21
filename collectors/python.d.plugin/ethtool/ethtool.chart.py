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
        order.append('dev_{0}_rx_bw'.format(d.id))
        charts.update(
            {
                'dev_{0}_rx_bw'.format(d.id): {
                    'options': [None, 'Rx Bandwidth Bytes/sec', 'absolute', 'network_device',
                                'ethtool.dev_rx_bandwidth', 'line'],
                    'lines': [
                        ['dev_{0}_rx_bw'.format(d.id), 'device {0}'.format(d.id)],
                    ]
                }
            }
        )


    for d in devs:
        order.append('dev_{0}_packets_lost'.format(d.id))
        charts.update(
            {
                'dev_{0}_packets_lost'.format(d.id): {
                    'options': [None, 'Lost packets', 'Lost packets', 'network_device', 'ethtool.packets_lost', 'line'],
                    'lines': [
                        ['dev_{0}_packets_lost'.format(d.id), 'adapter {0}'.format(d.id)],
                    ]
                }
            }
        )

    return order, charts


class Metrics:
    def __init__(self, device_id, bytes_received, rx_bw, rx_discards_phy):
        self.id = device_id
        self.bytes_received = bytes_received
        self.rx_discards_phy = rx_discards_phy
        self.rx_bw = rx_bw

    def data(self):
        return {
            'dev_{0}_bytes_received'.format(self.id): self.bytes_received,
            'dev_{0}_rx_bw'.format(self.id): self.rx_bw,
            'dev_{0}_packets_lost'.format(self.id): self.rx_discards_phy,
        }


class Service(ExecutableService):
    def __init__(self, configuration=None, name=None):
        ExecutableService.__init__(self, configuration=configuration, name=name)
        self.dev_filter = self.configuration.get('dev_filter','')
        self.order = list()
        self.definitions = dict()
        self.s = find_binary('sudo')
        self.ethtool = find_binary('ethtool')
        self.ethtool_info =  [self.ethtool, '-S']
        self.devices = self.find_devices()
        self.last_update = 0
        self.start_rx_bytes_received = 0
        self.last_rx_bytes_received = 0
        

        
    def format_data(self, raw_data, name, last_update):
        rx_bytes_received = 0
        rx_discards_phy = 0
        for line in raw_data :
            if 'rx_bytes_phy' in line.split(':')[0] :
                rx_bytes_received = int(line.split(':')[1])
            if 'rx_discards_phy' in line.split(':')[0] :
                rx_discards_phy = int(line.split(':')[1])

        if self.last_update == 0 :
            rx_bw = 0
            self.start_rx_bytes_received = rx_bytes_received
            self.debug('init rx bw and last update start rx bytes received {}'.format(self.start_rx_bytes_received))
        else:
            nb_sec = (last_update - self.last_update).total_seconds()
            self.debug(' rx_bytes_received {0} last_rx_bytes_received {1} nb seconds {2}'.format(rx_bytes_received, self.last_rx_bytes_received, nb_sec))
            rx_bw = int((rx_bytes_received - self.last_rx_bytes_received) / nb_sec)
        self.last_rx_bytes_received = rx_bytes_received
        self.last_update = last_update
        toto = Metrics(name, self.last_rx_bytes_received, rx_bw, rx_discards_phy)
        self.debug('last_rx_bytes_received {}'.format(self.last_rx_bytes_received))
        self.debug('toto {}'.format(toto.data()))
        return toto

    def find_devices(self):
        ls = find_binary('ls')
        command = [ls, '/sys/class/net']
        raw_data = self._get_raw_data(command=command)
        device_names = [s.rstrip() for s in raw_data ]
        devices_filtered = []
        # Filter devices according to the parameter set in the conf file
        if self.dev_filter:
            for device in device_names:
                if self.dev_filter in device :
                    devices_filtered.append(device)
        else:
            devices_filtered = device_names

        return devices_filtered

    def check(self):
        self.debug('------ check my ethtool service ')
        if not self.devices:
            self.error('Not devices found "{0}" output'.format(' '.join(self.ethtool_info)))
            return False

        data = []
        for d in self.devices:
            command = self.ethtool_info.copy()
            command.append(d)
            current_time = datetime.datetime.now()
            raw_data = self._get_raw_data(command=command)
            data.append(self.format_data(raw_data, d, current_time))
            

        o, c = device_charts(data)
        self.order.extend(o)
        self.definitions.update(c)
        return True

    def get_ethtool_data(self):
        self.debug('------ get_ethtool data my ethtool service ')
        data = dict()

        for d in self.devices:
            command = self.ethtool_info.copy()
            command.append(d)
            current_time = datetime.datetime.now()
            raw_data = self._get_raw_data(command=command)
            data.update(self.format_data(raw_data, d, current_time).data())

        return data

    def get_data(self):
        data = dict()
        data.update(self.get_ethtool_data())
        return data or None

