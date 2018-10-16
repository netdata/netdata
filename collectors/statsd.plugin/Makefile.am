# SPDX-License-Identifier: GPL-3.0-or-later

AUTOMAKE_OPTIONS = subdir-objects
MAINTAINERCLEANFILES = $(srcdir)/Makefile.in

dist_noinst_DATA = \
	README.md \
	$(NULL)

statsdconfigdir=$(libconfigdir)/statsd.d
dist_statsdconfig_DATA = \
    $(top_srcdir)/installer/.keep \
    example.conf \
    $(NULL)

userstatsdconfigdir=$(configdir)/statsd.d
dist_userstatsdconfig_DATA = \
    $(top_srcdir)/installer/.keep \
    $(NULL)

