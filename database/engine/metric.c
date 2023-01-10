#include "metric.h"

typedef int32_t REFCOUNT;
#define REFCOUNT_DELETING (-100)

struct metric {
    uuid_t uuid;                    // never changes
    Word_t section;                 // never changes
    time_t first_time_s;            //
    time_t latest_time_s_clean;     // archived pages latest time
    time_t latest_time_s_hot;       // latest time of the currently collected page
    uint32_t latest_update_every_s; //
    SPINLOCK timestamps_lock;       // protects the 3 timestamps

    // THIS IS allocated with malloc()
    // YOU HAVE TO INITIALIZE IT YOURSELF !
};

struct mrg {
    struct pgc_index {
        ARAL *aral;
        netdata_rwlock_t rwlock;
        Pvoid_t uuid_judy;          // each UUID has a JudyL of sections (tiers)
    } index;

    struct mrg_statistics stats;
};

static inline void MRG_STATS_DUPLICATE_ADD(MRG *mrg) {
    __atomic_add_fetch(&mrg->stats.additions_duplicate, 1, __ATOMIC_RELAXED);
}

static inline void MRG_STATS_ADDED_METRIC(MRG *mrg) {
    __atomic_add_fetch(&mrg->stats.entries, 1, __ATOMIC_RELAXED);
    __atomic_add_fetch(&mrg->stats.additions, 1, __ATOMIC_RELAXED);
    __atomic_add_fetch(&mrg->stats.size, sizeof(METRIC), __ATOMIC_RELAXED);
}

static inline void MRG_STATS_DELETED_METRIC(MRG *mrg) {
    __atomic_sub_fetch(&mrg->stats.entries, 1, __ATOMIC_RELAXED);
    __atomic_sub_fetch(&mrg->stats.size, sizeof(METRIC), __ATOMIC_RELAXED);
    __atomic_add_fetch(&mrg->stats.deletions, 1, __ATOMIC_RELAXED);
}

static inline void MRG_STATS_SEARCH_HIT(MRG *mrg) {
    __atomic_add_fetch(&mrg->stats.search_hits, 1, __ATOMIC_RELAXED);
}

static inline void MRG_STATS_SEARCH_MISS(MRG *mrg) {
    __atomic_add_fetch(&mrg->stats.search_misses, 1, __ATOMIC_RELAXED);
}

static inline void MRG_STATS_DELETE_MISS(MRG *mrg) {
    __atomic_add_fetch(&mrg->stats.delete_misses, 1, __ATOMIC_RELAXED);
}

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

static inline void mrg_stats_size_judyl_change(MRG *mrg, size_t mem_before_judyl, size_t mem_after_judyl) {
    if(mem_after_judyl > mem_before_judyl)
        __atomic_add_fetch(&mrg->stats.size, mem_after_judyl - mem_before_judyl, __ATOMIC_RELAXED);
    else if(mem_after_judyl < mem_before_judyl)
        __atomic_sub_fetch(&mrg->stats.size, mem_before_judyl - mem_after_judyl, __ATOMIC_RELAXED);
}

static inline void mrg_stats_size_judyhs_added_uuid(MRG *mrg) {
    __atomic_add_fetch(&mrg->stats.size, sizeof(uuid_t) * 3, __ATOMIC_RELAXED);
}

static inline void mrg_stats_size_judyhs_removed_uuid(MRG *mrg) {
    __atomic_sub_fetch(&mrg->stats.size, sizeof(uuid_t) * 3, __ATOMIC_RELAXED);
}

