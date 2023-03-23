#include "metric.h"

typedef int32_t REFCOUNT;
#define REFCOUNT_DELETING (-100)

typedef enum __attribute__ ((__packed__)) {
    METRIC_FLAG_HAS_RETENTION = (1 << 0),
} METRIC_FLAGS;

struct metric {
    uuid_t uuid;                    // never changes
    Word_t section;                 // never changes

    time_t first_time_s;            //
    time_t latest_time_s_clean;     // archived pages latest time
    time_t latest_time_s_hot;       // latest time of the currently collected page
    uint32_t latest_update_every_s; //
    pid_t writer;
    METRIC_FLAGS flags;
    REFCOUNT refcount;
    SPINLOCK spinlock;              // protects all variable members

    // THIS IS allocated with malloc()
    // YOU HAVE TO INITIALIZE IT YOURSELF !
};

static struct aral_statistics mrg_aral_statistics;

struct mrg {
    ARAL *aral[MRG_PARTITIONS];

    struct pgc_index {
        netdata_rwlock_t rwlock;
        Pvoid_t uuid_judy;          // each UUID has a JudyL of sections (tiers)
    } index[MRG_PARTITIONS];

    struct mrg_statistics stats;

    size_t entries_per_partition[MRG_PARTITIONS];
};

static inline void MRG_STATS_DUPLICATE_ADD(MRG *mrg) {
    __atomic_add_fetch(&mrg->stats.additions_duplicate, 1, __ATOMIC_RELAXED);
}

static inline void MRG_STATS_ADDED_METRIC(MRG *mrg, size_t partition) {
    __atomic_add_fetch(&mrg->stats.entries, 1, __ATOMIC_RELAXED);
    __atomic_add_fetch(&mrg->stats.additions, 1, __ATOMIC_RELAXED);
    __atomic_add_fetch(&mrg->stats.size, sizeof(METRIC), __ATOMIC_RELAXED);

    __atomic_add_fetch(&mrg->entries_per_partition[partition], 1, __ATOMIC_RELAXED);
}

