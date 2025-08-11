// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PLUGIN_PROC_H
#define NETDATA_PLUGIN_PROC_H 1

#include "database/rrd.h"

#define PLUGIN_PROC_CONFIG_NAME "proc"
#define PLUGIN_PROC_NAME PLUGIN_PROC_CONFIG_NAME ".plugin"

#define THREAD_NETDEV_NAME "P[proc netdev]"
void netdev_main(void *ptr_is_null);

int do_proc_net_wireless(int update_every, usec_t dt);
int do_proc_diskstats(int update_every, usec_t dt);
int do_proc_mdstat(int update_every, usec_t dt);
int do_proc_net_netstat(int update_every, usec_t dt);
int do_proc_net_stat_conntrack(int update_every, usec_t dt);
int do_proc_net_ip_vs_stats(int update_every, usec_t dt);
int do_proc_stat(int update_every, usec_t dt);
int do_proc_meminfo(int update_every, usec_t dt);
int do_proc_vmstat(int update_every, usec_t dt);
int do_proc_net_rpc_nfs(int update_every, usec_t dt);
int do_proc_net_rpc_nfsd(int update_every, usec_t dt);
int do_proc_sys_fs_file_nr(int update_every, usec_t dt);
int do_proc_sys_kernel_random_entropy_avail(int update_every, usec_t dt);
int do_proc_interrupts(int update_every, usec_t dt);
int do_proc_softirqs(int update_every, usec_t dt);
int do_proc_pressure(int update_every, usec_t dt);
int do_sys_kernel_mm_ksm(int update_every, usec_t dt);
int do_sys_block_zram(int update_every, usec_t dt);
int do_proc_loadavg(int update_every, usec_t dt);
int do_proc_net_stat_synproxy(int update_every, usec_t dt);
int do_proc_net_softnet_stat(int update_every, usec_t dt);
int do_proc_uptime(int update_every, usec_t dt);
int do_proc_sys_devices_system_edac_mc(int update_every, usec_t dt);
int do_proc_sys_devices_pci_aer(int update_every, usec_t dt);
int do_proc_sys_devices_system_node(int update_every, usec_t dt);
int do_proc_spl_kstat_zfs_arcstats(int update_every, usec_t dt);
int do_sys_fs_btrfs(int update_every, usec_t dt);
int do_proc_net_sockstat(int update_every, usec_t dt);
int do_proc_net_sockstat6(int update_every, usec_t dt);
int do_proc_net_sctp_snmp(int update_every, usec_t dt);
int do_proc_ipc(int update_every, usec_t dt);
int do_sys_class_power_supply(int update_every, usec_t dt);
int do_proc_pagetypeinfo(int update_every, usec_t dt);
int do_sys_class_infiniband(int update_every, usec_t dt);
int do_sys_class_drm(int update_every, usec_t dt);
int get_numa_node_count(void);
int do_run_reboot_required(int update_every, usec_t dt);

// Plugin cleanup functions
void proc_ipc_cleanup(void);
void proc_net_netstat_cleanup(void);
void proc_net_stat_conntrack_cleanup(void);
void proc_stat_plugin_cleanup(void);
void proc_net_sockstat_plugin_cleanup(void);
void proc_loadavg_plugin_cleanup(void);
void sys_class_infiniband_plugin_cleanup(void);
void pci_aer_plugin_cleanup(void);

// metrics that need to be shared among data collectors
extern unsigned long long zfs_arcstats_shrinkable_cache_size_bytes;
extern bool inside_lxc_container;

extern bool is_mem_swap_enabled;
extern bool is_mem_zswap_enabled;
extern bool is_mem_ksm_enabled;

// netdev renames
void cgroup_rename_task_add(
    const char *host_device,
    const char *container_device,
    const char *container_name,
    RRDLABELS *labels,
    const char *ctx_prefix,
    const DICTIONARY_ITEM *cgroup_netdev_link);

void cgroup_rename_task_device_del(const char *host_device);

#include "proc_self_mountinfo.h"
#include "proc_pressure.h"
#include "zfs_common.h"

#endif /* NETDATA_PLUGIN_PROC_H */
