#
# Copyright (C) 2015 Alon Bar-Lev <alon.barlev@gmail.com>
#
MAINTAINERCLEANFILES= $(srcdir)/Makefile.in
CLEANFILES = \
	netdata-openrc \
	netdata.logrotate \
	netdata.service \
	netdata-init-d \
	netdata-lsb \
	$(NULL)

include $(top_srcdir)/build/subst.inc

SUFFIXES = .in

nodist_noinst_DATA = \
	netdata-openrc \
	netdata.logrotate \
	netdata.service \
	netdata-init-d \
	netdata-lsb \
	$(NULL)

dist_noinst_DATA = \
	netdata-openrc.in \
	netdata.logrotate.in \
	netdata.service.in \
	netdata-init-d.in \
	netdata-lsb.in \
	netdata.conf \
	$(NULL)
