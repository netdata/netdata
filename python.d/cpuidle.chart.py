# -*- coding: utf-8 -*-
# Description: cpuidle netdata python.d module
# Author: Steven Noonan (tycho)

import glob
import os
import platform
import time
from base import SimpleService

import ctypes
syscall = ctypes.CDLL('libc.so.6').syscall

# default module values (can be overridden per job in `config`)
# update_every = 2

class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        prefix = os.getenv('NETDATA_HOST_PREFIX', "")
        if prefix.endswith('/'):
            prefix = prefix[:-1]
        self.sys_dir = prefix + "/sys/devices/system/cpu"
        self.schedstat_path = prefix + "/proc/schedstat"
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.order = []
        self.definitions = {}
        self._orig_name = ""
        self.assignment = {}
        self.last_schedstat = None

    def __gettid(self):
        # This is horrendous. We need the *thread id* (not the *process id*),
        # but there's no Python standard library way of doing that. If you need
        # to enable this module on a non-x86 machine type, you'll have to find
        # the Linux syscall number for gettid() and add it to the dictionary
        # below.
        syscalls = {
            'i386':    224,
            'x86_64':  186,
        }
        if platform.machine() not in syscalls:
            return None
        tid = syscall(syscalls[platform.machine()])
        return tid

    def __wake_cpus(self, cpus):
        # Requires Python 3.3+. This will "tickle" each CPU to force it to
        # update its idle counters.
        if hasattr(os, 'sched_setaffinity'):
            pid = self.__gettid()
            save_affinity = os.sched_getaffinity(pid)
            for idx in cpus:
                os.sched_setaffinity(pid, [idx])
                os.sched_getaffinity(pid)
            os.sched_setaffinity(pid, save_affinity)

    def __read_schedstat(self):
        cpus = {}
        for line in open(self.schedstat_path, 'r'):
            if not line.startswith('cpu'):
                continue
            line = line.rstrip().split()
            cpu = line[0]
            active_time = line[7]
            cpus[cpu] = int(active_time) // 1000
        return cpus

    def _get_data(self):
        results = {}

        # Use the kernel scheduler stats to determine how much time was spent
        # in C0 (active).
        schedstat = self.__read_schedstat()

        # Determine if any of the CPUs are idle. If they are, then we need to
        # tickle them in order to update their C-state residency statistics.
        if self.last_schedstat is None:
            needs_tickle = list(self.assignment.keys())
        else:
            needs_tickle = []
            for cpu, active_time in self.last_schedstat.items():
                delta = schedstat[cpu] - active_time
                if delta < 1:
                    needs_tickle.append(cpu)

        if needs_tickle:
            # This line is critical for the stats to update. If we don't "tickle"
            # idle CPUs, then the counters for those CPUs stop counting.
            self.__wake_cpus([int(cpu[3:]) for cpu in needs_tickle])

            # Re-read schedstat now that we've tickled any idlers.
            schedstat = self.__read_schedstat()

        self.last_schedstat = schedstat

        for cpu, metrics in self.assignment.items():
            update_time = schedstat[cpu]
            results[cpu + '_active_time'] = update_time

            for metric, path in metrics.items():
                residency = int(open(path, 'r').read())
                results[metric] = residency

        return results

    def check(self):
        if self.__gettid() is None:
            self.error("Cannot get thread ID. Stats would be completely broken.")
            return False

        self._orig_name = self.chart_name

        for path in sorted(glob.glob(self.sys_dir + '/cpu*/cpuidle/state*/name')):
            # ['', 'sys', 'devices', 'system', 'cpu', 'cpu0', 'cpuidle', 'state3', 'name']
            path_elem = path.split('/')
            cpu = path_elem[-4]
            state = path_elem[-2]
            statename = open(path, 'rt').read().rstrip()

            orderid = '%s_cpuidle' % (cpu,)
            if orderid not in self.definitions:
                self.order.append(orderid)
                active_name = '%s_active_time' % (cpu,)
                self.definitions[orderid] = {
                    'options': [None, 'C-state residency', 'time%', 'cpuidle', None, 'stacked'],
                    'lines': [
                        [active_name, 'C0 (active)', 'percentage-of-incremental-row', 1, 1],
                    ],
                }
                self.assignment[cpu] = {}

            defid = '%s_%s_time' % (orderid, state)

            self.definitions[orderid]['lines'].append(
                [defid, statename, 'percentage-of-incremental-row', 1, 1]
            )

            self.assignment[cpu][defid] = '/'.join(path_elem[:-1] + ['time'])

        # Sort order by kernel-specified CPU index
        self.order.sort(key=lambda x: int(x.split('_')[0][3:]))

        if len(self.definitions) == 0:
            self.error("couldn't find cstate stats")
            return False

        return True

    def create(self):
        self.chart_name = "cpu"
        status = SimpleService.create(self)
        self.chart_name = self._orig_name
        return status

    def update(self, interval):
        self.chart_name = "cpu"
        status = SimpleService.update(self, interval=interval)
        self.chart_name = self._orig_name
        return status

# vim: set ts=4 sts=4 sw=4 et:
