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

void pulse_daemon_do(bool extended) {
    pulse_daemon_cpu_usage_do(extended);
    pulse_daemon_uptime_do(extended);
    pulse_daemon_memory_do(extended);
}
