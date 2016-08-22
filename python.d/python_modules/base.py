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

import time
# import sys
import os
import socket
import select
try:
    import urllib.request as urllib2
except ImportError:
    import urllib2

from subprocess import Popen, PIPE

import threading
import msg


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
        t_start = time.time()
        timetable = self.timetable
        chart_name = self.chart_name
        # check if it is time to execute job update() function
        if timetable['next'] > t_start:
            self.debug(chart_name, "will be run in", str(int((timetable['next'] - t_start) * 1000)), "ms")
            return True

        since_last = int((t_start - timetable['last']) * 1000000)
        self.debug(chart_name,
                   "ready to run, after", str(int((t_start - timetable['last']) * 1000)),
                   "ms (update_every:", str(timetable['freq'] * 1000),
                   "ms, latency:", str(int((t_start - timetable['next']) * 1000)), "ms")
        if self.__first_run:
            since_last = 0
        if not self.update(since_last):
            self.error("update function failed.")
            return False
        t_end = time.time()
        self.timetable['next'] = t_end - (t_end % timetable['freq']) + timetable['freq']
        # draw performance graph
        run_time = str(int((t_end - t_start) * 1000))
        # noinspection SqlNoDataSourceInspection
        print("BEGIN netdata.plugin_pythond_%s %s\nSET run_time = %s\nEND\n" %
              (self.chart_name, str(since_last), run_time))
        # sys.stdout.write("BEGIN netdata.plugin_pythond_%s %s\nSET run_time = %s\nEND\n" %
        #                  (self.chart_name, str(since_last), run_time))

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
        self.timetable['last'] = time.time()
        while True:  # run forever, unless something is wrong
            try:
                status = self._run_once()
            except Exception as e:
                self.error("Something wrong: ", str(e))
                return
            if status:  # handle retries if update failed
                time.sleep(self.timetable['next'] - time.time())
                self.retries_left = self.retries
            else:
                self.retries_left -= 1
                if self.retries_left <= 0:
                    self.error("no more retries. Exiting")
                    return
                else:
                    time.sleep(self.timetable['freq'])

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
        print(self._data_stream)
        self._data_stream = ""

    # --- ERROR HANDLING ---

    def error(self, *params):
        """
        Show error message on stderr
        """
        msg.error(self.chart_name, *params)

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
        if self._get_data() is None or len(self._get_data()) == 0:
            return False
        else:
            return True

    def create(self):
        """
        Create charts
        :return: boolean
        """
        data = self._get_data()
        if data is None:
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
            self.debug("_get_data() returned no data")
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


