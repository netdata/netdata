// SPDX-License-Identifier: GPL-3.0-or-later

#define PULSE_INTERNALS 1
#include "pulse-ingestion.h"

// Every chart completion on every collecting thread adds to these counters.
// A single shared array made all those threads fight for one cache line,
// so the counters are split into cache-line sized shards: each thread picks
// one shard on first use and the pulse reader sums them all.
#define INGEST_STATISTICS_SHARDS 64

struct ingest_statistics {
    uint64_t db_points_stored_per_tier[RRD_STORAGE_TIERS];
};

static struct ingest_statistics_shard {
    struct ingest_statistics stats;
} __attribute__((aligned(64))) ingest_statistics_shards[INGEST_STATISTICS_SHARDS] = { 0 };

static size_t ingest_statistics_next_shard = 0;
static __thread struct ingest_statistics *ingest_statistics_thread_shard = NULL;

static struct ingest_statistics *ingest_statistics_shard_get(void) {
    size_t slot = __atomic_fetch_add(&ingest_statistics_next_shard, 1, __ATOMIC_RELAXED);
    ingest_statistics_thread_shard = &ingest_statistics_shards[slot % INGEST_STATISTICS_SHARDS].stats;
    return ingest_statistics_thread_shard;
}

ALWAYS_INLINE void pulse_queries_rrdset_collection_completed(size_t *points_read_per_tier_array) {
    struct ingest_statistics *shard = ingest_statistics_thread_shard;
    if(unlikely(!shard))
        shard = ingest_statistics_shard_get();

    for(size_t tier = 0; tier < nd_profile.storage_tiers;tier++) {
        size_t points = points_read_per_tier_array[tier];
        if(!points) continue;

        // a shard may still be shared by more than one thread
        __atomic_fetch_add(&shard->db_points_stored_per_tier[tier], points, __ATOMIC_RELAXED);
        points_read_per_tier_array[tier] = 0;
    }
}

static inline void pulse_ingestion_copy(struct ingest_statistics *gs) {
    for(size_t tier = 0; tier < nd_profile.storage_tiers;tier++) {
        uint64_t sum = 0;
        for(size_t s = 0; s < INGEST_STATISTICS_SHARDS; s++)
            sum += __atomic_load_n(&ingest_statistics_shards[s].stats.db_points_stored_per_tier[tier], __ATOMIC_RELAXED);
        gs->db_points_stored_per_tier[tier] = sum;
    }
}

void pulse_ingestion_do(bool extended __maybe_unused) {
    static struct ingest_statistics gs;
    pulse_ingestion_copy(&gs);

    {
        static RRDSET *st_points_stored = NULL;
        static RRDDIM *rds[RRD_STORAGE_TIERS] = {};

        if (unlikely(!st_points_stored)) {
            st_points_stored = rrdset_create_localhost(
                "netdata"
                , "db_samples_collected"
                , NULL
                , "Data Collection Samples"
                , NULL
                , "Netdata Time-Series Collected Samples"
                , "samples/s"
                , "netdata"
                , "pulse"
                , 131003
                , localhost->rrd_update_every
                , RRDSET_TYPE_STACKED
            );

            for(size_t tier = 0; tier < nd_profile.storage_tiers;tier++) {
                char buf[30 + 1];
                snprintfz(buf, sizeof(buf) - 1, "tier%zu", tier);
                rds[tier] = rrddim_add(st_points_stored, buf, NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            }
        }

        for(size_t tier = 0; tier < nd_profile.storage_tiers;tier++)
            rrddim_set_by_pointer(st_points_stored, rds[tier], (collected_number)gs.db_points_stored_per_tier[tier]);

        rrdset_done(st_points_stored);
    }
}
