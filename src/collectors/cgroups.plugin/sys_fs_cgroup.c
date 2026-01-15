// SPDX-License-Identifier: GPL-3.0-or-later

#include "cgroup-internals.h"

// main cgroups thread worker jobs
#define WORKER_CGROUPS_LOCK 0
#define WORKER_CGROUPS_READ 1
#define WORKER_CGROUPS_CHART 2

// ----------------------------------------------------------------------------
// cgroup globals
unsigned long long host_ram_total = 0;
bool is_inside_k8s = false;
long system_page_size = 4096; // system will be queried via sysconf() in configuration()

int cgroup_use_unified_cgroups = CONFIG_BOOLEAN_AUTO;
bool cgroup_unified_exist = true;

bool cgroup_enable_blkio = true;
bool cgroup_enable_pressure = true;
bool cgroup_enable_memory = true;
bool cgroup_enable_cpuacct = true;
bool cgroup_enable_cpuacct_cpu_shares = false;

int cgroup_check_for_new_every = 10;
int cgroup_update_every = 1;
char *cgroup_cpuacct_base = NULL;
char *cgroup_cpuset_base = NULL;
char *cgroup_blkio_base = NULL;
char *cgroup_memory_base = NULL;
char *cgroup_pids_base = NULL;
char *cgroup_unified_base = NULL;
int cgroup_root_count = 0;
int cgroup_root_max = 1000;
int cgroup_max_depth = 0;
SIMPLE_PATTERN *enabled_cgroup_paths = NULL;
SIMPLE_PATTERN *enabled_cgroup_names = NULL;
SIMPLE_PATTERN *search_cgroup_paths = NULL;
SIMPLE_PATTERN *enabled_cgroup_renames = NULL;
SIMPLE_PATTERN *systemd_services_cgroups = NULL;
SIMPLE_PATTERN *entrypoint_parent_process_comm = NULL;
const char *cgroups_network_interface_script = NULL;
int cgroups_check = 0;
uint32_t Read_hash = 0;
uint32_t Write_hash = 0;
uint32_t user_hash = 0;
uint32_t system_hash = 0;
uint32_t user_usec_hash = 0;
uint32_t system_usec_hash = 0;
uint32_t nr_periods_hash = 0;
uint32_t nr_throttled_hash = 0;
uint32_t throttled_time_hash = 0;
uint32_t throttled_usec_hash = 0;

// *** WARNING *** The fields are not thread safe. Take care of safe usage.
struct cgroup *cgroup_root = NULL;
netdata_mutex_t cgroup_root_mutex;

struct cgroups_systemd_config_setting cgroups_systemd_options[] = {
        { .name = "legacy",  .setting = SYSTEMD_CGROUP_LEGACY  },
        { .name = "hybrid",  .setting = SYSTEMD_CGROUP_HYBRID  },
        { .name = "unified", .setting = SYSTEMD_CGROUP_UNIFIED },
        { .name = NULL,      .setting = SYSTEMD_CGROUP_ERR     },
};

struct discovery_thread discovery_thread = {
    .exited = 1,  // Start as "exited" until properly initialized
};


/* on Fed systemd is not in PATH for some reason */
#define SYSTEMD_CMD_RHEL "/usr/lib/systemd/systemd --version"
#define SYSTEMD_HIERARCHY_STRING "default-hierarchy="

#define MAXSIZE_PROC_CMDLINE 4096
static enum cgroups_systemd_setting cgroups_detect_systemd(const char *exec)
{
    enum cgroups_systemd_setting retval = SYSTEMD_CGROUP_ERR;
    char buf[MAXSIZE_PROC_CMDLINE];
    char *begin, *end;

    POPEN_INSTANCE *pi = spawn_popen_run(exec);
    if(!pi)
        return retval;

    struct pollfd pfd;
    pfd.fd = spawn_popen_read_fd(pi);
    pfd.events = POLLIN;

    int timeout = 3000; // milliseconds
    int ret = poll(&pfd, 1, timeout);

    if (ret == -1) {
        collector_error("Failed to get the output of \"%s\"", exec);
    } else if (ret == 0) {
        collector_info("Cannot get the output of \"%s\" within timeout (%d ms)", exec, timeout);
    } else {
        while (fgets(buf, MAXSIZE_PROC_CMDLINE, spawn_popen_stdout(pi)) != NULL) {
            if ((begin = strstr(buf, SYSTEMD_HIERARCHY_STRING))) {
                end = begin = begin + strlen(SYSTEMD_HIERARCHY_STRING);
                if (!*begin)
                    break;
                while (isalpha(*end))
                    end++;
                *end = 0;
                for (int i = 0; cgroups_systemd_options[i].name; i++) {
                    if (!strcmp(begin, cgroups_systemd_options[i].name)) {
                        retval = cgroups_systemd_options[i].setting;
                        break;
                    }
                }
                break;
            }
        }
    }

    if(spawn_popen_wait(pi) != 0)
        return SYSTEMD_CGROUP_ERR;

    return retval;
}

static enum cgroups_type cgroups_try_detect_version()
{
    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/sys/fs/cgroup");
    struct statfs fsinfo;

    // https://github.com/systemd/systemd/blob/main/docs/CGROUP_DELEGATION.md#three-different-tree-setups-
    // ├── statfs("/sys/fs/cgroup/")
    // │   └── .f_type
    // │       ├── CGROUP2_SUPER_MAGIC (Unified mode)
    // │       └── TMPFS_MAGIC (Legacy or Hybrid mode)
    //         ├── statfs("/sys/fs/cgroup/unified/")
    //         │   └── .f_type
    //         │       ├── CGROUP2_SUPER_MAGIC (Hybrid mode)
    //         │       └── Otherwise, you're in legacy mode
    if (!statfs(filename, &fsinfo)) {
#if defined CGROUP2_SUPER_MAGIC
        if (fsinfo.f_type == CGROUP2_SUPER_MAGIC)
            return CGROUPS_V2;
#endif
#if defined TMPFS_MAGIC
        if (fsinfo.f_type == TMPFS_MAGIC) {
            // either hybrid or legacy
            return CGROUPS_V1;
        }
#endif
    }

    collector_info("cgroups version: can't detect using statfs (fs type), falling back to heuristics.");

    char buf[MAXSIZE_PROC_CMDLINE];
    enum cgroups_systemd_setting systemd_setting;
    int cgroups2_available = 0;

    // 1. check if cgroups2 available on system at all
    POPEN_INSTANCE *pi = spawn_popen_run("grep cgroup /proc/filesystems");
    if(!pi) {
        collector_error("cannot run 'grep cgroup /proc/filesystems'");
        return CGROUPS_AUTODETECT_FAIL;
    }
    while (fgets(buf, MAXSIZE_PROC_CMDLINE, spawn_popen_stdout(pi)) != NULL) {
        if (strstr(buf, "cgroup2")) {
            cgroups2_available = 1;
            break;
        }
    }
    if(spawn_popen_wait(pi) != 0)
        return CGROUPS_AUTODETECT_FAIL;

    if(!cgroups2_available)
        return CGROUPS_V1;

