# -*- coding: utf-8 -*-
# Description: postfix netdata python.d module
# Author: Pawel Krupa (paulfantom)
# SPDX-License-Identifier: GPL-3.0+

from bases.FrameworkServices.ExecutableService import ExecutableService

# default module values (can be overridden per job in `config`)
# update_every = 2
priority = 60000
retries = 60

# charts order (can be overridden if you want less charts, or different order)
ORDER = ['qemails', 'qsize']

CHARTS = {
    'qemails': {
        'options': [None, "Postfix Queue Emails", "emails", 'queue', 'postfix.qemails', 'line'],
        'lines': [
            ['emails', None, 'absolute']
        ]},
    'qsize': {
        'options': [None, "Postfix Queue Emails Size", "emails size in KB", 'queue', 'postfix.qsize', 'area'],
        'lines': [
            ["size", None, 'absolute']
        ]}
}


class Service(ExecutableService):
    def __init__(self, configuration=None, name=None):
        ExecutableService.__init__(self, configuration=configuration, name=name)
        self.command = "postqueue -p"
        self.order = ORDER
        self.definitions = CHARTS

    def _get_data(self):
        """
        Format data received from shell command
        :return: dict
        """
        try:
            raw = self._get_raw_data()[-1].split(' ')
            if raw[0] == 'Mail' and raw[1] == 'queue':
                return {'emails': 0,
                        'size': 0}

            return {'emails': raw[4],
                    'size': raw[1]}
        except (ValueError, AttributeError):
            return None
