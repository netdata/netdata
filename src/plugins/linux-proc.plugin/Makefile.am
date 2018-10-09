# SPDX-License-Identifier: GPL-3.0-or-later

MAINTAINERCLEANFILES = Makefile.in

AM_CFLAGS = \
	$(OPTIONAL_MATH_CFLAGS) \
	$(OPTIONAL_ZLIB_CFLAGS) \
	$(OPTIONAL_UUID_CFLAGS) \
	$(NULL)

noinst_LIBRARIES=libplugin-linux-proc.a

libplugin_linux_proc_a_SOURCES = \
	ipc.c \
	ipc.h \
	plugin_proc.c \
	plugin_proc.h \
	proc_diskstats.c \
	proc_interrupts.c \
	proc_softirqs.c \
	proc_loadavg.c \
	proc_meminfo.c \
	proc_net_dev.c \
	proc_net_ip_vs_stats.c \
	proc_net_netstat.c \
	proc_net_rpc_nfs.c \
	proc_net_rpc_nfsd.c \
	proc_net_snmp.c \
	proc_net_snmp6.c \
	proc_net_sctp_snmp.c \
	proc_net_sockstat.c \
	proc_net_sockstat6.c \
	proc_net_softnet_stat.c \
	proc_net_stat_conntrack.c \
	proc_net_stat_synproxy.c \
	proc_self_mountinfo.c \
	proc_self_mountinfo.h \
	zfs_common.c \
    zfs_common.h \
	proc_spl_kstat_zfs.c \
	proc_stat.c \
	proc_sys_kernel_random_entropy_avail.c \
	proc_vmstat.c \
	proc_uptime.c \
	sys_kernel_mm_ksm.c \
	sys_devices_system_edac_mc.c \
	sys_devices_system_node.c \
	sys_fs_btrfs.c \
	$(NULL)
