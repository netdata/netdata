// SPDX-License-Identifier: GPL-3.0-or-later

#include "plugin_proc.h"

static struct proc_module {
    const char *name;
    const char *dim;

    int enabled;

    int (*func)(int update_every, usec_t dt);
    void (*cleanup)(void);  // Cleanup function pointer

    RRDDIM *rd;

} proc_modules[] = {

    // system metrics
    {.name = "/proc/stat",                   .dim = "stat",         .func = do_proc_stat, .cleanup = proc_stat_plugin_cleanup},
    {.name = "/proc/uptime",                 .dim = "uptime",       .func = do_proc_uptime},
    {.name = "/proc/loadavg",                .dim = "loadavg",      .func = do_proc_loadavg, .cleanup = proc_loadavg_plugin_cleanup},
    {.name = "/proc/sys/fs/file-nr",         .dim = "file-nr",      .func = do_proc_sys_fs_file_nr},
    {.name = "/proc/sys/kernel/random/entropy_avail", .dim = "entropy", .func = do_proc_sys_kernel_random_entropy_avail},

    {.name = "/run/reboot_required",         .dim = "reboot-required", .func = do_run_reboot_required},

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
    {.name = "/sys/devices/system/edac/mc",  .dim = "edac",         .func = do_proc_sys_devices_system_edac_mc},
    {.name = "/sys/devices/pci/aer",         .dim = "pci_aer",      .func = do_proc_sys_devices_pci_aer, .cleanup = pci_aer_plugin_cleanup},
    {.name = "/sys/devices/system/node",     .dim = "numa",         .func = do_proc_sys_devices_system_node},
    {.name = "/proc/pagetypeinfo",           .dim = "pagetypeinfo", .func = do_proc_pagetypeinfo},

    // network metrics
    {.name = "/proc/net/wireless",           .dim = "netwireless",  .func = do_proc_net_wireless},
    {.name = "/proc/net/sockstat",           .dim = "sockstat",     .func = do_proc_net_sockstat, .cleanup = proc_net_sockstat_plugin_cleanup},
    {.name = "/proc/net/sockstat6",          .dim = "sockstat6",    .func = do_proc_net_sockstat6},
    {.name = "/proc/net/netstat",            .dim = "netstat",      .func = do_proc_net_netstat, .cleanup = proc_net_netstat_cleanup},
    {.name = "/proc/net/sctp/snmp",          .dim = "sctp",         .func = do_proc_net_sctp_snmp},
    {.name = "/proc/net/softnet_stat",       .dim = "softnet",      .func = do_proc_net_softnet_stat},
    {.name = "/proc/net/ip_vs/stats",        .dim = "ipvs",         .func = do_proc_net_ip_vs_stats},
    {.name = "/sys/class/infiniband",        .dim = "infiniband",   .func = do_sys_class_infiniband, .cleanup = sys_class_infiniband_plugin_cleanup},

    // firewall metrics
    {.name = "/proc/net/stat/conntrack",     .dim = "conntrack",    .func = do_proc_net_stat_conntrack, .cleanup = proc_net_stat_conntrack_cleanup},
    {.name = "/proc/net/stat/synproxy",      .dim = "synproxy",     .func = do_proc_net_stat_synproxy},

    // disk metrics
    {.name = "/proc/diskstats",              .dim = "diskstats",    .func = do_proc_diskstats},
    {.name = "/proc/mdstat",                 .dim = "mdstat",       .func = do_proc_mdstat},

    // NFS metrics
    {.name = "/proc/net/rpc/nfsd",           .dim = "nfsd",         .func = do_proc_net_rpc_nfsd},
    {.name = "/proc/net/rpc/nfs",            .dim = "nfs",          .func = do_proc_net_rpc_nfs},

    // ZFS metrics
    {.name = "/proc/spl/kstat/zfs/arcstats", .dim = "zfs_arcstats", .func = do_proc_spl_kstat_zfs_arcstats},

    // BTRFS metrics
    {.name = "/sys/fs/btrfs",                .dim = "btrfs",        .func = do_sys_fs_btrfs},

    // IPC metrics
    {.name = "ipc",                          .dim = "ipc",          .func = do_proc_ipc, .cleanup = proc_ipc_cleanup},

    // linux power supply metrics
    {.name = "/sys/class/power_supply",      .dim = "power_supply", .func = do_sys_class_power_supply},
    
    // GPU metrics
    {.name = "/sys/class/drm",               .dim = "drm",          .func = do_sys_class_drm},

    // the terminator of this array
    {.name = NULL, .dim = NULL, .func = NULL}
};

#if WORKER_UTILIZATION_MAX_JOB_TYPES < 36
#error WORKER_UTILIZATION_MAX_JOB_TYPES has to be at least 36
#endif

static ND_THREAD *netdev_thread = NULL;

static void proc_main_cleanup(void *pptr)
{
    struct netdata_static_thread *static_thread = CLEANUP_FUNCTION_GET_PTR(pptr);
    if(!static_thread) return;

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;

    // Run all module cleanup functions
    int i;
    for(i = 0; proc_modules[i].name; i++) {
        if(proc_modules[i].cleanup)
            proc_modules[i].cleanup();
    }

    nd_thread_join(netdev_thread);
    worker_unregister();

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
}

bool inside_lxc_container = false;
bool is_mem_swap_enabled = false;
bool is_mem_zswap_enabled = false;
bool is_mem_ksm_enabled = false;

