# Copyright (C) 2015 Alon Bar-Lev <alon.barlev@gmail.com>
# SPDX-License-Identifier: GPL-3.0-or-later

MAINTAINERCLEANFILES = $(srcdir)/Makefile.in
CLEANFILES = \
    cron/netdata-updater-daily \
    freebsd/rc.d/netdata \
    initd/init.d/netdata \
    launchd/netdata.plist \
    logrotate/netdata \
    lsb/init.d/netdata \
    openrc/conf.d/netdata \
    openrc/init.d/netdata \
    systemd/netdata.service \
    systemd/netdata.service.v235 \
    systemd/netdata-updater.service \
    $(NULL)

include $(top_srcdir)/build/subst.inc
SUFFIXES = .in

dist_config_SCRIPTS = \
    edit-config \
    $(NULL)

dist_config_DATA = \
    .install-type \
    $(NULL)

libsyscrondir=$(libsysdir)/cron
libsysfreebsddir=$(libsysdir)/freebsd
libsysfreebsdrcddir=$(libsysfreebsddir)/rc.d
libsysinitddir=$(libsysdir)/initd
libsysinitdinitddir=$(libsysinitddir)/init.d
libsyslaunchddir=$(libsysdir)/launchd
libsyslogrotatedir=$(libsysdir)/logrotate
libsyslsbdir=$(libsysdir)/lsb
libsyslsbinitddir=$(libsyslsbdir)/init.d
libsysopenrcdir=$(libsysdir)/openrc
libsysopenrcinitddir=$(libsysopenrcdir)/init.d
libsysopenrcconfddir=$(libsysopenrcdir)/conf.d
libsyssystemddir=$(libsysdir)/systemd

# Explicitly install directories to avoid permission issues due to umask
install-exec-local:
	$(INSTALL) -d $(DESTDIR)$(configdir)
	$(INSTALL) -d $(DESTDIR)$(libsysdir)
	$(INSTALL) -d $(DESTDIR)$(libsyscrondir)
	$(INSTALL) -d $(DESTDIR)$(libsysfreebsdrcddir)
	$(INSTALL) -d $(DESTDIR)$(libsysinitdinitddir)
	$(INSTALL) -d $(DESTDIR)$(libsyslaunchddir)
	$(INSTALL) -d $(DESTDIR)$(libsyslogrotatedir)
	$(INSTALL) -d $(DESTDIR)$(libsyslsbinitddir)
	$(INSTALL) -d $(DESTDIR)$(libsyssystemddir)
	$(INSTALL) -d $(DESTDIR)$(libsysopenrcinitddir)
	$(INSTALL) -d $(DESTDIR)$(libsysopenrcconfddir)

libexecnetdatadir=$(libexecdir)/netdata
nodist_libexecnetdata_SCRIPTS = \
    install-service.sh \
    $(NULL)

nodist_libsyscron_DATA = \
    cron/netdata-updater-daily \
    $(NULL)

nodist_libsysfreebsdrcd_DATA = \
    freebsd/rc.d/netdata \
    $(NULL)

nodist_libsysinitdinitd_DATA = \
    initd/init.d/netdata \
    $(NULL)

nodist_libsyslaunchd_DATA = \
    launchd/netdata.plist \
    $(NULL)

nodist_libsyslogrotate_DATA = \
    logrotate/netdata \
    $(NULL)

nodist_libsyslsbinitd_DATA = \
    lsb/init.d/netdata \
    $(NULL)

nodist_libsysopenrcinitd_DATA = \
    openrc/init.d/netdata \
    $(NULL)

nodist_libsysopenrcconfd_DATA = \
    openrc/conf.d/netdata \
    $(NULL)

nodist_libsyssystemd_DATA = \
    systemd/netdata.service \
    systemd/netdata.service.v235 \
    systemd/netdata-updater.service \
    $(NULL)

dist_libsyssystemd_DATA = \
    systemd/netdata-updater.timer \
    $(NULL)

dist_noinst_DATA = \
    install-service.sh.in \
    netdata.conf \
    cron/netdata-updater-daily.in \
    freebsd/rc.d/netdata.in \
    initd/init.d/netdata.in \
    launchd/netdata.plist.in \
    logrotate/netdata.in \
    lsb/init.d/netdata.in \
    openrc/conf.d/netdata.in \
    openrc/init.d/netdata.in \
    systemd/netdata.service.in \
    systemd/netdata.service.v235.in \
    systemd/netdata-updater.service.in \
    $(NULL)
