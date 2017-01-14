#include "common.h"

// ----------------------------------------------------------------------------
// cgroup globals

static int cgroup_enable_cpuacct_stat = CONFIG_ONDEMAND_ONDEMAND;
static int cgroup_enable_cpuacct_usage = CONFIG_ONDEMAND_ONDEMAND;
static int cgroup_enable_memory = CONFIG_ONDEMAND_ONDEMAND;
static int cgroup_enable_devices = CONFIG_ONDEMAND_ONDEMAND;
static int cgroup_enable_blkio = CONFIG_ONDEMAND_ONDEMAND;
static int cgroup_enable_new_cgroups_detected_at_runtime = 1;
static int cgroup_check_for_new_every = 10;
static char *cgroup_cpuacct_base = NULL;
static char *cgroup_blkio_base = NULL;
static char *cgroup_memory_base = NULL;
static char *cgroup_devices_base = NULL;

static int cgroup_root_count = 0;
static int cgroup_root_max = 500;
static int cgroup_max_depth = 0;

static NETDATA_SIMPLE_PATTERN *disabled_cgroups_patterns = NULL;
static NETDATA_SIMPLE_PATTERN *disabled_cgroup_paths = NULL;
static NETDATA_SIMPLE_PATTERN *disabled_cgroup_renames = NULL;
static NETDATA_SIMPLE_PATTERN *systemd_services_cgroups = NULL;

static char *cgroups_rename_script = PLUGINS_DIR "/cgroup-name.sh";

void read_cgroup_plugin_configuration() {
    cgroup_check_for_new_every = (int)config_get_number("plugin:cgroups", "check for new cgroups every", cgroup_check_for_new_every);

    cgroup_enable_cpuacct_stat = config_get_boolean_ondemand("plugin:cgroups", "enable cpuacct stat", cgroup_enable_cpuacct_stat);
    cgroup_enable_cpuacct_usage = config_get_boolean_ondemand("plugin:cgroups", "enable cpuacct usage", cgroup_enable_cpuacct_usage);
    cgroup_enable_memory = config_get_boolean_ondemand("plugin:cgroups", "enable memory", cgroup_enable_memory);
    cgroup_enable_blkio = config_get_boolean_ondemand("plugin:cgroups", "enable blkio", cgroup_enable_blkio);

    char filename[FILENAME_MAX + 1], *s;
    struct mountinfo *mi, *root = mountinfo_read(0);

    mi = mountinfo_find_by_filesystem_super_option(root, "cgroup", "cpuacct");
    if(!mi) mi = mountinfo_find_by_filesystem_mount_source(root, "cgroup", "cpuacct");
    if(!mi) {
        error("Cannot find cgroup cpuacct mountinfo. Assuming default: /sys/fs/cgroup/cpuacct");
        s = "/sys/fs/cgroup/cpuacct";
    }
    else s = mi->mount_point;
    snprintfz(filename, FILENAME_MAX, "%s%s", global_host_prefix, s);
    cgroup_cpuacct_base = config_get("plugin:cgroups", "path to /sys/fs/cgroup/cpuacct", filename);

    mi = mountinfo_find_by_filesystem_super_option(root, "cgroup", "blkio");
    if(!mi) mi = mountinfo_find_by_filesystem_mount_source(root, "cgroup", "blkio");
    if(!mi) {
        error("Cannot find cgroup blkio mountinfo. Assuming default: /sys/fs/cgroup/blkio");
        s = "/sys/fs/cgroup/blkio";
    }
    else s = mi->mount_point;
    snprintfz(filename, FILENAME_MAX, "%s%s", global_host_prefix, s);
    cgroup_blkio_base = config_get("plugin:cgroups", "path to /sys/fs/cgroup/blkio", filename);

    mi = mountinfo_find_by_filesystem_super_option(root, "cgroup", "memory");
    if(!mi) mi = mountinfo_find_by_filesystem_mount_source(root, "cgroup", "memory");
    if(!mi) {
        error("Cannot find cgroup memory mountinfo. Assuming default: /sys/fs/cgroup/memory");
        s = "/sys/fs/cgroup/memory";
    }
    else s = mi->mount_point;
    snprintfz(filename, FILENAME_MAX, "%s%s", global_host_prefix, s);
    cgroup_memory_base = config_get("plugin:cgroups", "path to /sys/fs/cgroup/memory", filename);

    mi = mountinfo_find_by_filesystem_super_option(root, "cgroup", "devices");
    if(!mi) mi = mountinfo_find_by_filesystem_mount_source(root, "cgroup", "devices");
    if(!mi) {
        error("Cannot find cgroup devices mountinfo. Assuming default: /sys/fs/cgroup/devices");
        s = "/sys/fs/cgroup/devices";
    }
    else s = mi->mount_point;
    snprintfz(filename, FILENAME_MAX, "%s%s", global_host_prefix, s);
    cgroup_devices_base = config_get("plugin:cgroups", "path to /sys/fs/cgroup/devices", filename);

    cgroup_root_max = (int)config_get_number("plugin:cgroups", "max cgroups to allow", cgroup_root_max);
    cgroup_max_depth = (int)config_get_number("plugin:cgroups", "max cgroups depth to monitor", cgroup_max_depth);

    cgroup_enable_new_cgroups_detected_at_runtime = config_get_boolean("plugin:cgroups", "enable new cgroups detected at run time", cgroup_enable_new_cgroups_detected_at_runtime);

    disabled_cgroups_patterns = netdata_simple_pattern_list_create(
            config_get("plugin:cgroups", "disable by default cgroups matching",
                    " / /lxc /docker /libvirt /qemu /systemd "
                    " /system /system.slice "
                    " /machine /machine.slice "
                    " /user /user.slice "
                    " /init.scope "
                    " *.swap "
                    " *.slice "
                    " *.user "
                    " *.mount "
                    " *.partition "
                    " */ns "                               //   /lxc/*/ns    #1397
            ), NETDATA_SIMPLE_PATTERN_MODE_EXACT);

    disabled_cgroup_paths = netdata_simple_pattern_list_create(
            config_get("plugin:cgroups", "do not search for cgroups in paths matching",
                    " /system "
                    " /systemd "
                    " /user.slice "
                    " /user "
                    " /init.scope "
                    " *-qemu "                             //  #345
            ), NETDATA_SIMPLE_PATTERN_MODE_EXACT);

    cgroups_rename_script = config_get("plugin:cgroups", "script to get cgroup names", cgroups_rename_script);

    disabled_cgroup_renames = netdata_simple_pattern_list_create(
            config_get("plugin:cgroups", "do not run script to rename cgroups matching",
                    " / "
                    " *.service "
                    " *.slice "
                    " *.scope "
                    " *.swap "
                    " *.mount "
                    " *.partition "
                    " *.user "
            ), NETDATA_SIMPLE_PATTERN_MODE_EXACT);

    systemd_services_cgroups = netdata_simple_pattern_list_create(
            config_get("plugin:cgroups", "cgroups to match as systemd services",
                    " *.service "
            ), NETDATA_SIMPLE_PATTERN_MODE_EXACT);

    mountinfo_free(root);
}

// ----------------------------------------------------------------------------
// cgroup objects

struct blkio {
    int updated;

    char *filename;

    unsigned long long Read;
    unsigned long long Write;
/*
    unsigned long long Sync;
    unsigned long long Async;
    unsigned long long Total;
*/
};

// https://www.kernel.org/doc/Documentation/cgroup-v1/memory.txt
struct memory {
    int updated;

    char *filename;

    int has_dirty_swap;

    unsigned long long cache;
    unsigned long long rss;
    unsigned long long rss_huge;
    unsigned long long mapped_file;
    unsigned long long writeback;
    unsigned long long dirty;
    unsigned long long swap;
    unsigned long long pgpgin;
    unsigned long long pgpgout;
    unsigned long long pgfault;
    unsigned long long pgmajfault;
/*
    unsigned long long inactive_anon;
    unsigned long long active_anon;
    unsigned long long inactive_file;
    unsigned long long active_file;
    unsigned long long unevictable;
    unsigned long long hierarchical_memory_limit;
    unsigned long long total_cache;
    unsigned long long total_rss;
    unsigned long long total_rss_huge;
    unsigned long long total_mapped_file;
    unsigned long long total_writeback;
    unsigned long long total_dirty;
    unsigned long long total_swap;
    unsigned long long total_pgpgin;
    unsigned long long total_pgpgout;
    unsigned long long total_pgfault;
    unsigned long long total_pgmajfault;
    unsigned long long total_inactive_anon;
    unsigned long long total_active_anon;
    unsigned long long total_inactive_file;
    unsigned long long total_active_file;
    unsigned long long total_unevictable;
*/

    int usage_in_bytes_updated;
    char *filename_usage_in_bytes;
    unsigned long long usage_in_bytes;

    int msw_usage_in_bytes_updated;
    char *filename_msw_usage_in_bytes;
    unsigned long long msw_usage_in_bytes;

    int failcnt_updated;
    char *filename_failcnt;
    unsigned long long failcnt;
};

// https://www.kernel.org/doc/Documentation/cgroup-v1/cpuacct.txt
struct cpuacct_stat {
    int updated;

    char *filename;

    unsigned long long user;
    unsigned long long system;
};

// https://www.kernel.org/doc/Documentation/cgroup-v1/cpuacct.txt
struct cpuacct_usage {
    int updated;

    char *filename;

    unsigned int cpus;
    unsigned long long *cpu_percpu;
};

