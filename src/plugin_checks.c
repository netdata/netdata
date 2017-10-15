#include "common.h"

#ifdef NETDATA_INTERNAL_CHECKS

void *checks_main(void *ptr) {
    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)ptr;

    info("CHECKS thread created with task id %d", gettid());

    if(pthread_setcanceltype(PTHREAD_CANCEL_DEFERRED, NULL) != 0)
        error("Cannot set pthread cancel type to DEFERRED.");

    if(pthread_setcancelstate(PTHREAD_CANCEL_ENABLE, NULL) != 0)
        error("Cannot set pthread cancel state to ENABLE.");

    usec_t usec = 0, susec = localhost->rrd_update_every * USEC_PER_SEC, loop_usec = 0, total_susec = 0;
    struct timeval now, last, loop;

    RRDSET *check1, *check2, *check3, *apps_cpu = NULL;

    check1 = rrdset_create_localhost(
            "netdata"
            , "check1"
            , NULL
            , "netdata"
            , NULL
            , "Caller gives microseconds"
            , "a million !"
            , "netdata"
            , "checks"
            , 99999
            , localhost->rrd_update_every
            , RRDSET_TYPE_LINE
    );

    rrddim_add(check1, "absolute", NULL, -1, 1, RRD_ALGORITHM_ABSOLUTE);
    rrddim_add(check1, "incremental", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

    check2 = rrdset_create_localhost(
            "netdata"
            , "check2"
            , NULL
            , "netdata"
            , NULL
            , "Netdata calcs microseconds"
            , "a million !"
            , "netdata"
            , "checks"
            , 99999
            , localhost->rrd_update_every
            , RRDSET_TYPE_LINE
    );
    rrddim_add(check2, "absolute", NULL, -1, 1, RRD_ALGORITHM_ABSOLUTE);
    rrddim_add(check2, "incremental", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

    check3 = rrdset_create_localhost(
            "netdata"
            , "checkdt"
            , NULL
            , "netdata"
            , NULL
            , "Clock difference"
            , "microseconds diff"
            , "netdata"
            , "checks"
            , 99999
            , localhost->rrd_update_every
            , RRDSET_TYPE_LINE
    );
    rrddim_add(check3, "caller", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    rrddim_add(check3, "netdata", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    rrddim_add(check3, "apps.plugin", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

    now_realtime_timeval(&last);
    while(1) {
        usleep(susec);

        // find the time to sleep in order to wait exactly update_every seconds
        now_realtime_timeval(&now);
        loop_usec = dt_usec(&now, &last);
        usec = loop_usec - susec;
        debug(D_PROCNETDEV_LOOP, "CHECK: last loop took %llu usec (worked for %llu, sleeped for %llu).", loop_usec, usec, susec);

        if(usec < (localhost->rrd_update_every * USEC_PER_SEC / 2ULL)) susec = (localhost->rrd_update_every * USEC_PER_SEC) - usec;
        else susec = localhost->rrd_update_every * USEC_PER_SEC / 2ULL;

        // --------------------------------------------------------------------
        // Calculate loop time

        last.tv_sec = now.tv_sec;
        last.tv_usec = now.tv_usec;
        total_susec += loop_usec;

        // --------------------------------------------------------------------
        // check chart 1

        if(check1->counter_done) rrdset_next_usec(check1, loop_usec);
        rrddim_set(check1, "absolute", 1000000);
        rrddim_set(check1, "incremental", total_susec);
        rrdset_done(check1);

        // --------------------------------------------------------------------
        // check chart 2

        if(check2->counter_done) rrdset_next(check2);
        rrddim_set(check2, "absolute", 1000000);
        rrddim_set(check2, "incremental", total_susec);
        rrdset_done(check2);

        // --------------------------------------------------------------------
        // check chart 3

        if(!apps_cpu) apps_cpu = rrdset_find_localhost("apps.cpu");
        if(check3->counter_done) rrdset_next_usec(check3, loop_usec);
        now_realtime_timeval(&loop);
        rrddim_set(check3, "caller", (long long) dt_usec(&loop, &check1->last_collected_time));
        rrddim_set(check3, "netdata", (long long) dt_usec(&loop, &check2->last_collected_time));
        if(apps_cpu) rrddim_set(check3, "apps.plugin", (long long) dt_usec(&loop, &apps_cpu->last_collected_time));
        rrdset_done(check3);
    }

    info("CHECKS thread exiting");

    static_thread->enabled = 0;
    pthread_exit(NULL);
    return NULL;
}

#endif // NETDATA_INTERNAL_CHECKS
