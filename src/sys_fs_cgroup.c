#include "common.h"

// ----------------------------------------------------------------------------
// cgroup globals

#define CHART_PRIORITY_SYSTEMD_SERVICES 19000
#define CHART_PRIORITY_CONTAINERS       40000

static long system_page_size = 4096; // system will be queried via sysconf() in configuration()

static int cgroup_enable_cpuacct_stat = CONFIG_BOOLEAN_AUTO;
static int cgroup_enable_cpuacct_usage = CONFIG_BOOLEAN_AUTO;
static int cgroup_enable_memory = CONFIG_BOOLEAN_AUTO;
static int cgroup_enable_detailed_memory = CONFIG_BOOLEAN_AUTO;
static int cgroup_enable_memory_failcnt = CONFIG_BOOLEAN_AUTO;
static int cgroup_enable_swap = CONFIG_BOOLEAN_AUTO;
static int cgroup_enable_blkio_io = CONFIG_BOOLEAN_AUTO;
static int cgroup_enable_blkio_ops = CONFIG_BOOLEAN_AUTO;
static int cgroup_enable_blkio_throttle_io = CONFIG_BOOLEAN_AUTO;
static int cgroup_enable_blkio_throttle_ops = CONFIG_BOOLEAN_AUTO;
static int cgroup_enable_blkio_merged_ops = CONFIG_BOOLEAN_AUTO;
static int cgroup_enable_blkio_queued_ops = CONFIG_BOOLEAN_AUTO;

static int cgroup_enable_systemd_services = CONFIG_BOOLEAN_YES;
static int cgroup_enable_systemd_services_detailed_memory = CONFIG_BOOLEAN_NO;
static int cgroup_used_memory_without_cache = CONFIG_BOOLEAN_YES;

static int cgroup_search_in_devices = 1;

static int cgroup_enable_new_cgroups_detected_at_runtime = 1;
static int cgroup_check_for_new_every = 10;
static int cgroup_update_every = 1;

static int cgroup_recheck_zero_blkio_every_iterations = 10;
static int cgroup_recheck_zero_mem_failcnt_every_iterations = 10;
static int cgroup_recheck_zero_mem_detailed_every_iterations = 10;

static char *cgroup_cpuacct_base = NULL;
static char *cgroup_blkio_base = NULL;
static char *cgroup_memory_base = NULL;
static char *cgroup_devices_base = NULL;

static int cgroup_root_count = 0;
static int cgroup_root_max = 500;
static int cgroup_max_depth = 0;

static SIMPLE_PATTERN *enabled_cgroup_patterns = NULL;
static SIMPLE_PATTERN *enabled_cgroup_paths = NULL;
static SIMPLE_PATTERN *enabled_cgroup_renames = NULL;
static SIMPLE_PATTERN *systemd_services_cgroups = NULL;

static char *cgroups_rename_script = NULL;

static int cgroups_check = 0;

static uint32_t Read_hash = 0;
static uint32_t Write_hash = 0;
static uint32_t user_hash = 0;
static uint32_t system_hash = 0;

void read_cgroup_plugin_configuration() {
    system_page_size = sysconf(_SC_PAGESIZE);

    Read_hash = simple_hash("Read");
    Write_hash = simple_hash("Write");
    user_hash = simple_hash("user");
    system_hash = simple_hash("system");

    cgroup_update_every = (int)config_get_number("plugin:cgroups", "update every", localhost->rrd_update_every);
    if(cgroup_update_every < localhost->rrd_update_every)
        cgroup_update_every = localhost->rrd_update_every;

    cgroup_check_for_new_every = (int)config_get_number("plugin:cgroups", "check for new cgroups every", cgroup_check_for_new_every * cgroup_update_every);
    if(cgroup_check_for_new_every < cgroup_update_every)
        cgroup_check_for_new_every = cgroup_update_every;

    cgroup_enable_cpuacct_stat = config_get_boolean_ondemand("plugin:cgroups", "enable cpuacct stat (total CPU)", cgroup_enable_cpuacct_stat);
    cgroup_enable_cpuacct_usage = config_get_boolean_ondemand("plugin:cgroups", "enable cpuacct usage (per core CPU)", cgroup_enable_cpuacct_usage);

    cgroup_enable_memory = config_get_boolean_ondemand("plugin:cgroups", "enable memory (used mem including cache)", cgroup_enable_memory);
    cgroup_enable_detailed_memory = config_get_boolean_ondemand("plugin:cgroups", "enable detailed memory", cgroup_enable_detailed_memory);
    cgroup_enable_memory_failcnt = config_get_boolean_ondemand("plugin:cgroups", "enable memory limits fail count", cgroup_enable_memory_failcnt);
    cgroup_enable_swap = config_get_boolean_ondemand("plugin:cgroups", "enable swap memory", cgroup_enable_swap);

    cgroup_enable_blkio_io = config_get_boolean_ondemand("plugin:cgroups", "enable blkio bandwidth", cgroup_enable_blkio_io);
    cgroup_enable_blkio_ops = config_get_boolean_ondemand("plugin:cgroups", "enable blkio operations", cgroup_enable_blkio_ops);
    cgroup_enable_blkio_throttle_io = config_get_boolean_ondemand("plugin:cgroups", "enable blkio throttle bandwidth", cgroup_enable_blkio_throttle_io);
    cgroup_enable_blkio_throttle_ops = config_get_boolean_ondemand("plugin:cgroups", "enable blkio throttle operations", cgroup_enable_blkio_throttle_ops);
    cgroup_enable_blkio_queued_ops = config_get_boolean_ondemand("plugin:cgroups", "enable blkio queued operations", cgroup_enable_blkio_queued_ops);
    cgroup_enable_blkio_merged_ops = config_get_boolean_ondemand("plugin:cgroups", "enable blkio merged operations", cgroup_enable_blkio_merged_ops);

    cgroup_recheck_zero_blkio_every_iterations = (int)config_get_number("plugin:cgroups", "recheck zero blkio every iterations", cgroup_recheck_zero_blkio_every_iterations);
    cgroup_recheck_zero_mem_failcnt_every_iterations = (int)config_get_number("plugin:cgroups", "recheck zero memory failcnt every iterations", cgroup_recheck_zero_mem_failcnt_every_iterations);
    cgroup_recheck_zero_mem_detailed_every_iterations = (int)config_get_number("plugin:cgroups", "recheck zero detailed memory every iterations", cgroup_recheck_zero_mem_detailed_every_iterations);

    cgroup_enable_systemd_services = config_get_boolean("plugin:cgroups", "enable systemd services", cgroup_enable_systemd_services);
    cgroup_enable_systemd_services_detailed_memory = config_get_boolean("plugin:cgroups", "enable systemd services detailed memory", cgroup_enable_systemd_services_detailed_memory);
    cgroup_used_memory_without_cache = config_get_boolean("plugin:cgroups", "report used memory without cache", cgroup_used_memory_without_cache);

    char filename[FILENAME_MAX + 1], *s;
    struct mountinfo *mi, *root = mountinfo_read(0);

    mi = mountinfo_find_by_filesystem_super_option(root, "cgroup", "cpuacct");
    if(!mi) mi = mountinfo_find_by_filesystem_mount_source(root, "cgroup", "cpuacct");
    if(!mi) {
        error("CGROUP: cannot find cpuacct mountinfo. Assuming default: /sys/fs/cgroup/cpuacct");
        s = "/sys/fs/cgroup/cpuacct";
    }
    else s = mi->mount_point;
    snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, s);
    cgroup_cpuacct_base = config_get("plugin:cgroups", "path to /sys/fs/cgroup/cpuacct", filename);

    mi = mountinfo_find_by_filesystem_super_option(root, "cgroup", "blkio");
    if(!mi) mi = mountinfo_find_by_filesystem_mount_source(root, "cgroup", "blkio");
    if(!mi) {
        error("CGROUP: cannot find blkio mountinfo. Assuming default: /sys/fs/cgroup/blkio");
        s = "/sys/fs/cgroup/blkio";
    }
    else s = mi->mount_point;
    snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, s);
    cgroup_blkio_base = config_get("plugin:cgroups", "path to /sys/fs/cgroup/blkio", filename);

    mi = mountinfo_find_by_filesystem_super_option(root, "cgroup", "memory");
    if(!mi) mi = mountinfo_find_by_filesystem_mount_source(root, "cgroup", "memory");
    if(!mi) {
        error("CGROUP: cannot find memory mountinfo. Assuming default: /sys/fs/cgroup/memory");
        s = "/sys/fs/cgroup/memory";
    }
    else s = mi->mount_point;
    snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, s);
    cgroup_memory_base = config_get("plugin:cgroups", "path to /sys/fs/cgroup/memory", filename);

    mi = mountinfo_find_by_filesystem_super_option(root, "cgroup", "devices");
    if(!mi) mi = mountinfo_find_by_filesystem_mount_source(root, "cgroup", "devices");
    if(!mi) {
        error("CGROUP: cannot find devices mountinfo. Assuming default: /sys/fs/cgroup/devices");
        s = "/sys/fs/cgroup/devices";
    }
    else s = mi->mount_point;
    snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, s);
    cgroup_devices_base = config_get("plugin:cgroups", "path to /sys/fs/cgroup/devices", filename);

    cgroup_root_max = (int)config_get_number("plugin:cgroups", "max cgroups to allow", cgroup_root_max);
    cgroup_max_depth = (int)config_get_number("plugin:cgroups", "max cgroups depth to monitor", cgroup_max_depth);

    cgroup_enable_new_cgroups_detected_at_runtime = config_get_boolean("plugin:cgroups", "enable new cgroups detected at run time", cgroup_enable_new_cgroups_detected_at_runtime);

    enabled_cgroup_patterns = simple_pattern_create(
            config_get("plugin:cgroups", "enable by default cgroups matching",
            // ----------------------------------------------------------------

                    " !*/init.scope "                      // ignore init.scope
                    " *.scope "                            // we need all *.scope for sure

            // ----------------------------------------------------------------

                    " !*/vcpu* "                           // libvirtd adds these sub-cgroups
                    " !*/emulator "                        // libvirtd adds these sub-cgroups
                    " !*.mount "
                    " !*.partition "
                    " !*.service "
                    " !*.slice "
                    " !*.swap "
                    " !*.user "
                    " !/ "
                    " !/docker "
                    " !/libvirt "
                    " !/lxc "
                    " !/lxc/*/ns "                         //  #1397
                    " !/machine "
                    " !/qemu "
                    " !/system "
                    " !/systemd "
                    " !/user "
                    " * "                                  // enable anything else
            ), SIMPLE_PATTERN_EXACT);

    enabled_cgroup_paths = simple_pattern_create(
            config_get("plugin:cgroups", "search for cgroups in subpaths matching",
                    " !*/init.scope "                      // ignore init.scope
                    " !*-qemu "                            //  #345
                    " !/init.scope "
                    " !/system "
                    " !/systemd "
                    " !/user "
                    " !/user.slice "
                    " !/lxc/*/ns/* "                       //  #2161
                    " * "
            ), SIMPLE_PATTERN_EXACT);

    snprintfz(filename, FILENAME_MAX, "%s/cgroup-name.sh", netdata_configured_plugins_dir);
    cgroups_rename_script = config_get("plugin:cgroups", "script to get cgroup names", filename);

    enabled_cgroup_renames = simple_pattern_create(
            config_get("plugin:cgroups", "run script to rename cgroups matching",
                    " *.scope "
                    " *docker* "
                    " *lxc* "
                    " *qemu* "
                    " !/ "
                    " !*.mount "
                    " !*.partition "
                    " !*.service "
                    " !*.slice "
                    " !*.swap "
                    " !*.user "
                    " * "
            ), SIMPLE_PATTERN_EXACT);

    if(cgroup_enable_systemd_services) {
        systemd_services_cgroups = simple_pattern_create(
                config_get("plugin:cgroups", "cgroups to match as systemd services",
                        " !/system.slice/*/*.service "
                        " /system.slice/*.service "
                ), SIMPLE_PATTERN_EXACT);
    }

    mountinfo_free(root);
}

// ----------------------------------------------------------------------------
// cgroup objects

