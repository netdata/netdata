// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_UNIT_TEST_BRIDGE_H
#define NETDATA_UNIT_TEST_BRIDGE_H

#include "common.h"

void unittest_storage_engine_store_metric(
    STORAGE_COLLECT_HANDLE *sch,
    usec_t point_in_time_ut,
    NETDATA_DOUBLE n,
    NETDATA_DOUBLE min_value,
    NETDATA_DOUBLE max_value,
    uint16_t count,
    uint16_t anomaly_count,
    SN_FLAGS flags);

void unittest_storage_engine_store_change_collection_frequency(STORAGE_COLLECT_HANDLE *sch, int update_every);
void unittest_storage_engine_store_flush(STORAGE_COLLECT_HANDLE *sch);

void unittest_storage_engine_query_init(
    STORAGE_ENGINE_BACKEND seb,
    STORAGE_METRIC_HANDLE *smh,
    struct storage_engine_query_handle *seqh,
    time_t start_time_s,
    time_t end_time_s,
    STORAGE_PRIORITY priority);

STORAGE_POINT unittest_storage_engine_query_next_metric(struct storage_engine_query_handle *seqh);
int unittest_storage_engine_query_is_finished(struct storage_engine_query_handle *seqh);
void unittest_storage_engine_query_finalize(struct storage_engine_query_handle *seqh);

#endif // NETDATA_UNIT_TEST_BRIDGE_H
