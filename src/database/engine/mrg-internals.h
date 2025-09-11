// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_MRG_INTERNALS_H
#define NETDATA_MRG_INTERNALS_H

#include "mrg.h"
#include "cache.h"
#include "libnetdata/locks/locks.h"
#include "rrddiskprotocol.h"

struct metric {
    Word_t section;                 // never changes
    UUIDMAP_ID uuid;                 // never changes

    REFCOUNT refcount;
    uint8_t partition;
    bool deleted;

    uint32_t latest_update_every_s; // the latest data collection frequency

    time_t first_time_s;            // the timestamp of the oldest point in the database
    time_t latest_time_s_clean;     // the timestamp of the newest point in the database
    time_t latest_time_s_hot;       // the timestamp of the latest point that has been collected (not yet stored)

#ifdef NETDATA_INTERNAL_CHECKS
    pid_t writer;
#endif

    // THIS IS allocated with malloc()
    // YOU HAVE TO INITIALIZE IT YOURSELF!
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

extern struct aral_statistics mrg_aral_statistics;

struct mrg {
    struct mrg_partition {
        ARAL *aral;                 // not protected by our spinlock - it has its own

        RW_SPINLOCK rw_spinlock;
        Pvoid_t uuid_judy;          // JudyL: each UUID has a JudyL of sections (tiers)

