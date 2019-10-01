# SPDX-License-Identifier: GPL-3.0-or-later

AUTOMAKE_OPTIONS = subdir-objects
MAINTAINERCLEANFILES = $(srcdir)/Makefile.in

check_PROGRAMS = str2ld_testdriver

TESTS = $(check_PROGRAMS)

str2ld_testdriver_CFLAGS  = @TEST_CFLAGS@
str2ld_testdriver_LDFLAGS = @TEST_LDFLAGS@
str2ld_testdriver_SOURCES = test_str2ld.c
