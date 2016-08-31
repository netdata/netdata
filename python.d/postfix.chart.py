# -*- coding: utf-8 -*-
# Description: postfix netdata python.d module
# Author: Pawel Krupa (paulfantom)
from subprocess import PIPE, Popen
from base import ExecutableService

# default module values (can be overridden per job in `config`)
# update_every = 2
priority = 60000
retries = 60

# charts order (can be overridden if you want less charts, or different order)
ORDER = ['qemails', 'qsize', 'qlengths']

CHARTS = {
    'qemails': {
        'options': [None, "Postfix Queue Emails", "emails", 'queue', 'postfix.qemails', 'line'],
        'lines': [
            ['emails', None, 'absolute']
        ]},
    'qsize': {
        'options': [None, "Postfix Queue Emails Size", "emails size in KB", 'queue', 'postfix.qsize', 'area'],
        'lines': [
            ["size", None, 'absolute']
        ]},
    'qlengths': {
        'options': [None, "Queue Lengths", "emails", 'queue', 'postfix.qlenghts', 'line'],
        'lines': [
            ['Incoming', None, 'absolute','1'],
            ['Active', None, 'absolute', '-1'],
            ['Deferred', None, 'absolute','1'],
            ['Bounced', None, 'absolute','1']
        ]}

}


class Service(ExecutableService):
    def __init__(self, configuration=None, name=None):
        ExecutableService.__init__(self, configuration=configuration, name=name)
        self.command = "postqueue -p"
        self.order = ORDER
        self.definitions = CHARTS


    def _get_data(self):
        """
        Format data received from shell command
        :return: dict
        """
        try:
            raw = self._get_raw_data()[-1].split(' ')
            if raw[0] == 'Mail' and raw[1] == 'queue':
                emails_count = 0
                size_kb = 0
            else:
                emails_count = raw[4]
                size_kb = raw[1]
            p1 = Popen(["postconf", "-h", "queue_directory"],stdin=PIPE,stdout=PIPE)
            command1 = p1.communicate()[0]
            command1 = command1.rstrip()

            command2 = command1 + "/active"
            p2 = Popen(["find", command2, "-type", "f"],stdin=PIPE,stdout=PIPE)
            active_files = p2.communicate()[0]
            active_files = active_files.rstrip()
            if active_files == '':
                active_count = 0
            else:
                active_count = len(active_files.split("\n"))

            command3 = command1 + "/incoming"
            p3 = Popen(["find", command3, "-type", "f"],stdin=PIPE,stdout=PIPE)
            incoming_files = p3.communicate()[0]
            incoming_files = incoming_files.rstrip()
            if incoming_files == '':
                incoming_count = 0
            else:
                incoming_count = len(incoming_files.split("\n"))

            command4 = command1 + "/deferred"
            p4 = Popen(["find", command4, "-type", "f"],stdin=PIPE,stdout=PIPE)
            deferred_files = p4.communicate()[0]
            deferred_files = deferred_files.rstrip()
            if deferred_files == '':
                deferred_count = 0
            else:
                deferred_count = len(deferred_files.split("\n"))

            command5 = command1 + "/bounced"
            p5 = Popen(["find", command5, "-type", "f"],stdin=PIPE,stdout=PIPE)
            bounced_files = p5.communicate()[0]
            bounced_files = bounced_files.rstrip()
            if bounced_files == '':
                bounced_count = 0
            else:
                bounced_count = len(bounced_files.split("\n"))

            return {'emails': emails_count,
                    'size': size_kb,
                    'Incoming': incoming_count,
                    'Active': active_count,
                    'Deferred': deferred_count,
                    'Bounced': bounced_count }
        except (ValueError, AttributeError):
            return None
