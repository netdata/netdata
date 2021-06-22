# SPDX-License-Identifier: GPL-3.0-or-later

AUTOMAKE_OPTIONS = subdir-objects
MAINTAINERCLEANFILES = $(srcdir)/Makefile.in

CLEANFILES = \
    reset_netdata_trace.sh \
    $(NULL)

include $(top_srcdir)/build/subst.inc
SUFFIXES = .in

userebpfconfigdir=$(configdir)/ebpf.d

# Explicitly install directories to avoid permission issues due to umask
install-exec-local:
	$(INSTALL) -d $(DESTDIR)$(userebpfconfigdir)

dist_plugins_SCRIPTS = \
    reset_netdata_trace.sh \
    $(NULL)

dist_noinst_DATA = \
    reset_netdata_trace.sh.in \
    README.md \
    $(NULL)

ebpfconfigdir=$(libconfigdir)/ebpf.d
dist_libconfig_DATA = \
    ebpf.d.conf \
    $(NULL)

dist_ebpfconfig_DATA = \
    ebpf.d/ebpf_kernel_reject_list.txt \
    ebpf.d/cachestat.conf \
    ebpf.d/dcstat.conf \
    ebpf.d/disk.conf \
    ebpf.d/filesystem.conf \
    ebpf.d/network.conf \
    ebpf.d/process.conf \
    ebpf.d/sync.conf \
    ebpf.d/swap.conf \
    ebpf.d/vfs.conf \
    $(NULL)
