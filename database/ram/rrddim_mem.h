// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDDIMMEM_H
#define NETDATA_RRDDIMMEM_H

#include "database/rrd.h"

struct mem_collect_handle {
    RRDDIM *rd;
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

extern STORAGE_METRIC_HANDLE *rrddim_metric_init(RRDDIM *rd, uuid_t *dim_uuid, STORAGE_INSTANCE *db_instance);
extern void rrddim_metric_free(STORAGE_METRIC_HANDLE *db_metric_handle);

extern STORAGE_COLLECT_HANDLE *rrddim_collect_init(STORAGE_METRIC_HANDLE *db_metric_handle);
extern void rrddim_collect_store_metric(STORAGE_COLLECT_HANDLE *collection_handle, usec_t point_in_time, NETDATA_DOUBLE number,
                                 NETDATA_DOUBLE min_value,
                                 NETDATA_DOUBLE max_value,
                                 uint16_t count,
                                 uint16_t anomaly_count,
                                 SN_FLAGS flags);
extern void rrddim_store_metric_flush(STORAGE_COLLECT_HANDLE *collection_handle);
extern int rrddim_collect_finalize(STORAGE_COLLECT_HANDLE *collection_handle);

extern void rrddim_query_init(STORAGE_METRIC_HANDLE *db_metric_handle, struct rrddim_query_handle *handle, time_t start_time, time_t end_time, TIER_QUERY_FETCH tier_query_fetch_type);
extern STORAGE_POINT rrddim_query_next_metric(struct rrddim_query_handle *handle);
extern int rrddim_query_is_finished(struct rrddim_query_handle *handle);
extern void rrddim_query_finalize(struct rrddim_query_handle *handle);
extern time_t rrddim_query_latest_time(STORAGE_METRIC_HANDLE *db_metric_handle);
extern time_t rrddim_query_oldest_time(STORAGE_METRIC_HANDLE *db_metric_handle);

#endif
