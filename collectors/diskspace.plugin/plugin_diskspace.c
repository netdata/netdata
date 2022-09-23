// SPDX-License-Identifier: GPL-3.0-or-later

#include "../proc.plugin/plugin_proc.h"

#define PLUGIN_DISKSPACE_NAME "diskspace.plugin"
#define THREAD_DISKSPACE_SLOW_NAME "PLUGIN[diskspace slow]"

#define DEFAULT_EXCLUDED_PATHS "/proc/* /sys/* /var/run/user/* /run/user/* /snap/* /var/lib/docker/*"
#define DEFAULT_EXCLUDED_FILESYSTEMS "*gvfs *gluster* *s3fs *ipfs *davfs2 *httpfs *sshfs *gdfs *moosefs fusectl autofs"
#define CONFIG_SECTION_DISKSPACE "plugin:proc:diskspace"

#define MAX_STAT_USEC 10000LU
#define SLOW_UPDATE_EVERY 5

static netdata_thread_t *diskspace_slow_thread = NULL;

static struct mountinfo *disk_mountinfo_root = NULL;
static int check_for_new_mountpoints_every = 15;
static int cleanup_mount_points = 1;

static inline void mountinfo_reload(int force) {
    static time_t last_loaded = 0;
    time_t now = now_realtime_sec();

    if(force || now - last_loaded >= check_for_new_mountpoints_every) {
        // mountinfo_free_all() can be called with NULL disk_mountinfo_root
        mountinfo_free_all(disk_mountinfo_root);

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
    int slow;

    DICTIONARY *chart_labels;

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

#define rrdset_obsolete_and_pointer_null(st) do { if(st) { rrdset_is_obsolete(st); (st) = NULL; } } while(st)

int mount_point_cleanup(const char *name, void *entry, int slow) {
    (void)name;
    
    struct mount_point_metadata *mp = (struct mount_point_metadata *)entry;
    if(!mp) return 0;

    if (slow != mp->slow)
        return 0;

    if(likely(mp->updated)) {
        mp->updated = 0;
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

int mount_point_cleanup_cb(const DICTIONARY_ITEM *item, void *entry, void *data __maybe_unused) {
    const char *name = dictionary_acquired_item_name(item);

    return mount_point_cleanup(name, (struct mount_point_metadata *)entry, 0);
}

// a copy of basic mountinfo fields
struct basic_mountinfo {
    char *persistent_id;    
    char *root;             
    char *mount_point;      
    char *filesystem;       

    struct basic_mountinfo *next;
};

static struct basic_mountinfo *slow_mountinfo_tmp_root = NULL;
static netdata_mutex_t slow_mountinfo_mutex;

static struct basic_mountinfo *basic_mountinfo_create_and_copy(struct mountinfo* mi)
{
    struct basic_mountinfo *bmi = callocz(1, sizeof(struct basic_mountinfo));
    
    if (mi) {
        bmi->persistent_id = strdupz(mi->persistent_id);
        bmi->root          = strdupz(mi->root);
        bmi->mount_point   = strdupz(mi->mount_point);
        bmi->filesystem    = strdupz(mi->filesystem);
    }

    return bmi;
}

static void add_basic_mountinfo(struct basic_mountinfo **root, struct mountinfo *mi)
{
    if (!root)
        return;

    struct basic_mountinfo *bmi = basic_mountinfo_create_and_copy(mi);

    bmi->next = *root;
    *root = bmi;
};

static void free_basic_mountinfo(struct basic_mountinfo *bmi)
{
    if (bmi) {
        freez(bmi->persistent_id);
        freez(bmi->root);
        freez(bmi->mount_point);
        freez(bmi->filesystem);

        freez(bmi);
    }
};

static void free_basic_mountinfo_list(struct basic_mountinfo *root)
{
    struct basic_mountinfo *bmi = root, *next;

    while (bmi) {
        next = bmi->next;
        free_basic_mountinfo(bmi);
        bmi = next;
    }
}

static void calculate_values_and_show_charts(
    struct basic_mountinfo *mi,
    struct mount_point_metadata *m,
    struct statvfs *buff_statvfs,
    int update_every)
{
    const char *family = mi->mount_point;
    const char *disk = mi->persistent_id;

    // logic found at get_fs_usage() in coreutils
    unsigned long bsize = (buff_statvfs->f_frsize) ? buff_statvfs->f_frsize : buff_statvfs->f_bsize;

    fsblkcnt_t bavail         = buff_statvfs->f_bavail;
    fsblkcnt_t btotal         = buff_statvfs->f_blocks;
    fsblkcnt_t bavail_root    = buff_statvfs->f_bfree;
    fsblkcnt_t breserved_root = bavail_root - bavail;
    fsblkcnt_t bused = likely(btotal >= bavail_root) ? btotal - bavail_root : bavail_root - btotal;

#ifdef NETDATA_INTERNAL_CHECKS
    if(unlikely(btotal != bavail + breserved_root + bused))
        error("DISKSPACE: disk block statistics for '%s' (disk '%s') do not sum up: total = %llu, available = %llu, reserved = %llu, used = %llu", mi->mount_point, disk, (unsigned long long)btotal, (unsigned long long)bavail, (unsigned long long)breserved_root, (unsigned long long)bused);
#endif

    // --------------------------------------------------------------------------

    fsfilcnt_t favail         = buff_statvfs->f_favail;
    fsfilcnt_t ftotal         = buff_statvfs->f_files;
    fsfilcnt_t favail_root    = buff_statvfs->f_ffree;
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
        if(unlikely(!m->st_space) || m->st_space->update_every != update_every) {
            m->do_space = CONFIG_BOOLEAN_YES;
            m->st_space = rrdset_find_active_bytype_localhost("disk_space", disk);
            if(unlikely(!m->st_space || m->st_space->update_every != update_every)) {
                char title[4096 + 1];
                snprintfz(title, 4096, "Disk Space Usage");
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

            rrdset_update_rrdlabels(m->st_space, m->chart_labels);

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
        if(unlikely(!m->st_inodes) || m->st_inodes->update_every != update_every) {
            m->do_inodes = CONFIG_BOOLEAN_YES;
            m->st_inodes = rrdset_find_active_bytype_localhost("disk_inodes", disk);
            if(unlikely(!m->st_inodes) || m->st_inodes->update_every != update_every) {
                char title[4096 + 1];
                snprintfz(title, 4096, "Disk Files (inodes) Usage");
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

            rrdset_update_rrdlabels(m->st_inodes, m->chart_labels);

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
}

static inline void do_disk_space_stats(struct mountinfo *mi, int update_every) {
    const char *disk = mi->persistent_id;

    static SIMPLE_PATTERN *excluded_mountpoints = NULL;
    static SIMPLE_PATTERN *excluded_filesystems = NULL;

    usec_t slow_timeout = MAX_STAT_USEC * update_every;

    int do_space, do_inodes;

    if(unlikely(!dict_mountpoints)) {
        SIMPLE_PREFIX_MODE mode = SIMPLE_PATTERN_EXACT;

        if(config_move("plugin:proc:/proc/diskstats", "exclude space metrics on paths", CONFIG_SECTION_DISKSPACE, "exclude space metrics on paths") != -1) {
            // old configuration, enable backwards compatibility
            mode = SIMPLE_PATTERN_PREFIX;
        }

        excluded_mountpoints = simple_pattern_create(
                config_get(CONFIG_SECTION_DISKSPACE, "exclude space metrics on paths", DEFAULT_EXCLUDED_PATHS)
                , NULL
                , mode
        );

        excluded_filesystems = simple_pattern_create(
                config_get(CONFIG_SECTION_DISKSPACE, "exclude space metrics on filesystems", DEFAULT_EXCLUDED_FILESYSTEMS)
                , NULL
                , SIMPLE_PATTERN_EXACT
        );

        dict_mountpoints = dictionary_create(DICT_OPTION_SINGLE_THREADED);
    }

    struct mount_point_metadata *m = dictionary_get(dict_mountpoints, mi->mount_point);
    if(unlikely(!m)) {
        int slow = 0;
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
            usec_t start_time = now_monotonic_high_precision_usec();
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

            if ((now_monotonic_high_precision_usec() - start_time) > slow_timeout)
                slow = 1;
        }

        do_space = config_get_boolean_ondemand(var_name, "space usage", def_space);
        do_inodes = config_get_boolean_ondemand(var_name, "inodes usage", def_inodes);

        struct mount_point_metadata mp = {
                .do_space = do_space,
                .do_inodes = do_inodes,
                .shown_error = 0,
                .updated = 0,
                .slow = 0,

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

        mp.chart_labels = rrdlabels_create();
        rrdlabels_add(mp.chart_labels, "mount_point", mi->mount_point, RRDLABEL_SRC_AUTO);
        rrdlabels_add(mp.chart_labels, "filesystem", mi->filesystem, RRDLABEL_SRC_AUTO);
        rrdlabels_add(mp.chart_labels, "mount_root", mi->root, RRDLABEL_SRC_AUTO);

        m = dictionary_set(dict_mountpoints, mi->mount_point, &mp, sizeof(struct mount_point_metadata));

        m->slow = slow;
    }

    if (m->slow) {
        add_basic_mountinfo(&slow_mountinfo_tmp_root, mi);
        return;
    }

    m->updated = 1;

    if(unlikely(m->do_space == CONFIG_BOOLEAN_NO && m->do_inodes == CONFIG_BOOLEAN_NO))
        return;

    if (unlikely(
            mi->flags & MOUNTINFO_READONLY &&
            !(mi->flags & MOUNTINFO_IS_IN_SYSD_PROTECTED_LIST) &&
            !m->collected &&
            m->do_space != CONFIG_BOOLEAN_YES &&
            m->do_inodes != CONFIG_BOOLEAN_YES))
        return;

    usec_t start_time = now_monotonic_high_precision_usec();
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
        return;
    }

    if ((now_monotonic_high_precision_usec() - start_time) > slow_timeout)
        m->slow = 1;

    m->shown_error = 0;

    struct basic_mountinfo bmi;
    bmi.mount_point = mi->mount_point;
    bmi.persistent_id = mi->persistent_id;
    bmi.filesystem = mi->filesystem;
    bmi.root = mi->root;

    calculate_values_and_show_charts(&bmi, m, &buff_statvfs, update_every);
}

static inline void do_slow_disk_space_stats(struct basic_mountinfo *mi, int update_every) {
    struct mount_point_metadata *m = dictionary_get(dict_mountpoints, mi->mount_point);

    m->updated = 1;

    struct statvfs buff_statvfs;
    if (statvfs(mi->mount_point, &buff_statvfs) < 0) {
        if(!m->shown_error) {
            error("DISKSPACE: failed to statvfs() mount point '%s' (disk '%s', filesystem '%s', root '%s')"
                  , mi->mount_point
                  , mi->persistent_id
                  , mi->filesystem?mi->filesystem:""
                  , mi->root?mi->root:""
            );
            m->shown_error = 1;
        }
        return;
    }
    m->shown_error = 0;

    calculate_values_and_show_charts(mi, m, &buff_statvfs, update_every);
}

static void diskspace_slow_worker_cleanup(void *ptr)
{
    UNUSED(ptr);

    info("cleaning up...");

    worker_unregister();
}

#define WORKER_JOB_SLOW_MOUNTPOINT 0
#define WORKER_JOB_SLOW_CLEANUP 1

struct slow_worker_data {
    netdata_thread_t *slow_thread;
    int update_every;
};

void *diskspace_slow_worker(void *ptr)
{
    struct slow_worker_data *data = (struct slow_worker_data *)ptr;
    
    worker_register("DISKSPACE_SLOW");
    worker_register_job_name(WORKER_JOB_SLOW_MOUNTPOINT, "mountpoint");
    worker_register_job_name(WORKER_JOB_SLOW_CLEANUP, "cleanup");

    struct basic_mountinfo *slow_mountinfo_root = NULL;

    int slow_update_every = data->update_every > SLOW_UPDATE_EVERY ? data->update_every : SLOW_UPDATE_EVERY;

    netdata_thread_cleanup_push(diskspace_slow_worker_cleanup, data->slow_thread);

    usec_t step = slow_update_every * USEC_PER_SEC;
    heartbeat_t hb;
    heartbeat_init(&hb);

    while(!netdata_exit) {
        worker_is_idle();
        heartbeat_next(&hb, step);

        usec_t start_time = now_monotonic_high_precision_usec();

        if (!dict_mountpoints)
            continue;

        if(unlikely(netdata_exit)) break;

        // --------------------------------------------------------------------------
        // disk space metrics

        worker_is_busy(WORKER_JOB_SLOW_MOUNTPOINT);

        netdata_mutex_lock(&slow_mountinfo_mutex);
        free_basic_mountinfo_list(slow_mountinfo_root);
        slow_mountinfo_root = slow_mountinfo_tmp_root;
        slow_mountinfo_tmp_root = NULL;
        netdata_mutex_unlock(&slow_mountinfo_mutex);

        struct basic_mountinfo *bmi;
        for(bmi = slow_mountinfo_root; bmi; bmi = bmi->next) {
            do_slow_disk_space_stats(bmi, slow_update_every);
            
            if(unlikely(netdata_exit)) break;
        }

        if(unlikely(netdata_exit)) break;

        worker_is_busy(WORKER_JOB_SLOW_CLEANUP);

        for(bmi = slow_mountinfo_root; bmi; bmi = bmi->next) {
            struct mount_point_metadata *m = dictionary_get(dict_mountpoints, bmi->mount_point);

            if (m)
                mount_point_cleanup(bmi->mount_point, m, 1);
        }

        usec_t dt = now_monotonic_high_precision_usec() - start_time;
        if (dt > step) {
            slow_update_every = (dt / USEC_PER_SEC) * 3 / 2;
            if (slow_update_every % SLOW_UPDATE_EVERY)
                slow_update_every += SLOW_UPDATE_EVERY - slow_update_every % SLOW_UPDATE_EVERY;
            step = slow_update_every * USEC_PER_SEC;
        }
    }

    netdata_thread_cleanup_pop(1);

    free_basic_mountinfo_list(slow_mountinfo_root);

    return NULL;
}

static void diskspace_main_cleanup(void *ptr) {
    worker_unregister();

    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)ptr;
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;

    info("cleaning up...");

    if (diskspace_slow_thread) {
        netdata_thread_join(*diskspace_slow_thread, NULL);
        freez(diskspace_slow_thread);
    }

    free_basic_mountinfo_list(slow_mountinfo_tmp_root);

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
}

#define WORKER_JOB_MOUNTINFO 0
#define WORKER_JOB_MOUNTPOINT 1
#define WORKER_JOB_CLEANUP 2

#if WORKER_UTILIZATION_MAX_JOB_TYPES < 3
#error WORKER_UTILIZATION_MAX_JOB_TYPES has to be at least 3
#endif

void *diskspace_main(void *ptr) {
    worker_register("DISKSPACE");
    worker_register_job_name(WORKER_JOB_MOUNTINFO, "mountinfo");
    worker_register_job_name(WORKER_JOB_MOUNTPOINT, "mountpoint");
    worker_register_job_name(WORKER_JOB_CLEANUP, "cleanup");

    netdata_thread_cleanup_push(diskspace_main_cleanup, ptr);

    cleanup_mount_points = config_get_boolean(CONFIG_SECTION_DISKSPACE, "remove charts of unmounted disks" , cleanup_mount_points);

    int update_every = (int)config_get_number(CONFIG_SECTION_DISKSPACE, "update every", localhost->rrd_update_every);
    if(update_every < localhost->rrd_update_every)
        update_every = localhost->rrd_update_every;

    check_for_new_mountpoints_every = (int)config_get_number(CONFIG_SECTION_DISKSPACE, "check for new mount points every", check_for_new_mountpoints_every);
    if(check_for_new_mountpoints_every < update_every)
        check_for_new_mountpoints_every = update_every;

    netdata_mutex_init(&slow_mountinfo_mutex);

    diskspace_slow_thread = mallocz(sizeof(netdata_thread_t));

    struct slow_worker_data slow_worker_data = {.slow_thread = diskspace_slow_thread, .update_every = update_every};

    netdata_thread_create(
        diskspace_slow_thread,
        THREAD_DISKSPACE_SLOW_NAME,
        NETDATA_THREAD_OPTION_JOINABLE,
        diskspace_slow_worker,
        &slow_worker_data);

    usec_t step = update_every * USEC_PER_SEC;
    heartbeat_t hb;
    heartbeat_init(&hb);
    while(!netdata_exit) {
        worker_is_idle();
        /* usec_t hb_dt = */ heartbeat_next(&hb, step);

        if(unlikely(netdata_exit)) break;

        // --------------------------------------------------------------------------
        // this is smart enough not to reload it every time

        worker_is_busy(WORKER_JOB_MOUNTINFO);
        mountinfo_reload(0);

        // --------------------------------------------------------------------------
        // disk space metrics

        netdata_mutex_lock(&slow_mountinfo_mutex);
        free_basic_mountinfo_list(slow_mountinfo_tmp_root);
        slow_mountinfo_tmp_root = NULL;

        struct mountinfo *mi;
        for(mi = disk_mountinfo_root; mi; mi = mi->next) {
            if(unlikely(mi->flags & (MOUNTINFO_IS_DUMMY | MOUNTINFO_IS_BIND)))
                continue;

            // exclude mounts made by ProtectHome and ProtectSystem systemd hardening options
            // https://github.com/netdata/netdata/issues/11498#issuecomment-950982878
            if(mi->flags & MOUNTINFO_READONLY && mi->flags & MOUNTINFO_IS_IN_SYSD_PROTECTED_LIST && !strcmp(mi->root, mi->mount_point))
                continue;

            worker_is_busy(WORKER_JOB_MOUNTPOINT);
            do_disk_space_stats(mi, update_every);
            if(unlikely(netdata_exit)) break;
        }
        netdata_mutex_unlock(&slow_mountinfo_mutex);

        if(unlikely(netdata_exit)) break;

        if(dict_mountpoints) {
            worker_is_busy(WORKER_JOB_CLEANUP);
            dictionary_walkthrough_read(dict_mountpoints, mount_point_cleanup_cb, NULL);
        }

    }
    worker_unregister();

    netdata_thread_cleanup_pop(1);
    return NULL;
}
