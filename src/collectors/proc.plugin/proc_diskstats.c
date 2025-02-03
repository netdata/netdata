// SPDX-License-Identifier: GPL-3.0-or-later

#include "plugin_proc.h"

#define PLUGIN_PROC_MODULE_DISKSTATS_NAME "/proc/diskstats"
#define CONFIG_SECTION_PLUGIN_PROC_DISKSTATS "plugin:" PLUGIN_PROC_CONFIG_NAME ":" PLUGIN_PROC_MODULE_DISKSTATS_NAME

#define _COMMON_PLUGIN_NAME PLUGIN_PROC_CONFIG_NAME
#define _COMMON_PLUGIN_MODULE_NAME PLUGIN_PROC_MODULE_DISKSTATS_NAME
#include "../common-contexts/common-contexts.h"

#define RRDFUNCTIONS_DISKSTATS_HELP "View block device statistics"

#define DISK_TYPE_UNKNOWN   0
#define DISK_TYPE_PHYSICAL  1
#define DISK_TYPE_PARTITION 2
#define DISK_TYPE_VIRTUAL   3

#define DEFAULT_PREFERRED_IDS "*"
#define DEFAULT_EXCLUDED_DISKS "loop* ram*"

// always 512 on Linux (https://github.com/torvalds/linux/blob/daa121128a2d2ac6006159e2c47676e4fcd21eab/include/linux/blk_types.h#L25-L34)
#define SECTOR_SIZE 512

static netdata_mutex_t diskstats_dev_mutex = NETDATA_MUTEX_INITIALIZER;

static struct disk {
    char *disk;             // the name of the disk (sda, sdb, etc, after being looked up)
    char *device;           // the device of the disk (before being looked up)
    char *disk_by_id;
    char *model;
    char *serial;
//    bool rotational;
//    bool removable;
    uint32_t hash;
    unsigned long major;
    unsigned long minor;
    int type;

    bool excluded;
    bool function_ready;

    char *mount_point;

    char *chart_id;

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

    ND_DISK_IO disk_io;
    ND_DISK_OPS disk_ops;
    ND_DISK_QOPS disk_qops;
    ND_DISK_UTIL disk_util;
    ND_DISK_BUSY disk_busy;
    ND_DISK_IOTIME disk_iotime;
    ND_DISK_AWAIT disk_await;
    ND_DISK_SVCTM disk_svctm;
    ND_DISK_AVGSZ disk_avgsz;

    RRDSET *st_ext_io;
    RRDDIM *rd_io_discards;

    RRDSET *st_ext_ops;
    RRDDIM *rd_ops_discards;
    RRDDIM *rd_ops_flushes;

    RRDSET *st_backlog;
    RRDDIM *rd_backlog_backlog;

    RRDSET *st_mops;
    RRDDIM *rd_mops_reads;
    RRDDIM *rd_mops_writes;

    RRDSET *st_ext_mops;
    RRDDIM *rd_mops_discards;

    RRDSET *st_ext_iotime;
    RRDDIM *rd_iotime_discards;
    RRDDIM *rd_iotime_flushes;

    RRDSET *st_ext_await;
    RRDDIM *rd_await_discards;
    RRDDIM *rd_await_flushes;

    RRDSET *st_ext_avgsz;
    RRDDIM *rd_avgsz_discards;

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

#define rrdset_obsolete_and_pointer_null(st) do { if(st) { rrdset_is_obsolete___safe_from_collector_thread(st); (st) = NULL; } } while(st)

static const char *path_to_sys_dev_block_major_minor_string = NULL;
static const char *path_to_sys_block_device = NULL;
static const char *path_to_sys_block_device_bcache = NULL;
static const char *path_to_sys_devices_virtual_block_device = NULL;
static const char *path_to_device_mapper = NULL;
static const char *path_to_dev_disk = NULL;
static const char *path_to_sys_block = NULL;
static const char *path_to_device_label = NULL;
static const char *path_to_device_id = NULL;
static const char *path_to_veritas_volume_groups = NULL;
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
    if(read_txt_file(filename, buffer, sizeof(buffer)) == 0) {
        static int unknown_units_error = 10;

        char *end = NULL;
        NETDATA_DOUBLE value = str2ndd(buffer, &end);
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
                collector_error("bcache file '%s' provides value '%s' with unknown units '%s'", filename, buffer, end);
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
            if(unlikely(words)) collector_error("Cannot read '%s' line %zu. Expected 2 params, read %zu.", d->bcache_filename_priority_stats, l, words);
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
                    , d->chart_id
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
        major_configs[major] = (char)inicfg_get_boolean(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_DISKSTATS, buffer, 1);
    }

    return (int)major_configs[major];
}

