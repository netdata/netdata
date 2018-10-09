# SPDX-License-Identifier: GPL-3.0-or-later

MAINTAINERCLEANFILES = Makefile.in

AM_CFLAGS = \
	$(OPTIONAL_MATH_CFLAGS) \
	$(OPTIONAL_NFACCT_CLFAGS) \
	$(OPTIONAL_ZLIB_CFLAGS) \
	$(OPTIONAL_UUID_CFLAGS) \
	$(OPTIONAL_LIBCAP_CFLAGS) \
	$(OPTIONAL_IPMIMONITORING_CFLAGS)\
	$(NULL)

noinst_LIBRARIES=libplugin-linux-cgroups.a

libplugin_linux_cgroups_a_SOURCES= \
    sys_fs_cgroup.h \
    sys_fs_cgroup.c \
	$(NULL)
