# -*- coding: utf-8 -*-
# Description: exim netdata python.d module
# Author: Pawel Krupa (paulfantom)
# SPDX-License-Identifier: GPL-3.0+

from bases.FrameworkServices.ExecutableService import ExecutableService

# default module values (can be overridden per job in `config`)
# update_every = 2
priority = 60000
retries = 60

# charts order (can be overridden if you want less charts, or different order)
ORDER = ['qemails']

CHARTS = {
    'qemails': {
        'options': [None, "Exim Queue Emails", "emails", 'queue', 'exim.qemails', 'line'],
        'lines': [
            ['emails', None, 'absolute']
        ]}
}


class Service(ExecutableService):
    def __init__(self, configuration=None, name=None):
        ExecutableService.__init__(self, configuration=configuration, name=name)
        self.command = "exim -bpc"
        self.order = ORDER
        self.definitions = CHARTS

    def _get_data(self):
        """
        Format data received from shell command
        :return: dict
        """
        try:
            return {'emails': int(self._get_raw_data()[0])}
        except (ValueError, AttributeError):
            return None
