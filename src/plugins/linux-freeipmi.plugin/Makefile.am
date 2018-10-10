# SPDX-License-Identifier: GPL-3.0-or-later

MAINTAINERCLEANFILES = Makefile.in

AM_CFLAGS = \
	$(OPTIONAL_MATH_CFLAGS) \
	$(OPTIONAL_ZLIB_CFLAGS) \
	$(OPTIONAL_UUID_CFLAGS) \
	$(OPTIONAL_IPMIMONITORING_CFLAGS) \
	$(NULL)

if ENABLE_PLUGIN_FREEIPMI
plugins_PROGRAMS = freeipmi.plugin
endif

freeipmi_plugin_SOURCES = \
	freeipmi_plugin.c \
	$(NULL)

freeipmi_plugin_LDADD = \
	../../libnetdata/libnetdata.a \
	$(OPTIONAL_IPMIMONITORING_LIBS) \
	$(NULL)
