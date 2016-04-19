#
# Copyright (C) 2015 Alon Bar-Lev <alon.barlev@gmail.com>
#
MAINTAINERCLEANFILES= $(srcdir)/Makefile.in
CLEANFILES = \
	netdata-openrc \
	netdata.logrotate \
	netdata.service \
	netdata-init-d \
	$(NULL)

include $(top_srcdir)/build/subst.inc

SUFFIXES = .in

nodist_noinst_DATA = \
	netdata-openrc \
	netdata.logrotate \
	netdata.service \
	netdata-init-d \
	$(NULL)

dist_noinst_DATA = \
	netdata-openrc.in \
	netdata.logrotate.in \
	netdata.service.in \
	netdata-init-d.in \
	netdata.conf \
	$(NULL)
