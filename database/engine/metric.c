// SPDX-License-Identifier: GPL-3.0-or-later
#include "metric.h"
#include "cache.h"
#include "rrddiskprotocol.h"

typedef int32_t REFCOUNT;
#define REFCOUNT_DELETING (-100)

struct metric {
    uuid_t uuid;                    // never changes
    Word_t section;                 // never changes

    time_t first_time_s;            // the timestamp of the oldest point in the database
    time_t latest_time_s_clean;     // the timestamp of the newest point in the database
    time_t latest_time_s_hot;       // the timestamp of the latest point that has been collected (not yet stored)
    uint32_t latest_update_every_s; // the latest data collection frequency
    pid_t writer;
    uint8_t partition;
    REFCOUNT refcount;

    // THIS IS allocated with malloc()
    // YOU HAVE TO INITIALIZE IT YOURSELF !
};

#define set_metric_field_with_condition(field, value, condition) ({ \
    typeof(field) _current = __atomic_load_n(&(field), __ATOMIC_RELAXED);   \
    typeof(field) _wanted = value;                                          \
    bool did_it = true;                                                     \
                                                                            \
    do {                                                                    \
        if((condition) && (_current != _wanted)) {                          \
            ;                                                               \
        }                                                                   \
        else {                                                              \
            did_it = false;                                                 \
            break;                                                          \
        }                                                                   \
    } while(!__atomic_compare_exchange_n(&(field), &_current, _wanted,      \
            false, __ATOMIC_RELAXED, __ATOMIC_RELAXED));                    \
                                                                            \
    did_it;                                                                 \
})

static struct aral_statistics mrg_aral_statistics;

struct mrg {
    size_t partitions;

    struct mrg_partition {
        ARAL *aral;                 // not protected by our spinlock - it has its own

        RW_SPINLOCK rw_spinlock;
        Pvoid_t uuid_judy;          // JudyHS: each UUID has a JudyL of sections (tiers)

        struct mrg_statistics stats;
    } index[];
};

static inline void MRG_STATS_DUPLICATE_ADD(MRG *mrg, size_t partition) {
    mrg->index[partition].stats.additions_duplicate++;
}

static inline void MRG_STATS_ADDED_METRIC(MRG *mrg, size_t partition) {
    mrg->index[partition].stats.entries++;
    mrg->index[partition].stats.additions++;
    mrg->index[partition].stats.size += sizeof(METRIC);
}

static inline void MRG_STATS_DELETED_METRIC(MRG *mrg, size_t partition) {
    mrg->index[partition].stats.entries--;
    mrg->index[partition].stats.size -= sizeof(METRIC);
    mrg->index[partition].stats.deletions++;
}

static inline void MRG_STATS_SEARCH_HIT(MRG *mrg, size_t partition) {
    __atomic_add_fetch(&mrg->index[partition].stats.search_hits, 1, __ATOMIC_RELAXED);
}

static inline void MRG_STATS_SEARCH_MISS(MRG *mrg, size_t partition) {
    __atomic_add_fetch(&mrg->index[partition].stats.search_misses, 1, __ATOMIC_RELAXED);
}

static inline void MRG_STATS_DELETE_MISS(MRG *mrg, size_t partition) {
    mrg->index[partition].stats.delete_misses++;
}

#define mrg_index_read_lock(mrg, partition) rw_spinlock_read_lock(&(mrg)->index[partition].rw_spinlock)
#define mrg_index_read_unlock(mrg, partition) rw_spinlock_read_unlock(&(mrg)->index[partition].rw_spinlock)
#define mrg_index_write_lock(mrg, partition) rw_spinlock_write_lock(&(mrg)->index[partition].rw_spinlock)
#define mrg_index_write_unlock(mrg, partition) rw_spinlock_write_unlock(&(mrg)->index[partition].rw_spinlock)

static inline void mrg_stats_size_judyl_change(MRG *mrg, size_t mem_before_judyl, size_t mem_after_judyl, size_t partition) {
    if(mem_after_judyl > mem_before_judyl)
        __atomic_add_fetch(&mrg->index[partition].stats.size, mem_after_judyl - mem_before_judyl, __ATOMIC_RELAXED);
    else if(mem_after_judyl < mem_before_judyl)
        __atomic_sub_fetch(&mrg->index[partition].stats.size, mem_before_judyl - mem_after_judyl, __ATOMIC_RELAXED);
}

