# -*- coding: utf-8 -*-
# Description: nvidia-smi netdata python.d module
# Author: Steven Noonan (tycho)

import copy
import subprocess
import threading
import xml.etree.ElementTree as et
import time
from base import SimpleService
from bases.collection import find_binary

CHART_TEMPLATES = {
    'pci_bandwidth': {
        'options': [None, 'PCI express bandwidth utilization', 'KB/s', 'nvidia_smi', 'nvidia_smi', 'area'],
        '_metrics': { 'pci': [ 'rx_util', 'tx_util' ] },
        'lines': []
    },
    'fan_speed': {
        'options': [None, 'Fan speed', 'percentage', 'nvidia_smi', 'nvidia_smi', 'line'],
        '_metrics': { None: [ 'fan_speed' ] },
        'lines': []
    },
    'gpu_utilization': {
        'options': [None, 'Compute utilization', 'percentage', 'nvidia_smi', 'nvidia_smi', 'line'],
        '_metrics': { 'utilization': [ 'gpu_util' ] },
        '_transform': lambda metricname: metricname.split('_')[0],
        'lines': []
    },
    'membw_utilization': {
        'options': [None, 'Memory bandwidth utilization', 'percentage', 'nvidia_smi', 'nvidia_smi', 'line'],
        '_metrics': { 'utilization': [ 'memory_util' ] },
        '_transform': lambda metricname: metricname.split('_')[0],
        'lines': []
    },
    'encoder_utilization': {
        'options': [None, 'Encoder/decoder utilization', 'percentage', 'nvidia_smi', 'nvidia_smi', 'line'],
        '_metrics': { 'utilization': [ 'encoder_util', 'decoder_util' ] },
        '_transform': lambda metricname: metricname.split('_')[0],
        'lines': []
    },
    'mem_allocated': {
        'options': [None, 'Memory utilization', 'MB', 'nvidia_smi', 'nvidia_smi', 'line'],
        '_metrics': { 'fb_memory_usage': [ 'used' ] },
        'lines': []
    },
    'temperature': {
        'options': [None, 'Temperature', 'Celsius', 'nvidia_smi', 'nvidia_smi', 'line'],
        '_metrics': { 'temperature': [ 'gpu_temp' ] },
        '_transform': lambda metricname: metricname.split('_')[1],
        'lines': []
    },
    'clocks': {
        'options': [None, 'Clock frequencies', 'MHz', 'nvidia_smi', 'nvidia_smi', 'line'],
        '_metrics': { 'clocks': [ 'graphics_clock', 'sm_clock', 'mem_clock', 'video_clock' ] },
        '_transform': lambda metricname: metricname.split('_')[0],
        'lines': []
    },
    'power': {
        'options': [None, 'Power utilization', 'Watts', 'nvidia_smi', 'nvidia_smi', 'line'],
        '_metrics': { 'power_readings': [ 'power_draw' ] },
        '_transform': lambda metricname: metricname.split('_')[0],
        '_divisor': 10,
        'lines': []
    },
}


