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
    sys.stderr.write(PROGRAM)
    sys.stderr.write(" ")
    sys.stderr.write(msg_type)
    sys.stderr.write(": ")
    for i in args:
        sys.stderr.write(" ")
        sys.stderr.write(str(i))
    sys.stderr.write("\n")
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