#define CGROUP_OPTIONS_DISABLED_DUPLICATE   0x00000001
#define CGROUP_OPTIONS_SYSTEM_SLICE_SERVICE 0x00000002

struct cgroup {
    uint32_t options;

    char available;      // found in the filesystem
    char enabled;        // enabled in the config

    char *id;
    uint32_t hash;

    char *chart_id;
    uint32_t hash_chart;

    char *chart_title;

    struct cpuacct_stat cpuacct_stat;
    struct cpuacct_usage cpuacct_usage;

    struct memory memory;

    struct blkio io_service_bytes;              // bytes
    struct blkio io_serviced;                   // operations

    struct blkio throttle_io_service_bytes;     // bytes
    struct blkio throttle_io_serviced;          // operations

    struct blkio io_merged;                     // operations
    struct blkio io_queued;                     // operations

    RRDSET *st_cpu;
    RRDSET *st_cpu_per_core;
    RRDSET *st_mem;
    RRDSET *st_writeback;
    RRDSET *st_mem_activity;
    RRDSET *st_pgfaults;
    RRDSET *st_mem_usage;
    RRDSET *st_mem_failcnt;
    RRDSET *st_io;
    RRDSET *st_serviced_ops;
    RRDSET *st_throttle_io;
    RRDSET *st_throttle_serviced_ops;
    RRDSET *st_queued_ops;
    RRDSET *st_merged_ops;

    struct cgroup *next;

} *cgroup_root = NULL;

// ----------------------------------------------------------------------------
// read values from /sys

void cgroup_read_cpuacct_stat(struct cpuacct_stat *cp) {
    static procfile *ff = NULL;

    static uint32_t user_hash = 0;
    static uint32_t system_hash = 0;

    if(unlikely(user_hash == 0)) {
        user_hash = simple_hash("user");
        system_hash = simple_hash("system");
    }

    cp->updated = 0;
    if(cp->filename) {
        ff = procfile_reopen(ff, cp->filename, NULL, PROCFILE_FLAG_DEFAULT);
        if(!ff) return;

        ff = procfile_readall(ff);
        if(!ff) return;

        unsigned long i, lines = procfile_lines(ff);

        if(lines < 1) {
            error("File '%s' should have 1+ lines.", cp->filename);
            return;
        }

        for(i = 0; i < lines ; i++) {
            char *s = procfile_lineword(ff, i, 0);
            uint32_t hash = simple_hash(s);

            if(hash == user_hash && !strcmp(s, "user"))
                cp->user = strtoull(procfile_lineword(ff, i, 1), NULL, 10);

            else if(hash == system_hash && !strcmp(s, "system"))
                cp->system = strtoull(procfile_lineword(ff, i, 1), NULL, 10);
        }

        cp->updated = 1;

        // fprintf(stderr, "READ '%s': user: %llu, system: %llu\n", cp->filename, cp->user, cp->system);
    }
}

void cgroup_read_cpuacct_usage(struct cpuacct_usage *ca) {
    static procfile *ff = NULL;

    ca->updated = 0;
    if(ca->filename) {
        ff = procfile_reopen(ff, ca->filename, NULL, PROCFILE_FLAG_DEFAULT);
        if(!ff) return;

        ff = procfile_readall(ff);
        if(!ff) return;

        if(procfile_lines(ff) < 1) {
            error("File '%s' should have 1+ lines but has %u.", ca->filename, procfile_lines(ff));
            return;
        }

        unsigned long i = procfile_linewords(ff, 0);
        if(i == 0) return;

        // we may have 1 more CPU reported
        while(i > 0) {
            char *s = procfile_lineword(ff, 0, i - 1);
            if(!*s) i--;
            else break;
        }

        if(i != ca->cpus) {
            freez(ca->cpu_percpu);
            ca->cpu_percpu = mallocz(sizeof(unsigned long long) * i);
            ca->cpus = (unsigned int)i;
        }

        for(i = 0; i < ca->cpus ;i++) {
            ca->cpu_percpu[i] = strtoull(procfile_lineword(ff, 0, i), NULL, 10);
            // fprintf(stderr, "READ '%s': cpu%d/%d: %llu ('%s')\n", ca->filename, i, ca->cpus, ca->cpu_percpu[i], procfile_lineword(ff, 0, i));
        }

        ca->updated = 1;
    }
}

void cgroup_read_blkio(struct blkio *io) {
    static procfile *ff = NULL;

    static uint32_t Read_hash = 0;
    static uint32_t Write_hash = 0;
/*
    static uint32_t Sync_hash = 0;
    static uint32_t Async_hash = 0;
    static uint32_t Total_hash = 0;
*/

    if(unlikely(Read_hash == 0)) {
        Read_hash = simple_hash("Read");
        Write_hash = simple_hash("Write");
/*
        Sync_hash = simple_hash("Sync");
        Async_hash = simple_hash("Async");
        Total_hash = simple_hash("Total");
*/
    }

    io->updated = 0;
    if(io->filename) {
        ff = procfile_reopen(ff, io->filename, NULL, PROCFILE_FLAG_DEFAULT);
        if(!ff) return;

        ff = procfile_readall(ff);
        if(!ff) return;

        unsigned long i, lines = procfile_lines(ff);

        if(lines < 1) {
            error("File '%s' should have 1+ lines.", io->filename);
            return;
        }

        io->Read = 0;
        io->Write = 0;
/*
        io->Sync = 0;
        io->Async = 0;
        io->Total = 0;
*/

        for(i = 0; i < lines ; i++) {
            char *s = procfile_lineword(ff, i, 1);
            uint32_t hash = simple_hash(s);

            if(hash == Read_hash && !strcmp(s, "Read"))
                io->Read += strtoull(procfile_lineword(ff, i, 2), NULL, 10);

            else if(hash == Write_hash && !strcmp(s, "Write"))
                io->Write += strtoull(procfile_lineword(ff, i, 2), NULL, 10);

/*
            else if(hash == Sync_hash && !strcmp(s, "Sync"))
                io->Sync += strtoull(procfile_lineword(ff, i, 2), NULL, 10);

            else if(hash == Async_hash && !strcmp(s, "Async"))
                io->Async += strtoull(procfile_lineword(ff, i, 2), NULL, 10);

            else if(hash == Total_hash && !strcmp(s, "Total"))
                io->Total += strtoull(procfile_lineword(ff, i, 2), NULL, 10);
*/
        }

        io->updated = 1;
        // fprintf(stderr, "READ '%s': Read: %llu, Write: %llu, Sync: %llu, Async: %llu, Total: %llu\n", io->filename, io->Read, io->Write, io->Sync, io->Async, io->Total);
    }
}

