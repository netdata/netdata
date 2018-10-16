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

chartsconfigdir=$(libconfigdir)/charts.d
dist_chartsconfig_DATA = \
    $(NULL)

include ap/Makefile.inc
include apache/Makefile.inc
include apcupsd/Makefile.inc
include cpu_apps/Makefile.inc
include cpufreq/Makefile.inc
include example/Makefile.inc
include exim/Makefile.inc
include hddtemp/Makefile.inc
include libreswan/Makefile.inc
include load_average/Makefile.inc
include mem_apps/Makefile.inc
include mysql/Makefile.inc
include nginx/Makefile.inc
include nut/Makefile.inc
include opensips/Makefile.inc
include phpfpm/Makefile.inc
include postfix/Makefile.inc
include sensors/Makefile.inc
include squid/Makefile.inc
include tomcat/Makefile.inc