static inline void mrg_stats_size_judyhs_added_uuid(MRG *mrg, size_t partition) {
    __atomic_add_fetch(&mrg->index[partition].stats.size, JUDYHS_INDEX_SIZE_ESTIMATE(sizeof(uuid_t)), __ATOMIC_RELAXED);
}

static inline void mrg_stats_size_judyhs_removed_uuid(MRG *mrg, size_t partition) {
    __atomic_sub_fetch(&mrg->index[partition].stats.size, JUDYHS_INDEX_SIZE_ESTIMATE(sizeof(uuid_t)), __ATOMIC_RELAXED);
}

static inline size_t uuid_partition(MRG *mrg __maybe_unused, uuid_t *uuid) {
    uint8_t *u = (uint8_t *)uuid;

    size_t n;
    memcpy(&n, &u[UUID_SZ - sizeof(size_t)], sizeof(size_t));

    return n % mrg->partitions;
}

static inline time_t mrg_metric_get_first_time_s_smart(MRG *mrg __maybe_unused, METRIC *metric) {
    time_t first_time_s = __atomic_load_n(&metric->first_time_s, __ATOMIC_RELAXED);

    if(first_time_s <= 0) {
        first_time_s = __atomic_load_n(&metric->latest_time_s_clean, __ATOMIC_RELAXED);
        if(first_time_s <= 0)
            first_time_s = __atomic_load_n(&metric->latest_time_s_hot, __ATOMIC_RELAXED);

        if(first_time_s <= 0)
            first_time_s = 0;
        else
            __atomic_store_n(&metric->first_time_s, first_time_s, __ATOMIC_RELAXED);
    }

    return first_time_s;
}

static inline REFCOUNT metric_acquire(MRG *mrg __maybe_unused, METRIC *metric) {
    size_t partition = metric->partition;
    REFCOUNT expected = __atomic_load_n(&metric->refcount, __ATOMIC_RELAXED);
    REFCOUNT refcount;

    do {
        if(expected < 0)
            fatal("METRIC: refcount is %d (negative) during acquire", metric->refcount);

        refcount = expected + 1;
    } while(!__atomic_compare_exchange_n(&metric->refcount, &expected, refcount, false, __ATOMIC_RELAXED, __ATOMIC_RELAXED));

    if(refcount == 1)
        __atomic_add_fetch(&mrg->index[partition].stats.entries_referenced, 1, __ATOMIC_RELAXED);

    __atomic_add_fetch(&mrg->index[partition].stats.current_references, 1, __ATOMIC_RELAXED);

    return refcount;
}

static inline bool metric_release_and_can_be_deleted(MRG *mrg __maybe_unused, METRIC *metric) {
    size_t partition = metric->partition;
    REFCOUNT expected = __atomic_load_n(&metric->refcount, __ATOMIC_RELAXED);
    REFCOUNT refcount;

    do {
        if(expected <= 0)
            fatal("METRIC: refcount is %d (zero or negative) during release", metric->refcount);

        refcount = expected - 1;
    } while(!__atomic_compare_exchange_n(&metric->refcount, &expected, refcount, false, __ATOMIC_RELAXED, __ATOMIC_RELAXED));

    if(unlikely(!refcount))
        __atomic_sub_fetch(&mrg->index[partition].stats.entries_referenced, 1, __ATOMIC_RELAXED);

    __atomic_sub_fetch(&mrg->index[partition].stats.current_references, 1, __ATOMIC_RELAXED);

    time_t first, last;
    mrg_metric_get_retention(mrg, metric, &first, &last, NULL);
    return (!first || !last || first > last);
}

