# SPDX-License-Identifier: GPL-3.0-or-later

AUTOMAKE_OPTIONS = subdir-objects
MAINTAINERCLEANFILES = $(srcdir)/Makefile.in

CLEANFILES = \
    reset_netdata_trace.sh \
    $(NULL)

include $(top_srcdir)/build/subst.inc
SUFFIXES = .in

dist_plugins_SCRIPTS = \
    reset_netdata_trace.sh \
    $(NULL)

dist_noinst_DATA = \
    reset_netdata_trace.sh.in \
    README.md \
    $(NULL)

dist_libconfig_DATA = \
    ebpf.conf \
    ebpf_kernel_reject_list.txt \
    $(NULL)