static METRIC *metric_add(MRG *mrg, MRG_ENTRY *entry, bool *ret) {
    mrg_index_write_lock(mrg);

    size_t mem_before_judyl, mem_after_judyl;

    Pvoid_t *sections_judy_pptr = JudyHSIns(&mrg->index.uuid_judy, &entry->uuid, sizeof(uuid_t), PJE0);
    if(!sections_judy_pptr || sections_judy_pptr == PJERR)
        fatal("DBENGINE METRIC: corrupted UUIDs JudyHS array");

    if(!*sections_judy_pptr)
        mrg_stats_size_judyhs_added_uuid(mrg);

    mem_before_judyl = JudyLMemUsed(*sections_judy_pptr);
    Pvoid_t *PValue = JudyLIns(sections_judy_pptr, entry->section, PJE0);
    mem_after_judyl = JudyLMemUsed(*sections_judy_pptr);
    mrg_stats_size_judyl_change(mrg, mem_before_judyl, mem_after_judyl);

    if(!PValue || PValue == PJERR)
        fatal("DBENGINE METRIC: corrupted section JudyL array");

    if(*PValue != NULL) {
        METRIC *metric = *PValue;
        mrg_index_write_unlock(mrg);

        if(ret)
            *ret = false;

        MRG_STATS_DUPLICATE_ADD(mrg);
        return metric;
    }

    METRIC *metric = arrayalloc_mallocz(mrg->index.aral);
    uuid_copy(metric->uuid, entry->uuid);
    metric->section = entry->section;
    metric->first_time_s = entry->first_time_s;
    metric->latest_time_s_clean = entry->last_time_s;
    metric->latest_time_s_hot = 0;
    metric->latest_update_every_s = entry->latest_update_every_s;
    metric->timestamps_lock = NETDATA_SPINLOCK_INITIALIZER;
    *PValue = metric;

    mrg_index_write_unlock(mrg);

    if(ret)
        *ret = true;

    MRG_STATS_ADDED_METRIC(mrg);

    return metric;
}

static METRIC *metric_get(MRG *mrg, uuid_t *uuid, Word_t section) {
    mrg_index_read_lock(mrg);

    Pvoid_t *sections_judy_pptr = JudyHSGet(mrg->index.uuid_judy, uuid, sizeof(uuid_t));
    if(!sections_judy_pptr) {
        mrg_index_read_unlock(mrg);
        MRG_STATS_SEARCH_MISS(mrg);
        return NULL;
    }

    Pvoid_t *PValue = JudyLGet(*sections_judy_pptr, section, PJE0);
    if(!PValue) {
        mrg_index_read_unlock(mrg);
        MRG_STATS_SEARCH_MISS(mrg);
        return NULL;
    }

    METRIC *metric = *PValue;

    mrg_index_read_unlock(mrg);

    MRG_STATS_SEARCH_HIT(mrg);
    return metric;
}

static bool metric_del(MRG *mrg, METRIC *metric) {
    size_t mem_before_judyl, mem_after_judyl;

    mrg_index_write_lock(mrg);

    Pvoid_t *sections_judy_pptr = JudyHSGet(mrg->index.uuid_judy, &metric->uuid, sizeof(uuid_t));
    if(!sections_judy_pptr || !*sections_judy_pptr) {
        mrg_index_write_unlock(mrg);
        MRG_STATS_DELETE_MISS(mrg);
        return false;
    }

    mem_before_judyl = JudyLMemUsed(*sections_judy_pptr);
    int rc = JudyLDel(sections_judy_pptr, metric->section, PJE0);
    mem_after_judyl = JudyLMemUsed(*sections_judy_pptr);
    mrg_stats_size_judyl_change(mrg, mem_before_judyl, mem_after_judyl);

    if(!rc) {
        mrg_index_write_unlock(mrg);
        MRG_STATS_DELETE_MISS(mrg);
        return false;
    }

    if(!*sections_judy_pptr) {
        rc = JudyHSDel(&mrg->index.uuid_judy, &metric->uuid, sizeof(uuid_t), PJE0);
        if(!rc)
            fatal("DBENGINE METRIC: cannot delete UUID from JudyHS");
        mrg_stats_size_judyhs_removed_uuid(mrg);
    }

    // arrayalloc is running lockless here
    arrayalloc_freez(mrg->index.aral, metric);

    mrg_index_write_unlock(mrg);

    MRG_STATS_DELETED_METRIC(mrg);

    return true;
}

// ----------------------------------------------------------------------------
// public API

MRG *mrg_create(void) {
    MRG *mrg = callocz(1, sizeof(MRG));
    netdata_rwlock_init(&mrg->index.rwlock);
    mrg->index.aral = arrayalloc_create(sizeof(METRIC), 65536 / sizeof(METRIC), NULL, NULL, false, true);
    mrg->stats.size = sizeof(MRG);
    return mrg;
}

void mrg_destroy(MRG *mrg __maybe_unused) {
    // no destruction possible
    // we can't traverse the metrics list

    // to delete entries, the caller needs to keep pointers to them
    // and delete them one by one

    ;
}

