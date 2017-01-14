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

ORDER = ['request_rate', 'hit_rate', 'transfer_rates', 'session', 'backend_traffic', 'bad', 'uptime']
EXTRA_ORDER = ['request_rate', 'hit_rate', 'backend_traffic', 'objects', 'transfer_rates', 'threads', 'memory_usage',
         'objects_per_objhead', 'losthdr', 'hcb', 'esi', 'session', 'session_herd', 'shm_writes', 
         'shm', 'allocations', 'vcl', 'bans', 'bans_lurker', 'expunge', 'lru', 'bad', 'gzip', 'uptime']

CHARTS = {'allocations': 
             {'lines': [['sm_nreq', None, 'incremental', 1, 1],
                       ['sma_nreq', None, 'incremental', 1, 1],
                       ['sms_nreq', None, 'incremental', 1, 1]],
              'options': [None, 'Memory allocation requests', 'units', 'allocations', 'varnish.alloc','line']},
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
              'options': [None, 'Backend traffic', 'units', 'backend_traffic', 'varnish.backend_traf', 'line']},
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
              'options': [None, 'Misbehavior', 'units', 'bad', 'varnish.bad', 'line']},
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
              'options': [None, 'Bans', 'units', 'bans', 'varnish.bans', 'line']},
          'bans_lurker': 
             {'lines': [['bans_lurker_tested', 'tested', 'incremental', 1, 1],
                       ['bans_lurker_tests_tested', 'tests_tested', 'incremental', 1, 1],
                       ['bans_lurker_obj_killed', 'obj_killed', 'incremental', 1, 1],
                       ['bans_lurker_contention', 'contention', 'incremental', 1, 1]],
              'options': [None, 'Ban Lurker', 'units', 'bans_lurker', 'varnish.bans_lurker', 'line']},
          'esi':
             {'lines': [['esi_parse', None, 'incremental', 1, 1],
                       ['esi_errors', None, 'incremental', 1, 1],
                       ['esi_warnings', None, 'incremental', 1, 1]],
              'options': [None, 'ESI', 'units', 'esi', 'varnish.esi', 'line']},
          'expunge':
             {'lines': [['n_expired', None, 'incremental', 1, 1],
                       ['n_lru_nuked_e', None, 'incremental', 1, 1]],
              'options': [None, 'Object expunging', 'units', 'expunge', 'varnish.expunge', 'line']},
          'gzip': 
             {'lines': [['n_gzip', None, 'incremental', 1, 1],
                       ['n_gunzip', None, 'incremental', 1, 1]],
              'options': [None, 'GZIP activity', 'units', 'gzip', 'varnish.gzip', 'line']},
          'hcb': 
             {'lines': [['hcb_nolock', 'nolock', 'incremental', 1, 1],
                       ['hcb_lock', 'lock', 'incremental', 1, 1],
                       ['hcb_insert', 'insert', 'incremental', 1, 1]],
              'options': [None, 'Critbit data', 'units', 'hcb', 'varnish.hcb', 'line']},
          'hit_rate': 
             {'lines': [['cache_hit_perc', 'hit', 'absolute', 1, 100],
                       ['cache_miss_perc', 'miss', 'absolute', 1, 100],
                       ['cache_hitpass_perc', 'hitpass', 'absolute', 1, 100]],
              'options': [None, 'Hit rates','percent', 'hit_rate', 'varnish.hit_rate', 'line']},
          'losthdr': 
             {'lines': [['losthdr', None, 'incremental', 1, 1]],
              'options': [None, 'HTTP Header overflows', 'units', 'losthdr', 'varnish.losthdr', 'line']},
          'lru':
             {'lines': [['n_lru_nuked', 'nuked', 'incremental', 1, 1],
                       ['n_lru_moved', 'moved', 'incremental', 1, 1]],
              'options': [None, 'LRU activity', 'units', 'lru', 'varnish.lru', 'line']},
          'memory_usage': 
             {'lines': [['sms_balloc', None, 'absolute', 1, 1],
                       ['sms_nbytes', None, 'absolute', 1, 1]],
              'options': [None, 'Memory usage', 'units', 'memory_usage', 'varnish.memory_usage', 'line']},
          'objects': 
             {'lines': [['n_object', 'object', 'absolute', 1, 1],
                       ['n_objectcore', 'objectcore', 'absolute', 1, 1],
                       ['n_vampireobject', 'vampireobject, ''absolute', 1, 1],
                       ['n_objecthead', 'objecthead', 'absolute', 1, 1]],
              'options': [None, 'Number of objects', 'units', 'objects', 'varnish.objects', 'line']},
          'objects_per_objhead': 
             {'lines': [['obj_per_objhead', 'per_objhead', 'absolute', 1, 100]],
              'options': [None, 'Objects per objecthead', 'units', 'objects_per_objhead', 'varnish.objects_per_objhead', 'line']},
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
              'options': [None, 'Request rates', 'units', 'request_rate', 'varnish.request_rate', 'line']},
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

              'options': [None, 'Sessions', 'units', 'session', 'varnish.session', 'line']},
          'session_herd': 
             {'lines': [['sess_herd', None, 'incremental', 1, 1]],
              'options': [None, 'Session herd', 'units', 'session_herd', 'varnish.session_herd', 'line']},
          'shm': 
             {'lines': [['shm_flushes', 'flushes', 'incremental', 1, 1],
                       ['shm_cont', 'cont', 'incremental', 1, 1],
                       ['shm_cycles', 'cycles', 'incremental', 1, 1]],
              'options': [None, 'SHM writes and records', 'units', 'shm', 'varnish.shm', 'line']},
          'shm_writes': 
             {'lines': [['shm_records', 'records', 'incremental', 1, 1],
                       ['shm_writes', 'writes', 'incremental', 1, 1]],
              'options': [None, 'SHM writes and records', 'units', 'shm_writes', 'varnish.shm_writes', 'line']},
          'threads': 
             {'lines': [['threads', None, 'absolute', 1, 1],
                       ['threads_created', 'created', 'incremental', 1, 1],
                       ['threads_failed', 'failed', 'incremental', 1, 1],
                       ['threads_limited', 'limited', 'incremental', 1, 1],
                       ['threads_destroyed', 'destroyed', 'incremental', 1, 1]],
              'options': [None, 'Thread status', 'units', 'threads', 'varnish.threads', 'line']},
          'transfer_rates': 
             {'lines': [['s_resp_hdrbytes', 'resp_hdrbytes', 'incremental', 8, 1],
                       ['s_resp_bodybytes', 'resp_bodybytes', 'incremental', 8, 1]],
              'options': [None, 'Transfer rates', 'bits/s', 'transfer_rates', 'varnish.transfer_rates', 'line']},
          'uptime': 
             {'lines': [['uptime', None, 'absolute', 1, 1]],
              'options': [None, 'Varnish uptime', 'seconds', 'uptime', 'varnish.uptime', 'line']},
          'vcl': 
             {'lines': [['n_backend', None, 'absolute', 1, 1],
                       ['n_vcl', None, 'incremental', 1, 1],
                       ['n_vcl_avail', None, 'incremental', 1, 1],
                       ['n_vcl_discard', None, 'incremental', 1, 1]],
              'options': [None, 'VCL', 'units', 'vcl', 'varnish.vcl', 'line']}}

