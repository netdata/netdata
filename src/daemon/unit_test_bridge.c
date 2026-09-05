// SPDX-License-Identifier: GPL-3.0-or-later

#include "unit_test_bridge.h"

NOINLINE void unittest_storage_engine_store_metric(
    STORAGE_COLLECT_HANDLE *sch,
    usec_t point_in_time_ut,
    NETDATA_DOUBLE n,
    NETDATA_DOUBLE min_value,
    NETDATA_DOUBLE max_value,
    uint16_t count,
    uint16_t anomaly_count,
    SN_FLAGS flags) {
    storage_engine_store_metric(
        sch, point_in_time_ut, n, min_value, max_value, count, anomaly_count, flags);
}

NOINLINE void unittest_storage_engine_store_change_collection_frequency(
    STORAGE_COLLECT_HANDLE *sch, int update_every) {
    storage_engine_store_change_collection_frequency(sch, update_every);
}

NOINLINE void unittest_storage_engine_store_flush(STORAGE_COLLECT_HANDLE *sch) {
    storage_engine_store_flush(sch);
}

NOINLINE void unittest_storage_engine_query_init(
    STORAGE_ENGINE_BACKEND seb,
    STORAGE_METRIC_HANDLE *smh,
    struct storage_engine_query_handle *seqh,
    time_t start_time_s,
    time_t end_time_s,
    STORAGE_PRIORITY priority) {
    storage_engine_query_init(seb, smh, seqh, start_time_s, end_time_s, priority);
}

NOINLINE STORAGE_POINT unittest_storage_engine_query_next_metric(
    struct storage_engine_query_handle *seqh) {
    return storage_engine_query_next_metric(seqh);
}

NOINLINE int unittest_storage_engine_query_is_finished(
    struct storage_engine_query_handle *seqh) {
    return storage_engine_query_is_finished(seqh);
}

NOINLINE void unittest_storage_engine_query_finalize(
    struct storage_engine_query_handle *seqh) {
    storage_engine_query_finalize(seqh);
}
