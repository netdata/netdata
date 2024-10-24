// SPDX-License-Identifier: GPL-3.0-or-later

#include "plugin_freebsd.h"
#include "collectors/proc.plugin/zfs_common.h"

extern struct arcstats arcstats;

unsigned long long zfs_arcstats_shrinkable_cache_size_bytes = 0;

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
        int hash_elements[5];
        int hash_elements_max[5];
        int hash_collisions[5];
        int hash_chains[5];
        int hash_chain_max[5];
        int p[5];
        int pd[5];
        int pm[5];
        int c[5];
        int c_min[5];
        int c_max[5];
        int size[5];
        int mru_size[5];
        int mfu_size[5];
        int l2_hits[5];
        int l2_misses[5];
        int l2_read_bytes[5];
        int l2_write_bytes[5];
        int l2_size[5];
        int l2_asize[5];
        int memory_throttle_count[5];
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
    GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.hash_elements", mibs.hash_elements, arcstats.hash_elements);
    GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.hash_elements_max", mibs.hash_elements_max, arcstats.hash_elements_max);
    GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.hash_collisions", mibs.hash_collisions, arcstats.hash_collisions);
    GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.hash_chains", mibs.hash_chains, arcstats.hash_chains);
    GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.hash_chain_max", mibs.hash_chain_max, arcstats.hash_chain_max);

#if __FreeBSD_version >= 1400000
    GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.pd", mibs.pd, arcstats.pd);
    GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.pm", mibs.pm, arcstats.pm);
#else
    GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.p", mibs.p, arcstats.p);
#endif

    GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.c", mibs.c, arcstats.c);
    GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.c_min", mibs.c_min, arcstats.c_min);
    GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.c_max", mibs.c_max, arcstats.c_max);
    GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.size", mibs.size, arcstats.size);
    GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.mru_size", mibs.mru_size, arcstats.mru_size);
    GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.mfu_size", mibs.mfu_size, arcstats.mfu_size);
    GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.l2_hits", mibs.l2_hits, arcstats.l2_hits);
    GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.l2_misses", mibs.l2_misses, arcstats.l2_misses);
    GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.l2_read_bytes", mibs.l2_read_bytes, arcstats.l2_read_bytes);
    GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.l2_write_bytes", mibs.l2_write_bytes, arcstats.l2_write_bytes);
    GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.l2_size", mibs.l2_size, arcstats.l2_size);
    GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.l2_asize", mibs.l2_asize, arcstats.l2_asize);
    GETSYSCTL_SIMPLE("kstat.zfs.misc.arcstats.memory_throttle_count", mibs.memory_throttle_count, arcstats.memory_throttle_count);

    if (arcstats.size > arcstats.c_min) {
        zfs_arcstats_shrinkable_cache_size_bytes = arcstats.size - arcstats.c_min;
    } else {
        zfs_arcstats_shrinkable_cache_size_bytes = 0;
    }

    generate_charts_arcstats("freebsd.plugin", "zfs", update_every);
    generate_charts_arc_summary("freebsd.plugin", "zfs", update_every);

    return 0;
}

struct trim_mib_group {
    int bytes_failed[6];
    int bytes_skipped[6];
    int bytes_written[6];
    int extents_failed[6];
    int extents_skipped[6];
    int extents_written[6];
};

struct trim_stats {
    uint64_t bytes_failed;
    uint64_t bytes_skipped;
    uint64_t bytes_written;
    uint64_t extents_failed;
    uint64_t extents_skipped;
    uint64_t extents_written;
};

