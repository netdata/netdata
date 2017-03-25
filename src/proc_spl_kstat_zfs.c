#include "common.h"

#define ZFS_PROC_ARCSTATS "/proc/spl/kstat/zfs/arcstats"
#define ZFS_FAMILY_SIZE "size"
#define ZFS_FAMILY_EFFICIENCY "efficiency"
#define ZFS_FAMILY_ACCESSES "accesses"
#define ZFS_FAMILY_OPERATIONS "operations"
#define ZFS_FAMILY_HASH "hashes"

static struct arcstats {
    unsigned long long hits;
    unsigned long long misses;
    unsigned long long demand_data_hits;
    unsigned long long demand_data_misses;
    unsigned long long demand_metadata_hits;
    unsigned long long demand_metadata_misses;
    unsigned long long prefetch_data_hits;
    unsigned long long prefetch_data_misses;
    unsigned long long prefetch_metadata_hits;
    unsigned long long prefetch_metadata_misses;
    unsigned long long mru_hits;
    unsigned long long mru_ghost_hits;
    unsigned long long mfu_hits;
    unsigned long long mfu_ghost_hits;
    unsigned long long deleted;
    unsigned long long mutex_miss;
    unsigned long long evict_skip;
    unsigned long long evict_not_enough;
    unsigned long long evict_l2_cached;
    unsigned long long evict_l2_eligible;
    unsigned long long evict_l2_ineligible;
    unsigned long long evict_l2_skip;
    unsigned long long hash_elements;
    unsigned long long hash_elements_max;
    unsigned long long hash_collisions;
    unsigned long long hash_chains;
    unsigned long long hash_chain_max;
    unsigned long long p;
    unsigned long long c;
    unsigned long long c_min;
    unsigned long long c_max;
    unsigned long long size;
    unsigned long long hdr_size;
    unsigned long long data_size;
    unsigned long long metadata_size;
    unsigned long long other_size;
    unsigned long long anon_size;
    unsigned long long anon_evictable_data;
    unsigned long long anon_evictable_metadata;
    unsigned long long mru_size;
    unsigned long long mru_evictable_data;
    unsigned long long mru_evictable_metadata;
    unsigned long long mru_ghost_size;
    unsigned long long mru_ghost_evictable_data;
    unsigned long long mru_ghost_evictable_metadata;
    unsigned long long mfu_size;
    unsigned long long mfu_evictable_data;
    unsigned long long mfu_evictable_metadata;
    unsigned long long mfu_ghost_size;
    unsigned long long mfu_ghost_evictable_data;
    unsigned long long mfu_ghost_evictable_metadata;
    unsigned long long l2_hits;
    unsigned long long l2_misses;
    unsigned long long l2_feeds;
    unsigned long long l2_rw_clash;
    unsigned long long l2_read_bytes;
    unsigned long long l2_write_bytes;
    unsigned long long l2_writes_sent;
    unsigned long long l2_writes_done;
    unsigned long long l2_writes_error;
    unsigned long long l2_writes_lock_retry;
    unsigned long long l2_evict_lock_retry;
    unsigned long long l2_evict_reading;
    unsigned long long l2_evict_l1cached;
    unsigned long long l2_free_on_write;
    unsigned long long l2_cdata_free_on_write;
    unsigned long long l2_abort_lowmem;
    unsigned long long l2_cksum_bad;
    unsigned long long l2_io_error;
    unsigned long long l2_size;
    unsigned long long l2_asize;
    unsigned long long l2_hdr_size;
    unsigned long long l2_compress_successes;
    unsigned long long l2_compress_zeros;
    unsigned long long l2_compress_failures;
    unsigned long long memory_throttle_count;
    unsigned long long duplicate_buffers;
    unsigned long long duplicate_buffers_size;
    unsigned long long duplicate_reads;
    unsigned long long memory_direct_count;
    unsigned long long memory_indirect_count;
    unsigned long long arc_no_grow;
    unsigned long long arc_tempreserve;
    unsigned long long arc_loaned_bytes;
    unsigned long long arc_prune;
    unsigned long long arc_meta_used;
    unsigned long long arc_meta_limit;
    unsigned long long arc_meta_max;
    unsigned long long arc_meta_min;
    unsigned long long arc_need_free;
    unsigned long long arc_sys_free;
} arcstats = { 0 };

int l2exist = -1;