static inline METRIC *metric_add_and_acquire(MRG *mrg, MRG_ENTRY *entry, bool *ret) {
    size_t partition = uuid_partition(mrg, entry->uuid);

    METRIC *allocation = aral_mallocz(mrg->index[partition].aral);

    mrg_index_write_lock(mrg, partition);

    size_t mem_before_judyl, mem_after_judyl;

    Pvoid_t *sections_judy_pptr = JudyHSIns(&mrg->index[partition].uuid_judy, entry->uuid, sizeof(uuid_t), PJE0);
    if(unlikely(!sections_judy_pptr || sections_judy_pptr == PJERR))
        fatal("DBENGINE METRIC: corrupted UUIDs JudyHS array");

    if(unlikely(!*sections_judy_pptr))
        mrg_stats_size_judyhs_added_uuid(mrg, partition);

    mem_before_judyl = JudyLMemUsed(*sections_judy_pptr);
    Pvoid_t *PValue = JudyLIns(sections_judy_pptr, entry->section, PJE0);
    mem_after_judyl = JudyLMemUsed(*sections_judy_pptr);
    mrg_stats_size_judyl_change(mrg, mem_before_judyl, mem_after_judyl, partition);

    if(unlikely(!PValue || PValue == PJERR))
        fatal("DBENGINE METRIC: corrupted section JudyL array");

    if(unlikely(*PValue != NULL)) {
        METRIC *metric = *PValue;

        metric_acquire(mrg, metric);

        MRG_STATS_DUPLICATE_ADD(mrg, partition);

        mrg_index_write_unlock(mrg, partition);

        if(ret)
            *ret = false;

        aral_freez(mrg->index[partition].aral, allocation);

        return metric;
    }

    METRIC *metric = allocation;
    uuid_copy(metric->uuid, *entry->uuid);
    metric->section = entry->section;
    metric->first_time_s = MAX(0, entry->first_time_s);
    metric->latest_time_s_clean = MAX(0, entry->last_time_s);
    metric->latest_time_s_hot = 0;
    metric->latest_update_every_s = entry->latest_update_every_s;
    metric->writer = 0;
    metric->refcount = 0;
    metric->partition = partition;
    metric_acquire(mrg, metric);
    *PValue = metric;

    MRG_STATS_ADDED_METRIC(mrg, partition);

    mrg_index_write_unlock(mrg, partition);

    if(ret)
        *ret = true;

    return metric;
}

static inline METRIC *metric_get_and_acquire(MRG *mrg, uuid_t *uuid, Word_t section) {
    size_t partition = uuid_partition(mrg, uuid);

    mrg_index_read_lock(mrg, partition);

    Pvoid_t *sections_judy_pptr = JudyHSGet(mrg->index[partition].uuid_judy, uuid, sizeof(uuid_t));
    if(unlikely(!sections_judy_pptr)) {
        mrg_index_read_unlock(mrg, partition);
        MRG_STATS_SEARCH_MISS(mrg, partition);
        return NULL;
    }

    Pvoid_t *PValue = JudyLGet(*sections_judy_pptr, section, PJE0);
    if(unlikely(!PValue)) {
        mrg_index_read_unlock(mrg, partition);
        MRG_STATS_SEARCH_MISS(mrg, partition);
        return NULL;
    }

    METRIC *metric = *PValue;

    metric_acquire(mrg, metric);

    mrg_index_read_unlock(mrg, partition);

    MRG_STATS_SEARCH_HIT(mrg, partition);
    return metric;
}

static inline bool acquired_metric_del(MRG *mrg, METRIC *metric) {
    size_t partition = metric->partition;

    size_t mem_before_judyl, mem_after_judyl;

    mrg_index_write_lock(mrg, partition);

    if(!metric_release_and_can_be_deleted(mrg, metric)) {
        mrg->index[partition].stats.delete_having_retention_or_referenced++;
        mrg_index_write_unlock(mrg, partition);
        return false;
    }

    Pvoid_t *sections_judy_pptr = JudyHSGet(mrg->index[partition].uuid_judy, &metric->uuid, sizeof(uuid_t));
    if(unlikely(!sections_judy_pptr || !*sections_judy_pptr)) {
        MRG_STATS_DELETE_MISS(mrg, partition);
        mrg_index_write_unlock(mrg, partition);
        return false;
    }

    mem_before_judyl = JudyLMemUsed(*sections_judy_pptr);
    int rc = JudyLDel(sections_judy_pptr, metric->section, PJE0);
    mem_after_judyl = JudyLMemUsed(*sections_judy_pptr);
    mrg_stats_size_judyl_change(mrg, mem_before_judyl, mem_after_judyl, partition);

    if(unlikely(!rc)) {
        MRG_STATS_DELETE_MISS(mrg, partition);
        mrg_index_write_unlock(mrg, partition);
        return false;
    }

    if(!*sections_judy_pptr) {
        rc = JudyHSDel(&mrg->index[partition].uuid_judy, &metric->uuid, sizeof(uuid_t), PJE0);
        if(unlikely(!rc))
            fatal("DBENGINE METRIC: cannot delete UUID from JudyHS");
        mrg_stats_size_judyhs_removed_uuid(mrg, partition);
    }

    MRG_STATS_DELETED_METRIC(mrg, partition);

    mrg_index_write_unlock(mrg, partition);

    aral_freez(mrg->index[partition].aral, metric);

    return true;
}

