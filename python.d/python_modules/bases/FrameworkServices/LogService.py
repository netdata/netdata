# -*- coding: utf-8 -*-
# Description:
# Author: Pawel Krupa (paulfantom)
# Author: Ilya Mashchenko (l2isbad)
# SPDX-License-Identifier: GPL-3.0-or-later

from glob import glob
import os

from bases.FrameworkServices.SimpleService import SimpleService


class LogService(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.log_path = self.configuration.get('path')
        self.__glob_path = self.log_path
        self._last_position = 0
        self.__re_find = dict(current=0, run=0, maximum=60)

    def _get_raw_data(self):
        """
        Get log lines since last poll
        :return: list
        """
        lines = list()
        try:
            if self.__re_find['current'] == self.__re_find['run']:
                self._find_recent_log_file()
            size = os.path.getsize(self.log_path)
            if size == self._last_position:
                self.__re_find['current'] += 1
                return list()  # return empty list if nothing has changed
            elif size < self._last_position:
                self._last_position = 0  # read from beginning if file has shrunk

            with open(self.log_path) as fp:
                fp.seek(self._last_position)
                for line in fp:
                    lines.append(line)
                self._last_position = fp.tell()
                self.__re_find['current'] = 0
        except (OSError, IOError) as error:
            self.__re_find['current'] += 1
            self.error(str(error))

        return lines or None

    def _find_recent_log_file(self):
        """
        :return:
        """
        self.__re_find['run'] = self.__re_find['maximum']
        self.__re_find['current'] = 0
        self.__glob_path = self.__glob_path or self.log_path  # workaround for modules w/o config files
        path_list = glob(self.__glob_path)
        if path_list:
            self.log_path = max(path_list)
            return True
        return False

    def check(self):
        """
        Parse basic configuration and check if log file exists
        :return: boolean
        """
        if not self.log_path:
            self.error('No path to log specified')
            return None

        if self._find_recent_log_file() and os.access(self.log_path, os.R_OK) and os.path.isfile(self.log_path):
            return True
        self.error('Cannot access {0}'.format(self.log_path))
        return False

    def create(self):
        # set cursor at last byte of log file
        self._last_position = os.path.getsize(self.log_path)
        status = SimpleService.create(self)
        return status
