#include "common.h"

#define RRD_TYPE_DISK "disk"

#define DISK_TYPE_PHYSICAL  1
#define DISK_TYPE_PARTITION 2
#define DISK_TYPE_CONTAINER 3

#define CONFIG_SECTION_DISKSTATS "plugin:proc:/proc/diskstats"
#define DELAULT_EXLUDED_DISKS "loop* ram*"

static struct disk {
    char *disk;             // the name of the disk (sda, sdb, etc, after being looked up)
    char *device;           // the device of the disk (before being looked up)
    unsigned long major;
    unsigned long minor;
    int sector_size;
    int type;

    char *mount_point;

    // disk options caching
    int configured;
    int do_io;
    int do_ops;
    int do_mops;
    int do_iotime;
    int do_qops;
    int do_util;
    int do_backlog;

    int updated;

    RRDSET *st_avgsz;
    RRDSET *st_await;
    RRDSET *st_backlog;
    RRDSET *st_io;
    RRDSET *st_iotime;
    RRDSET *st_mops;
    RRDSET *st_ops;
    RRDSET *st_qops;
    RRDSET *st_svctm;
    RRDSET *st_util;

    struct disk *next;
} *disk_root = NULL;

#define rrdset_obsolete_and_pointer_null(st) do { if(st) { rrdset_is_obsolete(st); st = NULL; } } while(st)

static char *path_to_get_hw_sector_size = NULL;
static char *path_to_get_hw_sector_size_partitions = NULL;
static char *path_to_find_block_device = NULL;
static char *path_to_device_mapper = NULL;

static inline char *get_disk_name(unsigned long major, unsigned long minor, char *disk) {
    static int enabled = 1;

    if(!enabled) goto cleanup;

    char filename[FILENAME_MAX + 1];
    char link[FILENAME_MAX + 1];

    DIR *dir = opendir(path_to_device_mapper);
    if (!dir) {
        error("DEVICE-MAPPER ('%s', %lu:%lu): Cannot open directory '%s'. Disabling device-mapper support.", disk, major, minor, path_to_device_mapper);
        enabled = 0;
        goto cleanup;
    }

    struct dirent *de = NULL;
    while ((de = readdir(dir))) {
        if(de->d_type != DT_LNK) continue;

        snprintfz(filename, FILENAME_MAX, "%s/%s", path_to_device_mapper, de->d_name);
        ssize_t len = readlink(filename, link, FILENAME_MAX);
        if(len <= 0) {
            error("DEVICE-MAPPER ('%s', %lu:%lu): Cannot read link '%s'.", disk, major, minor, filename);
            continue;
        }

        link[len] = '\0';
        if(link[0] != '/')
            snprintfz(filename, FILENAME_MAX, "%s/%s", path_to_device_mapper, link);
        else
            strncpyz(filename, link, FILENAME_MAX);

        struct stat sb;
        if(stat(filename, &sb) == -1) {
            error("DEVICE-MAPPER ('%s', %lu:%lu): Cannot stat() file '%s'.", disk, major, minor, filename);
            continue;
        }

        if((sb.st_mode & S_IFMT) != S_IFBLK) {
            // info("DEVICE-MAPPER ('%s', %lu:%lu): file '%s' is not a block device.", disk, major, minor, filename);
            continue;
        }

        if(major(sb.st_rdev) != major || minor(sb.st_rdev) != minor) {
            // info("DEVICE-MAPPER ('%s', %lu:%lu): filename '%s' does not match %lu:%lu.", disk, major, minor, filename, (unsigned long)major(sb.st_rdev), (unsigned long)minor(sb.st_rdev));
            continue;
        }

        // info("DEVICE-MAPPER ('%s', %lu:%lu): filename '%s' matches.", disk, major, minor, filename);

        strncpy(link, de->d_name, FILENAME_MAX);
        netdata_fix_chart_name(link);
        disk = link;
        break;
    }
    closedir(dir);

cleanup:
    return strdupz(disk);
}

static struct disk *get_disk(unsigned long major, unsigned long minor, char *disk) {
    static struct mountinfo *disk_mountinfo_root = NULL;

    struct disk *d;

