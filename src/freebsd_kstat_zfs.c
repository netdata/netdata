// SPDX-License-Identifier: GPL-3.0+
#include "common.h"
#include "zfs_common.h"

extern struct arcstats arcstats;

// --------------------------------------------------------------------------------------------------------------------
// kstat.zfs.misc.arcstats

int do_kstat_zfs_misc_arcstats(int update_every, usec_t dt) {
    (void)dt;

    unsigned long long l2_size;
    size_t uint64_t_size = sizeof(uint64_t);
    static struct mibs {
        int hits[5];
        int misses[5];
        int demand_data_hits[5];
        int demand_data_misses[5];
        int demand_metadata_hits[5];
        int demand_metadata_misses[5];
        int prefetch_data_hits[5];
        int prefetch_data_misses[5];
        int prefetch_metadata_hits[5];
        int prefetch_metadata_misses[5];
        int mru_hits[5];
        int mru_ghost_hits[5];
        int mfu_hits[5];
        int mfu_ghost_hits[5];
        int deleted[5];
        int mutex_miss[5];
        int evict_skip[5];
        int evict_not_enough[5];
        int evict_l2_cached[5];
        int evict_l2_eligible[5];
        int evict_l2_ineligible[5];
        int evict_l2_skip[5];
        int hash_elements[5];
        int hash_elements_max[5];
        int hash_collisions[5];
        int hash_chains[5];
        int hash_chain_max[5];
        int p[5];
        int c[5];
        int c_min[5];
        int c_max[5];
        int size[5];
        int hdr_size[5];
        int data_size[5];
        int metadata_size[5];
        int other_size[5];
        int anon_size[5];
        int anon_evictable_data[5];
        int anon_evictable_metadata[5];
        int mru_size[5];
        int mru_evictable_data[5];
        int mru_evictable_metadata[5];
        int mru_ghost_size[5];
        int mru_ghost_evictable_data[5];
        int mru_ghost_evictable_metadata[5];
        int mfu_size[5];
        int mfu_evictable_data[5];
        int mfu_evictable_metadata[5];
        int mfu_ghost_size[5];
        int mfu_ghost_evictable_data[5];
        int mfu_ghost_evictable_metadata[5];
        int l2_hits[5];
        int l2_misses[5];
        int l2_feeds[5];
        int l2_rw_clash[5];
        int l2_read_bytes[5];
        int l2_write_bytes[5];
        int l2_writes_sent[5];
        int l2_writes_done[5];
        int l2_writes_error[5];
        int l2_writes_lock_retry[5];
        int l2_evict_lock_retry[5];
        int l2_evict_reading[5];
        int l2_evict_l1cached[5];
        int l2_free_on_write[5];
        int l2_cdata_free_on_write[5];
        int l2_abort_lowmem[5];
        int l2_cksum_bad[5];
        int l2_io_error[5];
        int l2_size[5];
        int l2_asize[5];
        int l2_hdr_size[5];
        int l2_compress_successes[5];
        int l2_compress_zeros[5];
        int l2_compress_failures[5];
        int memory_throttle_count[5];
        int duplicate_buffers[5];
        int duplicate_buffers_size[5];
        int duplicate_reads[5];
        int memory_direct_count[5];
        int memory_indirect_count[5];
        int arc_no_grow[5];
        int arc_tempreserve[5];
        int arc_loaned_bytes[5];
        int arc_prune[5];
        int arc_meta_used[5];
        int arc_meta_limit[5];
        int arc_meta_max[5];
        int arc_meta_min[5];
        int arc_need_free[5];
        int arc_sys_free[5];
    } mibs;

    arcstats.l2exist = -1;

    if(unlikely(sysctlbyname("kstat.zfs.misc.arcstats.l2_size", &l2_size, &uint64_t_size, NULL, 0)))
        return 0;

    if(likely(l2_size))
        arcstats.l2exist = 1;
    else
        arcstats.l2exist = 0;

    GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.hits", mibs.hits, arcstats.hits);
    GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.misses", mibs.misses, arcstats.misses);
    GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.demand_data_hits", mibs.demand_data_hits, arcstats.demand_data_hits);
    GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.demand_data_misses", mibs.demand_data_misses, arcstats.demand_data_misses);
    GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.demand_metadata_hits", mibs.demand_metadata_hits, arcstats.demand_metadata_hits);
    GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.demand_metadata_misses", mibs.demand_metadata_misses, arcstats.demand_metadata_misses);
    GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.prefetch_data_hits", mibs.prefetch_data_hits, arcstats.prefetch_data_hits);
    GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.prefetch_data_misses", mibs.prefetch_data_misses, arcstats.prefetch_data_misses);
    GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.prefetch_metadata_hits", mibs.prefetch_metadata_hits, arcstats.prefetch_metadata_hits);
    GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.prefetch_metadata_misses", mibs.prefetch_metadata_misses, arcstats.prefetch_metadata_misses);
    GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.mru_hits", mibs.mru_hits, arcstats.mru_hits);
    GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.mru_ghost_hits", mibs.mru_ghost_hits, arcstats.mru_ghost_hits);
    GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.mfu_hits", mibs.mfu_hits, arcstats.mfu_hits);
    GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.mfu_ghost_hits", mibs.mfu_ghost_hits, arcstats.mfu_ghost_hits);
    GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.deleted", mibs.deleted, arcstats.deleted);
    GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.mutex_miss", mibs.mutex_miss, arcstats.mutex_miss);
    GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.evict_skip", mibs.evict_skip, arcstats.evict_skip);
    // not used: GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.evict_not_enough", mibs.evict_not_enough, arcstats.evict_not_enough);
    // not used: GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.evict_l2_cached", mibs.evict_l2_cached, arcstats.evict_l2_cached);
    // not used: GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.evict_l2_eligible", mibs.evict_l2_eligible, arcstats.evict_l2_eligible);
    // not used: GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.evict_l2_ineligible", mibs.evict_l2_ineligible, arcstats.evict_l2_ineligible);
    // not used: GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.evict_l2_skip", mibs.evict_l2_skip, arcstats.evict_l2_skip);
    GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.hash_elements", mibs.hash_elements, arcstats.hash_elements);
    GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.hash_elements_max", mibs.hash_elements_max, arcstats.hash_elements_max);
    GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.hash_collisions", mibs.hash_collisions, arcstats.hash_collisions);
    GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.hash_chains", mibs.hash_chains, arcstats.hash_chains);
    GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.hash_chain_max", mibs.hash_chain_max, arcstats.hash_chain_max);
    GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.p", mibs.p, arcstats.p);
    GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.c", mibs.c, arcstats.c);
    GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.c_min", mibs.c_min, arcstats.c_min);
    GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.c_max", mibs.c_max, arcstats.c_max);
    GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.size", mibs.size, arcstats.size);
    // not used: GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.hdr_size", mibs.hdr_size, arcstats.hdr_size);
    // not used: GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.data_size", mibs.data_size, arcstats.data_size);
    // not used: GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.metadata_size", mibs.metadata_size, arcstats.metadata_size);
    // not used: GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.other_size", mibs.other_size, arcstats.other_size);
    // not used: GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.anon_size", mibs.anon_size, arcstats.anon_size);
    // not used: GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.anon_evictable_data", mibs.anon_evictable_data, arcstats.anon_evictable_data);
    // not used: GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.anon_evictable_metadata", mibs.anon_evictable_metadata, arcstats.anon_evictable_metadata);
    GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.mru_size", mibs.mru_size, arcstats.mru_size);
    // not used: GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.mru_evictable_data", mibs.mru_evictable_data, arcstats.mru_evictable_data);
    // not used: GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.mru_evictable_metadata", mibs.mru_evictable_metadata, arcstats.mru_evictable_metadata);
    // not used: GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.mru_ghost_size", mibs.mru_ghost_size, arcstats.mru_ghost_size);
    // not used: GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.mru_ghost_evictable_data", mibs.mru_ghost_evictable_data, arcstats.mru_ghost_evictable_data);
    // not used: GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.mru_ghost_evictable_metadata", mibs.mru_ghost_evictable_metadata, arcstats.mru_ghost_evictable_metadata);
    GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.mfu_size", mibs.mfu_size, arcstats.mfu_size);
    // not used: GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.mfu_evictable_data", mibs.mfu_evictable_data, arcstats.mfu_evictable_data);
    // not used: GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.mfu_evictable_metadata", mibs.mfu_evictable_metadata, arcstats.mfu_evictable_metadata);
    // not used: GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.mfu_ghost_size", mibs.mfu_ghost_size, arcstats.mfu_ghost_size);
    // not used: GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.mfu_ghost_evictable_data", mibs.mfu_ghost_evictable_data, arcstats.mfu_ghost_evictable_data);
    // not used: GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.mfu_ghost_evictable_metadata", mibs.mfu_ghost_evictable_metadata, arcstats.mfu_ghost_evictable_metadata);
    GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.l2_hits", mibs.l2_hits, arcstats.l2_hits);
    GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.l2_misses", mibs.l2_misses, arcstats.l2_misses);
    // not used: GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.l2_feeds", mibs.l2_feeds, arcstats.l2_feeds);
    // not used: GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.l2_rw_clash", mibs.l2_rw_clash, arcstats.l2_rw_clash);
    GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.l2_read_bytes", mibs.l2_read_bytes, arcstats.l2_read_bytes);
    GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.l2_write_bytes", mibs.l2_write_bytes, arcstats.l2_write_bytes);
    // not used: GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.l2_writes_sent", mibs.l2_writes_sent, arcstats.l2_writes_sent);
    // not used: GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.l2_writes_done", mibs.l2_writes_done, arcstats.l2_writes_done);
    // not used: GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.l2_writes_error", mibs.l2_writes_error, arcstats.l2_writes_error);
    // not used: GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.l2_writes_lock_retry", mibs.l2_writes_lock_retry, arcstats.l2_writes_lock_retry);
    // not used: GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.l2_evict_lock_retry", mibs.l2_evict_lock_retry, arcstats.l2_evict_lock_retry);
    // not used: GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.l2_evict_reading", mibs.l2_evict_reading, arcstats.l2_evict_reading);
    // not used: GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.l2_evict_l1cached", mibs.l2_evict_l1cached, arcstats.l2_evict_l1cached);
    // not used: GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.l2_free_on_write", mibs.l2_free_on_write, arcstats.l2_free_on_write);
    // missing mib: GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.l2_cdata_free_on_write", mibs.l2_cdata_free_on_write, arcstats.l2_cdata_free_on_write);
    // not used: GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.l2_abort_lowmem", mibs.l2_abort_lowmem, arcstats.l2_abort_lowmem);
    // not used: GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.l2_cksum_bad", mibs.l2_cksum_bad, arcstats.l2_cksum_bad);
    // not used: GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.l2_io_error", mibs.l2_io_error, arcstats.l2_io_error);
    GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.l2_size", mibs.l2_size, arcstats.l2_size);
    GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.l2_asize", mibs.l2_asize, arcstats.l2_asize);
    // not used: GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.l2_hdr_size", mibs.l2_hdr_size, arcstats.l2_hdr_size);
    // missing mib: GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.l2_compress_successes", mibs.l2_compress_successes, arcstats.l2_compress_successes);
    // missing mib: GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.l2_compress_zeros", mibs.l2_compress_zeros, arcstats.l2_compress_zeros);
    // missing mib: GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.l2_compress_failures", mibs.l2_compress_failures, arcstats.l2_compress_failures);
    GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.memory_throttle_count", mibs.memory_throttle_count, arcstats.memory_throttle_count);
    // missing mib: GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.duplicate_buffers", mibs.duplicate_buffers, arcstats.duplicate_buffers);
    // missing mib: GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.duplicate_buffers_size", mibs.duplicate_buffers_size, arcstats.duplicate_buffers_size);
    // missing mib: GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.duplicate_reads", mibs.duplicate_reads, arcstats.duplicate_reads);
    // missing mib: GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.memory_direct_count", mibs.memory_direct_count, arcstats.memory_direct_count);
    // missing mib: GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.memory_indirect_count", mibs.memory_indirect_count, arcstats.memory_indirect_count);
    // missing mib: GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.arc_no_grow", mibs.arc_no_grow, arcstats.arc_no_grow);
    // missing mib: GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.arc_tempreserve", mibs.arc_tempreserve, arcstats.arc_tempreserve);
    // missing mib: GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.arc_loaned_bytes", mibs.arc_loaned_bytes, arcstats.arc_loaned_bytes);
    // missing mib: GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.arc_prune", mibs.arc_prune, arcstats.arc_prune);
    // not used: GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.arc_meta_used", mibs.arc_meta_used, arcstats.arc_meta_used);
    // not used: GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.arc_meta_limit", mibs.arc_meta_limit, arcstats.arc_meta_limit);
    // not used: GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.arc_meta_max", mibs.arc_meta_max, arcstats.arc_meta_max);
    // not used: GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.arc_meta_min", mibs.arc_meta_min, arcstats.arc_meta_min);
    // missing mib: GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.arc_need_free", mibs.arc_need_free, arcstats.arc_need_free);
    // missing mib: GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.arc_sys_free", mibs.arc_sys_free, arcstats.arc_sys_free);

    generate_charts_arcstats("freebsd", update_every);
    generate_charts_arc_summary("freebsd", update_every);

    return 0;
}

