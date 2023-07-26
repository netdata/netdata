// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_STORAGE_ENGINE_H
#define NETDATA_STORAGE_ENGINE_H

#include "ram/rrddim_mem.h"
#include "engine/rrddim_eng.h"

extern STORAGE_ENGINE_ID default_storage_engine_id;

static inline STORAGE_METRICS_GROUP *storage_engine_metrics_group_get(STORAGE_ENGINE_ID id, STORAGE_INSTANCE *instance, uuid_t *uuid)
{
    switch (id) {
        case STORAGE_ENGINE_NONE:
        case STORAGE_ENGINE_RAM:
        case STORAGE_ENGINE_MAP:
        case STORAGE_ENGINE_SAVE:
        case STORAGE_ENGINE_ALLOC:
            return rrddim_metrics_group_get(instance, uuid);
#ifdef ENABLE_DBENGINE
        case STORAGE_ENGINE_DBENGINE:
            return rrdeng_metrics_group_get(instance, uuid);
#endif
        default:
            __builtin_unreachable();
    }
}

static inline void storage_engine_metrics_group_release(STORAGE_ENGINE_ID id, STORAGE_INSTANCE *instance, STORAGE_METRICS_GROUP *smg)
{
    switch (id) {
        case STORAGE_ENGINE_NONE:
        case STORAGE_ENGINE_RAM:
        case STORAGE_ENGINE_MAP:
        case STORAGE_ENGINE_SAVE:
        case STORAGE_ENGINE_ALLOC:
            rrddim_metrics_group_release(instance, smg);
            break;
#ifdef ENABLE_DBENGINE
        case STORAGE_ENGINE_DBENGINE:
            rrdeng_metrics_group_release(instance, smg);
            break;
#endif
        default:
            __builtin_unreachable();
    }
}

static inline STORAGE_COLLECT_HANDLE *storage_metric_store_init(STORAGE_ENGINE_ID id, STORAGE_METRIC_HANDLE *metric_handle, uint32_t update_every, STORAGE_METRICS_GROUP *smg)
{
    switch (id) {
        case STORAGE_ENGINE_NONE:
        case STORAGE_ENGINE_RAM:
        case STORAGE_ENGINE_MAP:
        case STORAGE_ENGINE_SAVE:
        case STORAGE_ENGINE_ALLOC:
            return rrddim_collect_init(metric_handle, update_every, smg);
#ifdef ENABLE_DBENGINE
        case STORAGE_ENGINE_DBENGINE:
            return rrdeng_store_metric_init(metric_handle, update_every, smg);
#endif
        default:
            __builtin_unreachable();
    }
}

static inline void storage_engine_store_metric(
        STORAGE_ENGINE_ID id, STORAGE_COLLECT_HANDLE *collection_handle, usec_t point_in_time_ut,
        NETDATA_DOUBLE n, NETDATA_DOUBLE min_value, NETDATA_DOUBLE max_value,
        uint16_t count, uint16_t anomaly_count, SN_FLAGS flags)
{
    switch (id) {
        case STORAGE_ENGINE_NONE:
        case STORAGE_ENGINE_RAM:
        case STORAGE_ENGINE_MAP:
        case STORAGE_ENGINE_SAVE:
        case STORAGE_ENGINE_ALLOC:
            return rrddim_collect_store_metric(collection_handle, point_in_time_ut,
                                               n, min_value, max_value,
                                               count, anomaly_count, flags);
#ifdef ENABLE_DBENGINE
        case STORAGE_ENGINE_DBENGINE:
            return rrdeng_store_metric_next(collection_handle, point_in_time_ut,
                                            n, min_value, max_value,
                                            count, anomaly_count, flags);
#endif
        default:
            __builtin_unreachable();
    }
}

static inline size_t storage_engine_disk_space_max(STORAGE_ENGINE_ID id, STORAGE_INSTANCE *db_instance)
{
    switch (id) {
        case STORAGE_ENGINE_NONE:
        case STORAGE_ENGINE_RAM:
        case STORAGE_ENGINE_MAP:
        case STORAGE_ENGINE_SAVE:
        case STORAGE_ENGINE_ALLOC:
            return rrddim_disk_space_max(db_instance);
#ifdef ENABLE_DBENGINE
        case STORAGE_ENGINE_DBENGINE:
            return rrdeng_disk_space_max(db_instance);
#endif
        default:
            __builtin_unreachable();
    }
}

