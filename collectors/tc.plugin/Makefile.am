# SPDX-License-Identifier: GPL-3.0-or-later

AUTOMAKE_OPTIONS = subdir-objects
MAINTAINERCLEANFILES = $(srcdir)/Makefile.in

CLEANFILES = \
    tc-qos-helper.sh \
    $(NULL)

include $(top_srcdir)/build/subst.inc
SUFFIXES = .in

dist_plugins_SCRIPTS = \
    tc-qos-helper.sh \
    $(NULL)

dist_noinst_DATA = \
    tc-qos-helper.sh.in \
    README.md \
    $(NULL)