// --------------------------------------------------------------------------------------------------------------------
// kstat.zfs.misc.zio_trim

int do_kstat_zfs_misc_zio_trim(int update_every, usec_t dt) {
    (void)dt;
    static int mib_bytes[5] = {0, 0, 0, 0, 0}, mib_success[5] = {0, 0, 0, 0, 0},
               mib_failed[5] = {0, 0, 0, 0, 0}, mib_unsupported[5] = {0, 0, 0, 0, 0};
    uint64_t bytes, success, failed, unsupported;

    if (unlikely(GETSYSCTL_SIMPLE("kstat.zfs.misc.zio_trim.bytes", mib_bytes, bytes) ||
                 GETSYSCTL_SIMPLE("kstat.zfs.misc.zio_trim.success", mib_success, success) ||
                 GETSYSCTL_SIMPLE("kstat.zfs.misc.zio_trim.failed", mib_failed, failed) ||
                 GETSYSCTL_SIMPLE("kstat.zfs.misc.zio_trim.unsupported", mib_unsupported, unsupported))) {
        error("DISABLED: zfs.trim_bytes chart");
        error("DISABLED: zfs.trim_success chart");
        error("DISABLED: kstat.zfs.misc.zio_trim module");
        return 1;
     } else {

     // --------------------------------------------------------------------

        static RRDSET *st_bytes = NULL;
        static RRDDIM *rd_bytes = NULL;

        if (unlikely(!st_bytes)) {
            st_bytes = rrdset_create_localhost(
                    "zfs",
                    "trim_bytes",
                    NULL,
                    "trim",
                    NULL,
                    "Successfully TRIMmed bytes",
                    "bytes",
                    "freebsd",
                    "zfs",
                    2320,
                    update_every,
                    RRDSET_TYPE_LINE
            );

            rd_bytes = rrddim_add(st_bytes, "TRIMmed", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        else rrdset_next(st_bytes);

        rrddim_set_by_pointer(st_bytes, rd_bytes, bytes);
        rrdset_done(st_bytes);
        
        // --------------------------------------------------------------------

        static RRDSET *st_requests = NULL;
        static RRDDIM *rd_successful = NULL, *rd_failed = NULL, *rd_unsupported = NULL;

        if (unlikely(!st_requests)) {
            st_requests = rrdset_create_localhost(
                    "zfs",
                    "trim_requests",
                    NULL,
                    "trim",
                    NULL,
                    "TRIM requests",
                    "requests",
                    "freebsd",
                    "zfs",
                    2321,
                    update_every,
                    RRDSET_TYPE_STACKED
            );

            rd_successful  = rrddim_add(st_requests, "successful",  NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_failed      = rrddim_add(st_requests, "failed",      NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_unsupported = rrddim_add(st_requests, "unsupported", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        else rrdset_next(st_requests);

        rrddim_set_by_pointer(st_requests, rd_successful,  success);
        rrddim_set_by_pointer(st_requests, rd_failed,      failed);
        rrddim_set_by_pointer(st_requests, rd_unsupported, unsupported);
        rrdset_done(st_requests);
        
     }

    return 0;
}