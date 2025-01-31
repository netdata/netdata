// SPDX-License-Identifier: GPL-3.0-or-later

#include "../proc.plugin/plugin_proc.h"

#define PLUGIN_DISKSPACE_NAME "diskspace.plugin"

#define DEFAULT_EXCLUDED_PATHS "/dev /dev/shm /proc/* /sys/* /var/run/user/* /run/lock /run/user/* /snap/* /var/lib/docker/* /var/lib/containers/storage/* /run/credentials/* /run/containerd/*  /rpool /rpool/*"
#define DEFAULT_EXCLUDED_FILESYSTEMS "*gvfs *gluster* *s3fs *ipfs *davfs2 *httpfs *sshfs *gdfs *moosefs fusectl autofs cgroup cgroup2 hugetlbfs devtmpfs fuse.lxcfs"
#define DEFAULT_EXCLUDED_FILESYSTEMS_INODES "msdosfs msdos vfat overlayfs aufs* *unionfs"
#define CONFIG_SECTION_DISKSPACE "plugin:proc:diskspace"

#define RRDFUNCTIONS_DISKSPACE_HELP "View mount point statistics"

#define MAX_STAT_USEC 10000LU
#define SLOW_UPDATE_EVERY 5

static ND_THREAD *diskspace_slow_thread = NULL;

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

    bool shown_error;
    bool updated;
    bool slow;

    STRING *filesystem;
    STRING *mountroot;

    RRDLABELS *chart_labels;

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

#define rrdset_obsolete_and_pointer_null(st) do { if(st) { rrdset_is_obsolete___safe_from_collector_thread(st); (st) = NULL; } } while(st)

static void mount_points_cleanup(bool slow) {
    struct mount_point_metadata *mp;
    dfe_start_write(dict_mountpoints, mp) {
        if(mp->slow != slow) continue;

        if(mp->updated)
            mp->updated = false;
        else if(cleanup_mount_points)
            dictionary_del(dict_mountpoints, mp_dfe.name);
    }
    dfe_done(mp);

    dictionary_garbage_collect(dict_mountpoints);
}

void mountpoint_delete_cb(const DICTIONARY_ITEM *item __maybe_unused, void *entry, void *data __maybe_unused) {
    struct mount_point_metadata *mp = (struct mount_point_metadata *)entry;

    mp->collected = 0;
    mp->updated = false;
    mp->shown_error = false;

    string_freez(mp->filesystem);
    mp->filesystem = NULL;

    string_freez(mp->mountroot);
    mp->mountroot = NULL;

    rrdset_obsolete_and_pointer_null(mp->st_space);
    rrdset_obsolete_and_pointer_null(mp->st_inodes);

    mp->rd_space_avail = NULL;
    mp->rd_space_used = NULL;
    mp->rd_space_reserved = NULL;

    mp->rd_inodes_avail = NULL;
    mp->rd_inodes_used = NULL;
    mp->rd_inodes_reserved = NULL;
}

// a copy of basic mountinfo fields
struct basic_mountinfo {
    char *persistent_id;    
    char *root;             
    char *mount_point_stat_path;
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
        bmi->root = strdupz(mi->root);
        bmi->mount_point_stat_path = strdupz(mi->mount_point_stat_path);
        bmi->mount_point = strdupz(mi->mount_point);
        bmi->filesystem = strdupz(mi->filesystem);
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
}

static void free_basic_mountinfo(struct basic_mountinfo *bmi)
{
    if (bmi) {
        freez(bmi->persistent_id);
        freez(bmi->root);
        freez(bmi->mount_point_stat_path);
        freez(bmi->mount_point);
        freez(bmi->filesystem);

        freez(bmi);
    }
}

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
        collector_error("DISKSPACE: disk block statistics for '%s' (disk '%s') do not sum up: total = %llu, available = %llu, reserved = %llu, used = %llu", mi->mount_point, disk, (unsigned long long)btotal, (unsigned long long)bavail, (unsigned long long)breserved_root, (unsigned long long)bused);
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
        collector_error("DISKSPACE: disk inode statistics for '%s' (disk '%s') do not sum up: total = %llu, available = %llu, reserved = %llu, used = %llu", mi->mount_point, disk, (unsigned long long)ftotal, (unsigned long long)favail, (unsigned long long)freserved_root, (unsigned long long)fused);
