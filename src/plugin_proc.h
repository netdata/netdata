#ifndef NETDATA_PLUGIN_PROC_H
#define NETDATA_PLUGIN_PROC_H 1

/**
 * @file plugin_proc.h
 * @brief The header file of all internal proc data collectors.
 *
 * This is the header file of the proc data collection thread.
 *
 * __How to add a data collector__
 * \todo How to add a proc data collector.
 */

#include "common.h"

/** A proc file collector handled by proc_main() */
struct proc_module {
    const char *name; ///< What's collected
    const char *dim;  ///< chart dimension (proc.dim)

    int enabled;      ///< boolean

    /**
     * Data collector.
     *
     * Function doing the data collection.
     * This is called by the proc plugin thread every `update_every` second.
     * This function should update `rd`
     * 
     * @param update_every intervall in seconds this is called.
     * @param dt microseconds passed since the last call.
     * @return 0 on success. 1 on error.
     */
    int (*func)(int update_every, usec_t dt);
    usec_t duration;      ///< Duration of the last call of `func()`.

    struct rrddim *rd; ///< Round robin database dimension to update by `func()`.
};

/**
 * Function run by the proc plugin thread.
 *
 * This must only be startet once.
 *
 * @param ptr to struct netdata_static_thread
 * @return NULL
 */
void *proc_main(void *ptr);

/**
 * Data collector of `/proc/net/dev`.
 *
 * @param update_every intervall in seconds this is called.
 * @param dt microseconds passed since the last call.
 * @return 0 on success. 1 on error.
 */
extern int do_proc_net_dev(int update_every, usec_t dt);
/**
 * Data collector of `/proc/diskstats`.
 *
 * @param update_every intervall in seconds this is called.
 * @param dt microseconds passed since the last call.
 * @return 0 on success. 1 on error.
 */
extern int do_proc_diskstats(int update_every, usec_t dt);
/**
 * Data collector of `/proc/net/snmp`.
 *
 * @param update_every intervall in seconds this is called.
 * @param dt microseconds passed since the last call.
 * @return 0 on success. 1 on error.
 */
extern int do_proc_net_snmp(int update_every, usec_t dt);
/**
 * Data collector of `/proc/net/snmp6`.
 *
 * @param update_every intervall in seconds this is called.
 * @param dt microseconds passed since the last call.
 * @return 0 on success. 1 on error.
 */
extern int do_proc_net_snmp6(int update_every, usec_t dt);
/**
 * Data collector of `/proc/net/netstat`.
 *
 * @param update_every intervall in seconds this is called.
 * @param dt microseconds passed since the last call.
 * @return 0 on success. 1 on error.
 */
extern int do_proc_net_netstat(int update_every, usec_t dt);
/**
 * Data collector of `/proc/net/stat/conntrack`.
 *
 * @param update_every intervall in seconds this is called.
 * @param dt microseconds passed since the last call.
 * @return 0 on success. 1 on error.
 */
extern int do_proc_net_stat_conntrack(int update_every, usec_t dt);
/**
 * Data collector of `/proc/net/ip_vs/stats`.
 *
 * @param update_every intervall in seconds this is called.
 * @param dt microseconds passed since the last call.
 * @return 0 on success. 1 on error.
 */
extern int do_proc_net_ip_vs_stats(int update_every, usec_t dt);
/**
 * Data collector of `/proc/stat`.
 *
 * @param update_every intervall in seconds this is called.
 * @param dt microseconds passed since the last call.
 * @return 0 on success. 1 on error.
 */
extern int do_proc_stat(int update_every, usec_t dt);
/**
 * Data collector of `/proc/meminfo`.
 *
 * @param update_every intervall in seconds this is called.
 * @param dt microseconds passed since the last call.
 * @return 0 on success. 1 on error.
 */
