# -*- coding: utf-8 -*-
# Description: megacli netdata python.d module
# Author: Ilya Mashchenko (l2isbad)
# SPDX-License-Identifier: GPL-3.0+


import re

from bases.FrameworkServices.ExecutableService import ExecutableService
from bases.collection import find_binary


update_every = 5


def adapter_charts(ads):
    order = [
        'adapter_degraded',
    ]

    def dims(ad):
        return [['adapter_{0}_degraded'.format(a.id), 'adapter {0}'.format(a.id)] for a in ad]

    charts = {
        'adapter_degraded': {
            'options': [
                    None, 'Adapter State', 'is degraded', 'adapter', 'megacli.adapter_degraded', 'line'],
            'lines': dims(ads)
            },
    }

    return order, charts


def pd_charts(pds):
    order = [
        'pd_media_error',
        'pd_predictive_failure',
    ]

    def dims(k, pd):
        return [['slot_{0}_{1}'.format(p.id, k), 'slot {0}'.format(p.id), 'incremental'] for p in pd]

    charts = {
        'pd_media_error': {
            'options': [
                None, 'Physical Drives Media Errors', 'errors/s', 'pd', 'megacli.pd_media_error', 'line'
            ],
            'lines': dims("media_error", pds)},
        'pd_predictive_failure': {
            'options': [
                None, 'Physical Drives Predictive Failures', 'failures/s', 'pd', 'megacli.pd_predictive_failure', 'line'
            ],
            'lines': dims("predictive_failure", pds)}
    }

    return order, charts


RE_ADAPTER = re.compile(
    r'Adapter #([0-9]+) State\s+: ([a-zA-Z]+)'
)

RE_VD = re.compile(
    r'Slot Number: ([0-9]+) Media Error Count: ([0-9]+) Predictive Failure Count: ([0-9]+)'
)


def find_adapters(d):
    keys = ("Adapter #", "State")
    d = ' '.join(v.strip() for v in d if v.startswith(keys))
    return [Adapter(*v) for v in RE_ADAPTER.findall(d)]


def find_pds(d):
    keys = ("Slot Number",  "Media Error Count", "Predictive Failure Count")
    d = ' '.join(v.strip() for v in d if v.startswith(keys))
    return [PD(*v) for v in RE_VD.findall(d)]


class Adapter:
    def __init__(self, n, state):
        self.id = n
        self.state = int(state == 'Degraded')

    def data(self):
        return {
            'adapter_{0}_degraded'.format(self.id): self.state,
            }


class PD:
    def __init__(self, n, media_err, predict_fail):
        self.id = n
        self.media_err = media_err
        self.predict_fail = predict_fail

    def data(self):
        return {
            'slot_{0}_media_error'.format(self.id): self.media_err,
            'slot_{0}_predictive_failure'.format(self.id): self.predict_fail,
        }


class Command:
    def __init__(self):
        self.command = find_binary('megacli')
        self.disk_info = [self.command, '-LDPDInfo', '-aAll']
        self.battery_info = [self.command, '-AdpBbuCmd', '-a0']

    def __bool__(self):
        return bool(self.command)

    def __nonzero__(self):
        return self.__bool__()


class Service(ExecutableService):
    def __init__(self, configuration=None, name=None):
        ExecutableService.__init__(self, configuration=configuration, name=name)
        self.order = list()
        self.definitions = dict()
        self.command = Command()

    def check(self):
        if not self.command:
            self.error("can't locate \"megacli\" binary or binary is not executable by netdata")
            return None

        d = self._get_raw_data(command=self.command.disk_info)

        if not d:
            return None

        ads = find_adapters(d)
        pds = find_pds(d)

        if not (ads and pds):
            self.error('failed to parse "{0}" output'.format(' '.join(self.command.disk_info)))
            return None

        self.create_charts(ads, pds)
        return True

    def get_data(self):
        return self.get_adapter_pd_data()

    def get_adapter_pd_data(self):
        raw = self._get_raw_data(command=self.command.disk_info)

        if not raw:
            return None

        data = dict()

        for a in find_adapters(raw):
            data.update(a.data())

        for p in find_pds(raw):
            data.update(p.data())

        return data

    def create_charts(self, ads, pds):
        o, c = adapter_charts(ads)
        self.order.extend(o)
        self.definitions.update(c)

        o, c = pd_charts(pds)
        self.order.extend(o)
        self.definitions.update(c)
