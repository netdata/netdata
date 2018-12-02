# -*- coding: utf-8 -*-
# Description: nvidia-smi netdata python.d module
# Original Author: Steven Noonan (tycho)
# Author: Ilya Mashchenko (l2isbad)

import subprocess
import threading
import xml.etree.ElementTree as et

from bases.collection import find_binary
from bases.FrameworkServices.SimpleService import SimpleService

disabled_by_default = True


NVIDIA_SMI = 'nvidia-smi'

BAD_VALUE = 'N/A'

EMPTY_ROW = ''
EMPTY_ROW_LIMIT = 500
POLLER_BREAK_ROW = '</nvidia_smi_log>'

PCI_BANDWIDTH = 'pci_bandwidth'
FAN_SPEED = 'fan_speed'
GPU_UTIL = 'gpu_utilization'
MEM_UTIL = 'mem_utilization'
ENCODER_UTIL = 'encoder_utilization'
MEM_ALLOCATED = 'mem_allocated'
TEMPERATURE = 'temperature'
CLOCKS = 'clocks'
POWER = 'power'

ORDER = [
    PCI_BANDWIDTH,
    FAN_SPEED,
    GPU_UTIL,
    MEM_UTIL,
    ENCODER_UTIL,
    MEM_ALLOCATED,
    TEMPERATURE,
    CLOCKS,
    POWER,
]


def gpu_charts(gpu):
    fam = gpu.full_name()

    charts = {
        PCI_BANDWIDTH: {
            'options': [None, 'PCI Express Bandwidth Utilization', 'KB/s', fam, 'nvidia_smi.pci_bandwidth', 'area'],
            'lines': [
                ['rx_util', 'rx', 'absolute', 1, 1],
                ['tx_util', 'tx', 'absolute', 1, -1],
            ]
        },
        FAN_SPEED: {
            'options': [None, 'Fan Speed', '%', fam, 'nvidia_smi.fan_speed', 'line'],
            'lines': [
                ['fan_speed', 'speed'],
            ]
        },
        GPU_UTIL: {
            'options': [None, 'GPU Utilization', '%', fam, 'nvidia_smi.gpu_utilization', 'line'],
            'lines': [
                ['gpu_util', 'utilization'],
            ]
        },
        MEM_UTIL: {
            'options': [None, 'Memory Bandwidth Utilization', '%', fam, 'nvidia_smi.mem_utilization', 'line'],
            'lines': [
                ['memory_util', 'utilization'],
            ]
        },
        ENCODER_UTIL: {
            'options': [None, 'Encoder/Decoder Utilization', '%', fam, 'nvidia_smi.encoder_utilization', 'line'],
            'lines': [
                ['encoder_util', 'encoder'],
                ['decoder_util', 'decoder'],
            ]
        },
        MEM_ALLOCATED: {
            'options': [None, 'Memory Allocated', 'MB', fam, 'nvidia_smi.memory_allocated', 'line'],
            'lines': [
                ['fb_memory_usage', 'used'],
            ]
        },
        TEMPERATURE: {
            'options': [None, 'Temperature', 'celsius', fam, 'nvidia_smi.temperature', 'line'],
            'lines': [
                ['gpu_temp', 'temp'],
            ]
        },
        CLOCKS: {
            'options': [None, 'Clock Frequencies', 'MHz', fam, 'nvidia_smi.clocks', 'line'],
            'lines': [
                ['graphics_clock', 'graphics'],
                ['video_clock', 'video'],
                ['sm_clock', 'sm'],
                ['mem_clock', 'mem'],
            ]
        },
        POWER: {
            'options': [None, 'Power Utilization', 'Watts', fam, 'nvidia_smi.power', 'line'],
            'lines': [
                ['power_draw', 'power', 1, 100],
            ]
        },
    }

    idx = gpu.num

    order = ['gpu{0}_{1}'.format(idx, v) for v in ORDER]
    charts = dict(('gpu{0}_{1}'.format(idx, k), v) for k, v in charts.items())

    for chart in charts.values():
        for line in chart['lines']:
            line[0] = 'gpu{0}_{1}'.format(idx, line[0])

    return order, charts


class NvidiaSMI:
    def __init__(self):
        self.command = find_binary(NVIDIA_SMI)
        self.active_proc = None

    def run_once(self):
        proc = subprocess.Popen([self.command, '-x', '-q'], stdout=subprocess.PIPE)
        stdout, _ = proc.communicate()
        return stdout

    def run_loop(self, interval):
        if self.active_proc:
            self.kill()
        proc = subprocess.Popen([self.command, '-x', '-q', '-l', str(interval)], stdout=subprocess.PIPE)
        self.active_proc = proc
        return proc.stdout

    def kill(self):
        if self.active_proc:
            self.active_proc.kill()
            self.active_proc = None