class UrlService(SimpleService):
    # TODO add support for https connections
    def __init__(self, configuration=None, name=None):
        self.url = ""
        self.user = None
        self.password = None
        self.proxies = {}
        SimpleService.__init__(self, configuration=configuration, name=name)

    def __add_openers(self):
        # TODO add error handling
        self.opener = urllib2.build_opener()

        # Proxy handling
        # TODO currently self.proxies isn't parsed from configuration file
        # if len(self.proxies) > 0:
        #     for proxy in self.proxies:
        #         url = proxy['url']
        #         # TODO test this:
        #         if "user" in proxy and "pass" in proxy:
        #             if url.lower().startswith('https://'):
        #                 url = 'https://' + proxy['user'] + ':' + proxy['pass'] + '@' + url[8:]
        #             else:
        #                 url = 'http://' + proxy['user'] + ':' + proxy['pass'] + '@' + url[7:]
        #         # FIXME move proxy auth to sth like this:
        #         #     passman = urllib2.HTTPPasswordMgrWithDefaultRealm()
        #         #     passman.add_password(None, url, proxy['user'], proxy['password'])
        #         #     opener.add_handler(urllib2.HTTPBasicAuthHandler(passman))
        #
        #         if url.lower().startswith('https://'):
        #             opener.add_handler(urllib2.ProxyHandler({'https': url}))
        #         else:
        #             opener.add_handler(urllib2.ProxyHandler({'https': url}))

        # HTTP Basic Auth
        if self.user is not None and self.password is not None:
            passman = urllib2.HTTPPasswordMgrWithDefaultRealm()
            passman.add_password(None, self.url, self.user, self.password)
            self.opener.add_handler(urllib2.HTTPBasicAuthHandler(passman))
            self.debug("Enabling HTTP basic auth")

        #urllib2.install_opener(opener)

    def _get_raw_data(self):
        """
        Get raw data from http request
        :return: str
        """
        raw = None
        try:
            f = self.opener.open(self.url, timeout=self.update_every * 2)
            # f = urllib2.urlopen(self.url, timeout=self.update_every * 2)
        except Exception as e:
            self.error(str(e))
            return None

        try:
            raw = f.read().decode('utf-8')
        except Exception as e:
            self.error(str(e))
        finally:
            f.close()
        return raw

    def check(self):
        """
        Format configuration data and try to connect to server
        :return: boolean
        """
        if self.name is None or self.name == str(None):
            self.name = 'local'
            self.chart_name += "_" + self.name
        else:
            self.name = str(self.name)
        try:
            self.url = str(self.configuration['url'])
        except (KeyError, TypeError):
            pass
        try:
            self.user = str(self.configuration['user'])
        except (KeyError, TypeError):
            pass
        try:
            self.password = str(self.configuration['pass'])
        except (KeyError, TypeError):
            pass

        self.__add_openers()

        test = self._get_data()
        if test is None or len(test) == 0:
            return False
        else:
            return True


class SocketService(SimpleService):
    def __init__(self, configuration=None, name=None):
        self._sock = None
        self._keep_alive = False
        self.host = "localhost"
        self.port = None
        self.unix_socket = None
        self.request = ""
        self.__socket_config = None
        SimpleService.__init__(self, configuration=configuration, name=name)

    def _connect(self):
        """
        Recreate socket and connect to it since sockets cannot be reused after closing
        Available configurations are IPv6, IPv4 or UNIX socket
        :return:
        """
        try:
            if self.unix_socket is None:
                if self.__socket_config is None:
                    # establish ipv6 or ipv4 connection.
                    for res in socket.getaddrinfo(self.host, self.port, socket.AF_UNSPEC, socket.SOCK_STREAM):
                        try:
                            # noinspection SpellCheckingInspection
                            af, socktype, proto, canonname, sa = res
                            self._sock = socket.socket(af, socktype, proto)
                        except socket.error as e:
                            self.debug("Cannot create socket:", str(e))
                            self._sock = None
                            continue
                        try:
                            self._sock.connect(sa)
                        except socket.error as e:
                            self.debug("Cannot connect to socket:", str(e))
                            self._disconnect()
                            continue
                        self.__socket_config = res
                        break
                else:
                    # connect to socket with previously established configuration
                    try:
                        af, socktype, proto, canonname, sa = self.__socket_config
                        self._sock = socket.socket(af, socktype, proto)
                        self._sock.connect(sa)
                    except socket.error as e:
                        self.debug("Cannot create or connect to socket:", str(e))
                        self._disconnect()
            else:
                # connect to unix socket
                try:
                    self._sock = socket.socket(socket.AF_UNIX, socket.SOCK_DGRAM)
                    self._sock.connect(self.unix_socket)
                except socket.error:
                    self._sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
                    self._sock.connect(self.unix_socket)

        except Exception as e:
            self.error(str(e),
                       "Cannot create socket with following configuration: host:", str(self.host),
                       "port:", str(self.port),
                       "socket:", str(self.unix_socket))
            self._sock = None
        self._sock.setblocking(0)

    def _disconnect(self):
        """
        Close socket connection
        :return:
        """
        try:
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
        if self.request != "".encode():
            try:
                self._sock.send(self.request)
            except Exception as e:
                self._disconnect()
                self.error(str(e),
                           "used configuration: host:", str(self.host),
                           "port:", str(self.port),
                           "socket:", str(self.unix_socket))
                return False
        return True

    def _receive(self):
        """
        Receive data from socket
        :return: str
        """
        data = ""
        while True:
            try:
                ready_to_read, _, in_error = select.select([self._sock], [], [], 5)
            except Exception as e:
                self.debug("SELECT", str(e))
                self._disconnect()
                break
            if len(ready_to_read) > 0:
                buf = self._sock.recv(4096)
                if len(buf) == 0 or buf is None:  # handle server disconnect
                    break
                data += buf.decode()
                if self._check_raw_data(data):
                    break
            else:
                self.error("Socket timed out.")
                self._disconnect()
                break

        return data

    def _get_raw_data(self):
        """
        Get raw data with low-level "socket" module.
        :return: str
        """
        if self._sock is None:
            self._connect()

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
        return SimpleService.check(self)


