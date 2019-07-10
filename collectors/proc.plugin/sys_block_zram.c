// SPDX-License-Identifier: GPL-3.0-or-later

#include "plugin_proc.h"

#define PLUGIN_PROC_MODULE_ZRAM_NAME "/sys/block/zram"
#define rrdset_obsolete_and_pointer_null(st) do { if(st) { rrdset_is_obsolete(st); (st) = NULL; } } while(st)

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

    // --------------------------------------------------------------------

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
        , name
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

    snprintfz(chart_name, RRD_ID_LENGTH_MAX, "zram_savings.%s", name);
    d->st_savings = rrdset_create_localhost(
        "mem"
        , chart_name
        , chart_name
        , name
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

    snprintfz(chart_name, RRD_ID_LENGTH_MAX, "zram_ratio.%s", name);
    d->st_comp_ratio = rrdset_create_localhost(
        "mem"
        , chart_name
        , chart_name
        , name
        , "mem.zram_ratio"
        , "ZRAM Compression Ratio (original to compressed)"
        , "ratio"
        , PLUGIN_PROC_NAME
        , PLUGIN_PROC_MODULE_ZRAM_NAME
        , NETDATA_CHART_PRIO_MEM_ZRAM_RATIO
        , update_every
        , RRDSET_TYPE_LINE);
    d->rd_comp_ratio = rrddim_add(d->st_comp_ratio, "ratio", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);

    snprintfz(chart_name, RRD_ID_LENGTH_MAX, "zram_efficiency.%s", name);
    d->st_alloc_efficiency = rrdset_create_localhost(
        "mem"
        , chart_name
        , chart_name
        , name
        , "mem.zram_efficiency"
        , "ZRAM Efficiency"
        , "percentage"
        , PLUGIN_PROC_NAME
        , PLUGIN_PROC_MODULE_ZRAM_NAME
        , NETDATA_CHART_PRIO_MEM_ZRAM_EFFICIENCY
        , update_every
        , RRDSET_TYPE_LINE);
    d->rd_alloc_efficiency = rrddim_add(d->st_alloc_efficiency, "percent", NULL, 1, 10000, RRD_ALGORITHM_ABSOLUTE);
}

static int init_devices(DICTIONARY *devices, unsigned int zram_id, int update_every) {
    int count = 0;
    DIR *dir = opendir("/dev");
    struct dirent *de;
    struct stat st;
    char filename[FILENAME_MAX + 1];
    procfile *ff = NULL;
    ZRAM_DEVICE device;

    if (unlikely(!dir))
        return 0;
    while ((de = readdir(dir)))
    {
        snprintfz(filename, FILENAME_MAX, "/dev/%s", de->d_name);
        if (unlikely(stat(filename, &st) != 0))
        {
            error("ZRAM : Unable to stat %s: %s", filename, strerror(errno));
            continue;
        }
        if (major(st.st_rdev) == zram_id)
        {
            info("ZRAM : Found device %s", filename);
            snprintfz(filename, FILENAME_MAX, "/sys/block/%s/mm_stat", de->d_name);
            ff = procfile_open(filename, " \t:", PROCFILE_FLAG_DEFAULT);
            if (ff == NULL)
            {
                error("ZRAM : Failed to open %s: %s", filename, strerror(errno));
                continue;
            }
            device.file = ff;
            init_rrd(de->d_name, &device, update_every);
            dictionary_set(devices, de->d_name, &device, sizeof(ZRAM_DEVICE));
            count++;
        }
    }
    closedir(dir);
    return count;
}

static void free_device(DICTIONARY *dict, char *name)
{
    ZRAM_DEVICE *d = (ZRAM_DEVICE*)dictionary_get(dict, name);
    info("ZRAM : Disabling monitoring of device %s", name);
    rrdset_obsolete_and_pointer_null(d->st_usage);
    rrdset_obsolete_and_pointer_null(d->st_savings);
    rrdset_obsolete_and_pointer_null(d->st_alloc_efficiency);
    rrdset_obsolete_and_pointer_null(d->st_comp_ratio);
    dictionary_del(dict, name);
}
    // --------------------------------------------------------------------