static bool is_lxcfs_proc_mounted() {
    procfile *ff = NULL;

    if (unlikely(!ff)) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "/proc/self/mounts");
        ff = procfile_open(filename, " \t", PROCFILE_FLAG_DEFAULT);
        if (unlikely(!ff))
            return false;
    }

    ff = procfile_readall(ff);
    if (unlikely(!ff))
        return false;

    unsigned long l, lines = procfile_lines(ff);

    for (l = 0; l < lines; l++) {
        size_t words = procfile_linewords(ff, l);
        if (words < 2) {
            continue;
        }
        if (!strcmp(procfile_lineword(ff, l, 0), "lxcfs") && !strncmp(procfile_lineword(ff, l, 1), "/proc", 5)) {
            procfile_close(ff);
            return true;   
        }            
    }

    procfile_close(ff);

    return false;
}

static bool is_ksm_enabled() {
    unsigned long long ksm_run = 0;

    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s/sys/kernel/mm/ksm/run", netdata_configured_host_prefix);

    return !read_single_number_file(filename, &ksm_run) && ksm_run == 1;
}

static bool is_zswap_enabled() {
    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "/sys/module/zswap/parameters/enabled"); // host prefix is not needed here
    char state[1 + 1];                                                         // Y or N

    int ret = read_txt_file(filename, state, sizeof(state));

    return !ret && !strcmp(state, "Y");
}

static bool is_swap_enabled() {
    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s/proc/meminfo", netdata_configured_host_prefix);

    procfile *ff = procfile_open(filename, " \t:", PROCFILE_FLAG_DEFAULT);
    if (!ff) {
        return false;
    }

    ff = procfile_readall(ff);
    if (!ff) {
        procfile_close(ff);
        return false;
    }

    unsigned long long swap_total = 0;

    size_t lines = procfile_lines(ff), l;

    for (l = 0; l < lines; l++) {
        size_t words = procfile_linewords(ff, l);
        if (words < 2)
            continue;

        const char *key = procfile_lineword(ff, l, 0);
        if (strcmp(key, "SwapTotal") == 0) {
            swap_total = str2ull(procfile_lineword(ff, l, 1), NULL);
            break;
        }
    }

    procfile_close(ff);

    return swap_total > 0;
}

static bool log_proc_module(BUFFER *wb, void *data) {
    struct proc_module *pm = data;
    buffer_sprintf(wb, "proc.plugin[%s]", pm->name);
    return true;
}

void proc_main(void *ptr)
{
    CLEANUP_FUNCTION_REGISTER(proc_main_cleanup) cleanup_ptr = ptr;

    worker_register("PROC");

    rrd_collector_started();

    if (inicfg_get_boolean(&netdata_config, "plugin:proc", "/proc/net/dev", CONFIG_BOOLEAN_YES)) {
        netdata_log_debug(D_SYSTEM, "Starting thread %s.", THREAD_NETDEV_NAME);
        netdev_thread = nd_thread_create(THREAD_NETDEV_NAME, NETDATA_THREAD_OPTION_DEFAULT, netdev_main, NULL);
    }

    inicfg_get_boolean(&netdata_config, "plugin:proc", "/proc/pagetypeinfo", CONFIG_BOOLEAN_NO);

    // check the enabled status for each module
    int i;
    for(i = 0; proc_modules[i].name; i++) {
        struct proc_module *pm = &proc_modules[i];

        pm->enabled = inicfg_get_boolean(&netdata_config, "plugin:proc", pm->name, CONFIG_BOOLEAN_YES);
        pm->rd = NULL;

        worker_register_job_name(i, proc_modules[i].dim);
    }

    heartbeat_t hb;
    heartbeat_init(&hb, localhost->rrd_update_every * USEC_PER_SEC);

    inside_lxc_container = is_lxcfs_proc_mounted();
    is_mem_swap_enabled = is_swap_enabled();
    is_mem_zswap_enabled = is_zswap_enabled();
    is_mem_ksm_enabled = is_ksm_enabled();

#define LGS_MODULE_ID 0

    ND_LOG_STACK lgs[] = {
            [LGS_MODULE_ID] = ND_LOG_FIELD_TXT(NDF_MODULE, "proc.plugin"),
            ND_LOG_FIELD_END(),
    };
    ND_LOG_STACK_PUSH(lgs);

    while(service_running(SERVICE_COLLECTORS)) {
        worker_is_idle();
        usec_t hb_dt = heartbeat_next(&hb);

        if(unlikely(!service_running(SERVICE_COLLECTORS)))
            break;

        for(i = 0; proc_modules[i].name; i++) {
            if(unlikely(!service_running(SERVICE_COLLECTORS)))
                break;

            struct proc_module *pm = &proc_modules[i];
            if(unlikely(!pm->enabled))
                continue;

            worker_is_busy(i);
            lgs[LGS_MODULE_ID] = ND_LOG_FIELD_CB(NDF_MODULE, log_proc_module, pm);
            pm->enabled = !pm->func(localhost->rrd_update_every, hb_dt);
            lgs[LGS_MODULE_ID] = ND_LOG_FIELD_TXT(NDF_MODULE, "proc.plugin");
        }
    }
}

int get_numa_node_count(void)
{
    static int numa_node_count = -1;

    if (numa_node_count != -1)
        return numa_node_count;

    numa_node_count = 0;

    char name[FILENAME_MAX + 1];
    snprintfz(name, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/sys/devices/system/node");
    const char *dirname = inicfg_get(&netdata_config, "plugin:proc:/sys/devices/system/node", "directory to monitor", name);

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
