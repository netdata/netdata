// SPDX-License-Identifier: GPL-3.0-or-later

#include "plugin_proc.h"

#define PLUGIN_PROC_MODULE_ZRAM_NAME "/sys/block/zram"
#define rrdset_obsolete_and_pointer_null(st) do { if(st) { rrdset_is_obsolete___safe_from_collector_thread(st); (st) = NULL; } } while(st)

typedef struct mm_stat {
    unsigned long long orig_data_size;
    unsigned long long compr_data_size;
    unsigned long long mem_used_total;
    unsigned long long mem_limit;
    unsigned long long mem_used_max;
    unsigned long long same_pages;
    unsigned long long pages_compacted;
} MM_STAT;

typedef struct zram_device {
    procfile *file;

    RRDSET *st_usage;
    RRDDIM *rd_compr_data_size;
    RRDDIM *rd_metadata_size;

    RRDSET *st_savings;
    RRDDIM *rd_original_size;
    RRDDIM *rd_savings_size;

    RRDSET *st_comp_ratio;
    RRDDIM *rd_comp_ratio;

    RRDSET *st_alloc_efficiency;
    RRDDIM *rd_alloc_efficiency;
} ZRAM_DEVICE;

static int try_get_zram_major_number(procfile *file) {
    size_t i;
    unsigned int lines = procfile_lines(file);
    int id = -1;
    char *name = NULL;
    for (i = 0; i < lines; i++)
    {
        if (procfile_linewords(file, i) < 2)
            continue;
        name = procfile_lineword(file, i, 1);
        if (strcmp(name, "zram") == 0)
        {
            id = str2i(procfile_lineword(file, i, 0));
            if (id == 0)
                return -1;
            return id;
        }
    }
    return -1;
}