#endif

    int rendered = 0;

    if (m->do_space == CONFIG_BOOLEAN_YES || m->do_space == CONFIG_BOOLEAN_AUTO) {
        if(unlikely(!m->st_space) || m->st_space->update_every != update_every) {
            m->do_space = CONFIG_BOOLEAN_YES;
            m->st_space = rrdset_find_active_bytype_localhost("disk_space", disk);
            if(unlikely(!m->st_space || m->st_space->update_every != update_every)) {
                char title[4096 + 1];
                snprintfz(title, sizeof(title) - 1, "Disk Space Usage");
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

        rrddim_set_by_pointer(m->st_space, m->rd_space_avail,    (collected_number)bavail);
        rrddim_set_by_pointer(m->st_space, m->rd_space_used,     (collected_number)bused);
        rrddim_set_by_pointer(m->st_space, m->rd_space_reserved, (collected_number)breserved_root);
        rrdset_done(m->st_space);

        rendered++;
    }

    if (m->do_inodes == CONFIG_BOOLEAN_YES || m->do_inodes == CONFIG_BOOLEAN_AUTO) {
        if(unlikely(!m->st_inodes) || m->st_inodes->update_every != update_every) {
            m->do_inodes = CONFIG_BOOLEAN_YES;
            m->st_inodes = rrdset_find_active_bytype_localhost("disk_inodes", disk);
            if(unlikely(!m->st_inodes) || m->st_inodes->update_every != update_every) {
                char title[4096 + 1];
                snprintfz(title, sizeof(title) - 1, "Disk Files (inodes) Usage");
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

        rrddim_set_by_pointer(m->st_inodes, m->rd_inodes_avail,    (collected_number)favail);
        rrddim_set_by_pointer(m->st_inodes, m->rd_inodes_used,     (collected_number)fused);
        rrddim_set_by_pointer(m->st_inodes, m->rd_inodes_reserved, (collected_number)freserved_root);
        rrdset_done(m->st_inodes);

        rendered++;
    }

    if(likely(rendered))
        m->collected++;
}

static inline void do_disk_space_stats(struct mountinfo *mi, int update_every) {
    const char *disk = mi->persistent_id;

    static SIMPLE_PATTERN *excluded_mountpoints = NULL;
    static SIMPLE_PATTERN *excluded_filesystems = NULL;
    static SIMPLE_PATTERN *excluded_filesystems_inodes = NULL;

    usec_t slow_timeout = MAX_STAT_USEC * update_every;

    int do_space, do_inodes;

    if(unlikely(!dict_mountpoints)) {
        SIMPLE_PREFIX_MODE mode = SIMPLE_PATTERN_EXACT;

        if(inicfg_move(&netdata_config, "plugin:proc:/proc/diskstats", "exclude space metrics on paths", CONFIG_SECTION_DISKSPACE, "exclude space metrics on paths") != -1) {
            // old configuration, enable backwards compatibility
            mode = SIMPLE_PATTERN_PREFIX;
        }

        excluded_mountpoints = simple_pattern_create(
            inicfg_get(&netdata_config, CONFIG_SECTION_DISKSPACE, "exclude space metrics on paths", DEFAULT_EXCLUDED_PATHS),
            NULL,
            mode,
            true);

        excluded_filesystems = simple_pattern_create(
            inicfg_get(&netdata_config, CONFIG_SECTION_DISKSPACE, "exclude space metrics on filesystems", DEFAULT_EXCLUDED_FILESYSTEMS),
            NULL,
            SIMPLE_PATTERN_EXACT,
            true);

        excluded_filesystems_inodes = simple_pattern_create(
            inicfg_get(&netdata_config, CONFIG_SECTION_DISKSPACE, "exclude inode metrics on filesystems", DEFAULT_EXCLUDED_FILESYSTEMS_INODES),
            NULL,
            SIMPLE_PATTERN_EXACT,
            true);

        dict_mountpoints = dictionary_create_advanced(DICT_OPTION_FIXED_SIZE, &dictionary_stats_category_collectors, sizeof(struct mount_point_metadata));
        dictionary_register_delete_callback(dict_mountpoints, mountpoint_delete_cb, NULL);
    }

    const DICTIONARY_ITEM *item = dictionary_get_and_acquire_item(dict_mountpoints, mi->mount_point);
    if(unlikely(!item)) {
        bool slow = false;

        int def_space = inicfg_get_boolean_ondemand(&netdata_config, CONFIG_SECTION_DISKSPACE, "space usage for all disks", CONFIG_BOOLEAN_AUTO);
        int def_inodes = inicfg_get_boolean_ondemand(&netdata_config, CONFIG_SECTION_DISKSPACE, "inodes usage for all disks", CONFIG_BOOLEAN_AUTO);

        if(unlikely(simple_pattern_matches(excluded_mountpoints, mi->mount_point))) {
            def_space = CONFIG_BOOLEAN_NO;
            def_inodes = CONFIG_BOOLEAN_NO;
        }

        if(unlikely(simple_pattern_matches(excluded_filesystems, mi->filesystem))) {
            def_space = CONFIG_BOOLEAN_NO;
            def_inodes = CONFIG_BOOLEAN_NO;
        }
        if (unlikely(simple_pattern_matches(excluded_filesystems_inodes, mi->filesystem))) {
            def_inodes = CONFIG_BOOLEAN_NO;
        }

        // check if the mount point is a directory #2407
        // but only when it is enabled by default #4491
        if(def_space != CONFIG_BOOLEAN_NO || def_inodes != CONFIG_BOOLEAN_NO) {
            usec_t start_time = now_monotonic_high_precision_usec();
            struct stat bs;

            if(stat(mi->mount_point_stat_path, &bs) == -1) {
                collector_error("DISKSPACE: Cannot stat() mount point '%s' (disk '%s', filesystem '%s', root '%s')."
                               , mi->mount_point_stat_path
                               , disk
                               , mi->filesystem?mi->filesystem:""
                               , mi->root?mi->root:""
                               );
                def_space = CONFIG_BOOLEAN_NO;
                def_inodes = CONFIG_BOOLEAN_NO;
            }
            else {
                if((bs.st_mode & S_IFMT) != S_IFDIR) {
                    collector_error("DISKSPACE: Mount point '%s' (disk '%s', filesystem '%s', root '%s') is not a directory."
                                   , mi->mount_point_stat_path
                                   , disk
                                   , mi->filesystem?mi->filesystem:""
                                   , mi->root?mi->root:""
                                   );
                    def_space = CONFIG_BOOLEAN_NO;
                    def_inodes = CONFIG_BOOLEAN_NO;
                }
            }

            if ((now_monotonic_high_precision_usec() - start_time) > slow_timeout)
                slow = true;
        }

        char var_name[4096 + 1];
        snprintfz(var_name, 4096, "plugin:proc:diskspace:%s", mi->mount_point);

        do_space = def_space;
        do_inodes = def_inodes;

        if (inicfg_exists(&netdata_config, var_name, "space usage"))
            do_space = inicfg_get_boolean_ondemand(&netdata_config, var_name, "space usage", def_space);
        if (inicfg_exists(&netdata_config, var_name, "inodes usage"))
            do_inodes = inicfg_get_boolean_ondemand(&netdata_config, var_name, "inodes usage", def_inodes);

        struct mount_point_metadata mp = {
            .do_space = do_space,
            .do_inodes = do_inodes,
            .shown_error = false,
            .updated = false,
            .slow = slow,

            .collected = 0,
            .filesystem = string_strdupz(mi->filesystem),
            .mountroot = string_strdupz(mi->root),
            .chart_labels = rrdlabels_create(),

            .st_space = NULL,
            .rd_space_avail = NULL,
            .rd_space_used = NULL,
            .rd_space_reserved = NULL,

            .st_inodes = NULL,
            .rd_inodes_avail = NULL,
            .rd_inodes_used = NULL,
            .rd_inodes_reserved = NULL
        };

        rrdlabels_add(mp.chart_labels, "mount_point", mi->mount_point, RRDLABEL_SRC_AUTO);
        rrdlabels_add(mp.chart_labels, "filesystem", mi->filesystem, RRDLABEL_SRC_AUTO);
        rrdlabels_add(mp.chart_labels, "mount_root", mi->root, RRDLABEL_SRC_AUTO);

        item = dictionary_set_and_acquire_item(dict_mountpoints, mi->mount_point, &mp, sizeof(struct mount_point_metadata));
    }

    struct mount_point_metadata *m = dictionary_acquired_item_value(item);
    if (m->slow) {
        add_basic_mountinfo(&slow_mountinfo_tmp_root, mi);
        goto cleanup;
    }

    m->updated = true;

    if(unlikely(m->do_space == CONFIG_BOOLEAN_NO && m->do_inodes == CONFIG_BOOLEAN_NO)) {
        goto cleanup;
    }

    if (unlikely(
            mi->flags & MOUNTINFO_READONLY &&
            !(mi->flags & MOUNTINFO_IS_IN_SYSD_PROTECTED_LIST) &&
            !m->collected &&
            m->do_space != CONFIG_BOOLEAN_YES &&
            m->do_inodes != CONFIG_BOOLEAN_YES)) {
        goto cleanup;
    }

    usec_t start_time = now_monotonic_high_precision_usec();
    struct statvfs buff_statvfs;

    if (statvfs(mi->mount_point_stat_path, &buff_statvfs) < 0) {
        if(!m->shown_error) {
            collector_error("DISKSPACE: failed to statvfs() mount point '%s' (disk '%s', filesystem '%s', root '%s')"
                            , mi->mount_point_stat_path
                            , disk
                            , mi->filesystem?mi->filesystem:""
                            , mi->root?mi->root:""
                            );
            m->shown_error = true;
        }
        goto cleanup;
    }

    if ((now_monotonic_high_precision_usec() - start_time) > slow_timeout)
        m->slow = true;

    m->shown_error = false;

    struct basic_mountinfo bmi;
    bmi.mount_point = mi->mount_point;
    bmi.persistent_id = mi->persistent_id;
    bmi.filesystem = mi->filesystem;
    bmi.root = mi->root;

    calculate_values_and_show_charts(&bmi, m, &buff_statvfs, update_every);

cleanup:
    dictionary_acquired_item_release(dict_mountpoints, item);
}

static inline void do_slow_disk_space_stats(struct basic_mountinfo *mi, int update_every) {
    const DICTIONARY_ITEM *item = dictionary_get_and_acquire_item(dict_mountpoints, mi->mount_point);
    if(!item) return;

    struct mount_point_metadata *m = dictionary_acquired_item_value(item);
    m->updated = true;

    struct statvfs buff_statvfs;
    if (statvfs(mi->mount_point_stat_path, &buff_statvfs) < 0) {
        if(!m->shown_error) {
            collector_error("DISKSPACE: failed to statvfs() mount point '%s' (disk '%s', filesystem '%s', root '%s')"
                            , mi->mount_point_stat_path
                            , mi->persistent_id
                            , mi->filesystem?mi->filesystem:""
                            , mi->root?mi->root:""
                           );
            m->shown_error = true;
        }
        goto cleanup;
    }
    m->shown_error = false;

    calculate_values_and_show_charts(mi, m, &buff_statvfs, update_every);

cleanup:
    dictionary_acquired_item_release(dict_mountpoints, item);
}

static void diskspace_slow_worker_cleanup(void *pptr) {
    struct slow_worker_data *data = CLEANUP_FUNCTION_GET_PTR(pptr);
    if(data) return;

    worker_unregister();
}

#define WORKER_JOB_SLOW_MOUNTPOINT 0
#define WORKER_JOB_SLOW_CLEANUP 1

struct slow_worker_data {
    int update_every;
};

void *diskspace_slow_worker(void *ptr)
{
    struct slow_worker_data *data = (struct slow_worker_data *)ptr;
    CLEANUP_FUNCTION_REGISTER(diskspace_slow_worker_cleanup) cleanup_ptr = data;

    worker_register("DISKSPACE_SLOW");
    worker_register_job_name(WORKER_JOB_SLOW_MOUNTPOINT, "mountpoint");
    worker_register_job_name(WORKER_JOB_SLOW_CLEANUP, "cleanup");

    struct basic_mountinfo *slow_mountinfo_root = NULL;

    int slow_update_every = data->update_every > SLOW_UPDATE_EVERY ? data->update_every : SLOW_UPDATE_EVERY;

    usec_t step = slow_update_every * USEC_PER_SEC;
    usec_t real_step = USEC_PER_SEC;
    heartbeat_t hb;
    heartbeat_init(&hb, USEC_PER_SEC);

    while(service_running(SERVICE_COLLECTORS)) {
        worker_is_idle();
        heartbeat_next(&hb);

        if (real_step < step) {
            real_step += USEC_PER_SEC;
            continue;
        }
        real_step = USEC_PER_SEC;

        usec_t start_time = now_monotonic_high_precision_usec();

        if (!dict_mountpoints)
            continue;

        if(unlikely(!service_running(SERVICE_COLLECTORS))) break;

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
            
            if(unlikely(!service_running(SERVICE_COLLECTORS))) break;
        }

        if(unlikely(!service_running(SERVICE_COLLECTORS))) break;

        if(dict_mountpoints) {
            worker_is_busy(WORKER_JOB_SLOW_CLEANUP);
            mount_points_cleanup(true);
        }

        usec_t dt = now_monotonic_high_precision_usec() - start_time;
        if (dt > step) {
            slow_update_every = (dt / USEC_PER_SEC) * 3 / 2;
            if (slow_update_every % SLOW_UPDATE_EVERY)
                slow_update_every += SLOW_UPDATE_EVERY - slow_update_every % SLOW_UPDATE_EVERY;
            step = slow_update_every * USEC_PER_SEC;
        }
    }

    free_basic_mountinfo_list(slow_mountinfo_root);

    return NULL;
}

static void diskspace_main_cleanup(void *pptr) {
    struct netdata_static_thread *static_thread = CLEANUP_FUNCTION_GET_PTR(pptr);
    if(!static_thread) return;

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;

    rrd_collector_finished();
    worker_unregister();

    if (diskspace_slow_thread)
        nd_thread_join(diskspace_slow_thread);

    netdata_mutex_lock(&slow_mountinfo_mutex);
    free_basic_mountinfo_list(slow_mountinfo_tmp_root);
    netdata_mutex_unlock(&slow_mountinfo_mutex);

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
}

#define WORKER_JOB_MOUNTINFO 0
#define WORKER_JOB_MOUNTPOINT 1
#define WORKER_JOB_CLEANUP 2

#if WORKER_UTILIZATION_MAX_JOB_TYPES < 3
#error WORKER_UTILIZATION_MAX_JOB_TYPES has to be at least 3
#endif

static int diskspace_function_mount_points(BUFFER *wb, const char *function __maybe_unused, BUFFER *payload __maybe_unused, const char *source __maybe_unused) {
    netdata_mutex_lock(&slow_mountinfo_mutex);

    buffer_flush(wb);
    wb->content_type = CT_APPLICATION_JSON;
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_DEFAULT);

    buffer_json_member_add_string(wb, "hostname", rrdhost_hostname(localhost));
    buffer_json_member_add_uint64(wb, "status", HTTP_RESP_OK);
    buffer_json_member_add_string(wb, "type", "table");
    buffer_json_member_add_time_t(wb, "update_every", 1);
    buffer_json_member_add_boolean(wb, "has_history", false);
    buffer_json_member_add_string(wb, "help", RRDFUNCTIONS_DISKSPACE_HELP);
    buffer_json_member_add_array(wb, "data");

    double max_space_util = 0.0;
    double max_space_avail = 0.0;
    double max_space_used = 0.0;
    double max_space_reserved = 0.0;

    double max_inodes_util = 0.0;
    double max_inodes_avail = 0.0;
    double max_inodes_used = 0.0;
    double max_inodes_reserved = 0.0;

    struct mount_point_metadata *mp;
    dfe_start_read(dict_mountpoints, mp) {
        if (!mp->collected)
            continue;

        buffer_json_add_array_item_array(wb);

        buffer_json_add_array_item_string(wb, mp_dfe.name);
        buffer_json_add_array_item_string(wb, string2str(mp->filesystem));
        buffer_json_add_array_item_string(wb, string2str(mp->mountroot));

        double space_avail = rrddim_get_last_stored_value(mp->rd_space_avail, &max_space_avail, 1.0);
        double space_used = rrddim_get_last_stored_value(mp->rd_space_used, &max_space_used, 1.0);
        double space_reserved = rrddim_get_last_stored_value(mp->rd_space_reserved, &max_space_reserved, 1.0);
        double inodes_avail = rrddim_get_last_stored_value(mp->rd_inodes_avail, &max_inodes_avail, 1.0);
        double inodes_used = rrddim_get_last_stored_value(mp->rd_inodes_used, &max_inodes_used, 1.0);
        double inodes_reserved = rrddim_get_last_stored_value(mp->rd_inodes_reserved, &max_inodes_reserved, 1.0);

        double space_util = NAN;
        if (!isnan(space_avail) && !isnan(space_used)) {
            space_util = space_avail + space_used > 0 ? space_used * 100.0 / (space_avail + space_used) : 0;
            max_space_util = MAX(max_space_util, space_util);
        }
        double inodes_util = NAN;
        if (!isnan(inodes_avail) && !isnan(inodes_used)) {
            inodes_util = inodes_avail + inodes_used > 0 ? inodes_used * 100.0 / (inodes_avail + inodes_used) : 0;
            max_inodes_util = MAX(max_inodes_util, inodes_util);
        }

        buffer_json_add_array_item_double(wb, space_util);
        buffer_json_add_array_item_double(wb, space_avail);
        buffer_json_add_array_item_double(wb, space_used);
        buffer_json_add_array_item_double(wb, space_reserved);

        buffer_json_add_array_item_double(wb, inodes_util);
        buffer_json_add_array_item_double(wb, inodes_avail);
        buffer_json_add_array_item_double(wb, inodes_used);
        buffer_json_add_array_item_double(wb, inodes_reserved);

        buffer_json_array_close(wb);
    }
    dfe_done(mp);

    buffer_json_array_close(wb); // data
    buffer_json_member_add_object(wb, "columns");
    {
        size_t field_id = 0;

        buffer_rrdf_table_add_field(wb, field_id++, "Mountpoint", "Mountpoint Name",
                RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_UNIQUE_KEY | RRDF_FIELD_OPTS_STICKY | RRDF_FIELD_OPTS_FULL_WIDTH,
                NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "Filesystem", "Mountpoint Filesystem",
                RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_UNIQUE_KEY,
                NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "Root", "Mountpoint Root",
                RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                RRDF_FIELD_OPTS_UNIQUE_KEY,
                NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "Used%", "Space Utilization",
                RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER,
                2, "%", max_space_util, RRDF_FIELD_SORT_DESCENDING, NULL,
                RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_VISIBLE,
                NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "Avail", "Space Avail",
                RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER,
                2, "GiB", max_space_avail, RRDF_FIELD_SORT_DESCENDING, NULL,
                RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_VISIBLE,
                NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "Used", "Space Used",
                RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER,
                2, "GiB", max_space_used, RRDF_FIELD_SORT_DESCENDING, NULL,
                RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_VISIBLE,
                NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "Reserved", "Space Reserved for root",
                RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER,
                2, "GiB", max_space_reserved, RRDF_FIELD_SORT_DESCENDING, NULL,
                RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_VISIBLE,
                NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "iUsed%", "Inodes Utilization",
                RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER,
                2, "%", max_inodes_util, RRDF_FIELD_SORT_DESCENDING, NULL,
                RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_NONE,
                NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "iAvail", "Inodes Avail",
                RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER,
                2, "inodes", max_inodes_avail, RRDF_FIELD_SORT_DESCENDING, NULL,
                RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_NONE,
                NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "iUsed", "Inodes Used",
                RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER,
                2, "inodes", max_inodes_used, RRDF_FIELD_SORT_DESCENDING, NULL,
                RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_NONE,
                NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "iReserved", "Inodes Reserved for root",
                RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER,
                2, "inodes", max_inodes_reserved, RRDF_FIELD_SORT_DESCENDING, NULL,
                RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_NONE,
                NULL);
    }

    buffer_json_object_close(wb); // columns
    buffer_json_member_add_string(wb, "default_sort_column", "Used%");

    buffer_json_member_add_object(wb, "charts");
    {
        buffer_json_member_add_object(wb, "Utilization");
        {
            buffer_json_member_add_string(wb, "name", "Utilization");
            buffer_json_member_add_string(wb, "type", "stacked-bar");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "Used%");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "Usage");
        {
            buffer_json_member_add_string(wb, "name", "Usage");
            buffer_json_member_add_string(wb, "type", "stacked-bar");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "Avail");
                buffer_json_add_array_item_string(wb, "Used");
                buffer_json_add_array_item_string(wb, "Reserved");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "Inodes");
        {
            buffer_json_member_add_string(wb, "name", "Inodes");
            buffer_json_member_add_string(wb, "type", "stacked-bar");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "iAvail");
                buffer_json_add_array_item_string(wb, "iUsed");
                buffer_json_add_array_item_string(wb, "iReserved");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);
    }
    buffer_json_object_close(wb); // charts

    buffer_json_member_add_array(wb, "default_charts");
    {
        buffer_json_add_array_item_array(wb);
        buffer_json_add_array_item_string(wb, "Utilization");
        buffer_json_add_array_item_string(wb, "Mountpoint");
        buffer_json_array_close(wb);

        buffer_json_add_array_item_array(wb);
        buffer_json_add_array_item_string(wb, "Usage");
        buffer_json_add_array_item_string(wb, "Mountpoint");
        buffer_json_array_close(wb);
    }
    buffer_json_array_close(wb);

    buffer_json_member_add_time_t(wb, "expires", now_realtime_sec() + 1);
    buffer_json_finalize(wb);

    netdata_mutex_unlock(&slow_mountinfo_mutex);
    return HTTP_RESP_OK;
}