static inline int get_disk_name_from_path(const char *path, char *result, size_t result_size, unsigned long major, unsigned long minor, char *disk, char *prefix, int depth) {
    //collector_info("DEVICE-MAPPER ('%s', %lu:%lu): examining directory '%s' (allowed depth %d).", disk, major, minor, path, depth);
    int found = 0, preferred = 0;

    char *first_result = mallocz(result_size + 1);

    DIR *dir = opendir(path);
    if (!dir) {
        if (errno == ENOENT)
            nd_log_collector(NDLP_DEBUG, "DEVICE-MAPPER ('%s', %lu:%lu): Cannot open directory '%s': no such file or directory.", disk, major, minor, path);
        else
            collector_error("DEVICE-MAPPER ('%s', %lu:%lu): Cannot open directory '%s'.", disk, major, minor, path);
        goto failed;
    }

    struct dirent *de = NULL;
    while ((de = readdir(dir))) {
        if(de->d_type == DT_DIR) {
            if((de->d_name[0] == '.' && de->d_name[1] == '\0') || (de->d_name[0] == '.' && de->d_name[1] == '.' && de->d_name[2] == '\0'))
                continue;

            if(depth <= 0) {
                collector_error("DEVICE-MAPPER ('%s', %lu:%lu): Depth limit reached for path '%s/%s'. Ignoring path.", disk, major, minor, path, de->d_name);
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
                    collector_error("DEVICE-MAPPER ('%s', %lu:%lu): Cannot read link '%s'.", disk, major, minor, filename);
                    continue;
                }

                result[len] = '\0';
                if(result[0] != '/')
                    snprintfz(filename, FILENAME_MAX, "%s/%s", path, result);
                else
                    strncpyz(filename, result, FILENAME_MAX);
            }
            else
                snprintfz(filename, FILENAME_MAX, "%s/%s", path, de->d_name);

            struct stat sb;
            if(stat(filename, &sb) == -1) {
                collector_error("DEVICE-MAPPER ('%s', %lu:%lu): Cannot stat() file '%s'.", disk, major, minor, filename);
                continue;
            }

            if((sb.st_mode & S_IFMT) != S_IFBLK) {
                //collector_info("DEVICE-MAPPER ('%s', %lu:%lu): file '%s' is not a block device.", disk, major, minor, filename);
                continue;
            }

            if(major(sb.st_rdev) != major || minor(sb.st_rdev) != minor || strcmp(basename(filename), disk)) {
                //collector_info("DEVICE-MAPPER ('%s', %lu:%lu): filename '%s' does not match %lu:%lu.", disk, major, minor, filename, (unsigned long)major(sb.st_rdev), (unsigned long)minor(sb.st_rdev));
                continue;
            }

            //collector_info("DEVICE-MAPPER ('%s', %lu:%lu): filename '%s' matches.", disk, major, minor, filename);

            snprintfz(result, result_size - 1, "%s%s%s", (prefix)?prefix:"", (prefix)?"_":"", de->d_name);

            if(!found) {
                strncpyz(first_result, result, result_size - 1);
                found = 1;
            }

            result[result_size - 1] = '\0';
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
        strncpyz(result, first_result, result_size - 1);

    freez(first_result);

    return found;
}

static inline char *get_disk_name(unsigned long major, unsigned long minor, char *disk) {
    char result[FILENAME_MAX + 2] = "";

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

static inline bool ends_with(const char *str, const char *suffix) {
    if (!str || !suffix)
        return false;

    size_t len_str = strlen(str);
    size_t len_suffix = strlen(suffix);
    if (len_suffix > len_str)
        return false;

    return strncmp(str + len_str - len_suffix, suffix, len_suffix) == 0;
}

static inline char *get_disk_by_id(char *device) {
    char pathname[256 + 1];
    snprintfz(pathname, sizeof(pathname) - 1, "%s/by-id", path_to_dev_disk);

    struct dirent *entry;
    DIR *dp = opendir(pathname);
    if (dp == NULL) {
        internal_error(true, "Cannot open '%s'", pathname);
        return NULL;
    }

    while ((entry = readdir(dp))) {
        // We ignore the '.' and '..' entries
        if (strcmp(entry->d_name, ".") == 0 || strcmp(entry->d_name, "..") == 0)
            continue;

        if(strncmp(entry->d_name, "md-uuid-", 8) == 0 ||
                strncmp(entry->d_name, "dm-uuid-", 8) == 0 ||
                strncmp(entry->d_name, "nvme-eui.", 9) == 0 ||
                strncmp(entry->d_name, "wwn-", 4) == 0 ||
                strncmp(entry->d_name, "lvm-pv-uuid-", 12) == 0)
            continue;

        char link_target[256 + 1];
        char full_path[256 + 1];
        snprintfz(full_path, 256, "%s/%s", pathname, entry->d_name);

        ssize_t len = readlink(full_path, link_target, 256);
        if (len == -1)
            continue;

        link_target[len] = '\0';

        if (ends_with(link_target, device)) {
            char *s = strdupz(entry->d_name);
            closedir(dp);
            return s;
        }
    }

    closedir(dp);
    return NULL;
}

static inline char *get_disk_model(char *device) {
    char path[256 + 1];
    char buffer[256 + 1];

    snprintfz(path, sizeof(path) - 1, "%s/%s/device/model", path_to_sys_block, device);
    if(read_txt_file(path, buffer, sizeof(buffer)) != 0) {
        snprintfz(path, sizeof(path) - 1, "%s/%s/device/name", path_to_sys_block, device);
        if(read_txt_file(path, buffer, sizeof(buffer)) != 0)
            return NULL;
    }

    char *clean = trim(buffer);
    if (!clean)
        return NULL;

    return strdupz(clean);
}

static inline char *get_disk_serial(char *device) {
    char path[256 + 1];
    char buffer[256 + 1];

    snprintfz(path, sizeof(path) - 1, "%s/%s/device/serial", path_to_sys_block, device);
    if(read_txt_file(path, buffer, sizeof(buffer)) != 0)
        return NULL;

    return strdupz(buffer);
}

//static inline bool get_disk_rotational(char *device) {
//    char path[256 + 1];
//    char buffer[256 + 1];
//
//    snprintfz(path, 256, "%s/%s/queue/rotational", path_to_sys_block, device);
//    if(read_file(path, buffer, 256) != 0)
//        return false;
//
//    return buffer[0] == '1';
//}
//
//static inline bool get_disk_removable(char *device) {
//    char path[256 + 1];
//    char buffer[256 + 1];
//
//    snprintfz(path, 256, "%s/%s/removable", path_to_sys_block, device);
//    if(read_file(path, buffer, 256) != 0)
//        return false;
//
//    return buffer[0] == '1';
//}

static void get_disk_config(struct disk *d) {
    int def_enable = global_enable_new_disks_detected_at_runtime;

    if(def_enable != CONFIG_BOOLEAN_NO && (simple_pattern_matches(excluded_disks, d->device) || simple_pattern_matches(excluded_disks, d->disk))) {
        d->excluded = true;
        def_enable = CONFIG_BOOLEAN_NO;
    }

    char var_name[4096 + 1];
    snprintfz(var_name, 4096, CONFIG_SECTION_PLUGIN_PROC_DISKSTATS ":%s", d->disk);

    if (inicfg_exists(&netdata_config, var_name, "enable"))
        def_enable = inicfg_get_boolean_ondemand(&netdata_config, var_name, "enable", def_enable);

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
        if (inicfg_exists(&netdata_config, var_name, "enable performance metrics"))
            def_performance = inicfg_get_boolean_ondemand(&netdata_config, var_name, "enable performance metrics", def_performance);

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
        } else {
            d->excluded = true;
        }

        d->do_io = ddo_io;
        d->do_ops = ddo_ops;
        d->do_mops = ddo_mops;
        d->do_iotime = ddo_iotime;
        d->do_qops = ddo_qops;
        d->do_util = ddo_util;
        d->do_ext = ddo_ext;
        d->do_backlog = ddo_backlog;

        if (inicfg_exists(&netdata_config, var_name, "bandwidth"))
            d->do_io = inicfg_get_boolean_ondemand(&netdata_config, var_name, "bandwidth", ddo_io);
        if (inicfg_exists(&netdata_config, var_name, "operations"))
            d->do_ops = inicfg_get_boolean_ondemand(&netdata_config, var_name, "operations", ddo_ops);
        if (inicfg_exists(&netdata_config, var_name, "merged operations"))
            d->do_mops = inicfg_get_boolean_ondemand(&netdata_config, var_name, "merged operations", ddo_mops);
        if (inicfg_exists(&netdata_config, var_name, "i/o time"))
            d->do_iotime = inicfg_get_boolean_ondemand(&netdata_config, var_name, "i/o time", ddo_iotime);
        if (inicfg_exists(&netdata_config, var_name, "queued operations"))
            d->do_qops = inicfg_get_boolean_ondemand(&netdata_config, var_name, "queued operations", ddo_qops);
        if (inicfg_exists(&netdata_config, var_name, "utilization percentage"))
            d->do_util = inicfg_get_boolean_ondemand(&netdata_config, var_name, "utilization percentage", ddo_util);
        if (inicfg_exists(&netdata_config, var_name, "extended operations"))
            d->do_ext = inicfg_get_boolean_ondemand(&netdata_config, var_name, "extended operations", ddo_ext);
        if (inicfg_exists(&netdata_config, var_name, "backlog"))
            d->do_backlog = inicfg_get_boolean_ondemand(&netdata_config, var_name, "backlog", ddo_backlog);

        d->do_bcache = ddo_bcache;

        if (d->device_is_bcache) {
            if (inicfg_exists(&netdata_config, var_name, "bcache"))
                d->do_bcache = inicfg_get_boolean_ondemand(&netdata_config, var_name, "bcache", ddo_bcache);
        } else {
            d->do_bcache = 0;
        }
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

    d->excluded = false;
    d->function_ready = false;
    d->disk = get_disk_name(major, minor, disk);
    d->device = strdupz(disk);
    d->disk_by_id = get_disk_by_id(disk);
    d->model = get_disk_model(disk);
    d->serial = get_disk_serial(disk);
//    d->rotational = get_disk_rotational(disk);
//    d->removable = get_disk_removable(disk);
    d->hash = simple_hash(d->device);
    d->major = major;
    d->minor = minor;
    d->type = DISK_TYPE_UNKNOWN; // Default type. Changed later if not correct.
    d->next = NULL;

    // append it to the list
    if(unlikely(!disk_root))
        disk_root = d;
    else {
        struct disk *last;
        for(last = disk_root; last->next ;last = last->next);
        last->next = d;
    }

    d->chart_id = strdupz(d->device);

    // read device uuid if it is an LVM volume
    if (!strncmp(d->device, "dm-", 3)) {
        char uuid_filename[FILENAME_MAX + 1];
        int size = snprintfz(uuid_filename, FILENAME_MAX, path_to_sys_devices_virtual_block_device, disk);
        strncat(uuid_filename, "/dm/uuid", FILENAME_MAX - size);

        char device_uuid[RRD_ID_LENGTH_MAX + 1];
        if (!read_txt_file(uuid_filename, device_uuid, sizeof(device_uuid)) && !strncmp(device_uuid, "LVM-", 4)) {
            trim(device_uuid);

            char chart_id[RRD_ID_LENGTH_MAX + 1];
            snprintf(chart_id, RRD_ID_LENGTH_MAX, "%s-%s", d->device, device_uuid + 4);

            freez(d->chart_id);
            d->chart_id = strdupz(chart_id);
        }
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
                    collector_error("Unable to close dir %s", buffer);
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
            collector_error("bcache file '%s' cannot be read.", buffer2);

        snprintfz(buffer2, FILENAME_MAX, "%s/readahead", buffer);
        if(access(buffer2, R_OK) == 0)
            d->bcache_filename_stats_total_cache_readaheads = strdupz(buffer2);
        else
            collector_error("bcache file '%s' cannot be read.", buffer2);

        snprintfz(buffer2, FILENAME_MAX, "%s/cache/cache0/priority_stats", buffer); // only one cache is supported by bcache
        if(access(buffer2, R_OK) == 0)
            d->bcache_filename_priority_stats = strdupz(buffer2);
        else
            collector_error("bcache file '%s' cannot be read.", buffer2);

        snprintfz(buffer2, FILENAME_MAX, "%s/cache/internal/cache_read_races", buffer);
        if(access(buffer2, R_OK) == 0)
            d->bcache_filename_cache_read_races = strdupz(buffer2);
        else
            collector_error("bcache file '%s' cannot be read.", buffer2);

        snprintfz(buffer2, FILENAME_MAX, "%s/cache/cache0/io_errors", buffer);
        if(access(buffer2, R_OK) == 0)
            d->bcache_filename_cache_io_errors = strdupz(buffer2);
        else
            collector_error("bcache file '%s' cannot be read.", buffer2);

        snprintfz(buffer2, FILENAME_MAX, "%s/dirty_data", buffer);
        if(access(buffer2, R_OK) == 0)
            d->bcache_filename_dirty_data = strdupz(buffer2);
        else
            collector_error("bcache file '%s' cannot be read.", buffer2);

        snprintfz(buffer2, FILENAME_MAX, "%s/writeback_rate", buffer);
        if(access(buffer2, R_OK) == 0)
            d->bcache_filename_writeback_rate = strdupz(buffer2);
        else
            collector_error("bcache file '%s' cannot be read.", buffer2);

        snprintfz(buffer2, FILENAME_MAX, "%s/cache/cache_available_percent", buffer);
        if(access(buffer2, R_OK) == 0)
            d->bcache_filename_cache_available_percent = strdupz(buffer2);
        else
            collector_error("bcache file '%s' cannot be read.", buffer2);

        snprintfz(buffer2, FILENAME_MAX, "%s/stats_total/cache_hits", buffer);
        if(access(buffer2, R_OK) == 0)
            d->bcache_filename_stats_total_cache_hits = strdupz(buffer2);
        else
            collector_error("bcache file '%s' cannot be read.", buffer2);

        snprintfz(buffer2, FILENAME_MAX, "%s/stats_five_minute/cache_hit_ratio", buffer);
        if(access(buffer2, R_OK) == 0)
            d->bcache_filename_stats_five_minute_cache_hit_ratio = strdupz(buffer2);
        else
            collector_error("bcache file '%s' cannot be read.", buffer2);

        snprintfz(buffer2, FILENAME_MAX, "%s/stats_hour/cache_hit_ratio", buffer);
        if(access(buffer2, R_OK) == 0)
            d->bcache_filename_stats_hour_cache_hit_ratio = strdupz(buffer2);
        else
            collector_error("bcache file '%s' cannot be read.", buffer2);

        snprintfz(buffer2, FILENAME_MAX, "%s/stats_day/cache_hit_ratio", buffer);
        if(access(buffer2, R_OK) == 0)
            d->bcache_filename_stats_day_cache_hit_ratio = strdupz(buffer2);
        else
            collector_error("bcache file '%s' cannot be read.", buffer2);

        snprintfz(buffer2, FILENAME_MAX, "%s/stats_total/cache_hit_ratio", buffer);
        if(access(buffer2, R_OK) == 0)
            d->bcache_filename_stats_total_cache_hit_ratio = strdupz(buffer2);
        else
            collector_error("bcache file '%s' cannot be read.", buffer2);

        snprintfz(buffer2, FILENAME_MAX, "%s/stats_total/cache_misses", buffer);
        if(access(buffer2, R_OK) == 0)
            d->bcache_filename_stats_total_cache_misses = strdupz(buffer2);
        else
            collector_error("bcache file '%s' cannot be read.", buffer2);

        snprintfz(buffer2, FILENAME_MAX, "%s/stats_total/cache_bypass_hits", buffer);
        if(access(buffer2, R_OK) == 0)
            d->bcache_filename_stats_total_cache_bypass_hits = strdupz(buffer2);
        else
            collector_error("bcache file '%s' cannot be read.", buffer2);

        snprintfz(buffer2, FILENAME_MAX, "%s/stats_total/cache_bypass_misses", buffer);
        if(access(buffer2, R_OK) == 0)
            d->bcache_filename_stats_total_cache_bypass_misses = strdupz(buffer2);
        else
            collector_error("bcache file '%s' cannot be read.", buffer2);

        snprintfz(buffer2, FILENAME_MAX, "%s/stats_total/cache_miss_collisions", buffer);
        if(access(buffer2, R_OK) == 0)
            d->bcache_filename_stats_total_cache_miss_collisions = strdupz(buffer2);
        else
            collector_error("bcache file '%s' cannot be read.", buffer2);
    }

    get_disk_config(d);

    return d;
}

static const char *get_disk_type_string(int disk_type) {
    switch (disk_type) {
        case DISK_TYPE_PHYSICAL:
            return "physical";
        case DISK_TYPE_PARTITION:
            return "partition";
        case DISK_TYPE_VIRTUAL:
            return "virtual";
        default:
            return "unknown";
    }
}

static void add_labels_to_disk(struct disk *d, RRDSET *st) {
    rrdlabels_add(st->rrdlabels, "device", d->disk, RRDLABEL_SRC_AUTO);
    rrdlabels_add(st->rrdlabels, "mount_point", d->mount_point, RRDLABEL_SRC_AUTO);
    rrdlabels_add(st->rrdlabels, "id", d->disk_by_id, RRDLABEL_SRC_AUTO);
    rrdlabels_add(st->rrdlabels, "model", d->model, RRDLABEL_SRC_AUTO);
    rrdlabels_add(st->rrdlabels, "serial", d->serial, RRDLABEL_SRC_AUTO);
    rrdlabels_add(st->rrdlabels, "device_type", get_disk_type_string(d->type), RRDLABEL_SRC_AUTO);
}

static void disk_labels_cb(RRDSET *st, void *data) {
    add_labels_to_disk(data, st);
}

static int diskstats_function_block_devices(BUFFER *wb, const char *function __maybe_unused, BUFFER *payload __maybe_unused, const char *source __maybe_unused) {
    buffer_flush(wb);
    wb->content_type = CT_APPLICATION_JSON;
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_DEFAULT);

    buffer_json_member_add_string(wb, "hostname", rrdhost_hostname(localhost));
    buffer_json_member_add_uint64(wb, "status", HTTP_RESP_OK);
    buffer_json_member_add_string(wb, "type", "table");
    buffer_json_member_add_time_t(wb, "update_every", 1);
    buffer_json_member_add_boolean(wb, "has_history", false);
    buffer_json_member_add_string(wb, "help", RRDFUNCTIONS_DISKSTATS_HELP);
    buffer_json_member_add_array(wb, "data");

    double max_io_reads = 0.0;
    double max_io_writes = 0.0;
    double max_io = 0.0;
    double max_backlog_time = 0.0;
    double max_busy_time = 0.0;
    double max_busy_perc = 0.0;
    double max_iops_reads = 0.0;
    double max_iops_writes = 0.0;
    double max_iops_time_reads = 0.0;
    double max_iops_time_writes = 0.0;
    double max_iops_avg_time_read = 0.0;
    double max_iops_avg_time_write = 0.0;
    double max_iops_avg_size_read = 0.0;
    double max_iops_avg_size_write = 0.0;

    netdata_mutex_lock(&diskstats_dev_mutex);

    for (struct disk *d = disk_root; d; d = d->next) {
        if (unlikely(!d->function_ready))
            continue;

        buffer_json_add_array_item_array(wb);

        buffer_json_add_array_item_string(wb, d->device);
        buffer_json_add_array_item_string(wb, get_disk_type_string(d->type));
        buffer_json_add_array_item_string(wb, d->disk_by_id);
        buffer_json_add_array_item_string(wb, d->model);
        buffer_json_add_array_item_string(wb, d->serial);

        // IO
        double io_reads = rrddim_get_last_stored_value(d->disk_io.rd_io_reads, &max_io_reads, 1024.0);
        double io_writes = rrddim_get_last_stored_value(d->disk_io.rd_io_writes, &max_io_writes, 1024.0);
        double io_total = NAN;
        if (!isnan(io_reads) && !isnan(io_writes)) {
            io_total = io_reads + io_writes;
            max_io = MAX(max_io, io_total);
        }
        // Backlog and Busy Time
        double busy_perc = rrddim_get_last_stored_value(d->disk_util.rd_util, &max_busy_perc, 1);
        double busy_time = rrddim_get_last_stored_value(d->disk_busy.rd_busy, &max_busy_time, 1);
        double backlog_time = rrddim_get_last_stored_value(d->rd_backlog_backlog, &max_backlog_time, 1);
        // IOPS
        double iops_reads = rrddim_get_last_stored_value(d->disk_ops.rd_ops_reads, &max_iops_reads, 1);
        double iops_writes = rrddim_get_last_stored_value(d->disk_ops.rd_ops_writes, &max_iops_writes, 1);
        // IO Time
        double iops_time_reads = rrddim_get_last_stored_value(d->disk_iotime.rd_reads_ms, &max_iops_time_reads, 1);
        double iops_time_writes = rrddim_get_last_stored_value(d->disk_iotime.rd_writes_ms, &max_iops_time_writes, 1);
        // Avg IO Time
        double iops_avg_time_read = rrddim_get_last_stored_value(d->disk_await.rd_await_reads, &max_iops_avg_time_read, 1);
        double iops_avg_time_write = rrddim_get_last_stored_value(d->disk_await.rd_await_writes, &max_iops_avg_time_write, 1);
        // Avg IO Size
        double iops_avg_size_read = rrddim_get_last_stored_value(d->disk_avgsz.rd_avgsz_reads, &max_iops_avg_size_read, 1);
        double iops_avg_size_write = rrddim_get_last_stored_value(d->disk_avgsz.rd_avgsz_writes, &max_iops_avg_size_write, 1);


        buffer_json_add_array_item_double(wb, io_reads);
        buffer_json_add_array_item_double(wb, io_writes);
        buffer_json_add_array_item_double(wb, io_total);
        buffer_json_add_array_item_double(wb, busy_perc);
        buffer_json_add_array_item_double(wb, busy_time);
        buffer_json_add_array_item_double(wb, backlog_time);
        buffer_json_add_array_item_double(wb, iops_reads);
        buffer_json_add_array_item_double(wb, iops_writes);
        buffer_json_add_array_item_double(wb, iops_time_reads);
        buffer_json_add_array_item_double(wb, iops_time_writes);
        buffer_json_add_array_item_double(wb, iops_avg_time_read);
        buffer_json_add_array_item_double(wb, iops_avg_time_write);
        buffer_json_add_array_item_double(wb, iops_avg_size_read);
        buffer_json_add_array_item_double(wb, iops_avg_size_write);

        // End
        buffer_json_array_close(wb);
    }

    netdata_mutex_unlock(&diskstats_dev_mutex);

    buffer_json_array_close(wb); // data
    buffer_json_member_add_object(wb, "columns");
    {
        size_t field_id = 0;

        buffer_rrdf_table_add_field(wb, field_id++, "Device", "Device Name",
                RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_UNIQUE_KEY | RRDF_FIELD_OPTS_STICKY,
                NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "Type", "Device Type",
                RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                RRDF_FIELD_OPTS_UNIQUE_KEY,
                NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "ID", "Device ID",
                RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                RRDF_FIELD_OPTS_UNIQUE_KEY,
                NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "Model", "Device Model",
                RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                RRDF_FIELD_OPTS_UNIQUE_KEY,
                NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "Serial", "Device Serial Number",
                RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                RRDF_FIELD_OPTS_UNIQUE_KEY,
                NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "Read", "Data Read from Device",
                RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER,
                2, "MiB", max_io_reads, RRDF_FIELD_SORT_DESCENDING, NULL,
                RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_VISIBLE,
                NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "Written", "Data Writen to Device",
                RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER,
                2, "MiB", max_io_writes, RRDF_FIELD_SORT_DESCENDING, NULL,
                RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_VISIBLE,
                NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "Total", "Data Transferred to and from Device",
                RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER,
                2, "MiB", max_io, RRDF_FIELD_SORT_DESCENDING, NULL,
                RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_NONE,
                NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "Busy%", "Disk Busy Percentage",
                RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER,
                2, "%", max_busy_perc, RRDF_FIELD_SORT_DESCENDING, NULL,
                RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_VISIBLE,
                NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "Busy", "Disk Busy Time",
                RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER,
                2, "milliseconds", max_busy_time, RRDF_FIELD_SORT_DESCENDING, NULL,
                RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_VISIBLE,
                NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "Backlog", "Disk Backlog",
                RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER,
                2, "milliseconds", max_backlog_time, RRDF_FIELD_SORT_DESCENDING, NULL,
                RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_VISIBLE,
                NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "Reads", "Completed Read Operations",
                RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER,
                2, "ops", max_iops_reads, RRDF_FIELD_SORT_DESCENDING, NULL,
                RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_VISIBLE,
                NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "Writes", "Completed Write Operations",
                RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER,
                2, "ops", max_iops_writes, RRDF_FIELD_SORT_DESCENDING, NULL,
                RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_VISIBLE,
                NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "ReadsTime", "Read Operations Time",
                RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER,
                2, "milliseconds", max_iops_time_reads, RRDF_FIELD_SORT_DESCENDING, NULL,
                RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_VISIBLE,
                NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "WritesTime", "Write Operations Time",
                RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER,
                2, "milliseconds", max_iops_time_writes, RRDF_FIELD_SORT_DESCENDING, NULL,
                RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_VISIBLE,
                NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "ReadAvgTime", "Average Read Operation Service Time",
                RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER,
                2, "milliseconds", max_iops_avg_time_read, RRDF_FIELD_SORT_DESCENDING, NULL,
                RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_VISIBLE,
                NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "WriteAvgTime", "Average Write Operation Service Time",
                RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER,
                2, "milliseconds", max_iops_avg_time_write, RRDF_FIELD_SORT_DESCENDING, NULL,
                RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_VISIBLE,
                NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "ReadAvgSz", "Average Read Operation Size",
                RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER,
                2, "KiB", max_iops_avg_size_read, RRDF_FIELD_SORT_DESCENDING, NULL,
                RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_VISIBLE,
                NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "WriteAvgSz", "Average Write Operation Size",
                RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER,
                2, "KiB", max_iops_avg_size_write, RRDF_FIELD_SORT_DESCENDING, NULL,
                RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_VISIBLE,
                NULL);
    }

    buffer_json_object_close(wb); // columns
    buffer_json_member_add_string(wb, "default_sort_column", "Total");

    buffer_json_member_add_object(wb, "charts");
    {
        buffer_json_member_add_object(wb, "IO");
        {
            buffer_json_member_add_string(wb, "name", "IO");
            buffer_json_member_add_string(wb, "type", "stacked-bar");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "Read");
                buffer_json_add_array_item_string(wb, "Written");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "Busy");
        {
            buffer_json_member_add_string(wb, "name", "Busy");
            buffer_json_member_add_string(wb, "type", "stacked-bar");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "Busy");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);
    }
    buffer_json_object_close(wb); // charts

    buffer_json_member_add_array(wb, "default_charts");
    {
        buffer_json_add_array_item_array(wb);
        buffer_json_add_array_item_string(wb, "IO");
        buffer_json_add_array_item_string(wb, "Device");
        buffer_json_array_close(wb);

        buffer_json_add_array_item_array(wb);
        buffer_json_add_array_item_string(wb, "Busy");
        buffer_json_add_array_item_string(wb, "Device");
        buffer_json_array_close(wb);
    }
    buffer_json_array_close(wb);

    buffer_json_member_add_object(wb, "group_by");
    {
        buffer_json_member_add_object(wb, "Type");
        {
            buffer_json_member_add_string(wb, "name", "Type");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "Type");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);
    }
    buffer_json_object_close(wb); // group_by

    buffer_json_member_add_time_t(wb, "expires", now_realtime_sec() + 1);
    buffer_json_finalize(wb);

    return HTTP_RESP_OK;
}

