# SPDX-License-Identifier: GPL-3.0-or-later

MAINTAINERCLEANFILES = Makefile.in

AM_CFLAGS = \
	$(OPTIONAL_MATH_CFLAGS) \
	$(OPTIONAL_ZLIB_CFLAGS) \
	$(OPTIONAL_UUID_CFLAGS) \
	$(NULL)

plugins_PROGRAMS=

noinst_LIBRARIES=libplugin-linux-cgroups.a

libplugin_linux_cgroups_a_SOURCES= \
	sys_fs_cgroup.h \
	sys_fs_cgroup.c \
	$(NULL)

if ENABLE_PLUGIN_CGROUP_NETWORK
plugins_PROGRAMS += cgroup-network

cgroup_network_SOURCES = \
    cgroup-network.c \
	../../clocks.c \
	../../clocks.h \
	../../common.c \
	../../common.h \
	../../inlined.h \
	../../locks.c \
	../../locks.h \
	../../log.c \
	../../log.h \
	../../procfile.c \
	../../procfile.h \
	../../popen.c \
	../../popen.h \
	../../signals.c \
	../../signals.h \
	../../threads.c \
	../../threads.h \
	$(NULL)

cgroup_network_LDADD = \
	$(OPTIONAL_MATH_LIBS) \
	$(OPTIONAL_LIBCAP_LIBS) \
	$(NULL)

endif
