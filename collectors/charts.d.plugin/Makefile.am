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
	ap/README.md \
    apache/README.md \
    apcupsd/README.md \
    cpu_apps/README.md \
    cpufreq/README.md \
    example/README.md \
    exim/README.md \
    hddtemp/README.md \
    libreswan/README.md \
    load_average/README.md \
    mem_apps/README.md \
    mysql/README.md \
    nginx/README.md \
    nut/README.md \
    opensips/README.md \
    phpfpm/README.md \
    postfix/README.md \
    sensors/README.md \
    squid/README.md \
    tomcat/README.md \
	$(NULL)

dist_charts_SCRIPTS = \
	$(NULL)

dist_charts_DATA = \
	ap/ap.chart.sh \
	apcupsd/apcupsd.chart.sh \
	apache/apache.chart.sh \
	cpu_apps/cpu_apps.chart.sh \
	cpufreq/cpufreq.chart.sh \
	example/example.chart.sh \
	exim/exim.chart.sh \
	hddtemp/hddtemp.chart.sh \
	libreswan/libreswan.chart.sh \
	load_average/load_average.chart.sh \
	mem_apps/mem_apps.chart.sh \
	mysql/mysql.chart.sh \
	nginx/nginx.chart.sh \
	nut/nut.chart.sh \
	opensips/opensips.chart.sh \
	phpfpm/phpfpm.chart.sh \
	postfix/postfix.chart.sh \
	sensors/sensors.chart.sh \
	squid/squid.chart.sh \
	tomcat/tomcat.chart.sh \
	$(NULL)

chartsconfigdir=$(libconfigdir)/charts.d
dist_chartsconfig_DATA = \
    ap/ap.conf \
    apache/apache.conf \
    apcupsd/apcupsd.conf \
    cpu_apps/cpu_apps.conf \
    cpufreq/cpufreq.conf \
    example/example.conf \
    exim/exim.conf \
    hddtemp/hddtemp.conf \
    libreswan/libreswan.conf \
    load_average/load_average.conf \
    mem_apps/mem_apps.conf \
    mysql/mysql.conf \
    nginx/nginx.conf \
    nut/nut.conf \
    opensips/opensips.conf \
    phpfpm/phpfpm.conf \
    postfix/postfix.conf \
    sensors/sensors.conf \
    squid/squid.conf \
    tomcat/tomcat.conf \
    $(NULL)
