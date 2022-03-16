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
