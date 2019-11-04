# SPDX-License-Identifier: GPL-3.0-or-later

AUTOMAKE_OPTIONS = subdir-objects
MAINTAINERCLEANFILES = $(srcdir)/Makefile.in

SUBDIRS = \
    api \
    gui \
    server \
    $(NULL)

usersslconfigdir=$(configdir)/ssl

# Explicitly install directories to avoid permission issues due to umask
install-exec-local:
	$(INSTALL) -d $(DESTDIR)$(usersslconfigdir)

dist_noinst_DATA = \
    README.md \
    gui/confluence/README.md \
    gui/custom/README.md \
    $(NULL)
