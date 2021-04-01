// SPDX-License-Identifier: GPL-3.0-or-later

#include "plugin_timex.h"
#include "sys/timex.h"

#define PLUGIN_TIMEX_NAME "timex.plugin"

#define CONFIG_SECTION_TIMEX "plugin:timex"

static void timex_main_cleanup(void *ptr)
{
    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)ptr;
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;

    info("cleaning up...");

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
}

void *timex_main(void *ptr)
{
    netdata_thread_cleanup_push(timex_main_cleanup, ptr);

    int vdo_cpu_netdata = config_get_boolean(CONFIG_SECTION_TIMEX, "timex plugin resource charts", CONFIG_BOOLEAN_YES);

    int update_every = (int)config_get_number(CONFIG_SECTION_TIMEX, "update every", localhost->rrd_update_every);
    if (update_every < localhost->rrd_update_every)
        update_every = localhost->rrd_update_every;

    int do_sync = config_get_boolean(CONFIG_SECTION_TIMEX, "clock synchronization state", CONFIG_BOOLEAN_YES);
    int do_offset = config_get_boolean(CONFIG_SECTION_TIMEX, "time offset", CONFIG_BOOLEAN_YES);

    if (unlikely(do_sync == CONFIG_BOOLEAN_NO && do_offset == CONFIG_BOOLEAN_NO))
        return NULL;

    usec_t step = update_every * USEC_PER_SEC;
    heartbeat_t hb;
    heartbeat_init(&hb);
    while (!netdata_exit) {
        usec_t duration = heartbeat_monotonic_dt_to_now_usec(&hb);
        heartbeat_next(&hb, step);

        if (unlikely(netdata_exit))
            break;

        struct timex timex_buf = {};
        int sync_state = 0;
        sync_state = adjtimex(&timex_buf);

        if(unlikely(netdata_exit)) break;

        if(vdo_cpu_netdata) {
            static RRDSET *stcpu_thread = NULL, *st_duration = NULL;
            static RRDDIM *rd_user = NULL, *rd_system = NULL, *rd_duration = NULL;

            // ----------------------------------------------------------------

            struct rusage thread;
            getrusage(RUSAGE_THREAD, &thread);

            if(unlikely(!stcpu_thread)) {
                stcpu_thread = rrdset_create_localhost(
                        "netdata"
                        , "plugin_timex"
                        , NULL
                        , "timex"
                        , NULL
                        , "NetData Timex Plugin CPU usage"
                        , "milliseconds/s"
                        , PLUGIN_TIMEX_NAME
                        , NULL
                        , NETDATA_CHART_PRIO_NETDATA_TIMEX
                        , update_every
                        , RRDSET_TYPE_STACKED
                );

                rd_user   = rrddim_add(stcpu_thread, "user", NULL, 1, 1000, RRD_ALGORITHM_INCREMENTAL);
                rd_system = rrddim_add(stcpu_thread, "system", NULL, 1, 1000, RRD_ALGORITHM_INCREMENTAL);
            }
            else
                rrdset_next(stcpu_thread);

            rrddim_set_by_pointer(stcpu_thread, rd_user, thread.ru_utime.tv_sec * 1000000ULL + thread.ru_utime.tv_usec);
            rrddim_set_by_pointer(stcpu_thread, rd_system, thread.ru_stime.tv_sec * 1000000ULL + thread.ru_stime.tv_usec);
            rrdset_done(stcpu_thread);

            // ----------------------------------------------------------------

            if(unlikely(!st_duration)) {
                st_duration = rrdset_create_localhost(
                        "netdata"
                        , "plugin_timex_dt"
                        , NULL
                        , "timex"
                        , NULL
                        , "NetData Timex Plugin Duration"
                        , "milliseconds/run"
                        , PLUGIN_TIMEX_NAME
                        , NULL
                        , 132021
                        , update_every
                        , RRDSET_TYPE_AREA
                );

                rd_duration = rrddim_add(st_duration, "duration", NULL, 1, 1000, RRD_ALGORITHM_ABSOLUTE);
            }
            else
                rrdset_next(st_duration);

            rrddim_set_by_pointer(st_duration, rd_duration, duration);
            rrdset_done(st_duration);

            // ----------------------------------------------------------------

            if(unlikely(netdata_exit)) break;
        }
    }

    netdata_thread_cleanup_pop(1);
    return NULL;
}
