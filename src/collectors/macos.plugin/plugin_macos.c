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

static void macos_main_cleanup(void *pptr)
{
    struct netdata_static_thread *static_thread = CLEANUP_FUNCTION_GET_PTR(pptr);
    if(!static_thread) return;

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;

    worker_unregister();

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
}

void macos_main(void *ptr)
{
    CLEANUP_FUNCTION_REGISTER(macos_main_cleanup) cleanup_ptr = ptr;

    worker_register("MACOS");

    // check the enabled status for each module
    for (int i = 0; macos_modules[i].name; i++) {
        struct macos_module *pm = &macos_modules[i];

        pm->enabled = inicfg_get_boolean(&netdata_config, "plugin:macos", pm->name, pm->enabled);
        pm->rd = NULL;

        worker_register_job_name(i, macos_modules[i].dim);
    }

    heartbeat_t hb;
    heartbeat_init(&hb, localhost->rrd_update_every * USEC_PER_SEC);

    while(service_running(SERVICE_COLLECTORS)) {
        worker_is_idle();
        usec_t hb_dt = heartbeat_next(&hb);

        if (!service_running(SERVICE_COLLECTORS))
            break;

        for (int i = 0; macos_modules[i].name; i++) {
            struct macos_module *pm = &macos_modules[i];
            if (unlikely(!pm->enabled))
                continue;

            netdata_log_debug(D_PROCNETDEV_LOOP, "macos calling %s.", pm->name);

            worker_is_busy(i);
            pm->enabled = !pm->func(localhost->rrd_update_every, hb_dt);

            if (!service_running(SERVICE_COLLECTORS))
                break;
        }
    }
}
