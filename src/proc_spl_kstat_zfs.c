// SPDX-License-Identifier: GPL-3.0+

#include "common.h"
#include "zfs_common.h"

#define ZFS_PROC_ARCSTATS "/proc/spl/kstat/zfs/arcstats"

extern struct arcstats arcstats;

int do_proc_spl_kstat_zfs_arcstats(int update_every, usec_t dt) {
    (void)dt;

    static procfile *ff = NULL;
    static ARL_BASE *arl_base = NULL;

    arcstats.l2exist = -1;

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

        if(unlikely(arcstats.l2exist == -1)) {
            if(key[0] == 'l' && key[1] == '2' && key[2] == '_')
                arcstats.l2exist = 1;
        }

        if(unlikely(arl_check(arl_base, key, value))) break;
    }

    if(unlikely(arcstats.l2exist == -1))
        arcstats.l2exist = 0;

    generate_charts_arcstats("proc", update_every);
    generate_charts_arc_summary("proc", update_every);

    return 0;
}