extern int do_proc_meminfo(int update_every, usec_t dt);
/**
 * Data collector of `/proc/vmstat`.
 *
 * @param update_every intervall in seconds this is called.
 * @param dt microseconds passed since the last call.
 * @return 0 on success. 1 on error.
 */
extern int do_proc_vmstat(int update_every, usec_t dt);
/**
 * Data collector of `/proc/net/rpc/nfs`.
 *
 * @param update_every intervall in seconds this is called.
 * @param dt microseconds passed since the last call.
 * @return 0 on success. 1 on error.
 */
extern int do_proc_net_rpc_nfs(int update_every, usec_t dt);
/**
 * Data collector of `/proc/net/rpc/nfsd`.
 *
 * @param update_every intervall in seconds this is called.
 * @param dt microseconds passed since the last call.
 * @return 0 on success. 1 on error.
 */
extern int do_proc_net_rpc_nfsd(int update_every, usec_t dt);
/**
 * Data collector of `/proc/sys/kernel/random/entropy_avail`.
 *
 * @param update_every intervall in seconds this is called.
 * @param dt microseconds passed since the last call.
 * @return 0 on success. 1 on error.
 */
extern int do_proc_sys_kernel_random_entropy_avail(int update_every, usec_t dt);
/**
 * Data collector of `/proc/interrupts`.
 *
 * @param update_every intervall in seconds this is called.
 * @param dt microseconds passed since the last call.
 * @return 0 on success. 1 on error.
 */
extern int do_proc_interrupts(int update_every, usec_t dt);
/**
 * Data collector of `/proc/softirqs`.
 *
 * @param update_every intervall in seconds this is called.
 * @param dt microseconds passed since the last call.
 * @return 0 on success. 1 on error.
 */
extern int do_proc_softirqs(int update_every, usec_t dt);
/**
 * Data collector of `/sys/kernel/mm/ksm`.
 *
 * @param update_every intervall in seconds this is called.
 * @param dt microseconds passed since the last call.
 * @return 0 on success. 1 on error.
 */
extern int do_sys_kernel_mm_ksm(int update_every, usec_t dt);
/**
 * Data collector of `/proc/loadavg`.
 *
 * @param update_every intervall in seconds this is called.
 * @param dt microseconds passed since the last call.
 * @return 0 on success. 1 on error.
 */
extern int do_proc_loadavg(int update_every, usec_t dt);
/**
 * Data collector of `/proc/net/stat/synproxy`.
 *
 * @param update_every intervall in seconds this is called.
 * @param dt microseconds passed since the last call.
 * @return 0 on success. 1 on error.
 */
extern int do_proc_net_stat_synproxy(int update_every, usec_t dt);
/**
 * Data collector of `/proc/net/dev`.
 *
 * @param update_every intervall in seconds this is called.
 * @param dt microseconds passed since the last call.
 * @return 0 on success. 1 on error.
 */
extern int do_proc_net_softnet_stat(int update_every, usec_t dt);
/**
 * Data collector of `/proc/net/softnet_stat`.
 *
 * @param update_every intervall in seconds this is called.
 * @param dt microseconds passed since the last call.
 * @return 0 on success. 1 on error.
 */
extern int do_proc_uptime(int update_every, usec_t dt);
/**
 * Data collector of `/sys/devices/system/edac/mc`.
 *
 * @param update_every intervall in seconds this is called.
 * @param dt microseconds passed since the last call.
 * @return 0 on success. 1 on error.
 */
extern int do_proc_sys_devices_system_edac_mc(int update_every, usec_t dt);
/**
 * Data collector of `/sys/devices/system/node`.
 *
 * @param update_every intervall in seconds this is called.
 * @param dt microseconds passed since the last call.
 * @return 0 on success. 1 on error.
 */
extern int do_proc_sys_devices_system_node(int update_every, usec_t dt);

/**
 * Find the number of numa nodes in this system.
 *
 * @return the number of num nodes.
 */
extern int get_numa_node_count(void);

#endif /* NETDATA_PLUGIN_PROC_H */
