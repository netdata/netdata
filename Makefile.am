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
	.codeclimate.yml \
	.csslintrc \
	.eslintignore \
	.eslintrc \
	.travis \
	m4/jemalloc.m4 \
    m4/ax_c___atomic.m4 \
    m4/ax_check_enable_debug.m4 \
    m4/ax_c_mallinfo.m4 \
    m4/ax_gcc_func_attribute.m4 \
    m4/ax_check_compile_flag.m4 \
    m4/ax_c_statement_expressions.m4 \
    m4/ax_pthread.m4 \
    m4/ax_c_lto.m4 \
    m4/ax_c_mallopt.m4 \
    m4/tcmalloc.m4 \
    m4/ax_c__generic.m4 \
	autogen.sh \
	README.md \
	LICENSE \
	LICENSE.md \
	LICENSE-REDISTRIBUTED.md \
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
	diagrams/netdata-for-ephemeral-nodes.xml \
	diagrams/netdata-proxies-example.xml \
	diagrams/netdata-overview.xml \
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
	installer/functions.sh \
	makeself/build.sh \
	makeself/makeself.sh \
	makeself/makeself-license.txt \
	makeself/setup-x86_64-static.sh \
	makeself/post-installer.sh \
	makeself/jobs/10-prepare-destination.install.sh \
	makeself/jobs/50-curl-7.53.1.install.sh \
	makeself/jobs/50-bash-4.4.install.sh \
	makeself/jobs/50-fping-4.0.install.sh \
	makeself/jobs/70-netdata-git.install.sh \
	makeself/jobs/99-makeself.install.sh \
	makeself/run-all-jobs.sh \
	makeself/install-or-update.sh \
	makeself/build-x86_64-static.sh \
	makeself/makeself-header.sh \
	makeself/makeself-help-header.txt \
	makeself/makeself.lsm \
	makeself/functions.sh \
	$(NULL)
