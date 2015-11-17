#
# Copyright (C) 2015 Alon Bar-Lev <alon.barlev@gmail.com>
#
MAINTAINERCLEANFILES= $(srcdir)/Makefile.in

dist_web_DATA = \
	datasource.html \
	index.html \
	index.js \
	netdata.js \
	robots.txt \
	theme.css \
	$(NULL)

install-data-hook:
	if [ `id -u` == 0 ]; then \
		getent group netdata > /dev/null || groupadd -r netdata; \
		getent passwd netdata > /dev/null || useradd -r -g netdata -c netdata -s /sbin/nologin -d / netdata; \
		chown -c -R netdata $(DESTDIR)$(cachedir) $(DESTDIR)$(webdir); \
	fi
