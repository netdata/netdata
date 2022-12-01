// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_STORAGEENGINEAPI_H
#define NETDATA_STORAGEENGINEAPI_H

#include "rrd.h"

STORAGE_METRIC_HANDLE *se_metric_get(RRD_MEMORY_MODE mode,
                                     STORAGE_INSTANCE *db_instance,
                                     uuid_t *uuid,
                                     STORAGE_METRICS_GROUP *smg);

STORAGE_METRIC_HANDLE *se_metric_get_or_create(RRD_MEMORY_MODE mode,
                                               RRDDIM *rd,
                                               STORAGE_INSTANCE *db_instance,
                                               STORAGE_METRICS_GROUP *smg);

STORAGE_METRIC_HANDLE *se_metric_dup(RRD_MEMORY_MODE mode,
                                     STORAGE_METRIC_HANDLE *db_metric_handle);

void se_metric_release(RRD_MEMORY_MODE mode, STORAGE_METRIC_HANDLE *db_metric_handle);

STORAGE_COLLECT_HANDLE *se_store_metric_init(RRD_MEMORY_MODE mode,
                                             STORAGE_METRIC_HANDLE *db_metric_handle,
                                             uint32_t update_every);

void se_store_metric_next(RRD_MEMORY_MODE mode, STORAGE_COLLECT_HANDLE *collection_handle,
                          usec_t point_in_time_ut, NETDATA_DOUBLE n,
                          NETDATA_DOUBLE min_value, NETDATA_DOUBLE max_value, uint16_t count,
                          uint16_t anomaly_count, SN_FLAGS flags);

void se_store_metric_flush_current_page(RRD_MEMORY_MODE mode, STORAGE_COLLECT_HANDLE *collection_handle);

int se_collect_finalize(RRD_MEMORY_MODE mode, STORAGE_COLLECT_HANDLE *collection_handle);

void se_store_metric_change_collection_frequency(RRD_MEMORY_MODE mode,
                                                 STORAGE_COLLECT_HANDLE *collection_handle,
                                                 int update_every);

STORAGE_METRICS_GROUP *se_metrics_group_get(RRD_MEMORY_MODE mode,
                                            STORAGE_INSTANCE *db_instance,
                                            uuid_t *uuid);

void se_metrics_group_release(RRD_MEMORY_MODE mode,
                              STORAGE_INSTANCE *db_instance,
                              STORAGE_METRICS_GROUP *smg);

void se_query_init(RRD_MEMORY_MODE mode,
                   STORAGE_METRIC_HANDLE *db_metric_handle,
                   struct storage_engine_query_handle *handle,
                   time_t start_time, time_t end_time);

STORAGE_POINT se_query_next_metric(RRD_MEMORY_MODE mode, struct storage_engine_query_handle *handle);

int se_query_is_finished(RRD_MEMORY_MODE mode, struct storage_engine_query_handle *handle);

void se_query_finalize(RRD_MEMORY_MODE mode, struct storage_engine_query_handle *handle);

time_t se_metric_latest_time(RRD_MEMORY_MODE mode, STORAGE_METRIC_HANDLE *db_metric_handle);

time_t se_metric_oldest_time(RRD_MEMORY_MODE mode, STORAGE_METRIC_HANDLE *db_metric_handle);

#endif
