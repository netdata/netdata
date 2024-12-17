// SPDX-License-Identifier: GPL-3.0-or-later

#define PULSE_INTERNALS 1
#include "pulse-gorilla.h"

static struct gorilla_statistics {
    bool enabled;

    PAD64(uint64_t) tier0_hot_gorilla_buffers;

    PAD64(uint64_t) gorilla_tier0_disk_actual_bytes;
    PAD64(uint64_t) gorilla_tier0_disk_optimal_bytes;
    PAD64(uint64_t) gorilla_tier0_disk_original_bytes;
} gorilla_statistics = { 0 };

void pulse_gorilla_hot_buffer_added() {
    if(!gorilla_statistics.enabled) return;

    __atomic_fetch_add(&gorilla_statistics.tier0_hot_gorilla_buffers, 1, __ATOMIC_RELAXED);
}

void pulse_gorilla_tier0_page_flush(uint32_t actual, uint32_t optimal, uint32_t original) {
    if(!gorilla_statistics.enabled) return;

    __atomic_fetch_add(&gorilla_statistics.gorilla_tier0_disk_actual_bytes, actual, __ATOMIC_RELAXED);
    __atomic_fetch_add(&gorilla_statistics.gorilla_tier0_disk_optimal_bytes, optimal, __ATOMIC_RELAXED);
    __atomic_fetch_add(&gorilla_statistics.gorilla_tier0_disk_original_bytes, original, __ATOMIC_RELAXED);
}

static inline void global_statistics_copy(struct gorilla_statistics *gs) {
    gs->tier0_hot_gorilla_buffers     = __atomic_load_n(&gorilla_statistics.tier0_hot_gorilla_buffers, __ATOMIC_RELAXED);
    gs->gorilla_tier0_disk_actual_bytes = __atomic_load_n(&gorilla_statistics.gorilla_tier0_disk_actual_bytes, __ATOMIC_RELAXED);
    gs->gorilla_tier0_disk_optimal_bytes = __atomic_load_n(&gorilla_statistics.gorilla_tier0_disk_optimal_bytes, __ATOMIC_RELAXED);
    gs->gorilla_tier0_disk_original_bytes = __atomic_load_n(&gorilla_statistics.gorilla_tier0_disk_original_bytes, __ATOMIC_RELAXED);
}

void pulse_gorilla_do(bool extended __maybe_unused) {
#ifdef ENABLE_DBENGINE
    if(!extended) return;
    gorilla_statistics.enabled = true;

    struct gorilla_statistics gs;
    global_statistics_copy(&gs);

    if (tier_page_type[0] == RRDENG_PAGE_TYPE_GORILLA_32BIT)
    {
        static RRDSET *st_tier0_gorilla_pages = NULL;
        static RRDDIM *rd_num_gorilla_pages = NULL;

        if (unlikely(!st_tier0_gorilla_pages)) {
            st_tier0_gorilla_pages = rrdset_create_localhost(
                "netdata"
                , "tier0_gorilla_pages"
                , NULL
                , "dbengine gorilla"
                , NULL
                , "Number of gorilla_pages"
                , "count"
                , "netdata"
                , "pulse"
                , 131004
                , localhost->rrd_update_every
                , RRDSET_TYPE_LINE
            );

            rd_num_gorilla_pages = rrddim_add(st_tier0_gorilla_pages, "count", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st_tier0_gorilla_pages, rd_num_gorilla_pages, (collected_number)gs.tier0_hot_gorilla_buffers);

        rrdset_done(st_tier0_gorilla_pages);
    }

    if (tier_page_type[0] == RRDENG_PAGE_TYPE_GORILLA_32BIT)
    {
        static RRDSET *st_tier0_compression_info = NULL;

        static RRDDIM *rd_actual_bytes = NULL;
        static RRDDIM *rd_optimal_bytes = NULL;
        static RRDDIM *rd_uncompressed_bytes = NULL;

        if (unlikely(!st_tier0_compression_info)) {
            st_tier0_compression_info = rrdset_create_localhost(
                "netdata"
                , "tier0_gorilla_efficiency"
                , NULL
                , "dbengine gorilla"
                , NULL
                , "DBENGINE Gorilla Compression Efficiency on Tier 0"
                , "bytes"
                , "netdata"
                , "pulse"
                , 131005
                , localhost->rrd_update_every
                , RRDSET_TYPE_LINE
            );

            rd_actual_bytes = rrddim_add(st_tier0_compression_info, "actual", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_optimal_bytes = rrddim_add(st_tier0_compression_info, "optimal", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_uncompressed_bytes = rrddim_add(st_tier0_compression_info, "uncompressed", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        }

        rrddim_set_by_pointer(st_tier0_compression_info, rd_actual_bytes, (collected_number)gs.gorilla_tier0_disk_actual_bytes);
        rrddim_set_by_pointer(st_tier0_compression_info, rd_optimal_bytes, (collected_number)gs.gorilla_tier0_disk_optimal_bytes);
        rrddim_set_by_pointer(st_tier0_compression_info, rd_uncompressed_bytes, (collected_number)gs.gorilla_tier0_disk_original_bytes);

        rrdset_done(st_tier0_compression_info);
    }
#endif
}
