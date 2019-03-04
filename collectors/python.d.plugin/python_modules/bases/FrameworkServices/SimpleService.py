# -*- coding: utf-8 -*-
# Description:
# Author: Pawel Krupa (paulfantom)
# Author: Ilya Mashchenko (l2isbad)
# SPDX-License-Identifier: GPL-3.0-or-later


from time import sleep, time

from third_party.monotonic import monotonic

from bases.charts import Charts, ChartError, create_runtime_chart
from bases.collection import OldVersionCompatibility, safe_print
from bases.loggers import PythonDLimitedLogger

RUNTIME_CHART_UPDATE = 'BEGIN netdata.runtime_{job_name} {since_last}\n' \
                       'SET run_time = {elapsed}\n' \
                       'END\n'

PENALTY_EVERY = 5
MAX_PENALTY = 10 * 60  # 10 minutes


class RuntimeCounters:
    def __init__(self, configuration):
        """
        :param configuration: <dict>
        """
        self.update_every = int(configuration.pop('update_every'))
        self.do_penalty = configuration.pop('penalty')

        self.start_mono = 0
        self.start_real = 0
        self.retries = 0
        self.penalty = 0
        self.elapsed = 0
        self.prev_update = 0

        self.runs = 1

    def calc_next(self):
        self.start_mono = monotonic()
        return self.start_mono - (self.start_mono % self.update_every) + self.update_every + self.penalty

    def sleep_until_next(self):
        next_time = self.calc_next()
        while self.start_mono < next_time:
            sleep(next_time - self.start_mono)
            self.start_mono = monotonic()
            self.start_real = time()

    def handle_retries(self):
        self.retries += 1
        if self.do_penalty and self.retries % PENALTY_EVERY == 0:
            self.penalty = round(min(self.retries * self.update_every / 2, MAX_PENALTY))


class SimpleService(PythonDLimitedLogger, OldVersionCompatibility, object):
    """
    Prototype of Service class.
    Implemented basic functionality to run jobs by `python.d.plugin`
    """
    def __init__(self, configuration, name=''):
        """
        :param configuration: <dict>
        :param name: <str>
        """
        PythonDLimitedLogger.__init__(self)
        OldVersionCompatibility.__init__(self)
        self.configuration = configuration
        self.order = list()
        self.definitions = dict()

        self.module_name = self.__module__
        self.job_name = configuration.pop('job_name')
        self.override_name = configuration.pop('override_name')
        self.fake_name = None

        self._runtime_counters = RuntimeCounters(configuration=configuration)
        self.charts = Charts(job_name=self.actual_name,
                             priority=configuration.pop('priority'),
                             cleanup=configuration.pop('chart_cleanup'),
                             get_update_every=self.get_update_every,
                             module_name=self.module_name)

    def __repr__(self):
        return '<{cls_bases}: {name}>'.format(cls_bases=', '.join(c.__name__ for c in self.__class__.__bases__),
                                              name=self.name)

    @property
    def name(self):
        if self.job_name and self.job_name != self.module_name:
            return '_'.join([self.module_name, self.override_name or self.job_name])
        return self.module_name

    def actual_name(self):
        return self.fake_name or self.name

    @property
    def runs_counter(self):
        return self._runtime_counters.runs

    @property
    def update_every(self):
        return self._runtime_counters.update_every

    @update_every.setter
    def update_every(self, value):
        """
        :param value: <int>
        :return:
        """
        self._runtime_counters.update_every = value

    def get_update_every(self):
        return self.update_every

    def check(self):
        """
        check() prototype
        :return: boolean
        """
        self.debug("job doesn't implement check() method. Using default which simply invokes get_data().")
        data = self.get_data()
        if data and isinstance(data, dict):
            return True
        self.debug('returned value is wrong: {0}'.format(data))
        return False

    @create_runtime_chart
    def create(self):
        for chart_name in self.order:
            chart_config = self.definitions.get(chart_name)

            if not chart_config:
                self.debug("create() => [NOT ADDED] chart '{chart_name}' not in definitions. "
                           "Skipping it.".format(chart_name=chart_name))
                continue

            #  create chart
            chart_params = [chart_name] + chart_config['options']
            try:
                self.charts.add_chart(params=chart_params)
            except ChartError as error:
                self.error("create() => [NOT ADDED] (chart '{chart}': {error})".format(chart=chart_name,
                                                                                       error=error))
                continue

            # add dimensions to chart
            for dimension in chart_config['lines']:
                try:
                    self.charts[chart_name].add_dimension(dimension)
                except ChartError as error:
                    self.error("create() => [NOT ADDED] (dimension '{dimension}': {error})".format(dimension=dimension,
                                                                                                   error=error))
                    continue

            # add variables to chart
            if 'variables' in chart_config:
                for variable in chart_config['variables']:
                    try:
                        self.charts[chart_name].add_variable(variable)
                    except ChartError as error:
                        self.error("create() => [NOT ADDED] (variable '{var}': {error})".format(var=variable,
                                                                                                error=error))
                        continue

        del self.order
        del self.definitions

        # True if job has at least 1 chart else False
        return bool(self.charts)

    def run(self):
        """
        Runs job in thread. Handles retries.
        Exits when job failed or timed out.
        :return: None
        """
        job = self._runtime_counters
        self.debug('started, update frequency: {freq}'.format(freq=job.update_every))

        while True:
            job.sleep_until_next()

            since = 0
            if job.prev_update:
                since = int((job.start_real - job.prev_update) * 1e6)

            try:
                updated = self.update(interval=since)
            except Exception as error:
                self.error('update() unhandled exception: {error}'.format(error=error))
                updated = False

            job.runs += 1

            if not updated:
                job.handle_retries()
            else:
                job.elapsed = int((monotonic() - job.start_mono) * 1e3)
                job.prev_update = job.start_real
                job.retries, job.penalty = 0, 0
                safe_print(RUNTIME_CHART_UPDATE.format(job_name=self.name,
                                                       since_last=since,
                                                       elapsed=job.elapsed))
            self.debug('update => [{status}] (elapsed time: {elapsed}, failed retries in a row: {retries})'.format(
                status='OK' if updated else 'FAILED',
                elapsed=job.elapsed if updated else '-',
                retries=job.retries))

    def update(self, interval):
        """
        :return:
        """
        data = self.get_data()
        if not data:
            self.debug('get_data() returned no data')
            return False
        elif not isinstance(data, dict):
            self.debug('get_data() returned incorrect type data')
            return False

        updated = False

        for chart in self.charts:
            if chart.flags.obsoleted:
                if chart.can_be_updated(data):
                    chart.refresh()
                else:
                    continue
            elif self.charts.cleanup and chart.penalty >= self.charts.cleanup:
                chart.obsolete()
                self.error("chart '{0}' was suppressed due to non updating".format(chart.name))
                continue

            ok = chart.update(data, interval)
            if ok:
                updated = True

        if not updated:
            self.debug('none of the charts has been updated')

        return updated

    def get_data(self):
        return self._get_data()

    def _get_data(self):
        raise NotImplementedError
