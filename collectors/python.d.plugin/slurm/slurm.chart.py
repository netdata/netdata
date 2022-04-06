# -*- coding: utf-8 -*-
# Description: slurm netdata python.d module
# Author: Max Berrendorf (mberr)
# SPDX-License-Identifier: GPL-3.0-or-later

from bases.FrameworkServices.ExecutableService import ExecutableService

SQUEUE_COMMAND = "squeue -o %all"

ORDER = [
    "slurmjobs",
]

CHARTS = {
    "slurmjobs": {
        "options": [None, "Slurm Queue", "job", "queue", "slurm.total", "line"],
        "lines": [
            # ['total', None, 'absolute'],
            ["running", None, "absolute"],
            ["pending", None, "absolute"],
        ],
    },
}


class Service(ExecutableService):
    def __init__(self, configuration=None, name=None):
        ExecutableService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.command = SQUEUE_COMMAND

    def _get_data(self):
        """
        Format data received from shell command
        :return: dict
        """
        try:
            # _get_raw_data returns a list of lines
            raw_lines = self._get_raw_data()
            # first line is the header
            # comment: we could use pandas to easily parse this, and perform the subsequent operations
            header, *data = [line.strip().split("|") for line in raw_lines]
            # build dictionaries for each line
            data = [dict(zip(header, line)) for line in data]
            return {
                "total": len(data),
                "running": sum(1 for entry in data if entry["STATE"] == "RUNNING"),
                "pending": sum(1 for entry in data if entry["STATE"] == "PENDING"),
            }
        except (ValueError, AttributeError):
            return None
