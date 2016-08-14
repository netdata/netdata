# -*- coding: utf-8 -*-
# Description: haproxy netdata python.d module
# Author: Pawel Krupa (paulfantom)

from base import SocketService, UrlService
import csv

# default module values (can be overridden per job in `config`)
# update_every = 2
priority = 60000
retries = 60

# charts order (can be overridden if you want less charts, or different order)
ORDER = ['qcur', 'scur', 'bin', 'bout']
POSITION = [2, 4, 8, 9]

CHARTS = {
    'qcur': {
        'options': ["", "Current queue", '', '', '', 'line'],
        'lines': [
            ['name', None, 'incremental']
        ]},
    'scur': {
        'options': ["", "Current session rate", '', '', '', 'line'],
        'lines': [
            ['name', None, 'incremental']
        ]},
    'bin': {
        'options': ["", "Bytes in", 'kilobytes/s', '', '', 'line'],
        'lines': [
            ['name', None, 'incremental', 1, 1024]
        ]},
    'bout': {
        'options': ["", "Bytes out", 'kilobytes/s', '', '', 'line'],
        'lines': [
            ['name', None, 'incremental', 1, 1024]
        ]},
}


class Service(SocketService, UrlService):
    def __init__(self, configuration=None, name=None):
        self.use_socket = None
        if 'unix_socket' in configuration:
            SocketService.__init__(self, configuration=configuration, name=name)
            self.request = "show stat\r\n"
            self.use_socket = True
        else:
            UrlService.__init__(self, configuration=configuration, name=name)
            if not self.url.endswith("/;csv;norefresh"):
                self.url += "/;csv;norefresh"
            self.url = "http://localhost:9000/stat"
            self.use_socket = False

        # self.order and self.definitions are created with _create_definitions method
        # self.order = ORDER
        # self.definitions = CHARTS
        self.order = []
        self.definitions = {}

    def _get_parsed_data(self):
        """
        Retrieve and parse raw data
        :return: dict
        """
        try:
            if self.use_socket:
                raw = SocketService._get_raw_data(self)
            else:
                raw = UrlService._get_raw_data(self)
        except (ValueError, AttributeError):
            return None

        try:
            return [row for row in csv.reader(raw.splitlines(), delimiter=',')]
        except Exception as e:
            self.debug(str(e))
            return None

    def _get_data(self):
        """
        Format data
        :return: dict
        """
        parsed = self._get_parsed_data()
        if parsed is None or len(parsed) == 0:
            return None

        data = {}
        for node in parsed[1:]:
            try:
                prefix = node[0] + "_" + node[1]
            except IndexError:
                continue
            for i in range(len(ORDER)):
                try:
                    data[prefix + "_" + ORDER[i]] = int(node[POSITION[i]])
                except ValueError:
                    pass

        if len(data) == 0:
            return None
        return data

    def _check_raw_data(self, data):
        # FIXME
        return True

    def _create_definitions(self):
        try:
            data = self._get_parsed_data()[1:]
        except TypeError:
            return False

        # create order
        all_pxnames = []
        all_svnames = {}
        last_pxname = ""
        for node in data:
            try:
                pxname = node[0]
                svname = node[1]
            except IndexError:
                continue
            if pxname != last_pxname:
                all_pxnames.append(pxname)
                all_svnames[pxname] = [svname]
                for key in ORDER:
                    # order entry consists of pxname, "_", and column name (like qcur)
                    self.order.append(pxname + "_" + key)
            else:
                all_svnames[pxname].append(svname)
            last_pxname = pxname

        # create definitions
        for pxname in all_pxnames:
            for name in ORDER:
                options = list(CHARTS[name]['options'])
                options[3] = pxname
                options[4] = pxname + "." + name
                line_template = CHARTS[name]['lines'][0]
                lines = []
                # omit_first = False
                # if len(all_svnames[pxname]) > 1:
                #     omit_first = True
                for svname in all_svnames[pxname]:
                    # if omit_first:
                    #     omit_first = False
                    #     continue
                    tmp = list(line_template)
                    tmp[0] = pxname + "_" + svname + "_" + name
                    tmp[1] = svname
                    lines.append(tmp)
                self.definitions[pxname + "_" + name] = {'options': options, 'lines': lines}

        return True

    def check(self):
        if self.use_socket:
            SocketService.check(self)
        else:
            UrlService.check(self)

        try:
            return self._create_definitions()
        except Exception as e:
            self.debug(str(e))
            return False
