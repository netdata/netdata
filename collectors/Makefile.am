# SPDX-License-Identifier: GPL-3.0-or-later

MAINTAINERCLEANFILES = $(srcdir)/Makefile.in

SUBDIRS = \
    plugins.d \
    apps.plugin \
    cgroups.plugin \
    charts.d.plugin \
    cups.plugin \
    debugfs.plugin \
    diskspace.plugin \
    timex.plugin \
    ioping.plugin \
    freebsd.plugin \
    freeipmi.plugin \
    idlejitter.plugin \
    macos.plugin \
    nfacct.plugin \
    xenstat.plugin \
    perf.plugin \
    proc.plugin \
    python.d.plugin \
    slabinfo.plugin \
    statsd.plugin \
    ebpf.plugin \
    tc.plugin \
    $(NULL)

usercustompluginsconfigdir=$(configdir)/custom-plugins.d
usergoconfigdir=$(configdir)/go.d

# Explicitly install directories to avoid permission issues due to umask
install-exec-local:
	$(INSTALL) -d $(DESTDIR)$(usercustompluginsconfigdir)
	$(INSTALL) -d $(DESTDIR)$(usergoconfigdir)

dist_noinst_DATA = \
    README.md \
    $(NULL)
