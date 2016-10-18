#
# Copyright (C) 2015 Alon Bar-Lev <alon.barlev@gmail.com>
#
AUTOMAKE_OPTIONS=foreign dist-bzip2 dist-xz 1.10
ACLOCAL_AMFLAGS = -I m4

MAINTAINERCLEANFILES= \
	config.log config.status \
	$(srcdir)/Makefile.in \
	$(srcdir)/config.h.in $(srcdir)/config.h.in~ $(srcdir)/configure \
	$(srcdir)/install-sh $(srcdir)/ltmain.sh $(srcdir)/missing \
	$(srcdir)/compile $(srcdir)/depcomp $(srcdir)/aclocal.m4 \
	$(srcdir)/config.guess $(srcdir)/config.sub \
	$(srcdir)/m4/ltsugar.m4 $(srcdir)/m4/libtool.m4 \
	$(srcdir)/m4/ltversion.m4 $(srcdir)/m4/lt~obsolete.m4 \
	$(srcdir)/m4/ltoptions.m4 \
	$(srcdir)/pkcs11-helper.spec $(srcdir)/config-w32-vc.h

EXTRA_DIST = \
	.gitignore \
	autogen.sh \
	README.md \
	LICENSE.md \
	COPYING \
	autogen.sh \
	tests/stress.sh \
	$(NULL)

SUBDIRS = \
	charts.d \
	conf.d \
	node.d \
	plugins.d \
	python.d \
	src \
	system \
	web \
	contrib \
	$(NULL)

dist_noinst_DATA= \
	diagrams/config.puml \
	diagrams/registry.puml \
	configs.signatures \
	Dockerfile \
	netdata.spec \
	$(NULL)

# until integrated within build
# should be proper init.d/openrc/systemd usable
dist_noinst_SCRIPTS= \
	diagrams/build.sh \
	coverity-scan.sh \
	docker-build.sh \
	netdata-installer.sh \
	$(NULL)