    // search for it in our RAM list.
    // this is sequential, but since we just walk through
    // and the number of disks / partitions in a system
    // should not be that many, it should be acceptable
    for(d = disk_root; d ; d = d->next)
        if(unlikely(d->major == major && d->minor == minor))
            return d;

    // not found
    // create a new disk structure
    d = (struct disk *)callocz(1, sizeof(struct disk));

    d->disk = get_disk_name(major, minor, disk);
    d->device = strdupz(disk);
    d->major = major;
    d->minor = minor;
    d->type = DISK_TYPE_PHYSICAL; // Default type. Changed later if not correct.
    d->configured = 0;
    d->sector_size = 512; // the default, will be changed below
    d->next = NULL;

    // append it to the list
    if(unlikely(!disk_root))
        disk_root = d;
    else {
        struct disk *last;
        for(last = disk_root; last->next ;last = last->next);
        last->next = d;
    }

    // find if it is a partition
    // by checking if /sys/dev/block/MAJOR:MINOR/partition is readable.
    char buffer[FILENAME_MAX + 1];
    snprintfz(buffer, FILENAME_MAX, path_to_find_block_device, major, minor, "partition");
    if(likely(access(buffer, R_OK) == 0)) {
        d->type = DISK_TYPE_PARTITION;
    }
    else {
        // find if it is a container
        // by checking if /sys/dev/block/MAJOR:MINOR/slaves has entries
        snprintfz(buffer, FILENAME_MAX, path_to_find_block_device, major, minor, "slaves/");
        DIR *dirp = opendir(buffer);
        if(likely(dirp != NULL)) {
            struct dirent *dp;
            while( (dp = readdir(dirp)) ) {
                // . and .. are also files in empty folders.
                if(unlikely(strcmp(dp->d_name, ".") == 0 || strcmp(dp->d_name, "..") == 0)) {
                    continue;
                }

                d->type = DISK_TYPE_CONTAINER;

                // Stop the loop after we found one file.
                break;
            }
            if(unlikely(closedir(dirp) == -1))
                error("Unable to close dir %s", buffer);
        }
    }

    // ------------------------------------------------------------------------
    // check if we can find its mount point

    // mountinfo_find() can be called with NULL disk_mountinfo_root
    struct mountinfo *mi = mountinfo_find(disk_mountinfo_root, d->major, d->minor);
    if(unlikely(!mi)) {
        // mountinfo_free can be called with NULL
        mountinfo_free(disk_mountinfo_root);
        disk_mountinfo_root = mountinfo_read(0);
        mi = mountinfo_find(disk_mountinfo_root, d->major, d->minor);
    }

    if(unlikely(mi))
        d->mount_point = strdupz(mi->mount_point);
    else
        d->mount_point = NULL;

    // ------------------------------------------------------------------------
    // find the disk sector size

    {
        char tf[FILENAME_MAX + 1], *t;
        strncpyz(tf, d->device, FILENAME_MAX);

        // replace all / with !
        for(t = tf; *t ;t++)
            if(unlikely(*t == '/')) *t = '!';

        if(likely(d->type == DISK_TYPE_PARTITION))
            snprintfz(buffer, FILENAME_MAX, path_to_get_hw_sector_size_partitions, d->major, d->minor, tf);
        else
            snprintfz(buffer, FILENAME_MAX, path_to_get_hw_sector_size, tf);

        FILE *fpss = fopen(buffer, "r");
        if(likely(fpss)) {
            char buffer2[1024 + 1];
            char *tmp = fgets(buffer2, 1024, fpss);

            if(likely(tmp)) {
                d->sector_size = str2i(tmp);
                if(unlikely(d->sector_size <= 0)) {
                    error("Invalid sector size %d for device %s in %s. Assuming 512.", d->sector_size, d->device, buffer);
                    d->sector_size = 512;
                }
            }
            else error("Cannot read data for sector size for device %s from %s. Assuming 512.", d->device, buffer);

            fclose(fpss);
        }
        else error("Cannot read sector size for device %s from %s. Assuming 512.", d->device, buffer);
    }

    return d;
}

