# -*- coding: utf-8 -*-
# Description: nvidia-smi netdata python.d module
# Original Author: Steven Noonan (tycho)
# Author: Ilya Mashchenko (ilyam8)
# User Memory Stat Author: Guido Scatena (scatenag)

import subprocess
import threading
import os
import pwd

import xml.etree.ElementTree as et

from bases.FrameworkServices.SimpleService import SimpleService
from bases.collection import find_binary

disabled_by_default = True

NVIDIA_FORMAT_MODE = 'csv'
NVIDIA_FORMAT_MODE_CSV_QUERY_GPU = '--query-gpu=gpu_name,gpu_bus_id,utilization.gpu,utilization.memory,fan.speed,temperature.gpu,memory.free,memory.used'
NVIDIA_SMI = 'nvidia-smi'
DEFAULT_LOOP_MODE = False

EMPTY_ROW = ''
EMPTY_ROW_LIMIT = 500
POLLER_BREAK_ROW = '</nvidia_smi_log>'
POLLER_BREAK_ROW_CSV = 'endcsv'

PCI_BANDWIDTH = 'pci_bandwidth'
FAN_SPEED = 'fan_speed'
GPU_UTIL = 'gpu_utilization'
MEM_UTIL = 'mem_utilization'
ENCODER_UTIL = 'encoder_utilization'
MEM_USAGE = 'mem_usage'
BAR_USAGE = 'bar1_mem_usage'
TEMPERATURE = 'temperature'
CLOCKS = 'clocks'
POWER = 'power'
PROCESSES_MEM = 'processes_mem'
USER_MEM = 'user_mem'
USER_NUM = 'user_num'

ORDER = [
    PCI_BANDWIDTH,
    FAN_SPEED,
    GPU_UTIL,
    MEM_UTIL,
    ENCODER_UTIL,
    MEM_USAGE,
    BAR_USAGE,
    TEMPERATURE,
    CLOCKS,
    POWER,
    PROCESSES_MEM,
    USER_MEM,
    USER_NUM,
]


