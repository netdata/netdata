// SPDX-License-Identifier: GPL-3.0-or-later

#include "database/rrd.h"

#define CPU_IDLEJITTER_SLEEP_TIME_MS 20

static void cpuidlejitter_main_cleanup(void *pptr) {
    struct netdata_static_thread *static_thread = CLEANUP_FUNCTION_GET_PTR(pptr);
    if(!static_thread) return;

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;

    worker_unregister();

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
}

void cpuidlejitter_main(void *ptr) {
    CLEANUP_FUNCTION_REGISTER(cpuidlejitter_main_cleanup) cleanup_ptr = ptr;

    worker_register("IDLEJITTER");
    worker_register_job_name(0, "measurements");

    usec_t sleep_ut = inicfg_get_duration_ms(&netdata_config, "plugin:idlejitter", "loop time", CPU_IDLEJITTER_SLEEP_TIME_MS) * USEC_PER_MS;
    if(sleep_ut <= 0) {
        inicfg_set_duration_ms(&netdata_config, "plugin:idlejitter", "loop time", CPU_IDLEJITTER_SLEEP_TIME_MS);
        sleep_ut = CPU_IDLEJITTER_SLEEP_TIME_MS * USEC_PER_MS;
    }

    RRDSET *st = rrdset_create_localhost(
            "system"
            , "idlejitter"
            , NULL
            , "idlejitter"
            , NULL
            , "CPU Idle Jitter"
            , "microseconds lost/s"
            , "idlejitter.plugin"
            , NULL
            , NETDATA_CHART_PRIO_SYSTEM_IDLEJITTER
            , localhost->rrd_update_every
            , RRDSET_TYPE_AREA
    );
    RRDDIM *rd_min = rrddim_add(st, "min", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    RRDDIM *rd_max = rrddim_add(st, "max", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    RRDDIM *rd_avg = rrddim_add(st, "average", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

    usec_t update_every_ut = localhost->rrd_update_every * USEC_PER_SEC;
    struct timeval before, after;

    while (service_running(SERVICE_COLLECTORS)) {
        int iterations = 0;
        usec_t error_total = 0,
                error_min = 0,
                error_max = 0,
                elapsed = 0;

        while (elapsed < update_every_ut) {
            now_monotonic_high_precision_timeval(&before);
            worker_is_idle();
            sleep_usec(sleep_ut);
            worker_is_busy(0);
            now_monotonic_high_precision_timeval(&after);

            usec_t dt = dt_usec(&after, &before);
            elapsed += dt;

            usec_t error = dt - sleep_ut;
            error_total += error;

            if(unlikely(!iterations || error < error_min))
                error_min = error;

            if(error > error_max)
                error_max = error;

            iterations++;
        }

        if(iterations) {
            rrddim_set_by_pointer(st, rd_min, error_min);
            rrddim_set_by_pointer(st, rd_max, error_max);
            rrddim_set_by_pointer(st, rd_avg, error_total / iterations);
            rrdset_done(st);
        }
    }
}