// ----------------------------------------------------------------------------
// public API

inline MRG *mrg_create(ssize_t partitions) {
    if(partitions < 1)
        partitions = get_netdata_cpus();

    MRG *mrg = callocz(1, sizeof(MRG) + sizeof(struct mrg_partition) * partitions);
    mrg->partitions = partitions;

    for(size_t i = 0; i < mrg->partitions ; i++) {
        rw_spinlock_init(&mrg->index[i].rw_spinlock);

        char buf[ARAL_MAX_NAME + 1];
        snprintfz(buf, ARAL_MAX_NAME, "mrg[%zu]", i);

        mrg->index[i].aral = aral_create(buf, sizeof(METRIC), 0, 16384, &mrg_aral_statistics, NULL, NULL, false, false);
    }

    return mrg;
}

inline size_t mrg_aral_structures(void) {
    return aral_structures_from_stats(&mrg_aral_statistics);
}

inline size_t mrg_aral_overhead(void) {
    return aral_overhead_from_stats(&mrg_aral_statistics);
}

inline void mrg_destroy(MRG *mrg __maybe_unused) {
    // no destruction possible
    // we can't traverse the metrics list

    // to delete entries, the caller needs to keep pointers to them
    // and delete them one by one

    ;
}

inline METRIC *mrg_metric_add_and_acquire(MRG *mrg, MRG_ENTRY entry, bool *ret) {
//    internal_fatal(entry.latest_time_s > max_acceptable_collected_time(),
//        "DBENGINE METRIC: metric latest time is in the future");

    return metric_add_and_acquire(mrg, &entry, ret);
}

inline METRIC *mrg_metric_get_and_acquire(MRG *mrg, uuid_t *uuid, Word_t section) {
    return metric_get_and_acquire(mrg, uuid, section);
}

inline bool mrg_metric_release_and_delete(MRG *mrg, METRIC *metric) {
    return acquired_metric_del(mrg, metric);
}

inline METRIC *mrg_metric_dup(MRG *mrg, METRIC *metric) {
    metric_acquire(mrg, metric);
    return metric;
}

inline bool mrg_metric_release(MRG *mrg, METRIC *metric) {
    return metric_release_and_can_be_deleted(mrg, metric);
}

inline Word_t mrg_metric_id(MRG *mrg __maybe_unused, METRIC *metric) {
    return (Word_t)metric;
}

inline uuid_t *mrg_metric_uuid(MRG *mrg __maybe_unused, METRIC *metric) {
    return &metric->uuid;
}

inline Word_t mrg_metric_section(MRG *mrg __maybe_unused, METRIC *metric) {
    return metric->section;
}

inline bool mrg_metric_set_first_time_s(MRG *mrg __maybe_unused, METRIC *metric, time_t first_time_s) {
    internal_fatal(first_time_s < 0, "DBENGINE METRIC: timestamp is negative");

    if(unlikely(first_time_s < 0))
        return false;

    __atomic_store_n(&metric->first_time_s, first_time_s, __ATOMIC_RELAXED);

    return true;
}

