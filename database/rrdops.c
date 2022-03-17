#define NETDATA_RRD_INTERNALS
#include "rrd.h"

static inline void rrddim_memory_mode_init(RRDDIM *rd) {
    // init metric
    if (rd->rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE) {
#ifdef ENABLE_DBENGINE
        rrdeng_metric_init(rd);
#endif
    }

    // collect init
    if (rd->rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE) {
#ifdef ENABLE_DBENGINE
        rrdeng_store_metric_init(rd);
#endif
    } else {
        rrddim_collect_init(rd);
    }
}

void rrddim_initialize_metadata(RRDDIM *rd)
{
    if (!rd || rrddim_flag_check(rd, RRDDIM_FLAG_OPS_INITIALIZED))
        return;

#if defined(NETDATA_INTERNAL_CHECKS) && defined(NETDATA_VERIFY_LOCKS)
    int rc = netdata_rwlock_trywrlock(&rd->rrdset->rrdset_rwlock);
    if (rc == 0)
        fatal("called %s @ %s:%d without a rd/wr lock", __FUNCTION__, __FILE__, __LINE__);
#endif

    // promote to write lock
    netdata_rwlock_unlock(&rd->rrdset->rrdset_rwlock);
    netdata_rwlock_wrlock(&rd->rrdset->rrdset_rwlock);

    // double check for the unlikely scenario where another thread might
    // have acquired the write-lock before us
    if (rrddim_flag_check(rd, RRDDIM_FLAG_OPS_INITIALIZED))
        return;

    // find/create uuid for **all** dims and mark them active
    find_or_update_uuid_of_each_dimension(rd->rrdset);

    // initialize **all* dims for the given memory mode.
    for (RRDDIM *rdp = rd->rrdset->dimensions; rdp != NULL; rdp = rdp->next) {
        if (rrddim_flag_check(rdp, RRDDIM_FLAG_OPS_INITIALIZED))
            continue;

        rrddim_memory_mode_init(rdp);
        rrddim_flag_set(rdp, RRDDIM_FLAG_OPS_INITIALIZED);
    }
}

static void wrap_collect_init(RRDDIM *rd)
{
    if (rd->rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE) {
#ifdef ENABLE_DBENGINE
        rrdeng_store_metric_init(rd);
#endif
    } else {
        rrddim_collect_init(rd);
    }
}

static void wrap_collect_store_metric(RRDDIM *rd, usec_t point_in_time, storage_number number)
{
    rrddim_initialize_metadata(rd);

    if (rd->rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE) {
#ifdef ENABLE_DBENGINE
        rrdeng_store_metric_next(rd, point_in_time, number);
#endif
    } else {
        rrddim_collect_store_metric(rd, point_in_time, number);
    }
}

static int wrap_collect_finalize(RRDDIM *rd)
{
    rrddim_initialize_metadata(rd);

    if (rd->rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE) {
#ifdef ENABLE_DBENGINE
        return rrdeng_store_metric_finalize(rd);
#endif
    } else {
        return rrddim_collect_finalize(rd);
    }
}

static void wrap_query_init(RRDDIM *rd, struct rrddim_query_handle *handle, time_t start_time, time_t end_time)
{
    rrddim_initialize_metadata(rd);

    if (rd->rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE) {
#ifdef ENABLE_DBENGINE
        rrdeng_load_metric_init(rd, handle, start_time, end_time);
#endif
    } else {
        rrddim_query_init(rd,  handle, start_time, end_time);
    }
}

static storage_number wrap_query_next_metric(struct rrddim_query_handle *handle, time_t *current_time)
{
    rrddim_initialize_metadata(handle->rd);

    if (handle->rd->rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE) {
#ifdef ENABLE_DBENGINE
        return rrdeng_load_metric_next(handle, current_time);
#endif
    } else {
        return rrddim_query_next_metric(handle, current_time);
    }
}

static int wrap_query_is_finished(struct rrddim_query_handle *handle)
{
    rrddim_initialize_metadata(handle->rd);

    if (handle->rd->rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE) {
#ifdef ENABLE_DBENGINE
        return rrdeng_load_metric_is_finished(handle);
#endif
    } else {
        return rrddim_query_is_finished(handle);
    }
}

void wrap_query_finalize(struct rrddim_query_handle *handle)
{
    rrddim_initialize_metadata(handle->rd);

    if (handle->rd->rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE) {
#ifdef ENABLE_DBENGINE
        rrdeng_load_metric_finalize(handle);
#endif
    } else {
        rrddim_query_finalize(handle);
    }
}

time_t wrap_query_latest_time(RRDDIM *rd)
{
    rrddim_initialize_metadata(rd);

    if (rd->rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE) {
#ifdef ENABLE_DBENGINE
        return rrdeng_metric_latest_time(rd);
#endif
    } else {
        return rrddim_query_latest_time(rd);
    }
}

time_t wrap_query_oldest_time(RRDDIM *rd)
{
    rrddim_initialize_metadata(rd);

    if (rd->rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE) {
#ifdef ENABLE_DBENGINE
        return rrdeng_metric_oldest_time(rd);
#endif
    } else {
        return rrddim_query_oldest_time(rd);
    }
}

void rrdops_initialize(RRDDIM *rd) {
    rd->state->collect_ops.init = wrap_collect_init;
    rd->state->collect_ops.store_metric = wrap_collect_store_metric;
    rd->state->collect_ops.finalize = wrap_collect_finalize;

    rd->state->query_ops.init = wrap_query_init;
    rd->state->query_ops.next_metric = wrap_query_next_metric;
    rd->state->query_ops.is_finished = wrap_query_is_finished;
    rd->state->query_ops.finalize = wrap_query_finalize;

    rd->state->query_ops.latest_time = wrap_query_latest_time;
    rd->state->query_ops.oldest_time = wrap_query_oldest_time;
}
