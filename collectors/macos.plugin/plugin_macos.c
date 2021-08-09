// SPDX-License-Identifier: GPL-3.0-or-later

#include "plugin_macos.h"

static struct macos_module {
    const char *name;
    const char *dim;

    int enabled;

    int (*func)(int update_every, usec_t dt);
    usec_t duration;

    RRDDIM *rd;

} macos_modules[] = {
    {.name = "sysctl",                           .dim = "sysctl",   .enabled = 1, .func = do_macos_sysctl},
    {.name = "mach system management interface", .dim = "mach_smi", .enabled = 1, .func = do_macos_mach_smi},
    {.name = "iokit",                            .dim = "iokit",    .enabled = 1, .func = do_macos_iokit},

    // the terminator of this array
    {.name = NULL, .dim = NULL, .enabled = 0, .func = NULL}
};

static void macos_main_cleanup(void *ptr)
{
    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)ptr;
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;

    info("cleaning up...");

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
}

void *macos_main(void *ptr)
{
    netdata_thread_cleanup_push(macos_main_cleanup, ptr);

    int vdo_cpu_netdata = config_get_boolean("plugin:macos", "netdata server resources", CONFIG_BOOLEAN_YES);

    // check the enabled status for each module
    for (int i = 0; macos_modules[i].name; i++) {
        struct macos_module *pm = &macos_modules[i];

        pm->enabled = config_get_boolean("plugin:macos", pm->name, pm->enabled);
        pm->duration = 0ULL;
        pm->rd = NULL;
    }

    usec_t step = localhost->rrd_update_every * USEC_PER_SEC;
    heartbeat_t hb;
    heartbeat_init(&hb);

    while (!netdata_exit) {
        usec_t hb_dt = heartbeat_next(&hb, step);
        usec_t duration = 0ULL;

        // BEGIN -- the job to be done

        for (int i = 0; macos_modules[i].name; i++) {
            struct macos_module *pm = &macos_modules[i];
            if (unlikely(!pm->enabled))
                continue;

            debug(D_PROCNETDEV_LOOP, "macos calling %s.", pm->name);

            pm->enabled = !pm->func(localhost->rrd_update_every, hb_dt);
            pm->duration = heartbeat_monotonic_dt_to_now_usec(&hb) - duration;
            duration += pm->duration;

            if (unlikely(netdata_exit))
                break;
        }

        // END -- the job is done

        if (vdo_cpu_netdata) {
            static RRDSET *st_cpu_thread = NULL, *st_duration = NULL;
            static RRDDIM *rd_user = NULL, *rd_system = NULL;

            // ----------------------------------------------------------------

            struct rusage thread;
            getrusage(RUSAGE_THREAD, &thread);

            if (unlikely(!st_cpu_thread)) {
                st_cpu_thread = rrdset_create_localhost(
                    "netdata",
                    "plugin_macos_cpu",
                    NULL,
                    "macos",
                    NULL,
                    "Netdata macOS plugin CPU usage",
                    "milliseconds/s",
                    "macos.plugin",
                    "stats",
                    132000,
                    localhost->rrd_update_every,
                    RRDSET_TYPE_STACKED);

                rd_user = rrddim_add(st_cpu_thread, "user", NULL, 1, USEC_PER_MS, RRD_ALGORITHM_INCREMENTAL);
                rd_system = rrddim_add(st_cpu_thread, "system", NULL, 1, USEC_PER_MS, RRD_ALGORITHM_INCREMENTAL);
            } else {
                rrdset_next(st_cpu_thread);
            }

            rrddim_set_by_pointer(
                st_cpu_thread, rd_user, thread.ru_utime.tv_sec * USEC_PER_SEC + thread.ru_utime.tv_usec);
            rrddim_set_by_pointer(
                st_cpu_thread, rd_system, thread.ru_stime.tv_sec * USEC_PER_SEC + thread.ru_stime.tv_usec);
            rrdset_done(st_cpu_thread);

            // ----------------------------------------------------------------

            if (unlikely(!st_duration)) {
                st_duration = rrdset_find_active_bytype_localhost("netdata", "plugin_macos_modules");

                if (!st_duration) {
                    st_duration = rrdset_create_localhost(
                        "netdata",
                        "plugin_macos_modules",
                        NULL,
                        "macos",
                        NULL,
                        "Netdata macOS plugin modules durations",
                        "milliseconds/run",
                        "macos.plugin",
                        "stats",
                        132001,
                        localhost->rrd_update_every,
                        RRDSET_TYPE_STACKED);

                    for (int i = 0; macos_modules[i].name; i++) {
                        struct macos_module *pm = &macos_modules[i];
                        if (unlikely(!pm->enabled))
                            continue;

                        pm->rd = rrddim_add(st_duration, pm->dim, NULL, 1, 1000, RRD_ALGORITHM_ABSOLUTE);
                    }
                }
            } else
                rrdset_next(st_duration);

            for (int i = 0; macos_modules[i].name; i++) {
                struct macos_module *pm = &macos_modules[i];
                if (unlikely(!pm->enabled))
                    continue;

                rrddim_set_by_pointer(st_duration, pm->rd, pm->duration);
            }
            rrdset_done(st_duration);
        }
    }

    netdata_thread_cleanup_pop(1);
    return NULL;
}
