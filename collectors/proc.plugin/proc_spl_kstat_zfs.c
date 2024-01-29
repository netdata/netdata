// SPDX-License-Identifier: GPL-3.0-or-later

#include "plugin_proc.h"
#include "zfs_common.h"

#define ZFS_PROC_ARCSTATS "/proc/spl/kstat/zfs/arcstats"
#define ZFS_PROC_POOLS "/proc/spl/kstat/zfs"

#define STATE_SIZE 20
#define MAX_CHART_ID 256

extern struct arcstats arcstats;

unsigned long long zfs_arcstats_shrinkable_cache_size_bytes = 0;

int do_proc_spl_kstat_zfs_arcstats(int update_every, usec_t dt) {
    (void)dt;

    static int show_zero_charts = 0, do_zfs_stats = 0;
    static procfile *ff = NULL;
    static char *dirname = NULL;
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

        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/proc/spl/kstat/zfs");
        dirname = config_get("plugin:proc:" ZFS_PROC_ARCSTATS, "directory to monitor", filename);

        show_zero_charts = config_get_boolean_ondemand("plugin:proc:" ZFS_PROC_ARCSTATS, "show zero charts", CONFIG_BOOLEAN_NO);
        if(show_zero_charts == CONFIG_BOOLEAN_AUTO && netdata_zero_metrics_enabled == CONFIG_BOOLEAN_YES)
            show_zero_charts = CONFIG_BOOLEAN_YES;
        if(unlikely(show_zero_charts == CONFIG_BOOLEAN_YES))
            do_zfs_stats = 1;
    }

    // check if any pools exist
    if(likely(!do_zfs_stats)) {
        DIR *dir = opendir(dirname);
        if(unlikely(!dir)) {
            collector_error("Cannot read directory '%s'", dirname);
            return 1;
        }

        struct dirent *de = NULL;
        while(likely(de = readdir(dir))) {
            if(likely(de->d_type == DT_DIR
                && (
                    (de->d_name[0] == '.' && de->d_name[1] == '\0')
                    || (de->d_name[0] == '.' && de->d_name[1] == '.' && de->d_name[2] == '\0')
                    )))
                continue;

            if(unlikely(de->d_type == DT_LNK || de->d_type == DT_DIR)) {
                do_zfs_stats = 1;
                break;
            }
        }

        closedir(dir);
    }

    // do not show ZFS filesystem metrics if there haven't been any pools in the system yet
    if(unlikely(!do_zfs_stats))
        return 0;

    ff = procfile_readall(ff);
    if(unlikely(!ff))
        return 0; // we return 0, so that we will retry to open it next time

    size_t lines = procfile_lines(ff), l;

    arl_begin(arl_base);

    for(l = 0; l < lines ;l++) {
        size_t words = procfile_linewords(ff, l);
        if(unlikely(words < 3)) {
            if(unlikely(words)) collector_error("Cannot read " ZFS_PROC_ARCSTATS " line %zu. Expected 3 params, read %zu.", l, words);
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

    if (arcstats.size > arcstats.c_min) {
        zfs_arcstats_shrinkable_cache_size_bytes = arcstats.size - arcstats.c_min;
    } else {
        zfs_arcstats_shrinkable_cache_size_bytes = 0;
    }

    if(unlikely(arcstats.l2exist == -1))
        arcstats.l2exist = 0;

    generate_charts_arcstats(PLUGIN_PROC_NAME, ZFS_PROC_ARCSTATS, show_zero_charts, update_every);
    generate_charts_arc_summary(PLUGIN_PROC_NAME, ZFS_PROC_ARCSTATS, show_zero_charts, update_every);

    return 0;
}

struct zfs_pool {
    RRDSET *st;

    RRDDIM *rd_online;
    RRDDIM *rd_degraded;
    RRDDIM *rd_faulted;
    RRDDIM *rd_offline;
    RRDDIM *rd_removed;
    RRDDIM *rd_unavail;
    RRDDIM *rd_suspended;

    int updated;
    int disabled;

    int online;
    int degraded;
    int faulted;
    int offline;
    int removed;
    int unavail;
    int suspended;
};

struct deleted_zfs_pool {
    char *name;
    struct deleted_zfs_pool *next;
} *deleted_zfs_pools = NULL;

DICTIONARY *zfs_pools = NULL;

void disable_zfs_pool_state(struct zfs_pool *pool)
{
    if (pool->st)
        rrdset_is_obsolete___safe_from_collector_thread(pool->st);

    pool->st = NULL;

    pool->rd_online = NULL;
    pool->rd_degraded = NULL;
    pool->rd_faulted = NULL;
    pool->rd_offline = NULL;
    pool->rd_removed = NULL;
    pool->rd_unavail = NULL;
    pool->rd_suspended = NULL;

    pool->disabled = 1;
}

int update_zfs_pool_state_chart(const DICTIONARY_ITEM *item, void *pool_p, void *update_every_p) {
    const char *name = dictionary_acquired_item_name(item);
    struct zfs_pool *pool = (struct zfs_pool *)pool_p;
    int update_every = *(int *)update_every_p;

    if (pool->updated) {
        pool->updated = 0;

        if (!pool->disabled) {
            if (unlikely(!pool->st)) {
                char chart_id[MAX_CHART_ID + 1];
                snprintf(chart_id, MAX_CHART_ID, "state_%s", name);

                pool->st = rrdset_create_localhost(
                    "zfspool",
                    chart_id,
                    NULL,
                    name,
                    "zfspool.state",
                    "ZFS pool state",
                    "boolean",
                    PLUGIN_PROC_NAME,
                    ZFS_PROC_POOLS,
                    NETDATA_CHART_PRIO_ZFS_POOL_STATE,
                    update_every,
                    RRDSET_TYPE_LINE);

                pool->rd_online = rrddim_add(pool->st, "online", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                pool->rd_degraded = rrddim_add(pool->st, "degraded", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                pool->rd_faulted = rrddim_add(pool->st, "faulted", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                pool->rd_offline = rrddim_add(pool->st, "offline", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                pool->rd_removed = rrddim_add(pool->st, "removed", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                pool->rd_unavail = rrddim_add(pool->st, "unavail", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                pool->rd_suspended = rrddim_add(pool->st, "suspended", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

                rrdlabels_add(pool->st->rrdlabels, "pool", name, RRDLABEL_SRC_AUTO);
            }

            rrddim_set_by_pointer(pool->st, pool->rd_online, pool->online);
            rrddim_set_by_pointer(pool->st, pool->rd_degraded, pool->degraded);
            rrddim_set_by_pointer(pool->st, pool->rd_faulted, pool->faulted);
            rrddim_set_by_pointer(pool->st, pool->rd_offline, pool->offline);
            rrddim_set_by_pointer(pool->st, pool->rd_removed, pool->removed);
            rrddim_set_by_pointer(pool->st, pool->rd_unavail, pool->unavail);
            rrddim_set_by_pointer(pool->st, pool->rd_suspended, pool->suspended);
            rrdset_done(pool->st);
        }
    } else {
        disable_zfs_pool_state(pool);
        struct deleted_zfs_pool *new = callocz(1, sizeof(struct deleted_zfs_pool));
        new->name = strdupz(name);
        new->next = deleted_zfs_pools;
        deleted_zfs_pools = new;
    }

    return 0;
}

int do_proc_spl_kstat_zfs_pool_state(int update_every, usec_t dt)
{
    (void)dt;

    static int do_zfs_pool_state = -1;
    static char *dirname = NULL;

    int pool_found = 0, state_file_found = 0;

    if (unlikely(do_zfs_pool_state == -1)) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/proc/spl/kstat/zfs");
        dirname = config_get("plugin:proc:" ZFS_PROC_POOLS, "directory to monitor", filename);

        zfs_pools = dictionary_create_advanced(DICT_OPTION_SINGLE_THREADED, &dictionary_stats_category_collectors, 0);

        do_zfs_pool_state = 1;
    }

    if (likely(do_zfs_pool_state)) {
        DIR *dir = opendir(dirname);
        if (unlikely(!dir)) {
            if (errno == ENOENT)
                collector_info("Cannot read directory '%s'", dirname);
            else
                collector_error("Cannot read directory '%s'", dirname);
            return 1;
        }

        struct dirent *de = NULL;
        while (likely(de = readdir(dir))) {
            if (likely(
                    de->d_type == DT_DIR && ((de->d_name[0] == '.' && de->d_name[1] == '\0') ||
                                             (de->d_name[0] == '.' && de->d_name[1] == '.' && de->d_name[2] == '\0'))))
                continue;

            if (unlikely(de->d_type == DT_LNK || de->d_type == DT_DIR)) {
                pool_found = 1;

                struct zfs_pool *pool = dictionary_get(zfs_pools, de->d_name);

                if (unlikely(!pool)) {
                    struct zfs_pool new_zfs_pool = {};
                    pool = dictionary_set(zfs_pools, de->d_name, &new_zfs_pool, sizeof(struct zfs_pool));
                }

                pool->updated = 1;

                if (pool->disabled) {
                    state_file_found = 1;
                    continue;
                }
                
                pool->online = 0;
                pool->degraded = 0;
                pool->faulted = 0;
                pool->offline = 0;
                pool->removed = 0;
                pool->unavail = 0;
                pool->suspended = 0;

                char filename[FILENAME_MAX + 1];
                snprintfz(filename, FILENAME_MAX, "%s/%s/state", dirname, de->d_name);

                char state[STATE_SIZE + 1];
                int ret = read_txt_file(filename, state, sizeof(state));

                if (!ret) {
                    state_file_found = 1;

                    // ZFS pool states are described at https://openzfs.github.io/openzfs-docs/man/8/zpoolconcepts.8.html?#Device_Failure_and_Recovery
                    if (!strcmp(state, "ONLINE\n")) {
                        pool->online = 1;
                    } else if (!strcmp(state, "DEGRADED\n")) {
                        pool->degraded = 1;
                    } else if (!strcmp(state, "FAULTED\n")) {
                        pool->faulted = 1;
                    } else if (!strcmp(state, "OFFLINE\n")) {
                        pool->offline = 1;
                    } else if (!strcmp(state, "REMOVED\n")) {
                        pool->removed = 1;
                    } else if (!strcmp(state, "UNAVAIL\n")) {
                        pool->unavail = 1;
                    } else if (!strcmp(state, "SUSPENDED\n")) {
                        pool->suspended = 1;
                    } else {
                        disable_zfs_pool_state(pool);

                        char *c = strchr(state, '\n');
                        if (c)
                            *c = '\0';
                        collector_error("ZFS POOLS: Undefined state %s for zpool %s, disabling the chart", state, de->d_name);
                    }
                }
            }
        }

        closedir(dir);
    }

    if (do_zfs_pool_state && pool_found && !state_file_found) {
        collector_info("ZFS POOLS: State files not found. Disabling the module.");
        do_zfs_pool_state = 0;
    }

    if (do_zfs_pool_state)
        dictionary_walkthrough_read(zfs_pools, update_zfs_pool_state_chart, &update_every);

    while (deleted_zfs_pools) {
        struct deleted_zfs_pool *current_pool = deleted_zfs_pools;
        dictionary_del(zfs_pools, current_pool->name);

        deleted_zfs_pools = deleted_zfs_pools->next;
        
        freez(current_pool->name);
        freez(current_pool);
    }

    return 0;
}