static inline void MRG_STATS_DELETED_METRIC(MRG *mrg, size_t partition) {
    __atomic_sub_fetch(&mrg->stats.entries, 1, __ATOMIC_RELAXED);
    __atomic_sub_fetch(&mrg->stats.size, sizeof(METRIC), __ATOMIC_RELAXED);
    __atomic_add_fetch(&mrg->stats.deletions, 1, __ATOMIC_RELAXED);

    __atomic_sub_fetch(&mrg->entries_per_partition[partition], 1, __ATOMIC_RELAXED);
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

static inline void mrg_index_read_lock(MRG *mrg, size_t partition) {
    netdata_rwlock_rdlock(&mrg->index[partition].rwlock);
}
static inline void mrg_index_read_unlock(MRG *mrg, size_t partition) {
    netdata_rwlock_unlock(&mrg->index[partition].rwlock);
}
static inline void mrg_index_write_lock(MRG *mrg, size_t partition) {
    netdata_rwlock_wrlock(&mrg->index[partition].rwlock);
}
static inline void mrg_index_write_unlock(MRG *mrg, size_t partition) {
    netdata_rwlock_unlock(&mrg->index[partition].rwlock);
}

static inline void mrg_stats_size_judyl_change(MRG *mrg, size_t mem_before_judyl, size_t mem_after_judyl) {
    if(mem_after_judyl > mem_before_judyl)
        __atomic_add_fetch(&mrg->stats.size, mem_after_judyl - mem_before_judyl, __ATOMIC_RELAXED);
    else if(mem_after_judyl < mem_before_judyl)
        __atomic_sub_fetch(&mrg->stats.size, mem_before_judyl - mem_after_judyl, __ATOMIC_RELAXED);
}

static inline void mrg_stats_size_judyhs_added_uuid(MRG *mrg) {
    __atomic_add_fetch(&mrg->stats.size, JUDYHS_INDEX_SIZE_ESTIMATE(sizeof(uuid_t)), __ATOMIC_RELAXED);
}

static inline void mrg_stats_size_judyhs_removed_uuid(MRG *mrg) {
    __atomic_sub_fetch(&mrg->stats.size, JUDYHS_INDEX_SIZE_ESTIMATE(sizeof(uuid_t)), __ATOMIC_RELAXED);
}

static inline size_t uuid_partition(MRG *mrg __maybe_unused, uuid_t *uuid) {
    uint8_t *u = (uint8_t *)uuid;
    return u[UUID_SZ - 1] % MRG_PARTITIONS;
}

static inline bool metric_has_retention_unsafe(MRG *mrg __maybe_unused, METRIC *metric) {
    bool has_retention = (metric->first_time_s > 0 || metric->latest_time_s_clean > 0 || metric->latest_time_s_hot > 0);

    if(has_retention && !(metric->flags & METRIC_FLAG_HAS_RETENTION)) {
        metric->flags |= METRIC_FLAG_HAS_RETENTION;
        __atomic_add_fetch(&mrg->stats.entries_with_retention, 1, __ATOMIC_RELAXED);
    }
    else if(!has_retention && (metric->flags & METRIC_FLAG_HAS_RETENTION)) {
        metric->flags &= ~METRIC_FLAG_HAS_RETENTION;
        __atomic_sub_fetch(&mrg->stats.entries_with_retention, 1, __ATOMIC_RELAXED);
    }

    return has_retention;
}

static inline REFCOUNT metric_acquire(MRG *mrg __maybe_unused, METRIC *metric, bool having_spinlock) {
    REFCOUNT refcount;

    if(!having_spinlock)
        netdata_spinlock_lock(&metric->spinlock);

    if(unlikely(metric->refcount < 0))
        fatal("METRIC: refcount is %d (negative) during acquire", metric->refcount);

    refcount = ++metric->refcount;

    // update its retention flags
    metric_has_retention_unsafe(mrg, metric);

    if(!having_spinlock)
        netdata_spinlock_unlock(&metric->spinlock);

    if(refcount == 1)
        __atomic_add_fetch(&mrg->stats.entries_referenced, 1, __ATOMIC_RELAXED);

    __atomic_add_fetch(&mrg->stats.current_references, 1, __ATOMIC_RELAXED);

    return refcount;
}

static inline bool metric_release_and_can_be_deleted(MRG *mrg __maybe_unused, METRIC *metric) {
    bool ret = true;
    REFCOUNT refcount;

    netdata_spinlock_lock(&metric->spinlock);

    if(unlikely(metric->refcount <= 0))
        fatal("METRIC: refcount is %d (zero or negative) during release", metric->refcount);

    refcount = --metric->refcount;

    if(likely(metric_has_retention_unsafe(mrg, metric) || refcount != 0))
        ret = false;

    netdata_spinlock_unlock(&metric->spinlock);

    if(unlikely(!refcount))
        __atomic_sub_fetch(&mrg->stats.entries_referenced, 1, __ATOMIC_RELAXED);

    __atomic_sub_fetch(&mrg->stats.current_references, 1, __ATOMIC_RELAXED);

    return ret;
}

static METRIC *metric_add_and_acquire(MRG *mrg, MRG_ENTRY *entry, bool *ret) {
    size_t partition = uuid_partition(mrg, &entry->uuid);

    METRIC *allocation = aral_mallocz(mrg->aral[partition]);

    mrg_index_write_lock(mrg, partition);

    size_t mem_before_judyl, mem_after_judyl;

    Pvoid_t *sections_judy_pptr = JudyHSIns(&mrg->index[partition].uuid_judy, &entry->uuid, sizeof(uuid_t), PJE0);
    if(unlikely(!sections_judy_pptr || sections_judy_pptr == PJERR))
        fatal("DBENGINE METRIC: corrupted UUIDs JudyHS array");

    if(unlikely(!*sections_judy_pptr))
        mrg_stats_size_judyhs_added_uuid(mrg);

    mem_before_judyl = JudyLMemUsed(*sections_judy_pptr);
    Pvoid_t *PValue = JudyLIns(sections_judy_pptr, entry->section, PJE0);
    mem_after_judyl = JudyLMemUsed(*sections_judy_pptr);
    mrg_stats_size_judyl_change(mrg, mem_before_judyl, mem_after_judyl);

    if(unlikely(!PValue || PValue == PJERR))
        fatal("DBENGINE METRIC: corrupted section JudyL array");

    if(unlikely(*PValue != NULL)) {
        METRIC *metric = *PValue;

        metric_acquire(mrg, metric, false);
        mrg_index_write_unlock(mrg, partition);

        if(ret)
            *ret = false;

        aral_freez(mrg->aral[partition], allocation);

        MRG_STATS_DUPLICATE_ADD(mrg);
        return metric;
    }

    METRIC *metric = allocation;
    uuid_copy(metric->uuid, entry->uuid);
    metric->section = entry->section;
    metric->first_time_s = MAX(0, entry->first_time_s);
    metric->latest_time_s_clean = MAX(0, entry->last_time_s);
    metric->latest_time_s_hot = 0;
    metric->latest_update_every_s = entry->latest_update_every_s;
    metric->writer = 0;
    metric->refcount = 0;
    metric->flags = 0;
    netdata_spinlock_init(&metric->spinlock);
    metric_acquire(mrg, metric, true); // no spinlock use required here
    *PValue = metric;

    mrg_index_write_unlock(mrg, partition);

    if(ret)
        *ret = true;

    MRG_STATS_ADDED_METRIC(mrg, partition);

    return metric;
}

static METRIC *metric_get_and_acquire(MRG *mrg, uuid_t *uuid, Word_t section) {
    size_t partition = uuid_partition(mrg, uuid);

    mrg_index_read_lock(mrg, partition);

    Pvoid_t *sections_judy_pptr = JudyHSGet(mrg->index[partition].uuid_judy, uuid, sizeof(uuid_t));
    if(unlikely(!sections_judy_pptr)) {
        mrg_index_read_unlock(mrg, partition);
        MRG_STATS_SEARCH_MISS(mrg);
        return NULL;
    }

    Pvoid_t *PValue = JudyLGet(*sections_judy_pptr, section, PJE0);
    if(unlikely(!PValue)) {
        mrg_index_read_unlock(mrg, partition);
        MRG_STATS_SEARCH_MISS(mrg);
        return NULL;
    }

    METRIC *metric = *PValue;

    metric_acquire(mrg, metric, false);

    mrg_index_read_unlock(mrg, partition);

    MRG_STATS_SEARCH_HIT(mrg);
    return metric;
}

static bool acquired_metric_del(MRG *mrg, METRIC *metric) {
    size_t partition = uuid_partition(mrg, &metric->uuid);

    size_t mem_before_judyl, mem_after_judyl;

    mrg_index_write_lock(mrg, partition);

    if(!metric_release_and_can_be_deleted(mrg, metric)) {
        mrg_index_write_unlock(mrg, partition);
        __atomic_add_fetch(&mrg->stats.delete_having_retention_or_referenced, 1, __ATOMIC_RELAXED);
        return false;
    }

    Pvoid_t *sections_judy_pptr = JudyHSGet(mrg->index[partition].uuid_judy, &metric->uuid, sizeof(uuid_t));
    if(unlikely(!sections_judy_pptr || !*sections_judy_pptr)) {
        mrg_index_write_unlock(mrg, partition);
        MRG_STATS_DELETE_MISS(mrg);
        return false;
    }

    mem_before_judyl = JudyLMemUsed(*sections_judy_pptr);
    int rc = JudyLDel(sections_judy_pptr, metric->section, PJE0);
    mem_after_judyl = JudyLMemUsed(*sections_judy_pptr);
    mrg_stats_size_judyl_change(mrg, mem_before_judyl, mem_after_judyl);

    if(unlikely(!rc)) {
        mrg_index_write_unlock(mrg, partition);
        MRG_STATS_DELETE_MISS(mrg);
        return false;
    }

    if(!*sections_judy_pptr) {
        rc = JudyHSDel(&mrg->index[partition].uuid_judy, &metric->uuid, sizeof(uuid_t), PJE0);
        if(unlikely(!rc))
            fatal("DBENGINE METRIC: cannot delete UUID from JudyHS");
        mrg_stats_size_judyhs_removed_uuid(mrg);
    }

    mrg_index_write_unlock(mrg, partition);

    aral_freez(mrg->aral[partition], metric);

    MRG_STATS_DELETED_METRIC(mrg, partition);

    return true;
}

// ----------------------------------------------------------------------------
// public API

MRG *mrg_create(void) {
    MRG *mrg = callocz(1, sizeof(MRG));

    for(size_t i = 0; i < MRG_PARTITIONS ; i++) {
        netdata_rwlock_init(&mrg->index[i].rwlock);

        char buf[ARAL_MAX_NAME + 1];
        snprintfz(buf, ARAL_MAX_NAME, "mrg[%zu]", i);

        mrg->aral[i] = aral_create(buf,
                                   sizeof(METRIC),
                                   0,
                                   16384,
                                   &mrg_aral_statistics,
                                   NULL, NULL, false,
                                   false);
    }

    mrg->stats.size = sizeof(MRG);

    return mrg;
}

size_t mrg_aral_structures(void) {
    return aral_structures_from_stats(&mrg_aral_statistics);
}

size_t mrg_aral_overhead(void) {
    return aral_overhead_from_stats(&mrg_aral_statistics);
}

void mrg_destroy(MRG *mrg __maybe_unused) {
    // no destruction possible
    // we can't traverse the metrics list

    // to delete entries, the caller needs to keep pointers to them
    // and delete them one by one

    ;
}

METRIC *mrg_metric_add_and_acquire(MRG *mrg, MRG_ENTRY entry, bool *ret) {
//    internal_fatal(entry.latest_time_s > max_acceptable_collected_time(),
//        "DBENGINE METRIC: metric latest time is in the future");

    return metric_add_and_acquire(mrg, &entry, ret);
}

METRIC *mrg_metric_get_and_acquire(MRG *mrg, uuid_t *uuid, Word_t section) {
    return metric_get_and_acquire(mrg, uuid, section);
}

bool mrg_metric_release_and_delete(MRG *mrg, METRIC *metric) {
    return acquired_metric_del(mrg, metric);
}

METRIC *mrg_metric_dup(MRG *mrg, METRIC *metric) {
    metric_acquire(mrg, metric, false);
    return metric;
}

bool mrg_metric_release(MRG *mrg, METRIC *metric) {
    return metric_release_and_can_be_deleted(mrg, metric);
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
    internal_fatal(first_time_s < 0, "DBENGINE METRIC: timestamp is negative");

    if(unlikely(first_time_s < 0))
        return false;

    netdata_spinlock_lock(&metric->spinlock);
    metric->first_time_s = first_time_s;
    metric_has_retention_unsafe(mrg, metric);
    netdata_spinlock_unlock(&metric->spinlock);

    return true;
}

void mrg_metric_expand_retention(MRG *mrg __maybe_unused, METRIC *metric, time_t first_time_s, time_t last_time_s, time_t update_every_s) {
    internal_fatal(first_time_s < 0 || last_time_s < 0 || update_every_s < 0,
                   "DBENGINE METRIC: timestamp is negative");
    internal_fatal(first_time_s > max_acceptable_collected_time(),
                   "DBENGINE METRIC: metric first time is in the future");
    internal_fatal(last_time_s > max_acceptable_collected_time(),
                   "DBENGINE METRIC: metric last time is in the future");

    if(unlikely(first_time_s < 0))
        first_time_s = 0;

    if(unlikely(last_time_s < 0))
        last_time_s = 0;

    if(unlikely(update_every_s < 0))
        update_every_s = 0;

    if(unlikely(!first_time_s && !last_time_s && !update_every_s))
        return;

    netdata_spinlock_lock(&metric->spinlock);

    if(unlikely(first_time_s && (!metric->first_time_s || first_time_s < metric->first_time_s)))
        metric->first_time_s = first_time_s;

    if(likely(last_time_s && (!metric->latest_time_s_clean || last_time_s > metric->latest_time_s_clean))) {
        metric->latest_time_s_clean = last_time_s;

        if(likely(update_every_s))
            metric->latest_update_every_s = (uint32_t) update_every_s;
    }
    else if(unlikely(!metric->latest_update_every_s && update_every_s))
        metric->latest_update_every_s = (uint32_t) update_every_s;

    metric_has_retention_unsafe(mrg, metric);
    netdata_spinlock_unlock(&metric->spinlock);
}

bool mrg_metric_set_first_time_s_if_bigger(MRG *mrg __maybe_unused, METRIC *metric, time_t first_time_s) {
    internal_fatal(first_time_s < 0, "DBENGINE METRIC: timestamp is negative");

    bool ret = false;

    netdata_spinlock_lock(&metric->spinlock);
    if(first_time_s > metric->first_time_s) {
        metric->first_time_s = first_time_s;
        ret = true;
    }
    metric_has_retention_unsafe(mrg, metric);
    netdata_spinlock_unlock(&metric->spinlock);

    return ret;
}

time_t mrg_metric_get_first_time_s(MRG *mrg __maybe_unused, METRIC *metric) {
    time_t first_time_s;

    netdata_spinlock_lock(&metric->spinlock);

    if(unlikely(!metric->first_time_s)) {
        if(metric->latest_time_s_clean)
            metric->first_time_s = metric->latest_time_s_clean;

        else if(metric->latest_time_s_hot)
            metric->first_time_s = metric->latest_time_s_hot;
    }

    first_time_s = metric->first_time_s;

    netdata_spinlock_unlock(&metric->spinlock);

    return first_time_s;
}

void mrg_metric_get_retention(MRG *mrg __maybe_unused, METRIC *metric, time_t *first_time_s, time_t *last_time_s, time_t *update_every_s) {
    netdata_spinlock_lock(&metric->spinlock);

    if(unlikely(!metric->first_time_s)) {
        if(metric->latest_time_s_clean)
            metric->first_time_s = metric->latest_time_s_clean;

        else if(metric->latest_time_s_hot)
            metric->first_time_s = metric->latest_time_s_hot;
    }

    *first_time_s = metric->first_time_s;
    *last_time_s = MAX(metric->latest_time_s_clean, metric->latest_time_s_hot);
    *update_every_s = metric->latest_update_every_s;

    netdata_spinlock_unlock(&metric->spinlock);
}

bool mrg_metric_set_clean_latest_time_s(MRG *mrg __maybe_unused, METRIC *metric, time_t latest_time_s) {
    internal_fatal(latest_time_s < 0, "DBENGINE METRIC: timestamp is negative");

    if(unlikely(latest_time_s < 0))
        return false;

    netdata_spinlock_lock(&metric->spinlock);

//    internal_fatal(latest_time_s > max_acceptable_collected_time(),
//                   "DBENGINE METRIC: metric latest time is in the future");

//    internal_fatal(metric->latest_time_s_clean > latest_time_s,
//                   "DBENGINE METRIC: metric new clean latest time is older than the previous one");

    metric->latest_time_s_clean = latest_time_s;

    if(unlikely(!metric->first_time_s))
        metric->first_time_s = latest_time_s;

    metric_has_retention_unsafe(mrg, metric);
    netdata_spinlock_unlock(&metric->spinlock);
    return true;
}

// returns true when metric still has retention
bool mrg_metric_zero_disk_retention(MRG *mrg __maybe_unused, METRIC *metric) {
    Word_t section = mrg_metric_section(mrg, metric);
    bool do_again = false;
    size_t countdown = 5;
    bool ret = true;

    do {
        time_t min_first_time_s = LONG_MAX;
        time_t max_end_time_s = 0;
        PGC_PAGE *page;
        PGC_SEARCH method = PGC_SEARCH_FIRST;
        time_t page_first_time_s = 0;
        time_t page_end_time_s = 0;
        while ((page = pgc_page_get_and_acquire(main_cache, section, (Word_t)metric, page_first_time_s, method))) {
            method = PGC_SEARCH_NEXT;

            bool is_hot = pgc_is_page_hot(page);
            bool is_dirty = pgc_is_page_dirty(page);
            page_first_time_s = pgc_page_start_time_s(page);
            page_end_time_s = pgc_page_end_time_s(page);

            if ((is_hot || is_dirty) && page_first_time_s > 0 && page_first_time_s < min_first_time_s)
                min_first_time_s = page_first_time_s;

            if (is_dirty && page_end_time_s > max_end_time_s)
                max_end_time_s = page_end_time_s;

            pgc_page_release(main_cache, page);
        }

        if (min_first_time_s == LONG_MAX)
            min_first_time_s = 0;

        netdata_spinlock_lock(&metric->spinlock);
        if (--countdown && !min_first_time_s && metric->latest_time_s_hot)
            do_again = true;
        else {
            internal_error(!countdown, "METRIC: giving up on updating the retention of metric without disk retention");

            do_again = false;
            metric->first_time_s = min_first_time_s;
            metric->latest_time_s_clean = max_end_time_s;

            ret = metric_has_retention_unsafe(mrg, metric);
        }
        netdata_spinlock_unlock(&metric->spinlock);
    } while(do_again);

    return ret;
}

bool mrg_metric_set_hot_latest_time_s(MRG *mrg __maybe_unused, METRIC *metric, time_t latest_time_s) {
    internal_fatal(latest_time_s < 0, "DBENGINE METRIC: timestamp is negative");

//    internal_fatal(latest_time_s > max_acceptable_collected_time(),
//                   "DBENGINE METRIC: metric latest time is in the future");

    if(unlikely(latest_time_s < 0))
        return false;

    netdata_spinlock_lock(&metric->spinlock);
    metric->latest_time_s_hot = latest_time_s;

    if(unlikely(!metric->first_time_s))
        metric->first_time_s = latest_time_s;

    metric_has_retention_unsafe(mrg, metric);
    netdata_spinlock_unlock(&metric->spinlock);
    return true;
}

time_t mrg_metric_get_latest_time_s(MRG *mrg __maybe_unused, METRIC *metric) {
    time_t max;
    netdata_spinlock_lock(&metric->spinlock);
    max = MAX(metric->latest_time_s_clean, metric->latest_time_s_hot);
    netdata_spinlock_unlock(&metric->spinlock);
    return max;
}

bool mrg_metric_set_update_every(MRG *mrg __maybe_unused, METRIC *metric, time_t update_every_s) {
    internal_fatal(update_every_s < 0, "DBENGINE METRIC: timestamp is negative");

    if(update_every_s <= 0)
        return false;

    netdata_spinlock_lock(&metric->spinlock);
    metric->latest_update_every_s = (uint32_t) update_every_s;
    netdata_spinlock_unlock(&metric->spinlock);

    return true;
}

bool mrg_metric_set_update_every_s_if_zero(MRG *mrg __maybe_unused, METRIC *metric, time_t update_every_s) {
    internal_fatal(update_every_s < 0, "DBENGINE METRIC: timestamp is negative");

    if(update_every_s <= 0)
        return false;

    netdata_spinlock_lock(&metric->spinlock);
    if(!metric->latest_update_every_s)
        metric->latest_update_every_s = (uint32_t) update_every_s;
    netdata_spinlock_unlock(&metric->spinlock);

    return true;
}

time_t mrg_metric_get_update_every_s(MRG *mrg __maybe_unused, METRIC *metric) {
    time_t update_every_s;

    netdata_spinlock_lock(&metric->spinlock);
    update_every_s = metric->latest_update_every_s;
    netdata_spinlock_unlock(&metric->spinlock);

    return update_every_s;
}

bool mrg_metric_set_writer(MRG *mrg, METRIC *metric) {
    bool done = false;
    netdata_spinlock_lock(&metric->spinlock);
    if(!metric->writer) {
        metric->writer = gettid();
        __atomic_add_fetch(&mrg->stats.writers, 1, __ATOMIC_RELAXED);
        done = true;
    }
    else
        __atomic_add_fetch(&mrg->stats.writers_conflicts, 1, __ATOMIC_RELAXED);
    netdata_spinlock_unlock(&metric->spinlock);
    return done;
}

bool mrg_metric_clear_writer(MRG *mrg, METRIC *metric) {
    bool done = false;
    netdata_spinlock_lock(&metric->spinlock);
    if(metric->writer) {
        metric->writer = 0;
        __atomic_sub_fetch(&mrg->stats.writers, 1, __ATOMIC_RELAXED);
        done = true;
    }
    netdata_spinlock_unlock(&metric->spinlock);
    return done;
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
    METRIC *m1_t0, *m2_t0, *m3_t0, *m4_t0;
    METRIC *m1_t1, *m2_t1, *m3_t1, *m4_t1;
    bool ret;

    MRG_ENTRY entry = {
            .section = 0,
            .first_time_s = 2,
            .last_time_s = 3,
            .latest_update_every_s = 4,
    };
    uuid_generate(entry.uuid);
    m1_t0 = mrg_metric_add_and_acquire(mrg, entry, &ret);
    if(!ret)
        fatal("DBENGINE METRIC: failed to add metric");

    // add the same metric again
    m2_t0 = mrg_metric_add_and_acquire(mrg, entry, &ret);
    if(m2_t0 != m1_t0)
        fatal("DBENGINE METRIC: adding the same metric twice, does not return the same pointer");
    if(ret)
        fatal("DBENGINE METRIC: managed to add the same metric twice");

    m3_t0 = mrg_metric_get_and_acquire(mrg, &entry.uuid, entry.section);
    if(m3_t0 != m1_t0)
        fatal("DBENGINE METRIC: cannot find the metric added");

    // add the same metric again
    m4_t0 = mrg_metric_add_and_acquire(mrg, entry, &ret);
    if(m4_t0 != m1_t0)
        fatal("DBENGINE METRIC: adding the same metric twice, does not return the same pointer");
    if(ret)
        fatal("DBENGINE METRIC: managed to add the same metric twice");

    // add the same metric in another section
    entry.section = 1;
    m1_t1 = mrg_metric_add_and_acquire(mrg, entry, &ret);
    if(!ret)
        fatal("DBENGINE METRIC: failed to add metric in section %zu", (size_t)entry.section);

    // add the same metric again
    m2_t1 = mrg_metric_add_and_acquire(mrg, entry, &ret);
    if(m2_t1 != m1_t1)
        fatal("DBENGINE METRIC: adding the same metric twice (section %zu), does not return the same pointer", (size_t)entry.section);
    if(ret)
        fatal("DBENGINE METRIC: managed to add the same metric twice in (section 0)");

    m3_t1 = mrg_metric_get_and_acquire(mrg, &entry.uuid, entry.section);
    if(m3_t1 != m1_t1)
        fatal("DBENGINE METRIC: cannot find the metric added (section %zu)", (size_t)entry.section);

    // delete the first metric
    mrg_metric_release(mrg, m2_t0);
    mrg_metric_release(mrg, m3_t0);
    mrg_metric_release(mrg, m4_t0);
    mrg_metric_set_first_time_s(mrg, m1_t0, 0);
    mrg_metric_set_clean_latest_time_s(mrg, m1_t0, 0);
    mrg_metric_set_hot_latest_time_s(mrg, m1_t0, 0);
    if(!mrg_metric_release_and_delete(mrg, m1_t0))
        fatal("DBENGINE METRIC: cannot delete the first metric");

    m4_t1 = mrg_metric_get_and_acquire(mrg, &entry.uuid, entry.section);
    if(m4_t1 != m1_t1)
        fatal("DBENGINE METRIC: cannot find the metric added (section %zu), after deleting the first one", (size_t)entry.section);

    // delete the second metric
    mrg_metric_release(mrg, m2_t1);
    mrg_metric_release(mrg, m3_t1);
    mrg_metric_release(mrg, m4_t1);
    mrg_metric_set_first_time_s(mrg, m1_t1, 0);
    mrg_metric_set_clean_latest_time_s(mrg, m1_t1, 0);
    mrg_metric_set_hot_latest_time_s(mrg, m1_t1, 0);
    if(!mrg_metric_release_and_delete(mrg, m1_t1))
        fatal("DBENGINE METRIC: cannot delete the second metric");

    if(mrg->stats.entries != 0)
        fatal("DBENGINE METRIC: invalid entries counter");

#ifdef MRG_STRESS_TEST
    usec_t started_ut = now_monotonic_usec();
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
    usec_t ended_ut = now_monotonic_usec();

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