struct blkio {
    int updated;
    int enabled; // CONFIG_BOOLEAN_YES or CONFIG_BOOLEAN_AUTO
    int delay_counter;

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
    ARL_BASE *arl_base;
    ARL_ENTRY *arl_dirty;
    ARL_ENTRY *arl_swap;

    int updated_detailed;
    int updated_usage_in_bytes;
    int updated_msw_usage_in_bytes;
    int updated_failcnt;

    int enabled_detailed;           // CONFIG_BOOLEAN_YES or CONFIG_BOOLEAN_AUTO
    int enabled_usage_in_bytes;     // CONFIG_BOOLEAN_YES or CONFIG_BOOLEAN_AUTO
    int enabled_msw_usage_in_bytes; // CONFIG_BOOLEAN_YES or CONFIG_BOOLEAN_AUTO
    int enabled_failcnt;            // CONFIG_BOOLEAN_YES or CONFIG_BOOLEAN_AUTO

    int delay_counter_detailed;
    int delay_counter_failcnt;

    char *filename_detailed;
    char *filename_usage_in_bytes;
    char *filename_msw_usage_in_bytes;
    char *filename_failcnt;

    int detailed_has_dirty;
    int detailed_has_swap;

    // detailed metrics
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

    // single file metrics
    unsigned long long usage_in_bytes;
    unsigned long long msw_usage_in_bytes;
    unsigned long long failcnt;
};

// https://www.kernel.org/doc/Documentation/cgroup-v1/cpuacct.txt
struct cpuacct_stat {
    int updated;
    int enabled; // CONFIG_BOOLEAN_YES or CONFIG_BOOLEAN_AUTO

    char *filename;

    unsigned long long user;
    unsigned long long system;
};

// https://www.kernel.org/doc/Documentation/cgroup-v1/cpuacct.txt
struct cpuacct_usage {
    int updated;
    int enabled; // CONFIG_BOOLEAN_YES or CONFIG_BOOLEAN_AUTO

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

    // per cgroup charts
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

    // services
    RRDDIM *rd_cpu;
    RRDDIM *rd_mem_usage;
    RRDDIM *rd_mem_failcnt;
    RRDDIM *rd_swap_usage;

    RRDDIM *rd_mem_detailed_cache;
    RRDDIM *rd_mem_detailed_rss;
    RRDDIM *rd_mem_detailed_mapped;
    RRDDIM *rd_mem_detailed_writeback;
    RRDDIM *rd_mem_detailed_pgpgin;
    RRDDIM *rd_mem_detailed_pgpgout;
    RRDDIM *rd_mem_detailed_pgfault;
    RRDDIM *rd_mem_detailed_pgmajfault;

    RRDDIM *rd_io_service_bytes_read;
    RRDDIM *rd_io_serviced_read;
    RRDDIM *rd_throttle_io_read;
    RRDDIM *rd_throttle_io_serviced_read;
    RRDDIM *rd_io_queued_read;
    RRDDIM *rd_io_merged_read;

    RRDDIM *rd_io_service_bytes_write;
    RRDDIM *rd_io_serviced_write;
    RRDDIM *rd_throttle_io_write;
    RRDDIM *rd_throttle_io_serviced_write;
    RRDDIM *rd_io_queued_write;
    RRDDIM *rd_io_merged_write;

    struct cgroup *next;

} *cgroup_root = NULL;

// ----------------------------------------------------------------------------
// read values from /sys

static inline void cgroup_read_cpuacct_stat(struct cpuacct_stat *cp) {
    static procfile *ff = NULL;

    if(likely(cp->filename)) {
        ff = procfile_reopen(ff, cp->filename, NULL, PROCFILE_FLAG_DEFAULT);
        if(unlikely(!ff)) {
            cp->updated = 0;
            cgroups_check = 1;
            return;
        }

        ff = procfile_readall(ff);
        if(unlikely(!ff)) {
            cp->updated = 0;
            cgroups_check = 1;
            return;
        }

        unsigned long i, lines = procfile_lines(ff);

        if(unlikely(lines < 1)) {
            error("CGROUP: file '%s' should have 1+ lines.", cp->filename);
            cp->updated = 0;
            return;
        }

        for(i = 0; i < lines ; i++) {
            char *s = procfile_lineword(ff, i, 0);
            uint32_t hash = simple_hash(s);

            if(unlikely(hash == user_hash && !strcmp(s, "user")))
                cp->user = str2ull(procfile_lineword(ff, i, 1));

            else if(unlikely(hash == system_hash && !strcmp(s, "system")))
                cp->system = str2ull(procfile_lineword(ff, i, 1));
        }

        cp->updated = 1;

        if(unlikely(cp->enabled == CONFIG_BOOLEAN_AUTO && (cp->user || cp->system)))
            cp->enabled = CONFIG_BOOLEAN_YES;
    }
}

static inline void cgroup_read_cpuacct_usage(struct cpuacct_usage *ca) {
    static procfile *ff = NULL;

    if(likely(ca->filename)) {
        ff = procfile_reopen(ff, ca->filename, NULL, PROCFILE_FLAG_DEFAULT);
        if(unlikely(!ff)) {
            ca->updated = 0;
            cgroups_check = 1;
            return;
        }

        ff = procfile_readall(ff);
        if(unlikely(!ff)) {
            ca->updated = 0;
            cgroups_check = 1;
            return;
        }

        if(unlikely(procfile_lines(ff) < 1)) {
            error("CGROUP: file '%s' should have 1+ lines but has %zu.", ca->filename, procfile_lines(ff));
            ca->updated = 0;
            return;
        }

        unsigned long i = procfile_linewords(ff, 0);
        if(unlikely(i == 0)) {
            ca->updated = 0;
            return;
        }

        // we may have 1 more CPU reported
        while(i > 0) {
            char *s = procfile_lineword(ff, 0, i - 1);
            if(!*s) i--;
            else break;
        }

        if(unlikely(i != ca->cpus)) {
            freez(ca->cpu_percpu);
            ca->cpu_percpu = mallocz(sizeof(unsigned long long) * i);
            ca->cpus = (unsigned int)i;
        }

        unsigned long long total = 0;
        for(i = 0; i < ca->cpus ;i++) {
            unsigned long long n = str2ull(procfile_lineword(ff, 0, i));
            ca->cpu_percpu[i] = n;
            total += n;
        }

        ca->updated = 1;

        if(unlikely(ca->enabled == CONFIG_BOOLEAN_AUTO && total))
            ca->enabled = CONFIG_BOOLEAN_YES;
    }
}

static inline void cgroup_read_blkio(struct blkio *io) {
    static procfile *ff = NULL;

    if(unlikely(io->enabled == CONFIG_BOOLEAN_AUTO && io->delay_counter > 0)) {
        io->delay_counter--;
        return;
    }

    if(likely(io->filename)) {
        ff = procfile_reopen(ff, io->filename, NULL, PROCFILE_FLAG_DEFAULT);
        if(unlikely(!ff)) {
            io->updated = 0;
            cgroups_check = 1;
            return;
        }

        ff = procfile_readall(ff);
        if(unlikely(!ff)) {
            io->updated = 0;
            cgroups_check = 1;
            return;
        }

        unsigned long i, lines = procfile_lines(ff);

        if(unlikely(lines < 1)) {
            error("CGROUP: file '%s' should have 1+ lines.", io->filename);
            io->updated = 0;
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

            if(unlikely(hash == Read_hash && !strcmp(s, "Read")))
                io->Read += str2ull(procfile_lineword(ff, i, 2));

            else if(unlikely(hash == Write_hash && !strcmp(s, "Write")))
                io->Write += str2ull(procfile_lineword(ff, i, 2));

/*
            else if(unlikely(hash == Sync_hash && !strcmp(s, "Sync")))
                io->Sync += str2ull(procfile_lineword(ff, i, 2));

            else if(unlikely(hash == Async_hash && !strcmp(s, "Async")))
                io->Async += str2ull(procfile_lineword(ff, i, 2));

            else if(unlikely(hash == Total_hash && !strcmp(s, "Total")))
                io->Total += str2ull(procfile_lineword(ff, i, 2));
*/
        }

        io->updated = 1;

        if(unlikely(io->enabled == CONFIG_BOOLEAN_AUTO)) {
            if(unlikely(io->Read || io->Write))
                io->enabled = CONFIG_BOOLEAN_YES;
            else
                io->delay_counter = cgroup_recheck_zero_blkio_every_iterations;
        }
    }
}

static inline void cgroup_read_memory(struct memory *mem) {
    static procfile *ff = NULL;

    // read detailed ram usage
    if(likely(mem->filename_detailed)) {
        if(unlikely(mem->enabled_detailed == CONFIG_BOOLEAN_AUTO && mem->delay_counter_detailed > 0)) {
            mem->delay_counter_detailed--;
            goto memory_next;
        }

        ff = procfile_reopen(ff, mem->filename_detailed, NULL, PROCFILE_FLAG_DEFAULT);
        if(unlikely(!ff)) {
            mem->updated_detailed = 0;
            cgroups_check = 1;
            goto memory_next;
        }

        ff = procfile_readall(ff);
        if(unlikely(!ff)) {
            mem->updated_detailed = 0;
            cgroups_check = 1;
            goto memory_next;
        }

        unsigned long i, lines = procfile_lines(ff);

        if(unlikely(lines < 1)) {
            error("CGROUP: file '%s' should have 1+ lines.", mem->filename_detailed);
            mem->updated_detailed = 0;
            goto memory_next;
        }

        if(unlikely(!mem->arl_base)) {
            mem->arl_base = arl_create("cgroup/memory", NULL, 60);

            arl_expect(mem->arl_base, "cache", &mem->cache);
            arl_expect(mem->arl_base, "rss", &mem->rss);
            arl_expect(mem->arl_base, "rss_huge", &mem->rss_huge);
            arl_expect(mem->arl_base, "mapped_file", &mem->mapped_file);
            arl_expect(mem->arl_base, "writeback", &mem->writeback);
            mem->arl_dirty = arl_expect(mem->arl_base, "dirty", &mem->dirty);
            mem->arl_swap  = arl_expect(mem->arl_base, "swap", &mem->swap);
            arl_expect(mem->arl_base, "pgpgin", &mem->pgpgin);
            arl_expect(mem->arl_base, "pgpgout", &mem->pgpgout);
            arl_expect(mem->arl_base, "pgfault", &mem->pgfault);
            arl_expect(mem->arl_base, "pgmajfault", &mem->pgmajfault);
        }

        arl_begin(mem->arl_base);

        for(i = 0; i < lines ; i++) {
            if(arl_check(mem->arl_base,
                    procfile_lineword(ff, i, 0),
                    procfile_lineword(ff, i, 1))) break;
        }

        if(unlikely(mem->arl_dirty->flags & ARL_ENTRY_FLAG_FOUND))
            mem->detailed_has_dirty = 1;

        if(unlikely(mem->arl_swap->flags & ARL_ENTRY_FLAG_FOUND))
            mem->detailed_has_swap = 1;

        // fprintf(stderr, "READ: '%s', cache: %llu, rss: %llu, rss_huge: %llu, mapped_file: %llu, writeback: %llu, dirty: %llu, swap: %llu, pgpgin: %llu, pgpgout: %llu, pgfault: %llu, pgmajfault: %llu, inactive_anon: %llu, active_anon: %llu, inactive_file: %llu, active_file: %llu, unevictable: %llu, hierarchical_memory_limit: %llu, total_cache: %llu, total_rss: %llu, total_rss_huge: %llu, total_mapped_file: %llu, total_writeback: %llu, total_dirty: %llu, total_swap: %llu, total_pgpgin: %llu, total_pgpgout: %llu, total_pgfault: %llu, total_pgmajfault: %llu, total_inactive_anon: %llu, total_active_anon: %llu, total_inactive_file: %llu, total_active_file: %llu, total_unevictable: %llu\n", mem->filename, mem->cache, mem->rss, mem->rss_huge, mem->mapped_file, mem->writeback, mem->dirty, mem->swap, mem->pgpgin, mem->pgpgout, mem->pgfault, mem->pgmajfault, mem->inactive_anon, mem->active_anon, mem->inactive_file, mem->active_file, mem->unevictable, mem->hierarchical_memory_limit, mem->total_cache, mem->total_rss, mem->total_rss_huge, mem->total_mapped_file, mem->total_writeback, mem->total_dirty, mem->total_swap, mem->total_pgpgin, mem->total_pgpgout, mem->total_pgfault, mem->total_pgmajfault, mem->total_inactive_anon, mem->total_active_anon, mem->total_inactive_file, mem->total_active_file, mem->total_unevictable);

        mem->updated_detailed = 1;

        if(unlikely(mem->enabled_detailed == CONFIG_BOOLEAN_AUTO)) {
            if(mem->cache || mem->dirty || mem->rss || mem->rss_huge || mem->mapped_file || mem->writeback || mem->swap || mem->pgpgin || mem->pgpgout || mem->pgfault || mem->pgmajfault)
                mem->enabled_detailed = CONFIG_BOOLEAN_YES;
            else
                mem->delay_counter_detailed = cgroup_recheck_zero_mem_detailed_every_iterations;
        }
    }

memory_next:

    // read usage_in_bytes
    if(likely(mem->filename_usage_in_bytes)) {
        mem->updated_usage_in_bytes = !read_single_number_file(mem->filename_usage_in_bytes, &mem->usage_in_bytes);
        if(unlikely(mem->updated_usage_in_bytes && mem->enabled_usage_in_bytes == CONFIG_BOOLEAN_AUTO && mem->usage_in_bytes))
            mem->enabled_usage_in_bytes = CONFIG_BOOLEAN_YES;
    }

    // read msw_usage_in_bytes
    if(likely(mem->filename_msw_usage_in_bytes)) {
        mem->updated_msw_usage_in_bytes = !read_single_number_file(mem->filename_msw_usage_in_bytes, &mem->msw_usage_in_bytes);
        if(unlikely(mem->updated_msw_usage_in_bytes && mem->enabled_msw_usage_in_bytes == CONFIG_BOOLEAN_AUTO && mem->msw_usage_in_bytes))
            mem->enabled_msw_usage_in_bytes = CONFIG_BOOLEAN_YES;
    }

    // read failcnt
    if(likely(mem->filename_failcnt)) {
        if(unlikely(mem->enabled_failcnt == CONFIG_BOOLEAN_AUTO && mem->delay_counter_failcnt > 0)) {
            mem->updated_failcnt = 0;
            mem->delay_counter_failcnt--;
        }
        else {
            mem->updated_failcnt = !read_single_number_file(mem->filename_failcnt, &mem->failcnt);
            if(unlikely(mem->updated_failcnt && mem->enabled_failcnt == CONFIG_BOOLEAN_AUTO)) {
                if(unlikely(!mem->failcnt))
                    mem->delay_counter_failcnt = cgroup_recheck_zero_mem_failcnt_every_iterations;
                else
                    mem->enabled_failcnt = CONFIG_BOOLEAN_YES;
            }
        }
    }
}