DIRECTORIES = ['/bin/', '/usr/bin/', '/sbin/', '/usr/sbin/']


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        try:
            self.varnish = [''.join([directory, 'varnishstat']) for directory in DIRECTORIES
                         if is_executable(''.join([directory, 'varnishstat']), X_OK)][0]
        except IndexError:
            self.varnish = False
        self.regex = compile(r'([A-Z]+\.)([\d\w_.]+)\s+([0-9]+)')
        self.extra_charts = self.configuration.get('extra_charts', [])

    def check(self):
        # Cant start without varnishstat command
        if not self.varnish:
            self.error('varnishstat command was not found in %s or not executable by netdata' % DIRECTORIES)
            return False

        # If command is present and we can execute it we need to make sure..
        # 1. STDOUT is not empty
        reply = self._get_raw_data()
        if not reply:
            self.error('No output from varnishstat (not enough privileges?)')
            return False

        # 2. Output is parsable (list is not empty after regex findall)
        is_parsable = self.regex.findall(reply)
        if not is_parsable:
            self.error('Cant parse output (only varnish version 4+ supported)')
            return False
        
        # We are about to start!
        self.create_charts()

        self.info('Active charts: %s' % self.order)
        self.info('Plugin was started succesfully')
        return True
     

    def _get_raw_data(self):
        try:
            reply = Popen([self.varnish, '-1'], stdout=PIPE, stderr=PIPE, shell=False)
        except OSError:
            return None

        raw_data = reply.stdout.read()

        if not raw_data:
            return None

        return raw_data

    def _get_data(self):
        """
        Format data received from shell command
        :return: dict
        """
        raw_data = self._get_raw_data()
        data = self.regex.findall(raw_data)

        if not data:
            return None

        # ALL data from 'varnishstat -1'. t - type(MAIN, MEMPOOL etc)
        to_netdata = {k: int(v) for t, k, v in data}

        # ADD additional keys to dict
        cache_summary = sum([to_netdata.get('cache_hit', 0), to_netdata.get('cache_miss', 0), to_netdata.get('cache_hitpass', 0)])
        to_netdata['cache_hit_perc'] = find_percent(to_netdata.get('cache_hit', 0), cache_summary, 10000)
        to_netdata['cache_miss_perc'] = find_percent(to_netdata.get('cache_miss', 0), cache_summary, 10000)
        to_netdata['cache_hitpass_perc'] = find_percent(to_netdata.get('cache_hitpass', 0), cache_summary, 10000)
        to_netdata['obj_per_objhead'] = find_percent(to_netdata.get('n_object', 0), to_netdata.get('n_objecthead', 0), 100)
        to_netdata['backend_conn_bt'] = to_netdata.get('backend_conn', 0)
        to_netdata['sess_conn_rr'] = to_netdata.get('sess_conn', 0)
        to_netdata['n_lru_nuked_e'] = to_netdata.get('n_lru_nuked', 0)

        for elem in ['backend_busy', 'backend_unhealthy', 'esi_errors', 'esi_warnings', 'losthdr', 'sess_drop', 'sess_fail', 'sess_pipe_overflow',
                     'threads_destroyed', 'threads_failed', 'threads_limited']:
            to_netdata[''.join([elem, '_b'])] = to_netdata.get(elem, 0)

        return to_netdata

    def create_charts(self):
        if self.configuration.get('all_charts'):
            self.order = EXTRA_CHARTS
        else:
            try:
                extra_charts = list(filter(lambda chart: chart in EXTRA_ORDER, self.extra_charts.split()))
            except Exception:
                self.error('Extra charts disabled.')
                extra_charts = []
    
            self.order = ORDER[:]
            self.order.extend(extra_charts)
            self.order = self.order

        self.definitions = {k: v for k, v in CHARTS.items() if k in self.order}


def find_percent(value1, value2, multiply):
    # If value 2 is 0 return 0
    if not value2:
        return 0
    else:
        return round(float(value1) / float(value2) * multiply)
