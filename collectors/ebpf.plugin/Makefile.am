# SPDX-License-Identifier: GPL-3.0-or-later

AUTOMAKE_OPTIONS = subdir-objects
MAINTAINERCLEANFILES = $(srcdir)/Makefile.in

include $(top_srcdir)/build/subst.inc
SUFFIXES = .in

userebpfconfigdir=$(configdir)/ebpf.d

# Explicitly install directories to avoid permission issues due to umask
install-exec-local:
	$(INSTALL) -d $(DESTDIR)$(userebpfconfigdir)

dist_noinst_DATA = \
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
    ebpf.d/fd.conf \
    ebpf.d/filesystem.conf \
    ebpf.d/hardirq.conf \
    ebpf.d/mdflush.conf \
    ebpf.d/mount.conf \
    ebpf.d/network.conf \
    ebpf.d/oomkill.conf \
    ebpf.d/process.conf \
    ebpf.d/shm.conf \
    ebpf.d/softirq.conf \
    ebpf.d/sync.conf \
    ebpf.d/swap.conf \
    ebpf.d/vfs.conf \
    $(NULL)
