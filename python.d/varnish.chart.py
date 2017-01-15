# -*- coding: utf-8 -*-
# Description:  varnish netdata python.d module
# Author: l2isbad

from base import SimpleService
from re import compile
from os import access as is_executable, X_OK
from subprocess import Popen, PIPE


# default module values (can be overridden per job in `config`)
# update_every = 2
priority = 60000
retries = 60

ORDER = ['hit_rate', 'chit_rate', 'request_rate', 'transfer_rates', 'session', 'backend_traffic', 'memory_usage', 'bad', 'uptime']
EXTRA_ORDER = ['hit_rate','chit_rate', 'request_rate', 'transfer_rates', 'session', 'backend_traffic', 'bad',
               'objects', 'threads', 'memory_usage', 'objects_per_objhead', 'losthdr', 'hcb', 'esi', 'session_herd',
               'shm_writes', 'shm', 'allocations', 'vcl', 'bans', 'bans_lurker', 'expunge', 'lru', 'gzip', 'uptime']

CHARTS = {'allocations': 
             {'lines': [['sm_nreq', None, 'incremental', 1, 1],
                       ['sma_nreq', None, 'incremental', 1, 1],
                       ['sms_nreq', None, 'incremental', 1, 1]],
              'options': [None, 'Memory allocation requests', 'units', 'Extra charts', 'varnish.alloc','line']},
          'backend_traffic': 
             {'lines': [['backend_conn_bt', 'conn', 'incremental', 1, 1],
                       ['backend_unhealthy', 'unhealthy', 'incremental', 1, 1],
                       ['backend_busy', 'busy', 'incremental', 1, 1],
                       ['backend_fail', 'fail', 'incremental', 1, 1],
                       ['backend_reuse', 'reuse', 'incremental', 1, 1],
                       ['backend_recycle', 'resycle', 'incremental', 1, 1],
                       ['backend_toolate', 'toolate', 'incremental', 1, 1],
                       ['backend_retry', 'retry', 'incremental', 1, 1],
                       ['backend_req', 'req', 'incremental', 1, 1]],
              'options': [None, 'Backend health', 'units', 'Backend health', 'varnish.backend_traf', 'line']},
          'bad': 
             {'lines': [['sess_drop_b', None, 'incremental', 1, 1],
                       ['backend_unhealthy_b', None, 'incremental', 1, 1],
                       ['fetch_failed', None, 'incremental', 1, 1],
                       ['backend_busy_b', None, 'incremental', 1, 1],
                       ['threads_failed_b', None, 'incremental', 1, 1],
                       ['threads_limited_b', None, 'incremental', 1, 1],
                       ['threads_destroyed_b', None, 'incremental', 1, 1],
                       ['thread_queue_len', None, 'absolute', 1, 1],
                       ['losthdr_b', None, 'incremental', 1, 1],
                       ['esi_errors_b', None, 'incremental', 1, 1],
                       ['esi_warnings_b', None, 'incremental', 1, 1],
                       ['sess_fail_b', None, 'incremental', 1, 1],
                       ['sess_pipe_overflow_b', None, 'incremental', 1, 1]],
              'options': [None, 'Misbehavior', 'units', 'Problems summary', 'varnish.bad', 'line']},
          'bans': 
             {'lines': [['bans', None, 'absolute', 1, 1],
                       ['bans_added', 'added', 'incremental', 1, 1],
                       ['bans_deleted', 'deleted', 'incremental', 1, 1],
                       ['bans_completed', 'completed', 'absolute', 1, 1],
                       ['bans_obj', 'obj', 'absolute', 1, 1],
                       ['bans_req', 'req', 'absolute', 1, 1],
                       ['bans_tested', 'tested', 'incremental', 1, 1],
                       ['bans_obj_killed', 'obj_killed', 'incremental', 1, 1],
                       ['bans_tests_tested', 'tests_tested', 'incremental', 1, 1],
                       ['bans_dups', 'dups', 'absolute', 1, 1],
                       ['bans_persisted_bytes', 'pers_bytes', 'absolute', 1, 1],
                       ['bans_persisted_fragmentation', 'pers_fragmentation', 'absolute', 1, 1]],
              'options': [None, 'Bans', 'units', 'Extra charts', 'varnish.bans', 'line']},
          'bans_lurker': 
             {'lines': [['bans_lurker_tested', 'tested', 'incremental', 1, 1],
                       ['bans_lurker_tests_tested', 'tests_tested', 'incremental', 1, 1],
                       ['bans_lurker_obj_killed', 'obj_killed', 'incremental', 1, 1],
                       ['bans_lurker_contention', 'contention', 'incremental', 1, 1]],
              'options': [None, 'Ban Lurker', 'units', 'Extra charts', 'varnish.bans_lurker', 'line']},
          'esi':
             {'lines': [['esi_parse', None, 'incremental', 1, 1],
                       ['esi_errors', None, 'incremental', 1, 1],
                       ['esi_warnings', None, 'incremental', 1, 1]],
              'options': [None, 'ESI', 'units', 'Extra charts', 'varnish.esi', 'line']},
          'expunge':
             {'lines': [['n_expired', None, 'incremental', 1, 1],
                       ['n_lru_nuked_e', None, 'incremental', 1, 1]],
              'options': [None, 'Object expunging', 'units', 'Extra charts', 'varnish.expunge', 'line']},
          'gzip': 
             {'lines': [['n_gzip', None, 'incremental', 1, 1],
                       ['n_gunzip', None, 'incremental', 1, 1]],
              'options': [None, 'GZIP activity', 'units', 'Extra charts', 'varnish.gzip', 'line']},
          'hcb': 
             {'lines': [['hcb_nolock', 'nolock', 'incremental', 1, 1],
                       ['hcb_lock', 'lock', 'incremental', 1, 1],
                       ['hcb_insert', 'insert', 'incremental', 1, 1]],
              'options': [None, 'Critbit data', 'units', 'Extra charts', 'varnish.hcb', 'line']},
          'hit_rate': 
             {'lines': [['cache_hit_perc', 'hit', 'absolute', 1, 100],
                       ['cache_miss_perc', 'miss', 'absolute', 1, 100],
                       ['cache_hitpass_perc', 'hitpass', 'absolute', 1, 100]],
              'options': [None, 'All history hit rate ratio','percent', 'Cache perfomance', 'varnish.hit_rate', 'stacked']},
          'chit_rate': 
             {'lines': [['cache_hit_cperc', 'hit', 'absolute', 1, 100],
                       ['cache_miss_cperc', 'miss', 'absolute', 1, 100],
                       ['cache_hitpass_cperc', 'hitpass', 'absolute', 1, 100]],
              'options': [None, 'Current poll hit rate ratio','percent', 'Cache perfomance', 'varnish.chit_rate', 'stacked']},
          'losthdr': 
             {'lines': [['losthdr', None, 'incremental', 1, 1]],
              'options': [None, 'HTTP Header overflows', 'units', 'Extra charts', 'varnish.losthdr', 'line']},
          'lru':
             {'lines': [['n_lru_nuked', 'nuked', 'incremental', 1, 1],
                       ['n_lru_moved', 'moved', 'incremental', 1, 1]],
              'options': [None, 'LRU activity', 'units', 'Extra charts', 'varnish.lru', 'line']},
          'memory_usage': 
             {'lines': [['s0.g_space', 'available', 'absolute', 1, 1048576],
                       ['s0.g_bytes', 'allocated', 'absolute', -1, 1048576]],
              'options': [None, 'Memory usage', 'megabytes', 'Memory usage', 'varnish.memory_usage', 'stacked']},
          'objects': 
             {'lines': [['n_object', 'object', 'absolute', 1, 1],
                       ['n_objectcore', 'objectcore', 'absolute', 1, 1],
                       ['n_vampireobject', 'vampireobject, ''absolute', 1, 1],
                       ['n_objecthead', 'objecthead', 'absolute', 1, 1]],
              'options': [None, 'Number of objects', 'units', 'Extra charts', 'varnish.objects', 'line']},
          'objects_per_objhead': 
             {'lines': [['obj_per_objhead', 'per_objhead', 'absolute', 1, 100]],
              'options': [None, 'Objects per objecthead', 'units', 'Extra charts', 'varnish.objects_per_objhead', 'line']},
          'request_rate': 
             {'lines': [['sess_conn_rr', None, 'incremental', 1, 1],
                       ['client_req', None, 'incremental', 1, 1],
                       ['cache_hit', None, 'incremental', 1, 1],
                       ['cache_hitpass', None, 'incremental', 1, 1],
                       ['cache_miss', None, 'incremental', 1, 1],
                       ['backend_conn', None, 'incremental', 1, 1],
                       ['backend_unhealthy', None, 'incremental', 1, 1],
                       ['s_pipe', None, 'incremental', 1, 1],
                       ['s_pass', None, 'incremental', 1, 1]],
              'options': [None, 'Request rates', 'units', 'Varnish statistics', 'varnish.request_rate', 'line']},
          'session': 
             {'lines': [['sess_conn', 'conn', 'incremental', 1, 1],
                       ['sess_drop', 'drop', 'incremental', 1, 1],
                       ['sess_fail', 'fail', 'incremental', 1, 1],
                       ['sess_pipe_overflow', 'pipe_overflow', 'incremental', 1, 1],
                       ['sess_queued', 'queued', 'incremental', 1, 1],
                       ['sess_dropped', 'dropped', 'incremental', 1, 1],
                       ['sess_closed', 'closed', 'incremental', 1, 1],
                       ['sess_pipeline', 'pipeline', 'incremental', 1, 1],
                       ['sess_readahead' , 'readhead', 'incremental', 1, 1]],

              'options': [None, 'Sessions', 'units', 'Varnish statistics', 'varnish.session', 'line']},
          'session_herd': 
             {'lines': [['sess_herd', None, 'incremental', 1, 1]],
              'options': [None, 'Session herd', 'units', 'Extra charts', 'varnish.session_herd', 'line']},
          'shm': 
             {'lines': [['shm_flushes', 'flushes', 'incremental', 1, 1],
                       ['shm_cont', 'cont', 'incremental', 1, 1],
                       ['shm_cycles', 'cycles', 'incremental', 1, 1]],
              'options': [None, 'SHM writes and records', 'units', 'Extra charts', 'varnish.shm', 'line']},
          'shm_writes': 
             {'lines': [['shm_records', 'records', 'incremental', 1, 1],
                       ['shm_writes', 'writes', 'incremental', 1, 1]],
              'options': [None, 'SHM writes and records', 'units', 'Extra charts', 'varnish.shm_writes', 'line']},
          'threads': 
             {'lines': [['threads', None, 'absolute', 1, 1],
                       ['threads_created', 'created', 'incremental', 1, 1],
                       ['threads_failed', 'failed', 'incremental', 1, 1],
                       ['threads_limited', 'limited', 'incremental', 1, 1],
                       ['threads_destroyed', 'destroyed', 'incremental', 1, 1]],
              'options': [None, 'Thread status', 'units', 'Extra charts', 'varnish.threads', 'line']},
          'transfer_rates': 
             {'lines': [['s_resp_hdrbytes', 'header', 'incremental', 8, 1000],
                       ['s_resp_bodybytes', 'body', 'incremental', -8, 1000]],
              'options': [None, 'Transfer rates', 'kilobit/s', 'Varnish statistics', 'varnish.transfer_rates', 'area']},
          'uptime': 
             {'lines': [['uptime', None, 'absolute', 1, 1]],
              'options': [None, 'Varnish uptime', 'seconds', 'Varnish statistics', 'varnish.uptime', 'line']},
          'vcl': 
             {'lines': [['n_backend', None, 'absolute', 1, 1],
                       ['n_vcl', None, 'incremental', 1, 1],
                       ['n_vcl_avail', None, 'incremental', 1, 1],
                       ['n_vcl_discard', None, 'incremental', 1, 1]],
              'options': [None, 'VCL', 'units', 'Extra charts', 'varnish.vcl', 'line']}}

