# SPDX-License-Identifier: GPL-3.0-or-later

MAINTAINERCLEANFILES = Makefile.in

AM_CFLAGS = \
	$(OPTIONAL_MATH_CFLAGS) \
	$(OPTIONAL_ZLIB_CFLAGS) \
	$(OPTIONAL_UUID_CFLAGS) \
	$(OPTIONAL_LIBCAP_LIBS) \
	$(NULL)

if ENABLE_PLUGIN_FREEIPMI
plugins_PROGRAMS = apps.plugin
endif

apps_plugin_SOURCES = \
	apps_plugin.c \
	$(NULL)

if FREEBSD
apps_plugin_SOURCES += \
	../../plugin_freebsd.h \
	$(NULL)
endif

apps_plugin_LDADD = \
	../../libnetdata/libnetdata.a \
	$(OPTIONAL_MATH_LIBS) \
	$(OPTIONAL_LIBCAP_LIBS) \
	$(NULL)