class Poller(threading.Thread):
    def __init__(self, command, break_condition):
        threading.Thread.__init__(self)
        self.command = command
        self.break_condition = break_condition
        self.lock = threading.RLock()
        self.last_time = 0
        self.last_data = ''

    def _read(self):
        lines = []
        while True:
            line = self.process.stdout.readline().decode()
            lines.append(line)
            if self.break_condition(line):
                self.lock.acquire()
                self.last_data = '\n'.join(lines)
                self.last_time = time.time()
                self.lock.release()
                return

    def run(self):
        self.lock.acquire()
        try:
            self.process = subprocess.Popen(self.command, stdout=subprocess.PIPE)
        except (OSError, FileNotFoundError):
            return

        # Initial read
        self._read()
        self.lock.release()

        while True:
            self._read()


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.order = []
        self.definitions = {}
        self.fake_name = "gpu"
        self.nvidia_smi = find_binary('nvidia-smi')
        self.assignment = {}
        self.poller = Poller([self.nvidia_smi, '-x', '-q', '-l', '1'], lambda x: '</nvidia_smi_log>' in x)

    def _invoke_nvidia_smi(self):
        if self.poller.ident is None:
            proc = subprocess.Popen([self.nvidia_smi, '-x', '-q'], stdout=subprocess.PIPE)
            stdout, _ = proc.communicate()
        else:
            # TODO: Warn if self.poller.last_time is old.
            self.poller.lock.acquire()
            stdout = self.poller.last_data
            self.poller.lock.release()
        try:
            smi = et.fromstring(stdout)
        except et.ParseError:
            return None
        return smi

    def _get_data(self):
        data = {}

        smi = self._invoke_nvidia_smi()

        if smi is None:
            return data

        for gpu in smi.findall('gpu'):
            gpuid = self.assignment[gpu.get('id')]['gpuid']

            for template in CHART_TEMPLATES.values():
                for rootname, metrics in template['_metrics'].items():
                    if rootname:
                        root = gpu.find(rootname)
                    else:
                        root = gpu

                    for metric in metrics:
                        metric_name = gpuid + "_" + metric
                        elem = root.find(metric)

                        if elem is None:
                            continue

                        try:
                            val = float(elem.text.split()[0])
                        except ValueError:
                            continue

                        val *= float(template.get('_divisor', 1))

                        data[metric_name] = int(val)

        return data

    def check(self):
        if not self.nvidia_smi:
            self.error("Could not find 'nvidia-smi' binary. Do you have the proprietary NVIDIA driver and tools installed?")
            return False

        smi = self._invoke_nvidia_smi()
        if smi is None:
            self.error("Failed to invoke 'nvidia-smi'. Do you have the proprietary NVIDIA driver and tools installed?")
            return False

        gpuidx = 0
        for gpu in smi.findall('gpu'):
            gpuid = gpu.get('id')
            name = gpuid.replace(':', '_')
            name = name.replace('.', '_')
            self.assignment[gpuid] = {
                'gpuid': 'gpu%d' % (gpuidx,),
            }
            gpuidx += 1

        if len(self.assignment) == 0:
            self.error("Could not find any NVIDIA GPUs in nvidia-smi XML output")
            return False

        data = self._get_data()

        order = []

        for chart, template in CHART_TEMPLATES.items():
            for name in sorted(self.assignment):
                assignment = self.assignment[name]
                gpuid = assignment['gpuid']

                num_metrics = sum([len(metricnames) for rootname, metricnames in template['_metrics'].items()])
                should_be_instanced = num_metrics > 1

                if should_be_instanced:
                    chartname = chart + '_' + gpuid
                    chartdef = copy.deepcopy(template)
                else:
                    chartname = chart
                    chartdef = template

                for rootname, metrics in chartdef['_metrics'].items():
                    for metric in list(metrics):
                        metricname = gpuid + '_' + metric

                        if metricname not in data:
                            # This metric isn't present or doesn't parse
                            # properly. Don't create a chart for it if we can
                            # help it.
                            chartdef['_metrics'][rootname].remove(metric)
                            continue

                        direction = 1
                        if rootname == 'pci' and metric == 'tx_util':
                            direction = -1

                        transform = template.get('_transform', lambda metricname: metricname)

                        if should_be_instanced:
                            nickname = transform(metric)
                        else:
                            nickname = gpuid + '_' + transform(metric)

                        divisor = template.get('_divisor', 1)
                        chartdef['lines'].append([metricname, nickname, 'absolute', direction, divisor])

                metrics_available = False
                for rootname, metrics in chartdef['_metrics'].items():
                    if len(metrics):
                        metrics_available = True
                        break

                if metrics_available and chartname not in self.definitions:
                    if should_be_instanced:
                        priority_offset = 1
                    else:
                        priority_offset = 0
                    self.definitions[chartname] = chartdef
                    order.append((priority_offset, gpuid, chartname))


        self._get_data()

        self.order = [name for offset, topology, name in sorted(order)]

        self.poller.start()

        return True
