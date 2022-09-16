// SPDX-License-Identifier: GPL-3.0-or-later

#include "plugin_proc.h"

static struct proc_module {
    const char *name;
    const char *dim;

    int enabled;

    int (*func)(int update_every, usec_t dt);

    RRDDIM *rd;

} proc_modules[] = {

    // system metrics
    {.name = "/proc/stat",                   .dim = "stat",         .func = do_proc_stat},
    {.name = "/proc/uptime",                 .dim = "uptime",       .func = do_proc_uptime},
    {.name = "/proc/loadavg",                .dim = "loadavg",      .func = do_proc_loadavg},
    {.name = "/proc/sys/kernel/random/entropy_avail", .dim = "entropy", .func = do_proc_sys_kernel_random_entropy_avail},

    // pressure metrics
    {.name = "/proc/pressure",               .dim = "pressure",     .func = do_proc_pressure},

    // CPU metrics
    {.name = "/proc/interrupts",             .dim = "interrupts",   .func = do_proc_interrupts},
    {.name = "/proc/softirqs",               .dim = "softirqs",     .func = do_proc_softirqs},

    // memory metrics
    {.name = "/proc/vmstat",                 .dim = "vmstat",       .func = do_proc_vmstat},
    {.name = "/proc/meminfo",                .dim = "meminfo",      .func = do_proc_meminfo},
    {.name = "/sys/kernel/mm/ksm",           .dim = "ksm",          .func = do_sys_kernel_mm_ksm},
    {.name = "/sys/block/zram",              .dim = "zram",         .func = do_sys_block_zram},
    {.name = "/sys/devices/system/edac/mc",  .dim = "ecc",          .func = do_proc_sys_devices_system_edac_mc},
    {.name = "/sys/devices/system/node",     .dim = "numa",         .func = do_proc_sys_devices_system_node},
    {.name = "/proc/pagetypeinfo",           .dim = "pagetypeinfo", .func = do_proc_pagetypeinfo},

    // network metrics
    {.name = "/proc/net/wireless",           .dim = "netwireless",  .func = do_proc_net_wireless},
    {.name = "/proc/net/sockstat",           .dim = "sockstat",     .func = do_proc_net_sockstat},
    {.name = "/proc/net/sockstat6",          .dim = "sockstat6",    .func = do_proc_net_sockstat6},
    {.name = "/proc/net/netstat",
     .dim = "netstat",
     .func = do_proc_net_netstat}, // this has to be before /proc/net/snmp, because there is a shared metric
    {.name = "/proc/net/snmp",               .dim = "snmp",         .func = do_proc_net_snmp},
    {.name = "/proc/net/snmp6",              .dim = "snmp6",        .func = do_proc_net_snmp6},
    {.name = "/proc/net/sctp/snmp",          .dim = "sctp",         .func = do_proc_net_sctp_snmp},
    {.name = "/proc/net/softnet_stat",       .dim = "softnet",      .func = do_proc_net_softnet_stat},
    {.name = "/proc/net/ip_vs/stats",        .dim = "ipvs",         .func = do_proc_net_ip_vs_stats},
    {.name = "/sys/class/infiniband",        .dim = "infiniband",   .func = do_sys_class_infiniband},

    // firewall metrics
    {.name = "/proc/net/stat/conntrack",     .dim = "conntrack",    .func = do_proc_net_stat_conntrack},
    {.name = "/proc/net/stat/synproxy",      .dim = "synproxy",     .func = do_proc_net_stat_synproxy},

    // disk metrics
    {.name = "/proc/diskstats",              .dim = "diskstats",    .func = do_proc_diskstats},
    {.name = "/proc/mdstat",                 .dim = "mdstat",       .func = do_proc_mdstat},

    // NFS metrics
    {.name = "/proc/net/rpc/nfsd",           .dim = "nfsd",         .func = do_proc_net_rpc_nfsd},
    {.name = "/proc/net/rpc/nfs",            .dim = "nfs",          .func = do_proc_net_rpc_nfs},

    // ZFS metrics
    {.name = "/proc/spl/kstat/zfs/arcstats", .dim = "zfs_arcstats", .func = do_proc_spl_kstat_zfs_arcstats},
    {.name = "/proc/spl/kstat/zfs/pool/state",.dim = "zfs_pool_state",.func = do_proc_spl_kstat_zfs_pool_state},

    // BTRFS metrics
    {.name = "/sys/fs/btrfs",                .dim = "btrfs",        .func = do_sys_fs_btrfs},

    // IPC metrics
    {.name = "ipc",                          .dim = "ipc",          .func = do_ipc},

    {.name = "/sys/class/power_supply",      .dim = "power_supply", .func = do_sys_class_power_supply},
    // linux power supply metrics

    // the terminator of this array
    {.name = NULL, .dim = NULL, .func = NULL}
};

