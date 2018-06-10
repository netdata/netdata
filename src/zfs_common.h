// SPDX-License-Identifier: GPL-3.0+
#ifndef NETDATA_ZFS_COMMON_H
#define NETDATA_ZFS_COMMON_H

#define ZFS_FAMILY_SIZE "size"
#define ZFS_FAMILY_EFFICIENCY "efficiency"
#define ZFS_FAMILY_ACCESSES "accesses"
#define ZFS_FAMILY_OPERATIONS "operations"
#define ZFS_FAMILY_HASH "hashes"

struct arcstats {
    // values
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

    // flags
    int l2exist;
};

void generate_charts_arcstats(const char *plugin, int update_every);
void generate_charts_arc_summary(const char *plugin, int update_every);

#endif //NETDATA_ZFS_COMMON_H
