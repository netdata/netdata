# -*- coding: utf-8 -*-
# Description: example netdata python.d module
# Author: Pawel Krupa (paulfantom)

import os
import random
from base import SimpleService

NAME = os.path.basename(__file__).replace(".chart.py", "")

# default module values
# update_every = 4
priority = 90000
retries = 60


# Programmatic style:

class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        super(self.__class__,self).__init__(configuration=configuration, name=name)

    def check(self):
        return True
    
    def create(self):
        self.chart("example.python_random", '', 'A random number', 'random number',
                   'random', 'random', 'line', self.priority, self.update_every)
        self.dimension('random1')
        self.commit()
        return True
    
    def update(self, interval):
        self.begin("example.python_random", interval)
        self.set("random1", random.randint(0, 100))
        self.end()
        self.commit()
        return True


# Declarative style:

ORDER = ['chart1', 'chart2']
CHARTS = {
    'chart1': {
        'options': [None, '<chart title>', '<dimension>', 'bandwidth', 'example.chart1', 'line'],
        'lines': [
            ["<line description>", "line1", "absolute"],
        ]
    }
}

class DeclarativeService(SimpleService):
    # Rename it to "Service" to use it

    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS

        if self.check():
            # perform initialization only if needed
            pass

    def check(self):
        """Check if the plugin should be run
        :return: boolean
        """
        try:
            # check for the required files, processes, and so on..
            pass
            return True

        except Exception:
            return False

    def _get_data(self):
        """Get metrics
        :return: dict or None
        """
        data = {}
        # Gather metrics into a <metric name> -> <value> dict
        pass

        if len(data) == 0:
            return None

        return data


if __name__ == '__main__':
    # Used to test the plugin from command line
    # You might need to pass PYTHONPATH to locate the "base.py" module
    # And Netdata env vars like NETDATA_CONFIG_DIR
    s = Service(
        name="NAME",
        configuration=dict(
            priority=90000,
            retries=60,
            update_every=1
        ),
    )
    s.update(1)
