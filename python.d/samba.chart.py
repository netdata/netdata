# -*- coding: utf-8 -*-
# Description:  samba netdata python.d module
# Author: Christopher Cox <chris_cox@endlessnow.com>
#
# The netdata user needs to be able to be able to sudo the smbstatus program without password
# netdata ALL=(ALL)       NOPASSWD: /usr/bin/smbstatus -P
#
# This makes calls to smbstatus -P (note smbd needs to be run with the -P 1 option) a lot.
# 
# Right now this just looks for the smb2_ counters, but adjust the regex to get what you need.
#
# The first chart makes a bad assumption that somehow readout/readin and writeout/writein 
# have something to do with actual reads and writes.  While it seems to be mostly true for reads,
# I am not sure what the write values are.  Perhaps they shouldn't be paired?
#
# create and close are paired, maybe they shouldn't be?
#
# getinfo and setinfo are paired, maybe they shouldn't be?
#
# The Other Smb2 chart is merely a display of current counter values.  They didn't seem to change
# much to me.  However, if you notice something changing a lot there, bring one or more out into its own
# chart and make it incremental (like find and notify... good examples).


from base import SimpleService
from re import compile
from subprocess import Popen, PIPE

# default module values (can be overridden per job in `config`)
# update_every = 2
priority = 60000
retries = 60

ORDER = ['smb2_rw','smb2_create_close','smb2_info','smb2_find','smb2_notify','smb2_sm_count']

CHARTS = {
           'smb2_rw': {
             'lines': [
               ['smb2_read_outbytes', 'readout', 'incremental', 1, 1024],
               ['smb2_write_inbytes', 'writein', 'incremental', -1, 1024],
               ['smb2_read_inbytes', 'readin', 'incremental', 1, 1024],
               ['smb2_write_outbytes', 'writeout', 'incremental', -1, 1024]
             ],
             'options': [None, 'R/Ws', 'kilobytes/s', 'Smb2', 'smb2.readwrite', 'area']
           },
           'smb2_create_close': {
             'lines': [
               ['smb2_create_count', 'create', 'incremental', 1, 1],
               ['smb2_close_count', 'close', 'incremental', -1, 1]
             ],
             'options': [None, 'Create/Close', 'operations/s', 'Smb2', 'smb2.create_close', 'area']
           },
           'smb2_info': {
             'lines': [
               ['smb2_getinfo_count', 'getinfo', 'incremental', 1, 1],
               ['smb2_setinfo_count', 'setinfo', 'incremental', -1, 1]
             ],
             'options': [None, 'Info', 'operations/s', 'Smb2', 'smb2.get_set_info', 'area']
           },
           'smb2_find': {
             'lines': [
               ['smb2_find_count', 'find', 'incremental', 1, 1]
             ],
             'options': [None, 'Find', 'operations/s', 'Smb2', 'smb2.find', 'area']
           },
           'smb2_notify': {
             'lines': [
               ['smb2_notify_count', 'notify', 'incremental', 1, 1]
             ],
             'options': [None, 'Notify', 'operations/s', 'Smb2', 'smb2.notify', 'area']
           },
           'smb2_sm_count': {
             'lines': [
               ['smb2_tcon_count', 'tcon', 'absolute', 1, 1],
               ['smb2_negprot_count', 'negprot', 'absolute', 1, 1],
               ['smb2_tdis_count', 'tdis', 'absolute', 1, 1],
               ['smb2_cancel_count', 'cancel', 'absolute', 1, 1],
               ['smb2_logoff_count', 'logoff', 'absolute', 1, 1],
               ['smb2_flush_count', 'flush', 'absolute', 1, 1],
               ['smb2_lock_count', 'lock', 'absolute', 1, 1],
               ['smb2_keepalive_count', 'keepalive', 'absolute', 1, 1],
               ['smb2_break_count', 'break', 'absolute', 1, 1],
               ['smb2_sessetup_count', 'sessetup', 'absolute', 1, 1]
             ],
             'options': [None, 'Lesser Ops', 'count', 'Other Smb2', 'smb2.sm_counters', 'stacked']
           }
         }


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.smbstatus = self.find_binary('smbstatus')
        self.rgx_smb2 = compile(r'(smb2_[^:]+):\s+(\d+)')
        self.cache_prev = list()

    def check(self):
        # Cant start without 'smbstatus' command
        if not self.smbstatus:
            self.error('Can\'t locate \'smbstatus\' binary or binary is not executable by netdata')
            return False

        # If command is present and we can execute it we need to make sure..
        # 1. STDOUT is not empty
        reply = self._get_raw_data()
        if not reply:
            self.error('No output from \'smbstatus\' (not enough privileges?)')
            return False
        self.error(reply)

        # 2. Output is parsable (list is not empty after regex findall)
        is_parsable = self.rgx_smb2.findall(reply)
        if not is_parsable:
            self.error('Cant parse output...')
            return False

        # We are about to start!
        self.create_charts()

        self.info('Plugin was started successfully')
        return True
     
    def _get_raw_data(self):
        try:
            reply = Popen(['/usr/bin/sudo', '-n', self.smbstatus, '-P'], stdout=PIPE, stderr=PIPE, shell=False)
        except OSError:
            return None

        raw_data = reply.communicate()[0]

        if not raw_data:
            return None

        return raw_data.decode()

    def _get_data(self):
        """
        Format data received from shell command
        :return: dict
        """
        raw_data = self._get_raw_data()
        data_all = self.rgx_smb2.findall(raw_data)

        if not data_all:
            return None

        # 1. ALL data from 'smbstatus -P'.
        to_netdata = dict([(k, int(v)) for k, v in data_all])
        
        # Ready steady go!
        return to_netdata

    def create_charts(self):
        # If 'all_charts' is true...ALL charts are displayed. If no only default + 'extra_charts'
        #if self.configuration.get('all_charts'):
        #    self.order = EXTRA_ORDER
        #else:
        #    try:
        #        extra_charts = list(filter(lambda chart: chart in EXTRA_ORDER, self.extra_charts.split()))
        #    except (AttributeError, NameError, ValueError):
        #        self.error('Extra charts disabled.')
        #        extra_charts = []
    
        self.order = ORDER[:]
        #self.order.extend(extra_charts)

        # Create static charts
        #self.definitions = {chart: values for chart, values in CHARTS.items() if chart in self.order}
        self.definitions = CHARTS

