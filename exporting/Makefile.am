# SPDX-License-Identifier: GPL-3.0-or-later

AUTOMAKE_OPTIONS = subdir-objects
MAINTAINERCLEANFILES = $(srcdir)/Makefile.in

SUBDIRS = \
    tests \
    graphite \
    json \
    opentsdb \
    prometheus \
    aws_kinesis \
    pubsub \
    mongodb \
    $(NULL)

dist_libconfig_DATA = \
    exporting.conf \
    $(NULL)

dist_noinst_DATA = \
    README.md \
    TIMESCALE.md \
    WALKTHROUGH.md \
    $(NULL)

dist_noinst_SCRIPTS = \
    nc-exporting.sh \
    $(NULL)
