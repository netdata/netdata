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
	.codacy.yml \
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
	CONTRIBUTORS.md \
	COPYING \
	autogen.sh \
	$(NULL)

SUBDIRS = \
	charts.d \
	conf.d \
	diagrams \
	makeself \
	node.d \
	plugins.d \
	python.d \
	src \
	system \
	web \
	contrib \
	tests \
	$(NULL)

dist_noinst_DATA= \
	cppcheck.sh \
	configs.signatures \
	Dockerfile \
	Dockerfile.alpine \
	Dockerfile.aarch64 \
	Dockerfile.armv7hf \
	netdata.cppcheck \
	netdata.spec \
	package.json \
	$(NULL)

# until integrated within build
# should be proper init.d/openrc/systemd usable
dist_noinst_SCRIPTS= \
	coverity-scan.sh \
	docker-build.sh \
	kickstart.sh \
	kickstart-static64.sh \
	netdata-installer.sh \
	installer/functions.sh \
	$(NULL)
