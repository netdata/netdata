# SPDX-License-Identifier: GPL-3.0-or-later

AUTOMAKE_OPTIONS = subdir-objects
MAINTAINERCLEANFILES = $(srcdir)/Makefile.in
CLEANFILES = \
    networkviewer.plugin \
    $(NULL)

dist_noinst_DATA = \
    README.md \
    $(NULL)

include $(top_srcdir)/build/subst.inc
SUFFIXES = .in

dist_libconfig_DATA = \
    network_viewer.conf \
    $(NULL)
