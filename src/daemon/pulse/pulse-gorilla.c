// SPDX-License-Identifier: GPL-3.0-or-later

#define PULSE_INTERNALS 1
#include "pulse-gorilla.h"
#include "database/engine/dbengine-stats.h"

void pulse_gorilla_do(bool extended __maybe_unused) {
#ifdef ENABLE_DBENGINE
    if(!extended) return;

    const dbengine_stats_t *s = dbengine_get_stats();
    uint64_t hot_buffers    = __atomic_load_n(&s->gorilla.hot_buffers_added,        __ATOMIC_RELAXED);
    uint64_t actual_bytes   = __atomic_load_n(&s->gorilla.tier0_disk_actual_bytes,  __ATOMIC_RELAXED);
    uint64_t optimal_bytes  = __atomic_load_n(&s->gorilla.tier0_disk_optimal_bytes, __ATOMIC_RELAXED);
    uint64_t original_bytes = __atomic_load_n(&s->gorilla.tier0_disk_original_bytes, __ATOMIC_RELAXED);

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

        rrddim_set_by_pointer(st_tier0_gorilla_pages, rd_num_gorilla_pages, (collected_number)hot_buffers);

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

        rrddim_set_by_pointer(st_tier0_compression_info, rd_actual_bytes, (collected_number)actual_bytes);
        rrddim_set_by_pointer(st_tier0_compression_info, rd_optimal_bytes, (collected_number)optimal_bytes);
        rrddim_set_by_pointer(st_tier0_compression_info, rd_uncompressed_bytes, (collected_number)original_bytes);

        rrdset_done(st_tier0_compression_info);
    }
#endif
}