void cgroup_read_memory(struct memory *mem) {
    static procfile *ff = NULL;

    static uint32_t cache_hash = 0;
    static uint32_t rss_hash = 0;
    static uint32_t rss_huge_hash = 0;
    static uint32_t mapped_file_hash = 0;
    static uint32_t writeback_hash = 0;
    static uint32_t dirty_hash = 0;
    static uint32_t swap_hash = 0;
    static uint32_t pgpgin_hash = 0;
    static uint32_t pgpgout_hash = 0;
    static uint32_t pgfault_hash = 0;
    static uint32_t pgmajfault_hash = 0;
/*
    static uint32_t inactive_anon_hash = 0;
    static uint32_t active_anon_hash = 0;
    static uint32_t inactive_file_hash = 0;
    static uint32_t active_file_hash = 0;
    static uint32_t unevictable_hash = 0;
    static uint32_t hierarchical_memory_limit_hash = 0;
    static uint32_t total_cache_hash = 0;
    static uint32_t total_rss_hash = 0;
    static uint32_t total_rss_huge_hash = 0;
    static uint32_t total_mapped_file_hash = 0;
    static uint32_t total_writeback_hash = 0;
    static uint32_t total_dirty_hash = 0;
    static uint32_t total_swap_hash = 0;
    static uint32_t total_pgpgin_hash = 0;
    static uint32_t total_pgpgout_hash = 0;
    static uint32_t total_pgfault_hash = 0;
    static uint32_t total_pgmajfault_hash = 0;
    static uint32_t total_inactive_anon_hash = 0;
    static uint32_t total_active_anon_hash = 0;
    static uint32_t total_inactive_file_hash = 0;
    static uint32_t total_active_file_hash = 0;
    static uint32_t total_unevictable_hash = 0;
*/
    if(unlikely(cache_hash == 0)) {
        cache_hash = simple_hash("cache");
        rss_hash = simple_hash("rss");
        rss_huge_hash = simple_hash("rss_huge");
        mapped_file_hash = simple_hash("mapped_file");
        writeback_hash = simple_hash("writeback");
        dirty_hash = simple_hash("dirty");
        swap_hash = simple_hash("swap");
        pgpgin_hash = simple_hash("pgpgin");
        pgpgout_hash = simple_hash("pgpgout");
        pgfault_hash = simple_hash("pgfault");
        pgmajfault_hash = simple_hash("pgmajfault");
/*
        inactive_anon_hash = simple_hash("inactive_anon");
        active_anon_hash = simple_hash("active_anon");
        inactive_file_hash = simple_hash("inactive_file");
        active_file_hash = simple_hash("active_file");
        unevictable_hash = simple_hash("unevictable");
        hierarchical_memory_limit_hash = simple_hash("hierarchical_memory_limit");
        total_cache_hash = simple_hash("total_cache");
        total_rss_hash = simple_hash("total_rss");
        total_rss_huge_hash = simple_hash("total_rss_huge");
        total_mapped_file_hash = simple_hash("total_mapped_file");
        total_writeback_hash = simple_hash("total_writeback");
        total_dirty_hash = simple_hash("total_dirty");
        total_swap_hash = simple_hash("total_swap");
        total_pgpgin_hash = simple_hash("total_pgpgin");
        total_pgpgout_hash = simple_hash("total_pgpgout");
        total_pgfault_hash = simple_hash("total_pgfault");
        total_pgmajfault_hash = simple_hash("total_pgmajfault");
        total_inactive_anon_hash = simple_hash("total_inactive_anon");
        total_active_anon_hash = simple_hash("total_active_anon");
        total_inactive_file_hash = simple_hash("total_inactive_file");
        total_active_file_hash = simple_hash("total_active_file");
        total_unevictable_hash = simple_hash("total_unevictable");
*/
    }

    mem->updated = 0;
    if(mem->filename) {
        ff = procfile_reopen(ff, mem->filename, NULL, PROCFILE_FLAG_DEFAULT);
        if(!ff) return;

        ff = procfile_readall(ff);
        if(!ff) return;

        unsigned long i, lines = procfile_lines(ff);

        if(lines < 1) {
            error("File '%s' should have 1+ lines.", mem->filename);
            return;
        }

        for(i = 0; i < lines ; i++) {
            char *s = procfile_lineword(ff, i, 0);
            uint32_t hash = simple_hash(s);

            if(hash == cache_hash && !strcmp(s, "cache"))
                mem->cache = strtoull(procfile_lineword(ff, i, 1), NULL, 10);

            else if(hash == rss_hash && !strcmp(s, "rss"))
                mem->rss = strtoull(procfile_lineword(ff, i, 1), NULL, 10);

            else if(hash == rss_huge_hash && !strcmp(s, "rss_huge"))
                mem->rss_huge = strtoull(procfile_lineword(ff, i, 1), NULL, 10);

            else if(hash == mapped_file_hash && !strcmp(s, "mapped_file"))
                mem->mapped_file = strtoull(procfile_lineword(ff, i, 1), NULL, 10);

            else if(hash == writeback_hash && !strcmp(s, "writeback"))
                mem->writeback = strtoull(procfile_lineword(ff, i, 1), NULL, 10);

            else if(hash == dirty_hash && !strcmp(s, "dirty")) {
                mem->dirty = strtoull(procfile_lineword(ff, i, 1), NULL, 10);
                mem->has_dirty_swap = 1;
            }

            else if(hash == swap_hash && !strcmp(s, "swap")) {
                mem->swap = strtoull(procfile_lineword(ff, i, 1), NULL, 10);
                mem->has_dirty_swap = 1;
            }

            else if(hash == pgpgin_hash && !strcmp(s, "pgpgin"))
                mem->pgpgin = strtoull(procfile_lineword(ff, i, 1), NULL, 10);

            else if(hash == pgpgout_hash && !strcmp(s, "pgpgout"))
                mem->pgpgout = strtoull(procfile_lineword(ff, i, 1), NULL, 10);

            else if(hash == pgfault_hash && !strcmp(s, "pgfault"))
                mem->pgfault = strtoull(procfile_lineword(ff, i, 1), NULL, 10);

            else if(hash == pgmajfault_hash && !strcmp(s, "pgmajfault"))
                mem->pgmajfault = strtoull(procfile_lineword(ff, i, 1), NULL, 10);

/*
            else if(hash == inactive_anon_hash && !strcmp(s, "inactive_anon"))
                mem->inactive_anon = strtoull(procfile_lineword(ff, i, 1), NULL, 10);

            else if(hash == active_anon_hash && !strcmp(s, "active_anon"))
                mem->active_anon = strtoull(procfile_lineword(ff, i, 1), NULL, 10);

            else if(hash == inactive_file_hash && !strcmp(s, "inactive_file"))
                mem->inactive_file = strtoull(procfile_lineword(ff, i, 1), NULL, 10);

            else if(hash == active_file_hash && !strcmp(s, "active_file"))
                mem->active_file = strtoull(procfile_lineword(ff, i, 1), NULL, 10);

            else if(hash == unevictable_hash && !strcmp(s, "unevictable"))
                mem->unevictable = strtoull(procfile_lineword(ff, i, 1), NULL, 10);

            else if(hash == hierarchical_memory_limit_hash && !strcmp(s, "hierarchical_memory_limit"))
                mem->hierarchical_memory_limit = strtoull(procfile_lineword(ff, i, 1), NULL, 10);

            else if(hash == total_cache_hash && !strcmp(s, "total_cache"))
                mem->total_cache = strtoull(procfile_lineword(ff, i, 1), NULL, 10);

            else if(hash == total_rss_hash && !strcmp(s, "total_rss"))
                mem->total_rss = strtoull(procfile_lineword(ff, i, 1), NULL, 10);

            else if(hash == total_rss_huge_hash && !strcmp(s, "total_rss_huge"))
                mem->total_rss_huge = strtoull(procfile_lineword(ff, i, 1), NULL, 10);

            else if(hash == total_mapped_file_hash && !strcmp(s, "total_mapped_file"))
                mem->total_mapped_file = strtoull(procfile_lineword(ff, i, 1), NULL, 10);

            else if(hash == total_writeback_hash && !strcmp(s, "total_writeback"))
                mem->total_writeback = strtoull(procfile_lineword(ff, i, 1), NULL, 10);

            else if(hash == total_dirty_hash && !strcmp(s, "total_dirty"))
                mem->total_dirty = strtoull(procfile_lineword(ff, i, 1), NULL, 10);

            else if(hash == total_swap_hash && !strcmp(s, "total_swap"))
                mem->total_swap = strtoull(procfile_lineword(ff, i, 1), NULL, 10);

            else if(hash == total_pgpgin_hash && !strcmp(s, "total_pgpgin"))
                mem->total_pgpgin = strtoull(procfile_lineword(ff, i, 1), NULL, 10);

            else if(hash == total_pgpgout_hash && !strcmp(s, "total_pgpgout"))
                mem->total_pgpgout = strtoull(procfile_lineword(ff, i, 1), NULL, 10);

            else if(hash == total_pgfault_hash && !strcmp(s, "total_pgfault"))
                mem->total_pgfault = strtoull(procfile_lineword(ff, i, 1), NULL, 10);

            else if(hash == total_pgmajfault_hash && !strcmp(s, "total_pgmajfault"))
                mem->total_pgmajfault = strtoull(procfile_lineword(ff, i, 1), NULL, 10);

            else if(hash == total_inactive_anon_hash && !strcmp(s, "total_inactive_anon"))
                mem->total_inactive_anon = strtoull(procfile_lineword(ff, i, 1), NULL, 10);

            else if(hash == total_active_anon_hash && !strcmp(s, "total_active_anon"))
                mem->total_active_anon = strtoull(procfile_lineword(ff, i, 1), NULL, 10);

            else if(hash == total_inactive_file_hash && !strcmp(s, "total_inactive_file"))
                mem->total_inactive_file = strtoull(procfile_lineword(ff, i, 1), NULL, 10);

            else if(hash == total_active_file_hash && !strcmp(s, "total_active_file"))
                mem->total_active_file = strtoull(procfile_lineword(ff, i, 1), NULL, 10);

            else if(hash == total_unevictable_hash && !strcmp(s, "total_unevictable"))
                mem->total_unevictable = strtoull(procfile_lineword(ff, i, 1), NULL, 10);
*/
        }

        // fprintf(stderr, "READ: '%s', cache: %llu, rss: %llu, rss_huge: %llu, mapped_file: %llu, writeback: %llu, dirty: %llu, swap: %llu, pgpgin: %llu, pgpgout: %llu, pgfault: %llu, pgmajfault: %llu, inactive_anon: %llu, active_anon: %llu, inactive_file: %llu, active_file: %llu, unevictable: %llu, hierarchical_memory_limit: %llu, total_cache: %llu, total_rss: %llu, total_rss_huge: %llu, total_mapped_file: %llu, total_writeback: %llu, total_dirty: %llu, total_swap: %llu, total_pgpgin: %llu, total_pgpgout: %llu, total_pgfault: %llu, total_pgmajfault: %llu, total_inactive_anon: %llu, total_active_anon: %llu, total_inactive_file: %llu, total_active_file: %llu, total_unevictable: %llu\n", mem->filename, mem->cache, mem->rss, mem->rss_huge, mem->mapped_file, mem->writeback, mem->dirty, mem->swap, mem->pgpgin, mem->pgpgout, mem->pgfault, mem->pgmajfault, mem->inactive_anon, mem->active_anon, mem->inactive_file, mem->active_file, mem->unevictable, mem->hierarchical_memory_limit, mem->total_cache, mem->total_rss, mem->total_rss_huge, mem->total_mapped_file, mem->total_writeback, mem->total_dirty, mem->total_swap, mem->total_pgpgin, mem->total_pgpgout, mem->total_pgfault, mem->total_pgmajfault, mem->total_inactive_anon, mem->total_active_anon, mem->total_inactive_file, mem->total_active_file, mem->total_unevictable);

        mem->updated = 1;
    }

    mem->usage_in_bytes_updated = 0;
    if(mem->filename_usage_in_bytes) {
        if(likely(!read_single_number_file(mem->filename_usage_in_bytes, &mem->usage_in_bytes)))
            mem->usage_in_bytes_updated = 1;
    }

    mem->msw_usage_in_bytes_updated = 0;
    if(mem->filename_msw_usage_in_bytes) {
        if(likely(!read_single_number_file(mem->filename_msw_usage_in_bytes, &mem->msw_usage_in_bytes)))
            mem->msw_usage_in_bytes_updated = 1;
    }

    mem->failcnt_updated = 0;
    if(mem->filename_failcnt) {
        if(likely(!read_single_number_file(mem->filename_failcnt, &mem->failcnt)))
            mem->failcnt_updated = 1;
    }
}

