#include "common.h"

#define DELAULT_EXLUDED_PATHS "/proc/* /sys/* /var/run/user/* /run/user/* /snap/* /var/lib/docker/*"
#define DEFAULT_EXCLUDED_FILESYSTEMS ""
#define CONFIG_SECTION_DISKSPACE "plugin:proc:diskspace"

static struct mountinfo *disk_mountinfo_root = NULL;
static int check_for_new_mountpoints_every = 15;
static int cleanup_mount_points = 1;

static inline void mountinfo_reload(int force) {
    static time_t last_loaded = 0;
    time_t now = now_realtime_sec();

    if(force || now - last_loaded >= check_for_new_mountpoints_every) {
        // mountinfo_free() can be called with NULL disk_mountinfo_root
        mountinfo_free(disk_mountinfo_root);

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

#define rrdset_obsolete_and_pointer_null(st) do { if(st) { rrdset_is_obsolete(st); st = NULL; } } while(st)

int mount_point_cleanup(void *entry, void *data) {
    (void)data;

    struct mount_point_metadata *mp = (struct mount_point_metadata *)entry;
    if(!mp) return 0;

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

static inline void do_disk_space_stats(struct mountinfo *mi, int update_every) {
    const char *family = mi->mount_point;
    const char *disk = mi->persistent_id;

    static SIMPLE_PATTERN *excluded_mountpoints = NULL;
    static SIMPLE_PATTERN *excluded_filesystems = NULL;
    int do_space, do_inodes;

    if(unlikely(!dict_mountpoints)) {
        SIMPLE_PREFIX_MODE mode = SIMPLE_PATTERN_EXACT;

        if(config_move("plugin:proc:/proc/diskstats", "exclude space metrics on paths", CONFIG_SECTION_DISKSPACE, "exclude space metrics on paths") != -1) {
            // old configuration, enable backwards compatibility
            mode = SIMPLE_PATTERN_PREFIX;
        }

        excluded_mountpoints = simple_pattern_create(
                config_get(CONFIG_SECTION_DISKSPACE, "exclude space metrics on paths", DELAULT_EXLUDED_PATHS),
                mode
        );

        excluded_filesystems = simple_pattern_create(
                config_get(CONFIG_SECTION_DISKSPACE, "exclude space metrics on filesystems", DEFAULT_EXCLUDED_FILESYSTEMS),
                SIMPLE_PATTERN_EXACT
        );

        dict_mountpoints = dictionary_create(DICTIONARY_FLAG_SINGLE_THREADED);
    }

    struct mount_point_metadata *m = dictionary_get(dict_mountpoints, mi->mount_point);
    if(unlikely(!m)) {
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
        {
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

    m->updated = 1;

    if(unlikely(m->do_space == CONFIG_BOOLEAN_NO && m->do_inodes == CONFIG_BOOLEAN_NO))
        return;

    if(unlikely(mi->flags & MOUNTINFO_READONLY && !m->collected))
        return;

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

    if(m->do_space == CONFIG_BOOLEAN_YES || (m->do_space == CONFIG_BOOLEAN_AUTO && (bavail || breserved_root || bused))) {
        if(unlikely(!m->st_space)) {
            m->do_space = CONFIG_BOOLEAN_YES;
            m->st_space = rrdset_find_bytype_localhost("disk_space", disk);
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
                        , "GB"
                        , 2023
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

    if(m->do_inodes == CONFIG_BOOLEAN_YES || (m->do_inodes == CONFIG_BOOLEAN_AUTO && (favail || freserved_root || fused))) {
        if(unlikely(!m->st_inodes)) {
            m->do_inodes = CONFIG_BOOLEAN_YES;
            m->st_inodes = rrdset_find_bytype_localhost("disk_inodes", disk);
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
                        , "Inodes"
                        , 2024
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
}

void *proc_diskspace_main(void *ptr) {
    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)ptr;

    info("DISKSPACE thread created with task id %d", gettid());

    if(pthread_setcanceltype(PTHREAD_CANCEL_DEFERRED, NULL) != 0)
        error("DISKSPACE: Cannot set pthread cancel type to DEFERRED.");

    if(pthread_setcancelstate(PTHREAD_CANCEL_ENABLE, NULL) != 0)
        error("DISKSPACE: Cannot set pthread cancel state to ENABLE.");

    int vdo_cpu_netdata = config_get_boolean("plugin:proc", "netdata server resources", 1);

    cleanup_mount_points = config_get_boolean(CONFIG_SECTION_DISKSPACE, "remove charts of unmounted disks" , cleanup_mount_points);

    int update_every = (int)config_get_number(CONFIG_SECTION_DISKSPACE, "update every", localhost->rrd_update_every);
    if(update_every < localhost->rrd_update_every)
        update_every = localhost->rrd_update_every;

    check_for_new_mountpoints_every = (int)config_get_number(CONFIG_SECTION_DISKSPACE, "check for new mount points every", check_for_new_mountpoints_every);
    if(check_for_new_mountpoints_every < update_every)
        check_for_new_mountpoints_every = update_every;

    struct rusage thread;

    usec_t duration = 0;
    usec_t step = update_every * USEC_PER_SEC;
    heartbeat_t hb;
    heartbeat_init(&hb);
    for(;;) {
        duration = heartbeat_dt_usec(&hb);
        /* usec_t hb_dt = */ heartbeat_next(&hb, step);

        if(unlikely(netdata_exit)) break;


        // --------------------------------------------------------------------------
        // this is smart enough not to reload it every time

        mountinfo_reload(0);


        // --------------------------------------------------------------------------
        // disk space metrics

        struct mountinfo *mi;
        for(mi = disk_mountinfo_root; mi; mi = mi->next) {

            if(unlikely(mi->flags & (MOUNTINFO_IS_DUMMY | MOUNTINFO_IS_BIND)))
                continue;

            do_disk_space_stats(mi, update_every);
            if(unlikely(netdata_exit)) break;
        }

        if(unlikely(netdata_exit)) break;

        if(dict_mountpoints)
            dictionary_get_all(dict_mountpoints, mount_point_cleanup, NULL);

        if(vdo_cpu_netdata) {
            static RRDSET *stcpu_thread = NULL, *st_duration = NULL;
            static RRDDIM *rd_user = NULL, *rd_system = NULL, *rd_duration = NULL;

            // ----------------------------------------------------------------

            getrusage(RUSAGE_THREAD, &thread);

            if(!stcpu_thread) {
                stcpu_thread = rrdset_find_localhost("netdata.plugin_diskspace");
                if(!stcpu_thread)
                    stcpu_thread = rrdset_create_localhost(
                            "netdata"
                            , "plugin_diskspace"
                            , NULL
                            , "diskspace"
                            , NULL
                            , "NetData Disk Space Plugin CPU usage"
                            , "milliseconds/s"
                            , 132020
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

            if(!st_duration) {
                st_duration = rrdset_find_localhost("netdata.plugin_diskspace_dt");
                if(!st_duration)
                    st_duration = rrdset_create_localhost(
                            "netdata"
                            , "plugin_diskspace_dt"
                            , NULL
                            , "diskspace"
                            , NULL
                            , "NetData Disk Space Plugin Duration"
                            , "milliseconds/run"
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

    info("DISKSPACE thread exiting");

    static_thread->enabled = 0;
    pthread_exit(NULL);
    return NULL;
}
