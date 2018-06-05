# -*- coding: utf-8 -*-
# Description: BOINC netdata python.d module
# Author: Austin S. Hemmelgarn (Ferroin)
# SPDX-License-Identifier: GPL-3.0+

import platform
import socket

from bases.FrameworkServices.SimpleService import SimpleService

from third_party import boinc_client


ORDER = ['tasks', 'states', 'sched_states', 'process_states']

CHARTS = {
    'tasks': {
        'options': [None, 'Overall Tasks', 'tasks', 'boinc', 'boinc.tasks', 'line'],
        'lines': [
            ['total', 'Total', 'absolute', 1, 1],
            ['active', 'Active', 'absolute', 1, 1]
        ]
    },
    'states': {
        'options': [None, 'Tasks per State', 'tasks', 'boinc', 'boinc.states', 'line'],
        'lines': [
            ['new', 'New', 'absolute', 1, 1],
            ['downloading', 'Downloading', 'absolute', 1, 1],
            ['downloaded', 'Ready to Run', 'absolute', 1, 1],
            ['comperror', 'Compute Errors', 'absolute', 1, 1],
            ['uploading', 'Uploading', 'absolute', 1, 1],
            ['uploaded', 'Uploaded', 'absolute', 1, 1],
            ['aborted', 'Aborted', 'absolute', 1, 1],
            ['upload_failed', 'Failed Uploads', 'absolute', 1, 1]
        ]
    },
    'sched_states': {
        'options': [None, 'Tasks per Scheduler State', 'tasks', 'boinc', 'boinc.sched', 'line'],
        'lines': [
            ['uninit_sched', 'Uninitialized', 'absolute', 1, 1],
            ['preempted', 'Preempted', 'absolute', 1, 1],
            ['scheduled', 'Scheduled', 'absolute', 1, 1]
        ]
    },
    'process_states': {
        'options': [None, 'Tasks per Process State', 'tasks', 'boinc', 'boinc.process', 'line'],
        'lines': [
            ['uninit_proc', 'Uninitialized', 'absolute', 1, 1],
            ['executing', 'Executing', 'absolute', 1, 1],
            ['suspended', 'Suspended', 'absolute', 1, 1],
            ['aborting', 'Aborted', 'absolute', 1, 1],
            ['quit', 'Quit', 'absolute', 1, 1],
            ['copy_pending', 'Copy Pending', 'absolute', 1, 1]
        ]
    }
}

# A simple template used for pre-loading the return dictionary to make
# the _get_data() method simpler.
_DATA_TEMPLATE = {
    'total': 0,
    'active': 0,
    'new': 0,
    'downloading': 0,
    'downloaded': 0,
    'comperror': 0,
    'uploading': 0,
    'uploaded': 0,
    'aborted': 0,
    'upload_failed': 0,
    'uninit_sched': 0,
    'preempted': 0,
    'scheduled': 0,
    'uninit_proc': 0,
    'executing': 0,
    'suspended': 0,
    'aborting': 0,
    'quit': 0,
    'copy_pending': 0
}

# Map task states to dimensions
_TASK_MAP = {
    boinc_client.ResultState.NEW: 'new',
    boinc_client.ResultState.FILES_DOWNLOADING: 'downloading',
    boinc_client.ResultState.FILES_DOWNLOADED: 'downloaded',
    boinc_client.ResultState.COMPUTE_ERROR: 'comperror',
    boinc_client.ResultState.FILES_UPLOADING: 'uploading',
    boinc_client.ResultState.FILES_UPLOADED: 'uploaded',
    boinc_client.ResultState.ABORTED: 'aborted',
    boinc_client.ResultState.UPLOAD_FAILED: 'upload_failed'
}

# Map scheduler states to dimensions
_SCHED_MAP = {
    boinc_client.CpuSched.UNINITIALIZED: 'uninit_sched',
    boinc_client.CpuSched.PREEMPTED: 'preempted',
    boinc_client.CpuSched.SCHEDULED: 'scheduled',
}

# Maps process states to dimensions
_PROC_MAP = {
    boinc_client.Process.UNINITIALIZED: 'uninit_proc',
    boinc_client.Process.EXECUTING: 'executing',
    boinc_client.Process.SUSPENDED: 'suspended',
    boinc_client.Process.ABORT_PENDING: 'aborted',
    boinc_client.Process.QUIT_PENDING: 'quit',
    boinc_client.Process.COPY_PENDING: 'copy_pending'
}


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.host = self.configuration.get('host', 'localhost')
        self.port = self.configuration.get('port', 0)
        self.password = self.configuration.get('password', '')
        self.client = boinc_client.BoincClient(host=self.host, port=self.port, passwd=self.password)
        self.alive = False

    def check(self):
        if platform.system() != 'Linux':
            self.error('Only supported on Linux.')
            return False
        self.connect()
        self.alive = self.client.connected and self.client.authorized:
        return self.alive

    def connect(self):
        self.client.connect()

    def reconnect(self):
        try:
            self.client.disconnect()
        except socket.error:
            pass
        self.client.connect()
        self.alive = self.client.connected and self.client.authorized:
        return self.alive

    def is_alive(self):
        if (not self.alive) or \
           self.client.rpc.sock.getsockopt(socket.IPPROTO_TCP, socket.TCP_INFO, 0) != 1:
            return self.reconnect()
        return True

    def _get_data(self):
        if not self.is_alive():
            return None
        data = dict(_DATA_TEMPLATE)
        results = []
        try:
            results = self.client.get_tasks()
        except socket.error:
            self.error('Connection is dead')
            self.alive = False
            return None
        for task in results:
            data['total'] += 1
            data[_TASK_MAP[task.state]] += 1
            try:
                if task.active_task:
                    data['active'] += 1
                    data[_SCHED_MAP[task.scheduler_state]] += 1
                    data[_PROC_MAP[task.active_task_state]] += 1
            except AttributeError:
                pass
        return data
