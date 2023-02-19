// SPDX-License-Identifier: GPL-3.0-or-later

#include "daemon/common.h"
#include "libnetdata/os.h"

#define PLUGIN_TIMEX_NAME "timex.plugin"

#define CONFIG_SECTION_TIMEX "plugin:timex"

struct status_codes {
    char *name;
    int code;
    RRDDIM *rd;
} sta_codes[] = {
    // {"pll", STA_PLL, NULL},
    // {"ppsfreq", STA_PPSFREQ, NULL},
    // {"ppstime", STA_PPSTIME, NULL},
    // {"fll", STA_FLL, NULL},
    // {"ins", STA_INS, NULL},
    // {"del", STA_DEL, NULL},
    {"unsync", STA_UNSYNC, NULL},
    // {"freqhold", STA_FREQHOLD, NULL},
    // {"ppssignal", STA_PPSSIGNAL, NULL},
    // {"ppsjitter", STA_PPSJITTER, NULL},
    // {"ppswander", STA_PPSWANDER, NULL},
    // {"ppserror", STA_PPSERROR, NULL},
    {"clockerr", STA_CLOCKERR, NULL},
    // {"nano", STA_NANO, NULL},
    // {"clk", STA_CLK, NULL},
    {NULL, 0, NULL},
};

static void timex_main_cleanup(void *ptr)
{
    worker_unregister();

    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)ptr;
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;

    info("cleaning up...");

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
}

void *timex_main(void *ptr)
{
    worker_register("TIMEX");
    worker_register_job_name(0, "clock check");

    netdata_thread_cleanup_push(timex_main_cleanup, ptr);

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
    while (service_running(SERVICE_COLLECTORS)) {
        worker_is_idle();
        heartbeat_next(&hb, step);
        worker_is_busy(0);

        struct timex timex_buf = {};
        int sync_state = 0;
        static int prev_sync_state = 0;

        sync_state = ADJUST_TIMEX(&timex_buf);
        
        int non_seq_failure = (sync_state == -1 && prev_sync_state != -1);
        prev_sync_state = sync_state;

        if (non_seq_failure) {
            error("Cannot get clock synchronization state");
            continue;
        }

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
            }

            rrddim_set_by_pointer(st_sync_state, rd_sync_state, sync_state != TIME_ERROR ? 1 : 0);
            rrdset_done(st_sync_state);

            static RRDSET *st_clock_status = NULL;

            if (unlikely(!st_clock_status)) {
                st_clock_status = rrdset_create_localhost(
                    "system",
                    "clock_status",
                    NULL,
                    "clock synchronization",
                    NULL,
                    "System Clock Status",
                    "status",
                    PLUGIN_TIMEX_NAME,
                    NULL,
                    NETDATA_CHART_PRIO_CLOCK_STATUS,
                    update_every,
                    RRDSET_TYPE_LINE);

                for (int i = 0; sta_codes[i].name != NULL; i++) {
                    sta_codes[i].rd =
                        rrddim_add(st_clock_status, sta_codes[i].name, NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                }
            }

            for (int i = 0; sta_codes[i].name != NULL; i++)
                rrddim_set_by_pointer(st_clock_status, sta_codes[i].rd, timex_buf.status & sta_codes[i].code ? 1 : 0);

            rrdset_done(st_clock_status);
        }

        if (do_offset) {
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
            }

            rrddim_set_by_pointer(st_offset, rd_offset, timex_buf.offset);
            rrdset_done(st_offset);
        }
    }

exit:
    netdata_thread_cleanup_pop(1);
    return NULL;
}