static inline void init_rrd(const char *name, ZRAM_DEVICE *d, int update_every) {
    char chart_name[RRD_ID_LENGTH_MAX + 1];

    snprintfz(chart_name, RRD_ID_LENGTH_MAX, "zram_usage.%s", name);
    d->st_usage = rrdset_create_localhost(
        "mem"
        , chart_name
        , chart_name
        , "zram"
        , "mem.zram_usage"
        , "ZRAM Memory Usage"
        , "MiB"
        , PLUGIN_PROC_NAME
        , PLUGIN_PROC_MODULE_ZRAM_NAME
        , NETDATA_CHART_PRIO_MEM_ZRAM
        , update_every
        , RRDSET_TYPE_AREA);
    d->rd_compr_data_size = rrddim_add(d->st_usage, "compressed", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
    d->rd_metadata_size = rrddim_add(d->st_usage, "metadata", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
    rrdlabels_add(d->st_usage->rrdlabels, "device", name, RRDLABEL_SRC_AUTO);

    snprintfz(chart_name, RRD_ID_LENGTH_MAX, "zram_savings.%s", name);
    d->st_savings = rrdset_create_localhost(
        "mem"
        , chart_name
        , chart_name
        , "zram"
        , "mem.zram_savings"
        , "ZRAM Memory Savings"
        , "MiB"
        , PLUGIN_PROC_NAME
        , PLUGIN_PROC_MODULE_ZRAM_NAME
        , NETDATA_CHART_PRIO_MEM_ZRAM_SAVINGS
        , update_every
        , RRDSET_TYPE_AREA);
    d->rd_savings_size = rrddim_add(d->st_savings, "savings", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
    d->rd_original_size = rrddim_add(d->st_savings, "original", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
    rrdlabels_add(d->st_savings->rrdlabels, "device", name, RRDLABEL_SRC_AUTO);

    snprintfz(chart_name, RRD_ID_LENGTH_MAX, "zram_ratio.%s", name);
    d->st_comp_ratio = rrdset_create_localhost(
        "mem"
        , chart_name
        , chart_name
        , "zram"
        , "mem.zram_ratio"
        , "ZRAM Compression Ratio (original to compressed)"
        , "ratio"
        , PLUGIN_PROC_NAME
        , PLUGIN_PROC_MODULE_ZRAM_NAME
        , NETDATA_CHART_PRIO_MEM_ZRAM_RATIO
        , update_every
        , RRDSET_TYPE_LINE);
    d->rd_comp_ratio = rrddim_add(d->st_comp_ratio, "ratio", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
    rrdlabels_add(d->st_comp_ratio->rrdlabels, "device", name, RRDLABEL_SRC_AUTO);

    snprintfz(chart_name, RRD_ID_LENGTH_MAX, "zram_efficiency.%s", name);
    d->st_alloc_efficiency = rrdset_create_localhost(
        "mem"
        , chart_name
        , chart_name
        , "zram"
        , "mem.zram_efficiency"
        , "ZRAM Efficiency"
        , "percentage"
        , PLUGIN_PROC_NAME
        , PLUGIN_PROC_MODULE_ZRAM_NAME
        , NETDATA_CHART_PRIO_MEM_ZRAM_EFFICIENCY
        , update_every
        , RRDSET_TYPE_LINE);
    d->rd_alloc_efficiency = rrddim_add(d->st_alloc_efficiency, "percent", NULL, 1, 10000, RRD_ALGORITHM_ABSOLUTE);
    rrdlabels_add(d->st_alloc_efficiency->rrdlabels, "device", name, RRDLABEL_SRC_AUTO);
}

static int init_devices(DICTIONARY *devices, int update_every) {
    int count = 0;
    struct dirent *de;
    struct stat st;
    procfile *ff = NULL;
    ZRAM_DEVICE device;
    char filename[FILENAME_MAX + 1];

    snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/sys/block");
    DIR *dir = opendir(filename);

    if (unlikely(!dir))
        return 0;

    while ((de = readdir(dir))) {
        snprintfz(filename, FILENAME_MAX, "%s/sys/block/%s/mm_stat", netdata_configured_host_prefix, de->d_name);
        if (unlikely(stat(filename, &st) != 0)) {
            continue;
        }
        ff = procfile_open(filename, " \t:", PROCFILE_FLAG_DEFAULT);
        if (ff == NULL) {
            collector_error("ZRAM : Failed to open %s: %s", filename, strerror(errno));
            continue;
        }

        device.file = ff;
        init_rrd(de->d_name, &device, update_every);
        dictionary_set(devices, de->d_name, &device, sizeof(ZRAM_DEVICE));
        count++;
    }

    closedir(dir);

    return count;
}

static void free_device(DICTIONARY *dict, const char *name)
{
    ZRAM_DEVICE *d = (ZRAM_DEVICE*)dictionary_get(dict, name);
    collector_info("ZRAM : Disabling monitoring of device %s", name);
    rrdset_obsolete_and_pointer_null(d->st_usage);
    rrdset_obsolete_and_pointer_null(d->st_savings);
    rrdset_obsolete_and_pointer_null(d->st_alloc_efficiency);
    rrdset_obsolete_and_pointer_null(d->st_comp_ratio);
    dictionary_del(dict, name);
}

static inline int read_mm_stat(procfile *ff, MM_STAT *stats) {
    ff = procfile_readall(ff);
    if (!ff)
        return -1;
    if (procfile_lines(ff) < 1) {
        procfile_close(ff);
        return -1;
    }
    if (procfile_linewords(ff, 0) < 7) {
        procfile_close(ff);
        return -1;
    }

    stats->orig_data_size = str2ull(procfile_word(ff, 0), NULL);
    stats->compr_data_size = str2ull(procfile_word(ff, 1), NULL);
    stats->mem_used_total = str2ull(procfile_word(ff, 2), NULL);
    stats->mem_limit = str2ull(procfile_word(ff, 3), NULL);
    stats->mem_used_max = str2ull(procfile_word(ff, 4), NULL);
    stats->same_pages = str2ull(procfile_word(ff, 5), NULL);
    stats->pages_compacted = str2ull(procfile_word(ff, 6), NULL);
    return 0;
}

static int collect_zram_metrics(const DICTIONARY_ITEM *item, void *entry, void *data) {
    const char *name = dictionary_acquired_item_name(item);
    ZRAM_DEVICE *dev = entry;
    DICTIONARY *dict = data;

    MM_STAT mm;
    int value;

    if (unlikely(read_mm_stat(dev->file, &mm) < 0)) {
        free_device(dict, name);
        return -1;
    }

    // zram_usage
    rrddim_set_by_pointer(dev->st_usage, dev->rd_compr_data_size, mm.compr_data_size);
    rrddim_set_by_pointer(dev->st_usage, dev->rd_metadata_size, mm.mem_used_total - mm.compr_data_size);
    rrdset_done(dev->st_usage);

    // zram_savings
    rrddim_set_by_pointer(dev->st_savings, dev->rd_savings_size, mm.compr_data_size - mm.orig_data_size);
    rrddim_set_by_pointer(dev->st_savings, dev->rd_original_size, mm.orig_data_size);
    rrdset_done(dev->st_savings);

    // zram_ratio
    value = mm.compr_data_size == 0 ? 1 : mm.orig_data_size * 100 / mm.compr_data_size;
    rrddim_set_by_pointer(dev->st_comp_ratio, dev->rd_comp_ratio, value);
    rrdset_done(dev->st_comp_ratio);

    // zram_efficiency
    value = mm.mem_used_total == 0 ? 100 : (mm.compr_data_size * 1000000 / mm.mem_used_total);
    rrddim_set_by_pointer(dev->st_alloc_efficiency, dev->rd_alloc_efficiency, value);
    rrdset_done(dev->st_alloc_efficiency);

    return 0;
}

int do_sys_block_zram(int update_every, usec_t dt) {
    static procfile *ff = NULL;
    static DICTIONARY *devices = NULL;
    static int initialized = 0;
    static int device_count = 0;
    int zram_id = -1;

    (void)dt;

    if (unlikely(!initialized))
    {
        initialized = 1;

        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/proc/devices");

        ff = procfile_open(filename, " \t:", PROCFILE_FLAG_DEFAULT);
        if (ff == NULL)
        {
            collector_error("Cannot read %s", filename);
            return 1;
        }
        ff = procfile_readall(ff);
        if (!ff)
            return 1;
        zram_id = try_get_zram_major_number(ff);
        if (zram_id == -1)
        {
            if (ff != NULL)
                procfile_close(ff);
            return 1;
        }
        procfile_close(ff);

        devices = dictionary_create_advanced(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_FIXED_SIZE, &dictionary_stats_category_collectors, sizeof(ZRAM_DEVICE));
        device_count = init_devices(devices, update_every);
    }

    if (unlikely(device_count < 1))
        return 1;

    dictionary_walkthrough_write(devices, collect_zram_metrics, devices);
    return 0;
}
