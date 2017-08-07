# -*- coding: utf-8 -*-
# Description: netdata python modules framework
# Author: Pawel Krupa (paulfantom)

# Remember:
# ALL CODE NEEDS TO BE COMPATIBLE WITH Python > 2.7 and Python > 3.1
# Follow PEP8 as much as it is possible
# "check" and "create" CANNOT be blocking.
# "update" CAN be blocking
# "update" function needs to be fast, so follow:
#   https://wiki.python.org/moin/PythonSpeed/PerformanceTips
# basically:
#  - use local variables wherever it is possible
#  - avoid dots in expressions that are executed many times
#  - use "join()" instead of "+"
#  - use "import" only at the beginning
#
# using ".encode()" in one thread can block other threads as well (only in python2)

import os
import re
import socket
import time
import threading

import urllib3

from glob import glob
from subprocess import Popen, PIPE
from sys import exc_info

try:
    import MySQLdb
    PY_MYSQL = True
except ImportError:
    try:
        import pymysql as MySQLdb
        PY_MYSQL = True
    except ImportError:
        PY_MYSQL = False

import msg


PATH = os.getenv('PATH', '/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin').split(':')
try:
    urllib3.disable_warnings()
except AttributeError:
    msg.error('urllib3: warnings were not disabled')


# class BaseService(threading.Thread):
class SimpleService(threading.Thread):
    """
    Prototype of Service class.
    Implemented basic functionality to run jobs by `python.d.plugin`
    """
    def __init__(self, configuration=None, name=None):
        """
        This needs to be initialized in child classes
        :param configuration: dict
        :param name: str
        """
        threading.Thread.__init__(self)
        self._data_stream = ""
        self.daemon = True
        self.retries = 0
        self.retries_left = 0
        self.priority = 140000
        self.update_every = 1
        self.name = name
        self.override_name = None
        self.chart_name = ""
        self._dimensions = []
        self._charts = []
        self.__chart_set = False
        self.__first_run = True
        self.order = []
        self.definitions = {}
        self._data_from_check = dict()
        if configuration is None:
            self.error("BaseService: no configuration parameters supplied. Cannot create Service.")
            raise RuntimeError
        else:
            self._extract_base_config(configuration)
            self.timetable = {}
            self.create_timetable()

    # --- BASIC SERVICE CONFIGURATION ---

    def _extract_base_config(self, config):
        """
        Get basic parameters to run service
        Minimum config:
            config = {'update_every':1,
                      'priority':100000,
                      'retries':0}
        :param config: dict
        """
        pop = config.pop
        try:
            self.override_name = pop('name')
        except KeyError:
            pass
        self.update_every = int(pop('update_every'))
        self.priority = int(pop('priority'))
        self.retries = int(pop('retries'))
        self.retries_left = self.retries
        self.configuration = config

    def create_timetable(self, freq=None):
        """
        Create service timetable.
        `freq` is optional
        Example:
            timetable = {'last': 1466370091.3767564,
                         'next': 1466370092,
                         'freq': 1}
        :param freq: int
        """
        if freq is None:
            freq = self.update_every
        now = time.time()
        self.timetable = {'last': now,
                          'next': now - (now % freq) + freq,
                          'freq': freq}

    # --- THREAD CONFIGURATION ---

    def _run_once(self):
        """
        Executes self.update(interval) and draws run time chart.
        Return value presents exit status of update()
        :return: boolean
        """
        t_start = float(time.time())
        chart_name = self.chart_name

        since_last = int((t_start - self.timetable['last']) * 1000000)
        if self.__first_run:
            since_last = 0

        if not self.update(since_last):
            self.error("update function failed.")
            return False

        # draw performance graph
        run_time = int((time.time() - t_start) * 1000)
        print("BEGIN netdata.plugin_pythond_%s %s\nSET run_time = %s\nEND\n" %
              (self.chart_name, str(since_last), str(run_time)))

        self.debug(chart_name, "updated in", str(run_time), "ms")
        self.timetable['last'] = t_start
        self.__first_run = False
        return True

    def run(self):
        """
        Runs job in thread. Handles retries.
        Exits when job failed or timed out.
        :return: None
        """
        step = float(self.timetable['freq'])
        penalty = 0
        self.timetable['last'] = float(time.time() - step)
        self.debug("starting data collection - update frequency:", str(step), " retries allowed:", str(self.retries))
        while True:  # run forever, unless something is wrong
            now = float(time.time())
            next = self.timetable['next'] = now - (now % step) + step + penalty

            # it is important to do this in a loop
            # sleep() is interruptable
            while now < next:
                self.debug("sleeping for", str(next - now), "secs to reach frequency of",
                           str(step), "secs, now:", str(now), " next:", str(next), " penalty:", str(penalty))
                time.sleep(next - now)
                now = float(time.time())

            # do the job
            try:
                status = self._run_once()
            except Exception:
                status = False

            if status:
                # it is good
                self.retries_left = self.retries
                penalty = 0
            else:
                # it failed
                self.retries_left -= 1
                if self.retries_left <= 0:
                    if penalty == 0:
                        penalty = float(self.retries * step) / 2
                    else:
                        penalty *= 1.5

                    if penalty > 600:
                        penalty = 600

                    self.retries_left = self.retries
                    self.alert("failed to collect data for " + str(self.retries) +
                               " times - increasing penalty to " + str(penalty) + " sec and trying again")

                else:
                    self.error("failed to collect data - " + str(self.retries_left)
                               + " retries left - penalty: " + str(penalty) + " sec")

    # --- CHART ---

    @staticmethod
    def _format(*args):
        """
        Escape and convert passed arguments.
        :param args: anything
        :return: list
        """
        params = []
        append = params.append
        for p in args:
            if p is None:
                append(p)
                continue
            if type(p) is not str:
                p = str(p)
            if ' ' in p:
                p = "'" + p + "'"
            append(p)
        return params

    def _line(self, instruction, *params):
        """
        Converts *params to string and joins them with one space between every one.
        Result is appended to self._data_stream
        :param params: str/int/float
        """
        tmp = list(map((lambda x: "''" if x is None or len(x) == 0 else x), params))
        self._data_stream += "%s %s\n" % (instruction, str(" ".join(tmp)))

    def chart(self, type_id, name="", title="", units="", family="",
              category="", chart_type="line", priority="", update_every=""):
        """
        Defines a new chart.
        :param type_id: str
        :param name: str
        :param title: str
        :param units: str
        :param family: str
        :param category: str
        :param chart_type: str
        :param priority: int/str
        :param update_every: int/str
        """
        self._charts.append(type_id)

        p = self._format(type_id, name, title, units, family, category, chart_type, priority, update_every)
        self._line("CHART", *p)

    def dimension(self, id, name=None, algorithm="absolute", multiplier=1, divisor=1, hidden=False):
        """
        Defines a new dimension for the chart
        :param id: str
        :param name: str
        :param algorithm: str
        :param multiplier: int/str
        :param divisor: int/str
        :param hidden: boolean
        :return:
        """
        try:
            int(multiplier)
        except TypeError:
            self.error("malformed dimension: multiplier is not a number:", multiplier)
            multiplier = 1
        try:
            int(divisor)
        except TypeError:
            self.error("malformed dimension: divisor is not a number:", divisor)
            divisor = 1
        if name is None:
            name = id
        if algorithm not in ("absolute", "incremental", "percentage-of-absolute-row", "percentage-of-incremental-row"):
            algorithm = "absolute"

        self._dimensions.append(str(id))
        if hidden:
            p = self._format(id, name, algorithm, multiplier, divisor, "hidden")
        else:
            p = self._format(id, name, algorithm, multiplier, divisor)

        self._line("DIMENSION", *p)

    def begin(self, type_id, microseconds=0):
        """
        Begin data set
        :param type_id: str
        :param microseconds: int
        :return: boolean
        """
        if type_id not in self._charts:
            self.error("wrong chart type_id:", type_id)
            return False
        try:
            int(microseconds)
        except TypeError:
            self.error("malformed begin statement: microseconds are not a number:", microseconds)
            microseconds = ""

        self._line("BEGIN", type_id, str(microseconds))
        return True

    def set(self, id, value):
        """
        Set value to dimension
        :param id: str
        :param value: int/float
        :return: boolean
        """
        if id not in self._dimensions:
            self.error("wrong dimension id:", id, "Available dimensions are:", *self._dimensions)
            return False
        try:
            value = str(int(value))
        except TypeError:
            self.error("cannot set non-numeric value:", str(value))
            return False
        self._line("SET", id, "=", str(value))
        self.__chart_set = True
        return True

    def end(self):
        if self.__chart_set:
            self._line("END")
            self.__chart_set = False
        else:
            pos = self._data_stream.rfind("BEGIN")
            self._data_stream = self._data_stream[:pos]

    def commit(self):
        """
        Upload new data to netdata.
        """
        try:
            print(self._data_stream)
        except Exception as e:
            msg.fatal('cannot send data to netdata:', str(e))
        self._data_stream = ""

    # --- ERROR HANDLING ---

    def error(self, *params):
        """
        Show error message on stderr
        """
        msg.error(self.chart_name, *params)

    def alert(self, *params):
        """
        Show error message on stderr
        """
        msg.alert(self.chart_name, *params)

    def debug(self, *params):
        """
        Show debug message on stderr
        """
        msg.debug(self.chart_name, *params)

    def info(self, *params):
        """
        Show information message on stderr
        """
        msg.info(self.chart_name, *params)

    # --- MAIN METHODS ---

    def _get_data(self):
        """
        Get some data
        :return: dict
        """
        return {}

    def check(self):
        """
        check() prototype
        :return: boolean
        """
        self.debug("Module", str(self.__module__), "doesn't implement check() function. Using default.")
        data = self._get_data()

        if data is None:
            self.debug("failed to receive data during check().")
            return False

        if len(data) == 0:
            self.debug("empty data during check().")
            return False

        self.debug("successfully received data during check(): '" + str(data) + "'")
        return True

    def create(self):
        """
        Create charts
        :return: boolean
        """
        data = self._data_from_check or self._get_data()
        if data is None:
            self.debug("failed to receive data during create().")
            return False

        idx = 0
        for name in self.order:
            options = self.definitions[name]['options'] + [self.priority + idx, self.update_every]
            self.chart(self.chart_name + "." + name, *options)
            # check if server has this datapoint
            for line in self.definitions[name]['lines']:
                if line[0] in data:
                    self.dimension(*line)
            idx += 1

        self.commit()
        return True

    def update(self, interval):
        """
        Update charts
        :param interval: int
        :return: boolean
        """
        data = self._get_data()
        if data is None:
            self.debug("failed to receive data during update().")
            return False

        updated = False
        for chart in self.order:
            if self.begin(self.chart_name + "." + chart, interval):
                updated = True
                for dim in self.definitions[chart]['lines']:
                    try:
                        self.set(dim[0], data[dim[0]])
                    except KeyError:
                        pass
                self.end()

        self.commit()
        if not updated:
            self.error("no charts to update")

        return updated

    @staticmethod
    def find_binary(binary):
        try:
            if isinstance(binary, str):
                binary = os.path.basename(binary)
                return next(('/'.join([p, binary]) for p in PATH
                            if os.path.isfile('/'.join([p, binary]))
                            and os.access('/'.join([p, binary]), os.X_OK)))
            return None
        except StopIteration:
            return None

    def _add_new_dimension(self, dimension_id, chart_name, dimension=None, algorithm='incremental',
                           multiplier=1, divisor=1, priority=65000):
        """
        :param dimension_id:
        :param chart_name:
        :param dimension:
        :param algorithm:
        :param multiplier:
        :param divisor:
        :param priority:
        :return:
        """
        if not all([dimension_id not in self._dimensions,
                    chart_name in self.order,
                    chart_name in self.definitions]):
            return
        self._dimensions.append(dimension_id)
        dimension_list = list(map(str, [dimension_id,
                                        dimension if dimension else dimension_id,
                                        algorithm,
                                        multiplier,
                                        divisor]))
        self.definitions[chart_name]['lines'].append(dimension_list)
        add_to_name = self.override_name or self.name
        job_name = ('_'.join([self.__module__, re.sub('\s+', '_', add_to_name)])
                    if add_to_name != 'None' else self.__module__)
        chart = 'CHART {0}.{1} '.format(job_name, chart_name)
        options = '"" "{0}" {1} "{2}" {3} {4} '.format(*self.definitions[chart_name]['options'][1:6])
        other = '{0} {1}\n'.format(priority, self.update_every)
        new_dimension = "DIMENSION {0}\n".format(' '.join(dimension_list))
        print(chart + options + other + new_dimension)