class NvidiaSMIPoller(threading.Thread):
    def __init__(self, poll_interval):
        threading.Thread.__init__(self)
        self.daemon = True

        self.smi = NvidiaSMI()
        self.interval = poll_interval

        self.lock = threading.RLock()
        self.last_data = str()
        self.exit = False
        self.empty_rows = 0
        self.rows = list()

    def has_smi(self):
        return bool(self.smi.command)

    def run_once(self):
        return self.smi.run_once()

    def run(self):
        out = self.smi.run_loop(self.interval)

        for row in out:
            if self.exit or self.empty_rows > EMPTY_ROW_LIMIT:
                break
            self.process_row(row)
        self.smi.kill()

    def process_row(self, row):
        row = row.decode()
        self.empty_rows += (row == EMPTY_ROW)
        self.rows.append(row)

        if POLLER_BREAK_ROW in row:
            self.lock.acquire()
            self.last_data = '\n'.join(self.rows)
            self.lock.release()

            self.rows = list()
            self.empty_rows = 0

    def is_started(self):
        return self.ident is not None

    def shutdown(self):
        self.exit = True

    def data(self):
        self.lock.acquire()
        data = self.last_data
        self.lock.release()
        return data


def handle_attr_error(method):
    def on_call(*args, **kwargs):
        try:
            return method(*args, **kwargs)
        except AttributeError:
            return None
    return on_call


def handle_value_error(method):
    def on_call(*args, **kwargs):
        try:
            return method(*args, **kwargs)
        except ValueError:
            return None
    return on_call


class GPU:
    def __init__(self, num, root):
        self.num = num
        self.root = root

    def id(self):
        return self.root.get('id')

    def name(self):
        return self.root.find('product_name').text

    def full_name(self):
        return 'gpu{0} {1}'.format(self.num, self.name())

    @handle_attr_error
    def rx_util(self):
        return self.root.find('pci').find('rx_util').text.split()[0]

    @handle_attr_error
    def tx_util(self):
        return self.root.find('pci').find('tx_util').text.split()[0]

    @handle_attr_error
    def fan_speed(self):
        return self.root.find('fan_speed').text.split()[0]

    @handle_attr_error
    def gpu_util(self):
        return self.root.find('utilization').find('gpu_util').text.split()[0]

    @handle_attr_error
    def memory_util(self):
        return self.root.find('utilization').find('memory_util').text.split()[0]

    @handle_attr_error
    def encoder_util(self):
        return self.root.find('utilization').find('encoder_util').text.split()[0]

    @handle_attr_error
    def decoder_util(self):
        return self.root.find('utilization').find('decoder_util').text.split()[0]

    @handle_attr_error
    def fb_memory_usage(self):
        return self.root.find('fb_memory_usage').find('used').text.split()[0]

    @handle_attr_error
    def temperature(self):
        return self.root.find('temperature').find('gpu_temp').text.split()[0]

    @handle_attr_error
    def graphics_clock(self):
        return self.root.find('clocks').find('graphics_clock').text.split()[0]

    @handle_attr_error
    def video_clock(self):
        return self.root.find('clocks').find('video_clock').text.split()[0]

    @handle_attr_error
    def sm_clock(self):
        return self.root.find('clocks').find('sm_clock').text.split()[0]

    @handle_attr_error
    def mem_clock(self):
        return self.root.find('clocks').find('mem_clock').text.split()[0]

    @handle_value_error
    @handle_attr_error
    def power_draw(self):
        return float(self.root.find('power_readings').find('power_draw').text.split()[0]) * 100

    def data(self):
        data = {
            'rx_util': self.rx_util(),
            'tx_util': self.tx_util(),
            'fan_speed': self.fan_speed(),
            'gpu_util': self.gpu_util(),
            'memory_util': self.memory_util(),
            'encoder_util': self.encoder_util(),
            'decoder_util': self.decoder_util(),
            'fb_memory_usage': self.fb_memory_usage(),
            'gpu_temp': self.temperature(),
            'graphics_clock': self.graphics_clock(),
            'video_clock': self.video_clock(),
            'sm_clock': self.sm_clock(),
            'mem_clock': self.mem_clock(),
            'power_draw': self.power_draw(),
        }

        return dict(
            ('gpu{0}_{1}'.format(self.num, k), v) for k, v in data.items() if v is not None and v != BAD_VALUE
        )


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        super(Service, self).__init__(configuration=configuration, name=name)
        self.order = list()
        self.definitions = dict()

        poll = int(configuration.get('poll_seconds', 1))
        self.poller = NvidiaSMIPoller(poll)

    def get_data(self):
        if not self.poller.is_alive():
            self.debug('poller is off')
            return None

        last_data = self.poller.data()

        parsed = self.parse_xml(last_data)
        if parsed is None:
            return None

        data = dict()
        for idx, root in enumerate(parsed.findall('gpu')):
            data.update(GPU(idx, root).data())

        return data or None

    def check(self):
        if not self.poller.has_smi():
            self.error("couldn't find '{0}' binary".format(NVIDIA_SMI))
            return False

        raw_data = self.poller.run_once()
        if not raw_data:
            self.error("failed to invoke '{0}' binary".format(NVIDIA_SMI))
            return False

        parsed = self.parse_xml(raw_data)
        if parsed is None:
            return False

        gpus = parsed.findall('gpu')
        if not gpus:
            return False

        self.create_charts(gpus)
        self.poller.start()

        return True

    def parse_xml(self, data):
        try:
            return et.fromstring(data)
        except et.ParseError as error:
            self.error(error)

        return None

    def create_charts(self, gpus):
        for idx, root in enumerate(gpus):
            order, charts = gpu_charts(GPU(idx, root))
            self.order.extend(order)
            self.definitions.update(charts)
