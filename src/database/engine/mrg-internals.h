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

#endif //NETDATA_MRG_INTERNALS_H