void cgroup_read(struct cgroup *cg) {
    debug(D_CGROUP, "reading metrics for cgroups '%s'", cg->id);

    cgroup_read_cpuacct_stat(&cg->cpuacct_stat);
    cgroup_read_cpuacct_usage(&cg->cpuacct_usage);
    cgroup_read_memory(&cg->memory);
    cgroup_read_blkio(&cg->io_service_bytes);
    cgroup_read_blkio(&cg->io_serviced);
    cgroup_read_blkio(&cg->throttle_io_service_bytes);
    cgroup_read_blkio(&cg->throttle_io_serviced);
    cgroup_read_blkio(&cg->io_merged);
    cgroup_read_blkio(&cg->io_queued);
}

void read_all_cgroups(struct cgroup *root) {
    debug(D_CGROUP, "reading metrics for all cgroups");

    struct cgroup *cg;

    for(cg = root; cg ; cg = cg->next)
        if(cg->enabled && cg->available)
            cgroup_read(cg);
}

// ----------------------------------------------------------------------------
// add/remove/find cgroup objects

#define CGROUP_CHARTID_LINE_MAX 1024

static char *cgroup_title_strdupz(const char *s) {
    if(!s || !*s) s = "/";

    if(*s == '/' && s[1] != '\0') s++;

    char *r = strdupz(s);
    netdata_fix_chart_name(r);

    return r;
}

static char *cgroup_chart_id_strdupz(const char *s) {
    if(!s || !*s) s = "/";

    if(*s == '/' && s[1] != '\0') s++;

    char *r = strdupz(s);
    netdata_fix_chart_id(r);

    return r;
}

void cgroup_get_chart_name(struct cgroup *cg) {
    debug(D_CGROUP, "looking for the name of cgroup '%s' with chart id '%s' and title '%s'", cg->id, cg->chart_id, cg->chart_title);

    if(!netdata_simple_pattern_list_matches(disabled_cgroup_renames, cg->id) &&
            !netdata_simple_pattern_list_matches(disabled_cgroup_renames, cg->chart_id)) {

        pid_t cgroup_pid;
        char buffer[CGROUP_CHARTID_LINE_MAX + 1];

        snprintfz(buffer, CGROUP_CHARTID_LINE_MAX, "exec %s '%s'", cgroups_rename_script, cg->chart_id);

        debug(D_CGROUP, "executing command '%s' for cgroup '%s'", buffer, cg->id);
        FILE *fp = mypopen(buffer, &cgroup_pid);
        if(fp) {
            // debug(D_CGROUP, "reading from command '%s' for cgroup '%s'", buffer, cg->id);
            char *s = fgets(buffer, CGROUP_CHARTID_LINE_MAX, fp);
            // debug(D_CGROUP, "closing command for cgroup '%s'", cg->id);
            mypclose(fp, cgroup_pid);
            // debug(D_CGROUP, "closed command for cgroup '%s'", cg->id);

            if(s && *s && *s != '\n') {
                debug(D_CGROUP, "cgroup '%s' should be renamed to '%s'", cg->id, s);

                trim(s);

                freez(cg->chart_title);
                cg->chart_title = cgroup_title_strdupz(s);

                freez(cg->chart_id);
                cg->chart_id = cgroup_chart_id_strdupz(s);
                cg->hash_chart = simple_hash(cg->chart_id);
            }
        }
        else
            error("CGROUP: Cannot popen(\"%s\", \"r\").", buffer);

        debug(D_CGROUP, "cgroup '%s' renamed to '%s' (title: '%s')", cg->id, cg->chart_id, cg->chart_title);
    }
    else
        debug(D_CGROUP, "cgroup '%s' will not be renamed - it matches the list of disabled cgroup renames (will be shown as '%s')", cg->id, cg->chart_id);


    if(netdata_simple_pattern_list_matches(systemd_services_cgroups, cg->id) ||
            netdata_simple_pattern_list_matches(systemd_services_cgroups, cg->chart_id)) {
        debug(D_CGROUP, "cgroup '%s' with chart id '%s' (title: '%s') matches systemd services cgroups", cg->id, cg->chart_id, cg->chart_title);

        char buffer[CGROUP_CHARTID_LINE_MAX + 1];
        cg->options |= CGROUP_OPTIONS_SYSTEM_SLICE_SERVICE;

        strncpy(buffer, cg->id, CGROUP_CHARTID_LINE_MAX);
        char *s = buffer;

        // skip to the last slash
        size_t len = strlen(s);
        while(len--) if(unlikely(s[len] == '/')) break;
        if(len) s = &s[len+1];

        // remove extension
        //len = strlen(s);
        //while(len--) if(unlikely(s[len] == '.')) break;
        //if(len) s[len] = '\0';

        freez(cg->chart_title);
        cg->chart_title = cgroup_title_strdupz(s);

        freez(cg->chart_id);
        cg->chart_id = cgroup_chart_id_strdupz(s);
        cg->hash_chart = simple_hash(cg->chart_id);

        debug(D_CGROUP, "cgroup '%s' renamed to '%s' (title: '%s')", cg->id, cg->chart_id, cg->chart_title);
    }
    else
        debug(D_CGROUP, "cgroup '%s' with chart id '%s' (title: '%s') does not match systemd services groups", cg->id, cg->chart_id, cg->chart_title);
}

struct cgroup *cgroup_add(const char *id) {
    if(!id || !*id) id = "/";
    debug(D_CGROUP, "adding to list, cgroup with id '%s'", id);

    if(cgroup_root_count >= cgroup_root_max) {
        info("Maximum number of cgroups reached (%d). Not adding cgroup '%s'", cgroup_root_count, id);
        return NULL;
    }

    int def = netdata_simple_pattern_list_matches(disabled_cgroups_patterns, id)?0:cgroup_enable_new_cgroups_detected_at_runtime;
    struct cgroup *cg = callocz(1, sizeof(struct cgroup));

    cg->id = strdupz(id);
    cg->hash = simple_hash(cg->id);

    cg->chart_title = cgroup_title_strdupz(id);

    cg->chart_id = cgroup_chart_id_strdupz(id);
    cg->hash_chart = simple_hash(cg->chart_id);

    if(!cgroup_root)
        cgroup_root = cg;
    else {
        // append it
        struct cgroup *e;
        for(e = cgroup_root; e->next ;e = e->next) ;
        e->next = cg;
    }

    cgroup_root_count++;

    // fix the name by calling the external script
    cgroup_get_chart_name(cg);

    char option[FILENAME_MAX + 1];
    snprintfz(option, FILENAME_MAX, "enable cgroup %s", cg->chart_title);
    cg->enabled = (char)config_get_boolean("plugin:cgroups", option, def);

    if(cg->enabled) {
        struct cgroup *t;
        for (t = cgroup_root; t; t = t->next) {
            if (t != cg && t->enabled && t->hash_chart == cg->hash_chart && !strcmp(t->chart_id, cg->chart_id)) {
                if (!strncmp(t->chart_id, "/system.slice/", 14) && !strncmp(cg->chart_id, "/init.scope/system.slice/", 25)) {
                    error("Control group with chart id '%s' already exists with id '%s' and is enabled. Swapping them by enabling cgroup with id '%s' and disabling cgroup with id '%s'.",
                          cg->chart_id, t->id, cg->id, t->id);
                    debug(D_CGROUP, "Control group with chart id '%s' already exists with id '%s' and is enabled. Swapping them by enabling cgroup with id '%s' and disabling cgroup with id '%s'.",
                          cg->chart_id, t->id, cg->id, t->id);
                    t->enabled = 0;
                    t->options |= CGROUP_OPTIONS_DISABLED_DUPLICATE;
                }
                else {
                    error("Control group with chart id '%s' already exists with id '%s' and is enabled and available. Disabling cgroup with id '%s'.",
                          cg->chart_id, t->id, cg->id);
                    debug(D_CGROUP, "Control group with chart id '%s' already exists with id '%s' and is enabled and available. Disabling cgroup with id '%s'.",
                          cg->chart_id, t->id, cg->id);
                    cg->enabled = 0;
                    cg->options |= CGROUP_OPTIONS_DISABLED_DUPLICATE;
                }

                break;
            }
        }
    }

    debug(D_CGROUP, "ADDED CGROUP: '%s' with chart id '%s' and title '%s' as %s (default was %s)", cg->id, cg->chart_id, cg->chart_title, (cg->enabled)?"enabled":"disabled", (def)?"enabled":"disabled");

    return cg;
}

void cgroup_free(struct cgroup *cg) {
    debug(D_CGROUP, "Removing cgroup '%s' with chart id '%s' (was %s and %s)", cg->id, cg->chart_id, (cg->enabled)?"enabled":"disabled", (cg->available)?"available":"not available");

    freez(cg->cpuacct_usage.cpu_percpu);

    freez(cg->cpuacct_stat.filename);
    freez(cg->cpuacct_usage.filename);

    freez(cg->memory.filename);
    freez(cg->memory.filename_failcnt);
    freez(cg->memory.filename_usage_in_bytes);
    freez(cg->memory.filename_msw_usage_in_bytes);

    freez(cg->io_service_bytes.filename);
    freez(cg->io_serviced.filename);

    freez(cg->throttle_io_service_bytes.filename);
    freez(cg->throttle_io_serviced.filename);

    freez(cg->io_merged.filename);
    freez(cg->io_queued.filename);

    freez(cg->id);
    freez(cg->chart_id);
    freez(cg->chart_title);

    freez(cg);

    cgroup_root_count--;
}

