// SPDX-License-Identifier: GPL-3.0-or-later

#include "plugin_macos.h"

static struct macos_module {
    const char *name;
    const char *dim;

    int enabled;

    int (*func)(int update_every, usec_t dt);

    RRDDIM *rd;

} macos_modules[] = {
    {.name = "sysctl",                           .dim = "sysctl",   .enabled = 1, .func = do_macos_sysctl},
    {.name = "mach system management interface", .dim = "mach_smi", .enabled = 1, .func = do_macos_mach_smi},
    {.name = "iokit",                            .dim = "iokit",    .enabled = 1, .func = do_macos_iokit},

    // the terminator of this array
    {.name = NULL, .dim = NULL, .enabled = 0, .func = NULL}
};

#if WORKER_UTILIZATION_MAX_JOB_TYPES < 3
#error WORKER_UTILIZATION_MAX_JOB_TYPES has to be at least 3
#endif

static void macos_main_cleanup(void *ptr)
{
    worker_unregister();

    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)ptr;
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;

    info("cleaning up...");

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
}

void *macos_main(void *ptr)
{
    worker_register("MACOS");

    netdata_thread_cleanup_push(macos_main_cleanup, ptr);

    // check the enabled status for each module
    for (int i = 0; macos_modules[i].name; i++) {
        struct macos_module *pm = &macos_modules[i];

        pm->enabled = config_get_boolean("plugin:macos", pm->name, pm->enabled);
        pm->rd = NULL;

        worker_register_job_name(i, macos_modules[i].dim);
    }

    usec_t step = localhost->rrd_update_every * USEC_PER_SEC;
    heartbeat_t hb;
    heartbeat_init(&hb);

    while (!netdata_exit) {
        worker_is_idle();
        usec_t hb_dt = heartbeat_next(&hb, step);

        for (int i = 0; macos_modules[i].name; i++) {
            struct macos_module *pm = &macos_modules[i];
            if (unlikely(!pm->enabled))
                continue;

            debug(D_PROCNETDEV_LOOP, "macos calling %s.", pm->name);

            worker_is_busy(i);
            pm->enabled = !pm->func(localhost->rrd_update_every, hb_dt);

            if (unlikely(netdata_exit))
                break;
        }
    }

    netdata_thread_cleanup_pop(1);
    return NULL;
}
