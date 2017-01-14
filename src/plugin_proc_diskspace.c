#include "common.h"

#ifndef NETDATA_RELOAD_MOUNTINFO_EVERY
#define NETDATA_RELOAD_MOUNTINFO_EVERY 60
#endif

#define DELAULT_EXLUDED_PATHS "/proc/ /sys/ /var/run/user/ /run/user/"

static struct mountinfo *disk_mountinfo_root = NULL;

static inline void mountinfo_reload(int force) {
    static time_t last_loaded = 0;
    time_t now = now_realtime_sec();

    if(force || now - last_loaded >= NETDATA_RELOAD_MOUNTINFO_EVERY) {
        // mountinfo_free() can be called with NULL disk_mountinfo_root
        mountinfo_free(disk_mountinfo_root);

        // re-read mountinfo in case something changed
        disk_mountinfo_root = mountinfo_read();

        last_loaded = now;
    }
}

// Data to be stored in DICTIONARY mount_points used by do_disk_space_stats().
// This DICTIONARY is used to lookup the settings of the mount point on each iteration.
struct mount_point_metadata {
    int do_space;
    int do_inodes;
    int update_every;
};

static inline void do_disk_space_stats(struct mountinfo *mi, int update_every) {
    const char *family = mi->mount_point;
    const char *disk = mi->persistent_id;

    static DICTIONARY *mount_points = NULL;
    static NETDATA_SIMPLE_PATTERN *excluded_mountpoints = NULL;
    int do_space, do_inodes;

    if(unlikely(!mount_points)) {
        const char *s;

        if(config_exists("plugin:proc:/proc/diskstats", "exclude space metrics on paths") && !config_exists("plugin:proc:diskspace", "exclude space metrics on paths")) {
            // the config exists in the old section
            s = config_get("plugin:proc:/proc/diskstats", "exclude space metrics on paths", DELAULT_EXLUDED_PATHS);

            // set it to the new section
            config_set("plugin:proc:diskspace", "exclude space metrics on paths", s);
        }
        else
            s = config_get("plugin:proc:diskspace", "exclude space metrics on paths", DELAULT_EXLUDED_PATHS);

        mount_points = dictionary_create(DICTIONARY_FLAG_SINGLE_THREADED);
        excluded_mountpoints = netdata_simple_pattern_list_create(s, NETDATA_SIMPLE_PATTERN_MODE_PREFIX);
    }

    struct mount_point_metadata *m = dictionary_get(mount_points, mi->mount_point);
    if(unlikely(!m)) {
        char var_name[4096 + 1];
        snprintfz(var_name, 4096, "plugin:proc:diskspace:%s", mi->mount_point);

        int def_space = config_get_boolean_ondemand("plugin:proc:diskspace", "space usage for all disks", CONFIG_ONDEMAND_ONDEMAND);
        int def_inodes = config_get_boolean_ondemand("plugin:proc:diskspace", "inodes usage for all disks", CONFIG_ONDEMAND_ONDEMAND);

        if(unlikely(netdata_simple_pattern_list_matches(excluded_mountpoints, mi->mount_point))) {
            def_space = CONFIG_ONDEMAND_NO;
            def_inodes = CONFIG_ONDEMAND_NO;
        }

        do_space = config_get_boolean_ondemand(var_name, "space usage", def_space);
        do_inodes = config_get_boolean_ondemand(var_name, "inodes usage", def_inodes);

        struct mount_point_metadata mp = {
                .do_space = do_space,
                .do_inodes = do_inodes,
                .update_every = rrd_update_every
        };

        dictionary_set(mount_points, mi->mount_point, &mp, sizeof(struct mount_point_metadata));
    }
    else {
        do_space = m->do_space;
        do_inodes = m->do_inodes;
        update_every = m->update_every;
    }

    if(unlikely(do_space == CONFIG_ONDEMAND_NO && do_inodes == CONFIG_ONDEMAND_NO))
        return;

    struct statvfs buff_statvfs;
    if (statvfs(mi->mount_point, &buff_statvfs) < 0) {
        error("Failed statvfs() for '%s' (disk '%s')", mi->mount_point, disk);
        return;
    }

    // taken from get_fs_usage() found in coreutils
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
        error("Disk block statistics for '%s' (disk '%s') do not sum up: total = %llu, available = %llu, reserved = %llu, used = %llu", mi->mount_point, disk, (unsigned long long)btotal, (unsigned long long)bavail, (unsigned long long)breserved_root, (unsigned long long)bused);
#endif

    // --------------------------------------------------------------------------

    fsfilcnt_t favail         = buff_statvfs.f_favail;
    fsfilcnt_t ftotal         = buff_statvfs.f_files;
    fsfilcnt_t favail_root    = buff_statvfs.f_ffree;
    fsfilcnt_t freserved_root = favail_root - favail;
    fsfilcnt_t fused          = ftotal - favail_root;

#ifdef NETDATA_INTERNAL_CHECKS
    if(unlikely(btotal != bavail + breserved_root + bused))
        error("Disk inode statistics for '%s' (disk '%s') do not sum up: total = %llu, available = %llu, reserved = %llu, used = %llu", mi->mount_point, disk, (unsigned long long)ftotal, (unsigned long long)favail, (unsigned long long)freserved_root, (unsigned long long)fused);
#endif

    // --------------------------------------------------------------------------

    RRDSET *st;

    if(do_space == CONFIG_ONDEMAND_YES || (do_space == CONFIG_ONDEMAND_ONDEMAND && (bavail || breserved_root || bused))) {
        st = rrdset_find_bytype("disk_space", disk);
        if(unlikely(!st)) {
            char title[4096 + 1];
            snprintfz(title, 4096, "Disk Space Usage for %s [%s]", family, mi->mount_source);
            st = rrdset_create("disk_space", disk, NULL, family, "disk.space", title, "GB", 2023, update_every, RRDSET_TYPE_STACKED);

            rrddim_add(st, "avail", NULL, bsize, 1024*1024*1024, RRDDIM_ABSOLUTE);
            rrddim_add(st, "used" , NULL, bsize, 1024*1024*1024, RRDDIM_ABSOLUTE);
            rrddim_add(st, "reserved_for_root", "reserved for root", bsize, 1024*1024*1024, RRDDIM_ABSOLUTE);
        }
        else rrdset_next(st);

        rrddim_set(st, "avail", (collected_number)bavail);
        rrddim_set(st, "used", (collected_number)bused);
        rrddim_set(st, "reserved_for_root", (collected_number)breserved_root);
        rrdset_done(st);
    }

    // --------------------------------------------------------------------------

    if(do_inodes == CONFIG_ONDEMAND_YES || (do_inodes == CONFIG_ONDEMAND_ONDEMAND && (favail || freserved_root || fused))) {
        st = rrdset_find_bytype("disk_inodes", disk);
        if(unlikely(!st)) {
            char title[4096 + 1];
            snprintfz(title, 4096, "Disk Files (inodes) Usage for %s [%s]", family, mi->mount_source);
            st = rrdset_create("disk_inodes", disk, NULL, family, "disk.inodes", title, "Inodes", 2024, update_every, RRDSET_TYPE_STACKED);

            rrddim_add(st, "avail", NULL, 1, 1, RRDDIM_ABSOLUTE);
            rrddim_add(st, "used" , NULL, 1, 1, RRDDIM_ABSOLUTE);
            rrddim_add(st, "reserved_for_root", "reserved for root", 1, 1, RRDDIM_ABSOLUTE);
        }
        else rrdset_next(st);

        rrddim_set(st, "avail", (collected_number)favail);
        rrddim_set(st, "used", (collected_number)fused);
        rrddim_set(st, "reserved_for_root", (collected_number)freserved_root);
        rrdset_done(st);
    }
}

