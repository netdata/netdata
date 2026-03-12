// SPDX-License-Identifier: GPL-3.0-or-later

#define PULSE_INTERNALS 1
#include "pulse-daemon.h"

static void pulse_daemon_cpu_usage_do(bool extended __maybe_unused) {
    struct rusage me;
    getrusage(RUSAGE_SELF, &me);

    {
        static RRDSET *st_cpu = NULL;
        static RRDDIM *rd_cpu_user   = NULL,
                      *rd_cpu_system = NULL;

        if (unlikely(!st_cpu)) {
            st_cpu = rrdset_create_localhost(
                "netdata"
                , "server_cpu"
                , NULL
                , "CPU usage"
                , NULL
                , "Netdata CPU usage"
                , "milliseconds/s"
                , "netdata"
                , "pulse"
                , 130000
                , localhost->rrd_update_every
                , RRDSET_TYPE_STACKED
            );

            rd_cpu_user   = rrddim_add(st_cpu, "user",   NULL, 1, 1000, RRD_ALGORITHM_INCREMENTAL);
            rd_cpu_system = rrddim_add(st_cpu, "system", NULL, 1, 1000, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st_cpu, rd_cpu_user,   (collected_number )(me.ru_utime.tv_sec * 1000000ULL + me.ru_utime.tv_usec));
        rrddim_set_by_pointer(st_cpu, rd_cpu_system, (collected_number )(me.ru_stime.tv_sec * 1000000ULL + me.ru_stime.tv_usec));
        rrdset_done(st_cpu);
    }
}

static void pulse_daemon_uptime_do(bool extended __maybe_unused) {
    {
        static time_t netdata_boottime_time = 0;
        if (!netdata_boottime_time)
            netdata_boottime_time = now_boottime_sec();

        time_t netdata_uptime = now_boottime_sec() - netdata_boottime_time;

        static RRDSET *st_uptime = NULL;
        static RRDDIM *rd_uptime = NULL;

        if (unlikely(!st_uptime)) {
            st_uptime = rrdset_create_localhost(
                "netdata",
                "uptime",
                NULL,
                "Uptime",
                NULL,
                "Netdata uptime",
                "seconds",
                "netdata",
                "pulse",
                130150,
                localhost->rrd_update_every,
                RRDSET_TYPE_LINE);

            rd_uptime = rrddim_add(st_uptime, "uptime", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        }

        rrddim_set_by_pointer(st_uptime, rd_uptime, netdata_uptime);
        rrdset_done(st_uptime);
    }
}

// Called from a single pulse daemon thread, so last_refresh_ut needs no synchronization.
static void pulse_daemon_timezone_do(bool extended __maybe_unused) {
    static usec_t last_refresh_ut = 0;

    usec_t now_ut = now_monotonic_usec();

    // refresh every 30 minutes to pick up DST changes
    if (now_ut - last_refresh_ut >= 30 * 60 * USEC_PER_SEC) {
        if (system_timezone_is_user_configured()) {
            // User explicitly configured a timezone in netdata.conf — respect it,
            // just refresh the abbreviation and offset (handles DST transitions).
            static bool mismatch_logged = false;
            if (!mismatch_logged) {
                mismatch_logged = true;
                char sys_buf[FILENAME_MAX + 1];
                const char *sys_tz = detect_system_timezone_name(sys_buf, sizeof(sys_buf));
                SYSTEM_TZ tz = system_tz_get();
                if (sys_tz && tz.timezone && strcmp(sys_tz, tz.timezone) != 0)
                    nd_log(NDLS_DAEMON, NDLP_NOTICE,
                           "TIMEZONE: configured '%s' differs from system '%s'",
                           tz.timezone, sys_tz);
                system_tz_free(&tz);
            }
            SYSTEM_TZ tz = system_tz_get();
            refresh_system_timezone(tz.timezone, true);
            system_tz_free(&tz);
        } else {
            // Auto-detected timezone — re-detect to pick up system changes.
            char buf[FILENAME_MAX + 1];
            const char *detected = detect_system_timezone_name(buf, sizeof(buf));
            if (detected) {
                refresh_system_timezone(detected, true);
            } else {
                // Detection failed — use the stored name with its original tzdb flag.
                SYSTEM_TZ tz = system_tz_get();
                refresh_system_timezone(tz.timezone, system_timezone_is_tzdb_name());
                system_tz_free(&tz);
            }
        }
        last_refresh_ut = now_ut;
    }
}

void pulse_daemon_do(bool extended) {
    pulse_daemon_cpu_usage_do(extended);
    pulse_daemon_uptime_do(extended);
    pulse_daemon_memory_do(extended);
    pulse_daemon_timezone_do(extended);
}
