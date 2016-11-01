# -*- coding: utf-8 -*-
# Description: logging for netdata python.d modules

import sys
import time
import logging

LOG_FORMAT = '%(asctime)s python %(levelname)s: %(message)s'
#LOG_FORMAT = '%(asctime)s %(name)s %(levelname)s: %(message)s'
#LOG_FORMAT = '%(asctime)s %(levelname)s: %(message)s'
LOG_DATE_FMT = '%Y-%m-%d %X'
LOGS_PER_INTERVAL = 2
LOGS_INTERVAL = 2
DEFAULT_LOG_LEVEL = "ERROR"


class TruncateFilter(logging.Filter):
    message_counter = 1
    next_check = 0

    def filter(self, record):
        now = time.time()
        if self.next_check <= now:
            self.next_check = now - (now % LOGS_INTERVAL) + LOGS_INTERVAL
            if self.message_counter > LOGS_PER_INTERVAL:
                record.msg = "truncated %s log messages" % self.message_counter
                self.message_counter = 1
                return 1

        if self.message_counter > LOGS_PER_INTERVAL:
            self.message_counter += 1
            return 0

        self.message_counter += 1
        return 1

# logging.basicConfig(format=LOG_FORMAT, datefmt=LOG_DATE_FMT)
# log = logging.getLogger(__name__)
# log.setLevel(DEFAULT_LOG_LEVEL)
# msg = log


def message(type, *args):
    """
    Print message on stderr.
    """
    timestamp = time.strftime('%Y-%m-%d %X')
    msg = "%s: %s %s: %s" % (timestamp, "python", str(type), " ".join(args))
    sys.stderr.write(msg)

