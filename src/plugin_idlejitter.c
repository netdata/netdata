#include "common.h"

#define CPU_IDLEJITTER_SLEEP_TIME_MS 20

void *cpuidlejitter_main(void *ptr)
{
    if(ptr) { ; }

    info("CPU Idle Jitter thread created with task id %d", gettid());

    if(pthread_setcanceltype(PTHREAD_CANCEL_DEFERRED, NULL) != 0)
        error("Cannot set pthread cancel type to DEFERRED.");

    if(pthread_setcancelstate(PTHREAD_CANCEL_ENABLE, NULL) != 0)
        error("Cannot set pthread cancel state to ENABLE.");

    int sleep_ms = (int) config_get_number("plugin:idlejitter", "loop time in ms", CPU_IDLEJITTER_SLEEP_TIME_MS);
    if(sleep_ms <= 0) {
        config_set_number("plugin:idlejitter", "loop time in ms", CPU_IDLEJITTER_SLEEP_TIME_MS);
        sleep_ms = CPU_IDLEJITTER_SLEEP_TIME_MS;
    }

    RRDSET *st = rrdset_find("system.idlejitter");
    if(!st) {
        st = rrdset_create("system", "idlejitter", NULL, "processes", NULL, "CPU Idle Jitter", "microseconds lost/s", 9999, rrd_update_every, RRDSET_TYPE_LINE);
        rrddim_add(st, "jitter", NULL, 1, 1, RRDDIM_ABSOLUTE);
    }

    struct timeval before, after;
    unsigned long long counter;
    for(counter = 0; 1 ;counter++) {
        usec_t usec = 0, susec = 0;

        while(susec < (rrd_update_every * USEC_PER_SEC)) {

            now_realtime_timeval(&before);
            sleep_usec(sleep_ms * 1000);
            now_realtime_timeval(&after);

            // calculate the time it took for a full loop
            usec = dt_usec(&after, &before);
            susec += usec;
        }
        usec -= (sleep_ms * 1000);

        if(counter) rrdset_next(st);
        rrddim_set(st, "jitter", usec);
        rrdset_done(st);
    }

    pthread_exit(NULL);
    return NULL;
}

