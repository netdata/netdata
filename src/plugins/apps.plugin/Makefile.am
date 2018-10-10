# SPDX-License-Identifier: GPL-3.0-or-later

AUTOMAKE_OPTIONS = subdir-objects
MAINTAINERCLEANFILES = Makefile.in

AM_CFLAGS = \
	$(OPTIONAL_MATH_CFLAGS) \
	$(OPTIONAL_ZLIB_CFLAGS) \
	$(OPTIONAL_UUID_CFLAGS) \
	$(OPTIONAL_LIBCAP_LIBS) \
	$(NULL)

apps_dir = $(top_srcdir)/src/plugins/apps.plugin
freebsd_dir = $(top_srcdir)/src/plugins/freebsd.plugin

if ENABLE_PLUGIN_APPS
plugins_PROGRAMS = apps.plugin
endif

include ../../libnetdata/libnetdata_sources.am

apps_plugin_SOURCES = \
	$(apps_dir)/apps_plugin.c \
	$(libnetdata_SOURCES) \
	$(NULL)

if FREEBSD
apps_plugin_SOURCES += \
	$(freebsd_dir)/plugin_freebsd.h \
	$(NULL)
endif

apps_plugin_LDADD = \
	$(OPTIONAL_MATH_LIBS) \
	$(OPTIONAL_LIBCAP_LIBS) \
	$(NULL)
