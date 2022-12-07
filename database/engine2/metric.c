#include "metric.h"

typedef int32_t REFCOUNT;
#define REFCOUNT_DELETING (-100)

struct metric {
    uuid_t uuid;
    Word_t section;
    size_t pages;
    time_t first_time_t;
    time_t last_time_t;
    time_t latest_update_every;
};

struct mrg {
    struct pgc_index {
        netdata_rwlock_t rwlock;
        Pvoid_t uuid_judy;          // each UUID has a JudyL of sections
        Pvoid_t ptr_judy;
    } index;
};

static void mrg_index_read_lock(MRG *mrg) {
    netdata_rwlock_rdlock(&mrg->index.rwlock);
}
static void mrg_index_read_unlock(MRG *mrg) {
    netdata_rwlock_unlock(&mrg->index.rwlock);
}
static bool mrg_index_write_trylock(MRG *mrg) {
    return !netdata_rwlock_trywrlock(&mrg->index.rwlock);
}
static void mrg_index_write_lock(MRG *mrg) {
    netdata_rwlock_wrlock(&mrg->index.rwlock);
}
static void mrg_index_write_unlock(MRG *mrg) {
    netdata_rwlock_unlock(&mrg->index.rwlock);
}

bool metric_validate(MRG *mrg, METRIC *metric, bool having_lock) {
    // FIXME - validate 'metric' is a valid, active metric

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

METRIC *metric_add(MRG *mrg, MRG_ENTRY *entry) {
    mrg_index_write_lock(mrg);

    Pvoid_t *sections_judy_pptr = JudyHSIns(&mrg->index.uuid_judy, &entry->uuid, sizeof(uuid_t), PJE0);
    Pvoid_t *PValue = JudyLIns(sections_judy_pptr, entry->section, PJE0);
    if(*PValue != NULL) {
        METRIC *mtrc = *PValue;
        mrg_index_write_unlock(mrg);
        return mtrc;
    }

    METRIC *mtrc = callocz(1, sizeof(METRIC));
    uuid_copy(mtrc->uuid, entry->uuid);
    mtrc->section = entry->section;
    mtrc->pages = entry->pages;
    mtrc->first_time_t = entry->first_time_t;
    mtrc->last_time_t = entry->last_time_t;
    mtrc->latest_update_every = entry->latest_update_every;

    *PValue = mtrc;

    PValue = JudyLIns(&mrg->index.ptr_judy, (Word_t)mtrc, PJE0);
    if(*PValue != NULL)
        fatal("DBENGINE METRIC: pointer already exists in registry.");

    *PValue = mtrc;

    mrg_index_write_unlock(mrg);

    return mtrc;
}

METRIC *metric_get(MRG *mrg, uuid_t *uuid, Word_t section) {

}

bool metric_del_by_ptr(MRG *mrg, METRIC *metric) {
    if(unlikely(!metric_validate(mrg, metric, false)))
        return false;

}

Word_t metric_id(MRG *mrg, METRIC *metric) {
    if(unlikely(!metric_validate(mrg, metric, false)))
        return 0;

    return (Word_t)metric;
}

uuid_t *metric_uuid(MRG *mrg, METRIC *metric) {
    if(unlikely(!metric_validate(mrg, metric, false)))
        return NULL;

    return &metric->uuid;
}