static inline size_t storage_engine_disk_space_used(STORAGE_ENGINE_ID id, STORAGE_INSTANCE *db_instance)
{
    switch (id) {
        case STORAGE_ENGINE_NONE:
        case STORAGE_ENGINE_RAM:
        case STORAGE_ENGINE_MAP:
        case STORAGE_ENGINE_SAVE:
        case STORAGE_ENGINE_ALLOC:
            return rrddim_disk_space_used(db_instance);
#ifdef ENABLE_DBENGINE
        case STORAGE_ENGINE_DBENGINE:
            return rrdeng_disk_space_max(db_instance);
#endif
        default:
            __builtin_unreachable();
    }
}

static inline time_t storage_engine_global_first_time_s(STORAGE_ENGINE_ID id, STORAGE_INSTANCE *db_instance)
{
    switch (id) {
        case STORAGE_ENGINE_NONE:
        case STORAGE_ENGINE_RAM:
        case STORAGE_ENGINE_MAP:
        case STORAGE_ENGINE_SAVE:
        case STORAGE_ENGINE_ALLOC:
            return rrddim_global_first_time_s(db_instance);
#ifdef ENABLE_DBENGINE
        case STORAGE_ENGINE_DBENGINE:
            return rrdeng_global_first_time_s(db_instance);
#endif
        default:
            __builtin_unreachable();
    }
}

static inline size_t storage_engine_collected_metrics(STORAGE_ENGINE_ID id, STORAGE_INSTANCE *db_instance)
{
    switch (id) {
        case STORAGE_ENGINE_NONE:
        case STORAGE_ENGINE_RAM:
        case STORAGE_ENGINE_MAP:
        case STORAGE_ENGINE_SAVE:
        case STORAGE_ENGINE_ALLOC:
            return rrddim_currently_collected_metrics(db_instance);
#ifdef ENABLE_DBENGINE
        case STORAGE_ENGINE_DBENGINE:
            return rrdeng_currently_collected_metrics(db_instance);
#endif
        default:
            __builtin_unreachable();
    }
}

static inline void storage_engine_store_flush(STORAGE_ENGINE_ID id, STORAGE_COLLECT_HANDLE *collection_handle)
{
    switch (id) {
        case STORAGE_ENGINE_NONE:
        case STORAGE_ENGINE_RAM:
        case STORAGE_ENGINE_MAP:
        case STORAGE_ENGINE_SAVE:
        case STORAGE_ENGINE_ALLOC:
            rrddim_store_metric_flush(collection_handle);
            return;
#ifdef ENABLE_DBENGINE
        case STORAGE_ENGINE_DBENGINE:
            rrdeng_store_metric_flush_current_page(collection_handle);
            return;
#endif
        default:
            __builtin_unreachable();
    }
}

// a finalization function to run after collection is over
// returns 1 if it's safe to delete the dimension
static inline int storage_engine_store_finalize(STORAGE_ENGINE_ID id, STORAGE_COLLECT_HANDLE *collection_handle)
{
    switch (id) {
        case STORAGE_ENGINE_NONE:
        case STORAGE_ENGINE_RAM:
        case STORAGE_ENGINE_MAP:
        case STORAGE_ENGINE_SAVE:
        case STORAGE_ENGINE_ALLOC:
            return rrddim_collect_finalize(collection_handle);
#ifdef ENABLE_DBENGINE
        case STORAGE_ENGINE_DBENGINE:
            return rrdeng_store_metric_finalize(collection_handle);
#endif
        default:
            __builtin_unreachable();
    }
}

static inline void storage_engine_store_change_collection_frequency(STORAGE_ENGINE_ID id, STORAGE_COLLECT_HANDLE *collection_handle, int update_every)
{
    switch (id) {
        case STORAGE_ENGINE_NONE:
        case STORAGE_ENGINE_RAM:
        case STORAGE_ENGINE_MAP:
        case STORAGE_ENGINE_SAVE:
        case STORAGE_ENGINE_ALLOC:
            rrddim_store_metric_change_collection_frequency(collection_handle, update_every);
            return;
#ifdef ENABLE_DBENGINE
        case STORAGE_ENGINE_DBENGINE:
            rrdeng_store_metric_change_collection_frequency(collection_handle, update_every);
            return;
#endif
        default:
            __builtin_unreachable();
    }
}

