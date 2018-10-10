# SPDX-License-Identifier: GPL-3.0-or-later

MAINTAINERCLEANFILES = Makefile.in

AM_CFLAGS = \
	$(OPTIONAL_MATH_CFLAGS) \
	$(OPTIONAL_ZLIB_CFLAGS) \
	$(OPTIONAL_UUID_CFLAGS) \
	$(NULL)

noinst_LIBRARIES=libplugin-freebsd.a

libplugin_freebsd_a_SOURCES = \
	plugin_freebsd.c \
	plugin_freebsd.h \
	freebsd_sysctl.c \
	freebsd_getmntinfo.c \
	freebsd_getifaddrs.c \
	freebsd_devstat.c \
	freebsd_kstat_zfs.c \
	freebsd_ipfw.c \
	../linux-proc.plugin/zfs_common.c \
	../linux-proc.plugin/zfs_common.h \
	$(NULL)
