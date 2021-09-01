# SPDX-License-Identifier: GPL-3.0-or-later

AUTOMAKE_OPTIONS = subdir-objects
MAINTAINERCLEANFILES = $(srcdir)/Makefile.in

CLEANFILES = \
    $(srcdir)/health_mgmtapi/health-cmdapi-test.sh \
    $(srcdir)/acls/acl.sh \
    $(srcdir)/urls/request.sh \
    $(srcdir)/alarm_repetition/alarm.sh \
    $(srcdir)/template_dimension/template_dim.sh \
    $(NULL)

include $(top_srcdir)/build/subst.inc
SUFFIXES = .in

dist_noinst_DATA = \
    $(srcdir)/README.md \
    $(srcdir)/web/lib/jasmine-jquery.js \
    $(srcdir)/web/easypiechart.chart.spec.js \
    $(srcdir)/web/easypiechart.percentage.spec.js \
    $(srcdir)/web/karma.conf.js \
    $(srcdir)/web/fixtures/easypiechart.chart.fixture1.html \
    $(srcdir)/node.d/fronius.chart.spec.js \
    $(srcdir)/node.d/fronius.parse.spec.js \
    $(srcdir)/node.d/fronius.process.spec.js \
    $(srcdir)/node.d/fronius.validation.spec.js \
    $(srcdir)/health_mgmtapi/health-cmdapi-test.sh.in \
    $(srcdir)/acls/acl.sh.in \
    $(srcdir)/urls/request.sh.in \
    $(srcdir)/alarm_repetition/alarm.sh.in \
    $(srcdir)/template_dimension/template_dim.sh.in \
    $(NULL)

dist_plugins_SCRIPTS = \
    $(srcdir)/health_mgmtapi/health-cmdapi-test.sh \
    $(srcdir)/acls/acl.sh \
    $(srcdir)/urls/request.sh \
    $(srcdir)/alarm_repetition/alarm.sh \
    $(srcdir)/template_dimension/template_dim.sh \
    $(NULL)

dist_noinst_SCRIPTS = \
    $(srcdir)/stress.sh \
    $(NULL)
