# SPDX-License-Identifier: GPL-3.0-or-later

MAINTAINERCLEANFILES = Makefile.in

SUBDIRS = \
    apps.plugin \
    checks.plugin \
    idlejitter.plugin \
    plugins.d.plugin \
    statsd.plugin \
   	$(NULL)

if FREEBSD

SUBDIRS += \
    freebsd.plugin \
   	$(NULL)

else
if MACOS

SUBDIRS += \
    macos.plugin \
   	$(NULL)

else

SUBDIRS += \
    linux-cgroups.plugin \
    linux-diskspace.plugin \
    linux-freeipmi.plugin \
    linux-nfacct.plugin \
    linux-proc.plugin \
    linux-tc.plugin \
   	$(NULL)

endif
endif