    // 3. check systemd compiletime setting
    if ((systemd_setting = cgroups_detect_systemd("systemd --version")) == SYSTEMD_CGROUP_ERR)
        systemd_setting = cgroups_detect_systemd(SYSTEMD_CMD_RHEL);

    if(systemd_setting == SYSTEMD_CGROUP_ERR)
        return CGROUPS_AUTODETECT_FAIL;

    if(systemd_setting == SYSTEMD_CGROUP_LEGACY || systemd_setting == SYSTEMD_CGROUP_HYBRID) {
        // currently we prefer V1 if HYBRID is set as it seems to be more feature complete
        // in the future we might want to continue here if SYSTEMD_CGROUP_HYBRID
        // and go ahead with V2
        return CGROUPS_V1;
    }

    // 4. if we are unified as on Fedora (default cgroups2 only mode)
    //    check kernel command line flag that can override that setting
    FILE *fp = fopen("/proc/cmdline", "r");
    if (!fp) {
        collector_error("Error reading kernel boot commandline parameters");
        return CGROUPS_AUTODETECT_FAIL;
    }

    if (!fgets(buf, MAXSIZE_PROC_CMDLINE, fp)) {
        collector_error("couldn't read all cmdline params into buffer");
        fclose(fp);
        return CGROUPS_AUTODETECT_FAIL;
    }

    fclose(fp);

    if (strstr(buf, "systemd.unified_cgroup_hierarchy=0")) {
        collector_info("cgroups v2 (unified cgroups) is available but are disabled on this system.");
        return CGROUPS_V1;
    }
    return CGROUPS_V2;
}

void set_cgroup_base_path(char *filename, char *path) {
    if (strncmp(netdata_configured_host_prefix, path, strlen(netdata_configured_host_prefix)) == 0) {
        snprintfz(filename, FILENAME_MAX, "%s", path);
    } else {
        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, path);
    }
}

void read_cgroup_plugin_configuration() {
    system_page_size = sysconf(_SC_PAGESIZE);

    Read_hash = simple_hash("Read");
    Write_hash = simple_hash("Write");
    user_hash = simple_hash("user");
    system_hash = simple_hash("system");
    user_usec_hash = simple_hash("user_usec");
    system_usec_hash = simple_hash("system_usec");
    nr_periods_hash = simple_hash("nr_periods");
    nr_throttled_hash = simple_hash("nr_throttled");
    throttled_time_hash = simple_hash("throttled_time");
    throttled_usec_hash = simple_hash("throttled_usec");

    cgroup_update_every = (int)inicfg_get_duration_seconds(&netdata_config, "plugin:cgroups", "update every", localhost->rrd_update_every);
    if(cgroup_update_every < localhost->rrd_update_every) {
        cgroup_update_every = localhost->rrd_update_every;
        inicfg_set_duration_seconds(&netdata_config, "plugin:cgroups", "update every", localhost->rrd_update_every);
    }

    cgroup_check_for_new_every = (int)inicfg_get_duration_seconds(&netdata_config, "plugin:cgroups", "check for new cgroups every", cgroup_check_for_new_every);
    if(cgroup_check_for_new_every < cgroup_update_every) {
        cgroup_check_for_new_every = cgroup_update_every;
        inicfg_set_duration_seconds(&netdata_config, "plugin:cgroups", "check for new cgroups every", cgroup_check_for_new_every);
    }

    cgroup_use_unified_cgroups = inicfg_get_boolean_ondemand(&netdata_config, "plugin:cgroups", "use unified cgroups", CONFIG_BOOLEAN_AUTO);
    if (cgroup_use_unified_cgroups == CONFIG_BOOLEAN_AUTO)
        cgroup_use_unified_cgroups = (cgroups_try_detect_version() == CGROUPS_V2);
    collector_info("use unified cgroups %s", cgroup_use_unified_cgroups ? "true" : "false");

    char filename[FILENAME_MAX + 1], *s;
    struct mountinfo *mi, *root = mountinfo_read(0);
    if (!cgroup_use_unified_cgroups) {
        mi = mountinfo_find_by_filesystem_super_option(root, "cgroup", "cpuacct");
        if (!mi)
            mi = mountinfo_find_by_filesystem_mount_source(root, "cgroup", "cpuacct");
        if (!mi) {
            collector_error("CGROUP: cannot find cpuacct mountinfo. Assuming default: /sys/fs/cgroup/cpuacct");
            s = "/sys/fs/cgroup/cpuacct";
        } else
            s = mi->mount_point;
        set_cgroup_base_path(filename, s);
        cgroup_cpuacct_base = strdupz(filename);

        mi = mountinfo_find_by_filesystem_super_option(root, "cgroup", "cpuset");
        if (!mi)
            mi = mountinfo_find_by_filesystem_mount_source(root, "cgroup", "cpuset");
        if (!mi) {
            collector_error("CGROUP: cannot find cpuset mountinfo. Assuming default: /sys/fs/cgroup/cpuset");
            s = "/sys/fs/cgroup/cpuset";
        } else
            s = mi->mount_point;
        set_cgroup_base_path(filename, s);
        cgroup_cpuset_base = strdupz(filename);

        mi = mountinfo_find_by_filesystem_super_option(root, "cgroup", "blkio");
        if (!mi)
            mi = mountinfo_find_by_filesystem_mount_source(root, "cgroup", "blkio");
        if (!mi) {
            collector_error("CGROUP: cannot find blkio mountinfo. Assuming default: /sys/fs/cgroup/blkio");
            s = "/sys/fs/cgroup/blkio";
        } else
            s = mi->mount_point;
        set_cgroup_base_path(filename, s);
        cgroup_blkio_base = strdupz(filename);

        mi = mountinfo_find_by_filesystem_super_option(root, "cgroup", "memory");
        if (!mi)
            mi = mountinfo_find_by_filesystem_mount_source(root, "cgroup", "memory");
        if (!mi) {
            collector_error("CGROUP: cannot find memory mountinfo. Assuming default: /sys/fs/cgroup/memory");
            s = "/sys/fs/cgroup/memory";
        } else {
            s = mi->mount_point;
        }
        set_cgroup_base_path(filename, s);
        cgroup_memory_base = strdupz(filename);

        mi = mountinfo_find_by_filesystem_super_option(root, "cgroup", "pids");
        if (!mi)
            mi = mountinfo_find_by_filesystem_mount_source(root, "cgroup", "pids");
        if (!mi) {
            collector_error("CGROUP: cannot find pids mountinfo. Assuming default: /sys/fs/cgroup/pids");
            s = "/sys/fs/cgroup/pids";
        } else {
            s = mi->mount_point;
        }

        set_cgroup_base_path(filename, s);
        cgroup_pids_base = strdupz(filename);
    } else {
        //TODO: can there be more than 1 cgroup2 mount point?
        //there is no cgroup2 specific super option - for now use 'rw' option
        mi = mountinfo_find_by_filesystem_super_option(root, "cgroup2", "rw");
        if (!mi) {
            mi = mountinfo_find_by_filesystem_mount_source(root, "cgroup2", "cgroup");
        }
        if (!mi) {
            collector_error("CGROUP: cannot find cgroup2 mountinfo. Assuming default: /sys/fs/cgroup");
            s = "/sys/fs/cgroup";
        } else
            s = mi->mount_point;

        set_cgroup_base_path(filename, s);
        cgroup_unified_base = strdupz(filename);
    }

    cgroup_root_max = (int)inicfg_get_number(&netdata_config, "plugin:cgroups", "max cgroups to allow", cgroup_root_max);
    cgroup_max_depth = (int)inicfg_get_number(&netdata_config, "plugin:cgroups", "max cgroups depth to monitor", cgroup_max_depth);

    enabled_cgroup_paths = simple_pattern_create(
            inicfg_get(&netdata_config, "plugin:cgroups", "enable by default cgroups matching",
            // ----------------------------------------------------------------

                       " !*/init.scope "                      // ignore init.scope
                       " !/system.slice/run-*.scope "         // ignore system.slice/run-XXXX.scope
                       " *user.slice/docker-*"                // allow docker rootless containers
                       " !*user.slice*"                       // ignore the rest stuff in user.slice 
                       " *.scope "                            // we need all other *.scope for sure

                       // ----------------------------------------------------------------

                       " !/machine.slice/*/.control "
                       " !/machine.slice/*/payload* "
                       " !/machine.slice/*/supervisor "
                       " /machine.slice/*.service "           // #3367 systemd-nspawn

                       // ----------------------------------------------------------------

                       " */kubepods/pod*/* "                   // k8s containers
                       " */kubepods/*/pod*/* "                 // k8s containers
                       " */*-kubepods-pod*/* "                 // k8s containers
                       " */*-kubepods-*-pod*/* "               // k8s containers
                       " !*kubepods* !*kubelet* "              // all other k8s cgroups

                       // ----------------------------------------------------------------

                       " !*/vcpu* "                           // libvirtd adds these sub-cgroups
                       " !*/emulator "                        // libvirtd adds these sub-cgroups
                       " !*.mount "
                       " !*.partition "
                       " !*.service "
                       " !*.service/udev "
                       " !*.socket "
                       " !*.slice "
                       " !*.swap "
                       " !*.user "
                       " !/ "
                       " !/docker "
                       " !*/libvirt "
                       " !/lxc "
                       " !/lxc/*/* "                          //  #1397 #2649
                       " !/lxc.monitor* "
                       " !/lxc.pivot "
                       " !/lxc.payload "
                       " !*lxcfs.service/.control"
                       " !/machine "
                       " !/qemu "
                       " !/system "
                       " !/systemd "
                       " !/user "
                       " * "                                  // enable anything else
            ), NULL, SIMPLE_PATTERN_EXACT, true);

    enabled_cgroup_names = simple_pattern_create(
            inicfg_get(&netdata_config, "plugin:cgroups", "enable by default cgroups names matching",
                       " * "
            ), NULL, SIMPLE_PATTERN_EXACT, true);

    search_cgroup_paths = simple_pattern_create(
            inicfg_get(&netdata_config, "plugin:cgroups", "search for cgroups in subpaths matching",
                       " !*/init.scope "                      // ignore init.scope
                       " !*-qemu "                            //  #345
                       " !*.libvirt-qemu "                    //  #3010
                       " !/init.scope "
                       " !/system "
                       " !/systemd "
                       " !/user "
                       " !/lxc/*/* "                          //  #2161 #2649
                       " !/lxc.monitor "
                       " !/lxc.payload/*/* "
                       " !/lxc.payload.* "
                       " * "
            ), NULL, SIMPLE_PATTERN_EXACT, true);

    snprintfz(filename, FILENAME_MAX, "%s/cgroup-name.sh", netdata_configured_primary_plugins_dir);
    cgroups_rename_script = inicfg_get(&netdata_config, "plugin:cgroups", "script to get cgroup names", filename);

    snprintfz(filename, FILENAME_MAX, "%s/cgroup-network", netdata_configured_primary_plugins_dir);
    cgroups_network_interface_script = inicfg_get(&netdata_config, "plugin:cgroups", "script to get cgroup network interfaces", filename);

    enabled_cgroup_renames = simple_pattern_create(
            inicfg_get(&netdata_config, "plugin:cgroups", "run script to rename cgroups matching",
                       " !/ "
                       " !*.mount "
                       " !*.socket "
                       " !*.partition "
                       " /machine.slice/*.service "          // #3367 systemd-nspawn
                       " !*.service "
                       " !*.slice "
                       " !*.swap "
                       " !*.user "
                       " !init.scope "
                       " !*.scope/vcpu* "                    // libvirtd adds these sub-cgroups
                       " !*.scope/emulator "                 // libvirtd adds these sub-cgroups
                       " *.scope "
                       " *docker* "
                       " *lxc* "
                       " *qemu* "
                       " */kubepods/pod*/* "                   // k8s containers
                       " */kubepods/*/pod*/* "                 // k8s containers
                       " */*-kubepods-pod*/* "                 // k8s containers
                       " */*-kubepods-*-pod*/* "               // k8s containers
                       " !*kubepods* !*kubelet* "              // all other k8s cgroups
                       " *.libvirt-qemu "                    // #3010
                       " * "
            ), NULL, SIMPLE_PATTERN_EXACT, true);

    systemd_services_cgroups = simple_pattern_create(
        inicfg_get(&netdata_config, 
            "plugin:cgroups",
            "cgroups to match as systemd services",
            " !/system.slice/*/*.service "
            " /system.slice/*.service "),
        NULL,
        SIMPLE_PATTERN_EXACT,
        true);

    mountinfo_free_all(root);
}

