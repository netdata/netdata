#
# Copyright (C) 2015 Alon Bar-Lev <alon.barlev@gmail.com>
# SPDX-License-Identifier: GPL-3.0-or-later
#
MAINTAINERCLEANFILES= $(srcdir)/Makefile.in

SUBDIRS = \
	$(NULL)

AM_CFLAGS = \
	$(OPTIONAL_MATH_CFLAGS) \
	$(OPTIONAL_NFACCT_CLFAGS) \
	$(OPTIONAL_ZLIB_CFLAGS) \
	$(OPTIONAL_UUID_CFLAGS) \
	$(OPTIONAL_LIBCAP_CFLAGS) \
	$(OPTIONAL_IPMIMONITORING_CFLAGS)\
	$(NULL)

sbin_PROGRAMS = netdata
dist_cache_DATA = .keep
dist_varlib_DATA = .keep
dist_registry_DATA = .keep
dist_log_DATA = .keep
plugins_PROGRAMS =

netdata_SOURCES = \
    plugins/all.h \
	adaptive_resortable_list.c \
	adaptive_resortable_list.h \
	appconfig.c \
	appconfig.h \
	avl.c \
	avl.h \
	backend_prometheus.c \
	backend_prometheus.h \
	backends.c \
	backends.h \
	clocks.c \
	clocks.h \
	common.c \
	common.h \
	daemon.c \
	daemon.h \
	dictionary.c \
	dictionary.h \
	eval.c \
	eval.h \
	global_statistics.c \
	global_statistics.h \
	health.c \
	health.h \
	health_config.c \
	health_json.c \
	health_log.c \
	inlined.h \
	locks.c \
	locks.h \
	log.c \
	log.h \
	main.c \
	main.h \
	popen.c \
	popen.h \
	procfile.c \
	procfile.h \
	registry.c \
	registry.h \
	registry_db.c \
	registry_init.c \
	registry_internals.c \
	registry_internals.h \
	registry_log.c \
	registry_machine.c \
	registry_machine.h \
	registry_person.c \
	registry_person.h \
	registry_url.c \
	registry_url.h \
	rrd.c \
	rrd.h \
	rrd2json.c \
	rrd2json.h \
	rrdcalc.c \
	rrdcalctemplate.c \
	rrddim.c \
	rrddimvar.c \
	rrdfamily.c \
	rrdhost.c \
	rrdpush.c \
	rrdpush.h \
	rrdset.c \
	rrdsetvar.c \
	rrdvar.c \
	signals.c \
	signals.h \
	simple_pattern.c \
	simple_pattern.h \
	socket.c \
	socket.h \
	statistical.c \
	statistical.h \
	storage_number.c \
	storage_number.h \
	threads.c \
	threads.h \
	unit_test.c \
	unit_test.h \
	url.c \
	url.h \
	web_api_v1.c \
	web_api_v1.h \
	web_buffer.c \
	web_buffer.h \
	web_buffer_svg.c \
	web_buffer_svg.h \
	web_client.c \
	web_client.h \
	web_server.c \
	web_server.h \
	$(NULL)

netdata_LDADD = \
    $(top_builddir)/src/plugins/checks.plugin/libplugin-checks.a \
    $(top_builddir)/src/plugins/idlejitter.plugin/libplugin-idlejitter.a \
    $(top_builddir)/src/plugins/plugins.d.plugin/libplugin-plugins.d.a \
    $(top_builddir)/src/plugins/statsd.plugin/libplugin-statsd.a \
	$(NULL)

if FREEBSD

netdata_LDADD += \
    $(top_builddir)/src/plugins/freebsd.plugin/libplugin-freebsd.a \
    $(NULL)

else
if MACOS

netdata_LDADD += \
    $(top_builddir)/src/plugins/macos.plugin/libplugin-macos.a \
    $(NULL)

else

netdata_LDADD += \
    $(top_builddir)/src/plugins/linux-cgroups.plugin/libplugin-linux-cgroups.a \
    $(top_builddir)/src/plugins/linux-diskspace.plugin/libplugin-linux-diskspace.a \
    $(top_builddir)/src/plugins/linux-nfacct.plugin/libplugin-linux-nfacct.a \
    $(top_builddir)/src/plugins/linux-proc.plugin/libplugin-linux-proc.a \
    $(top_builddir)/src/plugins/linux-tc.plugin/libplugin-linux-tc.a \
    $(NULL)

endif
endif

if ENABLE_PLUGIN_CGROUP_NETWORK
plugins_PROGRAMS += cgroup-network
endif

cgroup_network_SOURCES = \
    cgroup-network.c \
	clocks.c \
	clocks.h \
    common.c \
    common.h \
    inlined.h \
	locks.c \
	locks.h \
    log.c \
    log.h \
	procfile.c \
	procfile.h \
	popen.c \
	popen.h \
	signals.c \
	signals.h \
	threads.c \
	threads.h \
    $(NULL)

cgroup_network_LDADD = \
	$(OPTIONAL_MATH_LIBS) \
	$(OPTIONAL_LIBCAP_LIBS) \
	$(NULL)

netdata_LDADD += \
	$(OPTIONAL_MATH_LIBS) \
	$(OPTIONAL_NFACCT_LIBS) \
	$(OPTIONAL_ZLIB_LIBS) \
	$(OPTIONAL_UUID_LIBS) \
	$(NULL)