// find if a given cgroup exists
struct cgroup *cgroup_find(const char *id) {
    debug(D_CGROUP, "searching for cgroup '%s'", id);

    uint32_t hash = simple_hash(id);

    struct cgroup *cg;
    for(cg = cgroup_root; cg ; cg = cg->next) {
        if(hash == cg->hash && strcmp(id, cg->id) == 0)
            break;
    }

    debug(D_CGROUP, "cgroup '%s' %s in memory", id, (cg)?"found":"not found");
    return cg;
}

// ----------------------------------------------------------------------------
// detect running cgroups

// callback for find_file_in_subdirs()
void found_subdir_in_dir(const char *dir) {
    debug(D_CGROUP, "examining cgroup dir '%s'", dir);

    struct cgroup *cg = cgroup_find(dir);
    if(!cg) {
        if(*dir && cgroup_max_depth > 0) {
            int depth = 0;
            const char *s;

            for(s = dir; *s ;s++)
                if(unlikely(*s == '/'))
                    depth++;

            if(depth > cgroup_max_depth) {
                info("cgroup '%s' is too deep (%d, while max is %d)", dir, depth, cgroup_max_depth);
                return;
            }
        }
        // debug(D_CGROUP, "will add dir '%s' as cgroup", dir);
        cg = cgroup_add(dir);
    }

    if(cg) cg->available = 1;
}

int find_dir_in_subdirs(const char *base, const char *this, void (*callback)(const char *)) {
    if(!this) this = base;
    debug(D_CGROUP, "searching for directories in '%s' (base '%s')", this?this:"", base);

    size_t dirlen = strlen(this), baselen = strlen(base);

    int ret = -1;
    int enabled = -1;

    const char *relative_path = &this[baselen];
    if(!*relative_path) relative_path = "/";

    DIR *dir = opendir(this);
    if(!dir) {
        error("Cannot read cgroups directory '%s'", base);
        return ret;
    }
    ret = 1;

    callback(relative_path);

    struct dirent *de = NULL;
    while((de = readdir(dir))) {
        if(de->d_type == DT_DIR
            && (
                (de->d_name[0] == '.' && de->d_name[1] == '\0')
                || (de->d_name[0] == '.' && de->d_name[1] == '.' && de->d_name[2] == '\0')
                ))
            continue;

        if(de->d_type == DT_DIR) {
            if(enabled == -1) {
                const char *r = relative_path;
                if(*r == '\0') r = "/";

                // do not decent in directories we are not interested
                int def = 1;
                if(netdata_simple_pattern_list_matches(disabled_cgroup_paths, r))
                    def = 0;

                // we check for this option here
                // so that the config will not have settings
                // for leaf directories
                char option[FILENAME_MAX + 1];
                snprintfz(option, FILENAME_MAX, "search for cgroups under %s", r);
                option[FILENAME_MAX] = '\0';
                enabled = config_get_boolean("plugin:cgroups", option, def);
            }

            if(enabled) {
                char *s = mallocz(dirlen + strlen(de->d_name) + 2);
                strcpy(s, this);
                strcat(s, "/");
                strcat(s, de->d_name);
                int ret2 = find_dir_in_subdirs(base, s, callback);
                if(ret2 > 0) ret += ret2;
                freez(s);
            }
        }
    }

    closedir(dir);
    return ret;
}

void mark_all_cgroups_as_not_available() {
    debug(D_CGROUP, "marking all cgroups as not available");

    struct cgroup *cg;

    // mark all as not available
    for(cg = cgroup_root; cg ; cg = cg->next) {
        cg->available = 0;
    }
}

void cleanup_all_cgroups() {
    struct cgroup *cg = cgroup_root, *last = NULL;

    for(; cg ;) {
        if(!cg->available) {
            // enable the first duplicate cgroup
            {
                struct cgroup *t;
                for(t = cgroup_root; t ; t = t->next) {
                    if(t != cg && t->available && !t->enabled && t->options & CGROUP_OPTIONS_DISABLED_DUPLICATE && t->hash_chart == cg->hash_chart && !strcmp(t->chart_id, cg->chart_id)) {
                        debug(D_CGROUP, "Enabling duplicate of cgroup '%s' with id '%s', because the original with id '%s' stopped.", t->chart_id, t->id, cg->id);
                        t->enabled = 1;
                        t->options &= ~CGROUP_OPTIONS_DISABLED_DUPLICATE;
                        break;
                    }
                }
            }

            if(!last)
                cgroup_root = cg->next;
            else
                last->next = cg->next;

            cgroup_free(cg);

            if(!last)
                cg = cgroup_root;
            else
                cg = last->next;
        }
        else {
            last = cg;
            cg = cg->next;
        }
    }
}

