# SPDX-License-Identifier: GPL-3.0-or-later

MAINTAINERCLEANFILES = Makefile.in

AM_CFLAGS = \
	$(OPTIONAL_MATH_CFLAGS) \
	$(OPTIONAL_ZLIB_CFLAGS) \
	$(OPTIONAL_UUID_CFLAGS) \
	$(NULL)

noinst_LIBRARIES=libplugin-plugins.d.a

libplugin_plugins_d_a_SOURCES = \
	plugins_d.c \
	plugins_d.h \
	$(NULL)
