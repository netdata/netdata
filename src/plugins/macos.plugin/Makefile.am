# SPDX-License-Identifier: GPL-3.0-or-later

MAINTAINERCLEANFILES = Makefile.in

AM_CFLAGS = \
	$(OPTIONAL_MATH_CFLAGS) \
	$(OPTIONAL_ZLIB_CFLAGS) \
	$(OPTIONAL_UUID_CFLAGS) \
	$(NULL)

noinst_LIBRARIES=libplugin-macos.a

libplugin_macos_a_SOURCES = \
	plugin_macos.c \
	plugin_macos.h \
	macos_sysctl.c \
	macos_mach_smi.c \
	macos_fw.c \
	$(NULL)