METRIC *mrg_metric_add_and_acquire(MRG *mrg, MRG_ENTRY entry, bool *ret) {
    // FIXME - support refcount

//    internal_fatal(entry.latest_time_s > now_realtime_sec(),
//        "DBENGINE METRIC: metric latest time is in the future");

    return metric_add(mrg, &entry, ret);
}

METRIC *mrg_metric_get_and_acquire(MRG *mrg, uuid_t *uuid, Word_t section) {
    // FIXME - support refcount
    return metric_get(mrg, uuid, section);
}

bool mrg_metric_release_and_delete(MRG *mrg, METRIC *metric) {
    // FIXME - support refcount
    return metric_del(mrg, metric);
}

METRIC *mrg_metric_dup(MRG *mrg __maybe_unused, METRIC *metric) {
    // FIXME - duplicate refcount
    return metric;
}

void mrg_metric_release(MRG *mrg __maybe_unused, METRIC *metric __maybe_unused) {
    // FIXME - release refcount

}

Word_t mrg_metric_id(MRG *mrg __maybe_unused, METRIC *metric) {
    return (Word_t)metric;
}

uuid_t *mrg_metric_uuid(MRG *mrg __maybe_unused, METRIC *metric) {
    return &metric->uuid;
}

Word_t mrg_metric_section(MRG *mrg __maybe_unused, METRIC *metric) {
    return metric->section;
}

bool mrg_metric_set_first_time_s(MRG *mrg __maybe_unused, METRIC *metric, time_t first_time_s) {
    netdata_spinlock_lock(&metric->timestamps_lock);
    metric->first_time_s = first_time_s;
    netdata_spinlock_unlock(&metric->timestamps_lock);

    return true;
}

void mrg_metric_expand_retention(MRG *mrg __maybe_unused, METRIC *metric, time_t first_time_s, time_t last_time_s, time_t update_every_s) {

    internal_fatal(first_time_s > now_realtime_sec() + 1,
                   "DBENGINE METRIC: metric first time is in the future");
    internal_fatal(last_time_s > now_realtime_sec() + 1,
                   "DBENGINE METRIC: metric last time is in the future");

    netdata_spinlock_lock(&metric->timestamps_lock);

    if(first_time_s && (!metric->first_time_s || first_time_s < metric->first_time_s))
        metric->first_time_s = first_time_s;

    if(last_time_s && (!metric->latest_time_s_clean || last_time_s > metric->latest_time_s_clean)) {
        metric->latest_time_s_clean = last_time_s;

        if(update_every_s)
            metric->latest_update_every_s = update_every_s;
    }
    else if(!metric->latest_update_every_s && update_every_s)
        metric->latest_update_every_s = update_every_s;

    netdata_spinlock_unlock(&metric->timestamps_lock);
}

bool mrg_metric_set_first_time_s_if_zero(MRG *mrg __maybe_unused, METRIC *metric, time_t first_time_s) {
    bool ret = false;

    netdata_spinlock_lock(&metric->timestamps_lock);
    if(!metric->first_time_s) {
        metric->first_time_s = first_time_s;

//        if(unlikely(metric->latest_time_s_clean < metric->first_time_s))
//            metric->latest_time_s_clean = metric->first_time_s;
//
//        if(unlikely(metric->latest_time_s_hot < metric->first_time_s))
//            metric->latest_time_s_hot = metric->first_time_s;

        ret = true;
    }
    netdata_spinlock_unlock(&metric->timestamps_lock);

    return ret;
}

time_t mrg_metric_get_first_time_s(MRG *mrg __maybe_unused, METRIC *metric) {
    time_t first_time_s;
    netdata_spinlock_lock(&metric->timestamps_lock);
    first_time_s = metric->first_time_s;
    if(!first_time_s) {
        if(metric->latest_time_s_clean)
            first_time_s = metric->latest_time_s_clean;

        if(!first_time_s || metric->latest_time_s_hot < metric->latest_time_s_clean)
            first_time_s = metric->latest_time_s_hot;
    }
    netdata_spinlock_unlock(&metric->timestamps_lock);

    return first_time_s;
}

