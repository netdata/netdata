# SPDX-License-Identifier: GPL-3.0-or-later

AUTOMAKE_OPTIONS = subdir-objects
MAINTAINERCLEANFILES = $(srcdir)/Makefile.in

CLEANFILES = \
	health_mgmtapi/health-cmdapi-test.sh \
	acls/acl.sh \
	urls/request.sh \
	alarm_repetition/alarm.sh \
	template_dimension/template_dim.sh \
	$(NULL)

include $(top_srcdir)/build/subst.inc
SUFFIXES = .in

dist_noinst_DATA = \
    README.md \
    web/lib/jasmine-jquery.js \
    web/easypiechart.chart.spec.js \
    web/easypiechart.percentage.spec.js \
    web/karma.conf.js \
    web/fixtures/easypiechart.chart.fixture1.html \
    node.d/fronius.chart.spec.js \
    node.d/fronius.parse.spec.js \
    node.d/fronius.process.spec.js \
    node.d/fronius.validation.spec.js \
    health_mgmtapi/health-cmdapi-test.sh.in \
    acls/acl.sh.in \
    urls/request.sh.in \
    alarm_repetition/alarm.sh.in \
    template_dimension/template_dim.sh.in \
	$(NULL)

dist_plugins_SCRIPTS = \
	health_mgmtapi/health-cmdapi-test.sh \
	acls/acl.sh \
	urls/request.sh \
	alarm_repetition/alarm.sh \
	template_dimension/template_dim.sh \
	$(NULL)

dist_noinst_SCRIPTS = \
	stress.sh \
	$(NULL)

