# SPDX-License-Identifier: GPL-3.0-or-later

AUTOMAKE_OPTIONS = subdir-objects
MAINTAINERCLEANFILES = $(srcdir)/Makefile.in

CLEANFILES = \
	alarm-notify.sh \
	$(NULL)

include $(top_srcdir)/build/subst.inc
SUFFIXES = .in

dist_plugins_SCRIPTS = \
	alarm-notify.sh \
	alarm-email.sh \
	alarm-test.sh \
	$(NULL)

dist_noinst_DATA = \
	alarm-notify.sh.in \
	$(NULL)