bool mrg_metric_set_clean_latest_time_s(MRG *mrg __maybe_unused, METRIC *metric, time_t latest_time_s) {
    netdata_spinlock_lock(&metric->timestamps_lock);

    internal_fatal(latest_time_s > now_realtime_sec() + 1,
                   "DBENGINE METRIC: metric latest time is in the future");

    internal_fatal(metric->latest_time_s_clean > latest_time_s,
                   "DBENGINE METRIC: metric new clean latest time is older than the previous one");

    metric->latest_time_s_clean = latest_time_s;

    if(unlikely(!metric->first_time_s))
        metric->first_time_s = latest_time_s;

//    if(unlikely(metric->first_time_s > latest_time_s))
//        metric->first_time_s = latest_time_s;

    netdata_spinlock_unlock(&metric->timestamps_lock);
    return true;
}

bool mrg_metric_set_hot_latest_time_s(MRG *mrg __maybe_unused, METRIC *metric, time_t latest_time_s) {
//    internal_fatal(latest_time_s > now_realtime_sec(),
//                   "DBENGINE METRIC: metric latest time is in the future");

    netdata_spinlock_lock(&metric->timestamps_lock);
    metric->latest_time_s_hot = latest_time_s;

    if(unlikely(!metric->first_time_s))
        metric->first_time_s = latest_time_s;

//    if(unlikely(metric->first_time_s > latest_time_s))
//        metric->first_time_s = latest_time_s;

    netdata_spinlock_unlock(&metric->timestamps_lock);
    return true;
}

time_t mrg_metric_get_latest_time_s(MRG *mrg __maybe_unused, METRIC *metric) {
    time_t max;
    netdata_spinlock_lock(&metric->timestamps_lock);
    max = MAX(metric->latest_time_s_clean, metric->latest_time_s_hot);
    netdata_spinlock_unlock(&metric->timestamps_lock);
    return max;
}

bool mrg_metric_set_update_every(MRG *mrg __maybe_unused, METRIC *metric, time_t update_every_s) {
    if(!update_every_s)
        return false;

    netdata_spinlock_lock(&metric->timestamps_lock);
    metric->latest_update_every_s = update_every_s;
    netdata_spinlock_unlock(&metric->timestamps_lock);

    return true;
}

bool mrg_metric_set_update_every_s_if_zero(MRG *mrg __maybe_unused, METRIC *metric, time_t update_every_s) {
    if(!update_every_s)
        return false;

    netdata_spinlock_lock(&metric->timestamps_lock);
    if(!metric->latest_update_every_s)
        metric->latest_update_every_s = update_every_s;
    netdata_spinlock_unlock(&metric->timestamps_lock);

    return true;
}

time_t mrg_metric_get_update_every_s(MRG *mrg __maybe_unused, METRIC *metric) {
    time_t update_every_s;

    netdata_spinlock_lock(&metric->timestamps_lock);
    update_every_s = metric->latest_update_every_s;
    netdata_spinlock_unlock(&metric->timestamps_lock);

    return update_every_s;
}

struct mrg_statistics mrg_get_statistics(MRG *mrg) {
    // FIXME - use atomics
    return mrg->stats;
}

// ----------------------------------------------------------------------------
// unit test

#ifdef MRG_STRESS_TEST

