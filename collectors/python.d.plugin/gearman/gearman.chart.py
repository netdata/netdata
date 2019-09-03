# Description: dovecot netdata python.d module
# Author: Kyle Agronick (agronick)
# SPDX-License-Identifier: GPL-3.0+

# Gearman Netdata Plugin

from bases.FrameworkServices.SocketService import SocketService
from copy import deepcopy


CHARTS = {
    'total_workers': {
        'options': [None, 'Total Jobs', 'Jobs', 'Total Jobs', 'gearman.total_jobs', 'line'],
        'lines': [
            ['total_pending', 'Pending', 'absolute'],
            ['total_running', 'Running', 'absolute'],
        ]
    },
}


def job_chart_template(job_name):
    return {
        'options': [None, job_name, 'Jobs', 'Activity by Job', 'gearman.single_job', 'stacked'],
        'lines': [
            ['{0}_pending'.format(job_name), 'Pending', 'absolute'],
            ['{0}_idle'.format(job_name), 'Idle', 'absolute'],
            ['{0}_running'.format(job_name), 'Running', 'absolute'],
        ]
    }

def build_result_dict(job):
    """
    Get the status for each job
    :return: dict
    """

    total, running, available = job['metrics']

    idle = available - running
    pending = total - running

    return {
        '{0}_pending'.format(job['job_name']): pending,
        '{0}_idle'.format(job['job_name']): idle,
        '{0}_running'.format(job['job_name']): running,
    }

def parse_worker_data(job):
    job_name = job[0]
    job_metrics = job[1:]

    return {
        'job_name': job_name,
        'metrics': job_metrics,
    }


class GearmanReadException(BaseException):
    pass


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

        try:
            active_jobs = self.get_active_jobs()
        except GearmanReadException:
            return None

        found_jobs, job_data = self.process_jobs(active_jobs)
        self.remove_stale_jobs(found_jobs)
        return job_data

    def get_active_jobs(self):
        active_jobs = []

        for job in self.get_worker_data():
            parsed_job = parse_worker_data(job)

            # Gearman does not clean up old jobs
            # We only care about jobs that have
            # some relevant data
            if not any(parsed_job['metrics']):
                continue

            active_jobs.append(parsed_job)

        return active_jobs

    def get_worker_data(self):
        """
        Split the data returned from Gearman
        into a list of lists

        This returns the same output that you
        would get from a gearadmin --status
        command.

        Example output returned from
        _get_raw_data():
        generic_worker2 78      78      500
        generic_worker3 0       0       760
        generic_worker1 0       0       500

        :return: list
        """

        try:
            raw = self._get_raw_data()
        except (ValueError, AttributeError):
            raise GearmanReadException()

        if raw is None:
            self.debug("Gearman returned no data")
            raise GearmanReadException()

        job_lines = raw.splitlines()[:-1]
        job_lines = [job.split() for job in sorted(job_lines)]

        for line in job_lines:
            line[1:] = map(int, line[1:])

        return job_lines

    def process_jobs(self, active_jobs):

        output = {
            'total_pending': 0,
            'total_idle': 0,
            'total_running': 0,
        }
        found_jobs = set()

        for parsed_job in active_jobs:

            job_name = self.add_job(parsed_job)
            found_jobs.add(job_name)
            job_data = build_result_dict(parsed_job)

            for sum_value in ('pending', 'running', 'idle'):
                output['total_{0}'.format(sum_value)] += job_data['{0}_{1}'.format(job_name, sum_value)]

            output.update(job_data)

        return found_jobs, output

    def remove_stale_jobs(self, active_job_list):
        """
        Removes jobs that have no workers, pending jobs,
        or running jobs
        :param active_job_list: The latest list of active jobs
        :type active_job_list: iterable
        :return: None
        """

        for to_remove in self.active_jobs - active_job_list:
            self.remove_job(to_remove)

    def add_job(self, parsed_job):
        """
        Adds a job to the list of active jobs
        :param parsed_job: A parsed job dict
        :type parsed_job: dict
        :return: None
        """

        def add_chart(job_name):
            """
            Adds a new job chart
            :param job_name: The name of the job to add
            :type job_name: string
            :return: None
            """

            job_key = 'job_{0}'.format(job_name)
            template = job_chart_template(job_name)
            new_chart = self.charts.add_chart([job_key] + template['options'])
            for dimension in template['lines']:
                new_chart.add_dimension(dimension)

        if parsed_job['job_name'] not in self.active_jobs:
            add_chart(parsed_job['job_name'])
            self.active_jobs.add(parsed_job['job_name'])

        return parsed_job['job_name']

    def remove_job(self, job_name):
        """
        Removes a job to the list of active jobs
        :param job_name: The name of the job to remove
        :type job_name: string
        :return: None
        """

        def remove_chart(job_name):
            """
            Removes a job chart
            :param job_name: The name of the job to remove
            :type job_name: string
            :return: None
            """

            job_key = 'job_{0}'.format(job_name)
            self.charts[job_key].obsolete()
            del self.charts[job_key]

        remove_chart(job_name)
        self.active_jobs.remove(job_name)
