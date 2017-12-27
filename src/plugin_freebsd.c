#include "common.h"

static struct freebsd_module {
    const char *name;
    const char *dim;

    int enabled;

    int (*func)(int update_every, usec_t dt);
    usec_t duration;

    RRDDIM *rd;

} freebsd_modules[] = {

        // system metrics
        { .name = "kern.cp_time",          .dim = "cp_time",        .enabled = 1, .func = do_kern_cp_time },
        { .name = "vm.loadavg",            .dim = "loadavg",        .enabled = 1, .func = do_vm_loadavg },
        { .name = "system.ram",            .dim = "system_ram",     .enabled = 1, .func = do_system_ram },
        { .name = "vm.swap_info",          .dim = "swap",           .enabled = 1, .func = do_vm_swap_info },
        { .name = "vm.stats.vm.v_swappgs", .dim = "swap_io",        .enabled = 1, .func = do_vm_stats_sys_v_swappgs },
        { .name = "vm.vmtotal",            .dim = "vmtotal",        .enabled = 1, .func = do_vm_vmtotal },
        { .name = "vm.stats.vm.v_forks",   .dim = "forks",          .enabled = 1, .func = do_vm_stats_sys_v_forks },
        { .name = "vm.stats.sys.v_swtch",  .dim = "context_swtch",  .enabled = 1, .func = do_vm_stats_sys_v_swtch },
        { .name = "hw.intrcnt",            .dim = "hw_intr",        .enabled = 1, .func = do_hw_intcnt },
        { .name = "vm.stats.sys.v_intr",   .dim = "dev_intr",       .enabled = 1, .func = do_vm_stats_sys_v_intr },
        { .name = "vm.stats.sys.v_soft",   .dim = "soft_intr",      .enabled = 1, .func = do_vm_stats_sys_v_soft },
        { .name = "net.isr",               .dim = "net_isr",        .enabled = 1, .func = do_net_isr },
        { .name = "kern.ipc.sem",          .dim = "semaphores",     .enabled = 1, .func = do_kern_ipc_sem },
        { .name = "kern.ipc.shm",          .dim = "shared_memory",  .enabled = 1, .func = do_kern_ipc_shm },
        { .name = "kern.ipc.msq",          .dim = "message_queues", .enabled = 1, .func = do_kern_ipc_msq },
        { .name = "uptime",                .dim = "uptime",         .enabled = 1, .func = do_uptime },

        // memory metrics
        { .name = "vm.stats.vm.v_pgfaults", .dim = "pgfaults",      .enabled = 1, .func = do_vm_stats_sys_v_pgfaults },

        // CPU metrics
        { .name = "kern.cp_times",         .dim = "cp_times",       .enabled = 1, .func = do_kern_cp_times },
        { .name = "dev.cpu.temperature",   .dim = "cpu_temperature", .enabled = 1, .func = do_dev_cpu_temperature },
        { .name = "dev.cpu.0.freq",        .dim = "cpu_frequency",  .enabled = 1, .func = do_dev_cpu_0_freq },

        // disk metrics
        { .name = "kern.devstat",          .dim = "kern_devstat",   .enabled = 1, .func = do_kern_devstat },
        { .name = "getmntinfo",            .dim = "getmntinfo",     .enabled = 1, .func = do_getmntinfo },

        // network metrics
        { .name = "net.inet.tcp.states",   .dim = "tcp_states",     .enabled = 1, .func = do_net_inet_tcp_states },
        { .name = "net.inet.tcp.stats",    .dim = "tcp_stats",      .enabled = 1, .func = do_net_inet_tcp_stats },
        { .name = "net.inet.udp.stats",    .dim = "udp_stats",      .enabled = 1, .func = do_net_inet_udp_stats },
        { .name = "net.inet.icmp.stats",   .dim = "icmp_stats",     .enabled = 1, .func = do_net_inet_icmp_stats },
        { .name = "net.inet.ip.stats",     .dim = "ip_stats",       .enabled = 1, .func = do_net_inet_ip_stats },
        { .name = "net.inet6.ip6.stats",   .dim = "ip6_stats",      .enabled = 1, .func = do_net_inet6_ip6_stats },
        { .name = "net.inet6.icmp6.stats", .dim = "icmp6_stats",    .enabled = 1, .func = do_net_inet6_icmp6_stats },

        // network interfaces metrics
        { .name = "getifaddrs",            .dim = "getifaddrs",   .enabled = 1, .func = do_getifaddrs },

        // ZFS metrics
        { .name = "kstat.zfs.misc.arcstats", .dim = "arcstats",   .enabled = 1, .func = do_kstat_zfs_misc_arcstats },
        { .name = "kstat.zfs.misc.zio_trim", .dim = "trim",       .enabled = 1, .func = do_kstat_zfs_misc_zio_trim },

        // ipfw metrics
        { .name = "ipfw",                  .dim = "ipfw",         .enabled = 1, .func = do_ipfw },

        // the terminator of this array
        { .name = NULL, .dim = NULL, .enabled = 0, .func = NULL }
};

