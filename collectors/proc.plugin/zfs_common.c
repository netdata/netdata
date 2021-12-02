// SPDX-License-Identifier: GPL-3.0-or-later

#include "zfs_common.h"

struct arcstats arcstats = { 0 };

void generate_charts_arcstats(const char *plugin, const char *module, int show_zero_charts, int update_every) {
    static int do_arc_size = -1, do_l2_size = -1, do_reads = -1, do_l2bytes = -1, do_ahits = -1, do_dhits = -1, \
               do_phits = -1, do_mhits = -1, do_l2hits = -1, do_list_hits = -1;

    if(unlikely(do_arc_size == -1))
        do_arc_size = do_l2_size = do_reads = do_l2bytes = do_ahits = do_dhits = do_phits = do_mhits \
        = do_l2hits = do_list_hits = show_zero_charts;

    // ARC reads
    unsigned long long aread = arcstats.hits + arcstats.misses;

    // Demand reads
    unsigned long long dhit = arcstats.demand_data_hits + arcstats.demand_metadata_hits;
    unsigned long long dmiss = arcstats.demand_data_misses + arcstats.demand_metadata_misses;
    unsigned long long dread = dhit + dmiss;

    // Prefetch reads
    unsigned long long phit = arcstats.prefetch_data_hits + arcstats.prefetch_metadata_hits;
    unsigned long long pmiss = arcstats.prefetch_data_misses + arcstats.prefetch_metadata_misses;
    unsigned long long pread = phit + pmiss;

    // Metadata reads
    unsigned long long mhit = arcstats.prefetch_metadata_hits + arcstats.demand_metadata_hits;
    unsigned long long mmiss = arcstats.prefetch_metadata_misses + arcstats.demand_metadata_misses;
    unsigned long long mread = mhit + mmiss;

    // l2 reads
    unsigned long long l2hit = arcstats.l2_hits;
    unsigned long long l2miss = arcstats.l2_misses;
    unsigned long long l2read = l2hit + l2miss;

    // --------------------------------------------------------------------

    if(do_arc_size == CONFIG_BOOLEAN_YES || arcstats.size || arcstats.c || arcstats.c_min || arcstats.c_max) {
        do_arc_size = CONFIG_BOOLEAN_YES;

        static RRDSET *st_arc_size = NULL;
        static RRDDIM *rd_arc_size = NULL;
        static RRDDIM *rd_arc_target_size = NULL;
        static RRDDIM *rd_arc_target_min_size = NULL;
        static RRDDIM *rd_arc_target_max_size = NULL;

        if (unlikely(!st_arc_size)) {
            st_arc_size = rrdset_create_localhost(
                    "zfs"
                    , "arc_size"
                    , NULL
                    , ZFS_FAMILY_SIZE
                    , NULL
                    , "ZFS ARC Size"
                    , "MiB"
                    , plugin
                    , module
                    , NETDATA_CHART_PRIO_ZFS_ARC_SIZE
                    , update_every
                    , RRDSET_TYPE_AREA
            );

            rd_arc_size            = rrddim_add(st_arc_size, "size",   "arcsz", 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
            rd_arc_target_size     = rrddim_add(st_arc_size, "target", NULL,    1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
            rd_arc_target_min_size = rrddim_add(st_arc_size, "min",    "min (hard limit)", 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
            rd_arc_target_max_size = rrddim_add(st_arc_size, "max",    "max (high water)", 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
        }
        else
            rrdset_next(st_arc_size);

        rrddim_set_by_pointer(st_arc_size, rd_arc_size,            arcstats.size);
        rrddim_set_by_pointer(st_arc_size, rd_arc_target_size,     arcstats.c);
        rrddim_set_by_pointer(st_arc_size, rd_arc_target_min_size, arcstats.c_min);
        rrddim_set_by_pointer(st_arc_size, rd_arc_target_max_size, arcstats.c_max);
        rrdset_done(st_arc_size);
    }

    // --------------------------------------------------------------------

    if(likely(arcstats.l2exist) && (do_l2_size == CONFIG_BOOLEAN_YES || arcstats.l2_size || arcstats.l2_asize)) {
        do_l2_size = CONFIG_BOOLEAN_YES;

        static RRDSET *st_l2_size = NULL;
        static RRDDIM *rd_l2_size = NULL;
        static RRDDIM *rd_l2_asize = NULL;

        if (unlikely(!st_l2_size)) {
            st_l2_size = rrdset_create_localhost(
                    "zfs"
                    , "l2_size"
                    , NULL
                    , ZFS_FAMILY_SIZE
                    , NULL
                    , "ZFS L2 ARC Size"
                    , "MiB"
                    , plugin
                    , module
                    , NETDATA_CHART_PRIO_ZFS_L2_SIZE
                    , update_every
                    , RRDSET_TYPE_AREA
            );

            rd_l2_asize = rrddim_add(st_l2_size, "actual", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
            rd_l2_size  = rrddim_add(st_l2_size, "size",   NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
        }
        else
            rrdset_next(st_l2_size);

        rrddim_set_by_pointer(st_l2_size, rd_l2_size,  arcstats.l2_size);
        rrddim_set_by_pointer(st_l2_size, rd_l2_asize, arcstats.l2_asize);
        rrdset_done(st_l2_size);
    }

    // --------------------------------------------------------------------

    if(likely(do_reads == CONFIG_BOOLEAN_YES || aread || dread || pread || mread || l2read)) {
        do_reads = CONFIG_BOOLEAN_YES;

        static RRDSET *st_reads = NULL;
        static RRDDIM *rd_aread = NULL;
        static RRDDIM *rd_dread = NULL;
        static RRDDIM *rd_pread = NULL;
        static RRDDIM *rd_mread = NULL;
        static RRDDIM *rd_l2read = NULL;

        if (unlikely(!st_reads)) {
            st_reads = rrdset_create_localhost(
                    "zfs"
                    , "reads"
                    , NULL
                    , ZFS_FAMILY_ACCESSES
                    , NULL
                    , "ZFS Reads"
                    , "reads/s"
                    , plugin
                    , module
                    , NETDATA_CHART_PRIO_ZFS_READS
                    , update_every
                    , RRDSET_TYPE_AREA
            );

            rd_aread  = rrddim_add(st_reads, "areads",  "arc",      1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_dread  = rrddim_add(st_reads, "dreads",  "demand",   1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_pread  = rrddim_add(st_reads, "preads",  "prefetch", 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_mread  = rrddim_add(st_reads, "mreads",  "metadata", 1, 1, RRD_ALGORITHM_INCREMENTAL);

            if(arcstats.l2exist)
                rd_l2read = rrddim_add(st_reads, "l2reads", "l2",       1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        else
            rrdset_next(st_reads);

        rrddim_set_by_pointer(st_reads, rd_aread,  aread);
        rrddim_set_by_pointer(st_reads, rd_dread,  dread);
        rrddim_set_by_pointer(st_reads, rd_pread,  pread);
        rrddim_set_by_pointer(st_reads, rd_mread,  mread);

        if(arcstats.l2exist)
            rrddim_set_by_pointer(st_reads, rd_l2read, l2read);

        rrdset_done(st_reads);
    }

    // --------------------------------------------------------------------

    if(likely(arcstats.l2exist && (do_l2bytes == CONFIG_BOOLEAN_YES || arcstats.l2_read_bytes || arcstats.l2_write_bytes))) {
        do_l2bytes = CONFIG_BOOLEAN_YES;

        static RRDSET *st_l2bytes = NULL;
        static RRDDIM *rd_l2_read_bytes = NULL;
        static RRDDIM *rd_l2_write_bytes = NULL;

        if (unlikely(!st_l2bytes)) {
            st_l2bytes = rrdset_create_localhost(
                    "zfs"
                    , "bytes"
                    , NULL
                    , ZFS_FAMILY_ACCESSES
                    , NULL
                    , "ZFS ARC L2 Read/Write Rate"
                    , "KiB/s"
                    , plugin
                    , module
                    , NETDATA_CHART_PRIO_ZFS_IO
                    , update_every
                    , RRDSET_TYPE_AREA
            );

            rd_l2_read_bytes  = rrddim_add(st_l2bytes, "read",  NULL,  1, 1024, RRD_ALGORITHM_INCREMENTAL);
            rd_l2_write_bytes = rrddim_add(st_l2bytes, "write", NULL, -1, 1024, RRD_ALGORITHM_INCREMENTAL);
        }
        else
            rrdset_next(st_l2bytes);

        rrddim_set_by_pointer(st_l2bytes, rd_l2_read_bytes, arcstats.l2_read_bytes);
        rrddim_set_by_pointer(st_l2bytes, rd_l2_write_bytes, arcstats.l2_write_bytes);
        rrdset_done(st_l2bytes);
    }

    // --------------------------------------------------------------------

    if(likely(do_ahits == CONFIG_BOOLEAN_YES || arcstats.hits || arcstats.misses)) {
        do_ahits = CONFIG_BOOLEAN_YES;

        static RRDSET *st_ahits = NULL;
        static RRDDIM *rd_ahits = NULL;
        static RRDDIM *rd_amisses = NULL;

        if (unlikely(!st_ahits)) {
            st_ahits = rrdset_create_localhost(
                    "zfs"
                    , "hits"
                    , NULL
                    , ZFS_FAMILY_EFFICIENCY
                    , NULL
                    , "ZFS ARC Hits"
                    , "percentage"
                    , plugin
                    , module
                    , NETDATA_CHART_PRIO_ZFS_HITS
                    , update_every
                    , RRDSET_TYPE_STACKED
            );

            rd_ahits   = rrddim_add(st_ahits, "hits", NULL,   1, 1, RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL);
            rd_amisses = rrddim_add(st_ahits, "misses", NULL, 1, 1, RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL);
        }
        else
            rrdset_next(st_ahits);

        rrddim_set_by_pointer(st_ahits, rd_ahits,   arcstats.hits);
        rrddim_set_by_pointer(st_ahits, rd_amisses, arcstats.misses);
        rrdset_done(st_ahits);
    }

    // --------------------------------------------------------------------

    if(likely(do_dhits == CONFIG_BOOLEAN_YES || dhit || dmiss)) {
        do_dhits = CONFIG_BOOLEAN_YES;

        static RRDSET *st_dhits = NULL;
        static RRDDIM *rd_dhits = NULL;
        static RRDDIM *rd_dmisses = NULL;

        if (unlikely(!st_dhits)) {
            st_dhits = rrdset_create_localhost(
                    "zfs"
                    , "dhits"
                    , NULL
                    , ZFS_FAMILY_EFFICIENCY
                    , NULL
                    , "ZFS Demand Hits"
                    , "percentage"
                    , plugin
                    , module
                    , NETDATA_CHART_PRIO_ZFS_DHITS
                    , update_every
                    , RRDSET_TYPE_STACKED
            );

            rd_dhits   = rrddim_add(st_dhits, "hits",   NULL, 1, 1, RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL);
            rd_dmisses = rrddim_add(st_dhits, "misses", NULL, 1, 1, RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL);
        }
        else
            rrdset_next(st_dhits);

        rrddim_set_by_pointer(st_dhits, rd_dhits,   dhit);
        rrddim_set_by_pointer(st_dhits, rd_dmisses, dmiss);
        rrdset_done(st_dhits);
    }

    // --------------------------------------------------------------------

    if(likely(do_phits == CONFIG_BOOLEAN_YES || phit || pmiss)) {
        do_phits = CONFIG_BOOLEAN_YES;

        static RRDSET *st_phits = NULL;
        static RRDDIM *rd_phits = NULL;
        static RRDDIM *rd_pmisses = NULL;

        if (unlikely(!st_phits)) {
            st_phits = rrdset_create_localhost(
                    "zfs"
                    , "phits"
                    , NULL
                    , ZFS_FAMILY_EFFICIENCY
                    , NULL
                    , "ZFS Prefetch Hits"
                    , "percentage"
                    , plugin
                    , module
                    , NETDATA_CHART_PRIO_ZFS_PHITS
                    , update_every
                    , RRDSET_TYPE_STACKED
            );

            rd_phits   = rrddim_add(st_phits, "hits",   NULL, 1, 1, RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL);
            rd_pmisses = rrddim_add(st_phits, "misses", NULL, 1, 1, RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL);
        }
        else
            rrdset_next(st_phits);

        rrddim_set_by_pointer(st_phits, rd_phits,   phit);
        rrddim_set_by_pointer(st_phits, rd_pmisses, pmiss);
        rrdset_done(st_phits);
    }

    // --------------------------------------------------------------------

    if(likely(do_mhits == CONFIG_BOOLEAN_YES || mhit || mmiss)) {
        do_mhits = CONFIG_BOOLEAN_YES;

        static RRDSET *st_mhits = NULL;
        static RRDDIM *rd_mhits = NULL;
        static RRDDIM *rd_mmisses = NULL;

        if (unlikely(!st_mhits)) {
            st_mhits = rrdset_create_localhost(
                    "zfs"
                    , "mhits"
                    , NULL
                    , ZFS_FAMILY_EFFICIENCY
                    , NULL
                    , "ZFS Metadata Hits"
                    , "percentage"
                    , plugin
                    , module
                    , NETDATA_CHART_PRIO_ZFS_MHITS
                    , update_every
                    , RRDSET_TYPE_STACKED
            );

            rd_mhits   = rrddim_add(st_mhits, "hits",   NULL, 1, 1, RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL);
            rd_mmisses = rrddim_add(st_mhits, "misses", NULL, 1, 1, RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL);
        }
        else
            rrdset_next(st_mhits);

        rrddim_set_by_pointer(st_mhits, rd_mhits,   mhit);
        rrddim_set_by_pointer(st_mhits, rd_mmisses, mmiss);
        rrdset_done(st_mhits);
    }

    // --------------------------------------------------------------------

    if(likely(arcstats.l2exist && (do_l2hits == CONFIG_BOOLEAN_YES || l2hit || l2miss))) {
        do_l2hits = CONFIG_BOOLEAN_YES;

        static RRDSET *st_l2hits = NULL;
        static RRDDIM *rd_l2hits = NULL;
        static RRDDIM *rd_l2misses = NULL;

        if (unlikely(!st_l2hits)) {
            st_l2hits = rrdset_create_localhost(
                    "zfs"
                    , "l2hits"
                    , NULL
                    , ZFS_FAMILY_EFFICIENCY
                    , NULL
                    , "ZFS L2 Hits"
                    , "percentage"
                    , plugin
                    , module
                    , NETDATA_CHART_PRIO_ZFS_L2HITS
                    , update_every
                    , RRDSET_TYPE_STACKED
            );

            rd_l2hits   = rrddim_add(st_l2hits, "hits",   NULL, 1, 1, RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL);
            rd_l2misses = rrddim_add(st_l2hits, "misses", NULL, 1, 1, RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL);
        }
        else
            rrdset_next(st_l2hits);

        rrddim_set_by_pointer(st_l2hits, rd_l2hits,   l2hit);
        rrddim_set_by_pointer(st_l2hits, rd_l2misses, l2miss);
        rrdset_done(st_l2hits);
    }

    // --------------------------------------------------------------------

    if(likely(do_list_hits == CONFIG_BOOLEAN_YES || arcstats.mfu_hits \
                                                 || arcstats.mru_hits \
                                                 || arcstats.mfu_ghost_hits \
                                                 || arcstats.mru_ghost_hits)) {
        do_list_hits = CONFIG_BOOLEAN_YES;

        static RRDSET *st_list_hits = NULL;
        static RRDDIM *rd_mfu = NULL;
        static RRDDIM *rd_mru = NULL;
        static RRDDIM *rd_mfug = NULL;
        static RRDDIM *rd_mrug = NULL;

        if (unlikely(!st_list_hits)) {
            st_list_hits = rrdset_create_localhost(
                    "zfs"
                    , "list_hits"
                    , NULL
                    , ZFS_FAMILY_EFFICIENCY
                    , NULL
                    , "ZFS List Hits"
                    , "hits/s"
                    , plugin
                    , module
                    , NETDATA_CHART_PRIO_ZFS_LIST_HITS
                    , update_every
                    , RRDSET_TYPE_AREA
            );

            rd_mfu = rrddim_add(st_list_hits,  "mfu",  NULL,        1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_mfug = rrddim_add(st_list_hits, "mfug", "mfu ghost", 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_mru = rrddim_add(st_list_hits,  "mru",  NULL,        1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_mrug = rrddim_add(st_list_hits, "mrug", "mru ghost", 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        else
            rrdset_next(st_list_hits);

        rrddim_set_by_pointer(st_list_hits, rd_mfu, arcstats.mfu_hits);
        rrddim_set_by_pointer(st_list_hits, rd_mru, arcstats.mru_hits);
        rrddim_set_by_pointer(st_list_hits, rd_mfug, arcstats.mfu_ghost_hits);
        rrddim_set_by_pointer(st_list_hits, rd_mrug, arcstats.mru_ghost_hits);
        rrdset_done(st_list_hits);
    }
}

void generate_charts_arc_summary(const char *plugin, const char *module, int show_zero_charts, int update_every) {
    static int do_arc_size_breakdown = -1, do_memory = -1, do_important_ops = -1, do_actual_hits = -1, \
               do_demand_data_hits = -1, do_prefetch_data_hits = -1, do_hash_elements = -1, do_hash_chains = -1;

    if(unlikely(do_arc_size_breakdown == -1))
        do_arc_size_breakdown = do_memory = do_important_ops = do_actual_hits = do_demand_data_hits \
        = do_prefetch_data_hits = do_hash_elements = do_hash_chains = show_zero_charts;

    unsigned long long arc_accesses_total = arcstats.hits + arcstats.misses;
    unsigned long long real_hits = arcstats.mfu_hits + arcstats.mru_hits;
    unsigned long long real_misses = arc_accesses_total - real_hits;

    //unsigned long long anon_hits = arcstats.hits - (arcstats.mfu_hits + arcstats.mru_hits + arcstats.mfu_ghost_hits + arcstats.mru_ghost_hits);

    unsigned long long arc_size = arcstats.size;
    unsigned long long mru_size = arcstats.p;
    //unsigned long long target_min_size = arcstats.c_min;
    //unsigned long long target_max_size = arcstats.c_max;
    unsigned long long target_size = arcstats.c;
    //unsigned long long target_size_ratio = (target_max_size / target_min_size);

    unsigned long long mfu_size;
    if(arc_size > target_size)
        mfu_size = arc_size - mru_size;
    else
        mfu_size = target_size - mru_size;

    // --------------------------------------------------------------------

    if(likely(do_arc_size_breakdown == CONFIG_BOOLEAN_YES || mru_size || mfu_size)) {
        do_arc_size_breakdown = CONFIG_BOOLEAN_YES;

        static RRDSET *st_arc_size_breakdown = NULL;
        static RRDDIM *rd_most_recent = NULL;
        static RRDDIM *rd_most_frequent = NULL;

        if (unlikely(!st_arc_size_breakdown)) {
            st_arc_size_breakdown = rrdset_create_localhost(
                    "zfs"
                    , "arc_size_breakdown"
                    , NULL
                    , ZFS_FAMILY_EFFICIENCY
                    , NULL
                    , "ZFS ARC Size Breakdown"
                    , "percentage"
                    , plugin
                    , module
                    , NETDATA_CHART_PRIO_ZFS_ARC_SIZE_BREAKDOWN
                    , update_every
                    , RRDSET_TYPE_STACKED
            );

            rd_most_recent   = rrddim_add(st_arc_size_breakdown, "recent", NULL,   1, 1, RRD_ALGORITHM_PCENT_OVER_ROW_TOTAL);
            rd_most_frequent = rrddim_add(st_arc_size_breakdown, "frequent", NULL, 1, 1, RRD_ALGORITHM_PCENT_OVER_ROW_TOTAL);
        }
        else
            rrdset_next(st_arc_size_breakdown);

        rrddim_set_by_pointer(st_arc_size_breakdown, rd_most_recent,   mru_size);
        rrddim_set_by_pointer(st_arc_size_breakdown, rd_most_frequent, mfu_size);
        rrdset_done(st_arc_size_breakdown);
    }

    // --------------------------------------------------------------------

    if(likely(do_memory == CONFIG_BOOLEAN_YES || arcstats.memory_direct_count \
                                              || arcstats.memory_throttle_count \
                                              || arcstats.memory_indirect_count)) {
        do_memory = CONFIG_BOOLEAN_YES;

        static RRDSET *st_memory = NULL;
#ifndef __FreeBSD__
        static RRDDIM *rd_direct = NULL;
#endif
        static RRDDIM *rd_throttled = NULL;
#ifndef __FreeBSD__
        static RRDDIM *rd_indirect = NULL;
#endif

        if (unlikely(!st_memory)) {
            st_memory = rrdset_create_localhost(
                    "zfs"
                    , "memory_ops"
                    , NULL
                    , ZFS_FAMILY_OPERATIONS
                    , NULL
                    , "ZFS Memory Operations"
                    , "operations/s"
                    , plugin
                    , module
                    , NETDATA_CHART_PRIO_ZFS_MEMORY_OPS
                    , update_every
                    , RRDSET_TYPE_LINE
            );

#ifndef __FreeBSD__
            rd_direct    = rrddim_add(st_memory, "direct",    NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
#endif
            rd_throttled = rrddim_add(st_memory, "throttled", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
#ifndef __FreeBSD__
            rd_indirect  = rrddim_add(st_memory, "indirect",  NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
#endif
        }
        else
            rrdset_next(st_memory);

#ifndef __FreeBSD__
        rrddim_set_by_pointer(st_memory, rd_direct,    arcstats.memory_direct_count);
#endif
        rrddim_set_by_pointer(st_memory, rd_throttled, arcstats.memory_throttle_count);
#ifndef __FreeBSD__
        rrddim_set_by_pointer(st_memory, rd_indirect,  arcstats.memory_indirect_count);
#endif
        rrdset_done(st_memory);
    }

    // --------------------------------------------------------------------

    if(likely(do_important_ops == CONFIG_BOOLEAN_YES || arcstats.deleted \
                                                     || arcstats.evict_skip \
                                                     || arcstats.mutex_miss \
                                                     || arcstats.hash_collisions)) {
        do_important_ops = CONFIG_BOOLEAN_YES;

        static RRDSET *st_important_ops = NULL;
        static RRDDIM *rd_deleted = NULL;
        static RRDDIM *rd_mutex_misses = NULL;
        static RRDDIM *rd_evict_skips = NULL;
        static RRDDIM *rd_hash_collisions = NULL;

        if (unlikely(!st_important_ops)) {
            st_important_ops = rrdset_create_localhost(
                    "zfs"
                    , "important_ops"
                    , NULL
                    , ZFS_FAMILY_OPERATIONS
                    , NULL
                    , "ZFS Important Operations"
                    , "operations/s"
                    , plugin
                    , module
                    , NETDATA_CHART_PRIO_ZFS_IMPORTANT_OPS
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_evict_skips     = rrddim_add(st_important_ops, "eskip",   "evict skip", 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_deleted         = rrddim_add(st_important_ops, "deleted", NULL,         1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_mutex_misses    = rrddim_add(st_important_ops, "mtxmis",  "mutex miss", 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_hash_collisions = rrddim_add(st_important_ops, "hash_collisions", "hash collisions", 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        else
            rrdset_next(st_important_ops);

        rrddim_set_by_pointer(st_important_ops, rd_deleted,      arcstats.deleted);
        rrddim_set_by_pointer(st_important_ops, rd_evict_skips,  arcstats.evict_skip);
        rrddim_set_by_pointer(st_important_ops, rd_mutex_misses, arcstats.mutex_miss);
        rrddim_set_by_pointer(st_important_ops, rd_hash_collisions, arcstats.hash_collisions);
        rrdset_done(st_important_ops);
    }

    // --------------------------------------------------------------------

    if(likely(do_actual_hits == CONFIG_BOOLEAN_YES || real_hits || real_misses)) {
        do_actual_hits = CONFIG_BOOLEAN_YES;

        static RRDSET *st_actual_hits = NULL;
        static RRDDIM *rd_actual_hits = NULL;
        static RRDDIM *rd_actual_misses = NULL;

        if (unlikely(!st_actual_hits)) {
            st_actual_hits = rrdset_create_localhost(
                    "zfs"
                    , "actual_hits"
                    , NULL
                    , ZFS_FAMILY_EFFICIENCY
                    , NULL
                    , "ZFS Actual Cache Hits"
                    , "percentage"
                    , plugin
                    , module
                    , NETDATA_CHART_PRIO_ZFS_ACTUAL_HITS
                    , update_every
                    , RRDSET_TYPE_STACKED
            );

            rd_actual_hits   = rrddim_add(st_actual_hits, "hits", NULL,   1, 1, RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL);
            rd_actual_misses = rrddim_add(st_actual_hits, "misses", NULL, 1, 1, RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL);
        }
        else
            rrdset_next(st_actual_hits);

        rrddim_set_by_pointer(st_actual_hits, rd_actual_hits,   real_hits);
        rrddim_set_by_pointer(st_actual_hits, rd_actual_misses, real_misses);
        rrdset_done(st_actual_hits);
    }

    // --------------------------------------------------------------------

    if(likely(do_demand_data_hits == CONFIG_BOOLEAN_YES || arcstats.demand_data_hits || arcstats.demand_data_misses)) {
        do_demand_data_hits = CONFIG_BOOLEAN_YES;

        static RRDSET *st_demand_data_hits = NULL;
        static RRDDIM *rd_demand_data_hits = NULL;
        static RRDDIM *rd_demand_data_misses = NULL;

        if (unlikely(!st_demand_data_hits)) {
            st_demand_data_hits = rrdset_create_localhost(
                    "zfs"
                    , "demand_data_hits"
                    , NULL
                    , ZFS_FAMILY_EFFICIENCY
                    , NULL
                    , "ZFS Data Demand Efficiency"
                    , "percentage"
                    , plugin
                    , module
                    , NETDATA_CHART_PRIO_ZFS_DEMAND_DATA_HITS
                    , update_every
                    , RRDSET_TYPE_STACKED
            );

            rd_demand_data_hits   = rrddim_add(st_demand_data_hits, "hits", NULL,   1, 1, RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL);
            rd_demand_data_misses = rrddim_add(st_demand_data_hits, "misses", NULL, 1, 1, RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL);
        }
        else
            rrdset_next(st_demand_data_hits);

        rrddim_set_by_pointer(st_demand_data_hits, rd_demand_data_hits,   arcstats.demand_data_hits);
        rrddim_set_by_pointer(st_demand_data_hits, rd_demand_data_misses, arcstats.demand_data_misses);
        rrdset_done(st_demand_data_hits);
    }

    // --------------------------------------------------------------------

    if(likely(do_prefetch_data_hits == CONFIG_BOOLEAN_YES || arcstats.prefetch_data_hits \
                                                          || arcstats.prefetch_data_misses)) {
        do_prefetch_data_hits = CONFIG_BOOLEAN_YES;

        static RRDSET *st_prefetch_data_hits = NULL;
        static RRDDIM *rd_prefetch_data_hits = NULL;
        static RRDDIM *rd_prefetch_data_misses = NULL;

        if (unlikely(!st_prefetch_data_hits)) {
            st_prefetch_data_hits = rrdset_create_localhost(
                    "zfs"
                    , "prefetch_data_hits"
                    , NULL
                    , ZFS_FAMILY_EFFICIENCY
                    , NULL
                    , "ZFS Data Prefetch Efficiency"
                    , "percentage"
                    , plugin
                    , module
                    , NETDATA_CHART_PRIO_ZFS_PREFETCH_DATA_HITS
                    , update_every
                    , RRDSET_TYPE_STACKED
            );

            rd_prefetch_data_hits   = rrddim_add(st_prefetch_data_hits, "hits", NULL,   1, 1, RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL);
            rd_prefetch_data_misses = rrddim_add(st_prefetch_data_hits, "misses", NULL, 1, 1, RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL);
        }
        else
            rrdset_next(st_prefetch_data_hits);

        rrddim_set_by_pointer(st_prefetch_data_hits, rd_prefetch_data_hits,   arcstats.prefetch_data_hits);
        rrddim_set_by_pointer(st_prefetch_data_hits, rd_prefetch_data_misses, arcstats.prefetch_data_misses);
        rrdset_done(st_prefetch_data_hits);
    }

    // --------------------------------------------------------------------

    if(likely(do_hash_elements == CONFIG_BOOLEAN_YES || arcstats.hash_elements || arcstats.hash_elements_max)) {
        do_hash_elements = CONFIG_BOOLEAN_YES;

        static RRDSET *st_hash_elements = NULL;
        static RRDDIM *rd_hash_elements_current = NULL;
        static RRDDIM *rd_hash_elements_max = NULL;

        if (unlikely(!st_hash_elements)) {
            st_hash_elements = rrdset_create_localhost(
                    "zfs"
                    , "hash_elements"
                    , NULL
                    , ZFS_FAMILY_HASH
                    , NULL
                    , "ZFS ARC Hash Elements"
                    , "elements"
                    , plugin
                    , module
                    , NETDATA_CHART_PRIO_ZFS_HASH_ELEMENTS
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_hash_elements_current = rrddim_add(st_hash_elements, "current", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_hash_elements_max     = rrddim_add(st_hash_elements, "max",     NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        }
        else
            rrdset_next(st_hash_elements);

        rrddim_set_by_pointer(st_hash_elements, rd_hash_elements_current, arcstats.hash_elements);
        rrddim_set_by_pointer(st_hash_elements, rd_hash_elements_max, arcstats.hash_elements_max);
        rrdset_done(st_hash_elements);
    }

    // --------------------------------------------------------------------

    if(likely(do_hash_chains == CONFIG_BOOLEAN_YES || arcstats.hash_chains || arcstats.hash_chain_max)) {
        do_hash_chains = CONFIG_BOOLEAN_YES;

        static RRDSET *st_hash_chains = NULL;
        static RRDDIM *rd_hash_chains_current = NULL;
        static RRDDIM *rd_hash_chains_max = NULL;

        if (unlikely(!st_hash_chains)) {
            st_hash_chains = rrdset_create_localhost(
                    "zfs"
                    , "hash_chains"
                    , NULL
                    , ZFS_FAMILY_HASH
                    , NULL
                    , "ZFS ARC Hash Chains"
                    , "chains"
                    , plugin
                    , module
                    , NETDATA_CHART_PRIO_ZFS_HASH_CHAINS
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_hash_chains_current = rrddim_add(st_hash_chains, "current", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_hash_chains_max     = rrddim_add(st_hash_chains, "max",     NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        }
        else
            rrdset_next(st_hash_chains);

        rrddim_set_by_pointer(st_hash_chains, rd_hash_chains_current, arcstats.hash_chains);
        rrddim_set_by_pointer(st_hash_chains, rd_hash_chains_max, arcstats.hash_chain_max);
        rrdset_done(st_hash_chains);
    }

    // --------------------------------------------------------------------

}