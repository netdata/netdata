# -*- coding: utf-8 -*-
# Description: logging for netdata python.d modules

import sys

DEBUG_FLAG = False
PROGRAM = ""


def log_msg(msg_type, *args):
    """
    Print message on stderr.
    :param msg_type: str
    """
    msg = PROGRAM + " " + str(msg_type) + ":"
    for i in args:
        msg += " "
        msg += str(i)
    sys.stderr.write(msg + "\n")
    sys.stderr.flush()


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
    sys.stdout.write('DISABLE\n')
    sys.exit(1)