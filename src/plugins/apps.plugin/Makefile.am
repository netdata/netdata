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

# FIXME: we should not reference parent files like this
# we need to build a libnetdata.a and reference that

apps_plugin_SOURCES = \
	apps_plugin.c \
	../../avl.c \
	../../avl.h \
	../../clocks.c \
	../../clocks.h \
	../../common.c \
	../../common.h \
	../../inlined.h \
	../../locks.c \
	../../locks.h \
	../../log.c log.h \
	../../procfile.c \
	../../procfile.h \
	../../threads.c \
	../../threads.h \
	../../web_buffer.c \
	../../web_buffer.h \
	$(NULL)

if FREEBSD
apps_plugin_SOURCES += \
	../../plugin_freebsd.h \
	$(NULL)
else
apps_plugin_SOURCES += \
	../../adaptive_resortable_list.c \
	../../adaptive_resortable_list.h \
	$(NULL)
endif

apps_plugin_LDADD = \
	$(OPTIONAL_MATH_LIBS) \
	$(OPTIONAL_LIBCAP_LIBS) \
	$(NULL)
