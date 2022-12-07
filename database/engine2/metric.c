#include "metric.h"

typedef int32_t REFCOUNT;
#define REFCOUNT_DELETING (-100)

struct metric {
    uuid_t uuid;
    Word_t section;
    time_t first_time_t;
    time_t latest_time_t;
    time_t latest_update_every;
};

struct mrg {
    struct pgc_index {
        netdata_rwlock_t rwlock;
        Pvoid_t uuid_judy;          // each UUID has a JudyL of sections (tiers)
        Pvoid_t ptr_judy;           // reverse pointer lookup
    } index;
};

static void mrg_index_read_lock(MRG *mrg) {
    netdata_rwlock_rdlock(&mrg->index.rwlock);
}
static void mrg_index_read_unlock(MRG *mrg) {
    netdata_rwlock_unlock(&mrg->index.rwlock);
}
static void mrg_index_write_lock(MRG *mrg) {
    netdata_rwlock_wrlock(&mrg->index.rwlock);
}
static void mrg_index_write_unlock(MRG *mrg) {
    netdata_rwlock_unlock(&mrg->index.rwlock);
}

static bool metric_validate(MRG *mrg, METRIC *metric, bool having_lock) {
    if(!having_lock)
        mrg_index_read_lock(mrg);

    Pvoid_t *PValue = JudyLGet(mrg->index.ptr_judy, (Word_t)metric, PJE0);
    if(PValue == PJERR)
        fatal("DBENGINE METRIC: corrupted ptr judy array");

    METRIC *found = (PValue) ? *PValue : NULL;

    if(!having_lock)
        mrg_index_read_unlock(mrg);

    if(found != metric)
        return false;

    return true;
}

static METRIC *metric_add(MRG *mrg, MRG_ENTRY *entry, bool *ret) {
    mrg_index_write_lock(mrg);

    Pvoid_t *sections_judy_pptr = JudyHSIns(&mrg->index.uuid_judy, &entry->uuid, sizeof(uuid_t), PJE0);
    Pvoid_t *PValue = JudyLIns(sections_judy_pptr, entry->section, PJE0);
    if(*PValue != NULL) {
        METRIC *metric = *PValue;
        mrg_index_write_unlock(mrg);

        if(ret)
            *ret = false;

        return metric;
    }

    METRIC *metric = callocz(1, sizeof(METRIC));
    uuid_copy(metric->uuid, entry->uuid);
    metric->section = entry->section;
    metric->first_time_t = entry->first_time_t;
    metric->latest_time_t = entry->latest_time_t;
    metric->latest_update_every = entry->latest_update_every;

    *PValue = metric;

    PValue = JudyLIns(&mrg->index.ptr_judy, (Word_t)metric, PJE0);
    if(*PValue != NULL)
        fatal("DBENGINE METRIC: pointer already exists in registry.");

    *PValue = metric;

    internal_fatal(!metric_validate(mrg, metric, true),
                   "DBENGINE CACHE: metric validation on insertion fails");

    mrg_index_write_unlock(mrg);

    if(ret)
        *ret = true;

    return metric;
}

static METRIC *metric_get(MRG *mrg, uuid_t *uuid, Word_t section) {
    mrg_index_read_lock(mrg);

    Pvoid_t *sections_judy_pptr = JudyHSGet(mrg->index.uuid_judy, uuid, sizeof(uuid_t));
    if(!sections_judy_pptr) {
        mrg_index_read_unlock(mrg);
        return NULL;
    }

    Pvoid_t *PValue = JudyLGet(*sections_judy_pptr, section, PJE0);
    if(!PValue) {
        mrg_index_read_unlock(mrg);
        return NULL;
    }

    METRIC *metric = *PValue;

    internal_fatal(!metric_validate(mrg, metric, true),
                   "DBENGINE CACHE: metric validation on lookup fails");

    mrg_index_read_unlock(mrg);

    return metric;
}

static bool metric_del(MRG *mrg, METRIC *metric) {
    mrg_index_write_lock(mrg);

    if(!JudyLDel(&mrg->index.ptr_judy, (Word_t)metric, PJE0)) {
        mrg_index_write_unlock(mrg);
        return false;
    }

    Pvoid_t *sections_judy_pptr = JudyHSGet(mrg->index.uuid_judy, &metric->uuid, sizeof(uuid_t));
    if(!sections_judy_pptr || !*sections_judy_pptr)
        fatal("DBENGINE METRIC: uuid should be in judy but it is not.");

    if(!JudyLDel(sections_judy_pptr, metric->section, PJE0))
        fatal("DBENGINE METRIC: metric not found in sections judy");

    if(!*sections_judy_pptr) {
        if(!JudyHSDel(mrg->index.uuid_judy, &metric->uuid, sizeof(uuid_t), PJE0))
            fatal("DBENGINE METRIC: cannot delete UUID from judy");
    }

    mrg_index_write_unlock(mrg);

    freez(metric);

    return true;
}

// ----------------------------------------------------------------------------
// public API

METRIC *mrg_metric_add(MRG *mrg, MRG_ENTRY entry, bool *ret) {
    return metric_add(mrg, &entry, ret);
}

METRIC *mrg_metric_get(MRG *mrg, uuid_t *uuid, Word_t section) {
    return metric_get(mrg, uuid, section);
}

bool mrg_metric_del(MRG *mrg, METRIC *metric) {
    return metric_del(mrg, metric);
}

Word_t mrg_metric_id(MRG *mrg, METRIC *metric) {
    if(unlikely(!metric_validate(mrg, metric, false)))
        return 0;

    return (Word_t)metric;
}

uuid_t *mrg_metric_uuid(MRG *mrg, METRIC *metric) {
    if(unlikely(!metric_validate(mrg, metric, false)))
        return NULL;

    return &metric->uuid;
}

bool mrg_metric_set_first_time_t(MRG *mrg, METRIC *metric, time_t first_time_t) {
    if(unlikely(!metric_validate(mrg, metric, false)))
        return false;

    __atomic_store_n(&metric->first_time_t, first_time_t, __ATOMIC_RELEASE);
    return true;
}

time_t mrg_metric_get_first_time_t(MRG *mrg, METRIC *metric) {
    if(unlikely(!metric_validate(mrg, metric, false)))
        return 0;

    return __atomic_load_n(&metric->first_time_t, __ATOMIC_ACQUIRE);
}

bool mrg_metric_set_latest_time_t(MRG *mrg, METRIC *metric, time_t latest_time_t) {
    if(unlikely(!metric_validate(mrg, metric, false)))
        return false;

    __atomic_store_n(&metric->latest_time_t, latest_time_t, __ATOMIC_RELEASE);
    return true;
}

time_t mrg_metric_get_latest_time_t(MRG *mrg, METRIC *metric) {
    if(unlikely(!metric_validate(mrg, metric, false)))
        return 0;

    return __atomic_load_n(&metric->latest_time_t, __ATOMIC_ACQUIRE);
}

bool mrg_metric_set_update_every(MRG *mrg, METRIC *metric, time_t update_every) {
    if(unlikely(!metric_validate(mrg, metric, false)))
        return false;

    __atomic_store_n(&metric->latest_update_every, update_every, __ATOMIC_RELEASE);
    return true;
}

time_t mrg_metric_get_update_every(MRG *mrg, METRIC *metric) {
    if(unlikely(!metric_validate(mrg, metric, false)))
        return 0;

    return __atomic_load_n(&metric->latest_update_every, __ATOMIC_ACQUIRE);
}
