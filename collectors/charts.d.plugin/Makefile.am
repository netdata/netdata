# SPDX-License-Identifier: GPL-3.0-or-later

MAINTAINERCLEANFILES = $(srcdir)/Makefile.in
CLEANFILES = \
    charts.d.plugin \
    $(NULL)

include $(top_srcdir)/build/subst.inc
SUFFIXES = .in

dist_libconfig_DATA = \
    charts.d.conf \
    $(NULL)

dist_plugins_SCRIPTS = \
    charts.d.dryrun-helper.sh \
    charts.d.plugin \
    loopsleepms.sh.inc \
    $(NULL)

dist_noinst_DATA = \
    charts.d.plugin.in \
    README.md \
    $(NULL)

dist_charts_SCRIPTS = \
    $(NULL)

dist_charts_DATA = \
    $(NULL)

userchartsconfigdir=$(configdir)/charts.d
dist_userchartsconfig_DATA = \
    $(NULL)

# Explicitly install directories to avoid permission issues due to umask
install-exec-local:
	$(INSTALL) -d $(DESTDIR)$(userchartsconfigdir)

chartsconfigdir=$(libconfigdir)/charts.d
dist_chartsconfig_DATA = \
    $(NULL)

include ap/Makefile.inc
include apcupsd/Makefile.inc
include example/Makefile.inc
include libreswan/Makefile.inc
include nut/Makefile.inc
include opensips/Makefile.inc
include sensors/Makefile.inc
