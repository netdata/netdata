# SPDX-License-Identifier: GPL-3.0-or-later

MAINTAINERCLEANFILES = Makefile.in

AM_CFLAGS = \
	$(OPTIONAL_MATH_CFLAGS) \
	$(OPTIONAL_ZLIB_CFLAGS) \
	$(OPTIONAL_UUID_CFLAGS) \
	$(NULL)

noinst_LIBRARIES=libplugin-linux-nfacct.a

libplugin_linux_nfacct_a_SOURCES = \
	plugin_nfacct.c \
	plugin_nfacct.h \
	$(NULL)
