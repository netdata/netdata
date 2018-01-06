#include "common.h"

#define CPU_IDLEJITTER_SLEEP_TIME_MS 20

static void cpuidlejitter_main_cleanup(void *ptr) {
    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)ptr;
    if(static_thread->enabled) {
        static_thread->enabled = 0;

        info("cleaning up...");
    }
}

void *cpuidlejitter_main(void *ptr) {
    netdata_thread_cleanup_push(cpuidlejitter_main_cleanup, ptr);

    usec_t sleep_ut = config_get_number("plugin:idlejitter", "loop time in ms", CPU_IDLEJITTER_SLEEP_TIME_MS) * USEC_PER_MS;
    if(sleep_ut <= 0) {
        config_set_number("plugin:idlejitter", "loop time in ms", CPU_IDLEJITTER_SLEEP_TIME_MS);
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
            , "idlejitter"
            , NULL
            , 800
            , localhost->rrd_update_every
            , RRDSET_TYPE_AREA
    );
    RRDDIM *rd_min = rrddim_add(st, "min", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    RRDDIM *rd_max = rrddim_add(st, "max", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    RRDDIM *rd_avg = rrddim_add(st, "average", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

    usec_t update_every_ut = localhost->rrd_update_every * USEC_PER_SEC;
    struct timeval before, after;
    unsigned long long counter;

    for(counter = 0; 1 ;counter++) {
        int iterations = 0;
        usec_t error_total = 0,
                error_min = 0,
                error_max = 0,
                elapsed = 0;

        if(netdata_exit) break;

        while(elapsed < update_every_ut) {
            now_monotonic_timeval(&before);
            sleep_usec(sleep_ut);
            now_monotonic_timeval(&after);

            usec_t dt = dt_usec(&after, &before);
            elapsed += dt;

            usec_t error = dt - sleep_ut;
            error_total += error;

            if(unlikely(!iterations))
                error_min = error;
            else if(error < error_min)
                error_min = error;

            if(error > error_max)
                error_max = error;

            iterations++;
        }

        if(netdata_exit) break;

        if(iterations) {
            if (likely(counter)) rrdset_next(st);
            rrddim_set_by_pointer(st, rd_min, error_min);
            rrddim_set_by_pointer(st, rd_max, error_max);
            rrddim_set_by_pointer(st, rd_avg, error_total / iterations);
            rrdset_done(st);
        }
    }

    netdata_thread_cleanup_pop(1);
    return NULL;
}

