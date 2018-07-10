# -*- coding: utf-8 -*-
# Description:
# Author: Ilya Mashchenko (l2isbad)
# SPDX-License-Identifier: GPL-3.0+

import os

PATH = os.getenv('PATH', '/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin').split(':')

CHART_BEGIN = 'BEGIN {0} {1}\n'
CHART_CREATE = "CHART {0} '{1}' '{2}' '{3}' '{4}' '{5}' {6} {7} {8}\n"
DIMENSION_CREATE = "DIMENSION '{0}' '{1}' {2} {3} {4} '{5}'\n"
DIMENSION_SET = "SET '{0}' = {1}\n"


def setdefault_values(config, base_dict):
    for key, value in base_dict.items():
        config.setdefault(key, value)
    return config


def run_and_exit(func):
    def wrapper(*args, **kwargs):
        func(*args, **kwargs)
        exit(1)
    return wrapper


def on_try_except_finally(on_except=(None, ), on_finally=(None, )):
    except_func = on_except[0]
    finally_func = on_finally[0]

    def decorator(func):
        def wrapper(*args, **kwargs):
            try:
                func(*args, **kwargs)
            except Exception:
                if except_func:
                    except_func(*on_except[1:])
            finally:
                if finally_func:
                    finally_func(*on_finally[1:])
        return wrapper
    return decorator


def static_vars(**kwargs):
    def decorate(func):
        for k in kwargs:
            setattr(func, k, kwargs[k])
        return func
    return decorate


@on_try_except_finally(on_except=(exit, 1))
def safe_print(*msg):
    """
    :param msg:
    :return:
    """
    print(''.join(msg))


def find_binary(binary):
    """
    :param binary: <str>
    :return:
    """
    for directory in PATH:
        binary_name = '/'.join([directory, binary])
        if os.path.isfile(binary_name) and os.access(binary_name, os.X_OK):
            return binary_name
    return None


def read_last_line(f):
    with open(f, 'rb') as opened:
        opened.seek(-2, 2)
        while opened.read(1) != b'\n':
            opened.seek(-2, 1)
            if opened.tell() == 0:
                break
        result = opened.readline()
    return result.decode()


class OldVersionCompatibility:

    def __init__(self):
        self._data_stream = str()

    def begin(self, type_id, microseconds=0):
        """
        :param type_id: <str>
        :param microseconds: <str> or <int>: must be a digit
        :return:
        """
        self._data_stream += CHART_BEGIN.format(type_id, microseconds)

    def set(self, dim_id, value):
        """
        :param dim_id: <str>
        :param value: <int> or <str>: must be a digit
        :return:
        """
        self._data_stream += DIMENSION_SET.format(dim_id, value)

    def end(self):
        self._data_stream += 'END\n'

    def chart(self, type_id, name='', title='', units='', family='', category='', chart_type='line',
              priority='', update_every=''):
        """
        :param type_id: <str>
        :param name: <str>
        :param title: <str>
        :param units: <str>
        :param family: <str>
        :param category: <str>
        :param chart_type: <str>
        :param priority: <str> or <int>
        :param update_every: <str> or <int>
        :return:
        """
        self._data_stream += CHART_CREATE.format(type_id, name, title, units,
                                                 family, category, chart_type,
                                                 priority, update_every)

    def dimension(self, dim_id, name=None, algorithm="absolute", multiplier=1, divisor=1, hidden=False):
        """
        :param dim_id: <str>
        :param name: <str> or None
        :param algorithm: <str>
        :param multiplier: <str> or <int>: must be a digit
        :param divisor: <str> or <int>: must be a digit
        :param hidden: <str>: literally "hidden" or ""
        :return:
        """
        self._data_stream += DIMENSION_CREATE.format(dim_id, name or dim_id, algorithm,
                                                     multiplier, divisor, hidden or str())

    @on_try_except_finally(on_except=(exit, 1))
    def commit(self):
        print(self._data_stream)
        self._data_stream = str()
