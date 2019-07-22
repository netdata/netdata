#
# Copyright (C) 2015 Alon Bar-Lev <alon.barlev@gmail.com>
# SPDX-License-Identifier: GPL-3.0-or-later
#
MAINTAINERCLEANFILES= $(srcdir)/Makefile.in
CLEANFILES = \
    edit-config \
	netdata-openrc \
	netdata.logrotate \
	netdata.service \
	netdata-init-d \
	netdata-lsb \
	netdata-freebsd \
	netdata.plist \
	$(NULL)

include $(top_srcdir)/build/subst.inc
SUFFIXES = .in

dist_config_SCRIPTS = \
    $(NULL)

nodist_noinst_DATA = \
	netdata-openrc \
	netdata.logrotate \
	netdata.service \
	netdata-init-d \
	netdata-lsb \
	netdata-freebsd \
	netdata.plist \
	$(NULL)

dist_noinst_DATA = \
	edit-config.in \
	netdata-openrc.in \
	netdata.logrotate.in \
	netdata.service.in \
	netdata-init-d.in \
	netdata-lsb.in \
	netdata-freebsd.in \
	netdata.plist.in \
	netdata.conf \
	$(NULL)