static void mrg_stress(MRG *mrg, size_t entries, size_t sections) {
    bool ret;

    info("DBENGINE METRIC: stress testing %zu entries on %zu sections...", entries, sections);

    METRIC *array[entries][sections];
    for(size_t i = 0; i < entries ; i++) {
        MRG_ENTRY e = {
                .first_time_s = (time_t)(i + 1),
                .latest_time_s = (time_t)(i + 2),
                .latest_update_every_s = (time_t)(i + 3),
        };
        uuid_generate_random(e.uuid);

        for(size_t section = 0; section < sections ;section++) {
            e.section = section;
            array[i][section] = mrg_metric_add_and_acquire(mrg, e, &ret);
            if(!ret)
                fatal("DBENGINE METRIC: failed to add metric %zu, section %zu", i, section);

            if(mrg_metric_add_and_acquire(mrg, e, &ret) != array[i][section])
                fatal("DBENGINE METRIC: adding the same metric twice, returns a different metric");

            if(ret)
                fatal("DBENGINE METRIC: adding the same metric twice, returns success");

            if(mrg_metric_get_and_acquire(mrg, &e.uuid, e.section) != array[i][section])
                fatal("DBENGINE METRIC: cannot get back the same metric");

            if(uuid_compare(*mrg_metric_uuid(mrg, array[i][section]), e.uuid) != 0)
                fatal("DBENGINE METRIC: uuids do not match");
        }
    }

    for(size_t i = 0; i < entries ; i++) {
        for (size_t section = 0; section < sections; section++) {
            uuid_t uuid;
            uuid_generate_random(uuid);

            if(mrg_metric_get_and_acquire(mrg, &uuid, section))
                fatal("DBENGINE METRIC: found non-existing uuid");

            if(mrg_metric_id(mrg, array[i][section]) != (Word_t)array[i][section])
                fatal("DBENGINE METRIC: metric id does not match");

            if(mrg_metric_get_first_time_s(mrg, array[i][section]) != (time_t)(i + 1))
                fatal("DBENGINE METRIC: wrong first time returned");
            if(mrg_metric_get_latest_time_s(mrg, array[i][section]) != (time_t)(i + 2))
                fatal("DBENGINE METRIC: wrong latest time returned");
            if(mrg_metric_get_update_every_s(mrg, array[i][section]) != (time_t)(i + 3))
                fatal("DBENGINE METRIC: wrong latest time returned");

            if(!mrg_metric_set_first_time_s(mrg, array[i][section], (time_t)((i + 1) * 2)))
                fatal("DBENGINE METRIC: cannot set first time");
            if(!mrg_metric_set_clean_latest_time_s(mrg, array[i][section], (time_t) ((i + 1) * 3)))
                fatal("DBENGINE METRIC: cannot set latest time");
            if(!mrg_metric_set_update_every(mrg, array[i][section], (time_t)((i + 1) * 4)))
                fatal("DBENGINE METRIC: cannot set update every");

            if(mrg_metric_get_first_time_s(mrg, array[i][section]) != (time_t)((i + 1) * 2))
                fatal("DBENGINE METRIC: wrong first time returned");
            if(mrg_metric_get_latest_time_s(mrg, array[i][section]) != (time_t)((i + 1) * 3))
                fatal("DBENGINE METRIC: wrong latest time returned");
            if(mrg_metric_get_update_every_s(mrg, array[i][section]) != (time_t)((i + 1) * 4))
                fatal("DBENGINE METRIC: wrong latest time returned");
        }
    }

    for(size_t i = 0; i < entries ; i++) {
        for (size_t section = 0; section < sections; section++) {
            if(!mrg_metric_release_and_delete(mrg, array[i][section]))
                fatal("DBENGINE METRIC: failed to delete metric");
        }
    }
}

static void *mrg_stress_test_thread1(void *ptr) {
    MRG *mrg = ptr;

    for(int i = 0; i < 5 ; i++)
        mrg_stress(mrg, 10000, 5);

    return ptr;
}

static void *mrg_stress_test_thread2(void *ptr) {
    MRG *mrg = ptr;

    for(int i = 0; i < 10 ; i++)
        mrg_stress(mrg, 500, 50);

    return ptr;
}

static void *mrg_stress_test_thread3(void *ptr) {
    MRG *mrg = ptr;

    for(int i = 0; i < 50 ; i++)
        mrg_stress(mrg, 5000, 1);

    return ptr;
}
#endif

