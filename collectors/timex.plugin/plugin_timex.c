// SPDX-License-Identifier: GPL-3.0-or-later

#include "daemon/common.h"
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

    int update_every = (int)config_get_number(CONFIG_SECTION_TIMEX, "update every", 10);
    if (update_every < localhost->rrd_update_every)
        update_every = localhost->rrd_update_every;

    int do_sync = config_get_boolean(CONFIG_SECTION_TIMEX, "clock synchronization state", CONFIG_BOOLEAN_YES);
    int do_offset = config_get_boolean(CONFIG_SECTION_TIMEX, "time offset", CONFIG_BOOLEAN_YES);

    if (unlikely(do_sync == CONFIG_BOOLEAN_NO && do_offset == CONFIG_BOOLEAN_NO)) {
        info("No charts to show");
        goto exit;
    }

    usec_t step = update_every * USEC_PER_SEC;
    heartbeat_t hb;
    heartbeat_init(&hb);
    while (!netdata_exit) {
        usec_t duration = heartbeat_monotonic_dt_to_now_usec(&hb);
        heartbeat_next(&hb, step);

        struct timex timex_buf = {};
        int sync_state = 0;
        sync_state = adjtimex(&timex_buf);

        collected_number divisor = USEC_PER_MS;
        if (timex_buf.status & STA_NANO)
            divisor = NSEC_PER_MSEC;

        // ----------------------------------------------------------------

        if (do_sync) {
            static RRDSET *st_sync_state = NULL;
            static RRDDIM *rd_sync_state;

            if (unlikely(!st_sync_state)) {
                st_sync_state = rrdset_create_localhost(
                    "system",
                    "clock_sync_state",
                    NULL,
                    "clock synchronization",
                    NULL,
                    "System Clock Synchronization State",
                    "state",
                    PLUGIN_TIMEX_NAME,
                    NULL,
                    NETDATA_CHART_PRIO_CLOCK_SYNC_STATE,
                    update_every,
                    RRDSET_TYPE_LINE);

                rd_sync_state = rrddim_add(st_sync_state, "state", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            } else {
                rrdset_next(st_sync_state);
            }

            rrddim_set_by_pointer(st_sync_state, rd_sync_state, sync_state != TIME_ERROR ? 1 : 0);
            rrdset_done(st_sync_state);
        }

        if (do_sync) {
            static RRDSET *st_offset = NULL;
            static RRDDIM *rd_offset;

            if (unlikely(!st_offset)) {
                st_offset = rrdset_create_localhost(
                    "system",
                    "clock_sync_offset",
                    NULL,
                    "clock synchronization",
                    NULL,
                    "Computed Time Offset Between Local System and Reference Clock",
                    "milliseconds",
                    PLUGIN_TIMEX_NAME,
                    NULL,
                    NETDATA_CHART_PRIO_CLOCK_SYNC_OFFSET,
                    update_every,
                    RRDSET_TYPE_LINE);

                rd_offset = rrddim_add(st_offset, "offset", NULL, 1, divisor, RRD_ALGORITHM_ABSOLUTE);
            } else {
                rrdset_next(st_offset);
            }

            rrddim_set_by_pointer(st_offset, rd_offset, timex_buf.offset);
            rrdset_done(st_offset);
        }

        if (vdo_cpu_netdata) {
            static RRDSET *stcpu_thread = NULL, *st_duration = NULL;
            static RRDDIM *rd_user = NULL, *rd_system = NULL, *rd_duration = NULL;

            // ----------------------------------------------------------------

            struct rusage thread;
            getrusage(RUSAGE_THREAD, &thread);

            if (unlikely(!stcpu_thread)) {
                stcpu_thread = rrdset_create_localhost(
                    "netdata",
                    "plugin_timex",
                    NULL,
                    "timex",
                    NULL,
                    "Netdata Timex Plugin CPU usage",
                    "milliseconds/s",
                    PLUGIN_TIMEX_NAME,
                    NULL,
                    NETDATA_CHART_PRIO_NETDATA_TIMEX,
                    update_every,
                    RRDSET_TYPE_STACKED);

                rd_user   = rrddim_add(stcpu_thread, "user", NULL, 1, USEC_PER_MS, RRD_ALGORITHM_INCREMENTAL);
                rd_system = rrddim_add(stcpu_thread, "system", NULL, 1, USEC_PER_MS, RRD_ALGORITHM_INCREMENTAL);
            } else {
                rrdset_next(stcpu_thread);
            }

            rrddim_set_by_pointer(stcpu_thread, rd_user, thread.ru_utime.tv_sec * USEC_PER_SEC + thread.ru_utime.tv_usec);
            rrddim_set_by_pointer(
                stcpu_thread, rd_system, thread.ru_stime.tv_sec * USEC_PER_SEC + thread.ru_stime.tv_usec);
            rrdset_done(stcpu_thread);

            // ----------------------------------------------------------------

            if (unlikely(!st_duration)) {
                st_duration = rrdset_create_localhost(
                    "netdata",
                    "plugin_timex_dt",
                    NULL,
                    "timex",
                    NULL,
                    "Netdata Timex Plugin Duration",
                    "milliseconds/run",
                    PLUGIN_TIMEX_NAME,
                    NULL,
                    NETDATA_CHART_PRIO_NETDATA_TIMEX + 1,
                    update_every,
                    RRDSET_TYPE_AREA);

                rd_duration = rrddim_add(st_duration, "duration", NULL, 1, USEC_PER_MS, RRD_ALGORITHM_ABSOLUTE);
            } else {
                rrdset_next(st_duration);
            }

            rrddim_set_by_pointer(st_duration, rd_duration, duration);
            rrdset_done(st_duration);
        }
    }

exit:
    netdata_thread_cleanup_pop(1);
    return NULL;
}