static inline int is_major_enabled(int major) {
    static int8_t *major_configs = NULL;
    static size_t major_size = 0;

    if(major < 0) return 1;

    size_t wanted_size = (size_t)major + 1;

    if(major_size < wanted_size) {
        major_configs = reallocz(major_configs, wanted_size * sizeof(int8_t));

        size_t i;
        for(i = major_size; i < wanted_size ; i++)
            major_configs[i] = -1;

        major_size = wanted_size;
    }

    if(major_configs[major] == -1) {
        char buffer[CONFIG_MAX_NAME + 1];
        snprintfz(buffer, CONFIG_MAX_NAME, "performance metrics for disks with major %d", major);
        major_configs[major] = (char)config_get_boolean(CONFIG_SECTION_DISKSTATS, buffer, 1);
    }

    return (int)major_configs[major];
}

int do_proc_diskstats(int update_every, usec_t dt) {
    static procfile *ff = NULL;
    static int  global_enable_new_disks_detected_at_runtime = CONFIG_BOOLEAN_YES,
                global_enable_performance_for_physical_disks = CONFIG_BOOLEAN_AUTO,
                global_enable_performance_for_virtual_disks = CONFIG_BOOLEAN_AUTO,
                global_enable_performance_for_partitions = CONFIG_BOOLEAN_NO,
                global_do_io = CONFIG_BOOLEAN_AUTO,
                global_do_ops = CONFIG_BOOLEAN_AUTO,
                global_do_mops = CONFIG_BOOLEAN_AUTO,
                global_do_iotime = CONFIG_BOOLEAN_AUTO,
                global_do_qops = CONFIG_BOOLEAN_AUTO,
                global_do_util = CONFIG_BOOLEAN_AUTO,
                global_do_backlog = CONFIG_BOOLEAN_AUTO,
                globals_initialized = 0,
                global_cleanup_removed_disks = 1;

    if(unlikely(!globals_initialized)) {
        globals_initialized = 1;

        global_enable_new_disks_detected_at_runtime = config_get_boolean(CONFIG_SECTION_DISKSTATS, "enable new disks detected at runtime", global_enable_new_disks_detected_at_runtime);
        global_enable_performance_for_physical_disks = config_get_boolean_ondemand(CONFIG_SECTION_DISKSTATS, "performance metrics for physical disks", global_enable_performance_for_physical_disks);
        global_enable_performance_for_virtual_disks = config_get_boolean_ondemand(CONFIG_SECTION_DISKSTATS, "performance metrics for virtual disks", global_enable_performance_for_virtual_disks);
        global_enable_performance_for_partitions = config_get_boolean_ondemand(CONFIG_SECTION_DISKSTATS, "performance metrics for partitions", global_enable_performance_for_partitions);

        global_do_io      = config_get_boolean_ondemand(CONFIG_SECTION_DISKSTATS, "bandwidth for all disks", global_do_io);
        global_do_ops     = config_get_boolean_ondemand(CONFIG_SECTION_DISKSTATS, "operations for all disks", global_do_ops);
        global_do_mops    = config_get_boolean_ondemand(CONFIG_SECTION_DISKSTATS, "merged operations for all disks", global_do_mops);
        global_do_iotime  = config_get_boolean_ondemand(CONFIG_SECTION_DISKSTATS, "i/o time for all disks", global_do_iotime);
        global_do_qops    = config_get_boolean_ondemand(CONFIG_SECTION_DISKSTATS, "queued operations for all disks", global_do_qops);
        global_do_util    = config_get_boolean_ondemand(CONFIG_SECTION_DISKSTATS, "utilization percentage for all disks", global_do_util);
        global_do_backlog = config_get_boolean_ondemand(CONFIG_SECTION_DISKSTATS, "backlog for all disks", global_do_backlog);

        global_cleanup_removed_disks = config_get_boolean(CONFIG_SECTION_DISKSTATS, "remove charts of removed disks" , global_cleanup_removed_disks);
        
        char buffer[FILENAME_MAX + 1];

        snprintfz(buffer, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/sys/dev/block/%lu:%lu/%s");
        path_to_find_block_device = config_get(CONFIG_SECTION_DISKSTATS, "path to get block device infos", buffer);

        snprintfz(buffer, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/sys/block/%s/queue/hw_sector_size");
        path_to_get_hw_sector_size = config_get(CONFIG_SECTION_DISKSTATS, "path to get h/w sector size", buffer);

        snprintfz(buffer, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/sys/dev/block/%lu:%lu/subsystem/%s/../queue/hw_sector_size");
        path_to_get_hw_sector_size_partitions = config_get(CONFIG_SECTION_DISKSTATS, "path to get h/w sector size for partitions", buffer);

        snprintfz(buffer, FILENAME_MAX, "%s/dev/mapper", netdata_configured_host_prefix);
        path_to_device_mapper = config_get(CONFIG_SECTION_DISKSTATS, "path to device mapper", buffer);
    }

    // --------------------------------------------------------------------------

    if(unlikely(!ff)) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/proc/diskstats");
        ff = procfile_open(config_get(CONFIG_SECTION_DISKSTATS, "filename to monitor", filename), " \t", PROCFILE_FLAG_DEFAULT);
    }
    if(unlikely(!ff)) return 0;

    ff = procfile_readall(ff);
    if(unlikely(!ff)) return 0; // we return 0, so that we will retry to open it next time

    size_t lines = procfile_lines(ff), l;

    for(l = 0; l < lines ;l++) {
        // --------------------------------------------------------------------------
        // Read parameters

        char *disk;
        unsigned long       major = 0, minor = 0;

        collected_number    reads = 0,  mreads = 0,  readsectors = 0,  readms = 0,
                            writes = 0, mwrites = 0, writesectors = 0, writems = 0,
                            queued_ios = 0, busy_ms = 0, backlog_ms = 0;

        collected_number    last_reads = 0,  last_readsectors = 0,  last_readms = 0,
                            last_writes = 0, last_writesectors = 0, last_writems = 0,
                            last_busy_ms = 0;

        size_t words = procfile_linewords(ff, l);
        if(unlikely(words < 14)) continue;

        major           = str2ul(procfile_lineword(ff, l, 0));
        minor           = str2ul(procfile_lineword(ff, l, 1));
        disk            = procfile_lineword(ff, l, 2);

        // # of reads completed # of writes completed
        // This is the total number of reads or writes completed successfully.
        reads           = str2ull(procfile_lineword(ff, l, 3));  // rd_ios
        writes          = str2ull(procfile_lineword(ff, l, 7));  // wr_ios

        // # of reads merged # of writes merged
        // Reads and writes which are adjacent to each other may be merged for
        // efficiency.  Thus two 4K reads may become one 8K read before it is
        // ultimately handed to the disk, and so it will be counted (and queued)
        mreads          = str2ull(procfile_lineword(ff, l, 4));  // rd_merges_or_rd_sec
        mwrites         = str2ull(procfile_lineword(ff, l, 8));  // wr_merges

        // # of sectors read # of sectors written
        // This is the total number of sectors read or written successfully.
        readsectors     = str2ull(procfile_lineword(ff, l, 5));  // rd_sec_or_wr_ios
        writesectors    = str2ull(procfile_lineword(ff, l, 9));  // wr_sec

        // # of milliseconds spent reading # of milliseconds spent writing
        // This is the total number of milliseconds spent by all reads or writes (as
        // measured from __make_request() to end_that_request_last()).
        readms          = str2ull(procfile_lineword(ff, l, 6));  // rd_ticks_or_wr_sec
        writems         = str2ull(procfile_lineword(ff, l, 10)); // wr_ticks

        // # of I/Os currently in progress
        // The only field that should go to zero. Incremented as requests are
        // given to appropriate struct request_queue and decremented as they finish.
        queued_ios      = str2ull(procfile_lineword(ff, l, 11)); // ios_pgr

        // # of milliseconds spent doing I/Os
        // This field increases so long as field queued_ios is nonzero.
        busy_ms         = str2ull(procfile_lineword(ff, l, 12)); // tot_ticks

        // weighted # of milliseconds spent doing I/Os
        // This field is incremented at each I/O start, I/O completion, I/O
        // merge, or read of these stats by the number of I/Os in progress
        // (field queued_ios) times the number of milliseconds spent doing I/O since the
        // last update of this field.  This can provide an easy measure of both
        // I/O completion time and the backlog that may be accumulating.
        backlog_ms      = str2ull(procfile_lineword(ff, l, 13)); // rq_ticks


        // --------------------------------------------------------------------------
        // remove slashes from disk names
        char *s;
        for(s = disk; *s ;s++)
            if(*s == '/') *s = '_';

        // --------------------------------------------------------------------------
        // get a disk structure for the disk

        struct disk *d = get_disk(major, minor, disk);
        d->updated = 1;

        // --------------------------------------------------------------------------
        // Set its family based on mount point

        char *family = d->mount_point;
        if(!family) family = d->disk;


        // --------------------------------------------------------------------------
        // Check the configuration for the device

        if(unlikely(!d->configured)) {
            d->configured = 1;

            static SIMPLE_PATTERN *excluded_disks = NULL;

            if(unlikely(!excluded_disks)) {
                excluded_disks = simple_pattern_create(
                        config_get(CONFIG_SECTION_DISKSTATS, "exclude disks", DELAULT_EXLUDED_DISKS),
                        SIMPLE_PATTERN_EXACT
                );
            }

            int def_enable = global_enable_new_disks_detected_at_runtime;

            if(def_enable != CONFIG_BOOLEAN_NO && (simple_pattern_matches(excluded_disks, d->device) || simple_pattern_matches(excluded_disks, d->disk)))
                def_enable = CONFIG_BOOLEAN_NO;

            char var_name[4096 + 1];
            snprintfz(var_name, 4096, "plugin:proc:/proc/diskstats:%s", d->disk);

            def_enable = config_get_boolean_ondemand(var_name, "enable", def_enable);
            if(unlikely(def_enable == CONFIG_BOOLEAN_NO)) {
                // the user does not want any metrics for this disk
                d->do_io = CONFIG_BOOLEAN_NO;
                d->do_ops = CONFIG_BOOLEAN_NO;
                d->do_mops = CONFIG_BOOLEAN_NO;
                d->do_iotime = CONFIG_BOOLEAN_NO;
                d->do_qops = CONFIG_BOOLEAN_NO;
                d->do_util = CONFIG_BOOLEAN_NO;
                d->do_backlog = CONFIG_BOOLEAN_NO;
            }
            else {
                // this disk is enabled
                // check its direct settings

                int def_performance = CONFIG_BOOLEAN_AUTO;

                // since this is 'on demand' we can figure the performance settings
                // based on the type of disk

                switch(d->type) {
                    case DISK_TYPE_PHYSICAL:
                        def_performance = global_enable_performance_for_physical_disks;
                        break;

                    case DISK_TYPE_PARTITION:
                        def_performance = global_enable_performance_for_partitions;
                        break;

                    case DISK_TYPE_CONTAINER:
                        def_performance = global_enable_performance_for_virtual_disks;
                        break;
                }

                // check if we have to disable performance for this disk
                if(def_performance)
                    def_performance = is_major_enabled((int)major);

                // ------------------------------------------------------------
                // now we have def_performance and def_space
                // to work further

                // def_performance
                // check the user configuration (this will also show our 'on demand' decision)
                def_performance = config_get_boolean_ondemand(var_name, "enable performance metrics", def_performance);

                int ddo_io = CONFIG_BOOLEAN_NO,
                    ddo_ops = CONFIG_BOOLEAN_NO,
                    ddo_mops = CONFIG_BOOLEAN_NO,
                    ddo_iotime = CONFIG_BOOLEAN_NO,
                    ddo_qops = CONFIG_BOOLEAN_NO,
                    ddo_util = CONFIG_BOOLEAN_NO,
                    ddo_backlog = CONFIG_BOOLEAN_NO;

                // we enable individual performance charts only when def_performance is not disabled
                if(unlikely(def_performance != CONFIG_BOOLEAN_NO)) {
                    ddo_io = global_do_io,
                    ddo_ops = global_do_ops,
                    ddo_mops = global_do_mops,
                    ddo_iotime = global_do_iotime,
                    ddo_qops = global_do_qops,
                    ddo_util = global_do_util,
                    ddo_backlog = global_do_backlog;
                }

                d->do_io      = config_get_boolean_ondemand(var_name, "bandwidth", ddo_io);
                d->do_ops     = config_get_boolean_ondemand(var_name, "operations", ddo_ops);
                d->do_mops    = config_get_boolean_ondemand(var_name, "merged operations", ddo_mops);
                d->do_iotime  = config_get_boolean_ondemand(var_name, "i/o time", ddo_iotime);
                d->do_qops    = config_get_boolean_ondemand(var_name, "queued operations", ddo_qops);
                d->do_util    = config_get_boolean_ondemand(var_name, "utilization percentage", ddo_util);
                d->do_backlog = config_get_boolean_ondemand(var_name, "backlog", ddo_backlog);
            }
        }

        // --------------------------------------------------------------------------
        // Do performance metrics

        if(d->do_io == CONFIG_BOOLEAN_YES || (d->do_io == CONFIG_BOOLEAN_AUTO && (readsectors || writesectors))) {
            d->do_io = CONFIG_BOOLEAN_YES;

            if(unlikely(!d->st_io)) {
                d->st_io = rrdset_create_localhost(
                        RRD_TYPE_DISK
                        , d->device
                        , d->disk
                        , family
                        , "disk.io"
                        , "Disk I/O Bandwidth"
                        , "kilobytes/s"
                        , 2000
                        , update_every
                        , RRDSET_TYPE_AREA
                );

                rrddim_add(d->st_io, "reads", NULL, d->sector_size, 1024, RRD_ALGORITHM_INCREMENTAL);
                rrddim_add(d->st_io, "writes", NULL, d->sector_size * -1, 1024, RRD_ALGORITHM_INCREMENTAL);
            }
            else rrdset_next(d->st_io);

            last_readsectors  = rrddim_set(d->st_io, "reads", readsectors);
            last_writesectors = rrddim_set(d->st_io, "writes", writesectors);
            rrdset_done(d->st_io);
        }

        // --------------------------------------------------------------------

        if(d->do_ops == CONFIG_BOOLEAN_YES || (d->do_ops == CONFIG_BOOLEAN_AUTO && (reads || writes))) {
            d->do_ops = CONFIG_BOOLEAN_YES;

            if(unlikely(!d->st_ops)) {
                d->st_ops = rrdset_create_localhost(
                        "disk_ops"
                        , d->device
                        , d->disk
                        , family
                        , "disk.ops"
                        , "Disk Completed I/O Operations"
                        , "operations/s"
                        , 2001
                        , update_every
                        , RRDSET_TYPE_LINE
                );

                rrdset_flag_set(d->st_ops, RRDSET_FLAG_DETAIL);

                rrddim_add(d->st_ops, "reads", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                rrddim_add(d->st_ops, "writes", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            }
            else rrdset_next(d->st_ops);

            last_reads  = rrddim_set(d->st_ops, "reads", reads);
            last_writes = rrddim_set(d->st_ops, "writes", writes);
            rrdset_done(d->st_ops);
        }

        // --------------------------------------------------------------------

        if(d->do_qops == CONFIG_BOOLEAN_YES || (d->do_qops == CONFIG_BOOLEAN_AUTO && queued_ios)) {
            d->do_qops = CONFIG_BOOLEAN_YES;

            if(unlikely(!d->st_qops)) {
                d->st_qops = rrdset_create_localhost(
                        "disk_qops"
                        , d->device
                        , d->disk
                        , family
                        , "disk.qops"
                        , "Disk Current I/O Operations"
                        , "operations"
                        , 2002
                        , update_every
                        , RRDSET_TYPE_LINE
                );

                rrdset_flag_set(d->st_qops, RRDSET_FLAG_DETAIL);

                rrddim_add(d->st_qops, "operations", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            }
            else rrdset_next(d->st_qops);

            rrddim_set(d->st_qops, "operations", queued_ios);
            rrdset_done(d->st_qops);
        }

        // --------------------------------------------------------------------

        if(d->do_backlog == CONFIG_BOOLEAN_YES || (d->do_backlog == CONFIG_BOOLEAN_AUTO && backlog_ms)) {
            d->do_backlog = CONFIG_BOOLEAN_YES;

            if(unlikely(!d->st_backlog)) {
                d->st_backlog = rrdset_create_localhost(
                        "disk_backlog"
                        , d->device
                        , d->disk
                        , family
                        , "disk.backlog"
                        , "Disk Backlog"
                        , "backlog (ms)"
                        , 2003
                        , update_every
                        , RRDSET_TYPE_AREA
                );

                rrdset_flag_set(d->st_backlog, RRDSET_FLAG_DETAIL);

                rrddim_add(d->st_backlog, "backlog", NULL, 1, 10, RRD_ALGORITHM_INCREMENTAL);
            }
            else rrdset_next(d->st_backlog);

            rrddim_set(d->st_backlog, "backlog", backlog_ms);
            rrdset_done(d->st_backlog);
        }

        // --------------------------------------------------------------------

        if(d->do_util == CONFIG_BOOLEAN_YES || (d->do_util == CONFIG_BOOLEAN_AUTO && busy_ms)) {
            d->do_util = CONFIG_BOOLEAN_YES;

            if(unlikely(!d->st_util)) {
                d->st_util = rrdset_create_localhost(
                        "disk_util"
                        , d->device
                        , d->disk
                        , family
                        , "disk.util"
                        , "Disk Utilization Time"
                        , "% of time working"
                        , 2004
                        , update_every
                        , RRDSET_TYPE_AREA
                );

                rrdset_flag_set(d->st_util, RRDSET_FLAG_DETAIL);

                rrddim_add(d->st_util, "utilization", NULL, 1, 10, RRD_ALGORITHM_INCREMENTAL);
            }
            else rrdset_next(d->st_util);

            last_busy_ms = rrddim_set(d->st_util, "utilization", busy_ms);
            rrdset_done(d->st_util);
        }

        // --------------------------------------------------------------------

        if(d->do_mops == CONFIG_BOOLEAN_YES || (d->do_mops == CONFIG_BOOLEAN_AUTO && (mreads || mwrites))) {
            d->do_mops = CONFIG_BOOLEAN_YES;

            if(unlikely(!d->st_mops)) {
                d->st_mops = rrdset_create_localhost(
                        "disk_mops"
                        , d->device
                        , d->disk
                        , family
                        , "disk.mops"
                        , "Disk Merged Operations"
                        , "merged operations/s"
                        , 2021
                        , update_every
                        , RRDSET_TYPE_LINE
                );

                rrdset_flag_set(d->st_mops, RRDSET_FLAG_DETAIL);

                rrddim_add(d->st_mops, "reads", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                rrddim_add(d->st_mops, "writes", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            }
            else rrdset_next(d->st_mops);

            rrddim_set(d->st_mops, "reads", mreads);
            rrddim_set(d->st_mops, "writes", mwrites);
            rrdset_done(d->st_mops);
        }

        // --------------------------------------------------------------------

        if(d->do_iotime == CONFIG_BOOLEAN_YES || (d->do_iotime == CONFIG_BOOLEAN_AUTO && (readms || writems))) {
            d->do_iotime = CONFIG_BOOLEAN_YES;

            if(unlikely(!d->st_iotime)) {
                d->st_iotime = rrdset_create_localhost(
                        "disk_iotime"
                        , d->device
                        , d->disk
                        , family
                        , "disk.iotime"
                        , "Disk Total I/O Time"
                        , "milliseconds/s"
                        , 2022
                        , update_every
                        , RRDSET_TYPE_LINE
                );

                rrdset_flag_set(d->st_iotime, RRDSET_FLAG_DETAIL);

                rrddim_add(d->st_iotime, "reads", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                rrddim_add(d->st_iotime, "writes", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            }
            else rrdset_next(d->st_iotime);

            last_readms  = rrddim_set(d->st_iotime, "reads", readms);
            last_writems = rrddim_set(d->st_iotime, "writes", writems);
            rrdset_done(d->st_iotime);
        }

        // --------------------------------------------------------------------
        // calculate differential charts
        // only if this is not the first time we run

        if(likely(dt)) {
            if( (d->do_iotime == CONFIG_BOOLEAN_YES || (d->do_iotime == CONFIG_BOOLEAN_AUTO && (readms || writems))) &&
                (d->do_ops    == CONFIG_BOOLEAN_YES || (d->do_ops    == CONFIG_BOOLEAN_AUTO && (reads || writes)))) {

                if(unlikely(!d->st_await)) {
                    d->st_await = rrdset_create_localhost(
                            "disk_await"
                            , d->device
                            , d->disk
                            , family
                            , "disk.await"
                            , "Average Completed I/O Operation Time"
                            , "ms per operation"
                            , 2005
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    rrdset_flag_set(d->st_await, RRDSET_FLAG_DETAIL);

                    rrddim_add(d->st_await, "reads", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                    rrddim_add(d->st_await, "writes", NULL, -1, 1, RRD_ALGORITHM_ABSOLUTE);
                }
                else rrdset_next(d->st_await);

                rrddim_set(d->st_await, "reads", (reads - last_reads) ? (readms - last_readms) / (reads - last_reads) : 0);
                rrddim_set(d->st_await, "writes", (writes - last_writes) ? (writems - last_writems) / (writes - last_writes) : 0);
                rrdset_done(d->st_await);
            }

            if( (d->do_io  == CONFIG_BOOLEAN_YES || (d->do_io  == CONFIG_BOOLEAN_AUTO && (readsectors || writesectors))) &&
                (d->do_ops == CONFIG_BOOLEAN_YES || (d->do_ops == CONFIG_BOOLEAN_AUTO && (reads || writes)))) {

                if(unlikely(!d->st_avgsz)) {
                    d->st_avgsz = rrdset_create_localhost(
                            "disk_avgsz"
                            , d->device
                            , d->disk
                            , family
                            , "disk.avgsz"
                            , "Average Completed I/O Operation Bandwidth"
                            , "kilobytes per operation"
                            , 2006
                            , update_every
                            , RRDSET_TYPE_AREA
                    );

                    rrdset_flag_set(d->st_avgsz, RRDSET_FLAG_DETAIL);

                    rrddim_add(d->st_avgsz, "reads", NULL, d->sector_size, 1024, RRD_ALGORITHM_ABSOLUTE);
                    rrddim_add(d->st_avgsz, "writes", NULL, d->sector_size * -1, 1024, RRD_ALGORITHM_ABSOLUTE);
                }
                else rrdset_next(d->st_avgsz);

                rrddim_set(d->st_avgsz, "reads", (reads - last_reads) ? (readsectors - last_readsectors) / (reads - last_reads) : 0);
                rrddim_set(d->st_avgsz, "writes", (writes - last_writes) ? (writesectors - last_writesectors) / (writes - last_writes) : 0);
                rrdset_done(d->st_avgsz);
            }

            if( (d->do_util == CONFIG_BOOLEAN_YES || (d->do_util == CONFIG_BOOLEAN_AUTO && busy_ms)) &&
                (d->do_ops  == CONFIG_BOOLEAN_YES || (d->do_ops  == CONFIG_BOOLEAN_AUTO && (reads || writes)))) {

                if(unlikely(!d->st_svctm)) {
                    d->st_svctm = rrdset_create_localhost(
                            "disk_svctm"
                            , d->device
                            , d->disk
                            , family
                            , "disk.svctm"
                            , "Average Service Time"
                            , "ms per operation"
                            , 2007
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    rrdset_flag_set(d->st_svctm, RRDSET_FLAG_DETAIL);

                    rrddim_add(d->st_svctm, "svctm", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                }
                else rrdset_next(d->st_svctm);

                rrddim_set(d->st_svctm, "svctm", ((reads - last_reads) + (writes - last_writes)) ? (busy_ms - last_busy_ms) / ((reads - last_reads) + (writes - last_writes)) : 0);
                rrdset_done(d->st_svctm);
            }
        }
    }

    // cleanup removed disks

    struct disk *d = disk_root, *last = NULL;
    while(d) {
        if(unlikely(global_cleanup_removed_disks && !d->updated)) {
            struct disk *t = d;

            rrdset_obsolete_and_pointer_null(d->st_avgsz);
            rrdset_obsolete_and_pointer_null(d->st_await);
            rrdset_obsolete_and_pointer_null(d->st_backlog);
            rrdset_obsolete_and_pointer_null(d->st_io);
            rrdset_obsolete_and_pointer_null(d->st_iotime);
            rrdset_obsolete_and_pointer_null(d->st_mops);
            rrdset_obsolete_and_pointer_null(d->st_ops);
            rrdset_obsolete_and_pointer_null(d->st_qops);
            rrdset_obsolete_and_pointer_null(d->st_svctm);
            rrdset_obsolete_and_pointer_null(d->st_util);

            if(d == disk_root) {
                disk_root = d = d->next;
                last = NULL;
            }
            else if(last) {
                last->next = d = d->next;
            }

            freez(t->disk);
            freez(t->device);
            freez(t->mount_point);
            freez(t);
        }
        else {
            d->updated = 0;
            last = d;
            d = d->next;
        }
    }

    return 0;
}