inline void mrg_metric_expand_retention(MRG *mrg __maybe_unused, METRIC *metric, time_t first_time_s, time_t last_time_s, uint32_t update_every_s) {
    internal_fatal(first_time_s < 0 || last_time_s < 0,
                   "DBENGINE METRIC: timestamp is negative");
    internal_fatal(first_time_s > max_acceptable_collected_time(),
                   "DBENGINE METRIC: metric first time is in the future");
    internal_fatal(last_time_s > max_acceptable_collected_time(),
                   "DBENGINE METRIC: metric last time is in the future");

    if(first_time_s > 0)
        set_metric_field_with_condition(metric->first_time_s, first_time_s, _current <= 0 || _wanted < _current);

    if(last_time_s > 0) {
        if(set_metric_field_with_condition(metric->latest_time_s_clean, last_time_s, _current <= 0 || _wanted > _current) &&
            update_every_s > 0)
            // set the latest update every too
            set_metric_field_with_condition(metric->latest_update_every_s, update_every_s, true);
    }
    else if(update_every_s > 0)
        // set it only if it is invalid
        set_metric_field_with_condition(metric->latest_update_every_s, update_every_s, _current <= 0);
}

inline bool mrg_metric_set_first_time_s_if_bigger(MRG *mrg __maybe_unused, METRIC *metric, time_t first_time_s) {
    internal_fatal(first_time_s < 0, "DBENGINE METRIC: timestamp is negative");
    return set_metric_field_with_condition(metric->first_time_s, first_time_s, _wanted > _current);
}

inline time_t mrg_metric_get_first_time_s(MRG *mrg __maybe_unused, METRIC *metric) {
    return mrg_metric_get_first_time_s_smart(mrg, metric);
}

inline void mrg_metric_get_retention(MRG *mrg __maybe_unused, METRIC *metric, time_t *first_time_s, time_t *last_time_s, uint32_t *update_every_s) {
    time_t clean = __atomic_load_n(&metric->latest_time_s_clean, __ATOMIC_RELAXED);
    time_t hot = __atomic_load_n(&metric->latest_time_s_hot, __ATOMIC_RELAXED);

    *last_time_s = MAX(clean, hot);
    *first_time_s = mrg_metric_get_first_time_s_smart(mrg, metric);
    if (update_every_s)
        *update_every_s = __atomic_load_n(&metric->latest_update_every_s, __ATOMIC_RELAXED);
}

inline bool mrg_metric_set_clean_latest_time_s(MRG *mrg __maybe_unused, METRIC *metric, time_t latest_time_s) {
    internal_fatal(latest_time_s < 0, "DBENGINE METRIC: timestamp is negative");

//    internal_fatal(latest_time_s > max_acceptable_collected_time(),
//                   "DBENGINE METRIC: metric latest time is in the future");

//    internal_fatal(metric->latest_time_s_clean > latest_time_s,
//                   "DBENGINE METRIC: metric new clean latest time is older than the previous one");

    if(latest_time_s > 0) {
        if(set_metric_field_with_condition(metric->latest_time_s_clean, latest_time_s, true)) {
            set_metric_field_with_condition(metric->first_time_s, latest_time_s, _current <= 0 || _wanted < _current);

            return true;
        }
    }

    return false;
}

// returns true when metric still has retention
inline bool mrg_metric_zero_disk_retention(MRG *mrg __maybe_unused, METRIC *metric) {
    Word_t section = mrg_metric_section(mrg, metric);
    bool do_again = false;
    size_t countdown = 5;

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

        if (--countdown && !min_first_time_s && __atomic_load_n(&metric->latest_time_s_hot, __ATOMIC_RELAXED))
            do_again = true;
        else {
            internal_error(!countdown, "METRIC: giving up on updating the retention of metric without disk retention");

            do_again = false;
            set_metric_field_with_condition(metric->first_time_s, min_first_time_s, true);
            set_metric_field_with_condition(metric->latest_time_s_clean, max_end_time_s, true);
        }
    } while(do_again);

    time_t first, last;
    mrg_metric_get_retention(mrg, metric, &first, &last, NULL);
    return (first && last && first < last);
}

inline bool mrg_metric_set_hot_latest_time_s(MRG *mrg __maybe_unused, METRIC *metric, time_t latest_time_s) {
    internal_fatal(latest_time_s < 0, "DBENGINE METRIC: timestamp is negative");

//    internal_fatal(latest_time_s > max_acceptable_collected_time(),
//                   "DBENGINE METRIC: metric latest time is in the future");

    if(likely(latest_time_s > 0)) {
        __atomic_store_n(&metric->latest_time_s_hot, latest_time_s, __ATOMIC_RELAXED);
        return true;
    }

    return false;
}

