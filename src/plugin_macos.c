#include "common.h"

static void macos_main_cleanup(void *ptr) {
    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)ptr;
    if(static_thread->enabled) {
        static_thread->enabled = 0;

        info("cleaning up...");
    }
}

void *macos_main(void *ptr) {
    netdata_thread_cleanup_push(macos_main_cleanup, ptr);

    // when ZERO, attempt to do it
    int vdo_cpu_netdata             = !config_get_boolean("plugin:macos", "netdata server resources", 1);
    int vdo_macos_sysctl            = !config_get_boolean("plugin:macos", "sysctl", 1);
    int vdo_macos_mach_smi          = !config_get_boolean("plugin:macos", "mach system management interface", 1);
    int vdo_macos_iokit             = !config_get_boolean("plugin:macos", "iokit", 1);

    // keep track of the time each module was called
    unsigned long long sutime_macos_sysctl = 0ULL;
    unsigned long long sutime_macos_mach_smi = 0ULL;
    unsigned long long sutime_macos_iokit = 0ULL;

    usec_t step = localhost->rrd_update_every * USEC_PER_SEC;
    heartbeat_t hb;
    heartbeat_init(&hb);

    while(!netdata_exit) {
        usec_t hb_dt = heartbeat_next(&hb, step);

        if(unlikely(netdata_exit)) break;

        // BEGIN -- the job to be done

        if(!vdo_macos_sysctl) {
            debug(D_PROCNETDEV_LOOP, "MACOS: calling do_macos_sysctl().");
            vdo_macos_sysctl = do_macos_sysctl(localhost->rrd_update_every, hb_dt);
        }
        if(unlikely(netdata_exit)) break;

        if(!vdo_macos_mach_smi) {
            debug(D_PROCNETDEV_LOOP, "MACOS: calling do_macos_mach_smi().");
            vdo_macos_mach_smi = do_macos_mach_smi(localhost->rrd_update_every, hb_dt);
        }
        if(unlikely(netdata_exit)) break;

        if(!vdo_macos_iokit) {
            debug(D_PROCNETDEV_LOOP, "MACOS: calling do_macos_iokit().");
            vdo_macos_iokit = do_macos_iokit(localhost->rrd_update_every, hb_dt);
        }
        if(unlikely(netdata_exit)) break;

        // END -- the job is done

        // --------------------------------------------------------------------

        if(!vdo_cpu_netdata) {
            global_statistics_charts();
            registry_statistics();
        }
    }

    netdata_thread_cleanup_pop(1);
    return NULL;
}