static inline void cgroup_read(struct cgroup *cg) {
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

static inline void read_all_cgroups(struct cgroup *root) {
    debug(D_CGROUP, "reading metrics for all cgroups");

    struct cgroup *cg;

    for(cg = root; cg ; cg = cg->next)
        if(cg->enabled && cg->available)
            cgroup_read(cg);
}

// ----------------------------------------------------------------------------
// add/remove/find cgroup objects

#define CGROUP_CHARTID_LINE_MAX 1024

static inline char *cgroup_title_strdupz(const char *s) {
    if(!s || !*s) s = "/";

    if(*s == '/' && s[1] != '\0') s++;

    char *r = strdupz(s);
    netdata_fix_chart_name(r);

    return r;
}

static inline char *cgroup_chart_id_strdupz(const char *s) {
    if(!s || !*s) s = "/";

    if(*s == '/' && s[1] != '\0') s++;

    char *r = strdupz(s);
    netdata_fix_chart_id(r);

    return r;
}

static inline void cgroup_get_chart_name(struct cgroup *cg) {
    debug(D_CGROUP, "looking for the name of cgroup '%s' with chart id '%s' and title '%s'", cg->id, cg->chart_id, cg->chart_title);

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
        error("CGROUP: cannot popen(\"%s\", \"r\").", buffer);
}

static inline struct cgroup *cgroup_add(const char *id) {
    if(!id || !*id) id = "/";
    debug(D_CGROUP, "adding to list, cgroup with id '%s'", id);

    if(cgroup_root_count >= cgroup_root_max) {
        info("CGROUP: maximum number of cgroups reached (%d). Not adding cgroup '%s'", cgroup_root_count, id);
        return NULL;
    }

    int def = simple_pattern_matches(enabled_cgroup_patterns, id)?cgroup_enable_new_cgroups_detected_at_runtime:0;
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

    // fix the chart_id and title by calling the external script
    if(simple_pattern_matches(enabled_cgroup_renames, cg->id)) {

        cgroup_get_chart_name(cg);

        debug(D_CGROUP, "cgroup '%s' renamed to '%s' (title: '%s')", cg->id, cg->chart_id, cg->chart_title);
    }
    else
        debug(D_CGROUP, "cgroup '%s' will not be renamed - it matches the list of disabled cgroup renames (will be shown as '%s')", cg->id, cg->chart_id);

    int user_configurable = 1;

    // check if this cgroup should be a systemd service
    if(cgroup_enable_systemd_services) {
        if(simple_pattern_matches(systemd_services_cgroups, cg->id) ||
                simple_pattern_matches(systemd_services_cgroups, cg->chart_id)) {
            debug(D_CGROUP, "cgroup '%s' with chart id '%s' (title: '%s') matches systemd services cgroups", cg->id, cg->chart_id, cg->chart_title);

            char buffer[CGROUP_CHARTID_LINE_MAX + 1];
            cg->options |= CGROUP_OPTIONS_SYSTEM_SLICE_SERVICE;

            strncpy(buffer, cg->id, CGROUP_CHARTID_LINE_MAX);
            char *s = buffer;

            //freez(cg->chart_id);
            //cg->chart_id = cgroup_chart_id_strdupz(s);
            //cg->hash_chart = simple_hash(cg->chart_id);

            // skip to the last slash
            size_t len = strlen(s);
            while(len--) if(unlikely(s[len] == '/')) break;
            if(len) s = &s[len + 1];

            // remove extension
            len = strlen(s);
            while(len--) if(unlikely(s[len] == '.')) break;
            if(len) s[len] = '\0';

            freez(cg->chart_title);
            cg->chart_title = cgroup_title_strdupz(s);

            cg->enabled = 1;
            user_configurable = 0;

            debug(D_CGROUP, "cgroup '%s' renamed to '%s' (title: '%s')", cg->id, cg->chart_id, cg->chart_title);
        }
        else
            debug(D_CGROUP, "cgroup '%s' with chart id '%s' (title: '%s') does not match systemd services groups", cg->id, cg->chart_id, cg->chart_title);
    }

    if(user_configurable) {
        // allow the user to enable/disable this individualy
        char option[FILENAME_MAX + 1];
        snprintfz(option, FILENAME_MAX, "enable cgroup %s", cg->chart_title);
        cg->enabled = (char) config_get_boolean("plugin:cgroups", option, def);
    }

    // detect duplicate cgroups
    if(cg->enabled) {
        struct cgroup *t;
        for (t = cgroup_root; t; t = t->next) {
            if (t != cg && t->enabled && t->hash_chart == cg->hash_chart && !strcmp(t->chart_id, cg->chart_id)) {
                if (!strncmp(t->chart_id, "/system.slice/", 14) && !strncmp(cg->chart_id, "/init.scope/system.slice/", 25)) {
                    error("CGROUP: chart id '%s' already exists with id '%s' and is enabled. Swapping them by enabling cgroup with id '%s' and disabling cgroup with id '%s'.",
                          cg->chart_id, t->id, cg->id, t->id);
                    debug(D_CGROUP, "Control group with chart id '%s' already exists with id '%s' and is enabled. Swapping them by enabling cgroup with id '%s' and disabling cgroup with id '%s'.",
                          cg->chart_id, t->id, cg->id, t->id);
                    t->enabled = 0;
                    t->options |= CGROUP_OPTIONS_DISABLED_DUPLICATE;
                }
                else {
                    error("CGROUP: chart id '%s' already exists with id '%s' and is enabled and available. Disabling cgroup with id '%s'.",
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

static inline void cgroup_free(struct cgroup *cg) {
    debug(D_CGROUP, "Removing cgroup '%s' with chart id '%s' (was %s and %s)", cg->id, cg->chart_id, (cg->enabled)?"enabled":"disabled", (cg->available)?"available":"not available");

    if(cg->st_cpu)                   rrdset_is_obsolete(cg->st_cpu);
    if(cg->st_cpu_per_core)          rrdset_is_obsolete(cg->st_cpu_per_core);
    if(cg->st_mem)                   rrdset_is_obsolete(cg->st_mem);
    if(cg->st_writeback)             rrdset_is_obsolete(cg->st_writeback);
    if(cg->st_mem_activity)          rrdset_is_obsolete(cg->st_mem_activity);
    if(cg->st_pgfaults)              rrdset_is_obsolete(cg->st_pgfaults);
    if(cg->st_mem_usage)             rrdset_is_obsolete(cg->st_mem_usage);
    if(cg->st_mem_failcnt)           rrdset_is_obsolete(cg->st_mem_failcnt);
    if(cg->st_io)                    rrdset_is_obsolete(cg->st_io);
    if(cg->st_serviced_ops)          rrdset_is_obsolete(cg->st_serviced_ops);
    if(cg->st_throttle_io)           rrdset_is_obsolete(cg->st_throttle_io);
    if(cg->st_throttle_serviced_ops) rrdset_is_obsolete(cg->st_throttle_serviced_ops);
    if(cg->st_queued_ops)            rrdset_is_obsolete(cg->st_queued_ops);
    if(cg->st_merged_ops)            rrdset_is_obsolete(cg->st_merged_ops);

    freez(cg->cpuacct_usage.cpu_percpu);

    freez(cg->cpuacct_stat.filename);
    freez(cg->cpuacct_usage.filename);

    arl_free(cg->memory.arl_base);
    freez(cg->memory.filename_detailed);
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
static inline struct cgroup *cgroup_find(const char *id) {
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
static inline void found_subdir_in_dir(const char *dir) {
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
                info("CGROUP: '%s' is too deep (%d, while max is %d)", dir, depth, cgroup_max_depth);
                return;
            }
        }
        // debug(D_CGROUP, "will add dir '%s' as cgroup", dir);
        cg = cgroup_add(dir);
    }

    if(cg) cg->available = 1;
}

static inline int find_dir_in_subdirs(const char *base, const char *this, void (*callback)(const char *)) {
    if(!this) this = base;
    debug(D_CGROUP, "searching for directories in '%s' (base '%s')", this?this:"", base);

    size_t dirlen = strlen(this), baselen = strlen(base);

    int ret = -1;
    int enabled = -1;

    const char *relative_path = &this[baselen];
    if(!*relative_path) relative_path = "/";

    DIR *dir = opendir(this);
    if(!dir) {
        error("CGROUP: cannot read directory '%s'", base);
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
                int def = simple_pattern_matches(enabled_cgroup_paths, r);

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

static inline void mark_all_cgroups_as_not_available() {
    debug(D_CGROUP, "marking all cgroups as not available");

    struct cgroup *cg;

    // mark all as not available
    for(cg = cgroup_root; cg ; cg = cg->next) {
        cg->available = 0;
    }
}

static inline void cleanup_all_cgroups() {
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

static inline void find_all_cgroups() {
    debug(D_CGROUP, "searching for cgroups");

    mark_all_cgroups_as_not_available();

    if(cgroup_enable_cpuacct_stat || cgroup_enable_cpuacct_usage) {
        if(find_dir_in_subdirs(cgroup_cpuacct_base, NULL, found_subdir_in_dir) == -1) {
            cgroup_enable_cpuacct_stat =
            cgroup_enable_cpuacct_usage = CONFIG_BOOLEAN_NO;
            error("CGROUP: disabled cpu statistics.");
        }
    }

    if(cgroup_enable_blkio_io || cgroup_enable_blkio_ops || cgroup_enable_blkio_throttle_io || cgroup_enable_blkio_throttle_ops || cgroup_enable_blkio_merged_ops || cgroup_enable_blkio_queued_ops) {
        if(find_dir_in_subdirs(cgroup_blkio_base, NULL, found_subdir_in_dir) == -1) {
            cgroup_enable_blkio_io =
            cgroup_enable_blkio_ops =
            cgroup_enable_blkio_throttle_io =
            cgroup_enable_blkio_throttle_ops =
            cgroup_enable_blkio_merged_ops =
            cgroup_enable_blkio_queued_ops = CONFIG_BOOLEAN_NO;
            error("CGROUP: disabled blkio statistics.");
        }
    }

    if(cgroup_enable_memory || cgroup_enable_detailed_memory || cgroup_enable_swap || cgroup_enable_memory_failcnt) {
        if(find_dir_in_subdirs(cgroup_memory_base, NULL, found_subdir_in_dir) == -1) {
            cgroup_enable_memory =
            cgroup_enable_detailed_memory =
            cgroup_enable_swap =
            cgroup_enable_memory_failcnt = CONFIG_BOOLEAN_NO;
            error("CGROUP: disabled memory statistics.");
        }
    }

    if(cgroup_search_in_devices) {
        if(find_dir_in_subdirs(cgroup_devices_base, NULL, found_subdir_in_dir) == -1) {
            cgroup_search_in_devices = 0;
            error("CGROUP: disabled devices statistics.");
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
            if(likely(stat(filename, &buf) != -1)) {
                cg->cpuacct_stat.filename = strdupz(filename);
                cg->cpuacct_stat.enabled = cgroup_enable_cpuacct_stat;
                debug(D_CGROUP, "cpuacct.stat filename for cgroup '%s': '%s'", cg->id, cg->cpuacct_stat.filename);
            }
            else
                debug(D_CGROUP, "cpuacct.stat file for cgroup '%s': '%s' does not exist.", cg->id, filename);
        }

        if(unlikely(cgroup_enable_cpuacct_usage && !cg->cpuacct_usage.filename && !(cg->options & CGROUP_OPTIONS_SYSTEM_SLICE_SERVICE))) {
            snprintfz(filename, FILENAME_MAX, "%s%s/cpuacct.usage_percpu", cgroup_cpuacct_base, cg->id);
            if(likely(stat(filename, &buf) != -1)) {
                cg->cpuacct_usage.filename = strdupz(filename);
                cg->cpuacct_usage.enabled = cgroup_enable_cpuacct_usage;
                debug(D_CGROUP, "cpuacct.usage_percpu filename for cgroup '%s': '%s'", cg->id, cg->cpuacct_usage.filename);
            }
            else
                debug(D_CGROUP, "cpuacct.usage_percpu file for cgroup '%s': '%s' does not exist.", cg->id, filename);
        }

        if(unlikely((cgroup_enable_detailed_memory || cgroup_used_memory_without_cache) && !cg->memory.filename_detailed && (cgroup_used_memory_without_cache || cgroup_enable_systemd_services_detailed_memory || !(cg->options & CGROUP_OPTIONS_SYSTEM_SLICE_SERVICE)))) {
            snprintfz(filename, FILENAME_MAX, "%s%s/memory.stat", cgroup_memory_base, cg->id);
            if(likely(stat(filename, &buf) != -1)) {
                cg->memory.filename_detailed = strdupz(filename);
                cg->memory.enabled_detailed = (cgroup_enable_detailed_memory == CONFIG_BOOLEAN_YES)?CONFIG_BOOLEAN_YES:CONFIG_BOOLEAN_AUTO;
                debug(D_CGROUP, "memory.stat filename for cgroup '%s': '%s'", cg->id, cg->memory.filename_detailed);
            }
            else
                debug(D_CGROUP, "memory.stat file for cgroup '%s': '%s' does not exist.", cg->id, filename);
        }

        if(unlikely(cgroup_enable_memory && !cg->memory.filename_usage_in_bytes)) {
            snprintfz(filename, FILENAME_MAX, "%s%s/memory.usage_in_bytes", cgroup_memory_base, cg->id);
            if(likely(stat(filename, &buf) != -1)) {
                cg->memory.filename_usage_in_bytes = strdupz(filename);
                cg->memory.enabled_usage_in_bytes = cgroup_enable_memory;
                debug(D_CGROUP, "memory.usage_in_bytes filename for cgroup '%s': '%s'", cg->id, cg->memory.filename_usage_in_bytes);
            }
            else
                debug(D_CGROUP, "memory.usage_in_bytes file for cgroup '%s': '%s' does not exist.", cg->id, filename);
        }

        if(unlikely(cgroup_enable_swap && !cg->memory.filename_msw_usage_in_bytes)) {
            snprintfz(filename, FILENAME_MAX, "%s%s/memory.msw_usage_in_bytes", cgroup_memory_base, cg->id);
            if(likely(stat(filename, &buf) != -1)) {
                cg->memory.filename_msw_usage_in_bytes = strdupz(filename);
                cg->memory.enabled_msw_usage_in_bytes = cgroup_enable_swap;
                debug(D_CGROUP, "memory.msw_usage_in_bytes filename for cgroup '%s': '%s'", cg->id, cg->memory.filename_msw_usage_in_bytes);
            }
            else
                debug(D_CGROUP, "memory.msw_usage_in_bytes file for cgroup '%s': '%s' does not exist.", cg->id, filename);
        }

        if(unlikely(cgroup_enable_memory_failcnt && !cg->memory.filename_failcnt)) {
            snprintfz(filename, FILENAME_MAX, "%s%s/memory.failcnt", cgroup_memory_base, cg->id);
            if(likely(stat(filename, &buf) != -1)) {
                cg->memory.filename_failcnt = strdupz(filename);
                cg->memory.enabled_failcnt = cgroup_enable_memory_failcnt;
                debug(D_CGROUP, "memory.failcnt filename for cgroup '%s': '%s'", cg->id, cg->memory.filename_failcnt);
            }
            else
                debug(D_CGROUP, "memory.failcnt file for cgroup '%s': '%s' does not exist.", cg->id, filename);
        }

        if(unlikely(cgroup_enable_blkio_io && !cg->io_service_bytes.filename)) {
            snprintfz(filename, FILENAME_MAX, "%s%s/blkio.io_service_bytes", cgroup_blkio_base, cg->id);
            if(likely(stat(filename, &buf) != -1)) {
                cg->io_service_bytes.filename = strdupz(filename);
                cg->io_service_bytes.enabled = cgroup_enable_blkio_io;
                debug(D_CGROUP, "io_service_bytes filename for cgroup '%s': '%s'", cg->id, cg->io_service_bytes.filename);
            }
            else
                debug(D_CGROUP, "io_service_bytes file for cgroup '%s': '%s' does not exist.", cg->id, filename);
        }

        if(unlikely(cgroup_enable_blkio_ops && !cg->io_serviced.filename)) {
            snprintfz(filename, FILENAME_MAX, "%s%s/blkio.io_serviced", cgroup_blkio_base, cg->id);
            if(likely(stat(filename, &buf) != -1)) {
                cg->io_serviced.filename = strdupz(filename);
                cg->io_serviced.enabled = cgroup_enable_blkio_ops;
                debug(D_CGROUP, "io_serviced filename for cgroup '%s': '%s'", cg->id, cg->io_serviced.filename);
            }
            else
                debug(D_CGROUP, "io_serviced file for cgroup '%s': '%s' does not exist.", cg->id, filename);
        }

        if(unlikely(cgroup_enable_blkio_throttle_io && !cg->throttle_io_service_bytes.filename)) {
            snprintfz(filename, FILENAME_MAX, "%s%s/blkio.throttle.io_service_bytes", cgroup_blkio_base, cg->id);
            if(likely(stat(filename, &buf) != -1)) {
                cg->throttle_io_service_bytes.filename = strdupz(filename);
                cg->throttle_io_service_bytes.enabled = cgroup_enable_blkio_throttle_io;
                debug(D_CGROUP, "throttle_io_service_bytes filename for cgroup '%s': '%s'", cg->id, cg->throttle_io_service_bytes.filename);
            }
            else
                debug(D_CGROUP, "throttle_io_service_bytes file for cgroup '%s': '%s' does not exist.", cg->id, filename);
        }

        if(unlikely(cgroup_enable_blkio_throttle_ops && !cg->throttle_io_serviced.filename)) {
            snprintfz(filename, FILENAME_MAX, "%s%s/blkio.throttle.io_serviced", cgroup_blkio_base, cg->id);
            if(likely(stat(filename, &buf) != -1)) {
                cg->throttle_io_serviced.filename = strdupz(filename);
                cg->throttle_io_serviced.enabled = cgroup_enable_blkio_throttle_ops;
                debug(D_CGROUP, "throttle_io_serviced filename for cgroup '%s': '%s'", cg->id, cg->throttle_io_serviced.filename);
            }
            else
                debug(D_CGROUP, "throttle_io_serviced file for cgroup '%s': '%s' does not exist.", cg->id, filename);
        }

        if(unlikely(cgroup_enable_blkio_merged_ops && !cg->io_merged.filename)) {
            snprintfz(filename, FILENAME_MAX, "%s%s/blkio.io_merged", cgroup_blkio_base, cg->id);
            if(likely(stat(filename, &buf) != -1)) {
                cg->io_merged.filename = strdupz(filename);
                cg->io_merged.enabled = cgroup_enable_blkio_merged_ops;
                debug(D_CGROUP, "io_merged filename for cgroup '%s': '%s'", cg->id, cg->io_merged.filename);
            }
            else
                debug(D_CGROUP, "io_merged file for cgroup '%s': '%s' does not exist.", cg->id, filename);
        }

        if(unlikely(cgroup_enable_blkio_queued_ops && !cg->io_queued.filename)) {
            snprintfz(filename, FILENAME_MAX, "%s%s/blkio.io_queued", cgroup_blkio_base, cg->id);
            if(likely(stat(filename, &buf) != -1)) {
                cg->io_queued.filename = strdupz(filename);
                cg->io_queued.enabled = cgroup_enable_blkio_queued_ops;
                debug(D_CGROUP, "io_queued filename for cgroup '%s': '%s'", cg->id, cg->io_queued.filename);
            }
            else
                debug(D_CGROUP, "io_queued file for cgroup '%s': '%s' does not exist.", cg->id, filename);
        }
    }

    debug(D_CGROUP, "done searching for cgroups");
    return;
}

// ----------------------------------------------------------------------------
// generate charts

#define CHART_TITLE_MAX 300

void update_systemd_services_charts(
          int update_every
        , int do_cpu
        , int do_mem_usage
        , int do_mem_detailed
        , int do_mem_failcnt
        , int do_swap_usage
        , int do_io
        , int do_io_ops
        , int do_throttle_io
        , int do_throttle_ops
        , int do_queued_ops
        , int do_merged_ops
) {
    static RRDSET
        *st_cpu = NULL,
        *st_mem_usage = NULL,
        *st_mem_failcnt = NULL,
        *st_swap_usage = NULL,

        *st_mem_detailed_cache = NULL,
        *st_mem_detailed_rss = NULL,
        *st_mem_detailed_mapped = NULL,
        *st_mem_detailed_writeback = NULL,
        *st_mem_detailed_pgfault = NULL,
        *st_mem_detailed_pgmajfault = NULL,
        *st_mem_detailed_pgpgin = NULL,
        *st_mem_detailed_pgpgout = NULL,

        *st_io_read = NULL,
        *st_io_serviced_read = NULL,
        *st_throttle_io_read = NULL,
        *st_throttle_ops_read = NULL,
        *st_queued_ops_read = NULL,
        *st_merged_ops_read = NULL,

        *st_io_write = NULL,
        *st_io_serviced_write = NULL,
        *st_throttle_io_write = NULL,
        *st_throttle_ops_write = NULL,
        *st_queued_ops_write = NULL,
        *st_merged_ops_write = NULL;

    // create the charts

    if(likely(do_cpu)) {
        if(unlikely(!st_cpu)) {
            char title[CHART_TITLE_MAX + 1];
            snprintfz(title, CHART_TITLE_MAX, "Systemd Services CPU utilization (%d%% = %d core%s)", (processors * 100), processors, (processors > 1) ? "s" : "");

            st_cpu = rrdset_create_localhost(
                    "services"
                    , "cpu"
                    , NULL
                    , "cpu"
                    , "services.cpu"
                    , title
                    , "%"
                    , CHART_PRIORITY_SYSTEMD_SERVICES
                    , update_every
                    , RRDSET_TYPE_STACKED
            );

        }
        else
            rrdset_next(st_cpu);
    }

    if(likely(do_mem_usage)) {
        if(unlikely(!st_mem_usage)) {

            st_mem_usage = rrdset_create_localhost(
                    "services"
                    , "mem_usage"
                    , NULL
                    , "mem"
                    , "services.mem_usage"
                    , (cgroup_used_memory_without_cache) ? "Systemd Services Used Memory without Cache"
                                                         : "Systemd Services Used Memory"
                    , "MB"
                    , CHART_PRIORITY_SYSTEMD_SERVICES + 10
                    , update_every
                    , RRDSET_TYPE_STACKED
            );

        }
        else
            rrdset_next(st_mem_usage);
    }

    if(likely(do_mem_detailed)) {
        if(unlikely(!st_mem_detailed_rss)) {

            st_mem_detailed_rss = rrdset_create_localhost(
                    "services"
                    , "mem_rss"
                    , NULL
                    , "mem"
                    , "services.mem_rss"
                    , "Systemd Services RSS Memory"
                    , "MB"
                    , CHART_PRIORITY_SYSTEMD_SERVICES + 20
                    , update_every
                    , RRDSET_TYPE_STACKED
            );

        }
        else
            rrdset_next(st_mem_detailed_rss);

        if(unlikely(!st_mem_detailed_mapped)) {

            st_mem_detailed_mapped = rrdset_create_localhost(
                    "services"
                    , "mem_mapped"
                    , NULL
                    , "mem"
                    , "services.mem_mapped"
                    , "Systemd Services Mapped Memory"
                    , "MB"
                    , CHART_PRIORITY_SYSTEMD_SERVICES + 30
                    , update_every
                    , RRDSET_TYPE_STACKED
            );

        }
        else
            rrdset_next(st_mem_detailed_mapped);

        if(unlikely(!st_mem_detailed_cache)) {

            st_mem_detailed_cache = rrdset_create_localhost(
                    "services"
                    , "mem_cache"
                    , NULL
                    , "mem"
                    , "services.mem_cache"
                    , "Systemd Services Cache Memory"
                    , "MB"
                    , CHART_PRIORITY_SYSTEMD_SERVICES + 40
                    , update_every
                    , RRDSET_TYPE_STACKED
            );

        }
        else
            rrdset_next(st_mem_detailed_cache);

        if(unlikely(!st_mem_detailed_writeback)) {

            st_mem_detailed_writeback = rrdset_create_localhost(
                    "services"
                    , "mem_writeback"
                    , NULL
                    , "mem"
                    , "services.mem_writeback"
                    , "Systemd Services Writeback Memory"
                    , "MB"
                    , CHART_PRIORITY_SYSTEMD_SERVICES + 50
                    , update_every
                    , RRDSET_TYPE_STACKED
            );

        }
        else
            rrdset_next(st_mem_detailed_writeback);

        if(unlikely(!st_mem_detailed_pgfault)) {

            st_mem_detailed_pgfault = rrdset_create_localhost(
                    "services"
                    , "mem_pgfault"
                    , NULL
                    , "mem"
                    , "services.mem_pgfault"
                    , "Systemd Services Memory Minor Page Faults"
                    , "MB/s"
                    , CHART_PRIORITY_SYSTEMD_SERVICES + 60
                    , update_every
                    , RRDSET_TYPE_STACKED
            );
        }
        else
            rrdset_next(st_mem_detailed_pgfault);

        if(unlikely(!st_mem_detailed_pgmajfault)) {

            st_mem_detailed_pgmajfault = rrdset_create_localhost(
                    "services"
                    , "mem_pgmajfault"
                    , NULL
                    , "mem"
                    , "services.mem_pgmajfault"
                    , "Systemd Services Memory Major Page Faults"
                    , "MB/s"
                    , CHART_PRIORITY_SYSTEMD_SERVICES + 70
                    , update_every
                    , RRDSET_TYPE_STACKED
            );

        }
        else
            rrdset_next(st_mem_detailed_pgmajfault);

        if(unlikely(!st_mem_detailed_pgpgin)) {

            st_mem_detailed_pgpgin = rrdset_create_localhost(
                    "services"
                    , "mem_pgpgin"
                    , NULL
                    , "mem"
                    , "services.mem_pgpgin"
                    , "Systemd Services Memory Charging Activity"
                    , "MB/s"
                    , CHART_PRIORITY_SYSTEMD_SERVICES + 80
                    , update_every
                    , RRDSET_TYPE_STACKED
            );

        }
        else
            rrdset_next(st_mem_detailed_pgpgin);

        if(unlikely(!st_mem_detailed_pgpgout)) {

            st_mem_detailed_pgpgout = rrdset_create_localhost(
                    "services"
                    , "mem_pgpgout"
                    , NULL
                    , "mem"
                    , "services.mem_pgpgout"
                    , "Systemd Services Memory Uncharging Activity"
                    , "MB/s"
                    , CHART_PRIORITY_SYSTEMD_SERVICES + 90
                    , update_every
                    , RRDSET_TYPE_STACKED
            );

        }
        else
            rrdset_next(st_mem_detailed_pgpgout);
    }

    if(likely(do_mem_failcnt)) {
        if(unlikely(!st_mem_failcnt)) {

            st_mem_failcnt = rrdset_create_localhost(
                    "services"
                    , "mem_failcnt"
                    , NULL
                    , "mem"
                    , "services.mem_failcnt"
                    , "Systemd Services Memory Limit Failures"
                    , "MB"
                    , CHART_PRIORITY_SYSTEMD_SERVICES + 110
                    , update_every
                    , RRDSET_TYPE_STACKED
            );

        }
        else
            rrdset_next(st_mem_failcnt);
    }

    if(likely(do_swap_usage)) {
        if(unlikely(!st_swap_usage)) {

            st_swap_usage = rrdset_create_localhost(
                    "services"
                    , "swap_usage"
                    , NULL
                    , "swap"
                    , "services.swap_usage"
                    , "Systemd Services Swap Memory Used"
                    , "MB"
                    , CHART_PRIORITY_SYSTEMD_SERVICES + 100
                    , update_every
                    , RRDSET_TYPE_STACKED
            );

        }
        else
            rrdset_next(st_swap_usage);
    }

    if(likely(do_io)) {
        if(unlikely(!st_io_read)) {

            st_io_read = rrdset_create_localhost(
                    "services"
                    , "io_read"
                    , NULL
                    , "disk"
                    , "services.io_read"
                    , "Systemd Services Disk Read Bandwidth"
                    , "KB/s"
                    , CHART_PRIORITY_SYSTEMD_SERVICES + 120
                    , update_every
                    , RRDSET_TYPE_STACKED
            );

        }
        else
            rrdset_next(st_io_read);

        if(unlikely(!st_io_write)) {

            st_io_write = rrdset_create_localhost(
                    "services"
                    , "io_write"
                    , NULL
                    , "disk"
                    , "services.io_write"
                    , "Systemd Services Disk Write Bandwidth"
                    , "KB/s"
                    , CHART_PRIORITY_SYSTEMD_SERVICES + 130
                    , update_every
                    , RRDSET_TYPE_STACKED
            );

        }
        else
            rrdset_next(st_io_write);
    }

    if(likely(do_io_ops)) {
        if(unlikely(!st_io_serviced_read)) {

            st_io_serviced_read = rrdset_create_localhost(
                    "services"
                    , "io_ops_read"
                    , NULL
                    , "disk"
                    , "services.io_ops_read"
                    , "Systemd Services Disk Read Operations"
                    , "operations/s"
                    , CHART_PRIORITY_SYSTEMD_SERVICES + 140
                    , update_every
                    , RRDSET_TYPE_STACKED
            );

        }
        else
            rrdset_next(st_io_serviced_read);

        if(unlikely(!st_io_serviced_write)) {

            st_io_serviced_write = rrdset_create_localhost(
                    "services"
                    , "io_ops_write"
                    , NULL
                    , "disk"
                    , "services.io_ops_write"
                    , "Systemd Services Disk Write Operations"
                    , "operations/s"
                    , CHART_PRIORITY_SYSTEMD_SERVICES + 150
                    , update_every
                    , RRDSET_TYPE_STACKED
            );

        }
        else
            rrdset_next(st_io_serviced_write);
    }

    if(likely(do_throttle_io)) {
        if(unlikely(!st_throttle_io_read)) {

            st_throttle_io_read = rrdset_create_localhost(
                    "services"
                    , "throttle_io_read"
                    , NULL
                    , "disk"
                    , "services.throttle_io_read"
                    , "Systemd Services Throttle Disk Read Bandwidth"
                    , "KB/s"
                    , CHART_PRIORITY_SYSTEMD_SERVICES + 160
                    , update_every
                    , RRDSET_TYPE_STACKED
            );

        }
        else
            rrdset_next(st_throttle_io_read);

        if(unlikely(!st_throttle_io_write)) {

            st_throttle_io_write = rrdset_create_localhost(
                    "services"
                    , "throttle_io_write"
                    , NULL
                    , "disk"
                    , "services.throttle_io_write"
                    , "Systemd Services Throttle Disk Write Bandwidth"
                    , "KB/s"
                    , CHART_PRIORITY_SYSTEMD_SERVICES + 170
                    , update_every
                    , RRDSET_TYPE_STACKED
            );

        }
        else
            rrdset_next(st_throttle_io_write);
    }

    if(likely(do_throttle_ops)) {
        if(unlikely(!st_throttle_ops_read)) {

            st_throttle_ops_read = rrdset_create_localhost(
                    "services"
                    , "throttle_io_ops_read"
                    , NULL
                    , "disk"
                    , "services.throttle_io_ops_read"
                    , "Systemd Services Throttle Disk Read Operations"
                    , "operations/s"
                    , CHART_PRIORITY_SYSTEMD_SERVICES + 180
                    , update_every
                    , RRDSET_TYPE_STACKED
            );

        }
        else
            rrdset_next(st_throttle_ops_read);

        if(unlikely(!st_throttle_ops_write)) {

            st_throttle_ops_write = rrdset_create_localhost(
                    "services"
                    , "throttle_io_ops_write"
                    , NULL
                    , "disk"
                    , "services.throttle_io_ops_write"
                    , "Systemd Services Throttle Disk Write Operations"
                    , "operations/s"
                    , CHART_PRIORITY_SYSTEMD_SERVICES + 190
                    , update_every
                    , RRDSET_TYPE_STACKED
            );

        }
        else
            rrdset_next(st_throttle_ops_write);
    }

    if(likely(do_queued_ops)) {
        if(unlikely(!st_queued_ops_read)) {

            st_queued_ops_read = rrdset_create_localhost(
                    "services"
                    , "queued_io_ops_read"
                    , NULL
                    , "disk"
                    , "services.queued_io_ops_read"
                    , "Systemd Services Queued Disk Read Operations"
                    , "operations/s"
                    , CHART_PRIORITY_SYSTEMD_SERVICES + 200
                    , update_every
                    , RRDSET_TYPE_STACKED
            );

        }
        else
            rrdset_next(st_queued_ops_read);

        if(unlikely(!st_queued_ops_write)) {

            st_queued_ops_write = rrdset_create_localhost(
                    "services"
                    , "queued_io_ops_write"
                    , NULL
                    , "disk"
                    , "services.queued_io_ops_write"
                    , "Systemd Services Queued Disk Write Operations"
                    , "operations/s"
                    , CHART_PRIORITY_SYSTEMD_SERVICES + 210
                    , update_every
                    , RRDSET_TYPE_STACKED
            );

        }
        else
            rrdset_next(st_queued_ops_write);
    }

    if(likely(do_merged_ops)) {
        if(unlikely(!st_merged_ops_read)) {

            st_merged_ops_read = rrdset_create_localhost(
                    "services"
                    , "merged_io_ops_read"
                    , NULL
                    , "disk"
                    , "services.merged_io_ops_read"
                    , "Systemd Services Merged Disk Read Operations"
                    , "operations/s"
                    , CHART_PRIORITY_SYSTEMD_SERVICES + 220
                    , update_every
                    , RRDSET_TYPE_STACKED
            );

        }
        else
            rrdset_next(st_merged_ops_read);

        if(unlikely(!st_merged_ops_write)) {

            st_merged_ops_write = rrdset_create_localhost(
                    "services"
                    , "merged_io_ops_write"
                    , NULL
                    , "disk"
                    , "services.merged_io_ops_write"
                    , "Systemd Services Merged Disk Write Operations"
                    , "operations/s"
                    , CHART_PRIORITY_SYSTEMD_SERVICES + 230
                    , update_every
                    , RRDSET_TYPE_STACKED
            );

        }
        else
            rrdset_next(st_merged_ops_write);
    }

    // update the values
    struct cgroup *cg;
    for(cg = cgroup_root; cg ; cg = cg->next) {
        if(unlikely(!cg->available || !cg->enabled || !(cg->options & CGROUP_OPTIONS_SYSTEM_SLICE_SERVICE)))
            continue;

        if(likely(do_cpu && cg->cpuacct_stat.updated)) {
            if(unlikely(!cg->rd_cpu))
                cg->rd_cpu = rrddim_add(st_cpu, cg->chart_id, cg->chart_title, 100, hz, RRD_ALGORITHM_INCREMENTAL);

            rrddim_set_by_pointer(st_cpu, cg->rd_cpu, cg->cpuacct_stat.user + cg->cpuacct_stat.system);
        }

        if(likely(do_mem_usage && cg->memory.updated_usage_in_bytes)) {
            if(unlikely(!cg->rd_mem_usage))
                cg->rd_mem_usage = rrddim_add(st_mem_usage, cg->chart_id, cg->chart_title, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);

            rrddim_set_by_pointer(st_mem_usage, cg->rd_mem_usage, cg->memory.usage_in_bytes - ((cgroup_used_memory_without_cache)?cg->memory.cache:0));
        }

        if(likely(do_mem_detailed && cg->memory.updated_detailed)) {
            if(unlikely(!cg->rd_mem_detailed_rss))
                cg->rd_mem_detailed_rss = rrddim_add(st_mem_detailed_rss, cg->chart_id, cg->chart_title, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);

            rrddim_set_by_pointer(st_mem_detailed_rss, cg->rd_mem_detailed_rss, cg->memory.rss + cg->memory.rss_huge);

            if(unlikely(!cg->rd_mem_detailed_mapped))
                cg->rd_mem_detailed_mapped = rrddim_add(st_mem_detailed_mapped, cg->chart_id, cg->chart_title, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);

            rrddim_set_by_pointer(st_mem_detailed_mapped, cg->rd_mem_detailed_mapped, cg->memory.mapped_file);

            if(unlikely(!cg->rd_mem_detailed_cache))
                cg->rd_mem_detailed_cache = rrddim_add(st_mem_detailed_cache, cg->chart_id, cg->chart_title, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);

            rrddim_set_by_pointer(st_mem_detailed_cache, cg->rd_mem_detailed_cache, cg->memory.cache);

            if(unlikely(!cg->rd_mem_detailed_writeback))
                cg->rd_mem_detailed_writeback = rrddim_add(st_mem_detailed_writeback, cg->chart_id, cg->chart_title, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);

            rrddim_set_by_pointer(st_mem_detailed_writeback, cg->rd_mem_detailed_writeback, cg->memory.writeback);

            if(unlikely(!cg->rd_mem_detailed_pgfault))
                cg->rd_mem_detailed_pgfault = rrddim_add(st_mem_detailed_pgfault, cg->chart_id, cg->chart_title, system_page_size, 1024 * 1024, RRD_ALGORITHM_INCREMENTAL);

            rrddim_set_by_pointer(st_mem_detailed_pgfault, cg->rd_mem_detailed_pgfault, cg->memory.pgfault);

            if(unlikely(!cg->rd_mem_detailed_pgmajfault))
                cg->rd_mem_detailed_pgmajfault = rrddim_add(st_mem_detailed_pgmajfault, cg->chart_id, cg->chart_title, system_page_size, 1024 * 1024, RRD_ALGORITHM_INCREMENTAL);

            rrddim_set_by_pointer(st_mem_detailed_pgmajfault, cg->rd_mem_detailed_pgmajfault, cg->memory.pgmajfault);

            if(unlikely(!cg->rd_mem_detailed_pgpgin))
                cg->rd_mem_detailed_pgpgin = rrddim_add(st_mem_detailed_pgpgin, cg->chart_id, cg->chart_title, system_page_size, 1024 * 1024, RRD_ALGORITHM_INCREMENTAL);

            rrddim_set_by_pointer(st_mem_detailed_pgpgin, cg->rd_mem_detailed_pgpgin, cg->memory.pgpgin);

            if(unlikely(!cg->rd_mem_detailed_pgpgout))
                cg->rd_mem_detailed_pgpgout = rrddim_add(st_mem_detailed_pgpgout, cg->chart_id, cg->chart_title, system_page_size, 1024 * 1024, RRD_ALGORITHM_INCREMENTAL);

            rrddim_set_by_pointer(st_mem_detailed_pgpgout, cg->rd_mem_detailed_pgpgout, cg->memory.pgpgout);
        }

        if(likely(do_mem_failcnt && cg->memory.updated_failcnt)) {
            if(unlikely(!cg->rd_mem_failcnt))
                cg->rd_mem_failcnt = rrddim_add(st_mem_failcnt, cg->chart_id, cg->chart_title, 1, 1, RRD_ALGORITHM_INCREMENTAL);

            rrddim_set_by_pointer(st_mem_failcnt, cg->rd_mem_failcnt, cg->memory.failcnt);
        }

        if(likely(do_swap_usage && cg->memory.updated_msw_usage_in_bytes)) {
            if(unlikely(!cg->rd_swap_usage))
                cg->rd_swap_usage = rrddim_add(st_swap_usage, cg->chart_id, cg->chart_title, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);

            rrddim_set_by_pointer(st_swap_usage, cg->rd_swap_usage, cg->memory.msw_usage_in_bytes);
        }

        if(likely(do_io && cg->io_service_bytes.updated)) {
            if(unlikely(!cg->rd_io_service_bytes_read))
                cg->rd_io_service_bytes_read = rrddim_add(st_io_read, cg->chart_id, cg->chart_title, 1, 1024, RRD_ALGORITHM_INCREMENTAL);

            rrddim_set_by_pointer(st_io_read, cg->rd_io_service_bytes_read, cg->io_service_bytes.Read);

            if(unlikely(!cg->rd_io_service_bytes_write))
                cg->rd_io_service_bytes_write = rrddim_add(st_io_write, cg->chart_id, cg->chart_title, 1, 1024, RRD_ALGORITHM_INCREMENTAL);

            rrddim_set_by_pointer(st_io_write, cg->rd_io_service_bytes_write, cg->io_service_bytes.Write);
        }

        if(likely(do_io_ops && cg->io_serviced.updated)) {
            if(unlikely(!cg->rd_io_serviced_read))
                cg->rd_io_serviced_read = rrddim_add(st_io_serviced_read, cg->chart_id, cg->chart_title, 1, 1, RRD_ALGORITHM_INCREMENTAL);

            rrddim_set_by_pointer(st_io_serviced_read, cg->rd_io_serviced_read, cg->io_serviced.Read);

            if(unlikely(!cg->rd_io_serviced_write))
                cg->rd_io_serviced_write = rrddim_add(st_io_serviced_write, cg->chart_id, cg->chart_title, 1, 1, RRD_ALGORITHM_INCREMENTAL);

            rrddim_set_by_pointer(st_io_serviced_write, cg->rd_io_serviced_write, cg->io_serviced.Write);
        }

        if(likely(do_throttle_io && cg->throttle_io_service_bytes.updated)) {
            if(unlikely(!cg->rd_throttle_io_read))
                cg->rd_throttle_io_read = rrddim_add(st_throttle_io_read, cg->chart_id, cg->chart_title, 1, 1024, RRD_ALGORITHM_INCREMENTAL);

            rrddim_set_by_pointer(st_throttle_io_read, cg->rd_throttle_io_read, cg->throttle_io_service_bytes.Read);

            if(unlikely(!cg->rd_throttle_io_write))
                cg->rd_throttle_io_write = rrddim_add(st_throttle_io_write, cg->chart_id, cg->chart_title, 1, 1024, RRD_ALGORITHM_INCREMENTAL);

            rrddim_set_by_pointer(st_throttle_io_write, cg->rd_throttle_io_write, cg->throttle_io_service_bytes.Write);
        }

        if(likely(do_throttle_ops && cg->throttle_io_serviced.updated)) {
            if(unlikely(!cg->rd_throttle_io_serviced_read))
                cg->rd_throttle_io_serviced_read = rrddim_add(st_throttle_ops_read, cg->chart_id, cg->chart_title, 1, 1, RRD_ALGORITHM_INCREMENTAL);

            rrddim_set_by_pointer(st_throttle_ops_read, cg->rd_throttle_io_serviced_read, cg->throttle_io_serviced.Read);

            if(unlikely(!cg->rd_throttle_io_serviced_write))
                cg->rd_throttle_io_serviced_write = rrddim_add(st_throttle_ops_write, cg->chart_id, cg->chart_title, 1, 1, RRD_ALGORITHM_INCREMENTAL);

            rrddim_set_by_pointer(st_throttle_ops_write, cg->rd_throttle_io_serviced_write, cg->throttle_io_serviced.Write);
        }

        if(likely(do_queued_ops && cg->io_queued.updated)) {
            if(unlikely(!cg->rd_io_queued_read))
                cg->rd_io_queued_read = rrddim_add(st_queued_ops_read, cg->chart_id, cg->chart_title, 1, 1, RRD_ALGORITHM_INCREMENTAL);

            rrddim_set_by_pointer(st_queued_ops_read, cg->rd_io_queued_read, cg->io_queued.Read);

            if(unlikely(!cg->rd_io_queued_write))
                cg->rd_io_queued_write = rrddim_add(st_queued_ops_write, cg->chart_id, cg->chart_title, 1, 1, RRD_ALGORITHM_INCREMENTAL);

            rrddim_set_by_pointer(st_queued_ops_write, cg->rd_io_queued_write, cg->io_queued.Write);
        }

        if(likely(do_merged_ops && cg->io_merged.updated)) {
            if(unlikely(!cg->rd_io_merged_read))
                cg->rd_io_merged_read = rrddim_add(st_merged_ops_read, cg->chart_id, cg->chart_title, 1, 1, RRD_ALGORITHM_INCREMENTAL);

            rrddim_set_by_pointer(st_merged_ops_read, cg->rd_io_merged_read, cg->io_merged.Read);

            if(unlikely(!cg->rd_io_merged_write))
                cg->rd_io_merged_write = rrddim_add(st_merged_ops_write, cg->chart_id, cg->chart_title, 1, 1, RRD_ALGORITHM_INCREMENTAL);

            rrddim_set_by_pointer(st_merged_ops_write, cg->rd_io_merged_write, cg->io_merged.Write);
        }
    }

    // complete the iteration
    if(likely(do_cpu))
        rrdset_done(st_cpu);

    if(likely(do_mem_usage))
        rrdset_done(st_mem_usage);

    if(unlikely(do_mem_detailed)) {
        rrdset_done(st_mem_detailed_cache);
        rrdset_done(st_mem_detailed_rss);
        rrdset_done(st_mem_detailed_mapped);
        rrdset_done(st_mem_detailed_writeback);
        rrdset_done(st_mem_detailed_pgfault);
        rrdset_done(st_mem_detailed_pgmajfault);
        rrdset_done(st_mem_detailed_pgpgin);
        rrdset_done(st_mem_detailed_pgpgout);
    }

    if(likely(do_mem_failcnt))
        rrdset_done(st_mem_failcnt);

    if(likely(do_swap_usage))
        rrdset_done(st_swap_usage);

    if(likely(do_io)) {
        rrdset_done(st_io_read);
        rrdset_done(st_io_write);
    }

    if(likely(do_io_ops)) {
        rrdset_done(st_io_serviced_read);
        rrdset_done(st_io_serviced_write);
    }

    if(likely(do_throttle_io)) {
        rrdset_done(st_throttle_io_read);
        rrdset_done(st_throttle_io_write);
    }

    if(likely(do_throttle_ops)) {
        rrdset_done(st_throttle_ops_read);
        rrdset_done(st_throttle_ops_write);
    }

    if(likely(do_queued_ops)) {
        rrdset_done(st_queued_ops_read);
        rrdset_done(st_queued_ops_write);
    }

    if(likely(do_merged_ops)) {
        rrdset_done(st_merged_ops_read);
        rrdset_done(st_merged_ops_write);
    }
}

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

    int services_do_cpu = 0,
            services_do_mem_usage = 0,
            services_do_mem_detailed = 0,
            services_do_mem_failcnt = 0,
            services_do_swap_usage = 0,
            services_do_io = 0,
            services_do_io_ops = 0,
            services_do_throttle_io = 0,
            services_do_throttle_ops = 0,
            services_do_queued_ops = 0,
            services_do_merged_ops = 0;

    struct cgroup *cg;
    for(cg = cgroup_root; cg ; cg = cg->next) {
        if(unlikely(!cg->available || !cg->enabled))
            continue;

        if(likely(cgroup_enable_systemd_services && cg->options & CGROUP_OPTIONS_SYSTEM_SLICE_SERVICE)) {
            if(cg->cpuacct_stat.updated && cg->cpuacct_stat.enabled == CONFIG_BOOLEAN_YES) services_do_cpu++;

            if(cgroup_enable_systemd_services_detailed_memory && cg->memory.updated_detailed && cg->memory.enabled_detailed) services_do_mem_detailed++;
            if(cg->memory.updated_usage_in_bytes && cg->memory.enabled_usage_in_bytes == CONFIG_BOOLEAN_YES) services_do_mem_usage++;
            if(cg->memory.updated_failcnt && cg->memory.enabled_failcnt == CONFIG_BOOLEAN_YES) services_do_mem_failcnt++;
            if(cg->memory.updated_msw_usage_in_bytes && cg->memory.enabled_msw_usage_in_bytes == CONFIG_BOOLEAN_YES) services_do_swap_usage++;

            if(cg->io_service_bytes.updated && cg->io_service_bytes.enabled == CONFIG_BOOLEAN_YES) services_do_io++;
            if(cg->io_serviced.updated && cg->io_serviced.enabled == CONFIG_BOOLEAN_YES) services_do_io_ops++;
            if(cg->throttle_io_service_bytes.updated && cg->throttle_io_service_bytes.enabled == CONFIG_BOOLEAN_YES) services_do_throttle_io++;
            if(cg->throttle_io_serviced.updated && cg->throttle_io_serviced.enabled == CONFIG_BOOLEAN_YES) services_do_throttle_ops++;
            if(cg->io_queued.updated && cg->io_queued.enabled == CONFIG_BOOLEAN_YES) services_do_queued_ops++;
            if(cg->io_merged.updated && cg->io_merged.enabled == CONFIG_BOOLEAN_YES) services_do_merged_ops++;
            continue;
        }

        type[0] = '\0';

        if(likely(cg->cpuacct_stat.updated && cg->cpuacct_stat.enabled == CONFIG_BOOLEAN_YES)) {
            if(unlikely(!cg->st_cpu)) {
                snprintfz(title, CHART_TITLE_MAX, "CPU Usage (%d%% = %d core%s) for cgroup %s", (processors * 100), processors, (processors > 1) ? "s" : "", cg->chart_title);

                cg->st_cpu = rrdset_create_localhost(
                        cgroup_chart_type(type, cg->chart_id, RRD_ID_LENGTH_MAX)
                        , "cpu"
                        , NULL
                        , "cpu"
                        , "cgroup.cpu"
                        , title
                        , "%"
                        , CHART_PRIORITY_CONTAINERS
                        , update_every
                        , RRDSET_TYPE_STACKED
                );

                rrddim_add(cg->st_cpu, "user", NULL, 100, hz, RRD_ALGORITHM_INCREMENTAL);
                rrddim_add(cg->st_cpu, "system", NULL, 100, hz, RRD_ALGORITHM_INCREMENTAL);
            }
            else
                rrdset_next(cg->st_cpu);

            rrddim_set(cg->st_cpu, "user", cg->cpuacct_stat.user);
            rrddim_set(cg->st_cpu, "system", cg->cpuacct_stat.system);
            rrdset_done(cg->st_cpu);
        }

        if(likely(cg->cpuacct_usage.updated && cg->cpuacct_usage.enabled == CONFIG_BOOLEAN_YES)) {
            char id[RRD_ID_LENGTH_MAX + 1];
            unsigned int i;

            if(unlikely(!cg->st_cpu_per_core)) {
                snprintfz(title, CHART_TITLE_MAX, "CPU Usage (%d%% = %d core%s) Per Core for cgroup %s", (processors * 100), processors, (processors > 1) ? "s" : "", cg->chart_title);

                cg->st_cpu_per_core = rrdset_create_localhost(
                        cgroup_chart_type(type, cg->chart_id, RRD_ID_LENGTH_MAX)
                        , "cpu_per_core"
                        , NULL
                        , "cpu"
                        , "cgroup.cpu_per_core"
                        , title
                        , "%"
                        , CHART_PRIORITY_CONTAINERS + 100
                        , update_every
                        , RRDSET_TYPE_STACKED
                );

                for(i = 0; i < cg->cpuacct_usage.cpus; i++) {
                    snprintfz(id, RRD_ID_LENGTH_MAX, "cpu%u", i);
                    rrddim_add(cg->st_cpu_per_core, id, NULL, 100, 1000000000, RRD_ALGORITHM_INCREMENTAL);
                }
            }
            else
                rrdset_next(cg->st_cpu_per_core);

            for(i = 0; i < cg->cpuacct_usage.cpus ;i++) {
                snprintfz(id, RRD_ID_LENGTH_MAX, "cpu%u", i);
                rrddim_set(cg->st_cpu_per_core, id, cg->cpuacct_usage.cpu_percpu[i]);
            }
            rrdset_done(cg->st_cpu_per_core);
        }

        if(likely(cg->memory.updated_detailed && cg->memory.enabled_detailed == CONFIG_BOOLEAN_YES)) {
            if(unlikely(!cg->st_mem)) {
                snprintfz(title, CHART_TITLE_MAX, "Memory Usage for cgroup %s", cg->chart_title);

                cg->st_mem = rrdset_create_localhost(
                        cgroup_chart_type(type, cg->chart_id, RRD_ID_LENGTH_MAX)
                        , "mem"
                        , NULL
                        , "mem"
                        , "cgroup.mem"
                        , title
                        , "MB"
                        , CHART_PRIORITY_CONTAINERS + 210
                        , update_every
                        , RRDSET_TYPE_STACKED
                );

                rrddim_add(cg->st_mem, "cache", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
                rrddim_add(cg->st_mem, "rss", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);

                if(cg->memory.detailed_has_swap)
                    rrddim_add(cg->st_mem, "swap", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);

                rrddim_add(cg->st_mem, "rss_huge", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
                rrddim_add(cg->st_mem, "mapped_file", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
            }
            else
                rrdset_next(cg->st_mem);

            rrddim_set(cg->st_mem, "cache", cg->memory.cache);
            rrddim_set(cg->st_mem, "rss", cg->memory.rss);

            if(cg->memory.detailed_has_swap)
                rrddim_set(cg->st_mem, "swap", cg->memory.swap);

            rrddim_set(cg->st_mem, "rss_huge", cg->memory.rss_huge);
            rrddim_set(cg->st_mem, "mapped_file", cg->memory.mapped_file);
            rrdset_done(cg->st_mem);

            if(unlikely(!cg->st_writeback)) {
                snprintfz(title, CHART_TITLE_MAX, "Writeback Memory for cgroup %s", cg->chart_title);

                cg->st_writeback = rrdset_create_localhost(
                        cgroup_chart_type(type, cg->chart_id, RRD_ID_LENGTH_MAX)
                        , "writeback"
                        , NULL
                        , "mem"
                        , "cgroup.writeback"
                        , title
                        , "MB"
                        , CHART_PRIORITY_CONTAINERS + 300
                        , update_every
                        , RRDSET_TYPE_AREA
                );

                if(cg->memory.detailed_has_dirty)
                    rrddim_add(cg->st_writeback, "dirty", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);

                rrddim_add(cg->st_writeback, "writeback", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
            }
            else
                rrdset_next(cg->st_writeback);

            if(cg->memory.detailed_has_dirty)
                rrddim_set(cg->st_writeback, "dirty", cg->memory.dirty);

            rrddim_set(cg->st_writeback, "writeback", cg->memory.writeback);
            rrdset_done(cg->st_writeback);

            if(unlikely(!cg->st_mem_activity)) {
                snprintfz(title, CHART_TITLE_MAX, "Memory Activity for cgroup %s", cg->chart_title);

                cg->st_mem_activity = rrdset_create_localhost(
                        cgroup_chart_type(type, cg->chart_id, RRD_ID_LENGTH_MAX)
                        , "mem_activity"
                        , NULL
                        , "mem"
                        , "cgroup.mem_activity"
                        , title
                        , "MB/s"
                        , CHART_PRIORITY_CONTAINERS + 400
                        , update_every
                        , RRDSET_TYPE_LINE
                );

                rrddim_add(cg->st_mem_activity, "pgpgin", "in", system_page_size, 1024 * 1024, RRD_ALGORITHM_INCREMENTAL);
                rrddim_add(cg->st_mem_activity, "pgpgout", "out", -system_page_size, 1024 * 1024, RRD_ALGORITHM_INCREMENTAL);
            }
            else
                rrdset_next(cg->st_mem_activity);

            rrddim_set(cg->st_mem_activity, "pgpgin", cg->memory.pgpgin);
            rrddim_set(cg->st_mem_activity, "pgpgout", cg->memory.pgpgout);
            rrdset_done(cg->st_mem_activity);

            if(unlikely(!cg->st_pgfaults)) {
                snprintfz(title, CHART_TITLE_MAX, "Memory Page Faults for cgroup %s", cg->chart_title);

                cg->st_pgfaults = rrdset_create_localhost(
                        cgroup_chart_type(type, cg->chart_id, RRD_ID_LENGTH_MAX)
                        , "pgfaults"
                        , NULL
                        , "mem"
                        , "cgroup.pgfaults"
                        , title
                        , "MB/s"
                        , CHART_PRIORITY_CONTAINERS + 500
                        , update_every
                        , RRDSET_TYPE_LINE
                );

                rrddim_add(cg->st_pgfaults, "pgfault", NULL, system_page_size, 1024 * 1024, RRD_ALGORITHM_INCREMENTAL);
                rrddim_add(cg->st_pgfaults, "pgmajfault", "swap", -system_page_size, 1024 * 1024, RRD_ALGORITHM_INCREMENTAL);
            }
            else
                rrdset_next(cg->st_pgfaults);

            rrddim_set(cg->st_pgfaults, "pgfault", cg->memory.pgfault);
            rrddim_set(cg->st_pgfaults, "pgmajfault", cg->memory.pgmajfault);
            rrdset_done(cg->st_pgfaults);
        }

        if(likely(cg->memory.updated_usage_in_bytes && cg->memory.enabled_usage_in_bytes == CONFIG_BOOLEAN_YES)) {
            if(unlikely(!cg->st_mem_usage)) {
                snprintfz(title, CHART_TITLE_MAX, "Used Memory %sfor cgroup %s", (cgroup_used_memory_without_cache && cg->memory.updated_detailed)?"without Cache ":"", cg->chart_title);

                cg->st_mem_usage = rrdset_create_localhost(
                        cgroup_chart_type(type, cg->chart_id, RRD_ID_LENGTH_MAX)
                        , "mem_usage"
                        , NULL
                        , "mem"
                        , "cgroup.mem_usage"
                        , title
                        , "MB"
                        , CHART_PRIORITY_CONTAINERS + 200
                        , update_every
                        , RRDSET_TYPE_STACKED
                );

                rrddim_add(cg->st_mem_usage, "ram", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
                rrddim_add(cg->st_mem_usage, "swap", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
            }
            else
                rrdset_next(cg->st_mem_usage);

            rrddim_set(cg->st_mem_usage, "ram", cg->memory.usage_in_bytes - ((cgroup_used_memory_without_cache)?cg->memory.cache:0));
            rrddim_set(cg->st_mem_usage, "swap", (cg->memory.msw_usage_in_bytes > cg->memory.usage_in_bytes)?cg->memory.msw_usage_in_bytes - cg->memory.usage_in_bytes:0);
            rrdset_done(cg->st_mem_usage);
        }

        if(likely(cg->memory.updated_failcnt && cg->memory.enabled_failcnt == CONFIG_BOOLEAN_YES)) {
            if(unlikely(!cg->st_mem_failcnt)) {
                snprintfz(title, CHART_TITLE_MAX, "Memory Limit Failures for cgroup %s", cg->chart_title);

                cg->st_mem_failcnt = rrdset_create_localhost(
                        cgroup_chart_type(type, cg->chart_id, RRD_ID_LENGTH_MAX)
                        , "mem_failcnt"
                        , NULL
                        , "mem"
                        , "cgroup.mem_failcnt"
                        , title
                        , "count"
                        , CHART_PRIORITY_CONTAINERS + 250
                        , update_every
                        , RRDSET_TYPE_LINE
                );

                rrddim_add(cg->st_mem_failcnt, "failures", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            }
            else
                rrdset_next(cg->st_mem_failcnt);

            rrddim_set(cg->st_mem_failcnt, "failures", cg->memory.failcnt);
            rrdset_done(cg->st_mem_failcnt);
        }

        if(likely(cg->io_service_bytes.updated && cg->io_service_bytes.enabled == CONFIG_BOOLEAN_YES)) {
            if(unlikely(!cg->st_io)) {
                snprintfz(title, CHART_TITLE_MAX, "I/O Bandwidth (all disks) for cgroup %s", cg->chart_title);

                cg->st_io = rrdset_create_localhost(
                        cgroup_chart_type(type, cg->chart_id, RRD_ID_LENGTH_MAX)
                        , "io"
                        , NULL
                        , "disk"
                        , "cgroup.io"
                        , title
                        , "KB/s"
                        , CHART_PRIORITY_CONTAINERS + 1200
                        , update_every
                        , RRDSET_TYPE_AREA
                );

                rrddim_add(cg->st_io, "read", NULL, 1, 1024, RRD_ALGORITHM_INCREMENTAL);
                rrddim_add(cg->st_io, "write", NULL, -1, 1024, RRD_ALGORITHM_INCREMENTAL);
            }
            else
                rrdset_next(cg->st_io);

            rrddim_set(cg->st_io, "read", cg->io_service_bytes.Read);
            rrddim_set(cg->st_io, "write", cg->io_service_bytes.Write);
            rrdset_done(cg->st_io);
        }

        if(likely(cg->io_serviced.updated && cg->io_serviced.enabled == CONFIG_BOOLEAN_YES)) {
            if(unlikely(!cg->st_serviced_ops)) {
                snprintfz(title, CHART_TITLE_MAX, "Serviced I/O Operations (all disks) for cgroup %s", cg->chart_title);

                cg->st_serviced_ops = rrdset_create_localhost(
                        cgroup_chart_type(type, cg->chart_id, RRD_ID_LENGTH_MAX)
                        , "serviced_ops"
                        , NULL
                        , "disk"
                        , "cgroup.serviced_ops"
                        , title
                        , "operations/s"
                        , CHART_PRIORITY_CONTAINERS + 1200
                        , update_every
                        , RRDSET_TYPE_LINE
                );

                rrddim_add(cg->st_serviced_ops, "read", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                rrddim_add(cg->st_serviced_ops, "write", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            }
            else
                rrdset_next(cg->st_serviced_ops);

            rrddim_set(cg->st_serviced_ops, "read", cg->io_serviced.Read);
            rrddim_set(cg->st_serviced_ops, "write", cg->io_serviced.Write);
            rrdset_done(cg->st_serviced_ops);
        }

        if(likely(cg->throttle_io_service_bytes.updated && cg->throttle_io_service_bytes.enabled == CONFIG_BOOLEAN_YES)) {
            if(unlikely(!cg->st_throttle_io)) {
                snprintfz(title, CHART_TITLE_MAX, "Throttle I/O Bandwidth (all disks) for cgroup %s", cg->chart_title);

                cg->st_throttle_io = rrdset_create_localhost(
                        cgroup_chart_type(type, cg->chart_id, RRD_ID_LENGTH_MAX)
                        , "throttle_io"
                        , NULL
                        , "disk"
                        , "cgroup.throttle_io"
                        , title
                        , "KB/s"
                        , CHART_PRIORITY_CONTAINERS + 1200
                        , update_every
                        , RRDSET_TYPE_AREA
                );

                rrddim_add(cg->st_throttle_io, "read", NULL, 1, 1024, RRD_ALGORITHM_INCREMENTAL);
                rrddim_add(cg->st_throttle_io, "write", NULL, -1, 1024, RRD_ALGORITHM_INCREMENTAL);
            }
            else
                rrdset_next(cg->st_throttle_io);

            rrddim_set(cg->st_throttle_io, "read", cg->throttle_io_service_bytes.Read);
            rrddim_set(cg->st_throttle_io, "write", cg->throttle_io_service_bytes.Write);
            rrdset_done(cg->st_throttle_io);
        }

        if(likely(cg->throttle_io_serviced.updated && cg->throttle_io_serviced.enabled == CONFIG_BOOLEAN_YES)) {
            if(unlikely(!cg->st_throttle_serviced_ops)) {
                snprintfz(title, CHART_TITLE_MAX, "Throttle Serviced I/O Operations (all disks) for cgroup %s", cg->chart_title);

                cg->st_throttle_serviced_ops = rrdset_create_localhost(
                        cgroup_chart_type(type, cg->chart_id, RRD_ID_LENGTH_MAX)
                        , "throttle_serviced_ops"
                        , NULL
                        , "disk"
                        , "cgroup.throttle_serviced_ops"
                        , title
                        , "operations/s"
                        , CHART_PRIORITY_CONTAINERS + 1200
                        , update_every
                        , RRDSET_TYPE_LINE
                );

                rrddim_add(cg->st_throttle_serviced_ops, "read", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                rrddim_add(cg->st_throttle_serviced_ops, "write", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            }
            else
                rrdset_next(cg->st_throttle_serviced_ops);

            rrddim_set(cg->st_throttle_serviced_ops, "read", cg->throttle_io_serviced.Read);
            rrddim_set(cg->st_throttle_serviced_ops, "write", cg->throttle_io_serviced.Write);
            rrdset_done(cg->st_throttle_serviced_ops);
        }

        if(likely(cg->io_queued.updated && cg->io_queued.enabled == CONFIG_BOOLEAN_YES)) {
            if(unlikely(!cg->st_queued_ops)) {
                snprintfz(title, CHART_TITLE_MAX, "Queued I/O Operations (all disks) for cgroup %s", cg->chart_title);

                cg->st_queued_ops = rrdset_create_localhost(
                        cgroup_chart_type(type, cg->chart_id, RRD_ID_LENGTH_MAX)
                        , "queued_ops"
                        , NULL
                        , "disk"
                        , "cgroup.queued_ops"
                        , title
                        , "operations"
                        , CHART_PRIORITY_CONTAINERS + 2000
                        , update_every
                        , RRDSET_TYPE_LINE
                );

                rrddim_add(cg->st_queued_ops, "read", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                rrddim_add(cg->st_queued_ops, "write", NULL, -1, 1, RRD_ALGORITHM_ABSOLUTE);
            }
            else
                rrdset_next(cg->st_queued_ops);

            rrddim_set(cg->st_queued_ops, "read", cg->io_queued.Read);
            rrddim_set(cg->st_queued_ops, "write", cg->io_queued.Write);
            rrdset_done(cg->st_queued_ops);
        }

        if(likely(cg->io_merged.updated && cg->io_merged.enabled == CONFIG_BOOLEAN_YES)) {
            if(unlikely(!cg->st_merged_ops)) {
                snprintfz(title, CHART_TITLE_MAX, "Merged I/O Operations (all disks) for cgroup %s", cg->chart_title);

                cg->st_merged_ops = rrdset_create_localhost(
                        cgroup_chart_type(type, cg->chart_id, RRD_ID_LENGTH_MAX)
                        , "merged_ops"
                        , NULL
                        , "disk"
                        , "cgroup.merged_ops"
                        , title
                        , "operations/s"
                        , CHART_PRIORITY_CONTAINERS + 2100
                        , update_every
                        , RRDSET_TYPE_LINE
                );

                rrddim_add(cg->st_merged_ops, "read", NULL, 1, 1024, RRD_ALGORITHM_INCREMENTAL);
                rrddim_add(cg->st_merged_ops, "write", NULL, -1, 1024, RRD_ALGORITHM_INCREMENTAL);
            }
            else
                rrdset_next(cg->st_merged_ops);

            rrddim_set(cg->st_merged_ops, "read", cg->io_merged.Read);
            rrddim_set(cg->st_merged_ops, "write", cg->io_merged.Write);
            rrdset_done(cg->st_merged_ops);
        }
    }

    if(likely(cgroup_enable_systemd_services))
        update_systemd_services_charts(update_every, services_do_cpu, services_do_mem_usage, services_do_mem_detailed
                                       , services_do_mem_failcnt, services_do_swap_usage, services_do_io
                                       , services_do_io_ops, services_do_throttle_io, services_do_throttle_ops
                                       , services_do_queued_ops, services_do_merged_ops
        );

    debug(D_CGROUP, "done updating cgroups charts");
}

// ----------------------------------------------------------------------------
// cgroups main

void *cgroups_main(void *ptr) {
    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)ptr;

    info("CGROUP plugin thread created with task id %d", gettid());

    if(pthread_setcanceltype(PTHREAD_CANCEL_DEFERRED, NULL) != 0)
        error("CGROUP: cannot set pthread cancel type to DEFERRED.");

    if(pthread_setcancelstate(PTHREAD_CANCEL_ENABLE, NULL) != 0)
        error("CGROUP: cannot set pthread cancel state to ENABLE.");

    struct rusage thread;

    // when ZERO, attempt to do it
    int vdo_cpu_netdata = config_get_boolean("plugin:cgroups", "cgroups plugin resource charts", 1);

    read_cgroup_plugin_configuration();

    RRDSET *stcpu_thread = NULL;

    heartbeat_t hb;
    heartbeat_init(&hb);
    usec_t step = cgroup_update_every * USEC_PER_SEC;
    usec_t find_every = cgroup_check_for_new_every * USEC_PER_SEC, find_dt = 0;
    for(;;) {
        usec_t hb_dt = heartbeat_next(&hb, step);
        if(unlikely(netdata_exit)) break;

        // BEGIN -- the job to be done

        find_dt += hb_dt;
        if(unlikely(find_dt >= find_every || cgroups_check)) {
            find_all_cgroups();
            find_dt = 0;
            cgroups_check = 0;
        }

        read_all_cgroups(cgroup_root);
        update_cgroup_charts(cgroup_update_every);

        // END -- the job is done

        // --------------------------------------------------------------------

        if(vdo_cpu_netdata) {
            getrusage(RUSAGE_THREAD, &thread);

            if(unlikely(!stcpu_thread)) {

                stcpu_thread = rrdset_create_localhost(
                        "netdata"
                        , "plugin_cgroups_cpu"
                        , NULL
                        , "cgroups"
                        , NULL
                        , "NetData CGroups Plugin CPU usage"
                        , "milliseconds/s"
                        , 132000
                        , cgroup_update_every
                        , RRDSET_TYPE_STACKED
                );

                rrddim_add(stcpu_thread, "user",  NULL,  1, 1000, RRD_ALGORITHM_INCREMENTAL);
                rrddim_add(stcpu_thread, "system", NULL, 1, 1000, RRD_ALGORITHM_INCREMENTAL);
            }
            else
                rrdset_next(stcpu_thread);

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
