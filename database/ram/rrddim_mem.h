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

STORAGE_METRIC_HANDLE *rrddim_metric_get_or_create(RRDDIM *rd, STORAGE_INSTANCE *db_instance, STORAGE_METRICS_GROUP *smg);
STORAGE_METRIC_HANDLE *rrddim_metric_get(STORAGE_INSTANCE *db_instance, uuid_t *uuid, STORAGE_METRICS_GROUP *smg);
STORAGE_METRIC_HANDLE *rrddim_metric_dup(STORAGE_METRIC_HANDLE *db_metric_handle);
void rrddim_metric_release(STORAGE_METRIC_HANDLE *db_metric_handle);

STORAGE_METRICS_GROUP *rrddim_metrics_group_get(STORAGE_INSTANCE *db_instance, uuid_t *uuid);
void rrddim_metrics_group_release(STORAGE_INSTANCE *db_instance, STORAGE_METRICS_GROUP *smg);

STORAGE_COLLECT_HANDLE *rrddim_collect_init(STORAGE_METRIC_HANDLE *db_metric_handle, uint32_t update_every);
void rrddim_store_metric_change_collection_frequency(STORAGE_COLLECT_HANDLE *collection_handle, int update_every);
void rrddim_collect_store_metric(STORAGE_COLLECT_HANDLE *collection_handle, usec_t point_in_time, NETDATA_DOUBLE number,
                                 NETDATA_DOUBLE min_value,
                                 NETDATA_DOUBLE max_value,
                                 uint16_t count,
                                 uint16_t anomaly_count,
                                 SN_FLAGS flags);
void rrddim_store_metric_flush(STORAGE_COLLECT_HANDLE *collection_handle);
int rrddim_collect_finalize(STORAGE_COLLECT_HANDLE *collection_handle);

void rrddim_query_init(STORAGE_METRIC_HANDLE *db_metric_handle, struct storage_engine_query_handle *handle, time_t start_time, time_t end_time);
STORAGE_POINT rrddim_query_next_metric(struct storage_engine_query_handle *handle);
int rrddim_query_is_finished(struct storage_engine_query_handle *handle);
void rrddim_query_finalize(struct storage_engine_query_handle *handle);
time_t rrddim_query_latest_time(STORAGE_METRIC_HANDLE *db_metric_handle);
time_t rrddim_query_oldest_time(STORAGE_METRIC_HANDLE *db_metric_handle);

#endif