inline time_t mrg_metric_get_latest_time_s(MRG *mrg __maybe_unused, METRIC *metric) {
    time_t clean = __atomic_load_n(&metric->latest_time_s_clean, __ATOMIC_RELAXED);
    time_t hot = __atomic_load_n(&metric->latest_time_s_hot, __ATOMIC_RELAXED);

    return MAX(clean, hot);
}

inline bool mrg_metric_set_update_every(MRG *mrg __maybe_unused, METRIC *metric, uint32_t update_every_s) {
    if(update_every_s > 0)
        return set_metric_field_with_condition(metric->latest_update_every_s, update_every_s, true);

    return false;
}

inline bool mrg_metric_set_update_every_s_if_zero(MRG *mrg __maybe_unused, METRIC *metric, uint32_t update_every_s) {
    if(update_every_s > 0)
        return set_metric_field_with_condition(metric->latest_update_every_s, update_every_s, _current <= 0);

    return false;
}

inline uint32_t mrg_metric_get_update_every_s(MRG *mrg __maybe_unused, METRIC *metric) {
    return __atomic_load_n(&metric->latest_update_every_s, __ATOMIC_RELAXED);
}

inline bool mrg_metric_set_writer(MRG *mrg, METRIC *metric) {
    pid_t expected = __atomic_load_n(&metric->writer, __ATOMIC_RELAXED);
    pid_t wanted = gettid();
    bool done = true;

    do {
        if(expected != 0) {
            done = false;
            break;
        }
    } while(!__atomic_compare_exchange_n(&metric->writer, &expected, wanted, false, __ATOMIC_RELAXED, __ATOMIC_RELAXED));

    if(done)
        __atomic_add_fetch(&mrg->index[metric->partition].stats.writers, 1, __ATOMIC_RELAXED);
    else
        __atomic_add_fetch(&mrg->index[metric->partition].stats.writers_conflicts, 1, __ATOMIC_RELAXED);

    return done;
}

inline bool mrg_metric_clear_writer(MRG *mrg, METRIC *metric) {
    // this function can be called from a different thread than the one than the writer

    pid_t expected = __atomic_load_n(&metric->writer, __ATOMIC_RELAXED);
    pid_t wanted = 0;
    bool done = true;

    do {
        if(!expected) {
            done = false;
            break;
        }
    } while(!__atomic_compare_exchange_n(&metric->writer, &expected, wanted, false, __ATOMIC_RELAXED, __ATOMIC_RELAXED));

    if(done)
        __atomic_sub_fetch(&mrg->index[metric->partition].stats.writers, 1, __ATOMIC_RELAXED);

    return done;
}

inline void mrg_update_metric_retention_and_granularity_by_uuid(
        MRG *mrg, Word_t section, uuid_t *uuid,
        time_t first_time_s, time_t last_time_s,
        uint32_t update_every_s, time_t now_s)
{
    if(unlikely(last_time_s > now_s)) {
        nd_log_limit_static_global_var(erl, 1, 0);
        nd_log_limit(&erl, NDLS_DAEMON, NDLP_WARNING,
                     "DBENGINE JV2: wrong last time on-disk (%ld - %ld, now %ld), "
                     "fixing last time to now",
                     first_time_s, last_time_s, now_s);
        last_time_s = now_s;
    }

    if (unlikely(first_time_s > last_time_s)) {
        nd_log_limit_static_global_var(erl, 1, 0);
        nd_log_limit(&erl, NDLS_DAEMON, NDLP_WARNING,
                     "DBENGINE JV2: wrong first time on-disk (%ld - %ld, now %ld), "
                     "fixing first time to last time",
                     first_time_s, last_time_s, now_s);

        first_time_s = last_time_s;
    }

    if (unlikely(first_time_s == 0 || last_time_s == 0)) {
        nd_log_limit_static_global_var(erl, 1, 0);
        nd_log_limit(&erl, NDLS_DAEMON, NDLP_WARNING,
                     "DBENGINE JV2: zero on-disk timestamps (%ld - %ld, now %ld), "
                     "using them as-is",
                     first_time_s, last_time_s, now_s);
    }

    bool added = false;
    METRIC *metric = mrg_metric_get_and_acquire(mrg, uuid, section);
    if (!metric) {
        MRG_ENTRY entry = {
                .uuid = uuid,
                .section = section,
                .first_time_s = first_time_s,
                .last_time_s = last_time_s,
                .latest_update_every_s = update_every_s
        };
        metric = mrg_metric_add_and_acquire(mrg, entry, &added);
    }

    if (likely(!added))
        mrg_metric_expand_retention(mrg, metric, first_time_s, last_time_s, update_every_s);

    mrg_metric_release(mrg, metric);
}

