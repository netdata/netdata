#
# Copyright (C) 2015 Alon Bar-Lev <alon.barlev@gmail.com>
# SPDX-License-Identifier: GPL-3.0-or-later
#
MAINTAINERCLEANFILES= $(srcdir)/Makefile.in
CLEANFILES = \
	alarm-notify.sh \
	charts.d.plugin \
	fping.plugin \
	node.d.plugin \
	python.d.plugin \
	tc-qos-helper.sh \
	$(NULL)

include $(top_srcdir)/build/subst.inc

SUFFIXES = .in

dist_plugins_DATA = \
	README.md \
	$(NULL)

dist_plugins_SCRIPTS = \
	alarm-email.sh \
	alarm-notify.sh \
	alarm-test.sh \
	charts.d.dryrun-helper.sh \
	charts.d.plugin \
	fping.plugin \
	node.d.plugin \
	python.d.plugin \
	tc-qos-helper.sh \
	loopsleepms.sh.inc \
	$(NULL)

dist_noinst_DATA = \
	alarm-notify.sh.in \
	charts.d.plugin.in \
	fping.plugin.in \
	node.d.plugin.in \
	python.d.plugin.in \
	tc-qos-helper.sh.in \
	$(NULL)