void find_all_cgroups() {
    debug(D_CGROUP, "searching for cgroups");

    mark_all_cgroups_as_not_available();

    if(cgroup_enable_cpuacct_stat || cgroup_enable_cpuacct_usage) {
        if (find_dir_in_subdirs(cgroup_cpuacct_base, NULL, found_subdir_in_dir) == -1) {
            cgroup_enable_cpuacct_stat = cgroup_enable_cpuacct_usage = 0;
            error("disabled cgroup cpu statistics.");
        }
    }

    if(cgroup_enable_blkio) {
        if (find_dir_in_subdirs(cgroup_blkio_base, NULL, found_subdir_in_dir) == -1) {
            cgroup_enable_blkio = 0;
            error("disabled cgroup blkio statistics.");
        }
    }

    if(cgroup_enable_memory) {
        if(find_dir_in_subdirs(cgroup_memory_base, NULL, found_subdir_in_dir) == -1) {
            cgroup_enable_memory = 0;
            error("disabled cgroup memory statistics.");
        }
    }

    if(cgroup_enable_devices) {
        if(find_dir_in_subdirs(cgroup_devices_base, NULL, found_subdir_in_dir) == -1) {
            cgroup_enable_devices = 0;
            error("disabled cgroup devices statistics.");
        }
    }

    // remove any non-existing cgroups
    cleanup_all_cgroups();

    struct cgroup *cg;
    struct stat buf;
    for(cg = cgroup_root; cg ; cg = cg->next) {
        // fprintf(stderr, " >>> CGROUP '%s' (%u - %s) with name '%s'\n", cg->id, cg->hash, cg->available?"available":"stopped", cg->name);

        if(unlikely(!cg->available))
            continue;

        debug(D_CGROUP, "checking paths for cgroup '%s'", cg->id);

        // check for newly added cgroups
        // and update the filenames they read
        char filename[FILENAME_MAX + 1];
        if(unlikely(cgroup_enable_cpuacct_stat && !cg->cpuacct_stat.filename)) {
            snprintfz(filename, FILENAME_MAX, "%s%s/cpuacct.stat", cgroup_cpuacct_base, cg->id);
            if(stat(filename, &buf) != -1) {
                cg->cpuacct_stat.filename = strdupz(filename);
                debug(D_CGROUP, "cpuacct.stat filename for cgroup '%s': '%s'", cg->id, cg->cpuacct_stat.filename);
            }
            else debug(D_CGROUP, "cpuacct.stat file for cgroup '%s': '%s' does not exist.", cg->id, filename);
        }

        if(unlikely(cgroup_enable_cpuacct_usage && !cg->cpuacct_usage.filename)) {
            snprintfz(filename, FILENAME_MAX, "%s%s/cpuacct.usage_percpu", cgroup_cpuacct_base, cg->id);
            if(stat(filename, &buf) != -1) {
                cg->cpuacct_usage.filename = strdupz(filename);
                debug(D_CGROUP, "cpuacct.usage_percpu filename for cgroup '%s': '%s'", cg->id, cg->cpuacct_usage.filename);
            }
            else debug(D_CGROUP, "cpuacct.usage_percpu file for cgroup '%s': '%s' does not exist.", cg->id, filename);
        }

        if(unlikely(cgroup_enable_memory)) {
            if(unlikely(!cg->memory.filename)) {
                snprintfz(filename, FILENAME_MAX, "%s%s/memory.stat", cgroup_memory_base, cg->id);
                if(stat(filename, &buf) != -1) {
                    cg->memory.filename = strdupz(filename);
                    debug(D_CGROUP, "memory.stat filename for cgroup '%s': '%s'", cg->id, cg->memory.filename);
                }
                else
                    debug(D_CGROUP, "memory.stat file for cgroup '%s': '%s' does not exist.", cg->id, filename);
            }
            if(unlikely(!cg->memory.filename_usage_in_bytes)) {
                snprintfz(filename, FILENAME_MAX, "%s%s/memory.usage_in_bytes", cgroup_memory_base, cg->id);
                if(stat(filename, &buf) != -1) {
                    cg->memory.filename_usage_in_bytes = strdupz(filename);
                    debug(D_CGROUP, "memory.usage_in_bytes filename for cgroup '%s': '%s'", cg->id, cg->memory.filename_usage_in_bytes);
                }
                else
                    debug(D_CGROUP, "memory.usage_in_bytes file for cgroup '%s': '%s' does not exist.", cg->id, filename);
            }
            if(unlikely(!cg->memory.filename_msw_usage_in_bytes)) {
                snprintfz(filename, FILENAME_MAX, "%s%s/memory.msw_usage_in_bytes", cgroup_memory_base, cg->id);
                if(stat(filename, &buf) != -1) {
                    cg->memory.filename_msw_usage_in_bytes = strdupz(filename);
                    debug(D_CGROUP, "memory.msw_usage_in_bytes filename for cgroup '%s': '%s'", cg->id, cg->memory.filename_msw_usage_in_bytes);
                }
                else
                    debug(D_CGROUP, "memory.msw_usage_in_bytes file for cgroup '%s': '%s' does not exist.", cg->id, filename);
            }
            if(unlikely(!cg->memory.filename_failcnt)) {
                snprintfz(filename, FILENAME_MAX, "%s%s/memory.failcnt", cgroup_memory_base, cg->id);
                if(stat(filename, &buf) != -1) {
                    cg->memory.filename_failcnt = strdupz(filename);
                    debug(D_CGROUP, "memory.failcnt filename for cgroup '%s': '%s'", cg->id, cg->memory.filename_failcnt);
                }
                else
                    debug(D_CGROUP, "memory.failcnt file for cgroup '%s': '%s' does not exist.", cg->id, filename);
            }
        }

        if(unlikely(cgroup_enable_blkio)) {
            if(unlikely(!cg->io_service_bytes.filename)) {
                snprintfz(filename, FILENAME_MAX, "%s%s/blkio.io_service_bytes", cgroup_blkio_base, cg->id);
                if(stat(filename, &buf) != -1) {
                    cg->io_service_bytes.filename = strdupz(filename);
                    debug(D_CGROUP, "io_service_bytes filename for cgroup '%s': '%s'", cg->id, cg->io_service_bytes.filename);
                }
                else debug(D_CGROUP, "io_service_bytes file for cgroup '%s': '%s' does not exist.", cg->id, filename);
            }
            if(unlikely(!cg->io_serviced.filename)) {
                snprintfz(filename, FILENAME_MAX, "%s%s/blkio.io_serviced", cgroup_blkio_base, cg->id);
                if(stat(filename, &buf) != -1) {
                    cg->io_serviced.filename = strdupz(filename);
                    debug(D_CGROUP, "io_serviced filename for cgroup '%s': '%s'", cg->id, cg->io_serviced.filename);
                }
                else debug(D_CGROUP, "io_serviced file for cgroup '%s': '%s' does not exist.", cg->id, filename);
            }
            if(unlikely(!cg->throttle_io_service_bytes.filename)) {
                snprintfz(filename, FILENAME_MAX, "%s%s/blkio.throttle.io_service_bytes", cgroup_blkio_base, cg->id);
                if(stat(filename, &buf) != -1) {
                    cg->throttle_io_service_bytes.filename = strdupz(filename);
                    debug(D_CGROUP, "throttle_io_service_bytes filename for cgroup '%s': '%s'", cg->id, cg->throttle_io_service_bytes.filename);
                }
                else debug(D_CGROUP, "throttle_io_service_bytes file for cgroup '%s': '%s' does not exist.", cg->id, filename);
            }
            if(unlikely(!cg->throttle_io_serviced.filename)) {
                snprintfz(filename, FILENAME_MAX, "%s%s/blkio.throttle.io_serviced", cgroup_blkio_base, cg->id);
                if(stat(filename, &buf) != -1) {
                    cg->throttle_io_serviced.filename = strdupz(filename);
                    debug(D_CGROUP, "throttle_io_serviced filename for cgroup '%s': '%s'", cg->id, cg->throttle_io_serviced.filename);
                }
                else debug(D_CGROUP, "throttle_io_serviced file for cgroup '%s': '%s' does not exist.", cg->id, filename);
            }
            if(unlikely(!cg->io_merged.filename)) {
                snprintfz(filename, FILENAME_MAX, "%s%s/blkio.io_merged", cgroup_blkio_base, cg->id);
                if(stat(filename, &buf) != -1) {
                    cg->io_merged.filename = strdupz(filename);
                    debug(D_CGROUP, "io_merged filename for cgroup '%s': '%s'", cg->id, cg->io_merged.filename);
                }
                else debug(D_CGROUP, "io_merged file for cgroup '%s': '%s' does not exist.", cg->id, filename);
            }
            if(unlikely(!cg->io_queued.filename)) {
                snprintfz(filename, FILENAME_MAX, "%s%s/blkio.io_queued", cgroup_blkio_base, cg->id);
                if(stat(filename, &buf) != -1) {
                    cg->io_queued.filename = strdupz(filename);
                    debug(D_CGROUP, "io_queued filename for cgroup '%s': '%s'", cg->id, cg->io_queued.filename);
                }
                else debug(D_CGROUP, "io_queued file for cgroup '%s': '%s' does not exist.", cg->id, filename);
            }
        }
    }

    debug(D_CGROUP, "done searching for cgroups");
    return;
}

// ----------------------------------------------------------------------------
// generate charts

#define CHART_TITLE_MAX 300

static inline char *cgroup_chart_type(char *buffer, const char *id, size_t len) {
    if(buffer[0]) return buffer;

    if(id[0] == '\0' || (id[0] == '/' && id[1] == '\0'))
        strncpy(buffer, "cgroup_root", len);
    else
        snprintfz(buffer, len, "cgroup_%s", id);

    netdata_fix_chart_id(buffer);
    return buffer;
}

