# -*- coding: utf-8 -*-
# Description:
# Author: Ilya Mashchenko (l2isbad)
# SPDX-License-Identifier: GPL-3.0-or-later

import logging
import traceback

from sys import exc_info

try:
    from time import monotonic as time
except ImportError:
    from time import time

from bases.collection import on_try_except_finally


LOGGING_LEVELS = {'CRITICAL': 50,
                  'ERROR': 40,
                  'WARNING': 30,
                  'INFO': 20,
                  'DEBUG': 10,
                  'NOTSET': 0}

DEFAULT_LOG_LINE_FORMAT = '%(asctime)s: %(name)s %(levelname)s : %(message)s'
DEFAULT_LOG_TIME_FORMAT = '%Y-%m-%d %H:%M:%S'

PYTHON_D_LOG_LINE_FORMAT = '%(asctime)s: %(name)s %(levelname)s: %(module_name)s: %(job_name)s: %(message)s'
PYTHON_D_LOG_NAME = 'python.d'


def limiter(log_max_count=30, allowed_in_seconds=60):
    def on_decorator(func):

        def on_call(*args):
            current_time = args[0]._runtime_counters.start_mono
            lc = args[0]._logger_counters

            if lc.logged and lc.logged % log_max_count == 0:
                if current_time - lc.time_to_compare <= allowed_in_seconds:
                    lc.dropped += 1
                    return
                lc.time_to_compare = current_time

            lc.logged += 1
            func(*args)

        return on_call
    return on_decorator


def add_traceback(func):
    def on_call(*args):
        self = args[0]

        if not self.log_traceback:
            func(*args)
        else:
            if exc_info()[0]:
                func(*args)
                func(self, traceback.format_exc())
            else:
                func(*args)

    return on_call


class LoggerCounters:
    def __init__(self):
        self.logged = 0
        self.dropped = 0
        self.time_to_compare = time()

    def __repr__(self):
        return 'LoggerCounter(logged: {logged}, dropped: {dropped})'.format(logged=self.logged,
                                                                            dropped=self.dropped)


class BaseLogger(object):
    def __init__(self, logger_name, log_fmt=DEFAULT_LOG_LINE_FORMAT, date_fmt=DEFAULT_LOG_TIME_FORMAT,
                 handler=logging.StreamHandler):
        """
        :param logger_name: <str>
        :param log_fmt: <str>
        :param date_fmt: <str>
        :param handler: <logging handler>
        """
        self.logger = logging.getLogger(logger_name)
        if not self.has_handlers():
            self.severity = 'INFO'
            self.logger.addHandler(handler())
            self.set_formatter(fmt=log_fmt, date_fmt=date_fmt)

    def __repr__(self):
        return '<Logger: {name})>'.format(name=self.logger.name)

    def set_formatter(self, fmt, date_fmt=DEFAULT_LOG_TIME_FORMAT):
        """
        :param fmt: <str>
        :param date_fmt: <str>
        :return:
        """
        if self.has_handlers():
            self.logger.handlers[0].setFormatter(logging.Formatter(fmt=fmt, datefmt=date_fmt))

    def has_handlers(self):
        return self.logger.handlers

    @property
    def severity(self):
        return self.logger.getEffectiveLevel()

    @severity.setter
    def severity(self, level):
        """
        :param level: <str> or <int>
        :return:
        """
        if level in LOGGING_LEVELS:
            self.logger.setLevel(LOGGING_LEVELS[level])

    def debug(self, *msg, **kwargs):
        self.logger.debug(' '.join(map(str, msg)), **kwargs)

    def info(self, *msg, **kwargs):
        self.logger.info(' '.join(map(str, msg)), **kwargs)

    def warning(self, *msg, **kwargs):
        self.logger.warning(' '.join(map(str, msg)), **kwargs)

    def error(self, *msg, **kwargs):
        self.logger.error(' '.join(map(str, msg)), **kwargs)

    def alert(self, *msg,  **kwargs):
        self.logger.critical(' '.join(map(str, msg)), **kwargs)

    @on_try_except_finally(on_finally=(exit, 1))
    def fatal(self, *msg, **kwargs):
        self.logger.critical(' '.join(map(str, msg)), **kwargs)


class PythonDLogger(object):
    def __init__(self, logger_name=PYTHON_D_LOG_NAME, log_fmt=PYTHON_D_LOG_LINE_FORMAT):
        """
        :param logger_name: <str>
        :param log_fmt: <str>
        """
        self.logger = BaseLogger(logger_name, log_fmt=log_fmt)
        self.module_name = 'plugin'
        self.job_name = 'main'
        self._logger_counters = LoggerCounters()

    _LOG_TRACEBACK = False

    @property
    def log_traceback(self):
        return PythonDLogger._LOG_TRACEBACK

    @log_traceback.setter
    def log_traceback(self, value):
        PythonDLogger._LOG_TRACEBACK = value

    def debug(self, *msg):
        self.logger.debug(*msg, extra={'module_name': self.module_name,
                                       'job_name': self.job_name or self.module_name})

    def info(self, *msg):
        self.logger.info(*msg, extra={'module_name': self.module_name,
                                      'job_name': self.job_name or self.module_name})

    def warning(self, *msg):
        self.logger.warning(*msg, extra={'module_name': self.module_name,
                                         'job_name': self.job_name or self.module_name})

    @add_traceback
    def error(self, *msg):
        self.logger.error(*msg, extra={'module_name': self.module_name,
                                       'job_name': self.job_name or self.module_name})

    @add_traceback
    def alert(self, *msg):
        self.logger.alert(*msg, extra={'module_name': self.module_name,
                                       'job_name': self.job_name or self.module_name})

    def fatal(self, *msg):
        self.logger.fatal(*msg, extra={'module_name': self.module_name,
                                       'job_name': self.job_name or self.module_name})


class PythonDLimitedLogger(PythonDLogger):
    @limiter()
    def info(self, *msg):
        PythonDLogger.info(self, *msg)

    @limiter()
    def warning(self, *msg):
        PythonDLogger.warning(self, *msg)

    @limiter()
    def error(self, *msg):
        PythonDLogger.error(self, *msg)

    @limiter()
    def alert(self, *msg):
        PythonDLogger.alert(self, *msg)
