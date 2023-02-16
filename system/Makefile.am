# Copyright (C) 2015 Alon Bar-Lev <alon.barlev@gmail.com>
# SPDX-License-Identifier: GPL-3.0-or-later

MAINTAINERCLEANFILES = $(srcdir)/Makefile.in
CLEANFILES = \
    netdata.logrotate \
    cron/netdata-updater-daily.in \
    freebsd/rc.d/netdata \
    initd/init.d/netdata \
    launchd/netdata.plist \
    lsb/init.d/netdata \
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
libsyslsbdir=$(libsysdir)/lsb
libsyslsbinitddir=$(libsyslsbdir)/init.d
libsysopenrcdir=$(libsysdir)/openrc
libsysopenrcinitddir=$(libsysopenrcdir)/init.d
libsyssystemddir=$(libsysdir)/systemd

# Explicitly install directories to avoid permission issues due to umask
install-exec-local:
	$(INSTALL) -d $(DESTDIR)$(configdir)
	$(INSTALL) -d $(DESTDIR)$(libsysdir)
	$(INSTALL) -d $(DESTDIR)$(libsyscrondir)
	$(INSTALL) -d $(DESTDIR)$(libsysfreebsdrcddir)
	$(INSTALL) -d $(DESTDIR)$(libsysinitdinitddir)
	$(INSTALL) -d $(DESTDIR)$(libsyslaunchddir)
	$(INSTALL) -d $(DESTDIR)$(libsyslsbinitddir)
	$(INSTALL) -d $(DESTDIR)$(libsyssystemddir)
	$(INSTALL) -d $(DESTDIR)$(libsysopenrcinitddir)

libexecnetdatadir=$(libexecdir)/netdata
nodist_libexecnetdata_SCRIPTS = \
    install-service.sh \
    $(NULL)

nodist_libsys_DATA = \
    netdata.logrotate \
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

nodist_libsyslsbinitd_DATA = \
    lsb/init.d/netdata \
    $(NULL)

nodist_libsysopenrcinitd_DATA = \
    openrc/init.d/netdata \
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
    netdata.logrotate.in \
    netdata.conf \
    cron/netdata-updater-daily.in \
    freebsd/rc.d/netdata.in \
    initd/init.d/netdata.in \
    launchd/netdata.plist.in \
    lsb/init.d/netdata.in \
    openrc/init.d/netdata.in \
    systemd/netdata.service.in \
    systemd/netdata.service.v235.in \
    systemd/netdata-updater.service.in \
    $(NULL)
