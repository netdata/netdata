# -*- coding: utf-8 -*-
# Description:
# Author: Pawel Krupa (paulfantom)
# Author: Ilya Mashchenko (l2isbad)

from threading import Thread

try:
    from time import sleep, monotonic as time
except ImportError:
    from time import sleep, time

from bases.charts import Charts, ChartError, create_runtime_chart
from bases.collection import OldVersionCompatibility, safe_print
from bases.loggers import PythonDLimitedLogger

RUNTIME_CHART_UPDATE = 'BEGIN netdata.runtime_{job_name} {since_last}\n' \
                       'SET run_time = {elapsed}\n' \
                       'END\n'


class RuntimeCounters:
    def __init__(self, configuration):
        """
        :param configuration: <dict>
        """
        self.FREQ = int(configuration.pop('update_every'))
        self.START_RUN = 0
        self.NEXT_RUN = 0
        self.PREV_UPDATE = 0
        self.SINCE_UPDATE = 0
        self.ELAPSED = 0
        self.RETRIES = 0
        self.RETRIES_MAX = configuration.pop('retries')
        self.PENALTY = 0

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
                             get_update_every=self.get_update_every)

    def __repr__(self):
        return '<{cls_bases}: {name}>'.format(cls_bases=', '.join(c.__name__ for c in self.__class__.__bases__),
                                              name=self.name)

    @property
    def name(self):
        if self.job_name:
            return '_'.join([self.module_name, self.override_name or self.job_name])
        return self.module_name

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
        self.debug('started, update frequency: {freq}, '
                   'retries: {retries}'.format(freq=job.FREQ, retries=job.RETRIES_MAX - job.RETRIES))

        while True:
            job.START_RUN = time()

            job.NEXT_RUN = job.START_RUN - (job.START_RUN % job.FREQ) + job.FREQ + job.PENALTY

            self.sleep_until_next_run()

            if job.PREV_UPDATE:
                job.SINCE_UPDATE = int((job.START_RUN - job.PREV_UPDATE) * 1e6)

            try:
                updated = self.update(interval=job.SINCE_UPDATE)
            except Exception as error:
                self.error('update() unhandled exception: {error}'.format(error=error))
                updated = False

            if not updated:
                if not self.manage_retries():
                    return
            else:
                job.ELAPSED = int((time() - job.START_RUN) * 1e3)
                job.PREV_UPDATE = job.START_RUN
                job.RETRIES, job.PENALTY = 0, 0
                safe_print(RUNTIME_CHART_UPDATE.format(job_name=self.name,
                                                       since_last=job.SINCE_UPDATE,
                                                       elapsed=job.ELAPSED))
            self.debug('update => [{status}] (elapsed time: {elapsed}, '
                       'retries left: {retries})'.format(status='OK' if updated else 'FAILED',
                                                         elapsed=job.ELAPSED if updated else '-',
                                                         retries=job.RETRIES_MAX - job.RETRIES))

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
                continue
            elif self.charts.cleanup and chart.penalty >= self.charts.cleanup:
                chart.obsolete()
                self.error("chart '{0}' was removed due to non updating".format(chart.name))
                continue

            ok = chart.update(data, interval)
            if ok:
                updated = True

        if not updated:
            self.debug('none of the charts has been updated')

        return updated

    def manage_retries(self):
        rc = self._runtime_counters
        rc.RETRIES += 1
        if rc.RETRIES % 5 == 0:
            rc.PENALTY = int(rc.RETRIES * self.update_every / 2)
        if rc.RETRIES >= rc.RETRIES_MAX:
            self.error('stopped after {0} data collection failures in a row'.format(rc.RETRIES_MAX))
            return False
        return True

    def sleep_until_next_run(self):
        job = self._runtime_counters

        # sleep() is interruptable
        while job.is_sleep_time():
            sleep_time = job.NEXT_RUN - job.START_RUN
            self.debug('sleeping for {sleep_time} to reach frequency of {freq} sec'.format(sleep_time=sleep_time,
                                                                                           freq=job.FREQ + job.PENALTY))
            sleep(sleep_time)
            job.START_RUN = time()

    def get_data(self):
        return self._get_data()

    def _get_data(self):
        raise NotImplementedError