DIRECTORIES = ['/bin/', '/usr/bin/', '/sbin/', '/usr/sbin/']


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        try:
            self.varnish = [''.join([directory, 'varnishstat']) for directory in DIRECTORIES
                         if is_executable(''.join([directory, 'varnishstat']), X_OK)][0]
        except IndexError:
            self.varnish = False
        self.rgx_all = compile(r'([A-Z]+\.)([\d\w_.]+)\s+(\d+)')
        # Could be
        # VBE.boot.super_backend.pipe_hdrbyte (new)
        # or
        # VBE.default2(127.0.0.2,,81).bereq_bodybytes (old)
        # Regex result: [('super_backend', 'beresp_hdrbytes', '0'), ('super_backend', 'beresp_bodybytes', '0')]
        self.rgx_bck = (compile(r'VBE.([\d\w_.]+)\(.*?\).(beresp[\w_]+)\s+(\d+)'),
                        compile(r'VBE.boot.([\w\d_]+).(beresp[\w_]+)\s+(\d+)'))
        self.extra_charts = self.configuration.get('extra_charts', [])
        self.cache_prev = list()

    def check(self):
        # Cant start without 'varnishstat' command
        if not self.varnish:
            self.error('\'varnishstat\' command was not found in %s or not executable by netdata' % DIRECTORIES)
            return False

        # If command is present and we can execute it we need to make sure..
        # 1. STDOUT is not empty
        reply = self._get_raw_data()
        if not reply:
            self.error('No output from \'varnishstat\' (not enough privileges?)')
            return False

        # 2. Output is parsable (list is not empty after regex findall)
        is_parsable = self.rgx_all.findall(reply)
        if not is_parsable:
            self.error('Cant parse output (only varnish version 4+ supported)')
            return False

        # We need to find the right regex for backend parse
        self.backend_list = self.rgx_bck[0].findall(reply)[::2]
        if self.backend_list:
            self.rgx_bck = self.rgx_bck[0]
        else:
            self.backend_list = self.rgx_bck[1].findall(reply)[::2]
            self.rgx_bck = self.rgx_back[1]

        # We are about to start!
        self.create_charts()

        self.info('Active charts: %s' % self.order)
        self.info('Plugin was started successfully')
        return True
     
    def _get_raw_data(self):
        try:
            reply = Popen([self.varnish, '-1'], stdout=PIPE, stderr=PIPE, shell=False)
        except OSError:
            return None

        raw_data = reply.communicate()[0]

        if not raw_data:
            return None

        return raw_data

    def _get_data(self):
        """
        Format data received from shell command
        :return: dict
        """
        raw_data = self._get_raw_data()
        data_all = self.rgx_all.findall(raw_data)
        data_backend = self.rgx_bck.findall(raw_data)

        if not data_all:
            return None

        # 1. ALL data from 'varnishstat -1'. t - type(MAIN, MEMPOOL etc)
        to_netdata = {k: int(v) for t, k, v in data_all}
        
        # 2. ADD backend statistics
        to_netdata.update({'_'.join([n, k]): int(v) for n, k, v in data_backend})

        # 3. ADD additional keys to dict
        # 3.1 Cache hit/miss/hitpass OVERALL in percent
        cache_summary = sum([to_netdata.get('cache_hit', 0), to_netdata.get('cache_miss', 0),
                             to_netdata.get('cache_hitpass', 0)])
        to_netdata['cache_hit_perc'] = find_percent(to_netdata.get('cache_hit', 0), cache_summary, 10000)
        to_netdata['cache_miss_perc'] = find_percent(to_netdata.get('cache_miss', 0), cache_summary, 10000)
        to_netdata['cache_hitpass_perc'] = find_percent(to_netdata.get('cache_hitpass', 0), cache_summary, 10000)

        # 3.2 Cache hit/miss/hitpass CURRENT in percent
        if self.cache_prev:
            cache_summary = sum([to_netdata.get('cache_hit', 0), to_netdata.get('cache_miss', 0),
                                 to_netdata.get('cache_hitpass', 0)]) - sum(self.cache_prev)
            to_netdata['cache_hit_cperc'] = find_percent(to_netdata.get('cache_hit', 0) - self.cache_prev[0], cache_summary, 10000)
            to_netdata['cache_miss_cperc'] = find_percent(to_netdata.get('cache_miss', 0) - self.cache_prev[1], cache_summary, 10000)
            to_netdata['cache_hitpass_cperc'] = find_percent(to_netdata.get('cache_hitpass', 0) - self.cache_prev[2], cache_summary, 10000)
        else:
            to_netdata['cache_hit_cperc'] = 0
            to_netdata['cache_miss_cperc'] = 0
            to_netdata['cache_hitpass_cperc'] = 0

        self.cache_prev = [to_netdata.get('cache_hit', 0), to_netdata.get('cache_miss', 0), to_netdata.get('cache_hitpass', 0)]

        # 3.2 Copy random stuff to new keys (do we need this?)
        to_netdata['obj_per_objhead'] = find_percent(to_netdata.get('n_object', 0),
                                                     to_netdata.get('n_objecthead', 0), 100)
        to_netdata['backend_conn_bt'] = to_netdata.get('backend_conn', 0)
        to_netdata['sess_conn_rr'] = to_netdata.get('sess_conn', 0)
        to_netdata['n_lru_nuked_e'] = to_netdata.get('n_lru_nuked', 0)

        for elem in ['backend_busy', 'backend_unhealthy', 'esi_errors', 'esi_warnings', 'losthdr', 'sess_drop',
                     'sess_fail', 'sess_pipe_overflow', 'threads_destroyed', 'threads_failed', 'threads_limited']:
            to_netdata[''.join([elem, '_b'])] = to_netdata.get(elem, 0)

        # Ready steady go!
        return to_netdata

    def create_charts(self):
        # If 'all_charts' is true...ALL charts are displayed. If no only default + 'extra_charts'
        if self.configuration.get('all_charts'):
            self.order = EXTRA_ORDER
        else:
            try:
                extra_charts = list(filter(lambda chart: chart in EXTRA_ORDER, self.extra_charts.split()))
            except (AttributeError, NameError, ValueError):
                self.error('Extra charts disabled.')
                extra_charts = []
    
            self.order = ORDER[:]
            self.order.extend(extra_charts)

        # Create static charts
        self.definitions = {chart: values for chart, values in CHARTS.items() if chart in self.order}
 
        # Create dynamic backend charts
        if self.backend_list:
            for backend in self.backend_list:
                self.order.insert(0, ''.join([backend[0], '_resp_stats']))
                self.definitions.update({''.join([backend[0], '_resp_stats']): {
                    'options': [None,
                                '%s response statistics' % backend[0].capitalize(),
                                "kilobit/s",
                                'Backend response',
                                'varnish.backend',
                                'area'],
                    'lines': [[''.join([backend[0], '_beresp_hdrbytes']),
                               'header', 'incremental', 8, 1000],
                              [''.join([backend[0], '_beresp_bodybytes']),
                               'body', 'incremental', -8, 1000]]}})


def find_percent(value1, value2, multiply):
    # If value2 is 0 return 0
    if not value2:
        return 0
    else:
        return round(float(value1) / float(value2) * multiply)