// ---------------------------------------------------------------------------------------------

static unsigned long long calc_delta(unsigned long long curr, unsigned long long prev) {
    if (prev > curr) {
        return 0;
    }
    return curr - prev;
}

static unsigned long long calc_percentage(unsigned long long value, unsigned long long total) {
    if (total == 0) {
        return 0;
    }
    return (unsigned long long)((NETDATA_DOUBLE)value / (NETDATA_DOUBLE)total * 100);
}

// ----------------------------------------------------------------------------
// read values from /sys

static inline void cgroup_read_cpuacct_stat(struct cpuacct_stat *cp) {
    static procfile *ff = NULL;

    if(likely(cp->filename)) {
        ff = procfile_reopen(ff, cp->filename, NULL, CGROUP_PROCFILE_FLAG);
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
            collector_error("CGROUP: file '%s' should have 1+ lines.", cp->filename);
            cp->updated = 0;
            return;
        }

        for(i = 0; i < lines ; i++) {
            char *s = procfile_lineword(ff, i, 0);
            uint32_t hash = simple_hash(s);

            if(unlikely(hash == user_hash && !strcmp(s, "user")))
                cp->user = str2ull(procfile_lineword(ff, i, 1), NULL);

            else if(unlikely(hash == system_hash && !strcmp(s, "system")))
                cp->system = str2ull(procfile_lineword(ff, i, 1), NULL);
        }

        cp->updated = 1;
    }
}