inline void mrg_get_statistics(MRG *mrg, struct mrg_statistics *s) {
    memset(s, 0, sizeof(struct mrg_statistics));

    for(size_t i = 0; i < mrg->partitions ;i++) {
        s->entries += __atomic_load_n(&mrg->index[i].stats.entries, __ATOMIC_RELAXED);
        s->entries_referenced += __atomic_load_n(&mrg->index[i].stats.entries_referenced, __ATOMIC_RELAXED);
        s->size += __atomic_load_n(&mrg->index[i].stats.size, __ATOMIC_RELAXED);
        s->current_references += __atomic_load_n(&mrg->index[i].stats.current_references, __ATOMIC_RELAXED);
        s->additions += __atomic_load_n(&mrg->index[i].stats.additions, __ATOMIC_RELAXED);
        s->additions_duplicate += __atomic_load_n(&mrg->index[i].stats.additions_duplicate, __ATOMIC_RELAXED);
        s->deletions += __atomic_load_n(&mrg->index[i].stats.deletions, __ATOMIC_RELAXED);
        s->delete_having_retention_or_referenced += __atomic_load_n(&mrg->index[i].stats.delete_having_retention_or_referenced, __ATOMIC_RELAXED);
        s->delete_misses += __atomic_load_n(&mrg->index[i].stats.delete_misses, __ATOMIC_RELAXED);
        s->search_hits += __atomic_load_n(&mrg->index[i].stats.search_hits, __ATOMIC_RELAXED);
        s->search_misses += __atomic_load_n(&mrg->index[i].stats.search_misses, __ATOMIC_RELAXED);
        s->writers += __atomic_load_n(&mrg->index[i].stats.writers, __ATOMIC_RELAXED);
        s->writers_conflicts += __atomic_load_n(&mrg->index[i].stats.writers_conflicts, __ATOMIC_RELAXED);
    }

    s->size += sizeof(MRG) + sizeof(struct mrg_partition) * mrg->partitions;
}

// ----------------------------------------------------------------------------
// unit test

struct mrg_stress_entry {
    uuid_t uuid;
    time_t after;
    time_t before;
};

struct mrg_stress {
    MRG *mrg;
    bool stop;
    size_t entries;
    struct mrg_stress_entry *array;
    size_t updates;
};

static void *mrg_stress(void *ptr) {
    struct mrg_stress *t = ptr;
    MRG *mrg = t->mrg;

    ssize_t start = 0;
    ssize_t end = (ssize_t)t->entries;
    ssize_t step = 1;

    if(gettid() % 2) {
        start = (ssize_t)t->entries - 1;
        end = -1;
        step = -1;
    }

    while(!__atomic_load_n(&t->stop, __ATOMIC_RELAXED)) {
        for (ssize_t i = start; i != end; i += step) {
            struct mrg_stress_entry *e = &t->array[i];

            time_t after = __atomic_sub_fetch(&e->after, 1, __ATOMIC_RELAXED);
            time_t before = __atomic_add_fetch(&e->before, 1, __ATOMIC_RELAXED);

            mrg_update_metric_retention_and_granularity_by_uuid(
                    mrg, 0x01,
                    &e->uuid,
                    after,
                    before,
                    1,
                    before);

            __atomic_add_fetch(&t->updates, 1, __ATOMIC_RELAXED);
        }
    }

    return ptr;
}

