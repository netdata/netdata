// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDDIMMEM_H
#define NETDATA_RRDDIMMEM_H

#include "database/rrd.h"

struct mem_collect_handle {
    long slot;
    long entries;
};

struct mem_query_handle {
    time_t dt;
    time_t next_timestamp;
    time_t last_timestamp;
    time_t slot_timestamp;
    size_t slot;
    size_t last_slot;
};

extern void rrddim_collect_init(RRDDIM *rd);
extern void rrddim_collect_store_metric(RRDDIM *rd, usec_t point_in_time, NETDATA_DOUBLE number, SN_FLAGS flags);
extern int rrddim_collect_finalize(RRDDIM *rd);

extern void rrddim_query_init(RRDDIM *rd, struct rrddim_query_handle *handle, time_t start_time, time_t end_time);
extern NETDATA_DOUBLE
rrddim_query_next_metric(struct rrddim_query_handle *handle, time_t *start_time, time_t *end_time, SN_FLAGS *flags);
extern int rrddim_query_is_finished(struct rrddim_query_handle *handle);
extern void rrddim_query_finalize(struct rrddim_query_handle *handle);
extern time_t rrddim_query_latest_time(RRDDIM *rd);
extern time_t rrddim_query_oldest_time(RRDDIM *rd);

#endif
