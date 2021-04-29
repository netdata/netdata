# SPDX-License-Identifier: GPL-3.0-or-later

AUTOMAKE_OPTIONS = subdir-objects
MAINTAINERCLEANFILES = $(srcdir)/Makefile.in

SUBDIRS = \
    badges \
    queries \
    exporters \
    formatters \
    health \
    $(NULL)

dist_noinst_DATA = \
    README.md \
    $(NULL)

dist_web_DATA = \
    netdata-swagger.yaml \
    netdata-swagger.json \
    $(NULL)
