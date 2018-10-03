# -*- coding: utf-8 -*-
# Description: Linux power_supply netdata python.d module
# Author: Austin S. Hemmelgarn (Ferroin)

import os
import platform

from bases.FrameworkServices.SimpleService import SimpleService

# Everything except percentages is reported as µ units.
PRECISION = 10 ** 6

# A priority of 90000 places us next to the other PSU related stuff.
PRIORITY = 90000

# We add our charts dynamically when we probe for the device attributes,
# so these are empty by default.
ORDER = []

CHARTS = {}


def get_capacity_chart(syspath):
    # Capacity is measured in percent.  We track one value.
    options = [None, 'Capacity', '%', 'power_supply', 'power_supply.capacity', 'line']
    lines = list()
    attr_now = 'capacity'
    if get_sysfs_value(os.path.join(syspath, attr_now)) is not None:
        lines.append([attr_now, attr_now, 'absolute', 1, 1])
        return {'capacity': {'options': options, 'lines': lines}}, [attr_now]
    else:
        return None, None


def get_generic_chart(syspath, name, unit, maxname, minname):
    # Used to generate charts for energy, charge, and voltage.
    options = [None, name.title(), unit, 'power_supply', 'power_supply.{0}'.format(name), 'line']
    lines = list()
    attrlist = list()
    attr_max_design = '{0}_{1}_design'.format(name, maxname)
    attr_max = '{0}_{1}'.format(name, maxname)
    attr_now = '{0}_now'.format(name)
    attr_min = '{0}_{1}'.format(name, minname)
    attr_min_design = '{0}_{1}_design'.format(name, minname)
    if get_sysfs_value(os.path.join(syspath, attr_now)) is not None:
        lines.append([attr_now, attr_now, 'absolute', 1, PRECISION])
        attrlist.append(attr_now)
    else:
        return None, None
    if get_sysfs_value(os.path.join(syspath, attr_max)) is not None:
        lines.insert(0, [attr_max, attr_max, 'absolute', 1, PRECISION])
        lines.append([attr_min, attr_min, 'absolute', 1, PRECISION])
        attrlist.append(attr_max)
        attrlist.append(attr_min)
    elif get_sysfs_value(os.path.join(syspath, attr_min)) is not None:
        lines.append([attr_min, attr_min, 'absolute', 1, PRECISION])
        attrlist.append(attr_min)
    if get_sysfs_value(os.path.join(syspath, attr_max_design)) is not None:
        lines.insert(0, [attr_max_design, attr_max_design, 'absolute', 1, PRECISION])
        lines.append([attr_min_design, attr_min_design, 'absolute', 1, PRECISION])
        attrlist.append(attr_max_design)
        attrlist.append(attr_min_design)
    elif get_sysfs_value(os.path.join(syspath, attr_min_design)) is not None:
        lines.append([attr_min_design, attr_min_design, 'absolute', 1, PRECISION])
        attrlist.append(attr_min_design)
    return {name: {'options': options, 'lines': lines}}, attrlist


def get_charge_chart(syspath):
    # Charge is measured in microamphours.  We track up to five
    # attributes.
    return get_generic_chart(syspath, 'charge', 'µAh', 'full', 'empty')


def get_energy_chart(syspath):
    # Energy is measured in microwatthours.  We track up to five
    # attributes.
    return get_generic_chart(syspath, 'energy', 'µWh', 'full', 'empty')


def get_voltage_chart(syspath):
    # Voltage is measured in microvolts. We track up to five attributes.
    return get_generic_chart(syspath, 'voltage', 'µV', 'min', 'max')


# This is a list of functions for generating charts.  Used below to save
# a bit of code (and to make it a bit easier to add new charts).
GET_CHART = {
    'capacity': get_capacity_chart,
    'charge': get_charge_chart,
    'energy': get_energy_chart,
    'voltage': get_voltage_chart
}


# This opens the specified file and returns the value in it or None if
# the file doesn't exist.
def get_sysfs_value(filepath):
    try:
        with open(filepath, 'r') as datasource:
            return int(datasource.read())
    except (OSError, IOError):
        return None


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.definitions = dict()
        self.order = list()
        self.attrlist = list()
        self.supply = self.configuration.get('supply', None)
        if self.supply is not None:
            self.syspath = '/sys/class/power_supply/{0}'.format(self.supply)
        self.types = self.configuration.get('charts', 'capacity').split()

    def check(self):
        if platform.system() != 'Linux':
            self.error('Only supported on Linux.')
            return False
        if self.supply is None:
            self.error('No power supply specified for monitoring.')
            return False
        if not self.types:
            self.error('No attributes requested for monitoring.')
            return False
        if not os.access(self.syspath, os.R_OK):
            self.error('Unable to access {0}'.format(self.syspath))
            return False
        return self.create_charts()

    def create_charts(self):
        chartset = set(GET_CHART).intersection(set(self.types))
        if not chartset:
            self.error('No valid attributes requested for monitoring.')
            return False
        charts = dict()
        attrlist = list()
        for item in chartset:
            chart, attrs = GET_CHART[item](self.syspath)
            if chart is not None:
                charts.update(chart)
                attrlist.extend(attrs)
        if len(charts) == 0:
            self.error('No charts can be created.')
            return False
        self.definitions.update(charts)
        self.order.extend(sorted(charts))
        self.attrlist.extend(attrlist)
        return True

    def _get_data(self):
        data = dict()
        for attr in self.attrlist:
            attrpath = os.path.join(self.syspath, attr)
            if attr.endswith(('_min', '_min_design', '_empty', '_empty_design')):
                data[attr] = get_sysfs_value(attrpath) or 0
            else:
                data[attr] = get_sysfs_value(attrpath)
        return data