void *proc_diskspace_main(void *ptr) {
    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)ptr;

    info("DISKSPACE thread created with task id %d", gettid());

    if(pthread_setcanceltype(PTHREAD_CANCEL_DEFERRED, NULL) != 0)
        error("Cannot set pthread cancel type to DEFERRED.");

    if(pthread_setcancelstate(PTHREAD_CANCEL_ENABLE, NULL) != 0)
        error("Cannot set pthread cancel state to ENABLE.");

    int update_every = (int)config_get_number("plugin:proc:diskspace", "update every", rrd_update_every);
    if(update_every < rrd_update_every)
        update_every = rrd_update_every;

    usec_t step = update_every * USEC_PER_SEC;
    for(;;) {
        usec_t now = now_monotonic_usec();
        usec_t next = now - (now % step) + step;

        while(now < next) {
            sleep_usec(next - now);
            now = now_monotonic_usec();
        }

        if(unlikely(netdata_exit)) break;

        // --------------------------------------------------------------------------
        // this is smart enough not to reload it every time

        mountinfo_reload(0);

        // --------------------------------------------------------------------------
        // disk space metrics

        struct mountinfo *mi;
        for(mi = disk_mountinfo_root; mi; mi = mi->next) {

            if(unlikely(mi->flags &
                        (MOUNTINFO_IS_DUMMY | MOUNTINFO_IS_BIND | MOUNTINFO_IS_SAME_DEV | MOUNTINFO_NO_STAT |
                         MOUNTINFO_NO_SIZE | MOUNTINFO_READONLY)))
                continue;

            do_disk_space_stats(mi, update_every);
        }
    }

    info("DISKSPACE thread exiting");

    static_thread->enabled = 0;
    static_thread->thread = NULL;
    pthread_exit(NULL);
    return NULL;
}
