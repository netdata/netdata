# SPDX-License-Identifier: GPL-3.0-or-later

AUTOMAKE_OPTIONS = subdir-objects
MAINTAINERCLEANFILES = $(srcdir)/Makefile.in

CLEANFILES = \
    ioping.plugin \
    $(NULL)

include $(top_srcdir)/build/subst.inc
SUFFIXES = .in

dist_plugins_SCRIPTS = \
    ioping.plugin \
    $(NULL)

dist_noinst_DATA = \
    ioping.plugin.in \
    README.md \
    $(NULL)

dist_libconfig_DATA = \
    ioping.conf \
    $(NULL)
