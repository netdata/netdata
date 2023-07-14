// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDDIM_ENG_H
#define NETDATA_RRDDIM_ENG_H

#include "database/storage_engine_types.h"

STORAGE_METRICS_GROUP *rrdeng_metrics_group_get(STORAGE_INSTANCE *db_instance, uuid_t *uuid);

void rrdeng_metrics_group_release(STORAGE_INSTANCE *db_instance, STORAGE_METRICS_GROUP *smg);

STORAGE_COLLECT_HANDLE *rrdeng_store_metric_init(STORAGE_METRIC_HANDLE *db_metric_handle, uint32_t update_every, STORAGE_METRICS_GROUP *smg);
void rrdeng_store_metric_next(STORAGE_COLLECT_HANDLE *collection_handle, usec_t point_in_time_ut, NETDATA_DOUBLE n, NETDATA_DOUBLE min_value, NETDATA_DOUBLE max_value, uint16_t count, uint16_t anomaly_count, SN_FLAGS flags);
void rrdeng_store_metric_flush_current_page(STORAGE_COLLECT_HANDLE *collection_handle);
void rrdeng_store_metric_change_collection_frequency(STORAGE_COLLECT_HANDLE *collection_handle, int update_every);

size_t rrdeng_disk_space_max(STORAGE_INSTANCE *db_instance);

size_t rrdeng_disk_space_used(STORAGE_INSTANCE *db_instance);

STORAGE_METRIC_HANDLE *rrdeng_metric_get_or_create(RRDDIM *rd, STORAGE_INSTANCE *db_instance);
STORAGE_METRIC_HANDLE *rrdeng_metric_get(STORAGE_INSTANCE *db_instance, uuid_t *uuid);
void rrdeng_metric_release(STORAGE_METRIC_HANDLE *db_metric_handle);
STORAGE_METRIC_HANDLE *rrdeng_metric_dup(STORAGE_METRIC_HANDLE *db_metric_handle);


void rrdeng_load_metric_init(STORAGE_METRIC_HANDLE *db_metric_handle, struct storage_engine_query_handle *rrddim_handle, time_t start_time_s, time_t end_time_s, STORAGE_PRIORITY priority);
STORAGE_POINT rrdeng_load_metric_next(struct storage_engine_query_handle *rrddim_handle);
int rrdeng_load_metric_is_finished(struct storage_engine_query_handle *rrddim_handle);
void rrdeng_load_metric_finalize(struct storage_engine_query_handle *rrddim_handle);
time_t rrdeng_load_align_to_optimal_before(struct storage_engine_query_handle *rrddim_handle);

time_t rrdeng_metric_latest_time(STORAGE_METRIC_HANDLE *db_metric_handle);
time_t rrdeng_metric_oldest_time(STORAGE_METRIC_HANDLE *db_metric_handle);


bool rrdeng_metric_retention_by_uuid(STORAGE_INSTANCE *db_instance, uuid_t *dim_uuid, time_t *first_entry_s, time_t *last_entry_s);

time_t rrdeng_global_first_time_s(STORAGE_INSTANCE *db_instance);

int rrdeng_store_metric_finalize(STORAGE_COLLECT_HANDLE *collection_handle);

size_t rrdeng_currently_collected_metrics(STORAGE_INSTANCE *db_instance);

time_t rrdeng_metric_oldest_time(STORAGE_METRIC_HANDLE *db_metric_handle);
time_t rrdeng_metric_latest_time(STORAGE_METRIC_HANDLE *db_metric_handle);

int rrdeng_load_metric_is_finished(struct storage_engine_query_handle *rrddim_handle);
void rrdeng_load_metric_finalize(struct storage_engine_query_handle *rrddim_handle);
time_t rrdeng_load_align_to_optimal_before(struct storage_engine_query_handle *rrddim_handle);

// The following functions are dbengine-specific. Include them here until
// we refactor the code so that, either we don't need them, or settle to a
// common API.

bool rrdeng_is_legacy(STORAGE_INSTANCE *db_instance);

void rrdeng_get_37_statistics(STORAGE_INSTANCE *db_instance, unsigned long long *array);

int rrdeng_init(STORAGE_INSTANCE **db_instance, const char *dbfiles_path,
                       unsigned disk_space_mb, size_t tier);

void rrdeng_readiness_wait(STORAGE_INSTANCE *db_instance);
void rrdeng_exit_mode(STORAGE_INSTANCE *db_instance);

int rrdeng_exit(STORAGE_INSTANCE *ctx);
void rrdeng_prepare_exit(STORAGE_INSTANCE *ctx);

size_t rrdeng_collectors_running(STORAGE_INSTANCE *db_instance);

void rrdeng_size_statistics(STORAGE_INSTANCE *db_instance, BUFFER *wb);

#endif /* NETDATA_RRDDIM_ENG_H */
