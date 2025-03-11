// SPDX-License-Identifier: GPL-3.0-or-later

#define PULSE_INTERNALS 1
#include "pulse-ingestion.h"

static struct ingest_statistics {
    uint64_t db_points_stored_per_tier[RRD_STORAGE_TIERS];
} ingest_statistics = { 0 };

ALWAYS_INLINE void pulse_queries_rrdset_collection_completed(size_t *points_read_per_tier_array) {
    for(size_t tier = 0; tier < nd_profile.storage_tiers;tier++) {
        __atomic_fetch_add(&ingest_statistics.db_points_stored_per_tier[tier], points_read_per_tier_array[tier], __ATOMIC_RELAXED);
        points_read_per_tier_array[tier] = 0;
    }
}

static inline void pulse_ingestion_copy(struct ingest_statistics *gs) {
    for(size_t tier = 0; tier < nd_profile.storage_tiers;tier++)
        gs->db_points_stored_per_tier[tier] = __atomic_load_n(&ingest_statistics.db_points_stored_per_tier[tier], __ATOMIC_RELAXED);
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