class UrlService(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.url = self.configuration.get('url')
        self.user = self.configuration.get('user')
        self.password = self.configuration.get('pass')
        self.proxy_user = self.configuration.get('proxy_user')
        self.proxy_password = self.configuration.get('proxy_pass')
        self.proxy_url = self.configuration.get('proxy_url')
        self._manager = None

    def __make_headers(self, **header_kw):
        user = header_kw.get('user') or self.user
        password = header_kw.get('pass') or self.password
        proxy_user = header_kw.get('proxy_user') or self.proxy_user
        proxy_password = header_kw.get('proxy_pass') or self.proxy_password
        header_params = dict(keep_alive=True)
        proxy_header_params = dict()
        if user and password:
            header_params['basic_auth'] = '{user}:{password}'.format(user=user,
                                                                     password=password)
        if proxy_user and proxy_password:
            proxy_header_params['proxy_basic_auth'] = '{user}:{password}'.format(user=proxy_user,
                                                                                 password=proxy_password)
        try:
            return urllib3.make_headers(**header_params), urllib3.make_headers(**proxy_header_params)
        except TypeError as error:
            self.error('build_header() error: {error}'.format(error=error))
            return None, None

    def _build_manager(self, **header_kw):
        header, proxy_header = self.__make_headers(**header_kw)
        if header is None or proxy_header is None:
            return None
        proxy_url = header_kw.get('proxy_url') or self.proxy_url
        if proxy_url:
            manager = urllib3.ProxyManager
            params = dict(proxy_url=proxy_url, headers=header, proxy_headers=proxy_header)
        else:
            manager = urllib3.PoolManager
            params = dict(headers=header)
        try:
            return manager(**params)
        except (urllib3.exceptions.ProxySchemeUnknown, TypeError) as error:
            self.error('build_manager() error:', str(error))
            return None

    def _get_raw_data(self, url=None, manager=None):
        """
        Get raw data from http request
        :return: str
        """
        try:
            url = url or self.url
            manager = manager or self._manager
            # TODO: timeout, retries and method hardcoded..
            response = manager.request(method='GET',
                                       url=url,
                                       timeout=1,
                                       retries=1,
                                       headers=manager.headers)
        except (urllib3.exceptions.HTTPError, TypeError, AttributeError) as error:
            self.error('Url: {url}. Error: {error}'.format(url=url, error=error))
            return None
        if response.status == 200:
            return response.data.decode()
        self.debug('Url: {url}. Http response status code: {code}'.format(url=url, code=response.status))
        return None

    def check(self):
        """
        Format configuration data and try to connect to server
        :return: boolean
        """
        if not (self.url and isinstance(self.url, str)):
            self.error('URL is not defined or type is not <str>')
            return False

        self._manager = self._build_manager()
        if not self._manager:
            return False

        try:
            data = self._get_data()
        except Exception as error:
            self.error('_get_data() failed. Url: {url}. Error: {error}'.format(url=self.url, error=error))
            return False

        if isinstance(data, dict) and data:
            self._data_from_check = data
            return True
        self.error('_get_data() returned no data or type is not <dict>')
        return False


class SocketService(SimpleService):
    def __init__(self, configuration=None, name=None):
        self._sock = None
        self._keep_alive = False
        self.host = "localhost"
        self.port = None
        self.unix_socket = None
        self.request = ""
        self.__socket_config = None
        self.__empty_request = "".encode()
        SimpleService.__init__(self, configuration=configuration, name=name)

    def _socketerror(self, message=None):
        if self.unix_socket is not None:
            self.error("unix socket '" + self.unix_socket + "':", message)
        else:
            if self.__socket_config is not None:
                af, socktype, proto, canonname, sa = self.__socket_config
                self.error("socket to '" + str(sa[0]) + "' port " + str(sa[1]) + ":", message)
            else:
                self.error("unknown socket:", message)

    def _connect2socket(self, res=None):
        """
        Connect to a socket, passing the result of getaddrinfo()
        :return: boolean
        """
        if res is None:
            res = self.__socket_config
            if res is None:
                self.error("Cannot create socket to 'None':")
                return False

        af, socktype, proto, canonname, sa = res
        try:
            self.debug("creating socket to '" + str(sa[0]) + "', port " + str(sa[1]))
            self._sock = socket.socket(af, socktype, proto)
        except socket.error as e:
            self.error("Failed to create socket to '" + str(sa[0]) + "', port " + str(sa[1]) + ":", str(e))
            self._sock = None
            self.__socket_config = None
            return False

        try:
            self.debug("connecting socket to '" + str(sa[0]) + "', port " + str(sa[1]))
            self._sock.connect(sa)
        except socket.error as e:
            self.error("Failed to connect to '" + str(sa[0]) + "', port " + str(sa[1]) + ":", str(e))
            self._disconnect()
            self.__socket_config = None
            return False

        self.debug("connected to '" + str(sa[0]) + "', port " + str(sa[1]))
        self.__socket_config = res
        return True

    def _connect2unixsocket(self):
        """
        Connect to a unix socket, given its filename
        :return: boolean
        """
        if self.unix_socket is None:
            self.error("cannot connect to unix socket 'None'")
            return False

        try:
            self.debug("attempting DGRAM unix socket '" + str(self.unix_socket) + "'")
            self._sock = socket.socket(socket.AF_UNIX, socket.SOCK_DGRAM)
            self._sock.connect(self.unix_socket)
            self.debug("connected DGRAM unix socket '" + str(self.unix_socket) + "'")
            return True
        except socket.error as e:
            self.debug("Failed to connect DGRAM unix socket '" + str(self.unix_socket) + "':", str(e))

        try:
            self.debug("attempting STREAM unix socket '" + str(self.unix_socket) + "'")
            self._sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
            self._sock.connect(self.unix_socket)
            self.debug("connected STREAM unix socket '" + str(self.unix_socket) + "'")
            return True
        except socket.error as e:
            self.debug("Failed to connect STREAM unix socket '" + str(self.unix_socket) + "':", str(e))
            self.error("Failed to connect to unix socket '" + str(self.unix_socket) + "':", str(e))
            self._sock = None
            return False

    def _connect(self):
        """
        Recreate socket and connect to it since sockets cannot be reused after closing
        Available configurations are IPv6, IPv4 or UNIX socket
        :return:
        """
        try:
            if self.unix_socket is not None:
                self._connect2unixsocket()

            else:
                if self.__socket_config is not None:
                    self._connect2socket()
                else:
                    for res in socket.getaddrinfo(self.host, self.port, socket.AF_UNSPEC, socket.SOCK_STREAM):
                        if self._connect2socket(res): break

        except Exception as e:
            self._sock = None
            self.__socket_config = None

        if self._sock is not None:
            self._sock.setblocking(0)
            self._sock.settimeout(5)
            self.debug("set socket timeout to: " + str(self._sock.gettimeout()))

    def _disconnect(self):
        """
        Close socket connection
        :return:
        """
        if self._sock is not None:
            try:
                self.debug("closing socket")
                self._sock.shutdown(2)  # 0 - read, 1 - write, 2 - all
                self._sock.close()
            except Exception:
                pass
            self._sock = None

    def _send(self):
        """
        Send request.
        :return: boolean
        """
        # Send request if it is needed
        if self.request != self.__empty_request:
            try:
                self.debug("sending request:", str(self.request))
                self._sock.send(self.request)
            except Exception as e:
                self._socketerror("error sending request:" + str(e))
                self._disconnect()
                return False
        return True

    def _receive(self):
        """
        Receive data from socket
        :return: str
        """
        data = ""
        while True:
            self.debug("receiving response")
            try:
                buf = self._sock.recv(4096)
            except Exception as e:
                self._socketerror("failed to receive response:" + str(e))
                self._disconnect()
                break

            if buf is None or len(buf) == 0:  # handle server disconnect
                if data == "":
                    self._socketerror("unexpectedly disconnected")
                else:
                    self.debug("server closed the connection")
                self._disconnect()
                break

            self.debug("received data:", str(buf))
            data += buf.decode('utf-8', 'ignore')
            if self._check_raw_data(data):
                break

        self.debug("final response:", str(data))
        return data

    def _get_raw_data(self):
        """
        Get raw data with low-level "socket" module.
        :return: str
        """
        if self._sock is None:
            self._connect()
            if self._sock is None:
                return None

        # Send request if it is needed
        if not self._send():
            return None

        data = self._receive()

        if not self._keep_alive:
            self._disconnect()

        return data

    def _check_raw_data(self, data):
        """
        Check if all data has been gathered from socket
        :param data: str
        :return: boolean
        """
        return True

    def _parse_config(self):
        """
        Parse configuration data
        :return: boolean
        """
        if self.name is None or self.name == str(None):
            self.name = ""
        else:
            self.name = str(self.name)

        try:
            self.unix_socket = str(self.configuration['socket'])
        except (KeyError, TypeError):
            self.debug("No unix socket specified. Trying TCP/IP socket.")
            self.unix_socket = None
            try:
                self.host = str(self.configuration['host'])
            except (KeyError, TypeError):
                self.debug("No host specified. Using: '" + self.host + "'")
            try:
                self.port = int(self.configuration['port'])
            except (KeyError, TypeError):
                self.debug("No port specified. Using: '" + str(self.port) + "'")

        try:
            self.request = str(self.configuration['request'])
        except (KeyError, TypeError):
            self.debug("No request specified. Using: '" + str(self.request) + "'")

        self.request = self.request.encode()

    def check(self):
        self._parse_config()
        return SimpleService.check(self)


class LogService(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.log_path = self.configuration.get('path')
        self.__glob_path = self.log_path
        self._last_position = 0
        self.retries = 100000  # basically always retry
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
            self.error("No path to log specified")
            return None

        if all([self._find_recent_log_file(),
                os.access(self.log_path, os.R_OK),
                os.path.isfile(self.log_path)]):
            return True
        self.error("Cannot access %s" % self.log_path)
        return False

    def create(self):
        # set cursor at last byte of log file
        self._last_position = os.path.getsize(self.log_path)
        status = SimpleService.create(self)
        # self._last_position = 0
        return status


class ExecutableService(SimpleService):

    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.command = None

    def _get_raw_data(self, stderr=False):
        """
        Get raw data from executed command
        :return: <list>
        """
        try:
            p = Popen(self.command, stdout=PIPE, stderr=PIPE)
        except Exception as error:
            self.error("Executing command", " ".join(self.command), "resulted in error:", str(error))
            return None
        data = list()
        std = p.stderr if stderr else p.stdout
        for line in std.readlines():
            data.append(line.decode())

        return data or None

    def check(self):
        """
        Parse basic configuration, check if command is whitelisted and is returning values
        :return: <boolean>
        """
        # Preference: 1. "command" from configuration file 2. "command" from plugin (if specified)
        if 'command' in self.configuration:
            self.command = self.configuration['command']

        # "command" must be: 1.not None 2. type <str>
        if not (self.command and isinstance(self.command, str)):
            self.error('Command is not defined or command type is not <str>')
            return False

        # Split "command" into: 1. command <str> 2. options <list>
        command, opts = self.command.split()[0], self.command.split()[1:]

        # Check for "bad" symbols in options. No pipes, redirects etc. TODO: what is missing?
        bad_opts = set(''.join(opts)) & set(['&', '|', ';', '>', '<'])
        if bad_opts:
            self.error("Bad command argument(s): %s" % bad_opts)
            return False

        # Find absolute path ('echo' => '/bin/echo')
        if '/' not in command:
            command = self.find_binary(command)
            if not command:
                self.error('Can\'t locate "%s" binary in PATH(%s)' % (self.command, PATH))
                return False
        # Check if binary exist and executable
        else:
            if not (os.path.isfile(command) and os.access(command, os.X_OK)):
                self.error('"%s" is not a file or not executable' % command)
                return False

        self.command = [command] + opts if opts else [command]

        try:
            data = self._get_data()
        except Exception as error:
            self.error('_get_data() failed. Command: %s. Error: %s' % (self.command, error))
            return False

        if isinstance(data, dict) and data:
            # We need this for create() method. No reason to execute get_data() again if result is not empty dict()
            self._data_from_check = data
            return True
        else:
            self.error("Command", str(self.command), "returned no data")
            return False


class MySQLService(SimpleService):

    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.__connection = None
        self.__conn_properties = dict()
        self.extra_conn_properties = dict()
        self.__queries = self.configuration.get('queries', dict())
        self.queries = dict()

    def __connect(self):
        try:
            connection = MySQLdb.connect(connect_timeout=self.update_every, **self.__conn_properties)
        except (MySQLdb.MySQLError, TypeError, AttributeError) as error:
            return None, str(error)
        else:
            return connection, None

    def check(self):
        def get_connection_properties(conf, extra_conf):
            properties = dict()
            if conf.get('user'):
                properties['user'] = conf['user']
            if conf.get('pass'):
                properties['passwd'] = conf['pass']
            if conf.get('socket'):
                properties['unix_socket'] = conf['socket']
            elif conf.get('host'):
                properties['host'] = conf['host']
                properties['port'] = int(conf.get('port', 3306))
            elif conf.get('my.cnf'):
                if MySQLdb.__name__ == 'pymysql':
                    self.error('"my.cnf" parsing is not working for pymysql')
                else:
                    properties['read_default_file'] = conf['my.cnf']
            if isinstance(extra_conf, dict) and extra_conf:
                properties.update(extra_conf)

            return properties or None

        def is_valid_queries_dict(raw_queries, log_error):
            """
            :param raw_queries: dict:
            :param log_error: function:
            :return: dict or None

            raw_queries is valid when: type <dict> and not empty after is_valid_query(for all queries)
            """
            def is_valid_query(query):
                return all([isinstance(query, str),
                            query.startswith(('SELECT', 'select', 'SHOW', 'show'))])

            if hasattr(raw_queries, 'keys') and raw_queries:
                valid_queries = dict([(n, q) for n, q in raw_queries.items() if is_valid_query(q)])
                bad_queries = set(raw_queries) - set(valid_queries)

                if bad_queries:
                    log_error('Removed query(s): %s' % bad_queries)
                return valid_queries
            else:
                log_error('Unsupported "queries" format. Must be not empty <dict>')
                return None

        if not PY_MYSQL:
            self.error('MySQLdb or PyMySQL module is needed to use mysql.chart.py plugin')
            return False

        # Preference: 1. "queries" from the configuration file 2. "queries" from the module
        self.queries = self.__queries or self.queries
        # Check if "self.queries" exist, not empty and all queries are in valid format
        self.queries = is_valid_queries_dict(self.queries, self.error)
        if not self.queries:
            return None

        # Get connection properties
        self.__conn_properties = get_connection_properties(self.configuration, self.extra_conn_properties)
        if not self.__conn_properties:
            self.error('Connection properties are missing')
            return False

        # Create connection to the database
        self.__connection, error = self.__connect()
        if error:
            self.error('Can\'t establish connection to MySQL: %s' % error)
            return False

        try:
            data = self._get_data() 
        except Exception as error:
            self.error('_get_data() failed. Error: %s' % error)
            return False

        if isinstance(data, dict) and data:
            # We need this for create() method
            self._data_from_check = data
            return True
        else:
            self.error("_get_data() returned no data or type is not <dict>")
            return False
    
    def _get_raw_data(self, description=None):
        """
        Get raw data from MySQL server
        :return: dict: fetchall() or (fetchall(), description)
        """

        if not self.__connection:
            self.__connection, error = self.__connect()
            if error:
                return None

        raw_data = dict()
        queries = dict(self.queries)
        try:
            with self.__connection as cursor:
                for name, query in queries.items():
                    try:
                        cursor.execute(query)
                    except (MySQLdb.ProgrammingError, MySQLdb.OperationalError) as error:
                        if self.__is_error_critical(err_class=exc_info()[0], err_text=str(error)):
                            raise RuntimeError
                        self.error('Removed query: %s[%s]. Error: %s'
                                   % (name, query, error))
                        self.queries.pop(name)
                        continue
                    else:
                        raw_data[name] = (cursor.fetchall(), cursor.description) if description else cursor.fetchall()
            self.__connection.commit()
        except (MySQLdb.MySQLError, RuntimeError, TypeError, AttributeError):
            self.__connection.close()
            self.__connection = None
            return None
        else:
            return raw_data or None

    @staticmethod
    def __is_error_critical(err_class, err_text):
        return err_class == MySQLdb.OperationalError and all(['denied' not in err_text,
                                                              'Unknown column' not in err_text])
