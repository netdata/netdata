# SPDX-License-Identifier: GPL-3.0-or-later

AUTOMAKE_OPTIONS = subdir-objects
MAINTAINERCLEANFILES = $(srcdir)/Makefile.in

CLEANFILES = \
	cgroup-name.sh \
	$(NULL)

include $(top_srcdir)/build/subst.inc
SUFFIXES = .in

dist_plugins_SCRIPTS = \
	cgroup-name.sh \
	cgroup-network-helper.sh \
	$(NULL)

dist_noinst_DATA = \
	cgroup-name.sh.in \
	README.md \
	$(NULL)
