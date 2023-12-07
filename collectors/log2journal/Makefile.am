# SPDX-License-Identifier: GPL-3.0-or-later

AUTOMAKE_OPTIONS = subdir-objects
MAINTAINERCLEANFILES = $(srcdir)/Makefile.in

dist_noinst_DATA = \
    tests.sh \
    README.md \
    tests.d/* \
    $(NULL)

log2journalconfigdir=$(libconfigdir)/log2journal.d
dist_log2journalconfig_DATA = \
    log2journal.d/nginx-combined.yaml \
    log2journal.d/nginx-json.yaml \
    log2journal.d/default.yaml \
    $(NULL)
