# -*- coding: utf-8 -*-
# Description:
# Author: Ilya Mashchenko (ilyam8)
# SPDX-License-Identifier: GPL-3.0-or-later

import logging
import os
import stat
import traceback

from sys import exc_info

try:
    from time import monotonic as time
except ImportError:
    from time import time

from bases.collection import on_try_except_finally, unicode_str

LOGGING_LEVELS = {
    'DISABLE': 99,
    'ERROR': 40,
    'WARNING': 30,
    'INFO': 20,
    'DEBUG': 10,
    'NOTSET': 0,
}


def is_stderr_connected_to_journal():
    journal_stream = os.environ.get("JOURNAL_STREAM")
    if not journal_stream:
        return False

    colon_index = journal_stream.find(":")
    if colon_index <= 0:
        return False

    device, inode = journal_stream[:colon_index], journal_stream[colon_index + 1:]

    try:
        device_number, inode_number = os.fstat(2)[stat.ST_DEV], os.fstat(2)[stat.ST_INO]
    except OSError:
        return False

    return str(device_number) == device and str(inode_number) == inode


is_journal = is_stderr_connected_to_journal()

DEFAULT_LOG_LINE_FORMAT = '%(asctime)s: %(name)s %(levelname)s : %(message)s'
PYTHON_D_LOG_LINE_FORMAT = '%(asctime)s: %(name)s %(levelname)s: %(module_name)s[%(job_name)s] : %(message)s'

if is_journal:
    DEFAULT_LOG_LINE_FORMAT = '%(name)s %(levelname)s : %(message)s'
    PYTHON_D_LOG_LINE_FORMAT = '%(name)s %(levelname)s: %(module_name)s[%(job_name)s] : %(message)s '

DEFAULT_LOG_TIME_FORMAT = '%Y-%m-%d %H:%M:%S'
PYTHON_D_LOG_NAME = 'python.d'


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


class BaseLogger(object):
    def __init__(
            self,
            logger_name,
            log_fmt=DEFAULT_LOG_LINE_FORMAT,
            date_fmt=DEFAULT_LOG_TIME_FORMAT,
            handler=logging.StreamHandler,
    ):
        self.logger = logging.getLogger(logger_name)
        self._muted = False
        if not self.has_handlers():
            self.severity = 'INFO'
            self.logger.addHandler(handler())
            self.set_formatter(fmt=log_fmt, date_fmt=date_fmt)

    def __repr__(self):
        return '<Logger: {name})>'.format(name=self.logger.name)

    def set_formatter(self, fmt, date_fmt=DEFAULT_LOG_TIME_FORMAT):
        if self.has_handlers():
            self.logger.handlers[0].setFormatter(logging.Formatter(fmt=fmt, datefmt=date_fmt))

    def has_handlers(self):
        return self.logger.handlers

    @property
    def severity(self):
        return self.logger.getEffectiveLevel()

    @severity.setter
    def severity(self, level):
        if level in LOGGING_LEVELS:
            self.logger.setLevel(LOGGING_LEVELS[level])

    def _log(self, level, *msg, **kwargs):
        if not self._muted:
            self.logger.log(level, ' '.join(map(unicode_str, msg)), **kwargs)

    def debug(self, *msg, **kwargs):
        self._log(logging.DEBUG, *msg, **kwargs)

    def info(self, *msg, **kwargs):
        self._log(logging.INFO, *msg, **kwargs)

    def warning(self, *msg, **kwargs):
        self._log(logging.WARN, *msg, **kwargs)

    def error(self, *msg, **kwargs):
        self._log(logging.ERROR, *msg, **kwargs)

    def alert(self, *msg, **kwargs):
        self._log(logging.CRITICAL, *msg, **kwargs)

    @on_try_except_finally(on_finally=(exit, 1))
    def fatal(self, *msg, **kwargs):
        self._log(logging.CRITICAL, *msg, **kwargs)

    def mute(self):
        self._muted = True

    def unmute(self):
        self._muted = False


class PythonDLogger(object):
    def __init__(
            self,
            logger_name=PYTHON_D_LOG_NAME,
            log_fmt=PYTHON_D_LOG_LINE_FORMAT,
    ):
        self.logger = BaseLogger(logger_name, log_fmt=log_fmt)
        self.module_name = 'plugin'
        self.job_name = 'main'

    _LOG_TRACEBACK = False

    @property
    def log_traceback(self):
        return PythonDLogger._LOG_TRACEBACK

    @log_traceback.setter
    def log_traceback(self, value):
        PythonDLogger._LOG_TRACEBACK = value

    def debug(self, *msg):
        self.logger.debug(*msg, extra={
            'module_name': self.module_name,
            'job_name': self.job_name or self.module_name,
        })

    def info(self, *msg):
        self.logger.info(*msg, extra={
            'module_name': self.module_name,
            'job_name': self.job_name or self.module_name,
        })

    def warning(self, *msg):
        self.logger.warning(*msg, extra={
            'module_name': self.module_name,
            'job_name': self.job_name or self.module_name,
        })

    @add_traceback
    def error(self, *msg):
        self.logger.error(*msg, extra={
            'module_name': self.module_name,
            'job_name': self.job_name or self.module_name,
        })

    @add_traceback
    def alert(self, *msg):
        self.logger.alert(*msg, extra={
            'module_name': self.module_name,
            'job_name': self.job_name or self.module_name,
        })

    def fatal(self, *msg):
        self.logger.fatal(*msg, extra={
            'module_name': self.module_name,
            'job_name': self.job_name or self.module_name,
        })
