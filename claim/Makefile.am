# SPDX-License-Identifier: GPL-3.0-or-later

AUTOMAKE_OPTIONS = subdir-objects
MAINTAINERCLEANFILES = $(srcdir)/Makefile.in

CLEANFILES = \
    netdata-claim.sh \
    $(NULL)

include $(top_srcdir)/build/subst.inc
SUFFIXES = .in

sbin_SCRIPTS = \
    netdata-claim.sh \
    $(NULL)

dist_noinst_DATA = \
    netdata-claim.sh.in \
    README.md \
    $(NULL)

