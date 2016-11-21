# -*- coding: utf-8 -*-
# Description: logging for netdata python.d modules

import traceback
import sys
from time import time, strftime

DEBUG_FLAG = False
TRACE_FLAG = False
PROGRAM = ""
LOG_COUNTER = 0
LOG_THROTTLE = 10000 # has to be too big during init
LOG_INTERVAL = 1     # has to be too low during init
LOG_NEXT_CHECK = 0

WRITE = sys.stderr.write
FLUSH = sys.stderr.flush


def log_msg(msg_type, *args):
    """
    Print message on stderr.
    :param msg_type: str
    """
    global LOG_COUNTER
    global LOG_THROTTLE
    global LOG_INTERVAL
    global LOG_NEXT_CHECK
    now = time()

    if not DEBUG_FLAG:
        LOG_COUNTER += 1

    # WRITE("COUNTER " + str(LOG_COUNTER) + " THROTTLE " + str(LOG_THROTTLE) + " INTERVAL " + str(LOG_INTERVAL) + " NOW " + str(now) + " NEXT " + str(LOG_NEXT_CHECK) + "\n")

    if LOG_COUNTER <= LOG_THROTTLE or msg_type == "FATAL" or msg_type == "ALERT":
        timestamp = strftime('%Y-%m-%d %X')
        msg = "%s: %s %s: %s" % (timestamp, PROGRAM, str(msg_type), " ".join(args))
        WRITE(msg + "\n")
        FLUSH()
    elif LOG_COUNTER == LOG_THROTTLE + 1:
        timestamp = strftime('%Y-%m-%d %X')
        msg = "%s: python.d.plugin: throttling further log messages for %s seconds" % (timestamp, str(int(LOG_NEXT_CHECK + 0.5) - int(now)))
        WRITE(msg + "\n")
        FLUSH()

    if LOG_NEXT_CHECK <= now:
        if LOG_COUNTER >= LOG_THROTTLE:
            timestamp = strftime('%Y-%m-%d %X')
            msg = "%s: python.d.plugin: Prevented %s log messages from displaying" % (timestamp, str(LOG_COUNTER - LOG_THROTTLE))
            WRITE(msg + "\n")
            FLUSH()
        LOG_NEXT_CHECK = now - (now % LOG_INTERVAL) + LOG_INTERVAL
        LOG_COUNTER = 0

    if TRACE_FLAG:
        if msg_type == "FATAL" or msg_type == "ERROR" or msg_type == "ALERT":
            traceback.print_exc()


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


def alert(*args):
    """
    Print message on stderr.
    """
    log_msg("ALERT", *args)


def info(*args):
    """
    Print message on stderr.
    """
    log_msg("INFO", *args)


def fatal(*args):
    """
    Print message on stderr and exit.
    """
    try:
        log_msg("FATAL", *args)
        print('DISABLE')
    except:
        pass
    sys.exit(1)
