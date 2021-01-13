// SPDX-License-Identifier: GPL-3.0-or-later

#include "plugin_diskspace.h"

#define PLUGIN_DISKSPACE_NAME "diskspace.plugin"

#define DELAULT_EXCLUDED_PATHS "/proc/* /sys/* /var/run/user/* /run/user/* /snap/* /var/lib/docker/*"
#define DEFAULT_EXCLUDED_FILESYSTEMS "*gvfs *gluster* *s3fs *ipfs *davfs2 *httpfs *sshfs *gdfs *moosefs fusectl autofs"
#define CONFIG_SECTION_DISKSPACE "plugin:proc:diskspace"

static struct mountinfo *disk_mountinfo_root = NULL;
static uv_rwlock_t disk_mountinfo_lock;
static struct mountinfo *disk_mountinfo_busy_root = NULL;
static int check_for_new_mountpoints_every = 15;
static int cleanup_mount_points = 1;

static inline void mountinfo_reload(int force) {
    static time_t last_loaded = 0;
    time_t now = now_realtime_sec();

    if(force || now - last_loaded >= check_for_new_mountpoints_every) {
        uv_rwlock_wrlock(&disk_mountinfo_lock);

        // free mountinfo structures, keep busy mountinfo structures in a separate list
        struct mountinfo *mi = disk_mountinfo_root;
        while(mi) {
            struct mountinfo *curr_mi = mi;
            mi = mi->next;
            if (!curr_mi->busy) {
                mountinfo_free(curr_mi);
            } else {
                curr_mi->next = disk_mountinfo_busy_root;
                disk_mountinfo_busy_root = curr_mi;
            }
        }

        // finally remove and free mountinfo structures if they aren't busy anymore
        mi = disk_mountinfo_busy_root;
        struct mountinfo *prev_mi = disk_mountinfo_busy_root;
        while (mi) {
            struct mountinfo *curr_mi = mi;
            mi = mi->next;
            if (curr_mi->busy) {
                prev_mi = curr_mi;
            } else {
                if (curr_mi == disk_mountinfo_busy_root)
                    disk_mountinfo_busy_root = mi;
                else
                    prev_mi->next = mi;

                mountinfo_free(curr_mi);
            }
        }

        uv_rwlock_wrunlock(&disk_mountinfo_lock);

        // re-read mountinfo in case something changed
        disk_mountinfo_root = mountinfo_read(0);

        last_loaded = now;
    }
}

// Data to be stored in DICTIONARY dict_mountpoints used by do_disk_space_stats().
// This DICTIONARY is used to lookup the settings of the mount point on each iteration.
struct mount_point_metadata {
    int do_space;
    int do_inodes;
    int shown_error;
    int updated;
    int busy;

    size_t collected; // the number of times this has been collected

    RRDSET *st_space;
    RRDDIM *rd_space_used;
    RRDDIM *rd_space_avail;
    RRDDIM *rd_space_reserved;

    RRDSET *st_inodes;
    RRDDIM *rd_inodes_used;
    RRDDIM *rd_inodes_avail;
    RRDDIM *rd_inodes_reserved;
};

static DICTIONARY *dict_mountpoints = NULL;
static uv_rwlock_t dict_mountpoints_lock;

#define rrdset_obsolete_and_pointer_null(st) do { if(st) { rrdset_is_obsolete(st); (st) = NULL; } } while(st)

int mount_point_cleanup(void *entry, void *data) {
    (void)data;

    struct mount_point_metadata *mp = (struct mount_point_metadata *)entry;
    if(!mp) return 0;


    if (mp->busy) {
        return 0;
    }

    if(likely(mp->updated > 0)) {
        mp->updated--;
        return 0;
    }

    if(likely(cleanup_mount_points && mp->collected)) {
        mp->collected = 0;
        mp->updated = 0;
        mp->shown_error = 0;

        mp->rd_space_avail = NULL;
        mp->rd_space_used = NULL;
        mp->rd_space_reserved = NULL;

        mp->rd_inodes_avail = NULL;
        mp->rd_inodes_used = NULL;
        mp->rd_inodes_reserved = NULL;

        rrdset_obsolete_and_pointer_null(mp->st_space);
        rrdset_obsolete_and_pointer_null(mp->st_inodes);
    }

    return 0;
}