        struct mrg_statistics stats;
    } index[UUIDMAP_PARTITIONS];
};

static inline void MRG_STATS_DUPLICATE_ADD(MRG *mrg, size_t partition) {
    mrg->index[partition].stats.additions_duplicate++;
}

static inline void MRG_STATS_ADDED_METRIC(MRG *mrg, size_t partition, Word_t section) {
    mrg->index[partition].stats.entries++;
    mrg->index[partition].stats.additions++;
    mrg->index[partition].stats.size += sizeof(METRIC);
    struct rrdengine_instance *ctx = (struct rrdengine_instance *) section;
    __atomic_add_fetch(&ctx->atomic.metrics, 1, __ATOMIC_RELAXED);
}

static inline void MRG_STATS_DELETED_METRIC(MRG *mrg, size_t partition, Word_t section) {
    mrg->index[partition].stats.entries--;
    mrg->index[partition].stats.size -= sizeof(METRIC);
    mrg->index[partition].stats.deletions++;
    struct rrdengine_instance *ctx = (struct rrdengine_instance *) section;
    __atomic_sub_fetch(&ctx->atomic.metrics, 1, __ATOMIC_RELAXED);
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

static inline void mrg_stats_judy_mem(MRG *mrg, size_t partition, int64_t judy_mem) {
    __atomic_add_fetch(&mrg->index[partition].stats.size, judy_mem, __ATOMIC_RELAXED);
}

static inline void metric_log(MRG *mrg __maybe_unused, METRIC *metric, const char *msg) {
    struct rrdengine_instance *ctx = (struct rrdengine_instance *)metric->section;

    nd_uuid_t uuid;
    uuidmap_uuid(metric->uuid, uuid);
    char uuid_txt[UUID_STR_LEN];
    uuid_unparse_lower(uuid, uuid_txt);
    nd_log(NDLS_DAEMON, NDLP_ERR,
           "METRIC: %s on %s at tier %d, refcount %d, partition %u, "
           "retention [%ld - %ld (hot), %ld (clean)], update every %"PRIu32
#ifdef NETDATA_INTERNAL_CHECKS
           ", writer pid %d "
#endif
           " --- PLEASE OPEN A GITHUB ISSUE TO REPORT THIS LOG LINE TO NETDATA --- ",
           msg,
           uuid_txt,
           ctx->config.tier,
           metric->refcount,
           metric->partition,
           metric->first_time_s,
           metric->latest_time_s_hot,
           metric->latest_time_s_clean,
           metric->latest_update_every_s
#ifdef NETDATA_INTERNAL_CHECKS
           , (int)metric->writer
#endif
    );
}


ALWAYS_INLINE
static time_t mrg_metric_get_first_time_s_smart(MRG *mrg __maybe_unused, METRIC *metric) {
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

ALWAYS_INLINE
static bool acquired_metric_has_retention(MRG *mrg, METRIC *metric) {
    time_t first, last;
    mrg_metric_get_retention(mrg, metric, &first, &last, NULL);
    bool rc = (first != 0 && last != 0 && first <= last);

    if(!rc && __atomic_load_n(&mrg->index[metric->partition].stats.writers, __ATOMIC_RELAXED) > 0)
        rc = true;

    return rc;
}

ALWAYS_INLINE
static void acquired_for_deletion_metric_delete(MRG *mrg, METRIC *metric) {
    JudyAllocThreadPulseReset();

    size_t partition = metric->partition;

    mrg_index_write_lock(mrg, partition);

    Pvoid_t *sections_judy_pptr = JudyLGet(mrg->index[partition].uuid_judy, metric->uuid, PJE0);
    if(unlikely(sections_judy_pptr == PJERR))
        fatal("METRIC: corrupted JudyL");

    if(unlikely(!sections_judy_pptr || !*sections_judy_pptr)) {
        MRG_STATS_DELETE_MISS(mrg, partition);
        mrg_index_write_unlock(mrg, partition);
        return;
    }

    int rc = JudyLDel(sections_judy_pptr, metric->section, PJE0);
    if(unlikely(!rc)) {
        MRG_STATS_DELETE_MISS(mrg, partition);
        mrg_index_write_unlock(mrg, partition);
        mrg_stats_judy_mem(mrg, partition, JudyAllocThreadPulseGetAndReset());
        return;
    }

    if(!*sections_judy_pptr) {
        rc = JudyLDel(&mrg->index[partition].uuid_judy, metric->uuid, PJE0);

        if(unlikely(!rc))
            fatal("DBENGINE METRIC: cannot delete UUID from JudyL");
    }

    MRG_STATS_DELETED_METRIC(mrg, partition, metric->section);

    mrg_index_write_unlock(mrg, partition);

    __atomic_store_n(&metric->deleted, true, __ATOMIC_RELEASE);

    mrg_stats_judy_mem(mrg, partition, JudyAllocThreadPulseGetAndReset());
}

ALWAYS_INLINE
static bool metric_acquire(MRG *mrg, METRIC *metric) {
    REFCOUNT rc = refcount_acquire_advanced(&metric->refcount);
    if(!REFCOUNT_ACQUIRED(rc))
        return false;

    if (__atomic_load_n(&metric->deleted, __ATOMIC_ACQUIRE)) {
        refcount_release(&metric->refcount);
        return false;
    }

    size_t partition = metric->partition;

    if(rc == 1)
        __atomic_add_fetch(&mrg->index[partition].stats.entries_acquired, 1, __ATOMIC_RELAXED);

    __atomic_add_fetch(&mrg->index[partition].stats.current_references, 1, __ATOMIC_RELAXED);

    return true;
}

ALWAYS_INLINE
static bool metric_release(MRG *mrg, METRIC *metric) {
    size_t partition = metric->partition;

    if (refcount_release(&metric->refcount) == 0) {
        // we are the last user
        bool already_deleted = __atomic_load_n(&metric->deleted, __ATOMIC_ACQUIRE);
        if (already_deleted || !acquired_metric_has_retention(mrg, metric)) {
            if (!already_deleted) {
                 acquired_for_deletion_metric_delete(mrg, metric);
            }
            uuidmap_free(metric->uuid);
            aral_freez(mrg->index[partition].aral, metric);
            __atomic_sub_fetch(&mrg->index[partition].stats.entries_acquired, 1, __ATOMIC_RELAXED);
            __atomic_sub_fetch(&mrg->index[partition].stats.current_references, 1, __ATOMIC_RELAXED);
            return true;
        }
    }

    __atomic_sub_fetch(&mrg->index[partition].stats.current_references, 1, __ATOMIC_RELAXED);
    return false;
}

ALWAYS_INLINE
static METRIC *metric_add_and_acquire(MRG *mrg, MRG_ENTRY *entry, bool *ret) {
    JudyAllocThreadPulseReset();

    UUIDMAP_ID id = uuidmap_create(*entry->uuid);

    size_t partition = uuid_to_uuidmap_partition(*entry->uuid);

    METRIC *allocation = aral_mallocz(mrg->index[partition].aral);
    Pvoid_t *PValue;

    while(1) {
        mrg_index_write_lock(mrg, partition);

        Pvoid_t *sections_judy_pptr = JudyLIns(&mrg->index[partition].uuid_judy, id, PJE0);
        if (unlikely(!sections_judy_pptr || sections_judy_pptr == PJERR))
            fatal("DBENGINE METRIC: corrupted UUIDs JudyL array");

        PValue = JudyLIns(sections_judy_pptr, entry->section, PJE0);
        if (unlikely(!PValue || PValue == PJERR))
            fatal("DBENGINE METRIC: corrupted section JudyL array");

        if (unlikely(*PValue != NULL)) {
            METRIC *metric = *PValue;

            if(!metric_acquire(mrg, metric)) {
                mrg_index_write_unlock(mrg, partition);
                continue;
            }

            MRG_STATS_DUPLICATE_ADD(mrg, partition);
            mrg_index_write_unlock(mrg, partition);

            if (ret)
                *ret = false;

            uuidmap_free(id);
            aral_freez(mrg->index[partition].aral, allocation);

            mrg_stats_judy_mem(mrg, partition, JudyAllocThreadPulseGetAndReset());
            return metric;
        }

        break;
    }

    METRIC *metric = allocation;
    metric->uuid = id;
    metric->section = entry->section;
    metric->first_time_s = MAX(0, entry->first_time_s);
    metric->latest_time_s_clean = MAX(0, entry->last_time_s);
    metric->latest_time_s_hot = 0;
    metric->latest_update_every_s = entry->latest_update_every_s;
    metric->deleted = false;
#ifdef NETDATA_INTERNAL_CHECKS
    metric->writer = 0;
#endif
    metric->refcount = 1;
    metric->partition = partition;
    *PValue = metric;

    __atomic_add_fetch(&mrg->index[partition].stats.entries_acquired, 1, __ATOMIC_RELAXED);
    __atomic_add_fetch(&mrg->index[partition].stats.current_references, 1, __ATOMIC_RELAXED);

    MRG_STATS_ADDED_METRIC(mrg, partition, metric->section);

    mrg_index_write_unlock(mrg, partition);

    if(ret)
        *ret = true;

    mrg_stats_judy_mem(mrg, partition, JudyAllocThreadPulseGetAndReset());
    return metric;
}

ALWAYS_INLINE
static METRIC *metric_get_and_acquire_by_id(MRG *mrg, UUIDMAP_ID id, Word_t section) {
    size_t partition = uuidmap_id_to_partition(id);

    while(1) {
        mrg_index_read_lock(mrg, partition);

        Pvoid_t *sections_judy_pptr = JudyLGet(mrg->index[partition].uuid_judy, id, PJE0);
        if (unlikely(!sections_judy_pptr)) {
            mrg_index_read_unlock(mrg, partition);
            MRG_STATS_SEARCH_MISS(mrg, partition);
            return NULL;
        }

        Pvoid_t *PValue = JudyLGet(*sections_judy_pptr, section, PJE0);
        if (unlikely(!PValue)) {
            mrg_index_read_unlock(mrg, partition);
            MRG_STATS_SEARCH_MISS(mrg, partition);
            return NULL;
        }

        METRIC *metric = *PValue;

        if(metric && !metric_acquire(mrg, metric))
            metric = NULL;

        mrg_index_read_unlock(mrg, partition);

        if(metric) {
            MRG_STATS_SEARCH_HIT(mrg, partition);
            return metric;
        }
    }
}

#endif //NETDATA_MRG_INTERNALS_H