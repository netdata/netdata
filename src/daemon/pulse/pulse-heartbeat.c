// SPDX-License-Identifier: GPL-3.0-or-later

#define PULSE_INTERNALS 1
#include "pulse-heartbeat.h"

void pulse_heartbeat_do(bool extended) {
    if(!extended) return;

    static RRDSET *st_heartbeat = NULL;
    static RRDDIM *rd_heartbeat_min = NULL;
    static RRDDIM *rd_heartbeat_max = NULL;
    static RRDDIM *rd_heartbeat_avg = NULL;

    if (unlikely(!st_heartbeat)) {
        st_heartbeat = rrdset_create_localhost(
            "netdata"
            , "heartbeat"
            , NULL
            , "heartbeat"
            , NULL
            , "System clock jitter"
            , "microseconds"
            , "netdata"
            , "pulse"
            , 900000
            , localhost->rrd_update_every
            , RRDSET_TYPE_AREA);

        rd_heartbeat_min = rrddim_add(st_heartbeat, "min", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        rd_heartbeat_max = rrddim_add(st_heartbeat, "max", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        rd_heartbeat_avg = rrddim_add(st_heartbeat, "average", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    usec_t min, max, average;
    size_t count;

    heartbeat_statistics(&min, &max, &average, &count);

    rrddim_set_by_pointer(st_heartbeat, rd_heartbeat_min, (collected_number)min);
    rrddim_set_by_pointer(st_heartbeat, rd_heartbeat_max, (collected_number)max);
    rrddim_set_by_pointer(st_heartbeat, rd_heartbeat_avg, (collected_number)average);

    rrdset_done(st_heartbeat);
}
