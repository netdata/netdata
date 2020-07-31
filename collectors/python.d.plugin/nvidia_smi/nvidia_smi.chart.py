# -*- coding: utf-8 -*-
# Description: nvidia-smi netdata python.d module
# Original Author: Steven Noonan (tycho)
# Author: Ilya Mashchenko (ilyam8)
# User Memory Stat Author: Guido Scatena (scatenag)

import subprocess
import threading
import xml.etree.ElementTree as et
import os
import pwd

from bases.FrameworkServices.SimpleService import SimpleService
from bases.collection import find_binary

disabled_by_default = True

####
def is_docker():
    #return os.environ.has_key('NETDATA_HOST_PREFIX')
    return false

# try and find the passwd file
__passwd_path = []
if(is_docker()):
    __passwd_path.append('/host/etc/passwd')
if os.environ.has_key('ETC_PASSWD'):
    __passwd_path.append(os.environ['ETC_PASSWD'])
if os.environ.has_key('ETC'):
    __passwd_path.append('%s/passwd' % os.environ['ETC'])
if os.environ.has_key('PYTHONHOME'):
    __passwd_path.append('%s/Etc/passwd' % os.environ['PYTHONHOME'])

passwd_file = None
for __i in __passwd_path:
    try:
        __f = open(__i, 'r')
        __f.close()
        passwd_file = __i
        break
    except:
        pass

# path conversion handlers
def __nullpathconv(path):
    return path.replace(os.altsep, os.sep)

def __unixpathconv(path):
    # two known drive letter variations: "x;" and "$x"
    if path[0] == '$':
        conv = path[1] + ':' + path[2:]
    elif path[1] == ';':
        conv = path[0] + ':' + path[2:]
    else:
        conv = path
    return conv.replace(os.altsep, os.sep)

# decide what field separator we can try to use - Unix standard, with
# the platform's path separator as an option.  No special field conversion
# handler is required when using the platform's path separator as field
# separator, but are required for the home directory and shell fields when
# using the standard Unix (":") field separator.
__field_sep = {':': __unixpathconv}
if os.pathsep:
    if os.pathsep != ':':
        __field_sep[os.pathsep] = __nullpathconv

# helper routine to identify which separator character is in use
def __get_field_sep(record):
    fs = None
    for c in __field_sep.keys():
        # there should be 6 delimiter characters (for 7 fields)
        if record.count(c) == 6:
            fs = c
            break
    if fs:
        return fs
    else:
        raiseKeyError("passwd database fields not delimited")

# class to match the new record field name accessors.
# the resulting object is intended to behave like a read-only tuple,
# with each member also accessible by a field name.
class Passwd:
    def __init__(self, name, passwd, uid, gid, gecos, dir, shell):
        self.__dict__['pw_name'] = name
        self.__dict__['pw_passwd'] = passwd
        self.__dict__['pw_uid'] = uid
        self.__dict__['pw_gid'] = gid
        self.__dict__['pw_gecos'] = gecos
        self.__dict__['pw_dir'] = dir
        self.__dict__['pw_shell'] = shell
        self.__dict__['_record'] = (self.pw_name, self.pw_passwd,
                                    self.pw_uid, self.pw_gid,
                                    self.pw_gecos, self.pw_dir,
                                    self.pw_shell)

    def __len__(self):
        return 7

    def __getitem__(self, key):
        return self._record[key]

    def __setattr__(self, name, value):
        raise AttributeError('attribute read-only: %s' % name)

    def __repr__(self):
        return str(self._record)

    def __cmp__(self, other):
        this = str(self._record)
        if this == other:
            return 0
        elif this < other:
            return -1
        else:
            return 1


# read the whole file, parsing each entry into tuple form
# with dictionaries to speed recall by UID or passwd name
def __read_passwd_file():
    if passwd_file:
        passwd = open(passwd_file, 'r')
    else:
        raise KeyError("no password database")
    uidx = {}
    namx = {}
    sep = None
    while 1:
        entry = passwd.readline().strip()
        if len(entry) > 6:
            if sep is None:
                sep = __get_field_sep(entry)
            fields = entry.split(sep)
            for i in (2, 3):
                fields[i] = int(fields[i])
            for i in (5, 6):
                fields[i] = __field_sep[sep](fields[i])
            record = Passwd(*fields)
            if not uidx.has_key(fields[2]):
                uidx[fields[2]] = record
            if not namx.has_key(fields[0]):
                namx[fields[0]] = record
        elif len(entry) > 0:
            pass                         # skip empty or malformed records
        else:
            break
    passwd.close()
    if len(uidx) == 0:
        raise KeyError
    return (uidx, namx)

