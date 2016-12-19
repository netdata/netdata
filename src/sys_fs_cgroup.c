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

void read_cgroup_plugin_configuration() {
    cgroup_check_for_new_every = config_get_number("plugin:cgroups", "check for new cgroups every", cgroup_check_for_new_every);

    cgroup_enable_cpuacct_stat = config_get_boolean_ondemand("plugin:cgroups", "enable cpuacct stat", cgroup_enable_cpuacct_stat);
    cgroup_enable_cpuacct_usage = config_get_boolean_ondemand("plugin:cgroups", "enable cpuacct usage", cgroup_enable_cpuacct_usage);
    cgroup_enable_memory = config_get_boolean_ondemand("plugin:cgroups", "enable memory", cgroup_enable_memory);
    cgroup_enable_blkio = config_get_boolean_ondemand("plugin:cgroups", "enable blkio", cgroup_enable_blkio);

    char filename[FILENAME_MAX + 1], *s;
    struct mountinfo *mi, *root = mountinfo_read();

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

    cgroup_root_max = config_get_number("plugin:cgroups", "max cgroups to allow", cgroup_root_max);
    cgroup_max_depth = config_get_number("plugin:cgroups", "max cgroups depth to monitor", cgroup_max_depth);

    cgroup_enable_new_cgroups_detected_at_runtime = config_get_boolean("plugin:cgroups", "enable new cgroups detected at run time", cgroup_enable_new_cgroups_detected_at_runtime);

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

#define CGROUP_OPTIONS_DISABLED_DUPLICATE 0x00000001

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

void cgroup_get_chart_id(struct cgroup *cg) {
    debug(D_CGROUP, "getting the name of cgroup '%s'", cg->id);

    pid_t cgroup_pid;
    char buffer[CGROUP_CHARTID_LINE_MAX + 1];

    snprintfz(buffer, CGROUP_CHARTID_LINE_MAX, "exec %s '%s'",
             config_get("plugin:cgroups", "script to get cgroup names", PLUGINS_DIR "/cgroup-name.sh"), cg->chart_id);

    debug(D_CGROUP, "executing command '%s' for cgroup '%s'", buffer, cg->id);
    FILE *fp = mypopen(buffer, &cgroup_pid);
    if(!fp) {
        error("CGROUP: Cannot popen(\"%s\", \"r\").", buffer);
        return;
    }
    debug(D_CGROUP, "reading from command '%s' for cgroup '%s'", buffer, cg->id);
    char *s = fgets(buffer, CGROUP_CHARTID_LINE_MAX, fp);
    debug(D_CGROUP, "closing command for cgroup '%s'", cg->id);
    mypclose(fp, cgroup_pid);
    debug(D_CGROUP, "closed command for cgroup '%s'", cg->id);

    if(s && *s && *s != '\n') {
        debug(D_CGROUP, "cgroup '%s' should be renamed to '%s'", cg->id, s);

        trim(s);

        freez(cg->chart_title);
        cg->chart_title = strdupz(s);
        netdata_fix_chart_name(cg->chart_title);

        freez(cg->chart_id);
        cg->chart_id = strdupz(s);
        netdata_fix_chart_id(cg->chart_id);
        cg->hash_chart = simple_hash(cg->chart_id);

        debug(D_CGROUP, "cgroup '%s' renamed to '%s' (title: '%s')", cg->id, cg->chart_id, cg->chart_title);
    }
    else debug(D_CGROUP, "cgroup '%s' is not to be renamed (will be shown as '%s')", cg->id, cg->chart_id);
}

struct cgroup *cgroup_add(const char *id) {
    debug(D_CGROUP, "adding cgroup '%s'", id);

    if(cgroup_root_count >= cgroup_root_max) {
        info("Maximum number of cgroups reached (%d). Not adding cgroup '%s'", cgroup_root_count, id);
        return NULL;
    }

    int def = cgroup_enable_new_cgroups_detected_at_runtime;
    const char *chart_id = id;
    if(!*chart_id) {
        chart_id = "/";

        // disable by default the root cgroup
        def = 0;
        debug(D_CGROUP, "cgroup '%s' is the root container (by default %s)", id, (def)?"enabled":"disabled");
    }
    else {
        if(*chart_id == '/') chart_id++;

        size_t len = strlen(chart_id);

        // disable by default the parent cgroup
        // for known cgroup managers
        if(!strcmp(chart_id, "lxc") ||
                !strcmp(chart_id, "docker") ||
                !strcmp(chart_id, "libvirt") ||
                !strcmp(chart_id, "qemu") ||
                !strcmp(chart_id, "systemd") ||
                !strcmp(chart_id, "system.slice") ||
                !strcmp(chart_id, "machine.slice") ||
                !strcmp(chart_id, "init.scope") ||
                !strcmp(chart_id, "user") ||
                !strcmp(chart_id, "system") ||
                !strcmp(chart_id, "machine") ||
                // starts with them
                (len >  6 && !strncmp(chart_id, "user/", 6)) ||
                (len > 11 && !strncmp(chart_id, "user.slice/", 11)) ||
                // ends with them
                (len >  5 && !strncmp(&chart_id[len -  5], ".user", 5)) ||
                (len >  5 && !strncmp(&chart_id[len -  5], ".swap", 5)) ||
                (len >  6 && !strncmp(&chart_id[len -  6], ".slice", 6)) ||
                (len >  6 && !strncmp(&chart_id[len -  6], ".mount", 6)) ||
                (len >  8 && !strncmp(&chart_id[len -  8], ".session", 8)) ||
                (len >  8 && !strncmp(&chart_id[len -  8], ".service", 8)) ||
                (len > 10 && !strncmp(&chart_id[len - 10], ".partition", 10)) ||
                // starts and ends with them
                (len > 7 && !strncmp(chart_id, "lxc/", 4) && !strncmp(&chart_id[len - 3], "/ns", 3)) // #1397
                ) {
            def = 0;
            debug(D_CGROUP, "cgroup '%s' is %s (by default)", id, (def)?"enabled":"disabled");
        }
    }

    struct cgroup *cg = callocz(1, sizeof(struct cgroup));

    cg->id = strdupz(id);
    cg->hash = simple_hash(cg->id);

    cg->chart_id = strdupz(chart_id);
    netdata_fix_chart_id(cg->chart_id);
    cg->hash_chart = simple_hash(cg->chart_id);

    cg->chart_title = strdupz(chart_id);

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
    cgroup_get_chart_id(cg);

    debug(D_CGROUP, "adding cgroup '%s' with chart id '%s'", id, chart_id);

    char option[FILENAME_MAX + 1];
    snprintfz(option, FILENAME_MAX, "enable cgroup %s", cg->chart_title);
    cg->enabled = config_get_boolean("plugin:cgroups", option, def);

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

    debug(D_CGROUP, "Added cgroup '%s' with chart id '%s' and title '%s' as %s (default was %s)", cg->id, cg->chart_id, cg->chart_title, (cg->enabled)?"enabled":"disabled", (def)?"enabled":"disabled");

    return cg;
}

void cgroup_free(struct cgroup *cg) {
    debug(D_CGROUP, "Removing cgroup '%s' with chart id '%s' (was %s and %s)", cg->id, cg->chart_id, (cg->enabled)?"enabled":"disabled", (cg->available)?"available":"not available");

    freez(cg->cpuacct_usage.cpu_percpu);

    freez(cg->cpuacct_stat.filename);
    freez(cg->cpuacct_usage.filename);
    freez(cg->memory.filename);
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

    debug(D_CGROUP, "cgroup_find('%s') %s", id, (cg)?"found":"not found");
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
        debug(D_CGROUP, "will add dir '%s' as cgroup", dir);
        cg = cgroup_add(dir);
    }

    if(cg) cg->available = 1;
}

