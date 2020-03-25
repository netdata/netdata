# SPDX-License-Identifier: GPL-3.0-or-later

AUTOMAKE_OPTIONS = subdir-objects
MAINTAINERCLEANFILES = $(srcdir)/Makefile.in

CLEANFILES = \
    tests/install-fake-charts.d.sh \
    $(NULL)

include $(top_srcdir)/build/subst.inc
SUFFIXES = .in

#sbin_SCRIPTS = \
#    tests/install-fake-charts.d.sh \
#    $(NULL)

dist_noinst_SCRIPTS = tests/install-fake-charts.d.sh
dist_noinst_DATA = README.md tests/install-fake-charts.d.sh.in