#define ZFS_TRIM_BASE "kstat.zfs.zroot.misc.iostats"
int do_kstat_zfs_misc_zio_trim(int update_every, usec_t dt) {
    (void)dt;

    static struct {
        struct trim_mib_group atrim;
        struct trim_mib_group trim;
    } mibs;

    struct trim_stats astats = {0}, stats = {0};

    if (GETSYSCTL_SIMPLE(ZFS_TRIM_BASE ".autotrim_bytes_failed", mibs.atrim.bytes_failed, astats.bytes_failed) ||
        GETSYSCTL_SIMPLE(ZFS_TRIM_BASE ".autotrim_bytes_skipped", mibs.atrim.bytes_skipped, astats.bytes_skipped) ||
        GETSYSCTL_SIMPLE(ZFS_TRIM_BASE ".autotrim_bytes_written", mibs.atrim.bytes_written, astats.bytes_written) ||
        GETSYSCTL_SIMPLE(ZFS_TRIM_BASE ".autotrim_extents_failed", mibs.atrim.extents_failed, astats.extents_failed) ||
        GETSYSCTL_SIMPLE(ZFS_TRIM_BASE ".autotrim_extents_skipped", mibs.atrim.extents_skipped, astats.extents_skipped) ||
        GETSYSCTL_SIMPLE(ZFS_TRIM_BASE ".autotrim_extents_written", mibs.atrim.extents_written, astats.extents_written) ||
        GETSYSCTL_SIMPLE(ZFS_TRIM_BASE ".trim_bytes_failed", mibs.trim.bytes_failed, stats.bytes_failed) ||
        GETSYSCTL_SIMPLE(ZFS_TRIM_BASE ".trim_bytes_skipped", mibs.trim.bytes_skipped, stats.bytes_skipped) ||
        GETSYSCTL_SIMPLE(ZFS_TRIM_BASE ".trim_bytes_written", mibs.trim.bytes_written, stats.bytes_written) ||
        GETSYSCTL_SIMPLE(ZFS_TRIM_BASE ".trim_extents_failed", mibs.trim.extents_failed, stats.extents_failed) ||
        GETSYSCTL_SIMPLE(ZFS_TRIM_BASE ".trim_extents_skipped", mibs.trim.extents_skipped, stats.extents_skipped) ||
        GETSYSCTL_SIMPLE(ZFS_TRIM_BASE ".trim_extents_written", mibs.trim.extents_written, stats.extents_written)) {
        collector_error("DISABLED: zfs trim charts");
        return 1;
    }

    static RRDSET *st_auto_bytes = NULL;
    static RRDDIM *rd_auto_bytes_written = NULL;
    static RRDDIM *rd_auto_bytes_failed = NULL;
    static RRDDIM *rd_auto_bytes_skipped = NULL;

    if (unlikely(!st_auto_bytes)) {
        st_auto_bytes = rrdset_create_localhost(
            "zfs",
            "autotrim_bytes",
            NULL,
            "trim",
            NULL,
            "Auto TRIMmed bytes",
            "bytes/s",
            "freebsd.plugin",
            "zfs",
            2320,
            update_every,
            RRDSET_TYPE_LINE);

        rd_auto_bytes_written = rrddim_add(st_auto_bytes, "written", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        rd_auto_bytes_failed = rrddim_add(st_auto_bytes, "failed", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        rd_auto_bytes_skipped = rrddim_add(st_auto_bytes, "skipped", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(st_auto_bytes, rd_auto_bytes_written, astats.bytes_written);
    rrddim_set_by_pointer(st_auto_bytes, rd_auto_bytes_failed, astats.bytes_failed);
    rrddim_set_by_pointer(st_auto_bytes, rd_auto_bytes_skipped, astats.bytes_skipped);
    rrdset_done(st_auto_bytes);


    static RRDSET *st_auto_extents = NULL;
    static RRDDIM *rd_auto_extents_written = NULL;
    static RRDDIM *rd_auto_extents_failed = NULL;
    static RRDDIM *rd_auto_extents_skipped = NULL;

    if (unlikely(!st_auto_extents)) {
        st_auto_extents = rrdset_create_localhost(
            "zfs",
            "autotrim_extents",
            NULL,
            "trim",
            NULL,
            "Auto TRIMmed extents",
            "extents/s",
            "freebsd.plugin",
            "zfs",
            2321,
            update_every,
            RRDSET_TYPE_LINE);

        rd_auto_extents_written = rrddim_add(st_auto_extents, "written", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        rd_auto_extents_failed = rrddim_add(st_auto_extents, "failed", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        rd_auto_extents_skipped = rrddim_add(st_auto_extents, "skipped", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(st_auto_extents, rd_auto_extents_written, astats.extents_written);
    rrddim_set_by_pointer(st_auto_extents, rd_auto_extents_failed, astats.extents_failed);
    rrddim_set_by_pointer(st_auto_extents, rd_auto_extents_skipped, astats.extents_skipped);
    rrdset_done(st_auto_extents);


    static RRDSET *st_bytes = NULL;
    static RRDDIM *rd_bytes_written = NULL;
    static RRDDIM *rd_bytes_failed = NULL;
    static RRDDIM *rd_bytes_skipped = NULL;

    if (unlikely(!st_bytes)) {
        st_bytes = rrdset_create_localhost(
            "zfs",
            "trim_bytes",
            NULL,
            "trim",
            NULL,
            "TRIMmed bytes",
            "bytes/s",
            "freebsd.plugin",
            "zfs",
            2322,
            update_every,
            RRDSET_TYPE_LINE);

        rd_bytes_written = rrddim_add(st_bytes, "written", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        rd_bytes_failed = rrddim_add(st_bytes, "failed", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        rd_bytes_skipped = rrddim_add(st_bytes, "skipped", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(st_bytes, rd_bytes_written, stats.bytes_written);
    rrddim_set_by_pointer(st_bytes, rd_bytes_failed, stats.bytes_failed);
    rrddim_set_by_pointer(st_bytes, rd_bytes_skipped, stats.bytes_skipped);
    rrdset_done(st_bytes);

    static RRDSET *st_extents = NULL;
    static RRDDIM *rd_extents_written = NULL;
    static RRDDIM *rd_extents_failed = NULL;
    static RRDDIM *rd_extents_skipped = NULL;

    if (unlikely(!st_extents)) {
        st_extents = rrdset_create_localhost(
            "zfs",
            "trim_extents",
            NULL,
            "trim",
            NULL,
            "TRIMmed extents",
            "extents/s",
            "freebsd.plugin",
            "zfs",
            2323,
            update_every,
            RRDSET_TYPE_LINE);

        rd_extents_written = rrddim_add(st_extents, "written", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        rd_extents_failed = rrddim_add(st_extents, "failed", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        rd_extents_skipped = rrddim_add(st_extents, "skipped", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(st_extents, rd_extents_written, stats.extents_written);
    rrddim_set_by_pointer(st_extents, rd_extents_failed, stats.extents_failed);
    rrddim_set_by_pointer(st_extents, rd_extents_skipped, stats.extents_skipped);
    rrdset_done(st_extents);

    return 0;
}