int mrg_unittest(void) {
    MRG *mrg = mrg_create(0);
    METRIC *m1_t0, *m2_t0, *m3_t0, *m4_t0;
    METRIC *m1_t1, *m2_t1, *m3_t1, *m4_t1;
    bool ret;

    uuid_t test_uuid;
    uuid_generate(test_uuid);
    MRG_ENTRY entry = {
            .uuid = &test_uuid,
            .section = 0,
            .first_time_s = 2,
            .last_time_s = 3,
            .latest_update_every_s = 4,
    };
    m1_t0 = mrg_metric_add_and_acquire(mrg, entry, &ret);
    if(!ret)
        fatal("DBENGINE METRIC: failed to add metric");

    // add the same metric again
    m2_t0 = mrg_metric_add_and_acquire(mrg, entry, &ret);
    if(m2_t0 != m1_t0)
        fatal("DBENGINE METRIC: adding the same metric twice, does not return the same pointer");
    if(ret)
        fatal("DBENGINE METRIC: managed to add the same metric twice");

    m3_t0 = mrg_metric_get_and_acquire(mrg, entry.uuid, entry.section);
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

    m3_t1 = mrg_metric_get_and_acquire(mrg, entry.uuid, entry.section);
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

    m4_t1 = mrg_metric_get_and_acquire(mrg, entry.uuid, entry.section);
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

    struct mrg_statistics s;
    mrg_get_statistics(mrg, &s);
    if(s.entries != 0)
        fatal("DBENGINE METRIC: invalid entries counter");

    size_t entries = 1000000;
    size_t threads = mrg->partitions / 3 + 1;
    size_t tiers = 3;
    size_t run_for_secs = 5;
    netdata_log_info("preparing stress test of %zu entries...", entries);
    struct mrg_stress t = {
            .mrg = mrg,
            .entries = entries,
            .array = callocz(entries, sizeof(struct mrg_stress_entry)),
    };

    time_t now = max_acceptable_collected_time();
    for(size_t i = 0; i < entries ;i++) {
        uuid_generate_random(t.array[i].uuid);
        t.array[i].after = now / 3;
        t.array[i].before = now / 2;
    }
    netdata_log_info("stress test is populating MRG with 3 tiers...");
    for(size_t i = 0; i < entries ;i++) {
        struct mrg_stress_entry *e = &t.array[i];
        for(size_t tier = 1; tier <= tiers ;tier++) {
            mrg_update_metric_retention_and_granularity_by_uuid(
                    mrg, tier,
                    &e->uuid,
                    e->after,
                    e->before,
                    1,
                    e->before);
        }
    }
    netdata_log_info("stress test ready to run...");

    usec_t started_ut = now_monotonic_usec();

    pthread_t th[threads];
    for(size_t i = 0; i < threads ; i++) {
        char buf[15 + 1];
        snprintfz(buf, sizeof(buf) - 1, "TH[%zu]", i);
        netdata_thread_create(&th[i], buf,
                              NETDATA_THREAD_OPTION_JOINABLE | NETDATA_THREAD_OPTION_DONT_LOG,
                              mrg_stress, &t);
    }

    sleep_usec(run_for_secs * USEC_PER_SEC);
    __atomic_store_n(&t.stop, true, __ATOMIC_RELAXED);

    for(size_t i = 0; i < threads ; i++)
        netdata_thread_cancel(th[i]);

    for(size_t i = 0; i < threads ; i++)
        netdata_thread_join(th[i], NULL);

    usec_t ended_ut = now_monotonic_usec();

    struct mrg_statistics stats;
    mrg_get_statistics(mrg, &stats);

    netdata_log_info("DBENGINE METRIC: did %zu additions, %zu duplicate additions, "
         "%zu deletions, %zu wrong deletions, "
         "%zu successful searches, %zu wrong searches, "
         "in %"PRIu64" usecs",
        stats.additions, stats.additions_duplicate,
        stats.deletions, stats.delete_misses,
        stats.search_hits, stats.search_misses,
        ended_ut - started_ut);

    netdata_log_info("DBENGINE METRIC: updates performance: %0.2fk/sec total, %0.2fk/sec/thread",
         (double)t.updates / (double)((ended_ut - started_ut) / USEC_PER_SEC) / 1000.0,
         (double)t.updates / (double)((ended_ut - started_ut) / USEC_PER_SEC) / 1000.0 / threads);

    mrg_destroy(mrg);

    netdata_log_info("DBENGINE METRIC: all tests passed!");

    return 0;
}