void update_cgroup_charts(int update_every) {
    debug(D_CGROUP, "updating cgroups charts");

    char type[RRD_ID_LENGTH_MAX + 1];
    char title[CHART_TITLE_MAX + 1];

    struct cgroup *cg;

    for(cg = cgroup_root; cg ; cg = cg->next) {
        if(!cg->available || !cg->enabled || cg->options & CGROUP_OPTIONS_SYSTEM_SLICE_SERVICE)
            continue;

        type[0] = '\0';

        if(cg->cpuacct_stat.updated) {
            if(unlikely(!cg->st_cpu)) {
                cg->st_cpu = rrdset_find_bytype(cgroup_chart_type(type, cg->chart_id, RRD_ID_LENGTH_MAX), "cpu");
                if(!cg->st_cpu) {
                    snprintfz(title, CHART_TITLE_MAX, "CPU Usage (%d%% = %d core%s) for cgroup %s", (processors * 100), processors, (processors > 1) ? "s" : "", cg->chart_title);
                    cg->st_cpu = rrdset_create(type, "cpu", NULL, "cpu", "cgroup.cpu", title, "%", 40000, update_every, RRDSET_TYPE_STACKED);

                    rrddim_add(cg->st_cpu, "user", NULL, 100, hz, RRDDIM_INCREMENTAL);
                    rrddim_add(cg->st_cpu, "system", NULL, 100, hz, RRDDIM_INCREMENTAL);
                }
            }
            else rrdset_next(cg->st_cpu);

            rrddim_set(cg->st_cpu, "user", cg->cpuacct_stat.user);
            rrddim_set(cg->st_cpu, "system", cg->cpuacct_stat.system);
            rrdset_done(cg->st_cpu);
        }

        if(cg->cpuacct_usage.updated) {
            char id[RRD_ID_LENGTH_MAX + 1];
            unsigned int i;

            if(unlikely(!cg->st_cpu_per_core)) {
                cg->st_cpu_per_core = rrdset_find_bytype(cgroup_chart_type(type, cg->chart_id, RRD_ID_LENGTH_MAX), "cpu_per_core");
                if(!cg->st_cpu_per_core) {
                    snprintfz(title, CHART_TITLE_MAX, "CPU Usage (%d%% = %d core%s) Per Core for cgroup %s", (processors * 100), processors, (processors > 1) ? "s" : "", cg->chart_title);
                    cg->st_cpu_per_core = rrdset_create(type, "cpu_per_core", NULL, "cpu", "cgroup.cpu_per_core", title, "%", 40100, update_every, RRDSET_TYPE_STACKED);

                    for(i = 0; i < cg->cpuacct_usage.cpus; i++) {
                        snprintfz(id, CHART_TITLE_MAX, "cpu%u", i);
                        rrddim_add(cg->st_cpu_per_core, id, NULL, 100, 1000000000, RRDDIM_INCREMENTAL);
                    }
                }
            }
            else rrdset_next(cg->st_cpu_per_core);

            for(i = 0; i < cg->cpuacct_usage.cpus ;i++) {
                snprintfz(id, CHART_TITLE_MAX, "cpu%u", i);
                rrddim_set(cg->st_cpu_per_core, id, cg->cpuacct_usage.cpu_percpu[i]);
            }
            rrdset_done(cg->st_cpu_per_core);
        }

        if(cg->memory.updated) {
            if(cg->memory.cache + cg->memory.rss + cg->memory.rss_huge + cg->memory.mapped_file > 0) {
                if(unlikely(!cg->st_mem)) {
                    cg->st_mem = rrdset_find_bytype(cgroup_chart_type(type, cg->chart_id, RRD_ID_LENGTH_MAX), "mem");
                    if(!cg->st_mem) {
                        snprintfz(title, CHART_TITLE_MAX, "Memory Usage for cgroup %s", cg->chart_title);
                        cg->st_mem = rrdset_create(type, "mem", NULL, "mem", "cgroup.mem", title, "MB", 40210, update_every, RRDSET_TYPE_STACKED);

                        rrddim_add(cg->st_mem, "cache", NULL, 1, 1024 * 1024, RRDDIM_ABSOLUTE);
                        rrddim_add(cg->st_mem, "rss", NULL, 1, 1024 * 1024, RRDDIM_ABSOLUTE);
                        if(cg->memory.has_dirty_swap)
                            rrddim_add(cg->st_mem, "swap", NULL, 1, 1024 * 1024, RRDDIM_ABSOLUTE);
                        rrddim_add(cg->st_mem, "rss_huge", NULL, 1, 1024 * 1024, RRDDIM_ABSOLUTE);
                        rrddim_add(cg->st_mem, "mapped_file", NULL, 1, 1024 * 1024, RRDDIM_ABSOLUTE);
                    }
                }
                else rrdset_next(cg->st_mem);

                rrddim_set(cg->st_mem, "cache", cg->memory.cache);
                rrddim_set(cg->st_mem, "rss", cg->memory.rss);
                if(cg->memory.has_dirty_swap)
                    rrddim_set(cg->st_mem, "swap", cg->memory.swap);
                rrddim_set(cg->st_mem, "rss_huge", cg->memory.rss_huge);
                rrddim_set(cg->st_mem, "mapped_file", cg->memory.mapped_file);
                rrdset_done(cg->st_mem);
            }

            if(unlikely(!cg->st_writeback)) {
                cg->st_writeback = rrdset_find_bytype(cgroup_chart_type(type, cg->chart_id, RRD_ID_LENGTH_MAX), "writeback");
                if(!cg->st_writeback) {
                    snprintfz(title, CHART_TITLE_MAX, "Writeback Memory for cgroup %s", cg->chart_title);
                    cg->st_writeback = rrdset_create(type, "writeback", NULL, "mem", "cgroup.writeback", title, "MB", 40300, update_every, RRDSET_TYPE_AREA);

                    if(cg->memory.has_dirty_swap)
                        rrddim_add(cg->st_writeback, "dirty", NULL, 1, 1024 * 1024, RRDDIM_ABSOLUTE);
                    rrddim_add(cg->st_writeback, "writeback", NULL, 1, 1024 * 1024, RRDDIM_ABSOLUTE);
                }
            }
            else rrdset_next(cg->st_writeback);

            if(cg->memory.has_dirty_swap)
                rrddim_set(cg->st_writeback, "dirty", cg->memory.dirty);
            rrddim_set(cg->st_writeback, "writeback", cg->memory.writeback);
            rrdset_done(cg->st_writeback);

            if(cg->memory.pgpgin + cg->memory.pgpgout > 0) {
                if(unlikely(!cg->st_mem_activity)) {
                    cg->st_mem_activity = rrdset_find_bytype(cgroup_chart_type(type, cg->chart_id, RRD_ID_LENGTH_MAX), "mem_activity");
                    if(!cg->st_mem_activity) {
                        snprintfz(title, CHART_TITLE_MAX, "Memory Activity for cgroup %s", cg->chart_title);
                        cg->st_mem_activity = rrdset_create(type, "mem_activity", NULL, "mem", "cgroup.mem_activity", title, "MB/s", 40400, update_every, RRDSET_TYPE_LINE);

                        rrddim_add(cg->st_mem_activity, "pgpgin", "in", sysconf(_SC_PAGESIZE), 1024 * 1024, RRDDIM_INCREMENTAL);
                        rrddim_add(cg->st_mem_activity, "pgpgout", "out", -sysconf(_SC_PAGESIZE), 1024 * 1024, RRDDIM_INCREMENTAL);
                    }
                }
                else rrdset_next(cg->st_mem_activity);

                rrddim_set(cg->st_mem_activity, "pgpgin", cg->memory.pgpgin);
                rrddim_set(cg->st_mem_activity, "pgpgout", cg->memory.pgpgout);
                rrdset_done(cg->st_mem_activity);
            }

            if(cg->memory.pgfault + cg->memory.pgmajfault > 0) {
                if(unlikely(!cg->st_pgfaults)) {
                    cg->st_pgfaults = rrdset_find_bytype(cgroup_chart_type(type, cg->chart_id, RRD_ID_LENGTH_MAX), "pgfaults");
                    if(!cg->st_pgfaults) {
                        snprintfz(title, CHART_TITLE_MAX, "Memory Page Faults for cgroup %s", cg->chart_title);
                        cg->st_pgfaults = rrdset_create(type, "pgfaults", NULL, "mem", "cgroup.pgfaults", title, "MB/s", 40500, update_every, RRDSET_TYPE_LINE);

                        rrddim_add(cg->st_pgfaults, "pgfault", NULL, sysconf(_SC_PAGESIZE), 1024 * 1024, RRDDIM_INCREMENTAL);
                        rrddim_add(cg->st_pgfaults, "pgmajfault", "swap", -sysconf(_SC_PAGESIZE), 1024 * 1024, RRDDIM_INCREMENTAL);
                    }
                }
                else rrdset_next(cg->st_pgfaults);

                rrddim_set(cg->st_pgfaults, "pgfault", cg->memory.pgfault);
                rrddim_set(cg->st_pgfaults, "pgmajfault", cg->memory.pgmajfault);
                rrdset_done(cg->st_pgfaults);
            }
        }

        if(cg->memory.usage_in_bytes_updated) {
            if(unlikely(!cg->st_mem_usage)) {
                cg->st_mem_usage = rrdset_find_bytype(cgroup_chart_type(type, cg->chart_id, RRD_ID_LENGTH_MAX), "mem_usage");
                if(!cg->st_mem_usage) {
                    snprintfz(title, CHART_TITLE_MAX, "Total Memory for cgroup %s", cg->chart_title);
                    cg->st_mem_usage = rrdset_create(type, "mem_usage", NULL, "mem", "cgroup.mem_usage", title, "MB", 40200, update_every, RRDSET_TYPE_STACKED);

                    rrddim_add(cg->st_mem_usage, "ram", NULL, 1, 1024 * 1024, RRDDIM_ABSOLUTE);
                    rrddim_add(cg->st_mem_usage, "swap", NULL, 1, 1024 * 1024, RRDDIM_ABSOLUTE);
                }
            }
            else rrdset_next(cg->st_mem_usage);

            rrddim_set(cg->st_mem_usage, "ram", cg->memory.usage_in_bytes);
            rrddim_set(cg->st_mem_usage, "swap", (cg->memory.msw_usage_in_bytes > cg->memory.usage_in_bytes)?cg->memory.msw_usage_in_bytes - cg->memory.usage_in_bytes:0);
            rrdset_done(cg->st_mem_usage);
        }

        if(cg->memory.failcnt_updated && cg->memory.failcnt > 0) {
            if(unlikely(!cg->st_mem_failcnt)) {
                cg->st_mem_failcnt = rrdset_find_bytype(cgroup_chart_type(type, cg->chart_id, RRD_ID_LENGTH_MAX), "mem_failcnt");
                if(!cg->st_mem_failcnt) {
                    snprintfz(title, CHART_TITLE_MAX, "Memory Limit Failures for cgroup %s", cg->chart_title);
                    cg->st_mem_failcnt = rrdset_create(type, "mem_failcnt", NULL, "mem", "cgroup.mem_failcnt", title, "MB", 40250, update_every, RRDSET_TYPE_LINE);

                    rrddim_add(cg->st_mem_failcnt, "failures", NULL, 1, 1, RRDDIM_INCREMENTAL);
                }
            }
            else rrdset_next(cg->st_mem_failcnt);

            rrddim_set(cg->st_mem_failcnt, "failures", cg->memory.failcnt);
            rrdset_done(cg->st_mem_failcnt);
        }

        if(cg->io_service_bytes.updated && cg->io_service_bytes.Read + cg->io_service_bytes.Write > 0) {
            if(unlikely(!cg->st_io)) {
                cg->st_io = rrdset_find_bytype(cgroup_chart_type(type, cg->chart_id, RRD_ID_LENGTH_MAX), "io");
                if(!cg->st_io) {
                    snprintfz(title, CHART_TITLE_MAX, "I/O Bandwidth (all disks) for cgroup %s", cg->chart_title);
                    cg->st_io = rrdset_create(type, "io", NULL, "disk", "cgroup.io", title, "KB/s", 41200, update_every, RRDSET_TYPE_AREA);

                    rrddim_add(cg->st_io, "read", NULL, 1, 1024, RRDDIM_INCREMENTAL);
                    rrddim_add(cg->st_io, "write", NULL, -1, 1024, RRDDIM_INCREMENTAL);
                }
            }
            else rrdset_next(cg->st_io);

            rrddim_set(cg->st_io, "read", cg->io_service_bytes.Read);
            rrddim_set(cg->st_io, "write", cg->io_service_bytes.Write);
            rrdset_done(cg->st_io);
        }

        if(cg->io_serviced.updated && cg->io_serviced.Read + cg->io_serviced.Write > 0) {
            if(unlikely(!cg->st_serviced_ops)) {
                cg->st_serviced_ops = rrdset_find_bytype(cgroup_chart_type(type, cg->chart_id, RRD_ID_LENGTH_MAX), "serviced_ops");
                if(!cg->st_serviced_ops) {
                    snprintfz(title, CHART_TITLE_MAX, "Serviced I/O Operations (all disks) for cgroup %s"
                              , cg->chart_title);
                    cg->st_serviced_ops = rrdset_create(type, "serviced_ops", NULL, "disk", "cgroup.serviced_ops", title, "operations/s", 41200, update_every, RRDSET_TYPE_LINE);

                    rrddim_add(cg->st_serviced_ops, "read", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(cg->st_serviced_ops, "write", NULL, -1, 1, RRDDIM_INCREMENTAL);
                }
            }
            else rrdset_next(cg->st_serviced_ops);

            rrddim_set(cg->st_serviced_ops, "read", cg->io_serviced.Read);
            rrddim_set(cg->st_serviced_ops, "write", cg->io_serviced.Write);
            rrdset_done(cg->st_serviced_ops);
        }

        if(cg->throttle_io_service_bytes.updated && cg->throttle_io_service_bytes.Read + cg->throttle_io_service_bytes.Write > 0) {
            if(unlikely(!cg->st_throttle_io)) {
                cg->st_throttle_io = rrdset_find_bytype(cgroup_chart_type(type, cg->chart_id, RRD_ID_LENGTH_MAX), "throttle_io");
                if(!cg->st_throttle_io) {
                    snprintfz(title, CHART_TITLE_MAX, "Throttle I/O Bandwidth (all disks) for cgroup %s", cg->chart_title);
                    cg->st_throttle_io = rrdset_create(type, "throttle_io", NULL, "disk", "cgroup.throttle_io", title, "KB/s", 41200, update_every, RRDSET_TYPE_AREA);

                    rrddim_add(cg->st_throttle_io, "read", NULL, 1, 1024, RRDDIM_INCREMENTAL);
                    rrddim_add(cg->st_throttle_io, "write", NULL, -1, 1024, RRDDIM_INCREMENTAL);
                }
            }
            else rrdset_next(cg->st_throttle_io);

            rrddim_set(cg->st_throttle_io, "read", cg->throttle_io_service_bytes.Read);
            rrddim_set(cg->st_throttle_io, "write", cg->throttle_io_service_bytes.Write);
            rrdset_done(cg->st_throttle_io);
        }


        if(cg->throttle_io_serviced.updated && cg->throttle_io_serviced.Read + cg->throttle_io_serviced.Write > 0) {
            if(unlikely(!cg->st_throttle_serviced_ops)) {
                cg->st_throttle_serviced_ops = rrdset_find_bytype(cgroup_chart_type(type, cg->chart_id, RRD_ID_LENGTH_MAX), "throttle_serviced_ops");
                if(!cg->st_throttle_serviced_ops) {
                    snprintfz(title, CHART_TITLE_MAX, "Throttle Serviced I/O Operations (all disks) for cgroup %s", cg->chart_title);
                    cg->st_throttle_serviced_ops = rrdset_create(type, "throttle_serviced_ops", NULL, "disk", "cgroup.throttle_serviced_ops", title, "operations/s", 41200, update_every, RRDSET_TYPE_LINE);

                    rrddim_add(cg->st_throttle_serviced_ops, "read", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(cg->st_throttle_serviced_ops, "write", NULL, -1, 1, RRDDIM_INCREMENTAL);
                }
            }
            else rrdset_next(cg->st_throttle_serviced_ops);

            rrddim_set(cg->st_throttle_serviced_ops, "read", cg->throttle_io_serviced.Read);
            rrddim_set(cg->st_throttle_serviced_ops, "write", cg->throttle_io_serviced.Write);
            rrdset_done(cg->st_throttle_serviced_ops);
        }

        if(cg->io_queued.updated) {
            if(unlikely(!cg->st_queued_ops)) {
                cg->st_queued_ops = rrdset_find_bytype(cgroup_chart_type(type, cg->chart_id, RRD_ID_LENGTH_MAX), "queued_ops");
                if(!cg->st_queued_ops) {
                    snprintfz(title, CHART_TITLE_MAX, "Queued I/O Operations (all disks) for cgroup %s", cg->chart_title);
                    cg->st_queued_ops = rrdset_create(type, "queued_ops", NULL, "disk", "cgroup.queued_ops", title, "operations", 42000, update_every, RRDSET_TYPE_LINE);

                    rrddim_add(cg->st_queued_ops, "read", NULL, 1, 1, RRDDIM_ABSOLUTE);
                    rrddim_add(cg->st_queued_ops, "write", NULL, -1, 1, RRDDIM_ABSOLUTE);
                }
            }
            else rrdset_next(cg->st_queued_ops);

            rrddim_set(cg->st_queued_ops, "read", cg->io_queued.Read);
            rrddim_set(cg->st_queued_ops, "write", cg->io_queued.Write);
            rrdset_done(cg->st_queued_ops);
        }

        if(cg->io_merged.updated && cg->io_merged.Read + cg->io_merged.Write > 0) {
            if(unlikely(!cg->st_merged_ops)) {
                cg->st_merged_ops = rrdset_find_bytype(cgroup_chart_type(type, cg->chart_id, RRD_ID_LENGTH_MAX), "merged_ops");
                if(!cg->st_merged_ops) {
                    snprintfz(title, CHART_TITLE_MAX, "Merged I/O Operations (all disks) for cgroup %s", cg->chart_title);
                    cg->st_merged_ops = rrdset_create(type, "merged_ops", NULL, "disk", "cgroup.merged_ops", title, "operations/s", 42100, update_every, RRDSET_TYPE_LINE);

                    rrddim_add(cg->st_merged_ops, "read", NULL, 1, 1024, RRDDIM_INCREMENTAL);
                    rrddim_add(cg->st_merged_ops, "write", NULL, -1, 1024, RRDDIM_INCREMENTAL);
                }
            }
            else rrdset_next(cg->st_merged_ops);

            rrddim_set(cg->st_merged_ops, "read", cg->io_merged.Read);
            rrddim_set(cg->st_merged_ops, "write", cg->io_merged.Write);
            rrdset_done(cg->st_merged_ops);
        }
    }

    debug(D_CGROUP, "done updating cgroups charts");
}

// ----------------------------------------------------------------------------
// cgroups main

int do_sys_fs_cgroup(int update_every, usec_t dt) {
    (void)dt;

    static int cgroup_global_config_read = 0;
    static time_t last_run = 0;
    time_t now = now_realtime_sec();

    if(unlikely(!cgroup_global_config_read)) {
        read_cgroup_plugin_configuration();
        cgroup_global_config_read = 1;
    }

    if(unlikely(cgroup_enable_new_cgroups_detected_at_runtime && now - last_run > cgroup_check_for_new_every)) {
        find_all_cgroups();
        last_run = now;
    }

    read_all_cgroups(cgroup_root);
    update_cgroup_charts(update_every);

    return 0;
}

void *cgroups_main(void *ptr) {
    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)ptr;

    info("CGROUP Plugin thread created with task id %d", gettid());

    if(pthread_setcanceltype(PTHREAD_CANCEL_DEFERRED, NULL) != 0)
        error("Cannot set pthread cancel type to DEFERRED.");

    if(pthread_setcancelstate(PTHREAD_CANCEL_ENABLE, NULL) != 0)
        error("Cannot set pthread cancel state to ENABLE.");

    struct rusage thread;

    // when ZERO, attempt to do it
    int vdo_sys_fs_cgroup           = 0;
    int vdo_cpu_netdata             = !config_get_boolean("plugin:cgroups", "cgroups plugin resources", 1);

    // keep track of the time each module was called
    usec_t sutime_sys_fs_cgroup = 0ULL;

    // the next time we will run - aligned properly
    usec_t sunext = (now_realtime_sec() - (now_realtime_sec() % rrd_update_every) + rrd_update_every) * USEC_PER_SEC;

    RRDSET *stcpu_thread = NULL;

    for(;;) {
        usec_t sunow;
        if(unlikely(netdata_exit)) break;

        // delay until it is our time to run
        while((sunow = now_realtime_usec()) < sunext)
            sleep_usec(sunext - sunow);

        // find the next time we need to run
        while(now_realtime_usec() > sunext)
            sunext += rrd_update_every * USEC_PER_SEC;

        if(unlikely(netdata_exit)) break;

        // BEGIN -- the job to be done

        if(!vdo_sys_fs_cgroup) {
            debug(D_PROCNETDEV_LOOP, "PROCNETDEV: calling do_sys_fs_cgroup().");
            sunow = now_realtime_usec();
            vdo_sys_fs_cgroup = do_sys_fs_cgroup(rrd_update_every, (sutime_sys_fs_cgroup > 0)?sunow - sutime_sys_fs_cgroup:0ULL);
            sutime_sys_fs_cgroup = sunow;
        }
        if(unlikely(netdata_exit)) break;

        // END -- the job is done

        // --------------------------------------------------------------------

        if(!vdo_cpu_netdata) {
            getrusage(RUSAGE_THREAD, &thread);

            if(!stcpu_thread) stcpu_thread = rrdset_find("netdata.plugin_cgroups_cpu");
            if(!stcpu_thread) {
                stcpu_thread = rrdset_create("netdata", "plugin_cgroups_cpu", NULL, "cgroups", NULL, "NetData CGroups Plugin CPU usage", "milliseconds/s", 132000, rrd_update_every, RRDSET_TYPE_STACKED);

                rrddim_add(stcpu_thread, "user",  NULL,  1, 1000, RRDDIM_INCREMENTAL);
                rrddim_add(stcpu_thread, "system", NULL, 1, 1000, RRDDIM_INCREMENTAL);
            }
            else rrdset_next(stcpu_thread);

            rrddim_set(stcpu_thread, "user"  , thread.ru_utime.tv_sec * 1000000ULL + thread.ru_utime.tv_usec);
            rrddim_set(stcpu_thread, "system", thread.ru_stime.tv_sec * 1000000ULL + thread.ru_stime.tv_usec);
            rrdset_done(stcpu_thread);
        }
    }

    info("CGROUP thread exiting");

    static_thread->enabled = 0;
    pthread_exit(NULL);
    return NULL;
}
