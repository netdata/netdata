# -*- coding: utf-8 -*-
# Description: ethtool netdata python.d module
# Author: Van Phan Quang (ghanapunq)

import datetime

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


def device_charts(devs):
    order = list()
    charts = dict()

    for d in devs:
        fam = d.id

        order.append('dev_{0}_bw'.format(d.id))
        charts.update(
            {
                'dev_{0}_bw'.format(d.id): {
                    'options': [None, 'Bandwidth Gb/s', 'absolute', fam,
                                'ethtool.dev_bandwidth', 'line'],
                    'lines': [
                        ['dev_{0}_rx_bw'.format(d.id), 'rx device'],
                        ['dev_{0}_tx_bw'.format(d.id), 'tx device'],
                    ]
                }
            }
        )

        order.append('dev_{0}_bw_util'.format(d.id))
        charts.update(
            {
                'dev_{0}_bw_util'.format(d.id): {
                    'options': [None, 'Bandwidth Percentage', 'percentage', fam,
                                'ethtool.dev_bandwidth_util', 'line'],
                    'lines': [
                        ['dev_{0}_rx_bw_util'.format(d.id), 'rx util'],
                        ['dev_{0}_tx_bw_util'.format(d.id), 'tx util'],
                    ]
                }
            }
        )

        order.append('dev_{0}_packets_lost'.format(d.id))
        charts.update(
            {
                'dev_{0}_packets_lost'.format(d.id): {
                    'options': [None, 'Lost packets', 'Lost packets', fam, 'ethtool.packets_lost', 'line'],
                    'lines': [
                        ['dev_{0}_rx_packets_lost'.format(d.id), 'rx lost packets'],
                        ['dev_{0}_tx_packets_lost'.format(d.id), 'tx lost packets'],
                    ]
                }
            }
        )

    return order, charts


class Metrics:
    def __init__(self, device_id, rx_bytes, rx_bw, rx_bw_util, rx_discards_phy,
                 tx_bytes, tx_bw, tx_bw_util, tx_discards_phy, update_time):
        self.id = device_id
        self.rx_bytes = rx_bytes
        self.rx_discards_phy = rx_discards_phy
        self.rx_bw = rx_bw
        self.rx_bw_util = rx_bw_util
        self.tx_bytes = tx_bytes
        self.tx_discards_phy = tx_discards_phy
        self.tx_bw = tx_bw
        self.tx_bw_util = tx_bw_util
        self.update_time = update_time

    def data(self):
        return {
            'dev_{0}_rx_bw'.format(self.id): self.rx_bw,
            'dev_{0}_tx_bw'.format(self.id): self.tx_bw,
            'dev_{0}_rx_bw_util'.format(self.id): self.rx_bw_util,
            'dev_{0}_tx_bw_util'.format(self.id): self.tx_bw_util,
            'dev_{0}_rx_packets_lost'.format(self.id): self.rx_discards_phy,
            'dev_{0}_tx_packets_lost'.format(self.id): self.tx_discards_phy,
        }


class Service(ExecutableService):
    def __init__(self, configuration=None, name=None):
        ExecutableService.__init__(self, configuration=configuration, name=name)
        self.dev_filter = self.configuration.get('dev_filter', '')
        self.order = list()
        self.definitions = dict()
        self.s = find_binary('sudo')
        self.ethtool = find_binary('ethtool')
        self.ethtool_info = [self.ethtool, '-S']
        self.devices = self.find_devices()
        self.metrics = dict()
        self.max_bw_per_device = self.get_max_bw_per_device()

    def format_data(self, raw_data, name, last_update):
        rx_bytes = 0
        rx_discards_phy = 0
        for line in raw_data:
            if 'rx_bytes_phy' in line.split(':')[0]:
                rx_bytes = int(line.split(':')[1])
            if 'rx_discards_phy' in line.split(':')[0]:
                rx_discards_phy = int(line.split(':')[1])
            if 'tx_bytes_phy' in line.split(':')[0]:
                tx_bytes = int(line.split(':')[1])
            if 'tx_discards_phy' in line.split(':')[0]:
                tx_discards_phy = int(line.split(':')[1])

        if name not in self.metrics.keys():
            self.debug('Init metrics of device {}'.format(name))
            self.metrics[name] = Metrics(name, 0, 0, 0, 0, 0, 0, 0, 0, last_update)
            rx_bw = 0
            rx_bw_util = 0
            tx_bw = 0
            tx_bw_util = 0
        else:
            nb_sec = (last_update - self.metrics[name].update_time).total_seconds()
            # self.debug(' rx_bytes {0} last_rx_bytes {1} nb seconds {2}'.format(rx_bytes, self.metrics[name].rx_bytes,
            #  nb_sec))
            # Calculation of the bandwidth in Gb/s
            rx_bw = int((rx_bytes - self.metrics[name].rx_bytes) / nb_sec * 8 / (1000*1000*1000))
            rx_bw_util = rx_bw / self.max_bw_per_device[name] * 100
            tx_bw = int((tx_bytes - self.metrics[name].tx_bytes) / nb_sec * 8 / (1000*1000*1000))
            tx_bw_util = int(tx_bw / self.max_bw_per_device[name] * 100)
        data = Metrics(name, rx_bytes, rx_bw, rx_bw_util, rx_discards_phy,
                       tx_bytes, tx_bw, tx_bw_util, tx_discards_phy, last_update)
        self.debug('toto {}'.format(data.data()))
        self.metrics[name] = data
        return data

    def find_devices(self):
        ls = find_binary('ls')
        command = [ls, '/sys/class/net']
        raw_data = self._get_raw_data(command=command)
        device_names = [s.rstrip() for s in raw_data]
        devices_filtered = []
        # Filter devices according to the parameter set in the conf file
        if self.dev_filter:
            for device in device_names:
                if self.dev_filter in device:
                    devices_filtered.append(device)
        else:
            devices_filtered = device_names

        return devices_filtered

    def get_max_bw_per_device(self):
        max_bw_per_device = dict()
        for d in self.devices:
            command = [find_binary('ethtool'), d]
            raw_data = self._get_raw_data(command=command)
            for line in raw_data:
                s = line.split(':')
                if 'Speed' in s[0]:
                    bw = int(s[1].split('M')[0]) / 1000  # in Gb/s
                    self.debug('------- Max speed of device {0} is {1} Gb/s'.format(d, bw))
                    max_bw_per_device[d] = bw

        return max_bw_per_device

    def check(self):
        # check when it is used
        self.debug('------ check my ethtool service ')
        if not self.devices:
            self.error('No devices found "{0}" output'.format(' '.join(self.ethtool_info)))
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