int find_dir_in_subdirs(const char *base, const char *this, void (*callback)(const char *)) {
    debug(D_CGROUP, "searching for directories in '%s'", base);

    int ret = -1;
    int enabled = -1;
    if(!this) this = base;
    size_t dirlen = strlen(this), baselen = strlen(base);
    const char *relative_path = &this[baselen];

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

        debug(D_CGROUP, "examining '%s/%s'", this, de->d_name);

        if(de->d_type == DT_DIR) {
            if(enabled == -1) {
                const char *r = relative_path;
                if(*r == '\0') r = "/";
                else if (*r == '/') r++;

                // do not decent in directories we are not interested
                // https://github.com/firehol/netdata/issues/345
                int def = 1;
                size_t len = strlen(r);
                if(len >  5 && !strncmp(&r[len -  5], "-qemu", 5))
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
        if(cgroup_enable_cpuacct_stat && !cg->cpuacct_stat.filename) {
            snprintfz(filename, FILENAME_MAX, "%s%s/cpuacct.stat", cgroup_cpuacct_base, cg->id);
            if(stat(filename, &buf) != -1) {
                cg->cpuacct_stat.filename = strdupz(filename);
                debug(D_CGROUP, "cpuacct.stat filename for cgroup '%s': '%s'", cg->id, cg->cpuacct_stat.filename);
            }
            else debug(D_CGROUP, "cpuacct.stat file for cgroup '%s': '%s' does not exist.", cg->id, filename);
        }
        if(cgroup_enable_cpuacct_usage && !cg->cpuacct_usage.filename) {
            snprintfz(filename, FILENAME_MAX, "%s%s/cpuacct.usage_percpu", cgroup_cpuacct_base, cg->id);
            if(stat(filename, &buf) != -1) {
                cg->cpuacct_usage.filename = strdupz(filename);
                debug(D_CGROUP, "cpuacct.usage_percpu filename for cgroup '%s': '%s'", cg->id, cg->cpuacct_usage.filename);
            }
            else debug(D_CGROUP, "cpuacct.usage_percpu file for cgroup '%s': '%s' does not exist.", cg->id, filename);
        }
        if(cgroup_enable_memory && !cg->memory.filename) {
            snprintfz(filename, FILENAME_MAX, "%s%s/memory.stat", cgroup_memory_base, cg->id);
            if(stat(filename, &buf) != -1) {
                cg->memory.filename = strdupz(filename);
                debug(D_CGROUP, "memory.stat filename for cgroup '%s': '%s'", cg->id, cg->memory.filename);
            }
            else debug(D_CGROUP, "memory.stat file for cgroup '%s': '%s' does not exist.", cg->id, filename);

            snprintfz(filename, FILENAME_MAX, "%s%s/memory.usage_in_bytes", cgroup_memory_base, cg->id);
            if(stat(filename, &buf) != -1) {
                cg->memory.filename_usage_in_bytes = strdupz(filename);
                debug(D_CGROUP, "memory.usage_in_bytes filename for cgroup '%s': '%s'", cg->id, cg->memory.filename_usage_in_bytes);
            }
            else debug(D_CGROUP, "memory.usage_in_bytes file for cgroup '%s': '%s' does not exist.", cg->id, filename);

            snprintfz(filename, FILENAME_MAX, "%s%s/memory.msw_usage_in_bytes", cgroup_memory_base, cg->id);
            if(stat(filename, &buf) != -1) {
                cg->memory.filename_msw_usage_in_bytes = strdupz(filename);
                debug(D_CGROUP, "memory.msw_usage_in_bytes filename for cgroup '%s': '%s'", cg->id, cg->memory.filename_msw_usage_in_bytes);
            }
            else debug(D_CGROUP, "memory.msw_usage_in_bytes file for cgroup '%s': '%s' does not exist.", cg->id, filename);

            snprintfz(filename, FILENAME_MAX, "%s%s/memory.failcnt", cgroup_memory_base, cg->id);
            if(stat(filename, &buf) != -1) {
                cg->memory.filename_failcnt = strdupz(filename);
                debug(D_CGROUP, "memory.failcnt filename for cgroup '%s': '%s'", cg->id, cg->memory.filename_failcnt);
            }
            else debug(D_CGROUP, "memory.failcnt file for cgroup '%s': '%s' does not exist.", cg->id, filename);
        }
        if(cgroup_enable_blkio) {
            if(!cg->io_service_bytes.filename) {
                snprintfz(filename, FILENAME_MAX, "%s%s/blkio.io_service_bytes", cgroup_blkio_base, cg->id);
                if(stat(filename, &buf) != -1) {
                    cg->io_service_bytes.filename = strdupz(filename);
                    debug(D_CGROUP, "io_service_bytes filename for cgroup '%s': '%s'", cg->id, cg->io_service_bytes.filename);
                }
                else debug(D_CGROUP, "io_service_bytes file for cgroup '%s': '%s' does not exist.", cg->id, filename);
            }
            if(!cg->io_serviced.filename) {
                snprintfz(filename, FILENAME_MAX, "%s%s/blkio.io_serviced", cgroup_blkio_base, cg->id);
                if(stat(filename, &buf) != -1) {
                    cg->io_serviced.filename = strdupz(filename);
                    debug(D_CGROUP, "io_serviced filename for cgroup '%s': '%s'", cg->id, cg->io_serviced.filename);
                }
                else debug(D_CGROUP, "io_serviced file for cgroup '%s': '%s' does not exist.", cg->id, filename);
            }
            if(!cg->throttle_io_service_bytes.filename) {
                snprintfz(filename, FILENAME_MAX, "%s%s/blkio.throttle.io_service_bytes", cgroup_blkio_base, cg->id);
                if(stat(filename, &buf) != -1) {
                    cg->throttle_io_service_bytes.filename = strdupz(filename);
                    debug(D_CGROUP, "throttle_io_service_bytes filename for cgroup '%s': '%s'", cg->id, cg->throttle_io_service_bytes.filename);
                }
                else debug(D_CGROUP, "throttle_io_service_bytes file for cgroup '%s': '%s' does not exist.", cg->id, filename);
            }
            if(!cg->throttle_io_serviced.filename) {
                snprintfz(filename, FILENAME_MAX, "%s%s/blkio.throttle.io_serviced", cgroup_blkio_base, cg->id);
                if(stat(filename, &buf) != -1) {
                    cg->throttle_io_serviced.filename = strdupz(filename);
                    debug(D_CGROUP, "throttle_io_serviced filename for cgroup '%s': '%s'", cg->id, cg->throttle_io_serviced.filename);
                }
                else debug(D_CGROUP, "throttle_io_serviced file for cgroup '%s': '%s' does not exist.", cg->id, filename);
            }
            if(!cg->io_merged.filename) {
                snprintfz(filename, FILENAME_MAX, "%s%s/blkio.io_merged", cgroup_blkio_base, cg->id);
                if(stat(filename, &buf) != -1) {
                    cg->io_merged.filename = strdupz(filename);
                    debug(D_CGROUP, "io_merged filename for cgroup '%s': '%s'", cg->id, cg->io_merged.filename);
                }
                else debug(D_CGROUP, "io_merged file for cgroup '%s': '%s' does not exist.", cg->id, filename);
            }
            if(!cg->io_queued.filename) {
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

void update_cgroup_charts(int update_every) {
    debug(D_CGROUP, "updating cgroups charts");

    char type[RRD_ID_LENGTH_MAX + 1];
    char title[CHART_TITLE_MAX + 1];

    struct cgroup *cg;
    RRDSET *st;

    for(cg = cgroup_root; cg ; cg = cg->next) {
        if(!cg->available || !cg->enabled)
            continue;

        if(cg->id[0] == '\0')
            strcpy(type, "cgroup_root");
        else if(cg->id[0] == '/')
            snprintfz(type, RRD_ID_LENGTH_MAX, "cgroup_%s", cg->chart_id);
        else
            snprintfz(type, RRD_ID_LENGTH_MAX, "cgroup_%s", cg->chart_id);

        netdata_fix_chart_id(type);

        if(cg->cpuacct_stat.updated) {
            st = rrdset_find_bytype(type, "cpu");
            if(!st) {
                snprintfz(title, CHART_TITLE_MAX, "CPU Usage (%d%% = %d core%s) for cgroup %s", (processors * 100), processors, (processors>1)?"s":"", cg->chart_title);
                st = rrdset_create(type, "cpu", NULL, "cpu", "cgroup.cpu", title, "%", 40000, update_every, RRDSET_TYPE_STACKED);

                rrddim_add(st, "user", NULL, 100, hz, RRDDIM_INCREMENTAL);
                rrddim_add(st, "system", NULL, 100, hz, RRDDIM_INCREMENTAL);
            }
            else rrdset_next(st);

            rrddim_set(st, "user", cg->cpuacct_stat.user);
            rrddim_set(st, "system", cg->cpuacct_stat.system);
            rrdset_done(st);
        }

        if(cg->cpuacct_usage.updated) {
            char id[RRD_ID_LENGTH_MAX + 1];
            unsigned int i;

            st = rrdset_find_bytype(type, "cpu_per_core");
            if(!st) {
                snprintfz(title, CHART_TITLE_MAX, "CPU Usage (%d%% = %d core%s) Per Core for cgroup %s", (processors * 100), processors, (processors>1)?"s":"", cg->chart_title);
                st = rrdset_create(type, "cpu_per_core", NULL, "cpu", "cgroup.cpu_per_core", title, "%", 40100, update_every, RRDSET_TYPE_STACKED);

                for(i = 0; i < cg->cpuacct_usage.cpus ;i++) {
                    snprintfz(id, CHART_TITLE_MAX, "cpu%u", i);
                    rrddim_add(st, id, NULL, 100, 1000000000, RRDDIM_INCREMENTAL);
                }
            }
            else rrdset_next(st);

            for(i = 0; i < cg->cpuacct_usage.cpus ;i++) {
                snprintfz(id, CHART_TITLE_MAX, "cpu%u", i);
                rrddim_set(st, id, cg->cpuacct_usage.cpu_percpu[i]);
            }
            rrdset_done(st);
        }

        if(cg->memory.updated) {
            if(cg->memory.cache + cg->memory.rss + cg->memory.rss_huge + cg->memory.mapped_file > 0) {
                st = rrdset_find_bytype(type, "mem");
                if(!st) {
                    snprintfz(title, CHART_TITLE_MAX, "Memory Usage for cgroup %s", cg->chart_title);
                    st = rrdset_create(type, "mem", NULL, "mem", "cgroup.mem", title, "MB", 40210, update_every,
                                       RRDSET_TYPE_STACKED);

                    rrddim_add(st, "cache", NULL, 1, 1024 * 1024, RRDDIM_ABSOLUTE);
                    rrddim_add(st, "rss", NULL, 1, 1024 * 1024, RRDDIM_ABSOLUTE);
                    if(cg->memory.has_dirty_swap)
                        rrddim_add(st, "swap", NULL, 1, 1024 * 1024, RRDDIM_ABSOLUTE);
                    rrddim_add(st, "rss_huge", NULL, 1, 1024 * 1024, RRDDIM_ABSOLUTE);
                    rrddim_add(st, "mapped_file", NULL, 1, 1024 * 1024, RRDDIM_ABSOLUTE);
                }
                else rrdset_next(st);

                rrddim_set(st, "cache", cg->memory.cache);
                rrddim_set(st, "rss", cg->memory.rss);
                if(cg->memory.has_dirty_swap)
                    rrddim_set(st, "swap", cg->memory.swap);
                rrddim_set(st, "rss_huge", cg->memory.rss_huge);
                rrddim_set(st, "mapped_file", cg->memory.mapped_file);
                rrdset_done(st);
            }

            st = rrdset_find_bytype(type, "writeback");
            if(!st) {
                snprintfz(title, CHART_TITLE_MAX, "Writeback Memory for cgroup %s", cg->chart_title);
                st = rrdset_create(type, "writeback", NULL, "mem", "cgroup.writeback", title, "MB", 40300,
                                   update_every, RRDSET_TYPE_AREA);

                if(cg->memory.has_dirty_swap)
                    rrddim_add(st, "dirty", NULL, 1, 1024 * 1024, RRDDIM_ABSOLUTE);
                rrddim_add(st, "writeback", NULL, 1, 1024 * 1024, RRDDIM_ABSOLUTE);
            }
            else rrdset_next(st);

            if(cg->memory.has_dirty_swap)
                rrddim_set(st, "dirty", cg->memory.dirty);
            rrddim_set(st, "writeback", cg->memory.writeback);
            rrdset_done(st);

            if(cg->memory.pgpgin + cg->memory.pgpgout > 0) {
                st = rrdset_find_bytype(type, "mem_activity");
                if(!st) {
                    snprintfz(title, CHART_TITLE_MAX, "Memory Activity for cgroup %s", cg->chart_title);
                    st = rrdset_create(type, "mem_activity", NULL, "mem", "cgroup.mem_activity", title, "MB/s",
                                       40400, update_every, RRDSET_TYPE_LINE);

                    rrddim_add(st, "pgpgin", "in", sysconf(_SC_PAGESIZE), 1024 * 1024, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "pgpgout", "out", -sysconf(_SC_PAGESIZE), 1024 * 1024, RRDDIM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set(st, "pgpgin", cg->memory.pgpgin);
                rrddim_set(st, "pgpgout", cg->memory.pgpgout);
                rrdset_done(st);
            }

            if(cg->memory.pgfault + cg->memory.pgmajfault > 0) {
                st = rrdset_find_bytype(type, "pgfaults");
                if(!st) {
                    snprintfz(title, CHART_TITLE_MAX, "Memory Page Faults for cgroup %s", cg->chart_title);
                    st = rrdset_create(type, "pgfaults", NULL, "mem", "cgroup.pgfaults", title, "MB/s", 40500,
                                       update_every, RRDSET_TYPE_LINE);

                    rrddim_add(st, "pgfault", NULL, sysconf(_SC_PAGESIZE), 1024 * 1024, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "pgmajfault", "swap", -sysconf(_SC_PAGESIZE), 1024 * 1024, RRDDIM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set(st, "pgfault", cg->memory.pgfault);
                rrddim_set(st, "pgmajfault", cg->memory.pgmajfault);
                rrdset_done(st);
            }
        }

        if(cg->memory.usage_in_bytes_updated) {
            st = rrdset_find_bytype(type, "mem_usage");
            if(!st) {
                snprintfz(title, CHART_TITLE_MAX, "Total Memory for cgroup %s", cg->chart_title);
                st = rrdset_create(type, "mem_usage", NULL, "mem", "cgroup.mem_usage", title, "MB", 40200,
                                   update_every, RRDSET_TYPE_STACKED);

                rrddim_add(st, "ram", NULL, 1, 1024 * 1024, RRDDIM_ABSOLUTE);
                rrddim_add(st, "swap", NULL, 1, 1024 * 1024, RRDDIM_ABSOLUTE);
            }
            else rrdset_next(st);

            rrddim_set(st, "ram", cg->memory.usage_in_bytes);
            rrddim_set(st, "swap", (cg->memory.msw_usage_in_bytes > cg->memory.usage_in_bytes)?cg->memory.msw_usage_in_bytes - cg->memory.usage_in_bytes:0);
            rrdset_done(st);
        }

        if(cg->memory.failcnt_updated && cg->memory.failcnt > 0) {
            st = rrdset_find_bytype(type, "mem_failcnt");
            if(!st) {
                snprintfz(title, CHART_TITLE_MAX, "Memory Limit Failures for cgroup %s", cg->chart_title);
                st = rrdset_create(type, "mem_failcnt", NULL, "mem", "cgroup.mem_failcnt", title, "MB", 40250,
                                   update_every, RRDSET_TYPE_LINE);

                rrddim_add(st, "failures", NULL, 1, 1, RRDDIM_INCREMENTAL);
            }
            else rrdset_next(st);

            rrddim_set(st, "failures", cg->memory.failcnt);
            rrdset_done(st);
        }

        if(cg->io_service_bytes.updated && cg->io_service_bytes.Read + cg->io_service_bytes.Write > 0) {
            st = rrdset_find_bytype(type, "io");
            if(!st) {
                snprintfz(title, CHART_TITLE_MAX, "I/O Bandwidth (all disks) for cgroup %s", cg->chart_title);
                st = rrdset_create(type, "io", NULL, "disk", "cgroup.io", title, "KB/s", 41200,
                                   update_every, RRDSET_TYPE_AREA);

                rrddim_add(st, "read", NULL, 1, 1024, RRDDIM_INCREMENTAL);
                rrddim_add(st, "write", NULL, -1, 1024, RRDDIM_INCREMENTAL);
            }
            else rrdset_next(st);

            rrddim_set(st, "read", cg->io_service_bytes.Read);
            rrddim_set(st, "write", cg->io_service_bytes.Write);
            rrdset_done(st);
        }

        if(cg->io_serviced.updated && cg->io_serviced.Read + cg->io_serviced.Write > 0) {
            st = rrdset_find_bytype(type, "serviced_ops");
            if(!st) {
                snprintfz(title, CHART_TITLE_MAX, "Serviced I/O Operations (all disks) for cgroup %s", cg->chart_title);
                st = rrdset_create(type, "serviced_ops", NULL, "disk", "cgroup.serviced_ops", title, "operations/s", 41200,
                                   update_every, RRDSET_TYPE_LINE);

                rrddim_add(st, "read", NULL, 1, 1, RRDDIM_INCREMENTAL);
                rrddim_add(st, "write", NULL, -1, 1, RRDDIM_INCREMENTAL);
            }
            else rrdset_next(st);

            rrddim_set(st, "read", cg->io_serviced.Read);
            rrddim_set(st, "write", cg->io_serviced.Write);
            rrdset_done(st);
        }

        if(cg->throttle_io_service_bytes.updated && cg->throttle_io_service_bytes.Read + cg->throttle_io_service_bytes.Write > 0) {
            st = rrdset_find_bytype(type, "throttle_io");
            if(!st) {
                snprintfz(title, CHART_TITLE_MAX, "Throttle I/O Bandwidth (all disks) for cgroup %s", cg->chart_title);
                st = rrdset_create(type, "throttle_io", NULL, "disk", "cgroup.throttle_io", title, "KB/s", 41200,
                                   update_every, RRDSET_TYPE_AREA);

                rrddim_add(st, "read", NULL, 1, 1024, RRDDIM_INCREMENTAL);
                rrddim_add(st, "write", NULL, -1, 1024, RRDDIM_INCREMENTAL);
            }
            else rrdset_next(st);

            rrddim_set(st, "read", cg->throttle_io_service_bytes.Read);
            rrddim_set(st, "write", cg->throttle_io_service_bytes.Write);
            rrdset_done(st);
        }


        if(cg->throttle_io_serviced.updated && cg->throttle_io_serviced.Read + cg->throttle_io_serviced.Write > 0) {
            st = rrdset_find_bytype(type, "throttle_serviced_ops");
            if(!st) {
                snprintfz(title, CHART_TITLE_MAX, "Throttle Serviced I/O Operations (all disks) for cgroup %s", cg->chart_title);
                st = rrdset_create(type, "throttle_serviced_ops", NULL, "disk", "cgroup.throttle_serviced_ops", title, "operations/s", 41200,
                                   update_every, RRDSET_TYPE_LINE);

                rrddim_add(st, "read", NULL, 1, 1, RRDDIM_INCREMENTAL);
                rrddim_add(st, "write", NULL, -1, 1, RRDDIM_INCREMENTAL);
            }
            else rrdset_next(st);

            rrddim_set(st, "read", cg->throttle_io_serviced.Read);
            rrddim_set(st, "write", cg->throttle_io_serviced.Write);
            rrdset_done(st);
        }

        if(cg->io_queued.updated) {
            st = rrdset_find_bytype(type, "queued_ops");
            if(!st) {
                snprintfz(title, CHART_TITLE_MAX, "Queued I/O Operations (all disks) for cgroup %s", cg->chart_title);
                st = rrdset_create(type, "queued_ops", NULL, "disk", "cgroup.queued_ops", title, "operations", 42000,
                                   update_every, RRDSET_TYPE_LINE);

                rrddim_add(st, "read", NULL, 1, 1, RRDDIM_ABSOLUTE);
                rrddim_add(st, "write", NULL, -1, 1, RRDDIM_ABSOLUTE);
            }
            else rrdset_next(st);

            rrddim_set(st, "read", cg->io_queued.Read);
            rrddim_set(st, "write", cg->io_queued.Write);
            rrdset_done(st);
        }

        if(cg->io_merged.updated && cg->io_merged.Read + cg->io_merged.Write > 0) {
            st = rrdset_find_bytype(type, "merged_ops");
            if(!st) {
                snprintfz(title, CHART_TITLE_MAX, "Merged I/O Operations (all disks) for cgroup %s", cg->chart_title);
                st = rrdset_create(type, "merged_ops", NULL, "disk", "cgroup.merged_ops", title, "operations/s", 42100,
                                   update_every, RRDSET_TYPE_LINE);

                rrddim_add(st, "read", NULL, 1, 1024, RRDDIM_INCREMENTAL);
                rrddim_add(st, "write", NULL, -1, 1024, RRDDIM_INCREMENTAL);
            }
            else rrdset_next(st);

            rrddim_set(st, "read", cg->io_merged.Read);
            rrddim_set(st, "write", cg->io_merged.Write);
            rrdset_done(st);
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

void *cgroups_main(void *ptr)
{
    (void)ptr;

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
                stcpu_thread = rrdset_create("netdata", "plugin_cgroups_cpu", NULL, "proc.internal", NULL, "NetData CGroups Plugin CPU usage", "milliseconds/s", 132000, rrd_update_every, RRDSET_TYPE_STACKED);

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

    pthread_exit(NULL);
    return NULL;
}
