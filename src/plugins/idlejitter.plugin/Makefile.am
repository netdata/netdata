# SPDX-License-Identifier: GPL-3.0-or-later

MAINTAINERCLEANFILES = Makefile.in

AM_CFLAGS = \
	$(OPTIONAL_MATH_CFLAGS) \
	$(OPTIONAL_ZLIB_CFLAGS) \
	$(OPTIONAL_UUID_CFLAGS) \
	$(NULL)

noinst_LIBRARIES=libplugin-idlejitter.a

libplugin_idlejitter_a_SOURCES = \
	plugin_idlejitter.c \
	plugin_idlejitter.h \
	$(NULL)
