# SPDX-License-Identifier: GPL-3.0-or-later

AUTOMAKE_OPTIONS = subdir-objects
MAINTAINERCLEANFILES = Makefile.in

AM_CFLAGS = \
	$(OPTIONAL_MATH_CFLAGS) \
	$(OPTIONAL_ZLIB_CFLAGS) \
	$(OPTIONAL_UUID_CFLAGS) \
	$(OPTIONAL_IPMIMONITORING_CFLAGS) \
	$(NULL)

include ../../libnetdata/libnetdata_sources.am

if ENABLE_PLUGIN_FREEIPMI
plugins_PROGRAMS = freeipmi.plugin
endif

freeipmi_plugin_SOURCES = \
	freeipmi_plugin.c \
	$(libnetdata_SOURCES) \
	$(NULL)

freeipmi_plugin_LDADD = \
	$(OPTIONAL_MATH_LIBS) \
	$(OPTIONAL_ZLIB_LIBS) \
	$(OPTIONAL_UUID_LIBS) \
	$(OPTIONAL_IPMIMONITORING_LIBS) \
	$(NULL)
