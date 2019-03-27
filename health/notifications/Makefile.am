# SPDX-License-Identifier: GPL-3.0-or-later

AUTOMAKE_OPTIONS = subdir-objects
MAINTAINERCLEANFILES = $(srcdir)/Makefile.in

CLEANFILES = \
	alarm-notify.sh \
	$(NULL)

include $(top_srcdir)/build/subst.inc
SUFFIXES = .in

dist_libconfig_DATA = \
    health_alarm_notify.conf \
    health_email_recipients.conf \
    $(NULL)

dist_plugins_SCRIPTS = \
	alarm-notify.sh \
	alarm-email.sh \
	alarm-test.sh \
	$(NULL)

dist_noinst_DATA = \
	alarm-notify.sh.in \
	README.md \
	$(NULL)

include alerta/Makefile.inc
include awssns/Makefile.inc
include discord/Makefile.inc
include email/Makefile.inc
include flock/Makefile.inc
include irc/Makefile.inc
include kavenegar/Makefile.inc
include messagebird/Makefile.inc
include pagerduty/Makefile.inc
include pushbullet/Makefile.inc
include pushover/Makefile.inc
include rocketchat/Makefile.inc
include slack/Makefile.inc
include smstools3/Makefile.inc
include syslog/Makefile.inc
include telegram/Makefile.inc
include twilio/Makefile.inc
include web/Makefile.inc
include custom/Makefile.inc