static void freebsd_main_cleanup(void *ptr) {
    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)ptr;
    if(static_thread->enabled) {
        static_thread->enabled = 0;

        info("%s: cleaning up...", netdata_thread_tag());
    }
}

void *freebsd_main(void *ptr) {
    netdata_thread_welcome("FREEBSD");
    pthread_cleanup_push(freebsd_main_cleanup, ptr);

    int vdo_cpu_netdata = config_get_boolean("plugin:freebsd", "netdata server resources", 1);

    // initialize FreeBSD plugin
    if (freebsd_plugin_init())
        netdata_exit = 1;

    // check the enabled status for each module
    int i;
    for(i = 0 ; freebsd_modules[i].name ;i++) {
        struct freebsd_module *pm = &freebsd_modules[i];

        pm->enabled = config_get_boolean("plugin:freebsd", pm->name, pm->enabled);
        pm->duration = 0ULL;
        pm->rd = NULL;
    }

    usec_t step = localhost->rrd_update_every * USEC_PER_SEC;
    heartbeat_t hb;
    heartbeat_init(&hb);

    while(!netdata_exit) {
        usec_t hb_dt = heartbeat_next(&hb, step);
        usec_t duration = 0ULL;

        if(unlikely(netdata_exit)) break;

        // BEGIN -- the job to be done

        for(i = 0 ; freebsd_modules[i].name ;i++) {
            struct freebsd_module *pm = &freebsd_modules[i];
            if(unlikely(!pm->enabled)) continue;

            debug(D_PROCNETDEV_LOOP, "FREEBSD calling %s.", pm->name);

            pm->enabled = !pm->func(localhost->rrd_update_every, hb_dt);
            pm->duration = heartbeat_dt_usec(&hb) - duration;
            duration += pm->duration;

            if(unlikely(netdata_exit)) break;
        }

        // END -- the job is done

        // --------------------------------------------------------------------

        if(vdo_cpu_netdata) {
            static RRDSET *st = NULL;

            if(unlikely(!st)) {
                st = rrdset_find_bytype_localhost("netdata", "plugin_freebsd_modules");

                if(!st) {
                    st = rrdset_create_localhost(
                            "netdata"
                            , "plugin_freebsd_modules"
                            , NULL
                            , "freebsd"
                            , NULL
                            , "NetData FreeBSD Plugin Modules Durations"
                            , "milliseconds/run"
                            , "netdata"
                            , "stats"
                            , 132001
                            , localhost->rrd_update_every
                            , RRDSET_TYPE_STACKED
                    );

                    for(i = 0 ; freebsd_modules[i].name ;i++) {
                        struct freebsd_module *pm = &freebsd_modules[i];
                        if(unlikely(!pm->enabled)) continue;

                        pm->rd = rrddim_add(st, pm->dim, NULL, 1, 1000, RRD_ALGORITHM_ABSOLUTE);
                    }
                }
            }
            else rrdset_next(st);

            for(i = 0 ; freebsd_modules[i].name ;i++) {
                struct freebsd_module *pm = &freebsd_modules[i];
                if(unlikely(!pm->enabled)) continue;

                rrddim_set_by_pointer(st, pm->rd, pm->duration);
            }
            rrdset_done(st);

            global_statistics_charts();
            registry_statistics();
        }
    }

    pthread_cleanup_pop(1);
    pthread_exit(NULL);
    return NULL;
}
