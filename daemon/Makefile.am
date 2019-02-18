# SPDX-License-Identifier: GPL-3.0-or-later

AUTOMAKE_OPTIONS = subdir-objects
MAINTAINERCLEANFILES= $(srcdir)/Makefile.in
CLEANFILES = \
	anonymous-statistics.sh \
	$(NULL)

include $(top_srcdir)/build/subst.inc
SUFFIXES = .in

dist_noinst_DATA = \
	README.md \
	config/README.md \
	anonymous-statistics.sh.in \
	$(NULL)

dist_plugins_SCRIPTS = \
	anonymous-statistics.sh \
	$(NULL)
