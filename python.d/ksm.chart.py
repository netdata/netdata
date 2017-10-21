# -*- coding: utf-8 -*-
# Description: kernel ksm netdata python.d plugin
# Author: Federico Ceratto <federico.ceratto@gmail.com>
# © 2017 - released under GPLv3, see LICENSE file

import os

from base import SimpleService

ORDER = ['pages_shared', 'pages_sharing', 'pages_unshared', 'pages_volatile',
         'stable_node_chains', 'stable_node_dups']

BASEDIR = "/sys/kernel/mm/ksm"


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        definitions = {}
        for metric in ORDER:
            desc = metric.replace('_', ' ').capitalize()
            definitions[metric] = {
                'options': [None, desc, 'Count', "Kernel same-page merging",
                            'ksm.' + metric, 'line'],
                'lines': [
                    [metric]
                ]
            }
        self.definitions = definitions

        if self.check():
            self._files = {}
            for metric in ORDER:
                fn = os.path.join(BASEDIR, metric)
                f = open(fn)
                self._files[metric] = f

    def check(self):
        """Run plugin if the BASEDIR/run file is present
        and contains "1"
        """
        try:
            fn = os.path.join(BASEDIR, "run")
            with open(fn) as f:
                val = int(f.read())
                return val == 1
        except Exception:
            return False

    def _get_data(self):
        """
        Get host metrics
        :return: dict
        """
        data = {}
        for metric, f in self._files.items():
            try:
                f.seek(0)
                val = int(f.read())
                data[metric] = val

            except Exception as e:
                self.error("Error reading KSM %s: %s" % (metric, str(e)))

        if len(data) == 0:
            return None

        return data


if __name__ == '__main__':
    s = Service(
        name="ksm",
        configuration=dict(
            priority=90000,
            retries=60,
            update_every=1
        ),
    )
    s.update(1)
