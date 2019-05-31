# -*- coding: utf-8 -*-
# Description: megacli netdata python.d module
# Author: Ilya Mashchenko (ilyam8)
# SPDX-License-Identifier: GPL-3.0-or-later


import re

from bases.FrameworkServices.ExecutableService import ExecutableService
from bases.collection import find_binary


disabled_by_default = True

update_every = 5


def adapter_charts(ads):
    order = [
        'adapter_degraded',
    ]

    def dims(ad):
        return [['adapter_{0}_degraded'.format(a.id), 'adapter {0}'.format(a.id)] for a in ad]

    charts = {
        'adapter_degraded': {
            'options': [None, 'Adapter State', 'is degraded', 'adapter', 'megacli.adapter_degraded', 'line'],
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
            'options': [None, 'Physical Drives Media Errors', 'errors/s', 'pd', 'megacli.pd_media_error', 'line'],
            'lines': dims('media_error', pds)
        },
        'pd_predictive_failure': {
            'options': [None, 'Physical Drives Predictive Failures', 'failures/s', 'pd',
                        'megacli.pd_predictive_failure', 'line'],
            'lines': dims('predictive_failure', pds)
        }
    }

    return order, charts


def battery_charts(bats):
    order = list()
    charts = dict()

    for b in bats:
        order.append('bbu_{0}_relative_charge'.format(b.id))
        charts.update(
            {
                'bbu_{0}_relative_charge'.format(b.id): {
                    'options': [None, 'Relative State of Charge', 'percentage', 'battery',
                                'megacli.bbu_relative_charge', 'line'],
                    'lines': [
                        ['bbu_{0}_relative_charge'.format(b.id), 'adapter {0}'.format(b.id)],
                    ]
                }
            }
        )

    for b in bats:
        order.append('bbu_{0}_cycle_count'.format(b.id))
        charts.update(
            {
                'bbu_{0}_cycle_count'.format(b.id): {
                    'options': [None, 'Cycle Count', 'cycle count', 'battery', 'megacli.bbu_cycle_count', 'line'],
                    'lines': [
                        ['bbu_{0}_cycle_count'.format(b.id), 'adapter {0}'.format(b.id)],
                    ]
                }
            }
        )

    return order, charts


RE_ADAPTER = re.compile(
    r'Adapter #([0-9]+) State(?:\s+)?: ([a-zA-Z]+)'
)

RE_VD = re.compile(
    r'Slot Number: ([0-9]+) Media Error Count: ([0-9]+) Predictive Failure Count: ([0-9]+)'
)

RE_BATTERY = re.compile(
    r'BBU Capacity Info for Adapter: ([0-9]+) Relative State of Charge: ([0-9]+) % Cycle Count: ([0-9]+)'
)


def find_adapters(d):
    keys = ('Adapter #', 'State')
    d = ' '.join(v.strip() for v in d if v.startswith(keys))
    return [Adapter(*v) for v in RE_ADAPTER.findall(d)]


def find_pds(d):
    keys = ('Slot Number',  'Media Error Count', 'Predictive Failure Count')
    d = ' '.join(v.strip() for v in d if v.startswith(keys))
    return [PD(*v) for v in RE_VD.findall(d)]


def find_batteries(d):
    keys = ('BBU Capacity Info for Adapter', 'Relative State of Charge', 'Cycle Count')
    d = ' '.join(v.strip() for v in d if v.strip().startswith(keys))
    return [Battery(*v) for v in RE_BATTERY.findall(d)]


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


class Battery:
    def __init__(self, adapt_id, rel_charge, cycle_count):
        self.id = adapt_id
        self.rel_charge = rel_charge
        self.cycle_count = cycle_count

    def data(self):
        return {
            'bbu_{0}_relative_charge'.format(self.id): self.rel_charge,
            'bbu_{0}_cycle_count'.format(self.id): self.cycle_count,
        }


# TODO: hardcoded sudo...
class Megacli:
    def __init__(self):
        self.s = find_binary('sudo')
        self.m = find_binary('megacli')
        self.sudo_check = [self.s, '-n', '-v']
        self.disk_info = [self.s, '-n', self.m, '-LDPDInfo', '-aAll', '-NoLog']
        self.battery_info = [self.s, '-n', self.m, '-AdpBbuCmd', '-a0', '-NoLog']

    def __bool__(self):
        return bool(self.s and self.m)

    def __nonzero__(self):
        return self.__bool__()


class Service(ExecutableService):
    def __init__(self, configuration=None, name=None):
        ExecutableService.__init__(self, configuration=configuration, name=name)
        self.order = list()
        self.definitions = dict()
        self.do_battery = self.configuration.get('do_battery')
        self.megacli = Megacli()

    def check_sudo(self):
        err = self._get_raw_data(command=self.megacli.sudo_check, stderr=True)
        if err:
            self.error(''.join(err))
            return False
        return True

    def check_disk_info(self):
        d = self._get_raw_data(command=self.megacli.disk_info)
        if not d:
            return False

        ads = find_adapters(d)
        pds = find_pds(d)

        if not (ads and pds):
            self.error('failed to parse "{0}" output'.format(' '.join(self.megacli.disk_info)))
            return False

        o, c = adapter_charts(ads)
        self.order.extend(o)
        self.definitions.update(c)

        o, c = pd_charts(pds)
        self.order.extend(o)
        self.definitions.update(c)

        return True

    def check_battery(self):
        d = self._get_raw_data(command=self.megacli.battery_info)
        if not d:
            return False

        bats = find_batteries(d)

        if not bats:
            self.error('failed to parse "{0}" output'.format(' '.join(self.megacli.battery_info)))
            return False

        o, c = battery_charts(bats)
        self.order.extend(o)
        self.definitions.update(c)
        return True

    def check(self):
        if not self.megacli:
            self.error('can\'t locate "sudo" or "megacli" binary')
            return None

        if not (self.check_sudo() and self.check_disk_info()):
            return False

        if self.do_battery:
            self.do_battery = self.check_battery()

        return True

    def get_data(self):
        data = dict()

        data.update(self.get_adapter_pd_data())

        if self.do_battery:
            data.update(self.get_battery_data())

        return data or None

    def get_adapter_pd_data(self):
        raw = self._get_raw_data(command=self.megacli.disk_info)
        data = dict()

        if not raw:
            return data

        for a in find_adapters(raw):
            data.update(a.data())

        for p in find_pds(raw):
            data.update(p.data())

        return data

    def get_battery_data(self):
        raw = self._get_raw_data(command=self.megacli.battery_info)
        data = dict()

        if not raw:
            return data

        for b in find_batteries(raw):
            data.update(b.data())

        return data
