# SPDX-License-Identifier: GPL-3.0-or-later

AUTOMAKE_OPTIONS = subdir-objects
MAINTAINERCLEANFILES = $(srcdir)/Makefile.in

userlogsmanagconfigdir=$(configdir)/logsmanagement.d

# Explicitly install directories to avoid permission issues due to umask
install-exec-local:
	$(INSTALL) -d $(DESTDIR)$(userlogsmanagconfigdir)

dist_libconfig_DATA = \
    stock_conf/logsmanagement.d.conf \
    $(NULL)

logsmanagconfigdir=$(libconfigdir)/logsmanagement.d

dist_logsmanagconfig_DATA = \
    stock_conf/logsmanagement.d/default.conf \
    stock_conf/logsmanagement.d/example_forward.conf \
    stock_conf/logsmanagement.d/example_mqtt.conf \
    stock_conf/logsmanagement.d/example_serial.conf \
    stock_conf/logsmanagement.d/example_syslog.conf \
    $(NULL)

dist_noinst_DATA = \
    README.md \
    stress_test/logrotate.conf \
    stress_test/logs_query.html \
    stress_test/run_stress_test.sh \
    stress_test/stress_test.c \
    $(NULL)
