# SPDX-License-Identifier: GPL-3.0-or-later

AUTOMAKE_OPTIONS = subdir-objects
MAINTAINERCLEANFILES= $(srcdir)/Makefile.in

SUBDIRS = \
	$(NULL)

AM_CFLAGS = \
	$(OPTIONAL_MATH_CFLAGS) \
	$(OPTIONAL_NFACCT_CLFAGS) \
	$(OPTIONAL_ZLIB_CFLAGS) \
	$(OPTIONAL_UUID_CFLAGS) \
	$(NULL)

sbin_PROGRAMS = netdata
dist_cache_DATA = .keep
dist_varlib_DATA = .keep
dist_registry_DATA = .keep
dist_log_DATA = .keep
plugins_PROGRAMS =

include libnetdata/libnetdata_sources.am
include plugins/checks.plugin/plugin_sources.am
include plugins/freebsd.plugin/plugin_sources.am
include plugins/health.plugin/plugin_sources.am
include plugins/idlejitter.plugin/plugin_sources.am
include plugins/linux-cgroups.plugin/plugin_sources.am
include plugins/linux-diskspace.plugin/plugin_sources.am
include plugins/linux-nfacct.plugin/plugin_sources.am
include plugins/linux-proc.plugin/plugin_sources.am
include plugins/linux-tc.plugin/plugin_sources.am
include plugins/macos.plugin/plugin_sources.am
include plugins/plugins.d.plugin/plugin_sources.am
include plugins/statsd.plugin/plugin_sources.am

netdata_SOURCES = \
    plugins/all.h \
	backend_prometheus.c \
	backend_prometheus.h \
	backends.c \
	backends.h \
	common.c \
	common.h \
	daemon.c \
	daemon.h \
	global_statistics.c \
	global_statistics.h \
	main.c \
	main.h \
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
	rrdcalc.h \
	rrdcalctemplate.c \
	rrdcalctemplate.h \
	rrddim.c \
	rrddimvar.c \
	rrddimvar.h \
	rrdfamily.c \
	rrdhost.c \
	rrdpush.c \
	rrdpush.h \
	rrdset.c \
	rrdsetvar.c \
	rrdsetvar.h \
	rrdvar.c \
	rrdvar.h \
	signals.c \
	signals.h \
	unit_test.c \
	unit_test.h \
	url.c \
	url.h \
	web_api_v1.c \
	web_api_v1.h \
	web_buffer_svg.c \
	web_buffer_svg.h \
	web_client.c \
	web_client.h \
	web_server.c \
	web_server.h \
	$(libnetdata_SOURCES) \
	$(checks_SOURCES) \
	$(health_SOURCES) \
	$(idlejitter_SOURCES) \
	$(pluginsd_SOURCES) \
	$(statsd_SOURCES) \
	$(NULL)

netdata_LDADD = \
	$(NULL)

if FREEBSD
    netdata_SOURCES += \
        $(freebsd_SOURCES) \
        $(NULL)
endif

if MACOS
    netdata_SOURCES += \
        $(macos_SOURCES) \
        $(NULL)
endif

if LINUX
    netdata_SOURCES += \
        $(cgroups_SOURCES) \
        $(diskspace_SOURCES) \
        $(nfacct_SOURCES) \
        $(proc_SOURCES) \
        $(tc_SOURCES) \
        $(NULL)
endif

netdata_LDADD += \
	$(OPTIONAL_MATH_LIBS) \
	$(OPTIONAL_NFACCT_LIBS) \
	$(OPTIONAL_ZLIB_LIBS) \
	$(OPTIONAL_UUID_LIBS) \
	$(NULL)