static inline void cgroup_read_cpuacct_cpu_stat(struct cpuacct_cpu_throttling *cp) {
    if (unlikely(!cp->filename)) {
        return;
    }

    static procfile *ff = NULL;
    ff = procfile_reopen(ff, cp->filename, NULL, CGROUP_PROCFILE_FLAG);
    if (unlikely(!ff)) {
        cp->updated = 0;
        cgroups_check = 1;
        return;
    }

    ff = procfile_readall(ff);
    if (unlikely(!ff)) {
        cp->updated = 0;
        cgroups_check = 1;
        return;
    }

    unsigned long lines = procfile_lines(ff);
    if (unlikely(lines < 3)) {
        collector_error("CGROUP: file '%s' should have 3 lines.", cp->filename);
        cp->updated = 0;
        return;
    }

    unsigned long long nr_periods_last = cp->nr_periods; 
    unsigned long long nr_throttled_last = cp->nr_throttled; 

    for (unsigned long i = 0; i < lines; i++) {
        char *s = procfile_lineword(ff, i, 0);
        uint32_t hash = simple_hash(s);

        if (unlikely(hash == nr_periods_hash && !strcmp(s, "nr_periods"))) {
            cp->nr_periods = str2ull(procfile_lineword(ff, i, 1), NULL);
        } else if (unlikely(hash == nr_throttled_hash && !strcmp(s, "nr_throttled"))) {
            cp->nr_throttled = str2ull(procfile_lineword(ff, i, 1), NULL);
        } else if (unlikely(hash == throttled_time_hash && !strcmp(s, "throttled_time"))) {
            cp->throttled_time = str2ull(procfile_lineword(ff, i, 1), NULL);
        }
    }
    cp->nr_throttled_perc =
        calc_percentage(calc_delta(cp->nr_throttled, nr_throttled_last), calc_delta(cp->nr_periods, nr_periods_last));

    cp->updated = 1;
}

static inline void cgroup2_read_cpuacct_cpu_stat(struct cpuacct_stat *cp, struct cpuacct_cpu_throttling *cpt) {
    static procfile *ff = NULL;
    if (unlikely(!cp->filename)) {
        return;
    }

    ff = procfile_reopen(ff, cp->filename, NULL, CGROUP_PROCFILE_FLAG);
    if (unlikely(!ff)) {
        cp->updated = 0;
        cgroups_check = 1;
        return;
    }

    ff = procfile_readall(ff);
    if (unlikely(!ff)) {
        cp->updated = 0;
        cgroups_check = 1;
        return;
    }

    unsigned long lines = procfile_lines(ff);

    if (unlikely(lines < 3)) {
        collector_error("CGROUP: file '%s' should have at least 3 lines.", cp->filename);
        cp->updated = 0;
        return;
    }

    unsigned long long nr_periods_last = cpt->nr_periods; 
    unsigned long long nr_throttled_last = cpt->nr_throttled; 

    for (unsigned long i = 0; i < lines; i++) {
        char *s = procfile_lineword(ff, i, 0);
        uint32_t hash = simple_hash(s);

        if (unlikely(hash == user_usec_hash && !strcmp(s, "user_usec"))) {
            cp->user = str2ull(procfile_lineword(ff, i, 1), NULL);
        } else if (unlikely(hash == system_usec_hash && !strcmp(s, "system_usec"))) {
            cp->system = str2ull(procfile_lineword(ff, i, 1), NULL);
        } else if (unlikely(hash == nr_periods_hash && !strcmp(s, "nr_periods"))) {
            cpt->nr_periods = str2ull(procfile_lineword(ff, i, 1), NULL);
        } else if (unlikely(hash == nr_throttled_hash && !strcmp(s, "nr_throttled"))) {
            cpt->nr_throttled = str2ull(procfile_lineword(ff, i, 1), NULL);
        } else if (unlikely(hash == throttled_usec_hash && !strcmp(s, "throttled_usec"))) {
            cpt->throttled_time = str2ull(procfile_lineword(ff, i, 1), NULL) * 1000; // usec -> ns
        }
    }
    cpt->nr_throttled_perc =
        calc_percentage(calc_delta(cpt->nr_throttled, nr_throttled_last), calc_delta(cpt->nr_periods, nr_periods_last));

    cp->updated = 1;
    cpt->updated = 1;
}

static inline void cgroup_read_cpuacct_cpu_shares(struct cpuacct_cpu_shares *cp) {
    if (unlikely(!cp->filename)) {
        return;
    }

    if (unlikely(read_single_number_file(cp->filename, &cp->shares))) {
        cp->updated = 0;
        cgroups_check = 1;
        return;
    }

    cp->updated = 1;
}

static inline void cgroup_read_cpuacct_usage(struct cpuacct_usage *ca) {
    static procfile *ff = NULL;

    if(likely(ca->filename)) {
        ff = procfile_reopen(ff, ca->filename, NULL, CGROUP_PROCFILE_FLAG);
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
            collector_error("CGROUP: file '%s' should have 1+ lines but has %zu.", ca->filename, procfile_lines(ff));
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
            unsigned long long n = str2ull(procfile_lineword(ff, 0, i), NULL);
            ca->cpu_percpu[i] = n;
            total += n;
        }

        ca->updated = 1;
    }
}

static inline void cgroup_read_blkio(struct blkio *io) {
    if (likely(io->filename)) {
        static procfile *ff = NULL;

        ff = procfile_reopen(ff, io->filename, NULL, CGROUP_PROCFILE_FLAG);
        if (unlikely(!ff)) {
            io->updated = 0;
            cgroups_check = 1;
            return;
        }

        ff = procfile_readall(ff);
        if (unlikely(!ff)) {
            io->updated = 0;
            cgroups_check = 1;
            return;
        }

        unsigned long i, lines = procfile_lines(ff);

        if (unlikely(lines < 1)) {
            collector_error("CGROUP: file '%s' should have 1+ lines.", io->filename);
            io->updated = 0;
            return;
        }

        io->Read = 0;
        io->Write = 0;

        for (i = 0; i < lines; i++) {
            char *s = procfile_lineword(ff, i, 1);
            uint32_t hash = simple_hash(s);

            if (unlikely(hash == Read_hash && !strcmp(s, "Read")))
                io->Read += str2ull(procfile_lineword(ff, i, 2), NULL);
            else if (unlikely(hash == Write_hash && !strcmp(s, "Write")))
                io->Write += str2ull(procfile_lineword(ff, i, 2), NULL);
        }

        io->updated = 1;
    }
}

static inline void cgroup2_read_blkio(struct blkio *io, unsigned int word_offset) {
    if (likely(io->filename)) {
        static procfile *ff = NULL;

        ff = procfile_reopen(ff, io->filename, NULL, CGROUP_PROCFILE_FLAG);
        if (unlikely(!ff)) {
            io->updated = 0;
            cgroups_check = 1;
            return;
        }

        ff = procfile_readall(ff);
        if (unlikely(!ff)) {
            io->updated = 0;
            cgroups_check = 1;
            return;
        }

        unsigned long i, lines = procfile_lines(ff);

        if (unlikely(lines < 1)) {
            collector_error("CGROUP: file '%s' should have 1+ lines.", io->filename);
            io->updated = 0;
            return;
        }

        io->Read = 0;
        io->Write = 0;

        for (i = 0; i < lines; i++) {
            io->Read += str2ull(procfile_lineword(ff, i, 2 + word_offset), NULL);
            io->Write += str2ull(procfile_lineword(ff, i, 4 + word_offset), NULL);
        }

        io->updated = 1;
    }
}

