// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PLUGIN_PROC_H
#define NETDATA_PLUGIN_PROC_H 1

#include "../../daemon/common.h"

#if (TARGET_OS == OS_LINUX)

#define NETDATA_PLUGIN_HOOK_LINUX_PROC \
    { \
        .name = "PLUGIN[proc]", \
        .config_section = CONFIG_SECTION_PLUGINS, \
        .config_name = "proc", \
        .enabled = 1, \
        .thread = NULL, \
        .init_routine = NULL, \
        .start_routine = proc_main \
    },


#define PLUGIN_PROC_CONFIG_NAME "proc"
#define PLUGIN_PROC_NAME PLUGIN_PROC_CONFIG_NAME ".plugin"

extern void *proc_main(void *ptr);

extern int do_proc_net_dev(int update_every, usec_t dt);
extern int do_proc_diskstats(int update_every, usec_t dt);
extern int do_proc_mdstat(int update_every, usec_t dt);
extern int do_proc_net_snmp(int update_every, usec_t dt);
extern int do_proc_net_snmp6(int update_every, usec_t dt);
extern int do_proc_net_netstat(int update_every, usec_t dt);
extern int do_proc_net_stat_conntrack(int update_every, usec_t dt);
extern int do_proc_net_ip_vs_stats(int update_every, usec_t dt);
extern int do_proc_stat(int update_every, usec_t dt);
extern int do_proc_meminfo(int update_every, usec_t dt);
extern int do_proc_vmstat(int update_every, usec_t dt);
extern int do_proc_net_rpc_nfs(int update_every, usec_t dt);
extern int do_proc_net_rpc_nfsd(int update_every, usec_t dt);
extern int do_proc_sys_kernel_random_entropy_avail(int update_every, usec_t dt);
extern int do_proc_interrupts(int update_every, usec_t dt);
extern int do_proc_softirqs(int update_every, usec_t dt);
extern int do_sys_kernel_mm_ksm(int update_every, usec_t dt);
extern int do_proc_loadavg(int update_every, usec_t dt);
extern int do_proc_net_stat_synproxy(int update_every, usec_t dt);
extern int do_proc_net_softnet_stat(int update_every, usec_t dt);
extern int do_proc_uptime(int update_every, usec_t dt);
extern int do_proc_sys_devices_system_edac_mc(int update_every, usec_t dt);
extern int do_proc_sys_devices_system_node(int update_every, usec_t dt);
extern int do_proc_spl_kstat_zfs_arcstats(int update_every, usec_t dt);
extern int do_sys_fs_btrfs(int update_every, usec_t dt);
extern int do_proc_net_sockstat(int update_every, usec_t dt);
extern int do_proc_net_sockstat6(int update_every, usec_t dt);
extern int do_proc_net_sctp_snmp(int update_every, usec_t dt);
extern int do_ipc(int update_every, usec_t dt);
extern int get_numa_node_count(void);

// metrics that need to be shared among data collectors
extern unsigned long long tcpext_TCPSynRetrans;

// netdev renames
extern void netdev_rename_device_add(const char *host_device, const char *container_device, const char *container_name);
extern void netdev_rename_device_del(const char *host_device);

#include "proc_self_mountinfo.h"
#include "zfs_common.h"

#else // (TARGET_OS == OS_LINUX)

#define NETDATA_PLUGIN_HOOK_LINUX_PROC

#endif // (TARGET_OS == OS_LINUX)


#endif /* NETDATA_PLUGIN_PROC_H */