static inline time_t storage_engine_oldest_time_s(STORAGE_ENGINE_ID id, STORAGE_METRIC_HANDLE *db_metric_handle)
{
    switch (id) {
        case STORAGE_ENGINE_NONE:
        case STORAGE_ENGINE_RAM:
        case STORAGE_ENGINE_MAP:
        case STORAGE_ENGINE_SAVE:
        case STORAGE_ENGINE_ALLOC:
            return rrddim_query_oldest_time_s(db_metric_handle);
#ifdef ENABLE_DBENGINE
        case STORAGE_ENGINE_DBENGINE:
            return rrdeng_metric_oldest_time(db_metric_handle);
#endif
        default:
            __builtin_unreachable();
    }
}

static inline time_t storage_engine_latest_time_s(STORAGE_ENGINE_ID id, STORAGE_METRIC_HANDLE *db_metric_handle)
{
    switch (id) {
        case STORAGE_ENGINE_NONE:
        case STORAGE_ENGINE_RAM:
        case STORAGE_ENGINE_MAP:
        case STORAGE_ENGINE_SAVE:
        case STORAGE_ENGINE_ALLOC:
            return rrddim_query_latest_time_s(db_metric_handle);
#ifdef ENABLE_DBENGINE
        case STORAGE_ENGINE_DBENGINE:
            return rrdeng_metric_latest_time(db_metric_handle);
#endif
        default:
            __builtin_unreachable();
    }
}

static inline void storage_engine_query_init(
        STORAGE_ENGINE_ID id,
        STORAGE_METRIC_HANDLE *db_metric_handle, struct storage_engine_query_handle *handle,
                time_t start_time_s, time_t end_time_s, STORAGE_PRIORITY priority)
{
    handle->id = id;

    switch (id) {
        case STORAGE_ENGINE_NONE:
        case STORAGE_ENGINE_RAM:
        case STORAGE_ENGINE_MAP:
        case STORAGE_ENGINE_SAVE:
        case STORAGE_ENGINE_ALLOC:
            rrddim_query_init(db_metric_handle, handle, start_time_s, end_time_s, priority);
            return;
#ifdef ENABLE_DBENGINE
        case STORAGE_ENGINE_DBENGINE:
            rrdeng_load_metric_init(db_metric_handle, handle, start_time_s, end_time_s, priority);
            return;
#endif
        default:
            __builtin_unreachable();
    }
}

static inline STORAGE_POINT storage_engine_query_next_metric(struct storage_engine_query_handle *handle)
{
    switch (handle->id) {
        case STORAGE_ENGINE_NONE:
        case STORAGE_ENGINE_RAM:
        case STORAGE_ENGINE_MAP:
        case STORAGE_ENGINE_SAVE:
        case STORAGE_ENGINE_ALLOC:
            return rrddim_query_next_metric(handle);
#ifdef ENABLE_DBENGINE
        case STORAGE_ENGINE_DBENGINE:
            return rrdeng_load_metric_next(handle);
#endif
        default:
            __builtin_unreachable();
    }
}

static inline int storage_engine_query_is_finished(struct storage_engine_query_handle *handle)
{
    switch (handle->id) {
        case STORAGE_ENGINE_NONE:
        case STORAGE_ENGINE_RAM:
        case STORAGE_ENGINE_MAP:
        case STORAGE_ENGINE_SAVE:
        case STORAGE_ENGINE_ALLOC:
            return rrddim_query_is_finished(handle);
#ifdef ENABLE_DBENGINE
        case STORAGE_ENGINE_DBENGINE:
            return rrdeng_load_metric_is_finished(handle);
#endif
        default:
            __builtin_unreachable();
    }
}

static inline void storage_engine_query_finalize(struct storage_engine_query_handle *handle)
{
    switch (handle->id) {
        case STORAGE_ENGINE_NONE:
        case STORAGE_ENGINE_RAM:
        case STORAGE_ENGINE_MAP:
        case STORAGE_ENGINE_SAVE:
        case STORAGE_ENGINE_ALLOC:
            rrddim_query_finalize(handle);
            return;
#ifdef ENABLE_DBENGINE
        case STORAGE_ENGINE_DBENGINE:
            rrdeng_load_metric_finalize(handle);
            return;
#endif
        default:
            __builtin_unreachable();
    }
}