static inline void cgroup2_read_pressure(struct pressure *res) {
    static procfile *ff = NULL;

    if (likely(res->filename)) {
        ff = procfile_reopen(ff, res->filename, " =", CGROUP_PROCFILE_FLAG);
        if (unlikely(!ff)) {
            res->updated = 0;
            cgroups_check = 1;
            return;
        }

        ff = procfile_readall(ff);
        if (unlikely(!ff)) {
            res->updated = 0;
            cgroups_check = 1;
            return;
        }

        size_t lines = procfile_lines(ff);
        if (lines < 1) {
            collector_error("CGROUP: file '%s' should have 1+ lines.", res->filename);
            res->updated = 0;
            return;
        }

        bool did_some = false, did_full = false;

        for(size_t l = 0; l < lines ;l++) {
            const char *key = procfile_lineword(ff, l, 0);
            if(strcmp(key, "some") == 0) {
                res->some.share_time.value10 = strtod(procfile_lineword(ff, l, 2), NULL);
                res->some.share_time.value60 = strtod(procfile_lineword(ff, l, 4), NULL);
                res->some.share_time.value300 = strtod(procfile_lineword(ff, l, 6), NULL);
                res->some.total_time.value_total = str2ull(procfile_lineword(ff, l, 8), NULL) / 1000; // us->ms
                did_some = true;
            }
            else if(strcmp(key, "full") == 0) {
                res->full.share_time.value10 = strtod(procfile_lineword(ff, l, 2), NULL);
                res->full.share_time.value60 = strtod(procfile_lineword(ff, l, 4), NULL);
                res->full.share_time.value300 = strtod(procfile_lineword(ff, l, 6), NULL);
                res->full.total_time.value_total = str2ull(procfile_lineword(ff, l, 8), NULL) / 1000; // us->ms
                did_full = true;
            }
        }

        res->updated = (did_full || did_some) ? 1 : 0;
        res->some.available = did_some;
        res->full.available = did_full;
    }
}

static inline void cgroup_read_memory(struct memory *mem, char parent_cg_is_unified) {
    static procfile *ff = NULL;

    if(likely(mem->filename_detailed)) {
        ff = procfile_reopen(ff, mem->filename_detailed, NULL, CGROUP_PROCFILE_FLAG);
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
            collector_error("CGROUP: file '%s' should have 1+ lines.", mem->filename_detailed);
            mem->updated_detailed = 0;
            goto memory_next;
        }


        if(unlikely(!mem->arl_base)) {
            if(parent_cg_is_unified == 0){
                mem->arl_base = arl_create("cgroup/memory", NULL, 60);

                arl_expect(mem->arl_base, "total_cache", &mem->total_cache);
                arl_expect(mem->arl_base, "total_rss", &mem->total_rss);
                arl_expect(mem->arl_base, "total_rss_huge", &mem->total_rss_huge);
                arl_expect(mem->arl_base, "total_mapped_file", &mem->total_mapped_file);
                arl_expect(mem->arl_base, "total_writeback", &mem->total_writeback);
                mem->arl_dirty = arl_expect(mem->arl_base, "total_dirty", &mem->total_dirty);
                mem->arl_swap  = arl_expect(mem->arl_base, "total_swap", &mem->total_swap);
                arl_expect(mem->arl_base, "total_pgpgin", &mem->total_pgpgin);
                arl_expect(mem->arl_base, "total_pgpgout", &mem->total_pgpgout);
                arl_expect(mem->arl_base, "total_pgfault", &mem->total_pgfault);
                arl_expect(mem->arl_base, "total_pgmajfault", &mem->total_pgmajfault);
                arl_expect(mem->arl_base, "total_inactive_file", &mem->total_inactive_file);
            } else {
                mem->arl_base = arl_create("cgroup/memory", NULL, 60);

                arl_expect(mem->arl_base, "anon", &mem->anon);
                arl_expect(mem->arl_base, "kernel_stack", &mem->kernel_stack);
                arl_expect(mem->arl_base, "slab", &mem->slab);
                arl_expect(mem->arl_base, "sock", &mem->sock);
                arl_expect(mem->arl_base, "anon_thp", &mem->anon_thp);
                arl_expect(mem->arl_base, "file", &mem->total_mapped_file);
                arl_expect(mem->arl_base, "file_writeback", &mem->total_writeback);
                mem->arl_dirty = arl_expect(mem->arl_base, "file_dirty", &mem->total_dirty);
                arl_expect(mem->arl_base, "pgfault", &mem->total_pgfault);
                arl_expect(mem->arl_base, "pgmajfault", &mem->total_pgmajfault);
                arl_expect(mem->arl_base, "inactive_file", &mem->total_inactive_file);
            }
        }

        arl_begin(mem->arl_base);

        for (i = 0; i < lines; i++) {
            if (arl_check(mem->arl_base, procfile_lineword(ff, i, 0), procfile_lineword(ff, i, 1)))
                break;
        }

        if (unlikely(mem->arl_dirty->flags & ARL_ENTRY_FLAG_FOUND))
            mem->detailed_has_dirty = 1;

        if (unlikely(parent_cg_is_unified == 0 && mem->arl_swap->flags & ARL_ENTRY_FLAG_FOUND))
            mem->detailed_has_swap = 1;

        mem->updated_detailed = 1;
    }

memory_next:

    if (likely(mem->filename_usage_in_bytes)) {
        mem->updated_usage_in_bytes = !read_single_number_file(mem->filename_usage_in_bytes, &mem->usage_in_bytes);
    }

    if (likely(mem->updated_usage_in_bytes && mem->updated_detailed)) {
        mem->usage_in_bytes =
            (mem->usage_in_bytes > mem->total_inactive_file) ? (mem->usage_in_bytes - mem->total_inactive_file) : 0;
    }

    if (likely(mem->filename_msw_usage_in_bytes)) {
        mem->updated_msw_usage_in_bytes =
            !read_single_number_file(mem->filename_msw_usage_in_bytes, &mem->msw_usage_in_bytes);
    }

    if (likely(mem->filename_failcnt)) {
        mem->updated_failcnt = !read_single_number_file(mem->filename_failcnt, &mem->failcnt);
    }
}

static void cgroup_read_pids_current(struct pids *pids) {
    pids->updated = 0;

    if (unlikely(!pids->filename))
        return;

    pids->updated = !read_single_number_file(pids->filename, &pids->pids_current);
}