static inline int read_mm_stat(procfile *ff, MM_STAT *stats) {
    ff = procfile_readall(ff);
    if (!ff)
        return -1;
    if (procfile_lines(ff) < 1)
        return -1;
    if (procfile_linewords(ff, 0) < 7)
        return -1;

    stats->orig_data_size = str2ull(procfile_word(ff, 0));
    stats->compr_data_size = str2ull(procfile_word(ff, 1));
    stats->mem_used_total = str2ull(procfile_word(ff, 2));
    stats->mem_limit = str2ull(procfile_word(ff, 3));
    stats->mem_used_max = str2ull(procfile_word(ff, 4));
    stats->same_pages = str2ull(procfile_word(ff, 5));
    stats->pages_compacted = str2ull(procfile_word(ff, 6));
    return 0;
}

static inline int _collect_zram_metrics(char* name, ZRAM_DEVICE *d, int advance, DICTIONARY* dict) {
    MM_STAT mm;
    int value;
    if (unlikely(read_mm_stat(d->file, &mm) < 0))
    {
        free_device(dict, name);
        return -1;
    }

    if (likely(advance))
    {
        rrdset_next(d->st_usage);
        rrdset_next(d->st_savings);
        rrdset_next(d->st_comp_ratio);
        rrdset_next(d->st_alloc_efficiency);
    }
    // zram_usage
    rrddim_set_by_pointer(d->st_usage, d->rd_compr_data_size, mm.compr_data_size);
    rrddim_set_by_pointer(d->st_usage, d->rd_metadata_size, mm.mem_used_total - mm.compr_data_size);
    rrdset_done(d->st_usage);
    // zram_savings
    rrddim_set_by_pointer(d->st_savings, d->rd_savings_size, mm.compr_data_size - mm.orig_data_size);
    rrddim_set_by_pointer(d->st_savings, d->rd_original_size, mm.orig_data_size);
    rrdset_done(d->st_savings);
    // zram_ratio
    value = mm.compr_data_size == 0 ? 1 : mm.orig_data_size * 100 / mm.compr_data_size;
    rrddim_set_by_pointer(d->st_comp_ratio, d->rd_comp_ratio, value);
    rrdset_done(d->st_comp_ratio);
    // zram_efficiency
    value = mm.mem_used_total == 0 ? 100 : (mm.compr_data_size * 1000000 / mm.mem_used_total);
    rrddim_set_by_pointer(d->st_alloc_efficiency, d->rd_alloc_efficiency, value);
    rrdset_done(d->st_alloc_efficiency);
    return 0;
}

static int collect_first_zram_metrics(char *name, void *entry, void *data) {
    // collect without calling rrdset_next (init only)
    return _collect_zram_metrics(name, (ZRAM_DEVICE *)entry, 0, (DICTIONARY *)data);
}

static int collect_zram_metrics(char *name, void *entry, void *data) {
    (void)name;
    // collect with calling rrdset_next
    return _collect_zram_metrics(name, (ZRAM_DEVICE *)entry, 1, (DICTIONARY *)data);
}

    // --------------------------------------------------------------------

int do_sys_block_zram(int update_every, usec_t dt) {
    (void)dt;
    static procfile *ff = NULL;
    static DICTIONARY *devices = NULL;
    static int initialized = 0;
    static int device_count = 0;
    int zram_id = -1;
    if (unlikely(!initialized))
    {
        initialized = 1;
        ff = procfile_open("/proc/devices", " \t:", PROCFILE_FLAG_DEFAULT);
        if (ff == NULL)
        {
            error("Cannot read /proc/devices");
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

        devices = dictionary_create(DICTIONARY_FLAG_SINGLE_THREADED);
        device_count = init_devices(devices, (unsigned int)zram_id, update_every);
        if (device_count < 1)
            return 1;
        dictionary_get_all_name_value(devices, collect_first_zram_metrics, devices);
    }
    else
    {
        if (unlikely(device_count < 1))
            return 1;
        dictionary_get_all_name_value(devices, collect_zram_metrics, devices);
    }
    return 0;
}