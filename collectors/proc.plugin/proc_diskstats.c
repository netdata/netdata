// SPDX-License-Identifier: GPL-3.0-or-later

#include "plugin_proc.h"

#define RRD_TYPE_DISK "disk"
#define PLUGIN_PROC_MODULE_DISKSTATS_NAME "/proc/diskstats"
#define CONFIG_SECTION_PLUGIN_PROC_DISKSTATS "plugin:" PLUGIN_PROC_CONFIG_NAME ":" PLUGIN_PROC_MODULE_DISKSTATS_NAME

#define DISK_TYPE_UNKNOWN   0
#define DISK_TYPE_PHYSICAL  1
#define DISK_TYPE_PARTITION 2
#define DISK_TYPE_VIRTUAL   3

#define DEFAULT_PREFERRED_IDS "*"
#define DEFAULT_EXCLUDED_DISKS "loop* ram*"

static struct disk {
    char *disk;             // the name of the disk (sda, sdb, etc, after being looked up)
    char *device;           // the device of the disk (before being looked up)
    uint32_t hash;
    unsigned long major;
    unsigned long minor;
    int sector_size;
    int type;

    char *mount_point;

    // disk options caching
    int do_io;
    int do_ops;
    int do_mops;
    int do_iotime;
    int do_qops;
    int do_util;
    int do_ext;
    int do_backlog;
    int do_bcache;

    int updated;

    int device_is_bcache;

    char *bcache_filename_dirty_data;
    char *bcache_filename_writeback_rate;
    char *bcache_filename_cache_congested;
    char *bcache_filename_cache_available_percent;
    char *bcache_filename_stats_five_minute_cache_hit_ratio;
    char *bcache_filename_stats_hour_cache_hit_ratio;
    char *bcache_filename_stats_day_cache_hit_ratio;
    char *bcache_filename_stats_total_cache_hit_ratio;
    char *bcache_filename_stats_total_cache_hits;
    char *bcache_filename_stats_total_cache_misses;
    char *bcache_filename_stats_total_cache_miss_collisions;
    char *bcache_filename_stats_total_cache_bypass_hits;
    char *bcache_filename_stats_total_cache_bypass_misses;
    char *bcache_filename_stats_total_cache_readaheads;
    char *bcache_filename_cache_read_races;
    char *bcache_filename_cache_io_errors;
    char *bcache_filename_priority_stats;

    usec_t bcache_priority_stats_update_every_usec;
    usec_t bcache_priority_stats_elapsed_usec;

    RRDSET *st_io;
    RRDDIM *rd_io_reads;
    RRDDIM *rd_io_writes;

    RRDSET *st_ext_io;
    RRDDIM *rd_io_discards;

    RRDSET *st_ops;
    RRDDIM *rd_ops_reads;
    RRDDIM *rd_ops_writes;

    RRDSET *st_ext_ops;
    RRDDIM *rd_ops_discards;
    RRDDIM *rd_ops_flushes;

    RRDSET *st_qops;
    RRDDIM *rd_qops_operations;

    RRDSET *st_backlog;
    RRDDIM *rd_backlog_backlog;

    RRDSET *st_busy;
    RRDDIM *rd_busy_busy;

    RRDSET *st_util;
    RRDDIM *rd_util_utilization;

    RRDSET *st_mops;
    RRDDIM *rd_mops_reads;
    RRDDIM *rd_mops_writes;

    RRDSET *st_ext_mops;
    RRDDIM *rd_mops_discards;

    RRDSET *st_iotime;
    RRDDIM *rd_iotime_reads;
    RRDDIM *rd_iotime_writes;

    RRDSET *st_ext_iotime;
    RRDDIM *rd_iotime_discards;
    RRDDIM *rd_iotime_flushes;

    RRDSET *st_await;
    RRDDIM *rd_await_reads;
    RRDDIM *rd_await_writes;

    RRDSET *st_ext_await;
    RRDDIM *rd_await_discards;
    RRDDIM *rd_await_flushes;

    RRDSET *st_avgsz;
    RRDDIM *rd_avgsz_reads;
    RRDDIM *rd_avgsz_writes;

    RRDSET *st_ext_avgsz;
    RRDDIM *rd_avgsz_discards;

    RRDSET *st_svctm;
    RRDDIM *rd_svctm_svctm;

    RRDSET *st_bcache_size;
    RRDDIM *rd_bcache_dirty_size;

    RRDSET *st_bcache_usage;
    RRDDIM *rd_bcache_available_percent;

    RRDSET *st_bcache_hit_ratio;
    RRDDIM *rd_bcache_hit_ratio_5min;
    RRDDIM *rd_bcache_hit_ratio_1hour;
    RRDDIM *rd_bcache_hit_ratio_1day;
    RRDDIM *rd_bcache_hit_ratio_total;

    RRDSET *st_bcache;
    RRDDIM *rd_bcache_hits;
    RRDDIM *rd_bcache_misses;
    RRDDIM *rd_bcache_miss_collisions;

    RRDSET *st_bcache_bypass;
    RRDDIM *rd_bcache_bypass_hits;
    RRDDIM *rd_bcache_bypass_misses;

    RRDSET *st_bcache_rates;
    RRDDIM *rd_bcache_rate_congested;
    RRDDIM *rd_bcache_readaheads;
    RRDDIM *rd_bcache_rate_writeback;

    RRDSET *st_bcache_cache_allocations;
    RRDDIM *rd_bcache_cache_allocations_unused;
    RRDDIM *rd_bcache_cache_allocations_clean;
    RRDDIM *rd_bcache_cache_allocations_dirty;
    RRDDIM *rd_bcache_cache_allocations_metadata;
    RRDDIM *rd_bcache_cache_allocations_unknown;

    RRDSET *st_bcache_cache_read_races;
    RRDDIM *rd_bcache_cache_read_races;
    RRDDIM *rd_bcache_cache_io_errors;

    struct disk *next;
} *disk_root = NULL;

#define rrdset_obsolete_and_pointer_null(st) do { if(st) { rrdset_is_obsolete(st); (st) = NULL; } } while(st)

// static char *path_to_get_hw_sector_size = NULL;
// static char *path_to_get_hw_sector_size_partitions = NULL;
static char *path_to_sys_dev_block_major_minor_string = NULL;
static char *path_to_sys_block_device = NULL;
static char *path_to_sys_block_device_bcache = NULL;
static char *path_to_sys_devices_virtual_block_device = NULL;
static char *path_to_device_mapper = NULL;
static char *path_to_device_label = NULL;
static char *path_to_device_id = NULL;
static char *path_to_veritas_volume_groups = NULL;
static int name_disks_by_id = CONFIG_BOOLEAN_NO;
static int global_bcache_priority_stats_update_every = 0; // disabled by default

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
        global_do_ext = CONFIG_BOOLEAN_AUTO,
        global_do_backlog = CONFIG_BOOLEAN_AUTO,
        global_do_bcache = CONFIG_BOOLEAN_AUTO,
        globals_initialized = 0,
        global_cleanup_removed_disks = 1;

static SIMPLE_PATTERN *preferred_ids = NULL;
static SIMPLE_PATTERN *excluded_disks = NULL;

static unsigned long long int bcache_read_number_with_units(const char *filename) {
    char buffer[50 + 1];
    if(read_file(filename, buffer, 50) == 0) {
        static int unknown_units_error = 10;

        char *end = NULL;
        long double value = str2ld(buffer, &end);
        if(end && *end) {
            if(*end == 'k')
                return (unsigned long long int)(value * 1024.0);
            else if(*end == 'M')
                return (unsigned long long int)(value * 1024.0 * 1024.0);
            else if(*end == 'G')
                return (unsigned long long int)(value * 1024.0 * 1024.0 * 1024.0);
            else if(*end == 'T')
                return (unsigned long long int)(value * 1024.0 * 1024.0 * 1024.0 * 1024.0);
            else if(unknown_units_error > 0) {
                error("bcache file '%s' provides value '%s' with unknown units '%s'", filename, buffer, end);
                unknown_units_error--;
            }
        }

        return (unsigned long long int)value;
    }

    return 0;
}