static void diskstats_cleanup_disks() {
    struct disk *d = disk_root, *last = NULL;
    while (d) {
        if (unlikely(global_cleanup_removed_disks && !d->updated)) {
            struct disk *t = d;

            rrdset_obsolete_and_pointer_null(d->disk_io.st_io);
            rrdset_obsolete_and_pointer_null(d->disk_ops.st_ops);
            rrdset_obsolete_and_pointer_null(d->disk_qops.st_qops);
            rrdset_obsolete_and_pointer_null(d->disk_util.st_util);
            rrdset_obsolete_and_pointer_null(d->disk_busy.st_busy);
            rrdset_obsolete_and_pointer_null(d->disk_iotime.st_iotime);
            rrdset_obsolete_and_pointer_null(d->disk_await.st_await);
            rrdset_obsolete_and_pointer_null(d->disk_svctm.st_svctm);

            rrdset_obsolete_and_pointer_null(d->disk_avgsz.st_avgsz);
            rrdset_obsolete_and_pointer_null(d->st_ext_avgsz);
            rrdset_obsolete_and_pointer_null(d->st_ext_await);
            rrdset_obsolete_and_pointer_null(d->st_backlog);
            rrdset_obsolete_and_pointer_null(d->disk_io.st_io);
            rrdset_obsolete_and_pointer_null(d->st_ext_io);
            rrdset_obsolete_and_pointer_null(d->st_ext_iotime);
            rrdset_obsolete_and_pointer_null(d->st_mops);
            rrdset_obsolete_and_pointer_null(d->st_ext_mops);
            rrdset_obsolete_and_pointer_null(d->st_ext_ops);
            rrdset_obsolete_and_pointer_null(d->st_bcache);
            rrdset_obsolete_and_pointer_null(d->st_bcache_bypass);
            rrdset_obsolete_and_pointer_null(d->st_bcache_rates);
            rrdset_obsolete_and_pointer_null(d->st_bcache_size);
            rrdset_obsolete_and_pointer_null(d->st_bcache_usage);
            rrdset_obsolete_and_pointer_null(d->st_bcache_hit_ratio);
            rrdset_obsolete_and_pointer_null(d->st_bcache_cache_allocations);
            rrdset_obsolete_and_pointer_null(d->st_bcache_cache_read_races);

            if (d == disk_root) {
                disk_root = d = d->next;
                last = NULL;
            } else if (last) {
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
            freez(t->disk_by_id);
            freez(t->model);
            freez(t->serial);
            freez(t->mount_point);
            freez(t->chart_id);
            freez(t);
        } else {
            d->updated = 0;
            last = d;
            d = d->next;
        }
    }
}

int do_proc_diskstats(int update_every, usec_t dt) {
    static procfile *ff = NULL;

    if(unlikely(!globals_initialized)) {
        globals_initialized = 1;

        global_enable_new_disks_detected_at_runtime = inicfg_get_boolean(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_DISKSTATS, "enable new disks detected at runtime", global_enable_new_disks_detected_at_runtime);
        global_enable_performance_for_physical_disks = inicfg_get_boolean_ondemand(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_DISKSTATS, "performance metrics for physical disks", global_enable_performance_for_physical_disks);
        global_enable_performance_for_virtual_disks = inicfg_get_boolean_ondemand(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_DISKSTATS, "performance metrics for virtual disks", global_enable_performance_for_virtual_disks);
        global_enable_performance_for_partitions = inicfg_get_boolean_ondemand(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_DISKSTATS, "performance metrics for partitions", global_enable_performance_for_partitions);

        global_do_io      = inicfg_get_boolean_ondemand(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_DISKSTATS, "bandwidth for all disks", global_do_io);
        global_do_ops     = inicfg_get_boolean_ondemand(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_DISKSTATS, "operations for all disks", global_do_ops);
        global_do_mops    = inicfg_get_boolean_ondemand(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_DISKSTATS, "merged operations for all disks", global_do_mops);
        global_do_iotime  = inicfg_get_boolean_ondemand(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_DISKSTATS, "i/o time for all disks", global_do_iotime);
        global_do_qops    = inicfg_get_boolean_ondemand(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_DISKSTATS, "queued operations for all disks", global_do_qops);
        global_do_util    = inicfg_get_boolean_ondemand(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_DISKSTATS, "utilization percentage for all disks", global_do_util);
        global_do_ext     = inicfg_get_boolean_ondemand(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_DISKSTATS, "extended operations for all disks", global_do_ext);
        global_do_backlog = inicfg_get_boolean_ondemand(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_DISKSTATS, "backlog for all disks", global_do_backlog);
        global_do_bcache  = inicfg_get_boolean_ondemand(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_DISKSTATS, "bcache for all disks", global_do_bcache);
        global_bcache_priority_stats_update_every = (int)inicfg_get_duration_seconds(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_DISKSTATS, "bcache priority stats update every", global_bcache_priority_stats_update_every);

        global_cleanup_removed_disks = inicfg_get_boolean(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_DISKSTATS, "remove charts of removed disks" , global_cleanup_removed_disks);

        char buffer[FILENAME_MAX + 1];

        snprintfz(buffer, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/sys/block/%s");
        path_to_sys_block_device = inicfg_get(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_DISKSTATS, "path to get block device", buffer);

        snprintfz(buffer, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/sys/block/%s/bcache");
        path_to_sys_block_device_bcache = inicfg_get(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_DISKSTATS, "path to get block device bcache", buffer);

        snprintfz(buffer, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/sys/devices/virtual/block/%s");
        path_to_sys_devices_virtual_block_device = inicfg_get(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_DISKSTATS, "path to get virtual block device", buffer);

        snprintfz(buffer, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/sys/dev/block/%lu:%lu/%s");
        path_to_sys_dev_block_major_minor_string = inicfg_get(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_DISKSTATS, "path to get block device infos", buffer);

        snprintfz(buffer, FILENAME_MAX, "%s/dev/mapper", netdata_configured_host_prefix);
        path_to_device_mapper = inicfg_get(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_DISKSTATS, "path to device mapper", buffer);

        snprintfz(buffer, FILENAME_MAX, "%s/dev/disk", netdata_configured_host_prefix);
        path_to_dev_disk = inicfg_get(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_DISKSTATS, "path to /dev/disk", buffer);

        snprintfz(buffer, FILENAME_MAX, "%s/sys/block", netdata_configured_host_prefix);
        path_to_sys_block = inicfg_get(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_DISKSTATS, "path to /sys/block", buffer);

        snprintfz(buffer, FILENAME_MAX, "%s/dev/disk/by-label", netdata_configured_host_prefix);
        path_to_device_label = inicfg_get(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_DISKSTATS, "path to /dev/disk/by-label", buffer);

        snprintfz(buffer, FILENAME_MAX, "%s/dev/disk/by-id", netdata_configured_host_prefix);
        path_to_device_id = inicfg_get(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_DISKSTATS, "path to /dev/disk/by-id", buffer);

        snprintfz(buffer, FILENAME_MAX, "%s/dev/vx/dsk", netdata_configured_host_prefix);
        path_to_veritas_volume_groups = inicfg_get(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_DISKSTATS, "path to /dev/vx/dsk", buffer);

        name_disks_by_id = inicfg_get_boolean(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_DISKSTATS, "name disks by id", name_disks_by_id);

        preferred_ids = simple_pattern_create(
                inicfg_get(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_DISKSTATS, "preferred disk ids", DEFAULT_PREFERRED_IDS), NULL,
                SIMPLE_PATTERN_EXACT, true);

        excluded_disks = simple_pattern_create(
                inicfg_get(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_DISKSTATS, "exclude disks", DEFAULT_EXCLUDED_DISKS), NULL,
                SIMPLE_PATTERN_EXACT, true);

        rrd_function_add_inline(localhost, NULL, "block-devices", 10,
                                RRDFUNCTIONS_PRIORITY_DEFAULT, RRDFUNCTIONS_VERSION_DEFAULT,
                                RRDFUNCTIONS_DISKSTATS_HELP,
                                "top", HTTP_ACCESS_ANONYMOUS_DATA,
                                diskstats_function_block_devices);
    }

    // --------------------------------------------------------------------------

    if(unlikely(!ff)) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/proc/diskstats");
        ff = procfile_open(inicfg_get(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_DISKSTATS, "filename to monitor", filename), " \t", PROCFILE_FLAG_DEFAULT);
    }
    if(unlikely(!ff)) return 0;

    ff = procfile_readall(ff);
    if(unlikely(!ff)) return 0; // we return 0, so that we will retry to open it next time

    size_t lines = procfile_lines(ff), l;

    collected_number system_read_kb = 0, system_write_kb = 0;

    int do_dc_stats = 0, do_fl_stats = 0;

    netdata_mutex_lock(&diskstats_dev_mutex);

    for(l = 0; l < lines ;l++) {
        // --------------------------------------------------------------------------
        // Read parameters

        char *disk;
        unsigned long       major = 0, minor = 0;

        collected_number rd_ios = 0,  mreads = 0,  readsectors = 0,  readms = 0, wr_ios = 0, mwrites = 0, writesectors = 0, writems = 0,
                            queued_ios = 0, busy_ms = 0, backlog_ms = 0,
                            discards = 0, mdiscards = 0, discardsectors = 0, discardms = 0,
                            flushes = 0, flushms = 0;


        collected_number last_rd_ios = 0,  last_readsectors = 0,  last_readms = 0,
                         last_wr_ios = 0, last_writesectors = 0, last_writems = 0,
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
        rd_ios = str2ull(procfile_lineword(ff, l, 3), NULL);  // rd_ios
        wr_ios = str2ull(procfile_lineword(ff, l, 7), NULL);  // wr_ios

        // # of reads merged # of writes merged
        // Reads and writes which are adjacent to each other may be merged for
        // efficiency.  Thus two 4K reads may become one 8K read before it is
        // ultimately handed to the disk, and so it will be counted (and queued)
        mreads          = str2ull(procfile_lineword(ff, l, 4), NULL);  // rd_merges_or_rd_sec
        mwrites         = str2ull(procfile_lineword(ff, l, 8), NULL);  // wr_merges

        // # of sectors read # of sectors written
        // This is the total number of sectors read or written successfully.
        readsectors     = str2ull(procfile_lineword(ff, l, 5), NULL);  // rd_sec_or_wr_ios
        writesectors    = str2ull(procfile_lineword(ff, l, 9), NULL);  // wr_sec

        // # of milliseconds spent reading # of milliseconds spent writing
        // This is the total number of milliseconds spent by all reads or writes (as
        // measured from __make_request() to end_that_request_last()).
        readms          = str2ull(procfile_lineword(ff, l, 6), NULL);  // rd_ticks_or_wr_sec
        writems         = str2ull(procfile_lineword(ff, l, 10), NULL); // wr_ticks

        // # of I/Os currently in progress
        // The only field that should go to zero. Incremented as requests are
        // given to appropriate struct request_queue and decremented as they finish.
        queued_ios      = str2ull(procfile_lineword(ff, l, 11), NULL); // ios_pgr

        // # of milliseconds spent doing I/Os
        // This field increases so long as field queued_ios is nonzero.
        busy_ms         = str2ull(procfile_lineword(ff, l, 12), NULL); // tot_ticks

        // weighted # of milliseconds spent doing I/Os
        // This field is incremented at each I/O start, I/O completion, I/O
        // merge, or read of these stats by the number of I/Os in progress
        // (field queued_ios) times the number of milliseconds spent doing I/O since the
        // last update of this field.  This can provide an easy measure of both
        // I/O completion time and the backlog that may be accumulating.
        backlog_ms      = str2ull(procfile_lineword(ff, l, 13), NULL); // rq_ticks

        if (unlikely(words > 13)) {
            do_dc_stats = 1;

            // # of discards completed
            // This is the total number of discards completed successfully.
            discards       = str2ull(procfile_lineword(ff, l, 14), NULL); // dc_ios

            // # of discards merged
            // See the description of mreads/mwrites
            mdiscards      = str2ull(procfile_lineword(ff, l, 15), NULL); // dc_merges

            // # of sectors discarded
            // This is the total number of sectors discarded successfully.
            discardsectors = str2ull(procfile_lineword(ff, l, 16), NULL); // dc_sec

            // # of milliseconds spent discarding
            // This is the total number of milliseconds spent by all discards (as
            // measured from __make_request() to end_that_request_last()).
            discardms      = str2ull(procfile_lineword(ff, l, 17), NULL); // dc_ticks
        }

        if (unlikely(words > 17)) {
            do_fl_stats = 1;

            // number of flush I/Os processed
            // These values increment when an flush I/O request completes.
            // Block layer combines flush requests and executes at most one at a time.
            // This counts flush requests executed by disk. Not tracked for partitions.
            flushes        = str2ull(procfile_lineword(ff, l, 18), NULL); // fl_ios

            // total wait time for flush requests
            flushms        = str2ull(procfile_lineword(ff, l, 19), NULL); // fl_ticks
        }

        // --------------------------------------------------------------------------
        // get a disk structure for the disk

        struct disk *d = get_disk(major, minor, disk);
        d->updated = 1;

        // --------------------------------------------------------------------------
        // count the global system disk I/O of physical disks

        if(unlikely(d->type == DISK_TYPE_PHYSICAL)) {
            system_read_kb  += readsectors * SECTOR_SIZE / 1024;
            system_write_kb += writesectors * SECTOR_SIZE / 1024;
        }

        // --------------------------------------------------------------------------
        // Set its family based on mount point

        char *family = d->mount_point;
        if(!family) family = d->disk;


        // --------------------------------------------------------------------------
        // Do performance metrics
        if (d->do_io == CONFIG_BOOLEAN_YES || d->do_io == CONFIG_BOOLEAN_AUTO) {
            d->do_io = CONFIG_BOOLEAN_YES;

            last_readsectors = d->disk_io.rd_io_reads ? d->disk_io.rd_io_reads->collector.last_collected_value / SECTOR_SIZE : 0;
            last_writesectors = d->disk_io.rd_io_writes ? d->disk_io.rd_io_writes->collector.last_collected_value / SECTOR_SIZE : 0;

            common_disk_io(&d->disk_io,
                           d->chart_id,
                           d->disk,
                           readsectors * SECTOR_SIZE,
                           writesectors * SECTOR_SIZE,
                           update_every,
                           disk_labels_cb,
                           d);
        }

        if (do_dc_stats && d->do_io == CONFIG_BOOLEAN_YES && d->do_ext != CONFIG_BOOLEAN_NO) {
            if (unlikely(!d->st_ext_io)) {
                d->st_ext_io = rrdset_create_localhost(
                        "disk_ext"
                        , d->chart_id
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

                d->rd_io_discards = rrddim_add(d->st_ext_io, "discards", NULL, SECTOR_SIZE, 1024, RRD_ALGORITHM_INCREMENTAL);

                add_labels_to_disk(d, d->st_ext_io);
            }

            last_discardsectors = rrddim_set_by_pointer(d->st_ext_io, d->rd_io_discards, discardsectors);
            rrdset_done(d->st_ext_io);
        }

        if (d->do_ops == CONFIG_BOOLEAN_YES || d->do_ops == CONFIG_BOOLEAN_AUTO) {
            d->do_ops = CONFIG_BOOLEAN_YES;

            last_rd_ios = d->disk_ops.rd_ops_reads ? d->disk_ops.rd_ops_reads->collector.last_collected_value : 0;
            last_wr_ios = d->disk_ops.rd_ops_writes ? d->disk_ops.rd_ops_writes->collector.last_collected_value : 0;

            common_disk_ops(&d->disk_ops,
                           d->chart_id,
                           d->disk, rd_ios, wr_ios,
                           update_every,
                           disk_labels_cb,
                           d);
        }

        if (do_dc_stats && d->do_ops == CONFIG_BOOLEAN_YES && d->do_ext != CONFIG_BOOLEAN_NO) {
            if (unlikely(!d->st_ext_ops)) {
                d->st_ext_ops = rrdset_create_localhost(
                        "disk_ext_ops"
                        , d->chart_id
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

                d->rd_ops_discards = rrddim_add(d->st_ext_ops, "discards", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                if (do_fl_stats)
                    d->rd_ops_flushes = rrddim_add(d->st_ext_ops, "flushes", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

                add_labels_to_disk(d, d->st_ext_ops);
            }

            last_discards = rrddim_set_by_pointer(d->st_ext_ops, d->rd_ops_discards, discards);
            if (do_fl_stats)
                last_flushes = rrddim_set_by_pointer(d->st_ext_ops, d->rd_ops_flushes, flushes);
            rrdset_done(d->st_ext_ops);
        }

        if (d->do_qops == CONFIG_BOOLEAN_YES || d->do_qops == CONFIG_BOOLEAN_AUTO) {
            d->do_qops = CONFIG_BOOLEAN_YES;

            common_disk_qops(
                    &d->disk_qops,
                    d->chart_id,
                    d->disk,
                    queued_ios,
                    update_every,
                    disk_labels_cb,
                    d);
        }

        if (d->do_backlog == CONFIG_BOOLEAN_YES || d->do_backlog == CONFIG_BOOLEAN_AUTO) {
            d->do_backlog = CONFIG_BOOLEAN_YES;

            if(unlikely(!d->st_backlog)) {
                d->st_backlog = rrdset_create_localhost(
                        "disk_backlog"
                        , d->chart_id
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

                d->rd_backlog_backlog = rrddim_add(d->st_backlog, "backlog", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

                add_labels_to_disk(d, d->st_backlog);
            }

            rrddim_set_by_pointer(d->st_backlog, d->rd_backlog_backlog, backlog_ms);
            rrdset_done(d->st_backlog);
        }

        if (d->do_util == CONFIG_BOOLEAN_YES || d->do_util == CONFIG_BOOLEAN_AUTO) {
            d->do_util = CONFIG_BOOLEAN_YES;

            last_busy_ms = d->disk_busy.rd_busy ? d->disk_busy.rd_busy->collector.last_collected_value : 0;

            common_disk_busy(&d->disk_busy,
                             d->chart_id,
                             d->disk,
                             busy_ms,
                             update_every,
                             disk_labels_cb,
                             d);

            collected_number disk_utilization = (busy_ms - last_busy_ms) / (10 * update_every);
            if (disk_utilization > 100)
                disk_utilization = 100;

            common_disk_util(&d->disk_util,
                             d->chart_id,
                             d->disk,
                             disk_utilization,
                             update_every,
                             disk_labels_cb,
                             d);

        }

        if (d->do_mops == CONFIG_BOOLEAN_YES || d->do_mops == CONFIG_BOOLEAN_AUTO) {
            d->do_mops = CONFIG_BOOLEAN_YES;

            if(unlikely(!d->st_mops)) {
                d->st_mops = rrdset_create_localhost(
                        "disk_mops"
                        , d->chart_id
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

                d->rd_mops_reads  = rrddim_add(d->st_mops, "reads",  NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                d->rd_mops_writes = rrddim_add(d->st_mops, "writes", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);

                add_labels_to_disk(d, d->st_mops);
            }

            rrddim_set_by_pointer(d->st_mops, d->rd_mops_reads,  mreads);
            rrddim_set_by_pointer(d->st_mops, d->rd_mops_writes, mwrites);
            rrdset_done(d->st_mops);
        }

        if(do_dc_stats && d->do_mops == CONFIG_BOOLEAN_YES && d->do_ext != CONFIG_BOOLEAN_NO) {
            d->do_mops = CONFIG_BOOLEAN_YES;

            if(unlikely(!d->st_ext_mops)) {
                d->st_ext_mops = rrdset_create_localhost(
                        "disk_ext_mops"
                        , d->chart_id
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

                d->rd_mops_discards = rrddim_add(d->st_ext_mops, "discards", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

                add_labels_to_disk(d, d->st_ext_mops);
            }

            rrddim_set_by_pointer(d->st_ext_mops, d->rd_mops_discards, mdiscards);
            rrdset_done(d->st_ext_mops);
        }

        if (d->do_iotime == CONFIG_BOOLEAN_YES || d->do_iotime == CONFIG_BOOLEAN_AUTO) {
            d->do_iotime = CONFIG_BOOLEAN_YES;

            last_readms  = d->disk_iotime.rd_reads_ms ? d->disk_iotime.rd_reads_ms->collector.last_collected_value : 0;
            last_writems = d->disk_iotime.rd_writes_ms ? d->disk_iotime.rd_writes_ms->collector.last_collected_value : 0;

            common_disk_iotime(
                    &d->disk_iotime,
                    d->chart_id,
                    d->disk,
                    readms,
                    writems,
                    update_every,
                    disk_labels_cb,
                    d);
        }

        if(do_dc_stats && d->do_iotime == CONFIG_BOOLEAN_YES && d->do_ext != CONFIG_BOOLEAN_NO) {
            if(unlikely(!d->st_ext_iotime)) {
                d->st_ext_iotime = rrdset_create_localhost(
                        "disk_ext_iotime"
                        , d->chart_id
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

                d->rd_iotime_discards = rrddim_add(d->st_ext_iotime, "discards", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                if (do_fl_stats)
                    d->rd_iotime_flushes = rrddim_add(d->st_ext_iotime, "flushes", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

                add_labels_to_disk(d, d->st_ext_iotime);
            }

            last_discardms = rrddim_set_by_pointer(d->st_ext_iotime, d->rd_iotime_discards, discardms);
            if (do_fl_stats)
                last_flushms = rrddim_set_by_pointer(d->st_ext_iotime, d->rd_iotime_flushes, flushms);
            rrdset_done(d->st_ext_iotime);
        }

        // calculate differential charts
        // only if this is not the first time we run

        if(likely(dt)) {
            if ((d->do_iotime == CONFIG_BOOLEAN_YES || d->do_iotime == CONFIG_BOOLEAN_AUTO) &&
                (d->do_ops == CONFIG_BOOLEAN_YES || d->do_ops == CONFIG_BOOLEAN_AUTO)) {

                double read_ms_avg = (rd_ios - last_rd_ios) ? (double)(readms - last_readms) / (rd_ios - last_rd_ios) : 0;
                double write_ms_avg = (wr_ios - last_wr_ios) ? (double)(writems - last_writems) / (wr_ios - last_wr_ios) : 0;

                common_disk_await(
                        &d->disk_await,
                        d->chart_id,
                        d->disk,
                        read_ms_avg,
                        write_ms_avg,
                        update_every,
                        disk_labels_cb,
                        d);
            }

            if (do_dc_stats && d->do_iotime == CONFIG_BOOLEAN_YES && d->do_ops == CONFIG_BOOLEAN_YES && d->do_ext != CONFIG_BOOLEAN_NO) {
                if(unlikely(!d->st_ext_await)) {
                    d->st_ext_await = rrdset_create_localhost(
                            "disk_ext_await"
                            , d->chart_id
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

                    d->rd_await_discards = rrddim_add(d->st_ext_await, "discards", NULL, 1, 1000, RRD_ALGORITHM_ABSOLUTE);
                    if (do_fl_stats)
                        d->rd_await_flushes = rrddim_add(d->st_ext_await, "flushes", NULL, 1, 1000, RRD_ALGORITHM_ABSOLUTE);

                    add_labels_to_disk(d, d->st_ext_await);
                }

                double discard_avg =
                    (discards - last_discards) ? (double)(discardms - last_discardms) / (discards - last_discards) : 0;
                double flushe_avg =
                    (flushes - last_flushes) ? (double)(flushms - last_flushms) / (flushes - last_flushes) : 0;

                rrddim_set_by_pointer(d->st_ext_await, d->rd_await_discards, (collected_number)(discard_avg * 1000));

                if (do_fl_stats)
                    rrddim_set_by_pointer(d->st_ext_await, d->rd_await_flushes, (collected_number)(flushe_avg * 1000));

                rrdset_done(d->st_ext_await);
            }

            if ((d->do_io == CONFIG_BOOLEAN_YES || d->do_io == CONFIG_BOOLEAN_AUTO) &&
                (d->do_ops == CONFIG_BOOLEAN_YES || d->do_ops == CONFIG_BOOLEAN_AUTO)) {

                kernel_uint_t avg_read_bytes = SECTOR_SIZE * ((rd_ios - last_rd_ios)  ? (readsectors  - last_readsectors)  / (rd_ios - last_rd_ios) : 0);
                kernel_uint_t avg_write_bytes = SECTOR_SIZE * ((wr_ios - last_wr_ios) ? (writesectors - last_writesectors) / (wr_ios - last_wr_ios) : 0);

                common_disk_avgsz(
                        &d->disk_avgsz,
                        d->chart_id,
                        d->disk,
                        avg_read_bytes,
                        avg_write_bytes,
                        update_every,
                        disk_labels_cb,
                        d);
            }

            if(do_dc_stats && d->do_io  == CONFIG_BOOLEAN_YES && d->do_ops == CONFIG_BOOLEAN_YES && d->do_ext != CONFIG_BOOLEAN_NO) {
                if(unlikely(!d->st_ext_avgsz)) {
                    d->st_ext_avgsz = rrdset_create_localhost(
                            "disk_ext_avgsz"
                            , d->chart_id
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

                    d->rd_avgsz_discards = rrddim_add(d->st_ext_avgsz, "discards", NULL, SECTOR_SIZE, 1024, RRD_ALGORITHM_ABSOLUTE);

                    add_labels_to_disk(d, d->st_ext_avgsz);
                }

                rrddim_set_by_pointer(
                    d->st_ext_avgsz, d->rd_avgsz_discards,
                    (discards - last_discards) ? (discardsectors - last_discardsectors) / (discards - last_discards) :
                                                 0);
                rrdset_done(d->st_ext_avgsz);
            }

            if ((d->do_util == CONFIG_BOOLEAN_YES || d->do_util == CONFIG_BOOLEAN_AUTO) &&
                (d->do_ops == CONFIG_BOOLEAN_YES || d->do_ops == CONFIG_BOOLEAN_AUTO)) {

                double svctm_avg =
                        ((rd_ios - last_rd_ios) + (wr_ios - last_wr_ios)) ?
                        (double) (busy_ms - last_busy_ms) / ((rd_ios - last_rd_ios) + (wr_ios - last_wr_ios)) :
                        0;

                common_disk_svctm(
                        &d->disk_svctm,
                        d->chart_id,
                        d->disk,
                        svctm_avg,
                        update_every,
                        disk_labels_cb,
                        d);
            }
        }

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
                            , d->chart_id
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

                    add_labels_to_disk(d, d->st_bcache_hit_ratio);
                }

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
                            , d->chart_id
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

                    add_labels_to_disk(d, d->st_bcache_rates);
                }

                rrddim_set_by_pointer(d->st_bcache_rates, d->rd_bcache_rate_writeback, writeback_rate);
                rrddim_set_by_pointer(d->st_bcache_rates, d->rd_bcache_rate_congested, cache_congested);
                rrdset_done(d->st_bcache_rates);
            }

            {
                if(unlikely(!d->st_bcache_size)) {
                    d->st_bcache_size = rrdset_create_localhost(
                            "disk_bcache_size"
                            , d->chart_id
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

                    add_labels_to_disk(d, d->st_bcache_size);
                }

                rrddim_set_by_pointer(d->st_bcache_size, d->rd_bcache_dirty_size, dirty_data);
                rrdset_done(d->st_bcache_size);
            }

            {
                if(unlikely(!d->st_bcache_usage)) {
                    d->st_bcache_usage = rrdset_create_localhost(
                            "disk_bcache_usage"
                            , d->chart_id
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

                    add_labels_to_disk(d, d->st_bcache_usage);
                }

                rrddim_set_by_pointer(d->st_bcache_usage, d->rd_bcache_available_percent, cache_available_percent);
                rrdset_done(d->st_bcache_usage);
            }

            {

                if(unlikely(!d->st_bcache_cache_read_races)) {
                    d->st_bcache_cache_read_races = rrdset_create_localhost(
                            "disk_bcache_cache_read_races"
                            , d->chart_id
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

                    add_labels_to_disk(d, d->st_bcache_cache_read_races);
                }

                rrddim_set_by_pointer(d->st_bcache_cache_read_races, d->rd_bcache_cache_read_races, cache_read_races);
                rrddim_set_by_pointer(d->st_bcache_cache_read_races, d->rd_bcache_cache_io_errors, cache_io_errors);
                rrdset_done(d->st_bcache_cache_read_races);
            }

            if (d->do_bcache == CONFIG_BOOLEAN_YES || d->do_bcache == CONFIG_BOOLEAN_AUTO) {
                if(unlikely(!d->st_bcache)) {
                    d->st_bcache = rrdset_create_localhost(
                            "disk_bcache"
                            , d->chart_id
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

                    d->rd_bcache_hits            = rrddim_add(d->st_bcache, "hits",       NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    d->rd_bcache_misses          = rrddim_add(d->st_bcache, "misses",     NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    d->rd_bcache_miss_collisions = rrddim_add(d->st_bcache, "collisions", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    d->rd_bcache_readaheads      = rrddim_add(d->st_bcache, "readaheads", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);

                    add_labels_to_disk(d, d->st_bcache);
                }

                rrddim_set_by_pointer(d->st_bcache, d->rd_bcache_hits, stats_total_cache_hits);
                rrddim_set_by_pointer(d->st_bcache, d->rd_bcache_misses, stats_total_cache_misses);
                rrddim_set_by_pointer(d->st_bcache, d->rd_bcache_miss_collisions, stats_total_cache_miss_collisions);
                rrddim_set_by_pointer(d->st_bcache, d->rd_bcache_readaheads, cache_readaheads);
                rrdset_done(d->st_bcache);
            }

            if (d->do_bcache == CONFIG_BOOLEAN_YES || d->do_bcache == CONFIG_BOOLEAN_AUTO) {
                if(unlikely(!d->st_bcache_bypass)) {
                    d->st_bcache_bypass = rrdset_create_localhost(
                            "disk_bcache_bypass"
                            , d->chart_id
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

                    d->rd_bcache_bypass_hits   = rrddim_add(d->st_bcache_bypass, "hits",   NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    d->rd_bcache_bypass_misses = rrddim_add(d->st_bcache_bypass, "misses", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);

                    add_labels_to_disk(d, d->st_bcache_bypass);
                }

                rrddim_set_by_pointer(d->st_bcache_bypass, d->rd_bcache_bypass_hits, stats_total_cache_bypass_hits);
                rrddim_set_by_pointer(d->st_bcache_bypass, d->rd_bcache_bypass_misses, stats_total_cache_bypass_misses);
                rrdset_done(d->st_bcache_bypass);
            }
        }

        d->function_ready = !d->excluded;
    }

    diskstats_cleanup_disks();

    netdata_mutex_unlock(&diskstats_dev_mutex);
    // update the system total I/O

    if (global_do_io == CONFIG_BOOLEAN_YES || global_do_io == CONFIG_BOOLEAN_AUTO) {
        common_system_io(system_read_kb * 1024, system_write_kb * 1024, update_every);
    }

    return 0;
}
