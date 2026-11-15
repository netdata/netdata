// SPDX-License-Identifier: GPL-3.0-or-later

// db = none: tier 0 keeps no data. Metrics are collected and streamed, but no point is stored, so there is nothing to
// query or replicate.
//
// Each dimension still gets a small metric handle that remembers when its last point was collected and reports that
// one instant as its retention: the contexts layer deletes a collected metric whose retention is zero
// (rrdmetric_update_retention()). The handle is reference counted, because queries keep their copy of it
// (query_metric_add()) independently of the dimension. Once the dimension is freed nothing can find it again, so
// get_by_id/uuid return NULL and its metric loses its retention.

#include "rrd.h"

struct storage_none_metric {
    STORAGE_COLLECT_HANDLE collect;     // the collect handle of this metric: the collector holds a reference with it
    REFCOUNT refcount;
    time_t last_s;                      // when the last point was collected; 0 before the first one
};

STORAGE_METRIC_HANDLE *storage_none_metric_get_or_create(RRDDIM *rd __maybe_unused, STORAGE_INSTANCE *si __maybe_unused) {
    struct storage_none_metric *m = callocz(1, sizeof(*m));
    m->collect.seb = STORAGE_ENGINE_BACKEND_NONE;
    m->refcount = 1;
    return (STORAGE_METRIC_HANDLE *)m;
}

STORAGE_METRIC_HANDLE *storage_none_metric_get_by_id(STORAGE_INSTANCE *si __maybe_unused, UUIDMAP_ID id __maybe_unused) {
    return NULL;
}

STORAGE_METRIC_HANDLE *storage_none_metric_get_by_uuid(STORAGE_INSTANCE *si __maybe_unused, nd_uuid_t *uuid __maybe_unused) {
    return NULL;
}

STORAGE_METRIC_HANDLE *storage_none_metric_dup(STORAGE_METRIC_HANDLE *smh) {
    struct storage_none_metric *m = (struct storage_none_metric *)smh;

    if(!refcount_acquire(&m->refcount))
        fatal("STORAGE NONE: cannot acquire a released metric");

    return smh;
}

void storage_none_metric_release(STORAGE_METRIC_HANDLE *smh) {
    struct storage_none_metric *m = (struct storage_none_metric *)smh;

    if(refcount_release_and_acquire_for_deletion(&m->refcount))
        freez(m);
}

bool storage_none_metric_retention_by_id(STORAGE_INSTANCE *si __maybe_unused, UUIDMAP_ID id __maybe_unused, time_t *first_entry_s, time_t *last_entry_s) {
    *first_entry_s = 0;
    *last_entry_s = 0;
    return false;
}

bool storage_none_metric_retention_by_uuid(STORAGE_INSTANCE *si __maybe_unused, nd_uuid_t *uuid __maybe_unused, time_t *first_entry_s, time_t *last_entry_s) {
    *first_entry_s = 0;
    *last_entry_s = 0;
    return false;
}

void storage_none_metric_retention_delete_by_id(STORAGE_INSTANCE *si __maybe_unused, UUIDMAP_ID id __maybe_unused) {
    ;
}

// ----------------------------------------------------------------------------
// collection

STORAGE_COLLECT_HANDLE *storage_none_collect_init(STORAGE_METRIC_HANDLE *smh) {
    struct storage_none_metric *m = (struct storage_none_metric *)storage_none_metric_dup(smh);
    return &m->collect;
}

void storage_none_store_metric(STORAGE_COLLECT_HANDLE *sch, usec_t point_in_time_ut) {
    struct storage_none_metric *m = (struct storage_none_metric *)sch;
    __atomic_store_n(&m->last_s, (time_t)(point_in_time_ut / USEC_PER_SEC), __ATOMIC_RELAXED);
}

// no data is kept: always safe to delete
int storage_none_collect_finalize(STORAGE_COLLECT_HANDLE *sch) {
    storage_none_metric_release((STORAGE_METRIC_HANDLE *)sch);
    return 1;
}

// ----------------------------------------------------------------------------
// queries: the retention is the instant of the last point, and it holds no points

time_t storage_none_latest_time_s(STORAGE_METRIC_HANDLE *smh) {
    struct storage_none_metric *m = (struct storage_none_metric *)smh;
    return __atomic_load_n(&m->last_s, __ATOMIC_RELAXED);
}