# return the passwd database entry by UID
def getpwuid(uid):
    u, n = __read_passwd_file()
    return u[uid]
###

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
MEM_USAGE = 'mem_usage'
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
            'options': [None, 'Number of User on GPU', 'num', fam, 'nvidia_smi.user_num', 'stacked'],
            'lines': ['user_num', 'users']
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
    def fb_memory_used(self):
        return self.root.find('fb_memory_usage').find('used').text.split()[0]

    @handle_attr_error
    def fb_memory_free(self):
        return self.root.find('fb_memory_usage').find('free').text.split()[0]

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

    @handle_attr_error
    def processes(self):
        p_nodes = self.root.find('processes').findall('process_info')
        ps = []
        for p in p_nodes:
            pid = int(p.find('pid').text)
            proc_stat_file = os.stat("/proc/%d" % pid)
            #if(is_docker()):
            #        proc_stat_file = os.path.join(host, 'proc', pid)
            #else:
            #        proc_stat_file = os.stat("/proc/%d" % pid)
            uid = proc_stat_file.st_uid
            username = pwd.getpwuid(uid)[0]
            #if(is_docker()):
            #        username = getpwuid(uid)[0]
            #else:
            #        username = pwd.getpwuid(uid)[0]
            ps.append({
                'pid': p.find('pid').text,
                'process_name': p.find('process_name').text,
                'used_memory': int(p.find('used_memory').text.split()[0]),
                'user_name': username,
            })
        return ps

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
            'gpu_temp': self.temperature(),
            'graphics_clock': self.graphics_clock(),
            'video_clock': self.video_clock(),
            'sm_clock': self.sm_clock(),
            'mem_clock': self.mem_clock(),
            'power_draw': self.power_draw(),
        }
        processes = self.processes() or []
        data.update({'process_mem_{0}'.format(p['pid']): p['used_memory'] for p in processes})
        users = set()
        for p in processes:
            if 'user_mem_{0}'.format(p['user_name']) in data:
                data['user_mem_{0}'.format(p['user_name'])] += p['used_memory']
            else:
                data['user_mem_{0}'.format(p['user_name'])] = p['used_memory']
            users.add(p['user_name'])

        data['user_num'] = len(users)

        return dict(
            ('gpu{0}_{1}'.format(self.num, k), v) for k, v in data.items() if v is not None and v != BAD_VALUE
        )


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        super(Service, self).__init__(configuration=configuration, name=name)
        self.order = list()
        self.definitions = dict()
        self.loop_mode = configuration.get('loop_mode', True)
        poll = int(configuration.get('poll_seconds', 1))
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

        if not last_data:
            return None

        parsed = self.parse_xml(last_data)
        if parsed is None:
            return None

        data = dict()
        for idx, root in enumerate(parsed.findall('gpu')):
            gpu = GPU(idx, root)
            data.update(gpu.data())
            self.update_processes_mem_chart(gpu)
            self.update_processes_user_mem_chart(gpu)
            self.update_user_num_chart(gpu)

        return data or None

    def update_processes_mem_chart(self, gpu):
        ps = gpu.processes()
        if not ps:
            return
        chart = self.charts['gpu{0}_{1}'.format(gpu.num, PROCESSES_MEM)]
        active_dim_ids = []
        for p in ps:
            dim_id = 'gpu{0}_process_mem_{1}'.format(gpu.num, p['pid'])
            active_dim_ids.append(dim_id)
            if dim_id not in chart:
                chart.add_dimension([dim_id, '{0} {1}'.format(p['pid'], p['process_name'])])
        for dim in chart:
            if dim.id not in active_dim_ids:
                chart.del_dimension(dim.id, hide=False)

    def update_processes_user_mem_chart(self, gpu):
        ps = gpu.processes()
        if not ps:
            return
        chart = self.charts['gpu{0}_{1}'.format(gpu.num, USER_MEM)]
        active_dim_ids = []
        for p in ps:
            dim_id = 'gpu{0}_user_mem_{1}'.format(gpu.num, p['user_name'])
            active_dim_ids.append(dim_id)
            if dim_id not in chart:
                chart.add_dimension([dim_id, '{0}'.format(p['user_name'])])
        for dim in chart:
            if dim.id not in active_dim_ids:
                chart.del_dimension(dim.id, hide=False)

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

        return True

    def parse_xml(self, data):
        try:
            return et.fromstring(data)
        except et.ParseError as error:
            self.error('xml parse failed: "{0}", error: {1}'.format(data, error))

        return None

    def create_charts(self, gpus):
        for idx, root in enumerate(gpus):
            order, charts = gpu_charts(GPU(idx, root))
            self.order.extend(order)
            self.definitions.update(charts)