def gpu_charts(gpu):
    fam = gpu.full_name()

    charts = {
        PCI_BANDWIDTH: {
            'options': [None, 'PCI Express Bandwidth Utilization', 'KiB/s', fam, 'nvidia_smi.pci_bandwidth', 'area'],
            'lines': [
                ['rx_util', 'rx', 'absolute', 1, 1],
                ['tx_util', 'tx', 'absolute', 1, -1],
            ]
        },
        FAN_SPEED: {
            'options': [None, 'Fan Speed', 'percentage', fam, 'nvidia_smi.fan_speed', 'line'],
            'lines': [
                ['fan_speed', 'speed'],
            ]
        },
        GPU_UTIL: {
            'options': [None, 'GPU Utilization', 'percentage', fam, 'nvidia_smi.gpu_utilization', 'line'],
            'lines': [
                ['gpu_util', 'utilization'],
            ]
        },
        MEM_UTIL: {
            'options': [None, 'Memory Bandwidth Utilization', 'percentage', fam, 'nvidia_smi.mem_utilization', 'line'],
            'lines': [
                ['memory_util', 'utilization'],
            ]
        },
        ENCODER_UTIL: {
            'options': [None, 'Encoder/Decoder Utilization', 'percentage', fam, 'nvidia_smi.encoder_utilization',
                        'line'],
            'lines': [
                ['encoder_util', 'encoder'],
                ['decoder_util', 'decoder'],
            ]
        },
        MEM_USAGE: {
            'options': [None, 'Memory Usage', 'MiB', fam, 'nvidia_smi.memory_allocated', 'stacked'],
            'lines': [
                ['fb_memory_free', 'free'],
                ['fb_memory_used', 'used'],
            ]
        },
        BAR_USAGE: {
            'options': [None, 'Bar1 Memory Usage', 'MiB', fam, 'nvidia_smi.bar1_memory_usage', 'stacked'],
            'lines': [
                ['bar1_memory_free', 'free'],
                ['bar1_memory_used', 'used'],
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
                ['power_draw', 'power', 'absolute', 1, 100],
            ]
        },
        PROCESSES_MEM: {
            'options': [None, 'Memory Used by Each Process', 'MiB', fam, 'nvidia_smi.processes_mem', 'stacked'],
            'lines': []
        },
        USER_MEM: {
            'options': [None, 'Memory Used by Each User', 'MiB', fam, 'nvidia_smi.user_mem', 'stacked'],
            'lines': []
        },
        USER_NUM: {
            'options': [None, 'Number of User on GPU', 'num', fam, 'nvidia_smi.user_num', 'line'],
            'lines': [
                ['user_num', 'users'],
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
        self.format_mode = NVIDIA_FORMAT_MODE

    def run_once(self):
        if self.format_mode == 'csv':
            proc = subprocess.Popen([self.command, NVIDIA_FORMAT_MODE_CSV_QUERY_GPU, '--format=csv,nounits'], stdout=subprocess.PIPE)
        else:
            proc = subprocess.Popen([self.command, '-x', '-q'], stdout=subprocess.PIPE)
        stdout, _ = proc.communicate()
        return stdout

    def run_loop(self, interval):
        if self.active_proc:
            self.kill()
        if self.format_mode == 'csv':
            proc = subprocess.Popen([self.command, NVIDIA_FORMAT_MODE_CSV_QUERY_GPU, '--format=csv,nounits', '-l', str(interval), '&& echo "endcsv"'], stdout=subprocess.PIPE)
        else:
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
        self.format_mode = NVIDIA_FORMAT_MODE

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

        if (POLLER_BREAK_ROW in row) or (POLLER_BREAK_ROW_CSV in row):
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


HOST_PREFIX = os.getenv('NETDATA_HOST_PREFIX')
ETC_PASSWD_PATH = '/etc/passwd'
PROC_PATH = '/proc'

IS_INSIDE_DOCKER = False

if HOST_PREFIX:
    ETC_PASSWD_PATH = os.path.join(HOST_PREFIX, ETC_PASSWD_PATH[1:])
    PROC_PATH = os.path.join(HOST_PREFIX, PROC_PATH[1:])
    IS_INSIDE_DOCKER = True


def read_passwd_file():
    data = dict()
    with open(ETC_PASSWD_PATH, 'r') as f:
        for line in f:
            line = line.strip()
            if line.startswith("#"):
                continue
            fields = line.split(":")
            # name, passwd, uid, gid, comment, home_dir, shell
            if len(fields) != 7:
                continue
            # uid, guid
            fields[2], fields[3] = int(fields[2]), int(fields[3])
            data[fields[2]] = fields
    return data


def read_passwd_file_safe():
    try:
        if IS_INSIDE_DOCKER:
            return read_passwd_file()
        return dict((k[2], k) for k in pwd.getpwall())
    except (OSError, IOError):
        return dict()


def get_username_by_pid_safe(pid, passwd_file):
    path = os.path.join(PROC_PATH, pid)
    try:
        uid = os.stat(path).st_uid
    except (OSError, IOError):
        return ''
    try:
        if IS_INSIDE_DOCKER:
            return passwd_file[uid][0]
        return pwd.getpwuid(uid)[0]
    except KeyError:
        return str(uid)


class GPU:
    def __init__(self, num, root, format_mode, exclude_zero_memory_users=False):
        self.num = num
        self.root = root
        self.format_mode = format_mode
        self.exclude_zero_memory_users = exclude_zero_memory_users

    def id(self):
        if self.format_mode == 'csv':
            return self.root.get('id')
        else:
            return self.root.get('id')

    def name(self):
        if self.format_mode == 'csv':
            return self.root.get('product_name')
        else:
            return self.root.find('product_name').text

    def full_name(self):
        return 'gpu{0} {1}'.format(self.num, self.name())

    @handle_attr_error
    def rx_util(self):
        if self.format_mode == 'csv':
            return '0'
        else:
            return self.root.find('pci').find('rx_util').text.split()[0]

    @handle_attr_error
    def tx_util(self):
        if self.format_mode == 'csv':
            return '0'
        else:
            return self.root.find('pci').find('tx_util').text.split()[0]

    @handle_attr_error
    def fan_speed(self):
        if self.format_mode == 'csv':
            return self.root.get('fan_speed')
        else:
            return self.root.find('fan_speed').text.split()[0]

    @handle_attr_error
    def gpu_util(self):
        if self.format_mode == 'csv':
            return self.root.get('gpu_util')
        else:
            return self.root.find('utilization').find('gpu_util').text.split()[0]

    @handle_attr_error
    def memory_util(self):
        if self.format_mode == 'csv':
            return self.root.get('memory_util')
        else:
            return self.root.find('utilization').find('memory_util').text.split()[0]

    @handle_attr_error
    def encoder_util(self):
        if self.format_mode == 'csv':
            return '0'
        else:
            return self.root.find('utilization').find('encoder_util').text.split()[0]

    @handle_attr_error
    def decoder_util(self):
        if self.format_mode == 'csv':
            return '0'
        else:
            return self.root.find('utilization').find('decoder_util').text.split()[0]

    @handle_attr_error
    def fb_memory_used(self):
        if self.format_mode == 'csv':
            return self.root.get('memory_used')
        else:
            return self.root.find('fb_memory_usage').find('used').text.split()[0]

    @handle_attr_error
    def fb_memory_free(self):
        if self.format_mode == 'csv':
            return self.root.get('memory_free')
        else:
            return self.root.find('fb_memory_usage').find('free').text.split()[0]

    @handle_attr_error
    def bar1_memory_used(self):
        if self.format_mode == 'csv':
            return '0'
        else:
            return self.root.find('bar1_memory_usage').find('used').text.split()[0]

    @handle_attr_error
    def bar1_memory_free(self):
        if self.format_mode == 'csv':
            return '0'
        else:
            return self.root.find('bar1_memory_usage').find('free').text.split()[0]

    @handle_attr_error
    def temperature(self):
        if self.format_mode == 'csv':
            return self.root.get('gpu_temp')
        else:
            return self.root.find('temperature').find('gpu_temp').text.split()[0]

    @handle_attr_error
    def graphics_clock(self):
        if self.format_mode == 'csv':
            return '0'
        else:
            return self.root.find('clocks').find('graphics_clock').text.split()[0]

    @handle_attr_error
    def video_clock(self):
        if self.format_mode == 'csv':
            return '0'
        else:
            return self.root.find('clocks').find('video_clock').text.split()[0]

    @handle_attr_error
    def sm_clock(self):
        if self.format_mode == 'csv':
            return '0'
        else:
            return self.root.find('clocks').find('sm_clock').text.split()[0]

    @handle_attr_error
    def mem_clock(self):
        if self.format_mode == 'csv':
            return '0'
        else:
            return self.root.find('clocks').find('mem_clock').text.split()[0]

    @handle_value_error
    @handle_attr_error
    def power_draw(self):
        if self.format_mode == 'csv':
            return '0'
        else:
            return float(self.root.find('power_readings').find('power_draw').text.split()[0]) * 100

    @handle_attr_error
    def processes(self):
        processes_info = self.root.find('processes').findall('process_info')
        if not processes_info:
            return list()

        passwd_file = read_passwd_file_safe()
        processes = list()

        for info in processes_info:
            pid = info.find('pid').text
            processes.append({
                'pid': int(pid),
                'process_name': info.find('process_name').text,
                'used_memory': int(info.find('used_memory').text.split()[0]),
                'username': get_username_by_pid_safe(pid, passwd_file),
            })
        return processes

    def data(self):
        data = {
            'rx_util': self.rx_util(),
            'tx_util': self.tx_util(),
            'fan_speed': self.fan_speed(),
            'gpu_util': self.gpu_util(),
            'memory_util': self.memory_util(),
            'encoder_util': self.encoder_util(),
            'decoder_util': self.decoder_util(),
            'fb_memory_used': self.fb_memory_used(),
            'fb_memory_free': self.fb_memory_free(),
            'bar1_memory_used': self.bar1_memory_used(),
            'bar1_memory_free': self.bar1_memory_free(),
            'gpu_temp': self.temperature(),
            'graphics_clock': self.graphics_clock(),
            'video_clock': self.video_clock(),
            'sm_clock': self.sm_clock(),
            'mem_clock': self.mem_clock(),
            'power_draw': self.power_draw(),
        }
        #processes = self.processes() or []
        #processes = []
        #users = set()
        # for p in processes:
        #     data['process_mem_{0}'.format(p['pid'])] = p['used_memory']
        #     if p['username']:
        #         if self.exclude_zero_memory_users and p['used_memory'] == 0:
        #             continue
        #         users.add(p['username'])
        #         key = 'user_mem_{0}'.format(p['username'])
        #         if key in data:
        #             data[key] += p['used_memory']
        #         else:
        #             data[key] = p['used_memory']
        # data['user_num'] = len(users)

        return dict(('gpu{0}_{1}'.format(self.num, k), v) for k, v in data.items())


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        super(Service, self).__init__(configuration=configuration, name=name)
        self.order = list()
        self.definitions = dict()
        self.loop_mode = configuration.get('loop_mode', DEFAULT_LOOP_MODE)
        self.format_mode = NVIDIA_FORMAT_MODE
        poll = int(configuration.get('poll_seconds', 1))
        self.exclude_zero_memory_users = configuration.get('exclude_zero_memory_users', False)
        self.poller = NvidiaSMIPoller(poll)

    def get_data_loop_mode(self):
        if not self.poller.is_started():
            self.poller.start()

        if not self.poller.is_alive():
            self.debug('poller is off')
            return None

        return self.poller.data()

    def get_data_normal_mode(self):
        return self.poller.run_once()

    def get_data(self):
        if self.loop_mode:
            last_data = self.get_data_loop_mode()
        else:
            last_data = self.get_data_normal_mode()

        #self.debug(last_data)

        if not last_data:
            return None

        parsed = self.parse_csv(last_data) if self.format_mode == 'csv' else self.parse_xml(last_data).findall('gpu')

        data = dict()
        for idx, root in enumerate(parsed):
            if self.format_mode == 'csv':
                gpu = GPU(idx, parsed[idx], self.format_mode, self.exclude_zero_memory_users)
            else:
                gpu = GPU(idx, root, self.format_mode, self.exclude_zero_memory_users)
            gpu_data = gpu.data()
            #self.debug(gpu_data)
            gpu_data = dict((k, v) for k, v in gpu_data.items() if is_gpu_data_value_valid(v))
            data.update(gpu_data)
            #self.update_processes_mem_chart(gpu)
            #self.update_processes_user_mem_chart(gpu)

        return data

    # def update_processes_mem_chart(self, gpu):
    #     ps = gpu.processes()
    #     if not ps:
    #         return
    #     chart = self.charts['gpu{0}_{1}'.format(gpu.num, PROCESSES_MEM)]
    #     active_dim_ids = []
    #     for p in ps:
    #         dim_id = 'gpu{0}_process_mem_{1}'.format(gpu.num, p['pid'])
    #         active_dim_ids.append(dim_id)
    #         if dim_id not in chart:
    #             chart.add_dimension([dim_id, '{0} {1}'.format(p['pid'], p['process_name'])])
    #     for dim in chart:
    #         if dim.id not in active_dim_ids:
    #             chart.del_dimension(dim.id, hide=False)

    # def update_processes_user_mem_chart(self, gpu):
    #     ps = gpu.processes()
    #     if not ps:
    #         return
    #     chart = self.charts['gpu{0}_{1}'.format(gpu.num, USER_MEM)]
    #     active_dim_ids = []
    #     for p in ps:
    #         if not p.get('username'):
    #             continue
    #         dim_id = 'gpu{0}_user_mem_{1}'.format(gpu.num, p['username'])
    #         active_dim_ids.append(dim_id)
    #         if dim_id not in chart:
    #             chart.add_dimension([dim_id, '{0}'.format(p['username'])])

    #     for dim in chart:
    #         if dim.id not in active_dim_ids:
    #             chart.del_dimension(dim.id, hide=False)

    def check(self):
        if not self.poller.has_smi():
            self.error("couldn't find '{0}' binary".format(NVIDIA_SMI))
            return False

        raw_data = self.poller.run_once()
        if not raw_data:
            self.error("failed to invoke '{0}' binary".format(NVIDIA_SMI))
            return False

        if self.format_mode == 'xml':
            parsed = self.parse_xml(raw_data)
            if parsed is None:
                return False
        
            gpus = parsed.findall('gpu')
            if not gpus:
                return False
                
        if self.format_mode == 'csv':
            gpus = self.parse_csv(raw_data)
            if gpus is None:
                return False
            
        self.create_charts(gpus)

        return True

    def parse_xml(self, data):
        try:
            return et.fromstring(data)
        except et.ParseError as error:
            self.error('xml parse failed: "{0}", error: {1}'.format(data, error))

        return None

    def parse_csv(self, data):
        try:
            data = data.decode().splitlines()
            labels = [l.split()[0].strip() for l in data[0].split(',')]
            labels_map = {
                'name': 'product_name',
                'pci.bus_id': 'id',
                'utilization.gpu': 'gpu_util',
                'utilization.memory': 'memory_util',
                'fan.speed': 'fan_speed',
                'temperature.gpu': 'gpu_temp',
                'memory.free': 'memory_free',
                'memory.used': 'memory_used'
            }
            labels = [labels_map.get(label,label) for label in labels]
            values = [k.strip().replace('[N/A]','0') for row in data[1:] for k in row.split(',')]
            n_gpus = len(data)-1
            n_cols = len(labels)
            data = dict()
            for gpu_n in range(n_gpus):
                data[gpu_n] = dict(zip(labels, values[(n_cols*gpu_n):n_cols*(gpu_n+1)]))
            return data
        except Exception as error:
            self.error('csv parse failed: "{0}", error: {1}'.format(data, error))

        return None

    def create_charts(self, gpus):
        for idx, root in enumerate(gpus):
            if self.format_mode == 'csv':
                order, charts = gpu_charts(GPU(idx, gpus[idx], self.format_mode))
            else:
                order, charts = gpu_charts(GPU(idx, root, self.format_mode))
            self.order.extend(order)
            self.definitions.update(charts)


def is_gpu_data_value_valid(value):
    try:
        int(value)
    except (TypeError, ValueError):
        return False
    return True