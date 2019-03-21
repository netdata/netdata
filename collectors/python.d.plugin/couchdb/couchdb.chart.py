# -*- coding: utf-8 -*-
# Description: couchdb netdata python.d module
# Author: wohali <wohali@apache.org>
# Thanks to ilyam8 for good examples :)
# SPDX-License-Identifier: GPL-3.0-or-later

from collections import namedtuple, defaultdict
from json import loads
from threading import Thread
from socket import gethostbyname, gaierror

try:
    from queue import Queue
except ImportError:
    from Queue import Queue

from bases.FrameworkServices.UrlService import UrlService


update_every = 1


METHODS = namedtuple('METHODS', ['get_data', 'url', 'stats'])

OVERVIEW_STATS = [
    'couchdb.database_reads.value',
    'couchdb.database_writes.value',
    'couchdb.httpd.view_reads.value',
    'couchdb.httpd_request_methods.COPY.value',
    'couchdb.httpd_request_methods.DELETE.value',
    'couchdb.httpd_request_methods.GET.value',
    'couchdb.httpd_request_methods.HEAD.value',
    'couchdb.httpd_request_methods.OPTIONS.value',
    'couchdb.httpd_request_methods.POST.value',
    'couchdb.httpd_request_methods.PUT.value',
    'couchdb.httpd_status_codes.200.value',
    'couchdb.httpd_status_codes.201.value',
    'couchdb.httpd_status_codes.202.value',
    'couchdb.httpd_status_codes.204.value',
    'couchdb.httpd_status_codes.206.value',
    'couchdb.httpd_status_codes.301.value',
    'couchdb.httpd_status_codes.302.value',
    'couchdb.httpd_status_codes.304.value',
    'couchdb.httpd_status_codes.400.value',
    'couchdb.httpd_status_codes.401.value',
    'couchdb.httpd_status_codes.403.value',
    'couchdb.httpd_status_codes.404.value',
    'couchdb.httpd_status_codes.405.value',
    'couchdb.httpd_status_codes.406.value',
    'couchdb.httpd_status_codes.409.value',
    'couchdb.httpd_status_codes.412.value',
    'couchdb.httpd_status_codes.413.value',
    'couchdb.httpd_status_codes.414.value',
    'couchdb.httpd_status_codes.415.value',
    'couchdb.httpd_status_codes.416.value',
    'couchdb.httpd_status_codes.417.value',
    'couchdb.httpd_status_codes.500.value',
    'couchdb.httpd_status_codes.501.value',
    'couchdb.open_os_files.value',
    'couch_replicator.jobs.running.value',
    'couch_replicator.jobs.pending.value',
    'couch_replicator.jobs.crashed.value',
]

SYSTEM_STATS = [
    'context_switches',
    'run_queue',
    'ets_table_count',
    'reductions',
    'memory.atom',
    'memory.atom_used',
    'memory.binary',
    'memory.code',
    'memory.ets',
    'memory.other',
    'memory.processes',
    'io_input',
    'io_output',
    'os_proc_count',
    'process_count',
    'internal_replication_jobs'
]

DB_STATS = [
    'doc_count',
    'doc_del_count',
    'sizes.file',
    'sizes.external',
    'sizes.active'
]

ORDER = [
    'activity',
    'request_methods',
    'response_codes',
    'active_tasks',
    'replicator_jobs',
    'open_files',
    'db_sizes_file',
    'db_sizes_external',
    'db_sizes_active',
    'db_doc_counts',
    'db_doc_del_counts',
    'erlang_memory',
    'erlang_proc_counts',
    'erlang_peak_msg_queue',
    'erlang_reductions'
]

