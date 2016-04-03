#
# Copyright (C) 2015 Alon Bar-Lev <alon.barlev@gmail.com>
#
MAINTAINERCLEANFILES= $(srcdir)/Makefile.in
CLEANFILES = \
	netdata-openrc \
	netdata.service \
	$(NULL)

include $(top_srcdir)/build/subst.inc

SUFFIXES = .in

nodist_noinst_DATA = \
	netdata-openrc \
	netdata.service \
	$(NULL)

dist_noinst_DATA = \
	netdata-openrc.in \
	netdata.service.in \
	$(NULL)
