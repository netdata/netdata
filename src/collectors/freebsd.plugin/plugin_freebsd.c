// SPDX-License-Identifier: GPL-3.0-or-later

#include "plugin_freebsd.h"

static struct freebsd_module {
    const char *name;
    const char *dim;

    int enabled;

    int (*func)(int update_every, usec_t dt);

    RRDDIM *rd;

} freebsd_modules[] = {

    // system metrics
    {.name = "kern.cp_time",            .dim = "cp_time",         .enabled = 1, .func = do_kern_cp_time},
    {.name = "vm.loadavg",              .dim = "loadavg",         .enabled = 1, .func = do_vm_loadavg},
    {.name = "system.ram",              .dim = "system_ram",      .enabled = 1, .func = do_system_ram},
    {.name = "vm.swap_info",            .dim = "swap",            .enabled = 1, .func = do_vm_swap_info},
    {.name = "vm.stats.vm.v_swappgs",   .dim = "swap_io",         .enabled = 1, .func = do_vm_stats_sys_v_swappgs},
    {.name = "vm.vmtotal",              .dim = "vmtotal",         .enabled = 1, .func = do_vm_vmtotal},
    {.name = "vm.stats.vm.v_forks",     .dim = "forks",           .enabled = 1, .func = do_vm_stats_sys_v_forks},
    {.name = "vm.stats.sys.v_swtch",    .dim = "context_swtch",   .enabled = 1, .func = do_vm_stats_sys_v_swtch},
    {.name = "hw.intrcnt",              .dim = "hw_intr",         .enabled = 1, .func = do_hw_intcnt},
    {.name = "vm.stats.sys.v_intr",     .dim = "dev_intr",        .enabled = 1, .func = do_vm_stats_sys_v_intr},
    {.name = "vm.stats.sys.v_soft",     .dim = "soft_intr",       .enabled = 1, .func = do_vm_stats_sys_v_soft},
    {.name = "net.isr",                 .dim = "net_isr",         .enabled = 1, .func = do_net_isr},
    {.name = "kern.ipc.sem",            .dim = "semaphores",      .enabled = 1, .func = do_kern_ipc_sem},
    {.name = "kern.ipc.shm",            .dim = "shared_memory",   .enabled = 1, .func = do_kern_ipc_shm},
    {.name = "kern.ipc.msq",            .dim = "message_queues",  .enabled = 1, .func = do_kern_ipc_msq},
    {.name = "uptime",                  .dim = "uptime",          .enabled = 1, .func = do_uptime},

    // memory metrics
    {.name = "vm.stats.vm.v_pgfaults",  .dim = "pgfaults",        .enabled = 1, .func = do_vm_stats_sys_v_pgfaults},

    // CPU metrics
    {.name = "kern.cp_times",           .dim = "cp_times",        .enabled = 1, .func = do_kern_cp_times},
    {.name = "dev.cpu.temperature",     .dim = "cpu_temperature", .enabled = 1, .func = do_dev_cpu_temperature},
    {.name = "dev.cpu.0.freq",          .dim = "cpu_frequency",   .enabled = 1, .func = do_dev_cpu_0_freq},

    // disk metrics
    {.name = "kern.devstat",            .dim = "kern_devstat",    .enabled = 1, .func = do_kern_devstat},
    {.name = "getmntinfo",              .dim = "getmntinfo",      .enabled = 1, .func = do_getmntinfo},

    // network metrics
    {.name = "net.inet.tcp.states",     .dim = "tcp_states",      .enabled = 1, .func = do_net_inet_tcp_states},
    {.name = "net.inet.tcp.stats",      .dim = "tcp_stats",       .enabled = 1, .func = do_net_inet_tcp_stats},
    {.name = "net.inet.udp.stats",      .dim = "udp_stats",       .enabled = 1, .func = do_net_inet_udp_stats},
    {.name = "net.inet.icmp.stats",     .dim = "icmp_stats",      .enabled = 1, .func = do_net_inet_icmp_stats},
    {.name = "net.inet.ip.stats",       .dim = "ip_stats",        .enabled = 1, .func = do_net_inet_ip_stats},
    {.name = "net.inet6.ip6.stats",     .dim = "ip6_stats",       .enabled = 1, .func = do_net_inet6_ip6_stats},
    {.name = "net.inet6.icmp6.stats",   .dim = "icmp6_stats",     .enabled = 1, .func = do_net_inet6_icmp6_stats},

    // network interfaces metrics
    {.name = "getifaddrs",              .dim = "getifaddrs",      .enabled = 1, .func = do_getifaddrs},

    // ZFS metrics
    {.name = "kstat.zfs.misc.arcstats", .dim = "arcstats",        .enabled = 1, .func = do_kstat_zfs_misc_arcstats},
    {.name = "kstat.zfs.misc.zio_trim", .dim = "trim",            .enabled = 1, .func = do_kstat_zfs_misc_zio_trim},

    // ipfw metrics
    {.name = "ipfw",                    .dim = "ipfw",            .enabled = 1, .func = do_ipfw},

    // the terminator of this array
    {.name = NULL, .dim = NULL, .enabled = 0, .func = NULL}
};

#if WORKER_UTILIZATION_MAX_JOB_TYPES < 33
#error WORKER_UTILIZATION_MAX_JOB_TYPES has to be at least 33
#endif

static void freebsd_main_cleanup(void *pptr)
{
    struct netdata_static_thread *static_thread = CLEANUP_FUNCTION_GET_PTR(pptr);
    if(!static_thread) return;

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;

    worker_unregister();

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
}

void *freebsd_main(void *ptr)
{
    CLEANUP_FUNCTION_REGISTER(freebsd_main_cleanup) cleanup_ptr = ptr;

    worker_register("FREEBSD");

    // initialize FreeBSD plugin
    if (freebsd_plugin_init())
        netdata_exit_fatal();

    // check the enabled status for each module
    int i;
    for (i = 0; freebsd_modules[i].name; i++) {
        struct freebsd_module *pm = &freebsd_modules[i];

        pm->enabled = inicfg_get_boolean(&netdata_config, "plugin:freebsd", pm->name, pm->enabled);
        pm->rd = NULL;

        worker_register_job_name(i, freebsd_modules[i].dim);
    }

    heartbeat_t hb;
    heartbeat_init(&hb, localhost->rrd_update_every * USEC_PER_SEC);

    while(service_running(SERVICE_COLLECTORS))  {
        worker_is_idle();

        usec_t hb_dt = heartbeat_next(&hb);

       if (!service_running(SERVICE_COLLECTORS))
            break;

        for (i = 0; freebsd_modules[i].name; i++) {
            struct freebsd_module *pm = &freebsd_modules[i];
            if (unlikely(!pm->enabled))
                continue;

            netdata_log_debug(D_PROCNETDEV_LOOP, "FREEBSD calling %s.", pm->name);

            worker_is_busy(i);
            pm->enabled = !pm->func(localhost->rrd_update_every, hb_dt);

           if (!service_running(SERVICE_COLLECTORS))
                break;
        }
    }

    return NULL;
}
