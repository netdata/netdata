# -*- coding: utf-8 -*-
# Description:
# Author: Pawel Krupa (paulfantom)
# Author: Ilya Mashchenko (l2isbad)

from threading import Thread

try:
    from time import sleep, monotonic as time
except ImportError:
    from time import sleep, time

from bases.charts import Charts, create_runtime_chart
from bases.collection import OldVersionCompatibility, UsefulFuncs, safe_print
from bases.loggers import PythonDLimitedLogger

CHART_OBSOLETE_PENALTY = 10

START_MSG = 'STARTED. Update frequency: {freq}, retries: {retries}.'
UPDATE_MSG = 'UPDATE {status}. Elapsed time: {elapsed}, retries left: {retries}.'
STOP_MSG = 'STOPPED after {retries_max} data collection failures in a row.'
SLEEP_MSG = 'SLEEPING for {sleep_time} to reach frequency of {freq} sec.'

RUNTIME_CHART_UPDATE = 'BEGIN netdata.runtime_{job_name} {since_last}\n' \
                       'SET run_time = {elapsed}\n' \
                       'END\n'


class RuntimeCounters:
    def __init__(self, configuration):
        """
        :param configuration: <dict>
        """
        self.FREQ = int(configuration.pop('update_every', 1))
        self.START_RUN = 0
        self.NEXT_RUN = 0
        self.PREV_UPDATE = 0
        self.SINCE_UPDATE = 0
        self.ELAPSED = 0
        self.RETRIES = 0
        self.RETRIES_MAX = configuration.pop('retries', 10)

    def is_sleep_time(self):
        return self.START_RUN < self.NEXT_RUN


class SimpleService(Thread, PythonDLimitedLogger, OldVersionCompatibility, object):
    """
    Prototype of Service class.
    Implemented basic functionality to run jobs by `python.d.plugin`
    """
    def __init__(self, configuration, name=''):
        """
        :param configuration: <dict>
        :param name: <str>
        """
        Thread.__init__(self)
        self.daemon = True
        PythonDLimitedLogger.__init__(self)
        OldVersionCompatibility.__init__(self)
        self.configuration = configuration
        self.module_name = configuration.pop('module_name')
        self.job_name = configuration.pop('job_name')
        self.override_name = configuration.pop('override_name')
        self.fake_name = None
        self.order = list()
        self.definitions = dict()
        self._runtime_counters = RuntimeCounters(configuration=configuration)
        self.charts = Charts(job_name=self.actual_name,
                             priority=configuration.pop('priority', 60000),
                             update_every=self._runtime_counters.FREQ)
        self.functions = UsefulFuncs()

    def __repr__(self):
        return '<{cls_bases}: {name}>'.format(cls_bases=', '.join(c.__name__ for c in self.__class__.__bases__),
                                              name=self.name)

    @property
    def name(self):
        if self.job_name:
            return '_'.join([self.module_name, self.override_name or self.job_name])
        return self.module_name

    @property
    def actual_name(self):
        return self.fake_name or self.name

    @property
    def update_every(self):
        return self._runtime_counters.FREQ

    @update_every.setter
    def update_every(self, value):
        """
        :param value: <int>
        :return:
        """
        self._runtime_counters.FREQ = value

    def check(self):
        """
        check() prototype
        :return: boolean
        """
        try:
            data = self._get_data()
        except Exception as error:
            self.debug('CHECK {{error: {error}}}'.format(error=error))
        else:
            if data and isinstance(data, dict):
                return True
            self.debug('CHECK returned no data')
            return False

    @create_runtime_chart
    def create(self):
        for chart_name in self.order:
            chart_config = self.definitions.get(chart_name)
            if not chart_config:
                self.debug('{chart_name} not in definitions'.format(chart_name=chart_name))
                continue

            chart_params = ([chart_name] + chart_config['options'])
            ok = self.charts.add_chart(params=chart_params)
            if not ok:
                self.debug('"{chart}" chart no added'.format(chart=chart_name))
                continue

            for line in chart_config['lines']:
                self.charts[chart_name].add_dimension(line)

        del self.order
        del self.definitions

        if self.charts.empty():
            return None

        for chart in self.charts:
            safe_print(chart.create())
        return True

    def run(self):
        """
        Runs job in thread. Handles retries.
        Exits when job failed or timed out.
        :return: None
        """
        job = self._runtime_counters
        self.debug(START_MSG.format(freq=job.FREQ, retries=job.RETRIES_MAX - job.RETRIES))

        while True:
            job.START_RUN = time()

            job.NEXT_RUN = job.START_RUN - (job.START_RUN % job.FREQ) + job.FREQ

            self.sleep_until_next_run()

            if job.PREV_UPDATE:
                job.SINCE_UPDATE = int((job.START_RUN - job.PREV_UPDATE) * 1e6)

            try:
                updated = self.update(interval=job.SINCE_UPDATE)
            except Exception as error:
                print(error)
                updated = False

            if not updated:
                if not self.manage_retries():
                    return
            else:
                current_time = time()
                job.PREV_UPDATE = current_time
                job.ELAPSED = int((current_time - job.START_RUN) * 1e3)
                job.RETRIES = 0
                safe_print(RUNTIME_CHART_UPDATE.format(job_name=self.name,
                                                       since_last=job.SINCE_UPDATE,
                                                       elapsed=job.ELAPSED))
            self.debug(UPDATE_MSG.format(status='OK' if updated else 'FAILED',
                                         elapsed=job.ELAPSED if updated else '-',
                                         retries=job.RETRIES_MAX - job.RETRIES))

    def update(self, interval):
        """
        :param interval: <int>
        :return:
        """
        data = self.get_data()
        if not data:
            return None
        charts_updated = False

        for chart in self.charts.penalty_exceeded(penalty_max=CHART_OBSOLETE_PENALTY):
            safe_print(chart.obsolete())
            del self.charts[chart.params['id']]

        for chart in self.charts:
            dimension_updated = str()
            for dimension in chart.dimensions:
                try:
                    value = int(data[dimension.params['id']])
                except (KeyError, TypeError):
                    continue
                dimension_updated += dimension.set(value)

            if dimension_updated:
                charts_updated = True
                safe_print(''.join([chart.begin(since_last=interval),
                                    dimension_updated, 'END\n']))
            else:
                chart.penalty += 1
        return charts_updated

    def manage_retries(self):
        self._runtime_counters.RETRIES += 1
        if self._runtime_counters.RETRIES >= self._runtime_counters.RETRIES_MAX:
            self.error(STOP_MSG.format(retries_max=self._runtime_counters.RETRIES_MAX))
            return False
        return True

    def sleep_until_next_run(self):
        job = self._runtime_counters

        # sleep() is interruptable
        while job.is_sleep_time():
            sleep_time = job.NEXT_RUN - job.START_RUN
            self.debug(SLEEP_MSG.format(sleep_time=sleep_time,
                                        freq=job.FREQ))
            sleep(sleep_time)
            job.START_RUN = time()

    def get_data(self):
        return self._get_data()

    def _get_data(self):
        raise NotImplementedError
