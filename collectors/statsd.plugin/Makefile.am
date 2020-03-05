# SPDX-License-Identifier: GPL-3.0-or-later

AUTOMAKE_OPTIONS = subdir-objects
MAINTAINERCLEANFILES = $(srcdir)/Makefile.in

dist_noinst_DATA = \
    README.md \
    $(NULL)

statsdconfigdir=$(libconfigdir)/statsd.d
dist_statsdconfig_DATA = \
    example.conf \
    $(NULL)

userstatsdconfigdir=$(configdir)/statsd.d
dist_userstatsdconfig_DATA = \
    $(NULL)

# Explicitly install directories to avoid permission issues due to umask
install-exec-local:
	$(INSTALL) -d $(DESTDIR)$(userstatsdconfigdir)