static inline void read_cgroup(struct cgroup *cg) {
    netdata_log_debug(D_CGROUP, "reading metrics for cgroups '%s'", cg->id);
    if (!(cg->options & CGROUP_OPTIONS_IS_UNIFIED)) {
        cgroup_read_cpuacct_stat(&cg->cpuacct_stat);
        cgroup_read_cpuacct_usage(&cg->cpuacct_usage);
        cgroup_read_cpuacct_cpu_stat(&cg->cpuacct_cpu_throttling);
        cgroup_read_cpuacct_cpu_shares(&cg->cpuacct_cpu_shares);
        cgroup_read_memory(&cg->memory, 0);
        cgroup_read_blkio(&cg->io_service_bytes);
        cgroup_read_blkio(&cg->io_serviced);
        cgroup_read_blkio(&cg->throttle_io_service_bytes);
        cgroup_read_blkio(&cg->throttle_io_serviced);
        cgroup_read_blkio(&cg->io_merged);
        cgroup_read_blkio(&cg->io_queued);
        cgroup_read_pids_current(&cg->pids_current);
    } else {
        cgroup2_read_blkio(&cg->io_service_bytes, 0);
        cgroup2_read_blkio(&cg->io_serviced, 4);
        cgroup2_read_cpuacct_cpu_stat(&cg->cpuacct_stat, &cg->cpuacct_cpu_throttling);
        cgroup_read_cpuacct_cpu_shares(&cg->cpuacct_cpu_shares);
        cgroup2_read_pressure(&cg->cpu_pressure);
        cgroup2_read_pressure(&cg->io_pressure);
        cgroup2_read_pressure(&cg->memory_pressure);
        cgroup2_read_pressure(&cg->irq_pressure);
        cgroup_read_memory(&cg->memory, 1);
        cgroup_read_pids_current(&cg->pids_current);
    }
}

static inline void read_all_discovered_cgroups(struct cgroup *root) {
    netdata_log_debug(D_CGROUP, "reading metrics for all cgroups");

    struct cgroup *cg;
    for (cg = root; cg; cg = cg->next) {
        if (cg->enabled && !cg->pending_renames) {
            read_cgroup(cg);
        }
    }
}

// update CPU and memory limits

static inline void update_cpu_limits(char **filename, unsigned long long *value, struct cgroup *cg) {
    if(*filename) {
        int ret = -1;

        if(value == &cg->cpuset_cpus) {
            unsigned long ncpus = os_read_cpuset_cpus(*filename, os_get_system_cpus());
            if(ncpus) {
                *value = ncpus;
                ret = 0;
            }
        }
        else if(value == &cg->cpu_cfs_period || value == &cg->cpu_cfs_quota) {
            ret = read_single_number_file(*filename, value);
        }
        else ret = -1;

        if(ret) {
            collector_error("Cannot refresh cgroup %s cpu limit by reading '%s'. Will not update its limit anymore.", cg->id, *filename);
            freez(*filename);
            *filename = NULL;
        }
    }
}

static inline void update_cpu_limits2(struct cgroup *cg) {
    if(cg->filename_cpu_cfs_quota){
        static procfile *ff = NULL;

        ff = procfile_reopen(ff, cg->filename_cpu_cfs_quota, NULL, CGROUP_PROCFILE_FLAG);
        if(unlikely(!ff)) {
            goto cpu_limits2_err;
        }

        ff = procfile_readall(ff);
        if(unlikely(!ff)) {
            goto cpu_limits2_err;
        }

        unsigned long lines = procfile_lines(ff);

        if (unlikely(lines < 1)) {
            collector_error("CGROUP: file '%s' should have 1 lines.", cg->filename_cpu_cfs_quota);
            return;
        }

        cg->cpu_cfs_period = str2ull(procfile_lineword(ff, 0, 1), NULL);
        cg->cpuset_cpus = os_get_system_cpus();

        char *s = "max\n\0";
        if(strcmp(s, procfile_lineword(ff, 0, 0)) == 0){
            cg->cpu_cfs_quota = cg->cpu_cfs_period * cg->cpuset_cpus;
        } else {
            cg->cpu_cfs_quota = str2ull(procfile_lineword(ff, 0, 0), NULL);
        }
        netdata_log_debug(D_CGROUP, "CPU limits values: %llu %llu %llu", cg->cpu_cfs_period, cg->cpuset_cpus, cg->cpu_cfs_quota);
        return;

cpu_limits2_err:
        collector_error("Cannot refresh cgroup %s cpu limit by reading '%s'. Will not update its limit anymore.", cg->id, cg->filename_cpu_cfs_quota);
        freez(cg->filename_cpu_cfs_quota);
        cg->filename_cpu_cfs_quota = NULL;

    }
}

static inline int update_memory_limits(struct cgroup *cg) {
    char **filename = &cg->filename_memory_limit;
    unsigned long long *value = &cg->memory_limit;

    if(*filename) {
        if(unlikely(!cg->chart_var_memory_limit)) {
            cg->chart_var_memory_limit = rrdvar_chart_variable_add_and_acquire(cg->st_mem_usage, "memory_limit");
            if(!cg->chart_var_memory_limit) {
                collector_error("Cannot create cgroup %s chart variable '%s'. Will not update its limit anymore.", cg->id, "memory_limit");
                freez(*filename);
                *filename = NULL;
            }
        }

        if(*filename && cg->chart_var_memory_limit) {
            if(!(cg->options & CGROUP_OPTIONS_IS_UNIFIED)) {
                if(read_single_number_file(*filename, value)) {
                    collector_error("Cannot refresh cgroup %s memory limit by reading '%s'. Will not update its limit anymore.", cg->id, *filename);
                    freez(*filename);
                    *filename = NULL;
                }
                else {
                    rrdvar_chart_variable_set(
                        cg->st_mem_usage, cg->chart_var_memory_limit, (NETDATA_DOUBLE)(*value) / (1024.0 * 1024.0));
                    return 1;
                }
            } else {
                char buffer[32];
                int ret = read_txt_file(*filename, buffer, sizeof(buffer));
                if(ret) {
                    collector_error("Cannot refresh cgroup %s memory limit by reading '%s'. Will not update its limit anymore.", cg->id, *filename);
                    freez(*filename);
                    *filename = NULL;
                    return 0;
                }
                char *s = "max\n\0";
                if(strcmp(s, buffer) == 0){
                    *value = UINT64_MAX;
                    rrdvar_chart_variable_set(cg->st_mem_usage, cg->chart_var_memory_limit, (NETDATA_DOUBLE)(*value) / (1024.0 * 1024.0));
                    return 1;
                }
                *value = str2ull(buffer, NULL);
                rrdvar_chart_variable_set(cg->st_mem_usage, cg->chart_var_memory_limit, (NETDATA_DOUBLE)(*value) / (1024.0 * 1024.0));
                return 1;
            }
        }
    }
    return 0;
}

// ----------------------------------------------------------------------------
// generate charts

void update_cgroup_systemd_services_charts() {
    for (struct cgroup *cg = cgroup_root; cg; cg = cg->next) {
        if (unlikely(!cg->enabled || cg->pending_renames || !is_cgroup_systemd_service(cg)))
            continue;

        if (likely(cg->cpuacct_stat.updated)) {
            update_cpu_utilization_chart(cg);
        }
        if (likely(cg->memory.updated_msw_usage_in_bytes)) {
            update_mem_usage_chart(cg);
        }
        if (likely(cg->memory.updated_failcnt)) {
            update_mem_failcnt_chart(cg);
        }
        if (likely(cg->memory.updated_detailed)) {
            update_mem_usage_detailed_chart(cg);
            update_mem_writeback_chart(cg);
            update_mem_pgfaults_chart(cg);
            if (!(cg->options & CGROUP_OPTIONS_IS_UNIFIED)) {
                update_mem_activity_chart(cg);
            }
        }
        if (likely(cg->io_service_bytes.updated)) {
            update_io_serviced_bytes_chart(cg);
        }
        if (likely(cg->io_serviced.updated)) {
            update_io_serviced_ops_chart(cg);
        }
        if (likely(cg->throttle_io_service_bytes.updated)) {
            update_throttle_io_serviced_bytes_chart(cg);
        }
        if (likely(cg->throttle_io_serviced.updated)) {
            update_throttle_io_serviced_ops_chart(cg);
        }
        if (likely(cg->io_queued.updated)) {
            update_io_queued_ops_chart(cg);
        }
        if (likely(cg->io_merged.updated)) {
            update_io_merged_ops_chart(cg);
        }

        if (likely(cg->pids_current.updated)) {
            update_pids_current_chart(cg);
        }

        cg->function_ready = true;
    }
}

