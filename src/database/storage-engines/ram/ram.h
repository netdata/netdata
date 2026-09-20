// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RAM_H
#define NETDATA_RAM_H

#include "libnetdata/libnetdata.h"
#include "database/storage-engines/storage-engine-types.h"

struct ram_collect_handle {
    struct storage_collect_handle common; // has to be first item

    STORAGE_METRIC_HANDLE *smh;
    RRDDIM *rd;
};

struct ram_query_handle {
    STORAGE_METRIC_HANDLE *smh;
    time_t dt;
    time_t next_timestamp;
    time_t last_timestamp;
    time_t slot_timestamp;
    size_t slot;
    size_t last_slot;
};

STORAGE_METRIC_HANDLE *ram_metric_get_or_create(RRDDIM *rd, STORAGE_INSTANCE *si);
STORAGE_METRIC_HANDLE *ram_metric_get_by_id(STORAGE_INSTANCE *si, UUIDMAP_ID id);
STORAGE_METRIC_HANDLE *ram_metric_get_by_uuid(STORAGE_INSTANCE *si, nd_uuid_t *uuid);
STORAGE_METRIC_HANDLE *ram_metric_dup(STORAGE_METRIC_HANDLE *smh);
void ram_metric_release(STORAGE_METRIC_HANDLE *smh);
bool ram_metric_release_from_rrddim(STORAGE_METRIC_HANDLE *smh, RRDDIM *rd);

bool ram_metric_retention_by_id(STORAGE_INSTANCE *si, UUIDMAP_ID id, time_t *first_entry_s, time_t *last_entry_s);
bool ram_metric_retention_by_uuid(STORAGE_INSTANCE *si, nd_uuid_t *uuid, time_t *first_entry_s, time_t *last_entry_s);
void ram_retention_delete_by_id(STORAGE_INSTANCE *si, UUIDMAP_ID id);

STORAGE_METRICS_GROUP *ram_metrics_group_get(void);
void ram_metrics_group_release(STORAGE_METRICS_GROUP *smg);

STORAGE_COLLECT_HANDLE *ram_store_init(STORAGE_METRIC_HANDLE *smh, uint32_t update_every, STORAGE_METRICS_GROUP *smg);
void ram_store_change_collection_frequency(STORAGE_COLLECT_HANDLE *sch, int update_every);
void ram_store_next(STORAGE_COLLECT_HANDLE *sch, usec_t point_in_time_ut, NETDATA_DOUBLE n,
                    NETDATA_DOUBLE min_value,
                    NETDATA_DOUBLE max_value,
                    uint16_t count,
                    uint16_t anomaly_count,
                    SN_FLAGS flags);
void ram_store_flush(STORAGE_COLLECT_HANDLE *sch);
int ram_store_finalize(STORAGE_COLLECT_HANDLE *sch);

void ram_query_init(STORAGE_METRIC_HANDLE *smh, struct storage_engine_query_handle *seqh, time_t start_time_s, time_t end_time_s, STORAGE_PRIORITY priority);
STORAGE_POINT ram_query_next(struct storage_engine_query_handle *seqh);
int ram_query_is_finished(struct storage_engine_query_handle *seqh);
void ram_query_finalize(struct storage_engine_query_handle *seqh);
time_t ram_latest_time_s(STORAGE_METRIC_HANDLE *smh);
time_t ram_oldest_time_s(STORAGE_METRIC_HANDLE *smh);
time_t ram_query_align_to_optimal_before(struct storage_engine_query_handle *seqh);

#endif
