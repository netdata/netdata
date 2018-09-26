#
# Copyright (C) 2015 Alon Bar-Lev <alon.barlev@gmail.com>
# SPDX-License-Identifier: GPL-3.0+
#
AUTOMAKE_OPTIONS=foreign 1.10
ACLOCAL_AMFLAGS = -I build/m4

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
	.lgtm.yml \
	.travis \
	build/build.sh
	build/Dockerfile
	build/m4/jemalloc.m4 \
	build/m4/ax_c___atomic.m4 \
	build/m4/ax_check_enable_debug.m4 \
	build/m4/ax_c_mallinfo.m4 \
	build/m4/ax_gcc_func_attribute.m4 \
	build/m4/ax_check_compile_flag.m4 \
	build/m4/ax_c_statement_expressions.m4 \
	build/m4/ax_pthread.m4 \
	build/m4/ax_c_lto.m4 \
	build/m4/ax_c_mallopt.m4 \
	build/m4/tcmalloc.m4 \
	build/m4/ax_c__generic.m4 \
	README.md \
	CONTRIBUTORS.md \
	LICENSE \
	REDISTRIBUTED.md \
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
	docker \
	netdata.cppcheck \
	netdata.spec \
	package.json \
	$(NULL)

# until integrated within build
# should be proper init.d/openrc/systemd usable
dist_noinst_SCRIPTS= \
	coverity-scan.sh \
	kickstart.sh \
	kickstart-static64.sh \
	netdata-installer.sh \
	installer/functions.sh \
	$(NULL)