void update_cgroup_charts() {
    for (struct cgroup *cg = cgroup_root; cg; cg = cg->next) {
        if (unlikely(!cg->enabled || cg->pending_renames || is_cgroup_systemd_service(cg)))
            continue;

        if (likely(cg->cpuacct_stat.updated)) {
            update_cpu_utilization_chart(cg);

            if (likely(cg->filename_cpuset_cpus || cg->filename_cpu_cfs_period || cg->filename_cpu_cfs_quota)) {
                if (!(cg->options & CGROUP_OPTIONS_IS_UNIFIED)) {
                    update_cpu_limits(&cg->filename_cpuset_cpus, &cg->cpuset_cpus, cg);
                    update_cpu_limits(&cg->filename_cpu_cfs_period, &cg->cpu_cfs_period, cg);
                    update_cpu_limits(&cg->filename_cpu_cfs_quota, &cg->cpu_cfs_quota, cg);
                } else {
                    update_cpu_limits2(cg);
                }

                if (unlikely(!cg->chart_var_cpu_limit)) {
                    cg->chart_var_cpu_limit = rrdvar_chart_variable_add_and_acquire(cg->st_cpu, "cpu_limit");
                    if (!cg->chart_var_cpu_limit) {
                        collector_error(
                            "Cannot create cgroup %s chart variable 'cpu_limit'. Will not update its limit anymore.",
                            cg->id);
                        if (cg->filename_cpuset_cpus)
                            freez(cg->filename_cpuset_cpus);
                        cg->filename_cpuset_cpus = NULL;
                        if (cg->filename_cpu_cfs_period)
                            freez(cg->filename_cpu_cfs_period);
                        cg->filename_cpu_cfs_period = NULL;
                        if (cg->filename_cpu_cfs_quota)
                            freez(cg->filename_cpu_cfs_quota);
                        cg->filename_cpu_cfs_quota = NULL;
                    }
                } else {
                    NETDATA_DOUBLE value = 0, quota = 0;

                    if (likely(
                            ((!(cg->options & CGROUP_OPTIONS_IS_UNIFIED)) &&
                             (cg->filename_cpuset_cpus ||
                              (cg->filename_cpu_cfs_period && cg->filename_cpu_cfs_quota))) ||
                            ((cg->options & CGROUP_OPTIONS_IS_UNIFIED) && cg->filename_cpu_cfs_quota))) {
                        if (unlikely(cg->cpu_cfs_quota > 0))
                            quota = (NETDATA_DOUBLE)cg->cpu_cfs_quota / (NETDATA_DOUBLE)cg->cpu_cfs_period;

                        if (unlikely(quota > 0 && quota < cg->cpuset_cpus))
                            value = quota * 100;
                        else
                            value = (NETDATA_DOUBLE)cg->cpuset_cpus * 100;
                    }
                    if (likely(value)) {
                        update_cpu_utilization_limit_chart(cg, value);
                    } else {
                        if (unlikely(cg->st_cpu_limit)) {
                            rrdset_is_obsolete___safe_from_collector_thread(cg->st_cpu_limit);
                            cg->st_cpu_limit = NULL;
                        }
                        rrdvar_chart_variable_set(cg->st_cpu, cg->chart_var_cpu_limit, NAN);
                    }
                }
            }
        }

        if (likely(cg->cpuacct_cpu_throttling.updated)) {
            update_cpu_throttled_chart(cg);
            update_cpu_throttled_duration_chart(cg);
        }

        if (unlikely(cg->cpuacct_cpu_shares.updated)) {
            update_cpu_shares_chart(cg);
        }

        if (likely(cg->cpuacct_usage.updated)) {
            update_cpu_per_core_usage_chart(cg);
        }

        if (likely(cg->memory.updated_detailed)) {
            update_mem_usage_detailed_chart(cg);
            update_mem_writeback_chart(cg);

            if(!(cg->options & CGROUP_OPTIONS_IS_UNIFIED)) {
                update_mem_activity_chart(cg);
            }

            update_mem_pgfaults_chart(cg);
        }

        if (likely(cg->memory.updated_usage_in_bytes)) {
            update_mem_usage_chart(cg);

            // FIXME: this "if" should be only for unlimited charts
            if (likely(host_ram_total)) {
                // FIXME: do we need to update mem limits on every data collection?
                if (likely(update_memory_limits(cg))) {
                    unsigned long long memory_limit = host_ram_total;
                    if (unlikely(cg->memory_limit < host_ram_total))
                        memory_limit = cg->memory_limit;

                    update_mem_usage_limit_chart(cg, memory_limit);
                    update_mem_utilization_chart(cg, memory_limit);
                } else {
                    if (unlikely(cg->st_mem_usage_limit)) {
                        rrdset_is_obsolete___safe_from_collector_thread(cg->st_mem_usage_limit);
                        cg->st_mem_usage_limit = NULL;
                    }

                    if (unlikely(cg->st_mem_utilization)) {
                        rrdset_is_obsolete___safe_from_collector_thread(cg->st_mem_utilization);
                        cg->st_mem_utilization = NULL;
                    }
                }
            }
        }

        if (likely(cg->memory.updated_failcnt)) {
            update_mem_failcnt_chart(cg);
        }

        if (likely(cg->io_service_bytes.updated)) {
            update_io_serviced_bytes_chart(cg);
        }

        if (likely(cg->io_serviced.updated)) {
            update_io_serviced_ops_chart(cg);
        }

        if (likely(cg->throttle_io_service_bytes.updated)) {
            update_throttle_io_serviced_bytes_chart(cg);
        }

        if (likely(cg->throttle_io_serviced.updated)) {
            update_throttle_io_serviced_ops_chart(cg);
        }

        if (likely(cg->io_queued.updated)) {
            update_io_queued_ops_chart(cg);
        }

        if (likely(cg->io_merged.updated)) {
            update_io_merged_ops_chart(cg);
        }

        if (likely(cg->pids_current.updated)) {
                update_pids_current_chart(cg);
        }

        if (cg->options & CGROUP_OPTIONS_IS_UNIFIED) {
            if (likely(cg->cpu_pressure.updated)) {
                    if (cg->cpu_pressure.some.available) {
                        update_cpu_some_pressure_chart(cg);
                        update_cpu_some_pressure_stall_time_chart(cg);
                    }
                    if (cg->cpu_pressure.full.available) {
                        update_cpu_full_pressure_chart(cg);
                        update_cpu_full_pressure_stall_time_chart(cg);
                    }
            }

            if (likely(cg->memory_pressure.updated)) {
                if (cg->memory_pressure.some.available) {
                        update_mem_some_pressure_chart(cg);
                        update_mem_some_pressure_stall_time_chart(cg);
                }
                if (cg->memory_pressure.full.available) {
                        update_mem_full_pressure_chart(cg);
                        update_mem_full_pressure_stall_time_chart(cg);
                }
            }

            if (likely(cg->irq_pressure.updated)) {
                if (cg->irq_pressure.some.available) {
                        update_irq_some_pressure_chart(cg);
                        update_irq_some_pressure_stall_time_chart(cg);
                }
                if (cg->irq_pressure.full.available) {
                        update_irq_full_pressure_chart(cg);
                        update_irq_full_pressure_stall_time_chart(cg);
                }
            }

            if (likely(cg->io_pressure.updated)) {
                if (cg->io_pressure.some.available) {
                        update_io_some_pressure_chart(cg);
                        update_io_some_pressure_stall_time_chart(cg);
                }
                if (cg->io_pressure.full.available) {
                        update_io_full_pressure_chart(cg);
                        update_io_full_pressure_stall_time_chart(cg);
                }
            }
        }

        cg->function_ready = true;
    }
}