CHARTS = {
    'activity': {
        'options': [None, 'Overall Activity', 'requests/s',
                    'dbactivity', 'couchdb.activity', 'stacked'],
        'lines': [
            ['couchdb_database_reads', 'DB reads', 'incremental'],
            ['couchdb_database_writes', 'DB writes', 'incremental'],
            ['couchdb_httpd_view_reads', 'View reads', 'incremental']
        ]
    },
    'request_methods': {
        'options': [None, 'HTTP request methods', 'requests/s',
                    'httptraffic', 'couchdb.request_methods',
                    'stacked'],
        'lines': [
            ['couchdb_httpd_request_methods_COPY', 'COPY', 'incremental'],
            ['couchdb_httpd_request_methods_DELETE', 'DELETE', 'incremental'],
            ['couchdb_httpd_request_methods_GET', 'GET', 'incremental'],
            ['couchdb_httpd_request_methods_HEAD', 'HEAD', 'incremental'],
            ['couchdb_httpd_request_methods_OPTIONS', 'OPTIONS',
                'incremental'],
            ['couchdb_httpd_request_methods_POST', 'POST', 'incremental'],
            ['couchdb_httpd_request_methods_PUT', 'PUT', 'incremental']
        ]
    },
    'response_codes': {
        'options': [None, 'HTTP response status codes', 'responses/s',
                    'httptraffic', 'couchdb.response_codes',
                    'stacked'],
        'lines': [
            ['couchdb_httpd_status_codes_200', '200 OK', 'incremental'],
            ['couchdb_httpd_status_codes_201', '201 Created', 'incremental'],
            ['couchdb_httpd_status_codes_202', '202 Accepted', 'incremental'],
            ['couchdb_httpd_status_codes_2xx', 'Other 2xx Success',
                'incremental'],
            ['couchdb_httpd_status_codes_3xx', '3xx Redirection',
                'incremental'],
            ['couchdb_httpd_status_codes_4xx', '4xx Client error',
                'incremental'],
            ['couchdb_httpd_status_codes_5xx', '5xx Server error',
                'incremental']
        ]
    },
    'open_files': {
        'options': [None, 'Open files', 'files', 'ops', 'couchdb.open_files', 'line'],
        'lines': [
            ['couchdb_open_os_files', '# files', 'absolute']
        ]
    },
    'active_tasks': {
        'options': [None, 'Active task breakdown', 'tasks', 'ops', 'couchdb.active_tasks', 'stacked'],
        'lines': [
            ['activetasks_indexer', 'Indexer', 'absolute'],
            ['activetasks_database_compaction', 'DB Compaction', 'absolute'],
            ['activetasks_replication', 'Replication', 'absolute'],
            ['activetasks_view_compaction', 'View Compaction', 'absolute']
        ]
    },
    'replicator_jobs': {
        'options': [None, 'Replicator job breakdown', 'jobs', 'ops', 'couchdb.replicator_jobs', 'stacked'],
        'lines': [
            ['couch_replicator_jobs_running', 'Running', 'absolute'],
            ['couch_replicator_jobs_pending', 'Pending', 'absolute'],
            ['couch_replicator_jobs_crashed', 'Crashed', 'absolute'],
            ['internal_replication_jobs', 'Internal replication jobs',
             'absolute']
        ]
    },
    'erlang_memory': {
        'options': [None, 'Erlang VM memory usage', 'B', 'erlang', 'couchdb.erlang_vm_memory', 'stacked'],
        'lines': [
            ['memory_atom', 'atom', 'absolute'],
            ['memory_binary', 'binaries', 'absolute'],
            ['memory_code', 'code', 'absolute'],
            ['memory_ets', 'ets', 'absolute'],
            ['memory_processes', 'procs', 'absolute'],
            ['memory_other', 'other', 'absolute']
        ]
    },
    'erlang_reductions': {
        'options': [None, 'Erlang reductions', 'count', 'erlang', 'couchdb.reductions', 'line'],
        'lines': [
            ['reductions', 'reductions', 'incremental']
        ]
    },
    'erlang_proc_counts': {
        'options': [None, 'Process counts', 'count', 'erlang', 'couchdb.proccounts', 'line'],
        'lines': [
            ['os_proc_count', 'OS procs', 'absolute'],
            ['process_count', 'erl procs', 'absolute']
        ]
    },
    'erlang_peak_msg_queue': {
        'options': [None, 'Peak message queue size', 'count', 'erlang', 'couchdb.peakmsgqueue',
                    'line'],
        'lines': [
            ['peak_msg_queue', 'peak size', 'absolute']
        ]
    },
    # Lines for the following are added as part of check()
    'db_sizes_file': {
        'options': [None, 'Database sizes (file)', 'KiB', 'perdbstats', 'couchdb.db_sizes_file', 'line'],
        'lines': []
    },
    'db_sizes_external': {
        'options': [None, 'Database sizes (external)', 'KiB', 'perdbstats', 'couchdb.db_sizes_external', 'line'],
        'lines': []
    },
    'db_sizes_active': {
        'options': [None, 'Database sizes (active)', 'KiB', 'perdbstats', 'couchdb.db_sizes_active', 'line'],
        'lines': []
    },
    'db_doc_counts': {
        'options': [None, 'Database # of docs', 'docs',
                    'perdbstats', 'couchdb_db_doc_count', 'line'],
        'lines': []
    },
    'db_doc_del_counts': {
        'options': [None, 'Database # of deleted docs', 'docs', 'perdbstats', 'couchdb_db_doc_del_count', 'line'],
        'lines': []
    }
}