class LogService(SimpleService):
    def __init__(self, configuration=None, name=None):
        self.log_path = ""
        self._last_position = 0
        # self._log_reader = None
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.retries = 100000  # basically always retry

    def _get_raw_data(self):
        """
        Get log lines since last poll
        :return: list
        """
        lines = []
        try:
            if os.path.getsize(self.log_path) < self._last_position:
                self._last_position = 0  # read from beginning if file has shrunk
            elif os.path.getsize(self.log_path) == self._last_position:
                self.debug("Log file hasn't changed. No new data.")
                return []  # return empty list if nothing has changed
            with open(self.log_path, "r") as fp:
                fp.seek(self._last_position)
                for i, line in enumerate(fp):
                    lines.append(line)
                self._last_position = fp.tell()
        except Exception as e:
            self.error(str(e))

        if len(lines) != 0:
            return lines
        else:
            self.error("No data collected.")
            return None

    def check(self):
        """
        Parse basic configuration and check if log file exists
        :return: boolean
        """
        if self.name is not None or self.name != str(None):
            self.name = ""
        else:
            self.name = str(self.name)
        try:
            self.log_path = str(self.configuration['path'])
        except (KeyError, TypeError):
            self.error("No path to log specified. Using: '" + self.log_path + "'")

        if os.access(self.log_path, os.R_OK):
            return True
        else:
            self.error("Cannot access file: '" + self.log_path + "'")
            return False

    def create(self):
        # set cursor at last byte of log file
        self._last_position = os.path.getsize(self.log_path)
        status = SimpleService.create(self)
        # self._last_position = 0
        return status


class ExecutableService(SimpleService):
    bad_substrings = ('&', '|', ';', '>', '<')

    def __init__(self, configuration=None, name=None):
        self.command = ""
        SimpleService.__init__(self, configuration=configuration, name=name)

    def _get_raw_data(self):
        """
        Get raw data from executed command
        :return: str
        """
        try:
            p = Popen(self.command, stdout=PIPE, stderr=PIPE)
        except Exception as e:
            self.error("Executing command", self.command, "resulted in error:", str(e))
            return None
        data = []
        for line in p.stdout.readlines():
            data.append(str(line.decode()))

        if len(data) == 0:
            self.error("No data collected.")
            return None

        return data

    def check(self):
        """
        Parse basic configuration, check if command is whitelisted and is returning values
        :return: boolean
        """
        if self.name is not None or self.name != str(None):
            self.name = ""
        else:
            self.name = str(self.name)
        try:
            self.command = str(self.configuration['command'])
        except (KeyError, TypeError):
            self.error("No command specified. Using: '" + self.command + "'")
        command = self.command.split(' ')

        for arg in command[1:]:
            if any(st in arg for st in self.bad_substrings):
                self.error("Bad command argument:" + " ".join(self.command[1:]))
                return False
        # test command and search for it in /usr/sbin or /sbin when failed
        base = command[0].split('/')[-1]
        if self._get_raw_data() is None:
            for prefix in ['/sbin/', '/usr/sbin/']:
                command[0] = prefix + base
                if os.path.isfile(command[0]):
                    break
        self.command = command
        if self._get_data() is None or len(self._get_data()) == 0:
            self.error("Command", self.command, "returned no data")
            return False
        return True