static inline void do_disk_space_stats(struct mountinfo *mi, int update_every) {
    const char *family = mi->mount_point;
    const char *disk = mi->persistent_id;

    static SIMPLE_PATTERN *excluded_mountpoints = NULL;
    static SIMPLE_PATTERN *excluded_filesystems = NULL;
    int do_space, do_inodes;

    if (!mi->busy) {
    #ifdef NETDATA_INTERNAL_CHECKS
        error("DISKSPACE: mointpoint %s is not marked busy", mi->mount_point);
    #endif
        mi->busy = 1;
    }

    if(unlikely(!dict_mountpoints)) {
        SIMPLE_PREFIX_MODE mode = SIMPLE_PATTERN_EXACT;

        if(config_move("plugin:proc:/proc/diskstats", "exclude space metrics on paths", CONFIG_SECTION_DISKSPACE, "exclude space metrics on paths") != -1) {
            // old configuration, enable backwards compatibility
            mode = SIMPLE_PATTERN_PREFIX;
        }

        excluded_mountpoints = simple_pattern_create(
                config_get(CONFIG_SECTION_DISKSPACE, "exclude space metrics on paths", DELAULT_EXCLUDED_PATHS)
                , NULL
                , mode
        );

        excluded_filesystems = simple_pattern_create(
                config_get(CONFIG_SECTION_DISKSPACE, "exclude space metrics on filesystems", DEFAULT_EXCLUDED_FILESYSTEMS)
                , NULL
                , SIMPLE_PATTERN_EXACT
        );

        dict_mountpoints = dictionary_create(DICTIONARY_FLAG_SINGLE_THREADED);
    }

    uv_rwlock_rdlock(&dict_mountpoints_lock);
    struct mount_point_metadata *m = dictionary_get(dict_mountpoints, mi->mount_point);
    if (likely(m)) {
        if (m->busy) {
            uv_rwlock_rdunlock(&dict_mountpoints_lock);
            return;
        }
        m->busy = 1;
    }
    uv_rwlock_rdunlock(&dict_mountpoints_lock);

    if (unlikely(!m)) {
        char var_name[4096 + 1];
        snprintfz(var_name, 4096, "plugin:proc:diskspace:%s", mi->mount_point);

        int def_space = config_get_boolean_ondemand(CONFIG_SECTION_DISKSPACE, "space usage for all disks", CONFIG_BOOLEAN_AUTO);
        int def_inodes = config_get_boolean_ondemand(CONFIG_SECTION_DISKSPACE, "inodes usage for all disks", CONFIG_BOOLEAN_AUTO);

        if(unlikely(simple_pattern_matches(excluded_mountpoints, mi->mount_point))) {
            def_space = CONFIG_BOOLEAN_NO;
            def_inodes = CONFIG_BOOLEAN_NO;
        }

        if(unlikely(simple_pattern_matches(excluded_filesystems, mi->filesystem))) {
            def_space = CONFIG_BOOLEAN_NO;
            def_inodes = CONFIG_BOOLEAN_NO;
        }

        // check if the mount point is a directory #2407
        // but only when it is enabled by default #4491
        if(def_space != CONFIG_BOOLEAN_NO || def_inodes != CONFIG_BOOLEAN_NO) {
            struct stat bs;
            if(stat(mi->mount_point, &bs) == -1) {
                error("DISKSPACE: Cannot stat() mount point '%s' (disk '%s', filesystem '%s', root '%s')."
                      , mi->mount_point
                      , disk
                      , mi->filesystem?mi->filesystem:""
                      , mi->root?mi->root:""
                );
                def_space = CONFIG_BOOLEAN_NO;
                def_inodes = CONFIG_BOOLEAN_NO;
            }
            else {
                if((bs.st_mode & S_IFMT) != S_IFDIR) {
                    error("DISKSPACE: Mount point '%s' (disk '%s', filesystem '%s', root '%s') is not a directory."
                          , mi->mount_point
                          , disk
                          , mi->filesystem?mi->filesystem:""
                          , mi->root?mi->root:""
                    );
                    def_space = CONFIG_BOOLEAN_NO;
                    def_inodes = CONFIG_BOOLEAN_NO;
                }
            }
        }

        do_space = config_get_boolean_ondemand(var_name, "space usage", def_space);
        do_inodes = config_get_boolean_ondemand(var_name, "inodes usage", def_inodes);

        struct mount_point_metadata mp = {
                .do_space = do_space,
                .do_inodes = do_inodes,
                .shown_error = 0,
                .updated = 0,
                .busy = 1,

                .collected = 0,

                .st_space = NULL,
                .rd_space_avail = NULL,
                .rd_space_used = NULL,
                .rd_space_reserved = NULL,

                .st_inodes = NULL,
                .rd_inodes_avail = NULL,
                .rd_inodes_used = NULL,
                .rd_inodes_reserved = NULL
        };
        m = dictionary_set(dict_mountpoints, mi->mount_point, &mp, sizeof(struct mount_point_metadata));
    }

    m->updated = 2;

    if(unlikely(m->do_space == CONFIG_BOOLEAN_NO && m->do_inodes == CONFIG_BOOLEAN_NO))
        goto exit;

    if(unlikely(mi->flags & MOUNTINFO_READONLY && !m->collected && m->do_space != CONFIG_BOOLEAN_YES && m->do_inodes != CONFIG_BOOLEAN_YES))
        goto exit;

    struct statvfs buff_statvfs;
    if (statvfs(mi->mount_point, &buff_statvfs) < 0) {
        if(!m->shown_error) {
            error("DISKSPACE: failed to statvfs() mount point '%s' (disk '%s', filesystem '%s', root '%s')"
                  , mi->mount_point
                  , disk
                  , mi->filesystem?mi->filesystem:""
                  , mi->root?mi->root:""
            );
            m->shown_error = 1;
        }
        goto exit;
    }
    m->shown_error = 0;

    // logic found at get_fs_usage() in coreutils
    unsigned long bsize = (buff_statvfs.f_frsize) ? buff_statvfs.f_frsize : buff_statvfs.f_bsize;

    fsblkcnt_t bavail         = buff_statvfs.f_bavail;
    fsblkcnt_t btotal         = buff_statvfs.f_blocks;
    fsblkcnt_t bavail_root    = buff_statvfs.f_bfree;
    fsblkcnt_t breserved_root = bavail_root - bavail;
    fsblkcnt_t bused;
    if(likely(btotal >= bavail_root))
        bused = btotal - bavail_root;
    else
        bused = bavail_root - btotal;

#ifdef NETDATA_INTERNAL_CHECKS
    if(unlikely(btotal != bavail + breserved_root + bused))
        error("DISKSPACE: disk block statistics for '%s' (disk '%s') do not sum up: total = %llu, available = %llu, reserved = %llu, used = %llu", mi->mount_point, disk, (unsigned long long)btotal, (unsigned long long)bavail, (unsigned long long)breserved_root, (unsigned long long)bused);
#endif

    // --------------------------------------------------------------------------

    fsfilcnt_t favail         = buff_statvfs.f_favail;
    fsfilcnt_t ftotal         = buff_statvfs.f_files;
    fsfilcnt_t favail_root    = buff_statvfs.f_ffree;
    fsfilcnt_t freserved_root = favail_root - favail;
    fsfilcnt_t fused          = ftotal - favail_root;

    if(m->do_inodes == CONFIG_BOOLEAN_AUTO && favail == (fsfilcnt_t)-1) {
        // this file system does not support inodes reporting
        // eg. cephfs
        m->do_inodes = CONFIG_BOOLEAN_NO;
    }

#ifdef NETDATA_INTERNAL_CHECKS
    if(unlikely(btotal != bavail + breserved_root + bused))
        error("DISKSPACE: disk inode statistics for '%s' (disk '%s') do not sum up: total = %llu, available = %llu, reserved = %llu, used = %llu", mi->mount_point, disk, (unsigned long long)ftotal, (unsigned long long)favail, (unsigned long long)freserved_root, (unsigned long long)fused);
#endif

    // --------------------------------------------------------------------------

    int rendered = 0;

    if(m->do_space == CONFIG_BOOLEAN_YES || (m->do_space == CONFIG_BOOLEAN_AUTO &&
                                             (bavail || breserved_root || bused ||
                                              netdata_zero_metrics_enabled == CONFIG_BOOLEAN_YES))) {
        if(unlikely(!m->st_space)) {
            m->do_space = CONFIG_BOOLEAN_YES;
            m->st_space = rrdset_find_active_bytype_localhost("disk_space", disk);
            if(unlikely(!m->st_space)) {
                char title[4096 + 1];
                snprintfz(title, 4096, "Disk Space Usage for %s [%s]", family, mi->mount_source);
                m->st_space = rrdset_create_localhost(
                        "disk_space"
                        , disk
                        , NULL
                        , family
                        , "disk.space"
                        , title
                        , "GiB"
                        , PLUGIN_DISKSPACE_NAME
                        , NULL
                        , NETDATA_CHART_PRIO_DISKSPACE_SPACE
                        , update_every
                        , RRDSET_TYPE_STACKED
                );
            }

            m->rd_space_avail    = rrddim_add(m->st_space, "avail", NULL, (collected_number)bsize, 1024 * 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
            m->rd_space_used     = rrddim_add(m->st_space, "used", NULL, (collected_number)bsize, 1024 * 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
            m->rd_space_reserved = rrddim_add(m->st_space, "reserved_for_root", "reserved for root", (collected_number)bsize, 1024 * 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
        }
        else
            rrdset_next(m->st_space);

        rrddim_set_by_pointer(m->st_space, m->rd_space_avail,    (collected_number)bavail);
        rrddim_set_by_pointer(m->st_space, m->rd_space_used,     (collected_number)bused);
        rrddim_set_by_pointer(m->st_space, m->rd_space_reserved, (collected_number)breserved_root);
        rrdset_done(m->st_space);

        rendered++;
    }

    // --------------------------------------------------------------------------

    if(m->do_inodes == CONFIG_BOOLEAN_YES || (m->do_inodes == CONFIG_BOOLEAN_AUTO &&
                                              (favail || freserved_root || fused ||
                                               netdata_zero_metrics_enabled == CONFIG_BOOLEAN_YES))) {
        if(unlikely(!m->st_inodes)) {
            m->do_inodes = CONFIG_BOOLEAN_YES;
            m->st_inodes = rrdset_find_active_bytype_localhost("disk_inodes", disk);
            if(unlikely(!m->st_inodes)) {
                char title[4096 + 1];
                snprintfz(title, 4096, "Disk Files (inodes) Usage for %s [%s]", family, mi->mount_source);
                m->st_inodes = rrdset_create_localhost(
                        "disk_inodes"
                        , disk
                        , NULL
                        , family
                        , "disk.inodes"
                        , title
                        , "inodes"
                        , PLUGIN_DISKSPACE_NAME
                        , NULL
                        , NETDATA_CHART_PRIO_DISKSPACE_INODES
                        , update_every
                        , RRDSET_TYPE_STACKED
                );
            }

            m->rd_inodes_avail    = rrddim_add(m->st_inodes, "avail", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            m->rd_inodes_used     = rrddim_add(m->st_inodes, "used", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            m->rd_inodes_reserved = rrddim_add(m->st_inodes, "reserved_for_root", "reserved for root", 1, 1, RRD_ALGORITHM_ABSOLUTE);
        }
        else
            rrdset_next(m->st_inodes);

        rrddim_set_by_pointer(m->st_inodes, m->rd_inodes_avail,    (collected_number)favail);
        rrddim_set_by_pointer(m->st_inodes, m->rd_inodes_used,     (collected_number)fused);
        rrddim_set_by_pointer(m->st_inodes, m->rd_inodes_reserved, (collected_number)freserved_root);
        rrdset_done(m->st_inodes);

        rendered++;
    }

    // --------------------------------------------------------------------------

    if(likely(rendered))
        m->collected++;

exit:
    m->busy = 0;
}

static struct loop_thread
{
    uv_thread_t thread;
    uv_loop_t loop;
    uv_async_t async;
} loop_thread;

static void diskspace_main_cleanup(void *ptr) {
    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)ptr;
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;

    info("cleaning up...");

    /* stop event loop */
    fatal_assert(0 == uv_async_send(&loop_thread.async));

    int error = uv_thread_join(&loop_thread.thread);
    if (error) {
        error("uv_thread_join(): %s", uv_strerror(error));
    }

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
}

void close_async(uv_async_t *handle)
{
    uv_close((uv_handle_t *)handle, NULL);
}

void run_event_loop(void *ptr)
{
    UNUSED(ptr);
    int error;

    error = uv_loop_init(&loop_thread.loop);
    if (error) {
        error("uv_loop_init(): %s", uv_strerror(error));
        return;
    }

    error = uv_async_init(&loop_thread.loop, &loop_thread.async, close_async);
    if (error) {
        error("uv_async_init(): %s", uv_strerror(error));
    }
    
    uv_run(&loop_thread.loop, UV_RUN_DEFAULT);

    fatal_assert(0 == uv_loop_close(&loop_thread.loop));
}

struct work_data {
    struct mountinfo *mi;
    int update_every;
};

void disk_space_stats_work(uv_work_t* req)
{
    struct work_data *d = req->data;

    do_disk_space_stats(d->mi, d->update_every);
}

void disk_space_stats_done(uv_work_t* req, int status)
{
    UNUSED(status);
    struct work_data *d = req->data;

    d->mi->busy = 0;
    free(d);
}

void *diskspace_main(void *ptr) {
    netdata_thread_cleanup_push(diskspace_main_cleanup, ptr);

    int vdo_cpu_netdata = config_get_boolean("plugin:proc", "netdata server resources", 1);

    cleanup_mount_points = config_get_boolean(CONFIG_SECTION_DISKSPACE, "remove charts of unmounted disks" , cleanup_mount_points);

    int update_every = (int)config_get_number(CONFIG_SECTION_DISKSPACE, "update every", localhost->rrd_update_every);
    if(update_every < localhost->rrd_update_every)
        update_every = localhost->rrd_update_every;

    check_for_new_mountpoints_every = (int)config_get_number(CONFIG_SECTION_DISKSPACE, "check for new mount points every", check_for_new_mountpoints_every);
    if(check_for_new_mountpoints_every < update_every)
        check_for_new_mountpoints_every = update_every;

    struct rusage thread;

    fatal_assert(0 == uv_rwlock_init(&disk_mountinfo_lock));
    fatal_assert(0 == uv_rwlock_init(&dict_mountpoints_lock));

    int error = uv_thread_create(&loop_thread.thread, run_event_loop, NULL);
    if (error) {
        error("uv_thread_create(): %s", uv_strerror(error));
        return NULL;
    }

    usec_t duration = 0;
    usec_t step = update_every * USEC_PER_SEC;
    heartbeat_t hb;
    heartbeat_init(&hb);
    while(!netdata_exit) {
        duration = heartbeat_monotonic_dt_to_now_usec(&hb);
        /* usec_t hb_dt = */ heartbeat_next(&hb, step);

        if(unlikely(netdata_exit)) break;


        // --------------------------------------------------------------------------
        // this is smart enough not to reload it every time

        mountinfo_reload(0);


        // --------------------------------------------------------------------------
        // disk space metrics

        struct mountinfo *mi;
        for(mi = disk_mountinfo_root; mi; mi = mi->next) {

            if(unlikely(mi->flags & (MOUNTINFO_IS_DUMMY | MOUNTINFO_IS_BIND) || mi->busy))
                continue;

            mi->busy = 1;

            struct work_data *d = mallocz(sizeof(struct work_data));
            d->mi = mi;
            d->update_every = update_every;
            mi->work.data = d;

            fatal_assert(0 == uv_queue_work(&loop_thread.loop, &mi->work, disk_space_stats_work, disk_space_stats_done));
            
            if(unlikely(netdata_exit)) break;
        }

        if(unlikely(netdata_exit)) break;

        if(dict_mountpoints) {
            uv_rwlock_wrlock(&dict_mountpoints_lock);
            dictionary_get_all(dict_mountpoints, mount_point_cleanup, NULL);
            uv_rwlock_wrunlock(&dict_mountpoints_lock);
        }

        if(vdo_cpu_netdata) {
            static RRDSET *stcpu_thread = NULL, *st_duration = NULL;
            static RRDDIM *rd_user = NULL, *rd_system = NULL, *rd_duration = NULL;

            // ----------------------------------------------------------------

            getrusage(RUSAGE_THREAD, &thread);

            if(unlikely(!stcpu_thread)) {
                stcpu_thread = rrdset_create_localhost(
                        "netdata"
                        , "plugin_diskspace"
                        , NULL
                        , "diskspace"
                        , NULL
                        , "NetData Disk Space Plugin CPU usage"
                        , "milliseconds/s"
                        , PLUGIN_DISKSPACE_NAME
                        , NULL
                        , NETDATA_CHART_PRIO_NETDATA_DISKSPACE
                        , update_every
                        , RRDSET_TYPE_STACKED
                );

                rd_user   = rrddim_add(stcpu_thread, "user", NULL, 1, 1000, RRD_ALGORITHM_INCREMENTAL);
                rd_system = rrddim_add(stcpu_thread, "system", NULL, 1, 1000, RRD_ALGORITHM_INCREMENTAL);
            }
            else
                rrdset_next(stcpu_thread);

            rrddim_set_by_pointer(stcpu_thread, rd_user, thread.ru_utime.tv_sec * 1000000ULL + thread.ru_utime.tv_usec);
            rrddim_set_by_pointer(stcpu_thread, rd_system, thread.ru_stime.tv_sec * 1000000ULL + thread.ru_stime.tv_usec);
            rrdset_done(stcpu_thread);

            // ----------------------------------------------------------------

            if(unlikely(!st_duration)) {
                st_duration = rrdset_create_localhost(
                        "netdata"
                        , "plugin_diskspace_dt"
                        , NULL
                        , "diskspace"
                        , NULL
                        , "NetData Disk Space Plugin Duration"
                        , "milliseconds/run"
                        , PLUGIN_DISKSPACE_NAME
                        , NULL
                        , 132021
                        , update_every
                        , RRDSET_TYPE_AREA
                );

                rd_duration = rrddim_add(st_duration, "duration", NULL, 1, 1000, RRD_ALGORITHM_ABSOLUTE);
            }
            else
                rrdset_next(st_duration);

            rrddim_set_by_pointer(st_duration, rd_duration, duration);
            rrdset_done(st_duration);

            // ----------------------------------------------------------------

            if(unlikely(netdata_exit)) break;
        }
    }

    netdata_thread_cleanup_pop(1);
    return NULL;
}