static inline time_t storage_engine_align_to_optimal_before(struct storage_engine_query_handle *handle)
{
    switch (handle->id) {
        case STORAGE_ENGINE_NONE:
        case STORAGE_ENGINE_RAM:
        case STORAGE_ENGINE_MAP:
        case STORAGE_ENGINE_SAVE:
        case STORAGE_ENGINE_ALLOC:
            return rrddim_query_align_to_optimal_before(handle);
#ifdef ENABLE_DBENGINE
        case STORAGE_ENGINE_DBENGINE:
            return rrdeng_load_align_to_optimal_before(handle);
#endif
        default:
            __builtin_unreachable();
    }
}

static inline bool storage_engine_metric_retention(STORAGE_ENGINE_ID id, STORAGE_INSTANCE *db_instance, uuid_t *uuid, time_t *first_entry_s, time_t *last_entry_s)
{
    switch (id) {
        case STORAGE_ENGINE_NONE:
        case STORAGE_ENGINE_RAM:
        case STORAGE_ENGINE_MAP:
        case STORAGE_ENGINE_SAVE:
        case STORAGE_ENGINE_ALLOC:
            return rrddim_metric_retention_by_uuid(db_instance, uuid, first_entry_s, last_entry_s);
#ifdef ENABLE_DBENGINE
        case STORAGE_ENGINE_DBENGINE:
            return rrdeng_metric_retention_by_uuid(db_instance, uuid, first_entry_s, last_entry_s);
#endif
        default:
            __builtin_unreachable();
    }
}

static inline STORAGE_METRIC_HANDLE *storage_engine_metric_get(STORAGE_ENGINE_ID id, STORAGE_INSTANCE *instance, uuid_t *uuid)
{
    switch (id) {
        case STORAGE_ENGINE_NONE:
        case STORAGE_ENGINE_RAM:
        case STORAGE_ENGINE_MAP:
        case STORAGE_ENGINE_SAVE:
        case STORAGE_ENGINE_ALLOC:
            return rrddim_metric_get(instance, uuid);
#ifdef ENABLE_DBENGINE
        case STORAGE_ENGINE_DBENGINE:
            return rrdeng_metric_get(instance, uuid);
#endif
        default:
            __builtin_unreachable();
    }
}

static inline STORAGE_METRIC_HANDLE *storage_engine_metric_get_or_create(RRDDIM *rd, STORAGE_ENGINE_ID id, STORAGE_INSTANCE *instance)
{
    switch (id) {
        case STORAGE_ENGINE_NONE:
        case STORAGE_ENGINE_RAM:
        case STORAGE_ENGINE_MAP:
        case STORAGE_ENGINE_SAVE:
        case STORAGE_ENGINE_ALLOC:
            return rrddim_metric_get_or_create(rd, instance);
#ifdef ENABLE_DBENGINE
        case STORAGE_ENGINE_DBENGINE:
            return rrdeng_metric_get_or_create(rd, instance);
#endif
        default:
            __builtin_unreachable();
    }
}

static inline void storage_engine_metric_release(STORAGE_ENGINE_ID id, STORAGE_METRIC_HANDLE *db_metric_handle)
{
    switch (id) {
        case STORAGE_ENGINE_NONE:
        case STORAGE_ENGINE_RAM:
        case STORAGE_ENGINE_MAP:
        case STORAGE_ENGINE_SAVE:
        case STORAGE_ENGINE_ALLOC:
            rrddim_metric_release(db_metric_handle);
            break;
#ifdef ENABLE_DBENGINE
        case STORAGE_ENGINE_DBENGINE:
            rrdeng_metric_release(db_metric_handle);
            break;
#endif
        default:
            __builtin_unreachable();
    }
}

static inline STORAGE_METRIC_HANDLE *storage_engine_metric_dup(STORAGE_ENGINE_ID id, STORAGE_METRIC_HANDLE *db_metric_handle)
{
    switch (id) {
        case STORAGE_ENGINE_NONE:
        case STORAGE_ENGINE_RAM:
        case STORAGE_ENGINE_MAP:
        case STORAGE_ENGINE_SAVE:
        case STORAGE_ENGINE_ALLOC:
            return rrddim_metric_dup(db_metric_handle);
#ifdef ENABLE_DBENGINE
        case STORAGE_ENGINE_DBENGINE:
            return rrdeng_metric_dup(db_metric_handle);
#endif
        default:
            __builtin_unreachable();
    }
}

#endif /* NETDATA_STORAGE_ENGINE_H */
