# -*- coding: utf-8 -*-
# Description: docker netdata python.d module
# Author: Kévin Darcel (@tuxity)

try:
    import docker
    HAS_DOCKER = True
except ImportError:
    HAS_DOCKER = False

from bases.FrameworkServices.SimpleService import SimpleService

from distutils.version import StrictVersion


# charts order (can be overridden if you want less charts, or different order)
ORDER = [
    'running_containers',
    'healthy_containers',
    'unhealthy_containers'
]

CHARTS = {
    'running_containers': {
        'options': [None, 'Number of running containers', 'containers', 'running containers',
                    'docker.running_containers', 'line'],
        'lines': [
            ['running_containers', 'running']
        ]
    },
    'healthy_containers': {
        'options': [None, 'Number of healthy containers', 'containers', 'healthy containers',
                    'docker.healthy_containers', 'line'],
        'lines': [
            ['healthy_containers', 'healthy']
        ]
    },
    'unhealthy_containers': {
        'options': [None, 'Number of unhealthy containers', 'containers', 'unhealthy containers',
                    'docker.unhealthy_containers', 'line'],
        'lines': [
            ['unhealthy_containers', 'unhealthy']
        ]
    }
}


MIN_REQUIRED_VERSION = '3.2.0'


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.client = None

    def check(self):
        if not HAS_DOCKER:
            self.error("'docker' package is needed to use dockerd module")
            return False

        if StrictVersion(docker.__version__) < StrictVersion(MIN_REQUIRED_VERSION):
            self.error("installed 'docker' package version {0}, minimum required version {1}, please upgrade".format(
                docker.__version__,
                MIN_REQUIRED_VERSION,
            ))
            return False

        self.client = docker.DockerClient(base_url=self.configuration.get('url', 'unix://var/run/docker.sock'))

        try:
            self.client.ping()
        except docker.errors.APIError as error:
            self.error(error)
            return False

        return True

    def get_data(self):
        data = dict()

        data['running_containers'] = len(self.client.containers.list(sparse=True))
        data['healthy_containers'] = len(self.client.containers.list(filters={'health': 'healthy'}, sparse=True))
        data['unhealthy_containers'] = len(self.client.containers.list(filters={'health': 'unhealthy'}, sparse=True))

        return data or None
