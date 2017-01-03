# -*- coding: utf-8 -*-
# Description: bind rndc netdata python.d module
# Author: l2isbad

from base import SimpleService
from re import compile, findall
from os.path import getsize, isfile, split
from os import access as is_accessible, R_OK
from subprocess import Popen

priority = 60000
retries = 60
update_every = 30

DIRECTORIES = ['/bin/', '/usr/bin/', '/sbin/', '/usr/sbin/']


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.named_stats_path = self.configuration.get('named_stats_path', '/var/log/bind/named.stats')
        self.regex_values = compile(r'([0-9]+) ([^\n]+)')
        # self.options = ['Incoming Requests', 'Incoming Queries', 'Outgoing Queries',
        # 'Name Server Statistics', 'Zone Maintenance Statistics', 'Resolver Statistics',
        # 'Cache DB RRsets', 'Socket I/O Statistics']
        self.options = ['Name Server Statistics']
        self.regex_options = [r'(%s(?= \+\+)) \+\+([^\+]+)' % option for option in self.options]
        try:
            self.rndc = [''.join([directory, 'rndc']) for directory in DIRECTORIES
                         if isfile(''.join([directory, 'rndc']))][0]
        except IndexError:
            self.rndc = False

    def check(self):

        # We cant start without 'rndc' command 
        if not self.rndc:
            self.error('Command "rndc" not found')
            return False

        # We cant if stats file is not exist or not readable by netdata user
        if not is_accessible(self.named_stats_path, R_OK):
            self.error('Cannot access file %s' % self.named_stats_path)
            return False

        size_before = getsize(self.named_stats_path)
        run_rndc = Popen([self.rndc, 'stats'], shell=False)
        run_rndc.wait()
        size_after = getsize(self.named_stats_path)

        # We cant start if netdata user has no permissions to run 'rndc stats'
        if not run_rndc.returncode:
            # 'rndc' was found, stats file is exist and readable and we can run 'rndc stats'. Lets go!
            self.create_charts()
            
            # BIND APPEND dump on every run 'rndc stats'
            # that is why stats file size can be VERY large if update_interval too small
            dump_size_24hr = round(86400 / self.update_every * (int(size_after) - int(size_before)) / 1048576, 3)
            
            # If update_every too small we should WARN user
            if self.update_every < 30:
                self.info('Update_every %s is NOT recommended for use. Increase the value to > 30' % self.update_every)
            
            self.info('With current update_interval it will be + %s MB every 24hr. '
                      'Don\'t forget to create logrotate conf file for %s' % (dump_size_24hr, self.named_stats_path))

            self.info('Plugin was started successfully.')

            return True
        else:
            self.error('Not enough permissions to run "%s stats"' % self.rndc)
            return False

    def _get_raw_data(self):

        """
        Run 'rndc stats' and read last dump from named.stats
        :return: tuple(
                       file.read() obj,
                       named.stats file size
                      )
        """

        try:
            current_size = getsize(self.named_stats_path)
        except OSError:
            return None, None
        
        run_rndc = Popen([self.rndc, 'stats'], shell=False)
        run_rndc.wait()

        if run_rndc.returncode:     
            return None, None

        try:
            with open(self.named_stats_path) as bind_rndc:
                bind_rndc.seek(current_size)
                result = bind_rndc.read()
        except OSError:
            return None, None
        else:
            return result, current_size

    def _get_data(self):

        """
        Parse data from _get_raw_data()
        :return: dict
        """

        raw_data, size = self._get_raw_data()

        if raw_data is None:
            return None

        rndc_stats = dict()
        
        for regex in self.regex_options:
            rndc_stats.update({k: [(y, int(x)) for x, y in self.regex_values.findall(v)]
                               for k, v in findall(regex, raw_data)})
        
        nms = dict(rndc_stats.get('Name Server Statistics', []))

        to_netdata = dict()

        to_netdata['requests'] = sum([v for k, v in nms.items() if 'request' in k and 'received' in k])
        to_netdata['responses'] = sum([v for k, v in nms.items() if 'responses' in k and 'sent' in k])
        to_netdata['success'] = nms.get('queries resulted in successful answer', 0)
        to_netdata['auth_answer'] = nms.get('queries resulted in authoritative answer', 0)
        to_netdata['nonauth_answer'] = nms.get('queries resulted in non authoritative answer', 0)
        to_netdata['nxrrset'] = nms.get('queries resulted in nxrrset', 0)
        to_netdata['failure'] = sum([nms.get('queries resulted in SERVFAIL', 0), nms.get('other query failures', 0)])
        to_netdata['nxdomain'] = nms.get('queries resulted in NXDOMAIN', 0)
        to_netdata['recursion'] = nms.get('queries caused recursion', 0)
        to_netdata['duplicate'] = nms.get('duplicate queries received', 0)
        to_netdata['rejections'] = nms.get('recursive queries rejected', 0)
        to_netdata['stats_size'] = size
        
        return to_netdata

    def create_charts(self):

        self.order = ['stats_size', 'bind_stats']
        self.definitions = {
            'bind_stats': {
                'options': [None, 'Name Server Statistics', 'stats', 'NS Statistics', 'bind_rndc.stats', 'line'],
                'lines': [
                         ["requests", None, "incremental"], ["responses", None, "incremental"],
                         ["success", None, "incremental"], ["auth_answer", None, "incremental"],
                         ["nonauth_answer", None, "incremental"], ["nxrrset", None, "incremental"],
                         ["failure", None, "incremental"], ["nxdomain", None, "incremental"],
                         ["recursion", None, "incremental"], ["duplicate", None, "incremental"],
                         ["rejections", None, "incremental"]
                         ]},
                'stats_size': {
                'options': [None, '%s file size' % split(self.named_stats_path)[1].capitalize(), 'megabyte',
                            '%s size' % split(self.named_stats_path)[1].capitalize(), 'bind_rndc.size', 'line'],
                'lines': [
                         ["stats_size", None, "absolute", 1, 1048576]
                        ]}
                     }
