# -*- coding: utf-8 -*-
# Description: logging for netdata python.d modules

import sys
from time import time, strftime

DEBUG_FLAG = False
PROGRAM = ""
LOG_COUNTER = 2
LOG_INTERVAL = 5
NEXT_CHECK = 0

WRITE = sys.stderr.write
FLUSH = sys.stderr.flush


def log_msg(msg_type, *args):
    """
    Print message on stderr.
    :param msg_type: str
    """
    global LOG_COUNTER
    if not DEBUG_FLAG:
        LOG_COUNTER -= 1
    now = time()
    if LOG_COUNTER >= 0:
        timestamp = strftime('%y-%m-%d %X')
        msg = "%s: %s %s: %s" % (timestamp, PROGRAM, str(msg_type), " ".join(args))
        WRITE(msg + "\n")
        FLUSH()

    global NEXT_CHECK
    if NEXT_CHECK <= now:
        NEXT_CHECK = now - (now % LOG_INTERVAL) + LOG_INTERVAL
        if LOG_COUNTER < 0:
            timestamp = strftime('%y-%m-%d %X')
            msg = "%s: Prevented %s log messages from displaying" % (timestamp, str(0 - LOG_COUNTER))
            WRITE(msg + "\n")
            FLUSH()


def debug(*args):
    """
    Print debug message on stderr.
    """
    if not DEBUG_FLAG:
        return

    log_msg("DEBUG", *args)


def error(*args):
    """
    Print message on stderr.
    """
    log_msg("ERROR", *args)


def info(*args):
    """
    Print message on stderr.
    """
    log_msg("INFO", *args)


def fatal(*args):
    """
    Print message on stderr and exit.
    """
    log_msg("FATAL", *args)
    # using sys.stdout causes IOError: Broken Pipe
    print('DISABLE')
    # sys.stdout.write('DISABLE\n')
    sys.exit(1)