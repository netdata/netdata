// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDDIMMEM_H
#define NETDATA_RRDDIMMEM_H

#include "database/rrd.h"

struct mem_collect_handle {
    struct storage_collect_handle common; // has to be first item

    STORAGE_METRIC_HANDLE *smh;
    RRDDIM *rd;
};

struct mem_query_handle {
    STORAGE_METRIC_HANDLE *smh;
    time_t dt;
    time_t next_timestamp;
    time_t last_timestamp;
    time_t slot_timestamp;
    size_t slot;
    size_t last_slot;
};

STORAGE_METRIC_HANDLE *rrddim_metric_get_or_create(RRDDIM *rd, STORAGE_INSTANCE *si);
STORAGE_METRIC_HANDLE *rrddim_metric_get_by_id(STORAGE_INSTANCE *si, UUIDMAP_ID id);
STORAGE_METRIC_HANDLE *rrddim_metric_get_by_uuid(STORAGE_INSTANCE *si, nd_uuid_t *uuid);
STORAGE_METRIC_HANDLE *rrddim_metric_dup(STORAGE_METRIC_HANDLE *smh);
void rrddim_metric_release(STORAGE_METRIC_HANDLE *smh);

bool rrddim_metric_retention_by_id(STORAGE_INSTANCE *si, UUIDMAP_ID id, time_t *first_entry_s, time_t *last_entry_s);
bool rrddim_metric_retention_by_uuid(STORAGE_INSTANCE *si, nd_uuid_t *uuid, time_t *first_entry_s, time_t *last_entry_s);
void rrddim_retention_delete_by_id(STORAGE_INSTANCE *si, UUIDMAP_ID id);

STORAGE_METRICS_GROUP *rrddim_metrics_group_get(STORAGE_INSTANCE *si, nd_uuid_t *uuid);
void rrddim_metrics_group_release(STORAGE_INSTANCE *si, STORAGE_METRICS_GROUP *smg);

STORAGE_COLLECT_HANDLE *rrddim_collect_init(STORAGE_METRIC_HANDLE *smh, uint32_t update_every, STORAGE_METRICS_GROUP *smg);
void rrddim_store_metric_change_collection_frequency(STORAGE_COLLECT_HANDLE *sch, int update_every);
void rrddim_collect_store_metric(STORAGE_COLLECT_HANDLE *sch, usec_t point_in_time_ut, NETDATA_DOUBLE n,
                                 NETDATA_DOUBLE min_value,
                                 NETDATA_DOUBLE max_value,
                                 uint16_t count,
                                 uint16_t anomaly_count,
                                 SN_FLAGS flags);
void rrddim_store_metric_flush(STORAGE_COLLECT_HANDLE *sch);
int rrddim_collect_finalize(STORAGE_COLLECT_HANDLE *sch);

void rrddim_query_init(STORAGE_METRIC_HANDLE *smh, struct storage_engine_query_handle *seqh, time_t start_time_s, time_t end_time_s, STORAGE_PRIORITY priority);
STORAGE_POINT rrddim_query_next_metric(struct storage_engine_query_handle *seqh);
int rrddim_query_is_finished(struct storage_engine_query_handle *seqh);
void rrddim_query_finalize(struct storage_engine_query_handle *seqh);
time_t rrddim_query_latest_time_s(STORAGE_METRIC_HANDLE *smh);
time_t rrddim_query_oldest_time_s(STORAGE_METRIC_HANDLE *smh);
time_t rrddim_query_align_to_optimal_before(struct storage_engine_query_handle *seqh);

#endif