int mrg_unittest(void) {
    MRG *mrg = mrg_create();
    METRIC *metric1, *metric2;
    bool ret;

    MRG_ENTRY entry = {
            .section = 1,
            .first_time_s = 2,
            .last_time_s = 3,
            .latest_update_every_s = 4,
    };
    uuid_generate(entry.uuid);
    metric1 = mrg_metric_add_and_acquire(mrg, entry, &ret);
    if(!ret)
        fatal("DBENGINE METRIC: failed to add metric");

    // add the same metric again
    if(mrg_metric_add_and_acquire(mrg, entry, &ret) != metric1)
        fatal("DBENGINE METRIC: adding the same metric twice, does not return the same pointer");
    if(ret)
        fatal("DBENGINE METRIC: managed to add the same metric twice");

    if(mrg_metric_get_and_acquire(mrg, &entry.uuid, entry.section) != metric1)
        fatal("DBENGINE METRIC: cannot find the metric added");

    // add the same metric again
    if(mrg_metric_add_and_acquire(mrg, entry, &ret) != metric1)
        fatal("DBENGINE METRIC: adding the same metric twice, does not return the same pointer");
    if(ret)
        fatal("DBENGINE METRIC: managed to add the same metric twice");

    // add the same metric in another section
    entry.section = 0;
    metric2 = mrg_metric_add_and_acquire(mrg, entry, &ret);
    if(!ret)
        fatal("DBENGINE METRIC: failed to add metric in different section");

    // add the same metric again
    if(mrg_metric_add_and_acquire(mrg, entry, &ret) != metric2)
        fatal("DBENGINE METRIC: adding the same metric twice (section 0), does not return the same pointer");
    if(ret)
        fatal("DBENGINE METRIC: managed to add the same metric twice in (section 0)");

    if(mrg_metric_get_and_acquire(mrg, &entry.uuid, entry.section) != metric2)
        fatal("DBENGINE METRIC: cannot find the metric added (section 0)");

    // delete the first metric
    if(!mrg_metric_release_and_delete(mrg, metric1))
        fatal("DBENGINE METRIC: cannot delete the first metric");

    if(mrg_metric_get_and_acquire(mrg, &entry.uuid, entry.section) != metric2)
        fatal("DBENGINE METRIC: cannot find the metric added (section 0), after deleting the first one");

    // delete the first metric again - metric1 pointer is invalid now
    if(mrg_metric_release_and_delete(mrg, metric1))
        fatal("DBENGINE METRIC: deleted again an already deleted metric");

    // find the section 0 metric again
    if(mrg_metric_get_and_acquire(mrg, &entry.uuid, entry.section) != metric2)
        fatal("DBENGINE METRIC: cannot find the metric added (section 0), after deleting the first one twice");

    // delete the second metric
    if(!mrg_metric_release_and_delete(mrg, metric2))
        fatal("DBENGINE METRIC: cannot delete the second metric");

    // delete the second metric again
    if(mrg_metric_release_and_delete(mrg, metric2))
        fatal("DBENGINE METRIC: managed to delete an already deleted metric");

    if(mrg->stats.entries != 0)
        fatal("DBENGINE METRIC: invalid entries counter");

#ifdef MRG_STRESS_TEST
    usec_t started_ut = now_realtime_usec();
    pthread_t thread1;
    netdata_thread_create(&thread1, "TH1",
                          NETDATA_THREAD_OPTION_JOINABLE | NETDATA_THREAD_OPTION_DONT_LOG,
                          mrg_stress_test_thread1, mrg);

    pthread_t thread2;
    netdata_thread_create(&thread2, "TH2",
                          NETDATA_THREAD_OPTION_JOINABLE | NETDATA_THREAD_OPTION_DONT_LOG,
                          mrg_stress_test_thread2, mrg);

    pthread_t thread3;
    netdata_thread_create(&thread3, "TH3",
                          NETDATA_THREAD_OPTION_JOINABLE | NETDATA_THREAD_OPTION_DONT_LOG,
                          mrg_stress_test_thread3, mrg);


    sleep_usec(5 * USEC_PER_SEC);

    netdata_thread_cancel(thread1);
    netdata_thread_cancel(thread2);
    netdata_thread_cancel(thread3);

    netdata_thread_join(thread1, NULL);
    netdata_thread_join(thread2, NULL);
    netdata_thread_join(thread3, NULL);
    usec_t ended_ut = now_realtime_usec();

    info("DBENGINE METRIC: did %zu additions, %zu duplicate additions, "
         "%zu deletions, %zu wrong deletions, "
         "%zu successful searches, %zu wrong searches, "
         "%zu successful pointer validations, %zu wrong pointer validations "
         "in %llu usecs",
        mrg->stats.additions, mrg->stats.additions_duplicate,
        mrg->stats.deletions, mrg->stats.delete_misses,
        mrg->stats.search_hits, mrg->stats.search_misses,
        mrg->stats.pointer_validation_hits, mrg->stats.pointer_validation_misses,
        ended_ut - started_ut);

#endif

    mrg_destroy(mrg);

    info("DBENGINE METRIC: all tests passed!");

    return 0;
}