void *diskspace_main(void *ptr) {
    CLEANUP_FUNCTION_REGISTER(diskspace_main_cleanup) cleanup_ptr = ptr;

    worker_register("DISKSPACE");
    worker_register_job_name(WORKER_JOB_MOUNTINFO, "mountinfo");
    worker_register_job_name(WORKER_JOB_MOUNTPOINT, "mountpoint");
    worker_register_job_name(WORKER_JOB_CLEANUP, "cleanup");

    rrd_function_add_inline(localhost, NULL, "mount-points", 10,
                            RRDFUNCTIONS_PRIORITY_DEFAULT, RRDFUNCTIONS_VERSION_DEFAULT,
                            RRDFUNCTIONS_DISKSPACE_HELP,
                            "top", HTTP_ACCESS_ANONYMOUS_DATA,
                            diskspace_function_mount_points);

    cleanup_mount_points = inicfg_get_boolean(&netdata_config, CONFIG_SECTION_DISKSPACE, "remove charts of unmounted disks" , cleanup_mount_points);

    int update_every = (int)inicfg_get_duration_seconds(&netdata_config, CONFIG_SECTION_DISKSPACE, "update every", localhost->rrd_update_every);
    if(update_every < localhost->rrd_update_every) {
        update_every = localhost->rrd_update_every;
        inicfg_set_duration_seconds(&netdata_config, CONFIG_SECTION_DISKSPACE, "update every", update_every);
    }

    check_for_new_mountpoints_every = (int)inicfg_get_duration_seconds(&netdata_config, CONFIG_SECTION_DISKSPACE, "check for new mount points every", check_for_new_mountpoints_every);
    if(check_for_new_mountpoints_every < update_every)
        check_for_new_mountpoints_every = update_every;

    netdata_mutex_init(&slow_mountinfo_mutex);

    struct slow_worker_data slow_worker_data = { .update_every = update_every };

    diskspace_slow_thread = nd_thread_create(
        "P[diskspace slow]",
        NETDATA_THREAD_OPTION_JOINABLE,
        diskspace_slow_worker,
        &slow_worker_data);

    heartbeat_t hb;
    heartbeat_init(&hb, update_every * USEC_PER_SEC);
    while(service_running(SERVICE_COLLECTORS)) {
        worker_is_idle();
        /* usec_t hb_dt = */ heartbeat_next(&hb);

        if(unlikely(!service_running(SERVICE_COLLECTORS))) break;

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
            if(unlikely(!service_running(SERVICE_COLLECTORS))) break;
        }
        netdata_mutex_unlock(&slow_mountinfo_mutex);

        if(unlikely(!service_running(SERVICE_COLLECTORS))) break;

        if(dict_mountpoints) {
            worker_is_busy(WORKER_JOB_CLEANUP);
            mount_points_cleanup(false);
        }
    }
    return NULL;
}