static void generate_charts_arcstats(int update_every) {

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
    unsigned long long l2hit = arcstats.l2_hits + arcstats.l2_misses;
    unsigned long long l2miss = arcstats.prefetch_metadata_misses + arcstats.demand_metadata_misses;
    unsigned long long l2read = l2hit + l2miss;

    // --------------------------------------------------------------------

    {
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
                    , "MB"
                    , 2000
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

    if(likely(l2exist)) {
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
                    , "MB"
                    , 2000
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

    {
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
                    , 2010
                    , update_every
                    , RRDSET_TYPE_AREA
            );

            rd_aread  = rrddim_add(st_reads, "areads",  "arc",      1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_dread  = rrddim_add(st_reads, "dreads",  "demand",   1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_pread  = rrddim_add(st_reads, "preads",  "prefetch", 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_mread  = rrddim_add(st_reads, "mreads",  "metadata", 1, 1, RRD_ALGORITHM_INCREMENTAL);

            if(l2exist)
                rd_l2read = rrddim_add(st_reads, "l2reads", "l2",       1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        else
            rrdset_next(st_reads);

        rrddim_set_by_pointer(st_reads, rd_aread,  aread);
        rrddim_set_by_pointer(st_reads, rd_dread,  dread);
        rrddim_set_by_pointer(st_reads, rd_pread,  pread);
        rrddim_set_by_pointer(st_reads, rd_mread,  mread);

        if(l2exist)
            rrddim_set_by_pointer(st_reads, rd_l2read, l2read);

        rrdset_done(st_reads);
    }

    // --------------------------------------------------------------------

    if(likely(l2exist)) {
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
                    , "kilobytes/s"
                    , 2200
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

    {
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
                    , 2020
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

    {
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
                    , 2030
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

    {
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
                    , 2040
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

    {
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
                    , 2050
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

    if(likely(l2exist)) {
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
                    , 2060
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

    {
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
                    , 2100
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

static void generate_charts_arc_summary(int update_every) {
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

    {
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
                    , 2020
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

    {
        static RRDSET *st_memory = NULL;
        static RRDDIM *rd_direct = NULL;
        static RRDDIM *rd_throttled = NULL;
        static RRDDIM *rd_indirect = NULL;

        if (unlikely(!st_memory)) {
            st_memory = rrdset_create_localhost(
                    "zfs"
                    , "memory_ops"
                    , NULL
                    , ZFS_FAMILY_OPERATIONS
                    , NULL
                    , "ZFS Memory Operations"
                    , "operations/s"
                    , 2023
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_direct    = rrddim_add(st_memory, "direct",    NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_throttled = rrddim_add(st_memory, "throttled", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_indirect  = rrddim_add(st_memory, "indirect",  NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        else
            rrdset_next(st_memory);

        rrddim_set_by_pointer(st_memory, rd_direct,    arcstats.memory_direct_count);
        rrddim_set_by_pointer(st_memory, rd_throttled, arcstats.memory_throttle_count);
        rrddim_set_by_pointer(st_memory, rd_indirect,  arcstats.memory_indirect_count);
        rrdset_done(st_memory);
    }

    // --------------------------------------------------------------------

    {
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
                    , 2022
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

    {
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
                    , 2019
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

    {
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
                    , 2031
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

    {
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
                    , 2032
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

    {
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
                    , 2300
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

    {
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
                    , 2310
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

int do_proc_spl_kstat_zfs_arcstats(int update_every, usec_t dt) {
    (void)dt;

    static procfile *ff = NULL;
    static ARL_BASE *arl_base = NULL;

    if(unlikely(!arl_base)) {
        arl_base = arl_create("arcstats", NULL, 60);

        arl_expect(arl_base, "hits", &arcstats.hits);
        arl_expect(arl_base, "misses", &arcstats.misses);
        arl_expect(arl_base, "demand_data_hits", &arcstats.demand_data_hits);
        arl_expect(arl_base, "demand_data_misses", &arcstats.demand_data_misses);
        arl_expect(arl_base, "demand_metadata_hits", &arcstats.demand_metadata_hits);
        arl_expect(arl_base, "demand_metadata_misses", &arcstats.demand_metadata_misses);
        arl_expect(arl_base, "prefetch_data_hits", &arcstats.prefetch_data_hits);
        arl_expect(arl_base, "prefetch_data_misses", &arcstats.prefetch_data_misses);
        arl_expect(arl_base, "prefetch_metadata_hits", &arcstats.prefetch_metadata_hits);
        arl_expect(arl_base, "prefetch_metadata_misses", &arcstats.prefetch_metadata_misses);
        arl_expect(arl_base, "mru_hits", &arcstats.mru_hits);
        arl_expect(arl_base, "mru_ghost_hits", &arcstats.mru_ghost_hits);
        arl_expect(arl_base, "mfu_hits", &arcstats.mfu_hits);
        arl_expect(arl_base, "mfu_ghost_hits", &arcstats.mfu_ghost_hits);
        arl_expect(arl_base, "deleted", &arcstats.deleted);
        arl_expect(arl_base, "mutex_miss", &arcstats.mutex_miss);
        arl_expect(arl_base, "evict_skip", &arcstats.evict_skip);
        arl_expect(arl_base, "evict_not_enough", &arcstats.evict_not_enough);
        arl_expect(arl_base, "evict_l2_cached", &arcstats.evict_l2_cached);
        arl_expect(arl_base, "evict_l2_eligible", &arcstats.evict_l2_eligible);
        arl_expect(arl_base, "evict_l2_ineligible", &arcstats.evict_l2_ineligible);
        arl_expect(arl_base, "evict_l2_skip", &arcstats.evict_l2_skip);
        arl_expect(arl_base, "hash_elements", &arcstats.hash_elements);
        arl_expect(arl_base, "hash_elements_max", &arcstats.hash_elements_max);
        arl_expect(arl_base, "hash_collisions", &arcstats.hash_collisions);
        arl_expect(arl_base, "hash_chains", &arcstats.hash_chains);
        arl_expect(arl_base, "hash_chain_max", &arcstats.hash_chain_max);
        arl_expect(arl_base, "p", &arcstats.p);
        arl_expect(arl_base, "c", &arcstats.c);
        arl_expect(arl_base, "c_min", &arcstats.c_min);
        arl_expect(arl_base, "c_max", &arcstats.c_max);
        arl_expect(arl_base, "size", &arcstats.size);
        arl_expect(arl_base, "hdr_size", &arcstats.hdr_size);
        arl_expect(arl_base, "data_size", &arcstats.data_size);
        arl_expect(arl_base, "metadata_size", &arcstats.metadata_size);
        arl_expect(arl_base, "other_size", &arcstats.other_size);
        arl_expect(arl_base, "anon_size", &arcstats.anon_size);
        arl_expect(arl_base, "anon_evictable_data", &arcstats.anon_evictable_data);
        arl_expect(arl_base, "anon_evictable_metadata", &arcstats.anon_evictable_metadata);
        arl_expect(arl_base, "mru_size", &arcstats.mru_size);
        arl_expect(arl_base, "mru_evictable_data", &arcstats.mru_evictable_data);
        arl_expect(arl_base, "mru_evictable_metadata", &arcstats.mru_evictable_metadata);
        arl_expect(arl_base, "mru_ghost_size", &arcstats.mru_ghost_size);
        arl_expect(arl_base, "mru_ghost_evictable_data", &arcstats.mru_ghost_evictable_data);
        arl_expect(arl_base, "mru_ghost_evictable_metadata", &arcstats.mru_ghost_evictable_metadata);
        arl_expect(arl_base, "mfu_size", &arcstats.mfu_size);
        arl_expect(arl_base, "mfu_evictable_data", &arcstats.mfu_evictable_data);
        arl_expect(arl_base, "mfu_evictable_metadata", &arcstats.mfu_evictable_metadata);
        arl_expect(arl_base, "mfu_ghost_size", &arcstats.mfu_ghost_size);
        arl_expect(arl_base, "mfu_ghost_evictable_data", &arcstats.mfu_ghost_evictable_data);
        arl_expect(arl_base, "mfu_ghost_evictable_metadata", &arcstats.mfu_ghost_evictable_metadata);
        arl_expect(arl_base, "l2_hits", &arcstats.l2_hits);
        arl_expect(arl_base, "l2_misses", &arcstats.l2_misses);
        arl_expect(arl_base, "l2_feeds", &arcstats.l2_feeds);
        arl_expect(arl_base, "l2_rw_clash", &arcstats.l2_rw_clash);
        arl_expect(arl_base, "l2_read_bytes", &arcstats.l2_read_bytes);
        arl_expect(arl_base, "l2_write_bytes", &arcstats.l2_write_bytes);
        arl_expect(arl_base, "l2_writes_sent", &arcstats.l2_writes_sent);
        arl_expect(arl_base, "l2_writes_done", &arcstats.l2_writes_done);
        arl_expect(arl_base, "l2_writes_error", &arcstats.l2_writes_error);
        arl_expect(arl_base, "l2_writes_lock_retry", &arcstats.l2_writes_lock_retry);
        arl_expect(arl_base, "l2_evict_lock_retry", &arcstats.l2_evict_lock_retry);
        arl_expect(arl_base, "l2_evict_reading", &arcstats.l2_evict_reading);
        arl_expect(arl_base, "l2_evict_l1cached", &arcstats.l2_evict_l1cached);
        arl_expect(arl_base, "l2_free_on_write", &arcstats.l2_free_on_write);
        arl_expect(arl_base, "l2_cdata_free_on_write", &arcstats.l2_cdata_free_on_write);
        arl_expect(arl_base, "l2_abort_lowmem", &arcstats.l2_abort_lowmem);
        arl_expect(arl_base, "l2_cksum_bad", &arcstats.l2_cksum_bad);
        arl_expect(arl_base, "l2_io_error", &arcstats.l2_io_error);
        arl_expect(arl_base, "l2_size", &arcstats.l2_size);
        arl_expect(arl_base, "l2_asize", &arcstats.l2_asize);
        arl_expect(arl_base, "l2_hdr_size", &arcstats.l2_hdr_size);
        arl_expect(arl_base, "l2_compress_successes", &arcstats.l2_compress_successes);
        arl_expect(arl_base, "l2_compress_zeros", &arcstats.l2_compress_zeros);
        arl_expect(arl_base, "l2_compress_failures", &arcstats.l2_compress_failures);
        arl_expect(arl_base, "memory_throttle_count", &arcstats.memory_throttle_count);
        arl_expect(arl_base, "duplicate_buffers", &arcstats.duplicate_buffers);
        arl_expect(arl_base, "duplicate_buffers_size", &arcstats.duplicate_buffers_size);
        arl_expect(arl_base, "duplicate_reads", &arcstats.duplicate_reads);
        arl_expect(arl_base, "memory_direct_count", &arcstats.memory_direct_count);
        arl_expect(arl_base, "memory_indirect_count", &arcstats.memory_indirect_count);
        arl_expect(arl_base, "arc_no_grow", &arcstats.arc_no_grow);
        arl_expect(arl_base, "arc_tempreserve", &arcstats.arc_tempreserve);
        arl_expect(arl_base, "arc_loaned_bytes", &arcstats.arc_loaned_bytes);
        arl_expect(arl_base, "arc_prune", &arcstats.arc_prune);
        arl_expect(arl_base, "arc_meta_used", &arcstats.arc_meta_used);
        arl_expect(arl_base, "arc_meta_limit", &arcstats.arc_meta_limit);
        arl_expect(arl_base, "arc_meta_max", &arcstats.arc_meta_max);
        arl_expect(arl_base, "arc_meta_min", &arcstats.arc_meta_min);
        arl_expect(arl_base, "arc_need_free", &arcstats.arc_need_free);
        arl_expect(arl_base, "arc_sys_free", &arcstats.arc_sys_free);
    }

    if(unlikely(!ff)) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, ZFS_PROC_ARCSTATS);
        ff = procfile_open(config_get("plugin:proc:" ZFS_PROC_ARCSTATS, "filename to monitor", filename), " \t:", PROCFILE_FLAG_DEFAULT);
        if(unlikely(!ff))
            return 1;
    }

    ff = procfile_readall(ff);
    if(unlikely(!ff))
        return 0; // we return 0, so that we will retry to open it next time

    size_t lines = procfile_lines(ff), l;

    arl_begin(arl_base);

    for(l = 0; l < lines ;l++) {
        size_t words = procfile_linewords(ff, l);
        if(unlikely(words < 3)) {
            if(unlikely(words)) error("Cannot read " ZFS_PROC_ARCSTATS " line %zu. Expected 3 params, read %zu.", l, words);
            continue;
        }

        const char *key   = procfile_lineword(ff, l, 0);
        const char *value = procfile_lineword(ff, l, 2);

        if(unlikely(l2exist == -1)) {
            if(key[0] == 'l' && key[1] == '2' && key[2] == '_')
                l2exist = 1;
        }

        if(unlikely(arl_check(arl_base, key, value))) break;
    }

    if(unlikely(l2exist == -1))
        l2exist = 0;

    generate_charts_arcstats(update_every);
    generate_charts_arc_summary(update_every);

    return 0;
}