class Service(UrlService):
    def __init__(self, configuration=None, name=None):
        UrlService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.host = self.configuration.get('host', '127.0.0.1')
        self.port = self.configuration.get('port', 5984)
        self.node = self.configuration.get('node', 'couchdb@127.0.0.1')
        self.scheme = self.configuration.get('scheme', 'http')
        self.user = self.configuration.get('user')
        self.password = self.configuration.get('pass')
        try:
            self.dbs = self.configuration.get('databases').split(' ')
        except (KeyError, AttributeError):
            self.dbs = list()

    def check(self):
        if not (self.host and self.port):
            self.error('Host is not defined in the module configuration file')
            return False
        try:
            self.host = gethostbyname(self.host)
        except gaierror as error:
            self.error(str(error))
            return False
        self.url = '{scheme}://{host}:{port}'.format(scheme=self.scheme,
                                                     host=self.host,
                                                     port=self.port)
        stats = self.url + '/_node/{node}/_stats'.format(node=self.node)
        active_tasks = self.url + '/_active_tasks'
        system = self.url + '/_node/{node}/_system'.format(node=self.node)
        self.methods = [METHODS(get_data=self._get_overview_stats,
                                url=stats,
                                stats=OVERVIEW_STATS),
                        METHODS(get_data=self._get_active_tasks_stats,
                                url=active_tasks,
                                stats=None),
                        METHODS(get_data=self._get_overview_stats,
                                url=system,
                                stats=SYSTEM_STATS),
                        METHODS(get_data=self._get_dbs_stats,
                                url=self.url,
                                stats=DB_STATS)]
        # must initialise manager before using _get_raw_data
        self._manager = self._build_manager()
        self.dbs = [db for db in self.dbs
                    if self._get_raw_data(self.url + '/' + db)]
        for db in self.dbs:
            self.definitions['db_sizes_file']['lines'].append(
                ['db_'+db+'_sizes_file', db, 'absolute', 1, 1000]
            )
            self.definitions['db_sizes_external']['lines'].append(
                ['db_'+db+'_sizes_external', db, 'absolute', 1, 1000]
            )
            self.definitions['db_sizes_active']['lines'].append(
                ['db_'+db+'_sizes_active', db, 'absolute', 1, 1000]
            )
            self.definitions['db_doc_counts']['lines'].append(
                ['db_'+db+'_doc_count', db, 'absolute']
            )
            self.definitions['db_doc_del_counts']['lines'].append(
                ['db_'+db+'_doc_del_count', db, 'absolute']
            )
        return UrlService.check(self)

    def _get_data(self):
        threads = list()
        queue = Queue()
        result = dict()

        for method in self.methods:
            th = Thread(target=method.get_data,
                        args=(queue, method.url, method.stats))
            th.start()
            threads.append(th)

        for thread in threads:
            thread.join()
            result.update(queue.get())

        # self.info('couchdb result = ' + str(result))
        return result or None

    def _get_overview_stats(self, queue, url, stats):
        raw_data = self._get_raw_data(url)
        if not raw_data:
            return queue.put(dict())
        data = loads(raw_data)
        to_netdata = self._fetch_data(raw_data=data, metrics=stats)
        if 'message_queues' in data:
            to_netdata['peak_msg_queue'] = get_peak_msg_queue(data)
        return queue.put(to_netdata)

    def _get_active_tasks_stats(self, queue, url, _):
        taskdict = defaultdict(int)
        taskdict["activetasks_indexer"] = 0
        taskdict["activetasks_database_compaction"] = 0
        taskdict["activetasks_replication"] = 0
        taskdict["activetasks_view_compaction"] = 0
        raw_data = self._get_raw_data(url)
        if not raw_data:
            return queue.put(dict())
        data = loads(raw_data)
        for task in data:
            taskdict["activetasks_" + task["type"]] += 1
        return queue.put(dict(taskdict))

    def _get_dbs_stats(self, queue, url, stats):
        to_netdata = {}
        for db in self.dbs:
            raw_data = self._get_raw_data(url + '/' + db)
            if not raw_data:
                continue
            data = loads(raw_data)
            for metric in stats:
                value = data
                metrics_list = metric.split('.')
                try:
                    for m in metrics_list:
                        value = value[m]
                except KeyError as e:
                    self.debug('cannot process ' + metric + ' for ' + db
                               + ": " + str(e))
                    continue
                metric_name = 'db_{0}_{1}'.format(db, '_'.join(metrics_list))
                to_netdata[metric_name] = value
        return queue.put(to_netdata)

    def _fetch_data(self, raw_data, metrics):
        data = dict()
        for metric in metrics:
            value = raw_data
            metrics_list = metric.split('.')
            try:
                for m in metrics_list:
                    value = value[m]
            except KeyError as e:
                self.debug('cannot process ' + metric + ': ' + str(e))
                continue
            # strip off .value from end of stat
            if metrics_list[-1] == 'value':
                metrics_list = metrics_list[:-1]
            # sum up 3xx/4xx/5xx
            if metrics_list[0:2] == ['couchdb', 'httpd_status_codes'] and \
                    int(metrics_list[2]) > 202:
                metrics_list[2] = '{0}xx'.format(int(metrics_list[2]) // 100)
                if '_'.join(metrics_list) in data:
                    data['_'.join(metrics_list)] += value
                else:
                    data['_'.join(metrics_list)] = value
            else:
                data['_'.join(metrics_list)] = value
        return data


def get_peak_msg_queue(data):
    maxsize = 0
    queues = data['message_queues']
    for queue in iter(queues.values()):
        if isinstance(queue, dict) and 'count' in queue:
            value = queue['count']
        elif isinstance(queue, int):
            value = queue
        else:
            continue
        maxsize = max(maxsize, value)
    return maxsize
