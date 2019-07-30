# Description: dovecot netdata python.d module
# Author: Kyle Agronick (agronick)
# SPDX-License-Identifier: GPL-3.0+

# Gearman Netdata Plugin

from bases.FrameworkServices.SocketService import SocketService
from copy import deepcopy

update_every = 3
priority = 60500


CHARTS = {
    'total_workers': {
        'options': ['total', 'Total Workers', 'Workers', 'Total Workers', 'gearman.total', 'line'],
        'lines': [
            ['total_queued', 'Queued', 'absolute'],
            ['total_running', 'Running', 'absolute'],
        ]
    },
}


def job_chart_template(job_name):
    return {
        'options': [job_name, job_name, 'Workers', 'Jobs', 'gearman.job', 'stacked'],
        'lines': [
            ['{0}_queued'.format(job_name), 'Queued', 'absolute'],
            ['{0}_idle'.format(job_name), 'Idle', 'absolute'],
            ['{0}_running'.format(job_name), 'Running', 'absolute'],
        ]
    }


class Service(SocketService):
    def __init__(self, configuration=None, name=None):
        super(Service, self).__init__(configuration=configuration, name=name)
        self.request = "status\n"
        self._keep_alive = True

        self.host = self.configuration.get('host', 'localhost')
        self.port = self.configuration.get('port', 4730)

        self.tls = self.configuration.get('tls', False)
        self.cert = self.configuration.get('cert', None)
        self.key = self.configuration.get('key', None)

        self.active_jobs = set()
        self.definitions = deepcopy(CHARTS)
        self.order = ['total_workers']

    def _get_data(self):
        """
        Format data received from socket
        :return: dict
        """

        # There is no way to get an accurate total idle count because
        # a worker can be assigned to more than one job.
        output = {
            'total_queued': 0,
            'total_idle': 0,
            'total_running': 0,
        }

        found_jobs = set()

        for job in self._get_worker_data():
            job_name = job[0]

            # Gearman does not clean up old jobs
            # We only care about jobs that have
            # some relevant data
            if any(job[1:]):

                if job_name not in self.active_jobs:
                    self._add_chart(job_name)

                job_data = self._build_job(job)
                output.update(job_data)
                found_jobs.add(job_name)

                for sum_value in ('queued', 'running', 'idle'):
                    output['total_{0}'.format(sum_value)] += job_data['{0}_{1}'.format(job_name, sum_value)]

        for to_remove in self.active_jobs - found_jobs:
            self._remove_chart(to_remove)

        if found_jobs:
            return output

    def _get_worker_data(self):
        """
        Split the data returned from Gearman into a list of lists
        :return: list
        """

        try:
            raw = self._get_raw_data()
        except (ValueError, AttributeError):
            return

        if raw is None:
            self.debug("Gearman returned no data")
            return

        for line in sorted([job.split() for job in raw.splitlines()][:-1], key=lambda x: x[0]):
            line[1:] = map(int, line[1:])
            yield line

    def _build_job(self, job):
        """
        Get the status for each job
        :return: dict
        """

        total, running, available = job[1:]

        idle = available - running
        pending = total - running

        return {
            '{0}_queued'.format(job[0]): pending,
            '{0}_idle'.format(job[0]): idle,
            '{0}_running'.format(job[0]): running,
        }

    def _add_chart(self, job_name):
        self.active_jobs.add(job_name)
        job_key = 'job_{0}'.format(job_name)
        template = job_chart_template(job_name)
        new_chart = self.charts.add_chart([job_key] + template['options'])
        for dimension in template['lines']:
            new_chart.add_dimension(dimension)

    def _remove_chart(self, job_name):
        self.active_jobs.remove(job_name)
        job_key = 'job_{0}'.format(job_name)
        self.charts[job_key].obsolete()