// ----------------------------------------------------------------------------
// cgroups main

static void cgroup_main_cleanup(void *pptr) {
    struct netdata_static_thread *static_thread = CLEANUP_FUNCTION_GET_PTR(pptr);
    if(!static_thread) return;

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;

    worker_unregister();

    usec_t max = 2 * USEC_PER_SEC, step = 50000;

    if (!__atomic_load_n(&discovery_thread.exited, __ATOMIC_RELAXED)) {
        collector_info("waiting for discovery thread to finish...");
        while (!__atomic_load_n(&discovery_thread.exited, __ATOMIC_RELAXED) && max > 0) {
            netdata_mutex_lock(&discovery_thread.mutex);
            netdata_cond_signal(&discovery_thread.cond_var);
            netdata_mutex_unlock(&discovery_thread.mutex);
            max -= step;
            sleep_usec(step);
        }
    }
    // We should be done, but just in case, avoid blocking shutdown
    if (__atomic_load_n(&discovery_thread.exited, __ATOMIC_RELAXED))
        (void) nd_thread_join(discovery_thread.thread);

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
}

void cgroup_read_host_total_ram() {
    procfile *ff = NULL;
    char filename[FILENAME_MAX + 1];

    snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/proc/meminfo");
    ff = procfile_open(filename, " \t:", PROCFILE_FLAG_DEFAULT);

    if (likely((ff = procfile_readall(ff)) && procfile_lines(ff) && !strncmp(procfile_word(ff, 0), "MemTotal", 8)))
        host_ram_total = str2ull(procfile_word(ff, 1), NULL) * 1024;
    else
        collector_error("Cannot read file %s. Will not create RAM limit charts.", filename);

    procfile_close(ff);
}

void cgroups_main(void *ptr) {
    CLEANUP_FUNCTION_REGISTER(cgroup_main_cleanup) cleanup_ptr = ptr;

    worker_register("CGROUPS");
    worker_register_job_name(WORKER_CGROUPS_LOCK, "lock");
    worker_register_job_name(WORKER_CGROUPS_READ, "read");
    worker_register_job_name(WORKER_CGROUPS_CHART, "chart");

    if (getenv("KUBERNETES_SERVICE_HOST") != NULL && getenv("KUBERNETES_SERVICE_PORT") != NULL) {
        is_inside_k8s = true;
        cgroup_enable_cpuacct_cpu_shares = true;
    }

    read_cgroup_plugin_configuration();

    cgroup_read_host_total_ram();

    if (netdata_mutex_init(&cgroup_root_mutex)) {
        collector_error("CGROUP: cannot initialize mutex for the main cgroup list");
        return;
    }

    // we register this only on localhost
    // for the other nodes, the origin server should register it
    cgroup_netdev_link_init();

    if (netdata_mutex_init(&discovery_thread.mutex)) {
        collector_error("CGROUP: cannot initialize mutex for discovery thread");
        return;
    }
    if (netdata_cond_init(&discovery_thread.cond_var)) {
        collector_error("CGROUP: cannot initialize conditional variable for discovery thread");
        return;
    }

    // Mark thread as "running" only after mutex/cond are initialized
    // but before creating the thread. This ensures cleanup won't try
    // to access uninitialized synchronization primitives.
    discovery_thread.exited = 0;

    discovery_thread.thread = nd_thread_create("CGDISCOVER", NETDATA_THREAD_OPTION_DEFAULT, cgroup_discovery_worker, NULL);

    if (!discovery_thread.thread) {
        collector_error("CGROUP: cannot create thread worker");
        discovery_thread.exited = 1;  // Reset since thread wasn't created
        return;
    }

    rrd_function_add_inline(localhost, NULL, "containers-vms", 10,
                            RRDFUNCTIONS_PRIORITY_DEFAULT / 2, RRDFUNCTIONS_VERSION_DEFAULT,
                            RRDFUNCTIONS_CGTOP_HELP,
                            "top", HTTP_ACCESS_ANONYMOUS_DATA,
                            cgroup_function_cgroup_top);

    rrd_function_add_inline(localhost, NULL, "systemd-services", 10,
                            RRDFUNCTIONS_PRIORITY_DEFAULT / 3, RRDFUNCTIONS_VERSION_DEFAULT,
                            RRDFUNCTIONS_SYSTEMD_SERVICES_HELP,
                            "top", HTTP_ACCESS_ANONYMOUS_DATA,
                            cgroup_function_systemd_top);

    heartbeat_t hb;
    heartbeat_init(&hb, cgroup_update_every * USEC_PER_SEC);
    usec_t find_every = cgroup_check_for_new_every * USEC_PER_SEC, find_dt = 0;

    while(service_running(SERVICE_COLLECTORS)) {
        worker_is_idle();

        usec_t hb_dt = heartbeat_next(&hb);

        if (unlikely(!service_running(SERVICE_COLLECTORS)))
            break;

        find_dt += hb_dt;
        if (unlikely(find_dt >= find_every || (!is_inside_k8s && cgroups_check))) {
            netdata_mutex_lock(&discovery_thread.mutex);
            netdata_cond_signal(&discovery_thread.cond_var);
            netdata_mutex_unlock(&discovery_thread.mutex);
            find_dt = 0;
            cgroups_check = 0;
        }

        worker_is_busy(WORKER_CGROUPS_LOCK);
        netdata_mutex_lock(&cgroup_root_mutex);

        worker_is_busy(WORKER_CGROUPS_READ);
        read_all_discovered_cgroups(cgroup_root);

        if (unlikely(!service_running(SERVICE_COLLECTORS))) {
            netdata_mutex_unlock(&cgroup_root_mutex);
            break;
        }

        worker_is_busy(WORKER_CGROUPS_CHART);

        update_cgroup_charts();
        update_cgroup_systemd_services_charts();

        if (unlikely(!service_running(SERVICE_COLLECTORS))) {
            netdata_mutex_unlock(&cgroup_root_mutex);
           break;
        }

        worker_is_idle();
        netdata_mutex_unlock(&cgroup_root_mutex);
    }
}
