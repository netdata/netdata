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
	m4/ax_check_enable_debug.m4 \
	m4/ax_c_statement_expressions.m4 \
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
	os-templates/conf.d/linux/health.d/cpu.conf \
	os-templates/conf.d/linux/health.d/ram.conf \
	os-templates/conf.d/linux/health.d/swap.conf \
	os-templates/web/dashboard_info_context_linux.js \
	os-templates/web/dashboard_info_context_post.js \
	os-templates/web/dashboard_info_context_pre.js \
	os-templates/web/dashboard_info_footer.js \
	os-templates/web/dashboard_info_header.js \
	os-templates/web/dashboard_info_menu_linux.js \
	os-templates/web/dashboard_info_menu_post.js \
	os-templates/web/dashboard_info_menu_pre.js \
	os-templates/web/dashboard_info_submenu_linux.js \
	os-templates/web/dashboard_info_submenu_post.js \
	os-templates/web/dashboard_info_submenu_pre.js \
	$(NULL)

# until integrated within build
# should be proper init.d/openrc/systemd usable
dist_noinst_SCRIPTS= \
	diagrams/build.sh \
	coverity-scan.sh \
	docker-build.sh \
	netdata-installer.sh \
	os-templates/generate-dashboard-info-js.sh \
	os-templates/generate-conf.d.sh \
	$(NULL)