#if WORKER_UTILIZATION_MAX_JOB_TYPES < 36
#error WORKER_UTILIZATION_MAX_JOB_TYPES has to be at least 36
#endif

static netdata_thread_t *netdev_thread = NULL;

static void proc_main_cleanup(void *ptr)
{
    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)ptr;
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;

    info("cleaning up...");

    if (netdev_thread) {
        netdata_thread_join(*netdev_thread, NULL);
        freez(netdev_thread);
    }

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;

    worker_unregister();
}

void *proc_main(void *ptr)
{
    worker_register("PROC");

    if (config_get_boolean("plugin:proc", "/proc/net/dev", CONFIG_BOOLEAN_YES)) {
        netdev_thread = mallocz(sizeof(netdata_thread_t));
        debug(D_SYSTEM, "Starting thread %s.", THREAD_NETDEV_NAME);
        netdata_thread_create(
            netdev_thread, THREAD_NETDEV_NAME, NETDATA_THREAD_OPTION_JOINABLE, netdev_main, netdev_thread);
    }

    netdata_thread_cleanup_push(proc_main_cleanup, ptr);

    config_get_boolean("plugin:proc", "/proc/pagetypeinfo", CONFIG_BOOLEAN_NO);

    // check the enabled status for each module
    int i;
    for (i = 0; proc_modules[i].name; i++) {
        struct proc_module *pm = &proc_modules[i];

        pm->enabled = config_get_boolean("plugin:proc", pm->name, CONFIG_BOOLEAN_YES);
        pm->rd = NULL;

        worker_register_job_name(i, proc_modules[i].dim);
    }

    usec_t step = localhost->rrd_update_every * USEC_PER_SEC;
    heartbeat_t hb;
    heartbeat_init(&hb);

    while (!netdata_exit) {
        worker_is_idle();
        usec_t hb_dt = heartbeat_next(&hb, step);

        if (unlikely(netdata_exit))
            break;

        for (i = 0; proc_modules[i].name; i++) {
            if (unlikely(netdata_exit))
                break;

            struct proc_module *pm = &proc_modules[i];
            if (unlikely(!pm->enabled))
                continue;

            debug(D_PROCNETDEV_LOOP, "PROC calling %s.", pm->name);

            worker_is_busy(i);
            pm->enabled = !pm->func(localhost->rrd_update_every, hb_dt);
        }
    }

    netdata_thread_cleanup_pop(1);
    return NULL;
}

int get_numa_node_count(void)
{
    static int numa_node_count = -1;

    if (numa_node_count != -1)
        return numa_node_count;

    numa_node_count = 0;

    char name[FILENAME_MAX + 1];
    snprintfz(name, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/sys/devices/system/node");
    char *dirname = config_get("plugin:proc:/sys/devices/system/node", "directory to monitor", name);

    DIR *dir = opendir(dirname);
    if (dir) {
        struct dirent *de = NULL;
        while ((de = readdir(dir))) {
            if (de->d_type != DT_DIR)
                continue;

            if (strncmp(de->d_name, "node", 4) != 0)
                continue;

            if (!isdigit(de->d_name[4]))
                continue;

            numa_node_count++;
        }
        closedir(dir);
    }

    return numa_node_count;
}