void bcache_read_priority_stats(struct disk *d, const char *family, int update_every, usec_t dt) {
    static procfile *ff = NULL;
    static char *separators = " \t:%[]";

    static ARL_BASE *arl_base = NULL;

    static unsigned long long unused;
    static unsigned long long clean;
    static unsigned long long dirty;
    static unsigned long long metadata;
    static unsigned long long unknown;

    // check if it is time to update this metric
    d->bcache_priority_stats_elapsed_usec += dt;
    if(likely(d->bcache_priority_stats_elapsed_usec < d->bcache_priority_stats_update_every_usec)) return;
    d->bcache_priority_stats_elapsed_usec = 0;

    // initialize ARL
    if(unlikely(!arl_base)) {
        arl_base = arl_create("bcache/priority_stats", NULL, 60);
        arl_expect(arl_base, "Unused", &unused);
        arl_expect(arl_base, "Clean", &clean);
        arl_expect(arl_base, "Dirty", &dirty);
        arl_expect(arl_base, "Metadata", &metadata);
    }

    ff = procfile_reopen(ff, d->bcache_filename_priority_stats, separators, PROCFILE_FLAG_DEFAULT);
    if(likely(ff)) ff = procfile_readall(ff);
    if(unlikely(!ff)) {
        separators = " \t:%[]";
        return;
    }

    // do not reset the separators on every iteration
    separators = NULL;

    arl_begin(arl_base);
    unused = clean = dirty = metadata = unknown = 0;

    size_t lines = procfile_lines(ff), l;

    for(l = 0; l < lines ;l++) {
        size_t words = procfile_linewords(ff, l);
        if(unlikely(words < 2)) {
            if(unlikely(words)) error("Cannot read '%s' line %zu. Expected 2 params, read %zu.", d->bcache_filename_priority_stats, l, words);
            continue;
        }

        if(unlikely(arl_check(arl_base,
                procfile_lineword(ff, l, 0),
                procfile_lineword(ff, l, 1)))) break;
    }

    unknown = 100 - unused - clean - dirty - metadata;

    // create / update the cache allocations chart
    {
        if(unlikely(!d->st_bcache_cache_allocations)) {
            d->st_bcache_cache_allocations = rrdset_create_localhost(
                    "disk_bcache_cache_alloc"
                    , d->device
                    , d->disk
                    , family
                    , "disk.bcache_cache_alloc"
                    , "BCache Cache Allocations"
                    , "percentage"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_DISKSTATS_NAME
                    , NETDATA_CHART_PRIO_BCACHE_CACHE_ALLOC
                    , update_every
                    , RRDSET_TYPE_STACKED
            );

            d->rd_bcache_cache_allocations_unused    = rrddim_add(d->st_bcache_cache_allocations, "unused",     NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            d->rd_bcache_cache_allocations_dirty     = rrddim_add(d->st_bcache_cache_allocations, "dirty",      NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            d->rd_bcache_cache_allocations_clean     = rrddim_add(d->st_bcache_cache_allocations, "clean",      NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            d->rd_bcache_cache_allocations_metadata  = rrddim_add(d->st_bcache_cache_allocations, "metadata",   NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            d->rd_bcache_cache_allocations_unknown   = rrddim_add(d->st_bcache_cache_allocations, "undefined",  NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

            d->bcache_priority_stats_update_every_usec = update_every * USEC_PER_SEC;
        }
        else rrdset_next(d->st_bcache_cache_allocations);

        rrddim_set_by_pointer(d->st_bcache_cache_allocations, d->rd_bcache_cache_allocations_unused, unused);
        rrddim_set_by_pointer(d->st_bcache_cache_allocations, d->rd_bcache_cache_allocations_dirty, dirty);
        rrddim_set_by_pointer(d->st_bcache_cache_allocations, d->rd_bcache_cache_allocations_clean, clean);
        rrddim_set_by_pointer(d->st_bcache_cache_allocations, d->rd_bcache_cache_allocations_metadata, metadata);
        rrddim_set_by_pointer(d->st_bcache_cache_allocations, d->rd_bcache_cache_allocations_unknown, unknown);
        rrdset_done(d->st_bcache_cache_allocations);
    }
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
        major_configs[major] = (char)config_get_boolean(CONFIG_SECTION_PLUGIN_PROC_DISKSTATS, buffer, 1);
    }

    return (int)major_configs[major];
}

static inline int get_disk_name_from_path(const char *path, char *result, size_t result_size, unsigned long major, unsigned long minor, char *disk, char *prefix, int depth) {
    //info("DEVICE-MAPPER ('%s', %lu:%lu): examining directory '%s' (allowed depth %d).", disk, major, minor, path, depth);

    int found = 0, preferred = 0;

    char *first_result = mallocz(result_size);

    DIR *dir = opendir(path);
    if (!dir) {
        error("DEVICE-MAPPER ('%s', %lu:%lu): Cannot open directory '%s'.", disk, major, minor, path);
        goto failed;
    }

    struct dirent *de = NULL;
    while ((de = readdir(dir))) {
        if(de->d_type == DT_DIR) {
            if((de->d_name[0] == '.' && de->d_name[1] == '\0') || (de->d_name[0] == '.' && de->d_name[1] == '.' && de->d_name[2] == '\0'))
                continue;

            if(depth <= 0) {
                error("DEVICE-MAPPER ('%s', %lu:%lu): Depth limit reached for path '%s/%s'. Ignoring path.", disk, major, minor, path, de->d_name);
                break;
            }
            else {
                char *path_nested = NULL;
                char *prefix_nested = NULL;

                {
                    char buffer[FILENAME_MAX + 1];
                    snprintfz(buffer, FILENAME_MAX, "%s/%s", path, de->d_name);
                    path_nested = strdupz(buffer);

                    snprintfz(buffer, FILENAME_MAX, "%s%s%s", (prefix)?prefix:"", (prefix)?"_":"", de->d_name);
                    prefix_nested = strdupz(buffer);
                }

                found = get_disk_name_from_path(path_nested, result, result_size, major, minor, disk, prefix_nested, depth - 1);
                freez(path_nested);
                freez(prefix_nested);

                if(found) break;
            }
        }
        else if(de->d_type == DT_LNK || de->d_type == DT_BLK) {
            char filename[FILENAME_MAX + 1];

            if(de->d_type == DT_LNK) {
                snprintfz(filename, FILENAME_MAX, "%s/%s", path, de->d_name);
                ssize_t len = readlink(filename, result, result_size - 1);
                if(len <= 0) {
                    error("DEVICE-MAPPER ('%s', %lu:%lu): Cannot read link '%s'.", disk, major, minor, filename);
                    continue;
                }

                result[len] = '\0';
                if(result[0] != '/')
                    snprintfz(filename, FILENAME_MAX, "%s/%s", path, result);
                else
                    strncpyz(filename, result, FILENAME_MAX);
            }
            else {
                snprintfz(filename, FILENAME_MAX, "%s/%s", path, de->d_name);
            }

            struct stat sb;
            if(stat(filename, &sb) == -1) {
                error("DEVICE-MAPPER ('%s', %lu:%lu): Cannot stat() file '%s'.", disk, major, minor, filename);
                continue;
            }

            if((sb.st_mode & S_IFMT) != S_IFBLK) {
                //info("DEVICE-MAPPER ('%s', %lu:%lu): file '%s' is not a block device.", disk, major, minor, filename);
                continue;
            }

            if(major(sb.st_rdev) != major || minor(sb.st_rdev) != minor || strcmp(basename(filename), disk)) {
                //info("DEVICE-MAPPER ('%s', %lu:%lu): filename '%s' does not match %lu:%lu.", disk, major, minor, filename, (unsigned long)major(sb.st_rdev), (unsigned long)minor(sb.st_rdev));
                continue;
            }

            //info("DEVICE-MAPPER ('%s', %lu:%lu): filename '%s' matches.", disk, major, minor, filename);

            snprintfz(result, result_size - 1, "%s%s%s", (prefix)?prefix:"", (prefix)?"_":"", de->d_name);

            if(!found) {
                strncpyz(first_result, result, result_size);
                found = 1;
            }

            if(simple_pattern_matches(preferred_ids, result)) {
                preferred = 1;
                break;
            }
        }
    }
    closedir(dir);


failed:

    if(!found)
        result[0] = '\0';
    else if(!preferred)
        strncpyz(result, first_result, result_size);

    freez(first_result);

    return found;
}

static inline char *get_disk_name(unsigned long major, unsigned long minor, char *disk) {
    char result[FILENAME_MAX + 1] = "";

    if(!path_to_device_mapper || !*path_to_device_mapper || !get_disk_name_from_path(path_to_device_mapper, result, FILENAME_MAX + 1, major, minor, disk, NULL, 0))
        if(!path_to_device_label || !*path_to_device_label || !get_disk_name_from_path(path_to_device_label, result, FILENAME_MAX + 1, major, minor, disk, NULL, 0))
            if(!path_to_veritas_volume_groups || !*path_to_veritas_volume_groups || !get_disk_name_from_path(path_to_veritas_volume_groups, result, FILENAME_MAX + 1, major, minor, disk, "vx", 2))
                if(name_disks_by_id != CONFIG_BOOLEAN_YES || !path_to_device_id || !*path_to_device_id || !get_disk_name_from_path(path_to_device_id, result, FILENAME_MAX + 1, major, minor, disk, NULL, 0))
                    strncpy(result, disk, FILENAME_MAX);

    if(!result[0])
        strncpy(result, disk, FILENAME_MAX);

    netdata_fix_chart_name(result);
    return strdup(result);
}

static void get_disk_config(struct disk *d) {
    int def_enable = global_enable_new_disks_detected_at_runtime;

    if(def_enable != CONFIG_BOOLEAN_NO && (simple_pattern_matches(excluded_disks, d->device) || simple_pattern_matches(excluded_disks, d->disk)))
        def_enable = CONFIG_BOOLEAN_NO;

    char var_name[4096 + 1];
    snprintfz(var_name, 4096, CONFIG_SECTION_PLUGIN_PROC_DISKSTATS ":%s", d->disk);

    def_enable = config_get_boolean_ondemand(var_name, "enable", def_enable);
    if(unlikely(def_enable == CONFIG_BOOLEAN_NO)) {
        // the user does not want any metrics for this disk
        d->do_io = CONFIG_BOOLEAN_NO;
        d->do_ops = CONFIG_BOOLEAN_NO;
        d->do_mops = CONFIG_BOOLEAN_NO;
        d->do_iotime = CONFIG_BOOLEAN_NO;
        d->do_qops = CONFIG_BOOLEAN_NO;
        d->do_util = CONFIG_BOOLEAN_NO;
        d->do_ext = CONFIG_BOOLEAN_NO;
        d->do_backlog = CONFIG_BOOLEAN_NO;
        d->do_bcache = CONFIG_BOOLEAN_NO;
    }
    else {
        // this disk is enabled
        // check its direct settings

        int def_performance = CONFIG_BOOLEAN_AUTO;

        // since this is 'on demand' we can figure the performance settings
        // based on the type of disk

        if(!d->device_is_bcache) {
            switch(d->type) {
                default:
                case DISK_TYPE_UNKNOWN:
                    break;

                case DISK_TYPE_PHYSICAL:
                    def_performance = global_enable_performance_for_physical_disks;
                    break;

                case DISK_TYPE_PARTITION:
                    def_performance = global_enable_performance_for_partitions;
                    break;

                case DISK_TYPE_VIRTUAL:
                    def_performance = global_enable_performance_for_virtual_disks;
                    break;
            }
        }

        // check if we have to disable performance for this disk
        if(def_performance)
            def_performance = is_major_enabled((int)d->major);

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
                ddo_ext = CONFIG_BOOLEAN_NO,
                ddo_backlog = CONFIG_BOOLEAN_NO,
                ddo_bcache = CONFIG_BOOLEAN_NO;

        // we enable individual performance charts only when def_performance is not disabled
        if(unlikely(def_performance != CONFIG_BOOLEAN_NO)) {
            ddo_io = global_do_io,
            ddo_ops = global_do_ops,
            ddo_mops = global_do_mops,
            ddo_iotime = global_do_iotime,
            ddo_qops = global_do_qops,
            ddo_util = global_do_util,
            ddo_ext = global_do_ext,
            ddo_backlog = global_do_backlog,
            ddo_bcache = global_do_bcache;
        }

        d->do_io      = config_get_boolean_ondemand(var_name, "bandwidth", ddo_io);
        d->do_ops     = config_get_boolean_ondemand(var_name, "operations", ddo_ops);
        d->do_mops    = config_get_boolean_ondemand(var_name, "merged operations", ddo_mops);
        d->do_iotime  = config_get_boolean_ondemand(var_name, "i/o time", ddo_iotime);
        d->do_qops    = config_get_boolean_ondemand(var_name, "queued operations", ddo_qops);
        d->do_util    = config_get_boolean_ondemand(var_name, "utilization percentage", ddo_util);
        d->do_ext     = config_get_boolean_ondemand(var_name, "extended operations", ddo_ext);
        d->do_backlog = config_get_boolean_ondemand(var_name, "backlog", ddo_backlog);

        if(d->device_is_bcache)
            d->do_bcache  = config_get_boolean_ondemand(var_name, "bcache", ddo_bcache);
        else
            d->do_bcache = 0;
    }
}

static struct disk *get_disk(unsigned long major, unsigned long minor, char *disk) {
    static struct mountinfo *disk_mountinfo_root = NULL;

    struct disk *d;

    uint32_t hash = simple_hash(disk);

    // search for it in our RAM list.
    // this is sequential, but since we just walk through
    // and the number of disks / partitions in a system
    // should not be that many, it should be acceptable
    for(d = disk_root; d ; d = d->next){
        if (unlikely(
                d->major == major && d->minor == minor && d->hash == hash && !strcmp(d->device, disk)))
            return d;
    }

    // not found
    // create a new disk structure
    d = (struct disk *)callocz(1, sizeof(struct disk));

    d->disk = get_disk_name(major, minor, disk);
    d->device = strdupz(disk);
    d->hash = simple_hash(d->device);
    d->major = major;
    d->minor = minor;
    d->type = DISK_TYPE_UNKNOWN; // Default type. Changed later if not correct.
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

    char buffer[FILENAME_MAX + 1];

    // find if it is a physical disk
    // by checking if /sys/block/DISK is readable.
    snprintfz(buffer, FILENAME_MAX, path_to_sys_block_device, disk);
    if(likely(access(buffer, R_OK) == 0)) {
        // assign it here, but it will be overwritten if it is not a physical disk
        d->type = DISK_TYPE_PHYSICAL;
    }

    // find if it is a partition
    // by checking if /sys/dev/block/MAJOR:MINOR/partition is readable.
    snprintfz(buffer, FILENAME_MAX, path_to_sys_dev_block_major_minor_string, major, minor, "partition");
    if(likely(access(buffer, R_OK) == 0)) {
        d->type = DISK_TYPE_PARTITION;
    }
    else {
        // find if it is a virtual disk
        // by checking if /sys/devices/virtual/block/DISK is readable.
        snprintfz(buffer, FILENAME_MAX, path_to_sys_devices_virtual_block_device, disk);
        if(likely(access(buffer, R_OK) == 0)) {
            d->type = DISK_TYPE_VIRTUAL;
        }
        else {
            // find if it is a virtual device
            // by checking if /sys/dev/block/MAJOR:MINOR/slaves has entries
            snprintfz(buffer, FILENAME_MAX, path_to_sys_dev_block_major_minor_string, major, minor, "slaves/");
            DIR *dirp = opendir(buffer);
            if (likely(dirp != NULL)) {
                struct dirent *dp;
                while ((dp = readdir(dirp))) {
                    // . and .. are also files in empty folders.
                    if (unlikely(strcmp(dp->d_name, ".") == 0 || strcmp(dp->d_name, "..") == 0)) {
                        continue;
                    }

                    d->type = DISK_TYPE_VIRTUAL;

                    // Stop the loop after we found one file.
                    break;
                }
                if (unlikely(closedir(dirp) == -1))
                    error("Unable to close dir %s", buffer);
            }
        }
    }

    // ------------------------------------------------------------------------
    // check if we can find its mount point

    // mountinfo_find() can be called with NULL disk_mountinfo_root
    struct mountinfo *mi = mountinfo_find(disk_mountinfo_root, d->major, d->minor, d->device);
    if(unlikely(!mi)) {
        // mountinfo_free_all can be called with NULL
        mountinfo_free_all(disk_mountinfo_root);
        disk_mountinfo_root = mountinfo_read(0);
        mi = mountinfo_find(disk_mountinfo_root, d->major, d->minor, d->device);
    }

    if(unlikely(mi))
        d->mount_point = strdupz(mi->mount_point);
    else
        d->mount_point = NULL;

    // ------------------------------------------------------------------------
    // find the disk sector size

    /*
     * sector size is always 512 bytes inside the kernel #3481
     *
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
    */

    // ------------------------------------------------------------------------
    // check if the device is a bcache

    struct stat bcache;
    snprintfz(buffer, FILENAME_MAX, path_to_sys_block_device_bcache, disk);
    if(unlikely(stat(buffer, &bcache) == 0 && (bcache.st_mode & S_IFMT) == S_IFDIR)) {
        // we have the 'bcache' directory
        d->device_is_bcache = 1;

        char buffer2[FILENAME_MAX + 1];

        snprintfz(buffer2, FILENAME_MAX, "%s/cache/congested", buffer);
        if(access(buffer2, R_OK) == 0)
            d->bcache_filename_cache_congested = strdupz(buffer2);
        else
            error("bcache file '%s' cannot be read.", buffer2);

        snprintfz(buffer2, FILENAME_MAX, "%s/readahead", buffer);
        if(access(buffer2, R_OK) == 0)
            d->bcache_filename_stats_total_cache_readaheads = strdupz(buffer2);
        else
            error("bcache file '%s' cannot be read.", buffer2);

        snprintfz(buffer2, FILENAME_MAX, "%s/cache/cache0/priority_stats", buffer); // only one cache is supported by bcache
        if(access(buffer2, R_OK) == 0)
            d->bcache_filename_priority_stats = strdupz(buffer2);
        else
            error("bcache file '%s' cannot be read.", buffer2);

        snprintfz(buffer2, FILENAME_MAX, "%s/cache/internal/cache_read_races", buffer);
        if(access(buffer2, R_OK) == 0)
            d->bcache_filename_cache_read_races = strdupz(buffer2);
        else
            error("bcache file '%s' cannot be read.", buffer2);

        snprintfz(buffer2, FILENAME_MAX, "%s/cache/cache0/io_errors", buffer);
        if(access(buffer2, R_OK) == 0)
            d->bcache_filename_cache_io_errors = strdupz(buffer2);
        else
            error("bcache file '%s' cannot be read.", buffer2);

        snprintfz(buffer2, FILENAME_MAX, "%s/dirty_data", buffer);
        if(access(buffer2, R_OK) == 0)
            d->bcache_filename_dirty_data = strdupz(buffer2);
        else
            error("bcache file '%s' cannot be read.", buffer2);

        snprintfz(buffer2, FILENAME_MAX, "%s/writeback_rate", buffer);
        if(access(buffer2, R_OK) == 0)
            d->bcache_filename_writeback_rate = strdupz(buffer2);
        else
            error("bcache file '%s' cannot be read.", buffer2);

        snprintfz(buffer2, FILENAME_MAX, "%s/cache/cache_available_percent", buffer);
        if(access(buffer2, R_OK) == 0)
            d->bcache_filename_cache_available_percent = strdupz(buffer2);
        else
            error("bcache file '%s' cannot be read.", buffer2);

        snprintfz(buffer2, FILENAME_MAX, "%s/stats_total/cache_hits", buffer);
        if(access(buffer2, R_OK) == 0)
            d->bcache_filename_stats_total_cache_hits = strdupz(buffer2);
        else
            error("bcache file '%s' cannot be read.", buffer2);

        snprintfz(buffer2, FILENAME_MAX, "%s/stats_five_minute/cache_hit_ratio", buffer);
        if(access(buffer2, R_OK) == 0)
            d->bcache_filename_stats_five_minute_cache_hit_ratio = strdupz(buffer2);
        else
            error("bcache file '%s' cannot be read.", buffer2);

        snprintfz(buffer2, FILENAME_MAX, "%s/stats_hour/cache_hit_ratio", buffer);
        if(access(buffer2, R_OK) == 0)
            d->bcache_filename_stats_hour_cache_hit_ratio = strdupz(buffer2);
        else
            error("bcache file '%s' cannot be read.", buffer2);

        snprintfz(buffer2, FILENAME_MAX, "%s/stats_day/cache_hit_ratio", buffer);
        if(access(buffer2, R_OK) == 0)
            d->bcache_filename_stats_day_cache_hit_ratio = strdupz(buffer2);
        else
            error("bcache file '%s' cannot be read.", buffer2);

        snprintfz(buffer2, FILENAME_MAX, "%s/stats_total/cache_hit_ratio", buffer);
        if(access(buffer2, R_OK) == 0)
            d->bcache_filename_stats_total_cache_hit_ratio = strdupz(buffer2);
        else
            error("bcache file '%s' cannot be read.", buffer2);

        snprintfz(buffer2, FILENAME_MAX, "%s/stats_total/cache_misses", buffer);
        if(access(buffer2, R_OK) == 0)
            d->bcache_filename_stats_total_cache_misses = strdupz(buffer2);
        else
            error("bcache file '%s' cannot be read.", buffer2);

        snprintfz(buffer2, FILENAME_MAX, "%s/stats_total/cache_bypass_hits", buffer);
        if(access(buffer2, R_OK) == 0)
            d->bcache_filename_stats_total_cache_bypass_hits = strdupz(buffer2);
        else
            error("bcache file '%s' cannot be read.", buffer2);

        snprintfz(buffer2, FILENAME_MAX, "%s/stats_total/cache_bypass_misses", buffer);
        if(access(buffer2, R_OK) == 0)
            d->bcache_filename_stats_total_cache_bypass_misses = strdupz(buffer2);
        else
            error("bcache file '%s' cannot be read.", buffer2);

        snprintfz(buffer2, FILENAME_MAX, "%s/stats_total/cache_miss_collisions", buffer);
        if(access(buffer2, R_OK) == 0)
            d->bcache_filename_stats_total_cache_miss_collisions = strdupz(buffer2);
        else
            error("bcache file '%s' cannot be read.", buffer2);
    }

    get_disk_config(d);
    return d;
}

int do_proc_diskstats(int update_every, usec_t dt) {
    static procfile *ff = NULL;

    if(unlikely(!globals_initialized)) {
        globals_initialized = 1;

        global_enable_new_disks_detected_at_runtime = config_get_boolean(CONFIG_SECTION_PLUGIN_PROC_DISKSTATS, "enable new disks detected at runtime", global_enable_new_disks_detected_at_runtime);
        global_enable_performance_for_physical_disks = config_get_boolean_ondemand(CONFIG_SECTION_PLUGIN_PROC_DISKSTATS, "performance metrics for physical disks", global_enable_performance_for_physical_disks);
        global_enable_performance_for_virtual_disks = config_get_boolean_ondemand(CONFIG_SECTION_PLUGIN_PROC_DISKSTATS, "performance metrics for virtual disks", global_enable_performance_for_virtual_disks);
        global_enable_performance_for_partitions = config_get_boolean_ondemand(CONFIG_SECTION_PLUGIN_PROC_DISKSTATS, "performance metrics for partitions", global_enable_performance_for_partitions);

        global_do_io      = config_get_boolean_ondemand(CONFIG_SECTION_PLUGIN_PROC_DISKSTATS, "bandwidth for all disks", global_do_io);
        global_do_ops     = config_get_boolean_ondemand(CONFIG_SECTION_PLUGIN_PROC_DISKSTATS, "operations for all disks", global_do_ops);
        global_do_mops    = config_get_boolean_ondemand(CONFIG_SECTION_PLUGIN_PROC_DISKSTATS, "merged operations for all disks", global_do_mops);
        global_do_iotime  = config_get_boolean_ondemand(CONFIG_SECTION_PLUGIN_PROC_DISKSTATS, "i/o time for all disks", global_do_iotime);
        global_do_qops    = config_get_boolean_ondemand(CONFIG_SECTION_PLUGIN_PROC_DISKSTATS, "queued operations for all disks", global_do_qops);
        global_do_util    = config_get_boolean_ondemand(CONFIG_SECTION_PLUGIN_PROC_DISKSTATS, "utilization percentage for all disks", global_do_util);
        global_do_ext     = config_get_boolean_ondemand(CONFIG_SECTION_PLUGIN_PROC_DISKSTATS, "extended operations for all disks", global_do_ext);
        global_do_backlog = config_get_boolean_ondemand(CONFIG_SECTION_PLUGIN_PROC_DISKSTATS, "backlog for all disks", global_do_backlog);
        global_do_bcache  = config_get_boolean_ondemand(CONFIG_SECTION_PLUGIN_PROC_DISKSTATS, "bcache for all disks", global_do_bcache);
        global_bcache_priority_stats_update_every = (int)config_get_number(CONFIG_SECTION_PLUGIN_PROC_DISKSTATS, "bcache priority stats update every", global_bcache_priority_stats_update_every);

        global_cleanup_removed_disks = config_get_boolean(CONFIG_SECTION_PLUGIN_PROC_DISKSTATS, "remove charts of removed disks" , global_cleanup_removed_disks);

        char buffer[FILENAME_MAX + 1];

        snprintfz(buffer, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/sys/block/%s");
        path_to_sys_block_device = config_get(CONFIG_SECTION_PLUGIN_PROC_DISKSTATS, "path to get block device", buffer);

        snprintfz(buffer, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/sys/block/%s/bcache");
        path_to_sys_block_device_bcache = config_get(CONFIG_SECTION_PLUGIN_PROC_DISKSTATS, "path to get block device bcache", buffer);

        snprintfz(buffer, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/sys/devices/virtual/block/%s");
        path_to_sys_devices_virtual_block_device = config_get(CONFIG_SECTION_PLUGIN_PROC_DISKSTATS, "path to get virtual block device", buffer);

        snprintfz(buffer, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/sys/dev/block/%lu:%lu/%s");
        path_to_sys_dev_block_major_minor_string = config_get(CONFIG_SECTION_PLUGIN_PROC_DISKSTATS, "path to get block device infos", buffer);

        //snprintfz(buffer, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/sys/block/%s/queue/hw_sector_size");
        //path_to_get_hw_sector_size = config_get(CONFIG_SECTION_PLUGIN_PROC_DISKSTATS, "path to get h/w sector size", buffer);

        //snprintfz(buffer, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/sys/dev/block/%lu:%lu/subsystem/%s/../queue/hw_sector_size");
        //path_to_get_hw_sector_size_partitions = config_get(CONFIG_SECTION_PLUGIN_PROC_DISKSTATS, "path to get h/w sector size for partitions", buffer);

        snprintfz(buffer, FILENAME_MAX, "%s/dev/mapper", netdata_configured_host_prefix);
        path_to_device_mapper = config_get(CONFIG_SECTION_PLUGIN_PROC_DISKSTATS, "path to device mapper", buffer);

        snprintfz(buffer, FILENAME_MAX, "%s/dev/disk/by-label", netdata_configured_host_prefix);
        path_to_device_label = config_get(CONFIG_SECTION_PLUGIN_PROC_DISKSTATS, "path to /dev/disk/by-label", buffer);

        snprintfz(buffer, FILENAME_MAX, "%s/dev/disk/by-id", netdata_configured_host_prefix);
        path_to_device_id = config_get(CONFIG_SECTION_PLUGIN_PROC_DISKSTATS, "path to /dev/disk/by-id", buffer);

        snprintfz(buffer, FILENAME_MAX, "%s/dev/vx/dsk", netdata_configured_host_prefix);
        path_to_veritas_volume_groups = config_get(CONFIG_SECTION_PLUGIN_PROC_DISKSTATS, "path to /dev/vx/dsk", buffer);

        name_disks_by_id = config_get_boolean(CONFIG_SECTION_PLUGIN_PROC_DISKSTATS, "name disks by id", name_disks_by_id);

        preferred_ids = simple_pattern_create(
                config_get(CONFIG_SECTION_PLUGIN_PROC_DISKSTATS, "preferred disk ids", DEFAULT_PREFERRED_IDS)
                , NULL
                , SIMPLE_PATTERN_EXACT
        );

        excluded_disks = simple_pattern_create(
                config_get(CONFIG_SECTION_PLUGIN_PROC_DISKSTATS, "exclude disks", DEFAULT_EXCLUDED_DISKS)
                , NULL
                , SIMPLE_PATTERN_EXACT
        );
    }

    // --------------------------------------------------------------------------

    if(unlikely(!ff)) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/proc/diskstats");
        ff = procfile_open(config_get(CONFIG_SECTION_PLUGIN_PROC_DISKSTATS, "filename to monitor", filename), " \t", PROCFILE_FLAG_DEFAULT);
    }
    if(unlikely(!ff)) return 0;

    ff = procfile_readall(ff);
    if(unlikely(!ff)) return 0; // we return 0, so that we will retry to open it next time

    size_t lines = procfile_lines(ff), l;

    collected_number system_read_kb = 0, system_write_kb = 0;

    int do_dc_stats = 0, do_fl_stats = 0;

    for(l = 0; l < lines ;l++) {
        // --------------------------------------------------------------------------
        // Read parameters

        char *disk;
        unsigned long       major = 0, minor = 0;

        collected_number    reads = 0,  mreads = 0,  readsectors = 0,  readms = 0,
                            writes = 0, mwrites = 0, writesectors = 0, writems = 0,
                            queued_ios = 0, busy_ms = 0, backlog_ms = 0,
                            discards = 0, mdiscards = 0, discardsectors = 0, discardms = 0,
                            flushes = 0, flushms = 0;


        collected_number    last_reads = 0,  last_readsectors = 0,  last_readms = 0,
                            last_writes = 0, last_writesectors = 0, last_writems = 0,
                            last_busy_ms = 0,
                            last_discards = 0, last_discardsectors = 0, last_discardms = 0,
                            last_flushes = 0, last_flushms = 0;

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

        if (unlikely(words > 13)) {
            do_dc_stats = 1;

            // # of discards completed
            // This is the total number of discards completed successfully.
            discards       = str2ull(procfile_lineword(ff, l, 14)); // dc_ios

            // # of discards merged
            // See the description of mreads/mwrites
            mdiscards      = str2ull(procfile_lineword(ff, l, 15)); // dc_merges

            // # of sectors discarded
            // This is the total number of sectors discarded successfully.
            discardsectors = str2ull(procfile_lineword(ff, l, 16)); // dc_sec

            // # of milliseconds spent discarding
            // This is the total number of milliseconds spent by all discards (as
            // measured from __make_request() to end_that_request_last()).
            discardms      = str2ull(procfile_lineword(ff, l, 17)); // dc_ticks
        }

        if (unlikely(words > 17)) {
            do_fl_stats = 1;

            // number of flush I/Os processed
            // These values increment when an flush I/O request completes.
            // Block layer combines flush requests and executes at most one at a time.
            // This counts flush requests executed by disk. Not tracked for partitions.
            flushes        = str2ull(procfile_lineword(ff, l, 18)); // fl_ios

            // total wait time for flush requests
            flushms        = str2ull(procfile_lineword(ff, l, 19)); // fl_ticks
        }

        // --------------------------------------------------------------------------
        // get a disk structure for the disk

        struct disk *d = get_disk(major, minor, disk);
        d->updated = 1;

        // --------------------------------------------------------------------------
        // count the global system disk I/O of physical disks

        if(unlikely(d->type == DISK_TYPE_PHYSICAL)) {
            system_read_kb  += readsectors * d->sector_size / 1024;
            system_write_kb += writesectors * d->sector_size / 1024;
        }

        // --------------------------------------------------------------------------
        // Set its family based on mount point

        char *family = d->mount_point;
        if(!family) family = d->disk;


        // --------------------------------------------------------------------------
        // Do performance metrics

        if(d->do_io == CONFIG_BOOLEAN_YES || (d->do_io == CONFIG_BOOLEAN_AUTO &&
                                              (readsectors || writesectors || discardsectors ||
                                               netdata_zero_metrics_enabled == CONFIG_BOOLEAN_YES))) {
            d->do_io = CONFIG_BOOLEAN_YES;

            if(unlikely(!d->st_io)) {
                d->st_io = rrdset_create_localhost(
                        RRD_TYPE_DISK
                        , d->device
                        , d->disk
                        , family
                        , "disk.io"
                        , "Disk I/O Bandwidth"
                        , "KiB/s"
                        , PLUGIN_PROC_NAME
                        , PLUGIN_PROC_MODULE_DISKSTATS_NAME
                        , NETDATA_CHART_PRIO_DISK_IO
                        , update_every
                        , RRDSET_TYPE_AREA
                );

                d->rd_io_reads  = rrddim_add(d->st_io, "reads",  NULL, d->sector_size, 1024,      RRD_ALGORITHM_INCREMENTAL);
                d->rd_io_writes = rrddim_add(d->st_io, "writes", NULL, d->sector_size * -1, 1024, RRD_ALGORITHM_INCREMENTAL);
            }
            else rrdset_next(d->st_io);

            last_readsectors  = rrddim_set_by_pointer(d->st_io, d->rd_io_reads, readsectors);
            last_writesectors = rrddim_set_by_pointer(d->st_io, d->rd_io_writes, writesectors);
            rrdset_done(d->st_io);
        }

        // --------------------------------------------------------------------

        if (do_dc_stats && d->do_io == CONFIG_BOOLEAN_YES && d->do_ext != CONFIG_BOOLEAN_NO) {
            if (unlikely(!d->st_ext_io)) {
                d->st_ext_io = rrdset_create_localhost(
                        "disk_ext"
                        , d->device
                        , d->disk
                        , family
                        , "disk_ext.io"
                        , "Amount of Discarded Data"
                        , "KiB/s"
                        , PLUGIN_PROC_NAME
                        , PLUGIN_PROC_MODULE_DISKSTATS_NAME
                        , NETDATA_CHART_PRIO_DISK_IO + 1
                        , update_every
                        , RRDSET_TYPE_AREA
                );

                d->rd_io_discards =
                    rrddim_add(d->st_ext_io, "discards", NULL, d->sector_size, 1024, RRD_ALGORITHM_INCREMENTAL);
            } else
                rrdset_next(d->st_ext_io);

            last_discardsectors = rrddim_set_by_pointer(d->st_ext_io, d->rd_io_discards, discardsectors);
            rrdset_done(d->st_ext_io);
        }

        // --------------------------------------------------------------------

        if(d->do_ops == CONFIG_BOOLEAN_YES || (d->do_ops == CONFIG_BOOLEAN_AUTO &&
                                               (reads || writes || discards || flushes ||
                                                netdata_zero_metrics_enabled == CONFIG_BOOLEAN_YES))) {
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
                        , PLUGIN_PROC_NAME
                        , PLUGIN_PROC_MODULE_DISKSTATS_NAME
                        , NETDATA_CHART_PRIO_DISK_OPS
                        , update_every
                        , RRDSET_TYPE_LINE
                );

                rrdset_flag_set(d->st_ops, RRDSET_FLAG_DETAIL);

                d->rd_ops_reads  = rrddim_add(d->st_ops, "reads",  NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                d->rd_ops_writes = rrddim_add(d->st_ops, "writes", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            }
            else rrdset_next(d->st_ops);

            last_reads  = rrddim_set_by_pointer(d->st_ops, d->rd_ops_reads, reads);
            last_writes = rrddim_set_by_pointer(d->st_ops, d->rd_ops_writes, writes);
            rrdset_done(d->st_ops);
        }

        // --------------------------------------------------------------------

        if (do_dc_stats && d->do_ops == CONFIG_BOOLEAN_YES && d->do_ext != CONFIG_BOOLEAN_NO) {
            if (unlikely(!d->st_ext_ops)) {
                d->st_ext_ops = rrdset_create_localhost(
                        "disk_ext_ops"
                        , d->device
                        , d->disk
                        , family
                        , "disk_ext.ops"
                        , "Disk Completed Extended I/O Operations"
                        , "operations/s"
                        , PLUGIN_PROC_NAME
                        , PLUGIN_PROC_MODULE_DISKSTATS_NAME
                        , NETDATA_CHART_PRIO_DISK_OPS + 1
                        , update_every
                        , RRDSET_TYPE_LINE
                );

                rrdset_flag_set(d->st_ext_ops, RRDSET_FLAG_DETAIL);

                d->rd_ops_discards = rrddim_add(d->st_ext_ops, "discards", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                if (do_fl_stats)
                    d->rd_ops_flushes = rrddim_add(d->st_ext_ops, "flushes", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            } else
                rrdset_next(d->st_ext_ops);

            last_discards = rrddim_set_by_pointer(d->st_ext_ops, d->rd_ops_discards, discards);
            if (do_fl_stats)
                last_flushes = rrddim_set_by_pointer(d->st_ext_ops, d->rd_ops_flushes, flushes);
            rrdset_done(d->st_ext_ops);
        }

        // --------------------------------------------------------------------

        if(d->do_qops == CONFIG_BOOLEAN_YES || (d->do_qops == CONFIG_BOOLEAN_AUTO &&
                                                (queued_ios || netdata_zero_metrics_enabled == CONFIG_BOOLEAN_YES))) {
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
                        , PLUGIN_PROC_NAME
                        , PLUGIN_PROC_MODULE_DISKSTATS_NAME
                        , NETDATA_CHART_PRIO_DISK_QOPS
                        , update_every
                        , RRDSET_TYPE_LINE
                );

                rrdset_flag_set(d->st_qops, RRDSET_FLAG_DETAIL);

                d->rd_qops_operations = rrddim_add(d->st_qops, "operations", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            }
            else rrdset_next(d->st_qops);

            rrddim_set_by_pointer(d->st_qops, d->rd_qops_operations, queued_ios);
            rrdset_done(d->st_qops);
        }

        // --------------------------------------------------------------------

        if(d->do_backlog == CONFIG_BOOLEAN_YES || (d->do_backlog == CONFIG_BOOLEAN_AUTO &&
                                                   (backlog_ms || netdata_zero_metrics_enabled == CONFIG_BOOLEAN_YES))) {
            d->do_backlog = CONFIG_BOOLEAN_YES;

            if(unlikely(!d->st_backlog)) {
                d->st_backlog = rrdset_create_localhost(
                        "disk_backlog"
                        , d->device
                        , d->disk
                        , family
                        , "disk.backlog"
                        , "Disk Backlog"
                        , "milliseconds"
                        , PLUGIN_PROC_NAME
                        , PLUGIN_PROC_MODULE_DISKSTATS_NAME
                        , NETDATA_CHART_PRIO_DISK_BACKLOG
                        , update_every
                        , RRDSET_TYPE_AREA
                );

                rrdset_flag_set(d->st_backlog, RRDSET_FLAG_DETAIL);

                d->rd_backlog_backlog = rrddim_add(d->st_backlog, "backlog", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            }
            else rrdset_next(d->st_backlog);

            rrddim_set_by_pointer(d->st_backlog, d->rd_backlog_backlog, backlog_ms);
            rrdset_done(d->st_backlog);
        }

        // --------------------------------------------------------------------

        if(d->do_util == CONFIG_BOOLEAN_YES || (d->do_util == CONFIG_BOOLEAN_AUTO &&
                                                (busy_ms || netdata_zero_metrics_enabled == CONFIG_BOOLEAN_YES))) {
            d->do_util = CONFIG_BOOLEAN_YES;

            if(unlikely(!d->st_busy)) {
                d->st_busy = rrdset_create_localhost(
                        "disk_busy"
                        , d->device
                        , d->disk
                        , family
                        , "disk.busy"
                        , "Disk Busy Time"
                        , "milliseconds"
                        , PLUGIN_PROC_NAME
                        , PLUGIN_PROC_MODULE_DISKSTATS_NAME
                        , NETDATA_CHART_PRIO_DISK_BUSY
                        , update_every
                        , RRDSET_TYPE_AREA
                );

                rrdset_flag_set(d->st_busy, RRDSET_FLAG_DETAIL);

                d->rd_busy_busy =
                    rrddim_add(d->st_busy, "busy", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            }
            else rrdset_next(d->st_busy);

            last_busy_ms = rrddim_set_by_pointer(d->st_busy, d->rd_busy_busy, busy_ms);
            rrdset_done(d->st_busy);

        // --------------------------------------------------------------------

            if(unlikely(!d->st_util)) {
                d->st_util = rrdset_create_localhost(
                        "disk_util"
                        , d->device
                        , d->disk
                        , family
                        , "disk.util"
                        , "Disk Utilization Time"
                        , "% of time working"
                        , PLUGIN_PROC_NAME
                        , PLUGIN_PROC_MODULE_DISKSTATS_NAME
                        , NETDATA_CHART_PRIO_DISK_UTIL
                        , update_every
                        , RRDSET_TYPE_AREA
                );

                rrdset_flag_set(d->st_util, RRDSET_FLAG_DETAIL);

                d->rd_util_utilization = rrddim_add(d->st_util, "utilization", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            }
            else rrdset_next(d->st_util);

            collected_number disk_utilization = (busy_ms - last_busy_ms) / (10 * update_every);
            if (disk_utilization > 100)
                disk_utilization = 100;

            rrddim_set_by_pointer(d->st_util, d->rd_util_utilization, disk_utilization);
            rrdset_done(d->st_util);
        }

        // --------------------------------------------------------------------

        if(d->do_mops == CONFIG_BOOLEAN_YES || (d->do_mops == CONFIG_BOOLEAN_AUTO &&
                                                (mreads || mwrites || mdiscards ||
                                                 netdata_zero_metrics_enabled == CONFIG_BOOLEAN_YES))) {
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
                        , PLUGIN_PROC_NAME
                        , PLUGIN_PROC_MODULE_DISKSTATS_NAME
                        , NETDATA_CHART_PRIO_DISK_MOPS
                        , update_every
                        , RRDSET_TYPE_LINE
                );

                rrdset_flag_set(d->st_mops, RRDSET_FLAG_DETAIL);

                d->rd_mops_reads  = rrddim_add(d->st_mops, "reads",  NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                d->rd_mops_writes = rrddim_add(d->st_mops, "writes", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            }
            else rrdset_next(d->st_mops);

            rrddim_set_by_pointer(d->st_mops, d->rd_mops_reads,  mreads);
            rrddim_set_by_pointer(d->st_mops, d->rd_mops_writes, mwrites);
            rrdset_done(d->st_mops);
        }

        // --------------------------------------------------------------------

        if(do_dc_stats && d->do_mops == CONFIG_BOOLEAN_YES && d->do_ext != CONFIG_BOOLEAN_NO) {
            d->do_mops = CONFIG_BOOLEAN_YES;

            if(unlikely(!d->st_ext_mops)) {
                d->st_ext_mops = rrdset_create_localhost(
                        "disk_ext_mops"
                        , d->device
                        , d->disk
                        , family
                        , "disk_ext.mops"
                        , "Disk Merged Discard Operations"
                        , "merged operations/s"
                        , PLUGIN_PROC_NAME
                        , PLUGIN_PROC_MODULE_DISKSTATS_NAME
                        , NETDATA_CHART_PRIO_DISK_MOPS + 1
                        , update_every
                        , RRDSET_TYPE_LINE
                );

                rrdset_flag_set(d->st_ext_mops, RRDSET_FLAG_DETAIL);

                d->rd_mops_discards = rrddim_add(d->st_ext_mops, "discards", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            } else
                rrdset_next(d->st_ext_mops);

            rrddim_set_by_pointer(d->st_ext_mops, d->rd_mops_discards, mdiscards);
            rrdset_done(d->st_ext_mops);
        }

        // --------------------------------------------------------------------

        if(d->do_iotime == CONFIG_BOOLEAN_YES || (d->do_iotime == CONFIG_BOOLEAN_AUTO &&
                                                  (readms || writems || discardms || flushms || netdata_zero_metrics_enabled == CONFIG_BOOLEAN_YES))) {
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
                        , PLUGIN_PROC_NAME
                        , PLUGIN_PROC_MODULE_DISKSTATS_NAME
                        , NETDATA_CHART_PRIO_DISK_IOTIME
                        , update_every
                        , RRDSET_TYPE_LINE
                );

                rrdset_flag_set(d->st_iotime, RRDSET_FLAG_DETAIL);

                d->rd_iotime_reads  = rrddim_add(d->st_iotime, "reads",  NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                d->rd_iotime_writes = rrddim_add(d->st_iotime, "writes", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            }
            else rrdset_next(d->st_iotime);

            last_readms  = rrddim_set_by_pointer(d->st_iotime, d->rd_iotime_reads, readms);
            last_writems = rrddim_set_by_pointer(d->st_iotime, d->rd_iotime_writes, writems);
            rrdset_done(d->st_iotime);
        }

        // --------------------------------------------------------------------

        if(do_dc_stats && d->do_iotime == CONFIG_BOOLEAN_YES && d->do_ext != CONFIG_BOOLEAN_NO) {
            if(unlikely(!d->st_ext_iotime)) {
                d->st_ext_iotime = rrdset_create_localhost(
                        "disk_ext_iotime"
                        , d->device
                        , d->disk
                        , family
                        , "disk_ext.iotime"
                        , "Disk Total I/O Time for Extended Operations"
                        , "milliseconds/s"
                        , PLUGIN_PROC_NAME
                        , PLUGIN_PROC_MODULE_DISKSTATS_NAME
                        , NETDATA_CHART_PRIO_DISK_IOTIME + 1 
                        , update_every
                        , RRDSET_TYPE_LINE
                );

                rrdset_flag_set(d->st_ext_iotime, RRDSET_FLAG_DETAIL);

                d->rd_iotime_discards = rrddim_add(d->st_ext_iotime, "discards", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                if (do_fl_stats)
                    d->rd_iotime_flushes =
                        rrddim_add(d->st_ext_iotime, "flushes", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            } else
                rrdset_next(d->st_ext_iotime);

            last_discardms = rrddim_set_by_pointer(d->st_ext_iotime, d->rd_iotime_discards, discardms);
            if (do_fl_stats)
                last_flushms = rrddim_set_by_pointer(d->st_ext_iotime, d->rd_iotime_flushes, flushms);
            rrdset_done(d->st_ext_iotime);
        }

        // --------------------------------------------------------------------
        // calculate differential charts
        // only if this is not the first time we run

        if(likely(dt)) {
            if( (d->do_iotime == CONFIG_BOOLEAN_YES || (d->do_iotime == CONFIG_BOOLEAN_AUTO &&
                                                        (readms || writems ||
                                                         netdata_zero_metrics_enabled == CONFIG_BOOLEAN_YES))) &&
                (d->do_ops    == CONFIG_BOOLEAN_YES || (d->do_ops    == CONFIG_BOOLEAN_AUTO &&
                                                        (reads || writes ||
                                                         netdata_zero_metrics_enabled == CONFIG_BOOLEAN_YES)))) {

                if(unlikely(!d->st_await)) {
                    d->st_await = rrdset_create_localhost(
                            "disk_await"
                            , d->device
                            , d->disk
                            , family
                            , "disk.await"
                            , "Average Completed I/O Operation Time"
                            , "milliseconds/operation"
                            , PLUGIN_PROC_NAME
                            , PLUGIN_PROC_MODULE_DISKSTATS_NAME
                            , NETDATA_CHART_PRIO_DISK_AWAIT
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    rrdset_flag_set(d->st_await, RRDSET_FLAG_DETAIL);

                    d->rd_await_reads  = rrddim_add(d->st_await, "reads",  NULL,  1, 1, RRD_ALGORITHM_ABSOLUTE);
                    d->rd_await_writes = rrddim_add(d->st_await, "writes", NULL, -1, 1, RRD_ALGORITHM_ABSOLUTE);
                }
                else rrdset_next(d->st_await);

                rrddim_set_by_pointer(d->st_await, d->rd_await_reads,  (reads  - last_reads)  ? (readms  - last_readms)  / (reads  - last_reads)  : 0);
                rrddim_set_by_pointer(d->st_await, d->rd_await_writes, (writes - last_writes) ? (writems - last_writems) / (writes - last_writes) : 0);
                rrdset_done(d->st_await);
            }

            if (do_dc_stats && d->do_iotime == CONFIG_BOOLEAN_YES && d->do_ops == CONFIG_BOOLEAN_YES && d->do_ext != CONFIG_BOOLEAN_NO) {
                if(unlikely(!d->st_ext_await)) {
                    d->st_ext_await = rrdset_create_localhost(
                            "disk_ext_await"
                            , d->device
                            , d->disk
                            , family
                            , "disk_ext.await"
                            , "Average Completed Extended I/O Operation Time"
                            , "milliseconds/operation"
                            , PLUGIN_PROC_NAME
                            , PLUGIN_PROC_MODULE_DISKSTATS_NAME
                            , NETDATA_CHART_PRIO_DISK_AWAIT + 1
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    rrdset_flag_set(d->st_ext_await, RRDSET_FLAG_DETAIL);

                    d->rd_await_discards = rrddim_add(d->st_ext_await, "discards", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                    if (do_fl_stats)
                        d->rd_await_flushes =
                            rrddim_add(d->st_ext_await, "flushes", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                } else
                    rrdset_next(d->st_ext_await);

                rrddim_set_by_pointer(
                    d->st_ext_await, d->rd_await_discards,
                    (discards - last_discards) ? (discardms - last_discardms) / (discards - last_discards) : 0);
                if (do_fl_stats)
                    rrddim_set_by_pointer(
                        d->st_ext_await, d->rd_await_flushes,
                        (flushes - last_flushes) ? (flushms - last_flushms) / (flushes - last_flushes) : 0);
                rrdset_done(d->st_ext_await);
            }

            if( (d->do_io  == CONFIG_BOOLEAN_YES || (d->do_io  == CONFIG_BOOLEAN_AUTO &&
                                                     (readsectors || writesectors || netdata_zero_metrics_enabled == CONFIG_BOOLEAN_YES))) &&
                (d->do_ops == CONFIG_BOOLEAN_YES || (d->do_ops == CONFIG_BOOLEAN_AUTO &&
                                                     (reads || writes || netdata_zero_metrics_enabled == CONFIG_BOOLEAN_YES)))) {

                if(unlikely(!d->st_avgsz)) {
                    d->st_avgsz = rrdset_create_localhost(
                            "disk_avgsz"
                            , d->device
                            , d->disk
                            , family
                            , "disk.avgsz"
                            , "Average Completed I/O Operation Bandwidth"
                            , "KiB/operation"
                            , PLUGIN_PROC_NAME
                            , PLUGIN_PROC_MODULE_DISKSTATS_NAME
                            , NETDATA_CHART_PRIO_DISK_AVGSZ
                            , update_every
                            , RRDSET_TYPE_AREA
                    );

                    rrdset_flag_set(d->st_avgsz, RRDSET_FLAG_DETAIL);

                    d->rd_avgsz_reads  = rrddim_add(d->st_avgsz, "reads",  NULL, d->sector_size, 1024,      RRD_ALGORITHM_ABSOLUTE);
                    d->rd_avgsz_writes = rrddim_add(d->st_avgsz, "writes", NULL, d->sector_size * -1, 1024, RRD_ALGORITHM_ABSOLUTE);
                }
                else rrdset_next(d->st_avgsz);

                rrddim_set_by_pointer(d->st_avgsz, d->rd_avgsz_reads,  (reads  - last_reads)  ? (readsectors  - last_readsectors)  / (reads  - last_reads)  : 0);
                rrddim_set_by_pointer(d->st_avgsz, d->rd_avgsz_writes, (writes - last_writes) ? (writesectors - last_writesectors) / (writes - last_writes) : 0);
                rrdset_done(d->st_avgsz);
            }

            if(do_dc_stats && d->do_io  == CONFIG_BOOLEAN_YES && d->do_ops == CONFIG_BOOLEAN_YES && d->do_ext != CONFIG_BOOLEAN_NO) {
                if(unlikely(!d->st_ext_avgsz)) {
                    d->st_ext_avgsz = rrdset_create_localhost(
                            "disk_ext_avgsz"
                            , d->device
                            , d->disk
                            , family
                            , "disk_ext.avgsz"
                            , "Average Amount of Discarded Data"
                            , "KiB/operation"
                            , PLUGIN_PROC_NAME
                            , PLUGIN_PROC_MODULE_DISKSTATS_NAME
                            , NETDATA_CHART_PRIO_DISK_AVGSZ
                            , update_every
                            , RRDSET_TYPE_AREA
                    );

                    rrdset_flag_set(d->st_ext_avgsz, RRDSET_FLAG_DETAIL);

                    d->rd_avgsz_discards =
                        rrddim_add(d->st_ext_avgsz, "discards", NULL, d->sector_size, 1024, RRD_ALGORITHM_ABSOLUTE);
                } else
                    rrdset_next(d->st_ext_avgsz);

                rrddim_set_by_pointer(
                    d->st_ext_avgsz, d->rd_avgsz_discards,
                    (discards - last_discards) ? (discardsectors - last_discardsectors) / (discards - last_discards) :
                                                 0);
                rrdset_done(d->st_ext_avgsz);
            }

            if( (d->do_util == CONFIG_BOOLEAN_YES || (d->do_util == CONFIG_BOOLEAN_AUTO &&
                                                      (busy_ms ||
                                                       netdata_zero_metrics_enabled == CONFIG_BOOLEAN_YES))) &&
                (d->do_ops  == CONFIG_BOOLEAN_YES || (d->do_ops  == CONFIG_BOOLEAN_AUTO &&
                                                      (reads || writes ||
                                                       netdata_zero_metrics_enabled == CONFIG_BOOLEAN_YES)))) {

                if(unlikely(!d->st_svctm)) {
                    d->st_svctm = rrdset_create_localhost(
                            "disk_svctm"
                            , d->device
                            , d->disk
                            , family
                            , "disk.svctm"
                            , "Average Service Time"
                            , "milliseconds/operation"
                            , PLUGIN_PROC_NAME
                            , PLUGIN_PROC_MODULE_DISKSTATS_NAME
                            , NETDATA_CHART_PRIO_DISK_SVCTM
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    rrdset_flag_set(d->st_svctm, RRDSET_FLAG_DETAIL);

                    d->rd_svctm_svctm = rrddim_add(d->st_svctm, "svctm", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                }
                else rrdset_next(d->st_svctm);

                rrddim_set_by_pointer(d->st_svctm, d->rd_svctm_svctm, ((reads - last_reads) + (writes - last_writes)) ? (busy_ms - last_busy_ms) / ((reads - last_reads) + (writes - last_writes)) : 0);
                rrdset_done(d->st_svctm);
            }
        }

        // --------------------------------------------------------------------------
        // read bcache metrics and generate the bcache charts

        if(d->device_is_bcache && d->do_bcache != CONFIG_BOOLEAN_NO) {
            unsigned long long int
                    stats_total_cache_bypass_hits = 0,
                    stats_total_cache_bypass_misses = 0,
                    stats_total_cache_hits = 0,
                    stats_total_cache_miss_collisions = 0,
                    stats_total_cache_misses = 0,
                    stats_five_minute_cache_hit_ratio = 0,
                    stats_hour_cache_hit_ratio = 0,
                    stats_day_cache_hit_ratio = 0,
                    stats_total_cache_hit_ratio = 0,
                    cache_available_percent = 0,
                    cache_readaheads = 0,
                    cache_read_races = 0,
                    cache_io_errors = 0,
                    cache_congested = 0,
                    dirty_data = 0,
                    writeback_rate = 0;

            // read the bcache values

            if(d->bcache_filename_dirty_data)
                dirty_data = bcache_read_number_with_units(d->bcache_filename_dirty_data);

            if(d->bcache_filename_writeback_rate)
                writeback_rate = bcache_read_number_with_units(d->bcache_filename_writeback_rate);

            if(d->bcache_filename_cache_congested)
                cache_congested = bcache_read_number_with_units(d->bcache_filename_cache_congested);

            if(d->bcache_filename_cache_available_percent)
                read_single_number_file(d->bcache_filename_cache_available_percent, &cache_available_percent);

            if(d->bcache_filename_stats_five_minute_cache_hit_ratio)
                read_single_number_file(d->bcache_filename_stats_five_minute_cache_hit_ratio, &stats_five_minute_cache_hit_ratio);

            if(d->bcache_filename_stats_hour_cache_hit_ratio)
                read_single_number_file(d->bcache_filename_stats_hour_cache_hit_ratio, &stats_hour_cache_hit_ratio);

            if(d->bcache_filename_stats_day_cache_hit_ratio)
                read_single_number_file(d->bcache_filename_stats_day_cache_hit_ratio, &stats_day_cache_hit_ratio);

            if(d->bcache_filename_stats_total_cache_hit_ratio)
                read_single_number_file(d->bcache_filename_stats_total_cache_hit_ratio, &stats_total_cache_hit_ratio);

            if(d->bcache_filename_stats_total_cache_hits)
                read_single_number_file(d->bcache_filename_stats_total_cache_hits, &stats_total_cache_hits);

            if(d->bcache_filename_stats_total_cache_misses)
                read_single_number_file(d->bcache_filename_stats_total_cache_misses, &stats_total_cache_misses);

            if(d->bcache_filename_stats_total_cache_miss_collisions)
                read_single_number_file(d->bcache_filename_stats_total_cache_miss_collisions, &stats_total_cache_miss_collisions);

            if(d->bcache_filename_stats_total_cache_bypass_hits)
                read_single_number_file(d->bcache_filename_stats_total_cache_bypass_hits, &stats_total_cache_bypass_hits);

            if(d->bcache_filename_stats_total_cache_bypass_misses)
                read_single_number_file(d->bcache_filename_stats_total_cache_bypass_misses, &stats_total_cache_bypass_misses);

            if(d->bcache_filename_stats_total_cache_readaheads)
                cache_readaheads = bcache_read_number_with_units(d->bcache_filename_stats_total_cache_readaheads);

            if(d->bcache_filename_cache_read_races)
                read_single_number_file(d->bcache_filename_cache_read_races, &cache_read_races);

            if(d->bcache_filename_cache_io_errors)
                read_single_number_file(d->bcache_filename_cache_io_errors, &cache_io_errors);

            if(d->bcache_filename_priority_stats && global_bcache_priority_stats_update_every >= 1)
                bcache_read_priority_stats(d, family, global_bcache_priority_stats_update_every, dt);

            // update the charts

            {
                if(unlikely(!d->st_bcache_hit_ratio)) {
                    d->st_bcache_hit_ratio = rrdset_create_localhost(
                            "disk_bcache_hit_ratio"
                            , d->device
                            , d->disk
                            , family
                            , "disk.bcache_hit_ratio"
                            , "BCache Cache Hit Ratio"
                            , "percentage"
                            , PLUGIN_PROC_NAME
                            , PLUGIN_PROC_MODULE_DISKSTATS_NAME
                            , NETDATA_CHART_PRIO_BCACHE_HIT_RATIO
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    d->rd_bcache_hit_ratio_5min  = rrddim_add(d->st_bcache_hit_ratio, "5min",  NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                    d->rd_bcache_hit_ratio_1hour = rrddim_add(d->st_bcache_hit_ratio, "1hour", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                    d->rd_bcache_hit_ratio_1day  = rrddim_add(d->st_bcache_hit_ratio, "1day",  NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                    d->rd_bcache_hit_ratio_total = rrddim_add(d->st_bcache_hit_ratio, "ever",  NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                }
                else rrdset_next(d->st_bcache_hit_ratio);

                rrddim_set_by_pointer(d->st_bcache_hit_ratio, d->rd_bcache_hit_ratio_5min, stats_five_minute_cache_hit_ratio);
                rrddim_set_by_pointer(d->st_bcache_hit_ratio, d->rd_bcache_hit_ratio_1hour, stats_hour_cache_hit_ratio);
                rrddim_set_by_pointer(d->st_bcache_hit_ratio, d->rd_bcache_hit_ratio_1day, stats_day_cache_hit_ratio);
                rrddim_set_by_pointer(d->st_bcache_hit_ratio, d->rd_bcache_hit_ratio_total, stats_total_cache_hit_ratio);
                rrdset_done(d->st_bcache_hit_ratio);
            }

            {

                if(unlikely(!d->st_bcache_rates)) {
                    d->st_bcache_rates = rrdset_create_localhost(
                            "disk_bcache_rates"
                            , d->device
                            , d->disk
                            , family
                            , "disk.bcache_rates"
                            , "BCache Rates"
                            , "KiB/s"
                            , PLUGIN_PROC_NAME
                            , PLUGIN_PROC_MODULE_DISKSTATS_NAME
                            , NETDATA_CHART_PRIO_BCACHE_RATES
                            , update_every
                            , RRDSET_TYPE_AREA
                    );

                    d->rd_bcache_rate_congested = rrddim_add(d->st_bcache_rates, "congested", NULL,  1, 1024, RRD_ALGORITHM_ABSOLUTE);
                    d->rd_bcache_rate_writeback = rrddim_add(d->st_bcache_rates, "writeback", NULL, -1, 1024, RRD_ALGORITHM_ABSOLUTE);
                }
                else rrdset_next(d->st_bcache_rates);

                rrddim_set_by_pointer(d->st_bcache_rates, d->rd_bcache_rate_writeback, writeback_rate);
                rrddim_set_by_pointer(d->st_bcache_rates, d->rd_bcache_rate_congested, cache_congested);
                rrdset_done(d->st_bcache_rates);
            }

            {
                if(unlikely(!d->st_bcache_size)) {
                    d->st_bcache_size = rrdset_create_localhost(
                            "disk_bcache_size"
                            , d->device
                            , d->disk
                            , family
                            , "disk.bcache_size"
                            , "BCache Cache Sizes"
                            , "MiB"
                            , PLUGIN_PROC_NAME
                            , PLUGIN_PROC_MODULE_DISKSTATS_NAME
                            , NETDATA_CHART_PRIO_BCACHE_SIZE
                            , update_every
                            , RRDSET_TYPE_AREA
                    );

                    d->rd_bcache_dirty_size = rrddim_add(d->st_bcache_size, "dirty", NULL,  1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
                }
                else rrdset_next(d->st_bcache_size);

                rrddim_set_by_pointer(d->st_bcache_size, d->rd_bcache_dirty_size, dirty_data);
                rrdset_done(d->st_bcache_size);
            }

            {
                if(unlikely(!d->st_bcache_usage)) {
                    d->st_bcache_usage = rrdset_create_localhost(
                            "disk_bcache_usage"
                            , d->device
                            , d->disk
                            , family
                            , "disk.bcache_usage"
                            , "BCache Cache Usage"
                            , "percentage"
                            , PLUGIN_PROC_NAME
                            , PLUGIN_PROC_MODULE_DISKSTATS_NAME
                            , NETDATA_CHART_PRIO_BCACHE_USAGE
                            , update_every
                            , RRDSET_TYPE_AREA
                    );

                    d->rd_bcache_available_percent = rrddim_add(d->st_bcache_usage, "avail", NULL,  1, 1, RRD_ALGORITHM_ABSOLUTE);
                }
                else rrdset_next(d->st_bcache_usage);

                rrddim_set_by_pointer(d->st_bcache_usage, d->rd_bcache_available_percent, cache_available_percent);
                rrdset_done(d->st_bcache_usage);
            }

            {

                if(unlikely(!d->st_bcache_cache_read_races)) {
                    d->st_bcache_cache_read_races = rrdset_create_localhost(
                            "disk_bcache_cache_read_races"
                            , d->device
                            , d->disk
                            , family
                            , "disk.bcache_cache_read_races"
                            , "BCache Cache Read Races"
                            , "operations/s"
                            , PLUGIN_PROC_NAME
                            , PLUGIN_PROC_MODULE_DISKSTATS_NAME
                            , NETDATA_CHART_PRIO_BCACHE_CACHE_READ_RACES
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    d->rd_bcache_cache_read_races = rrddim_add(d->st_bcache_cache_read_races, "races",  NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    d->rd_bcache_cache_io_errors  = rrddim_add(d->st_bcache_cache_read_races, "errors", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                }
                else rrdset_next(d->st_bcache_cache_read_races);

                rrddim_set_by_pointer(d->st_bcache_cache_read_races, d->rd_bcache_cache_read_races, cache_read_races);
                rrddim_set_by_pointer(d->st_bcache_cache_read_races, d->rd_bcache_cache_io_errors, cache_io_errors);
                rrdset_done(d->st_bcache_cache_read_races);
            }

            if(d->do_bcache == CONFIG_BOOLEAN_YES || (d->do_bcache == CONFIG_BOOLEAN_AUTO &&
                                                      (stats_total_cache_hits ||
                                                       stats_total_cache_misses ||
                                                       stats_total_cache_miss_collisions ||
                                                       netdata_zero_metrics_enabled == CONFIG_BOOLEAN_YES))) {

                if(unlikely(!d->st_bcache)) {
                    d->st_bcache = rrdset_create_localhost(
                            "disk_bcache"
                            , d->device
                            , d->disk
                            , family
                            , "disk.bcache"
                            , "BCache Cache I/O Operations"
                            , "operations/s"
                            , PLUGIN_PROC_NAME
                            , PLUGIN_PROC_MODULE_DISKSTATS_NAME
                            , NETDATA_CHART_PRIO_BCACHE_OPS
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    rrdset_flag_set(d->st_bcache, RRDSET_FLAG_DETAIL);

                    d->rd_bcache_hits            = rrddim_add(d->st_bcache, "hits",       NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    d->rd_bcache_misses          = rrddim_add(d->st_bcache, "misses",     NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    d->rd_bcache_miss_collisions = rrddim_add(d->st_bcache, "collisions", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    d->rd_bcache_readaheads      = rrddim_add(d->st_bcache, "readaheads", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                }
                else rrdset_next(d->st_bcache);

                rrddim_set_by_pointer(d->st_bcache, d->rd_bcache_hits, stats_total_cache_hits);
                rrddim_set_by_pointer(d->st_bcache, d->rd_bcache_misses, stats_total_cache_misses);
                rrddim_set_by_pointer(d->st_bcache, d->rd_bcache_miss_collisions, stats_total_cache_miss_collisions);
                rrddim_set_by_pointer(d->st_bcache, d->rd_bcache_readaheads, cache_readaheads);
                rrdset_done(d->st_bcache);
            }

            if(d->do_bcache == CONFIG_BOOLEAN_YES || (d->do_bcache == CONFIG_BOOLEAN_AUTO &&
                                                      (stats_total_cache_bypass_hits ||
                                                       stats_total_cache_bypass_misses ||
                                                       netdata_zero_metrics_enabled == CONFIG_BOOLEAN_YES))) {

                if(unlikely(!d->st_bcache_bypass)) {
                    d->st_bcache_bypass = rrdset_create_localhost(
                            "disk_bcache_bypass"
                            , d->device
                            , d->disk
                            , family
                            , "disk.bcache_bypass"
                            , "BCache Cache Bypass I/O Operations"
                            , "operations/s"
                            , PLUGIN_PROC_NAME
                            , PLUGIN_PROC_MODULE_DISKSTATS_NAME
                            , NETDATA_CHART_PRIO_BCACHE_BYPASS
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    rrdset_flag_set(d->st_bcache_bypass, RRDSET_FLAG_DETAIL);

                    d->rd_bcache_bypass_hits   = rrddim_add(d->st_bcache_bypass, "hits",   NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    d->rd_bcache_bypass_misses = rrddim_add(d->st_bcache_bypass, "misses", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                }
                else rrdset_next(d->st_bcache_bypass);

                rrddim_set_by_pointer(d->st_bcache_bypass, d->rd_bcache_bypass_hits, stats_total_cache_bypass_hits);
                rrddim_set_by_pointer(d->st_bcache_bypass, d->rd_bcache_bypass_misses, stats_total_cache_bypass_misses);
                rrdset_done(d->st_bcache_bypass);
            }
        }
    }


    // ------------------------------------------------------------------------
    // update the system total I/O

    if(global_do_io == CONFIG_BOOLEAN_YES || (global_do_io == CONFIG_BOOLEAN_AUTO &&
                                              (system_read_kb || system_write_kb ||
                                               netdata_zero_metrics_enabled == CONFIG_BOOLEAN_YES))) {
        static RRDSET *st_io = NULL;
        static RRDDIM *rd_in = NULL, *rd_out = NULL;

        if(unlikely(!st_io)) {
            st_io = rrdset_create_localhost(
                    "system"
                    , "io"
                    , NULL
                    , "disk"
                    , NULL
                    , "Disk I/O"
                    , "KiB/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_DISKSTATS_NAME
                    , NETDATA_CHART_PRIO_SYSTEM_IO
                    , update_every
                    , RRDSET_TYPE_AREA
            );

            rd_in  = rrddim_add(st_io, "in",  NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_out = rrddim_add(st_io, "out", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        else rrdset_next(st_io);

        rrddim_set_by_pointer(st_io, rd_in, system_read_kb);
        rrddim_set_by_pointer(st_io, rd_out, system_write_kb);
        rrdset_done(st_io);
    }


    // ------------------------------------------------------------------------
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
            rrdset_obsolete_and_pointer_null(d->st_bcache);
            rrdset_obsolete_and_pointer_null(d->st_bcache_bypass);
            rrdset_obsolete_and_pointer_null(d->st_bcache_rates);
            rrdset_obsolete_and_pointer_null(d->st_bcache_size);
            rrdset_obsolete_and_pointer_null(d->st_bcache_usage);
            rrdset_obsolete_and_pointer_null(d->st_bcache_hit_ratio);

            if(d == disk_root) {
                disk_root = d = d->next;
                last = NULL;
            }
            else if(last) {
                last->next = d = d->next;
            }

            freez(t->bcache_filename_dirty_data);
            freez(t->bcache_filename_writeback_rate);
            freez(t->bcache_filename_cache_congested);
            freez(t->bcache_filename_cache_available_percent);
            freez(t->bcache_filename_stats_five_minute_cache_hit_ratio);
            freez(t->bcache_filename_stats_hour_cache_hit_ratio);
            freez(t->bcache_filename_stats_day_cache_hit_ratio);
            freez(t->bcache_filename_stats_total_cache_hit_ratio);
            freez(t->bcache_filename_stats_total_cache_hits);
            freez(t->bcache_filename_stats_total_cache_misses);
            freez(t->bcache_filename_stats_total_cache_miss_collisions);
            freez(t->bcache_filename_stats_total_cache_bypass_hits);
            freez(t->bcache_filename_stats_total_cache_bypass_misses);
            freez(t->bcache_filename_stats_total_cache_readaheads);
            freez(t->bcache_filename_cache_read_races);
            freez(t->bcache_filename_cache_io_errors);
            freez(t->bcache_filename_priority_stats);

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
