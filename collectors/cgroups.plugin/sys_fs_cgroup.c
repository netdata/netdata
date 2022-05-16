// SPDX-License-Identifier: GPL-3.0-or-later

#include "sys_fs_cgroup.h"

#define PLUGIN_CGROUPS_NAME "cgroups.plugin"
#define PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME "systemd"
#define PLUGIN_CGROUPS_MODULE_CGROUPS_NAME "/sys/fs/cgroup"

// main cgroups thread worker jobs
#define WORKER_CGROUPS_LOCK 0
#define WORKER_CGROUPS_READ 1
#define WORKER_CGROUPS_CHART 2

// discovery cgroup thread worker jobs
#define WORKER_DISCOVERY_INIT               0
#define WORKER_DISCOVERY_FIND               1
#define WORKER_DISCOVERY_PROCESS            2
#define WORKER_DISCOVERY_PROCESS_RENAME     3
#define WORKER_DISCOVERY_PROCESS_NETWORK    4
#define WORKER_DISCOVERY_PROCESS_FIRST_TIME 5
#define WORKER_DISCOVERY_UPDATE             6
#define WORKER_DISCOVERY_CLEANUP            7
#define WORKER_DISCOVERY_COPY               8
#define WORKER_DISCOVERY_SHARE              9
#define WORKER_DISCOVERY_LOCK              10

#if WORKER_UTILIZATION_MAX_JOB_TYPES < 11
#error WORKER_UTILIZATION_MAX_JOB_TYPES has to be at least 11
#endif

// ----------------------------------------------------------------------------
// cgroup globals

static int is_inside_k8s = 0;

static long system_page_size = 4096; // system will be queried via sysconf() in configuration()

static int cgroup_enable_cpuacct_stat = CONFIG_BOOLEAN_AUTO;
static int cgroup_enable_cpuacct_usage = CONFIG_BOOLEAN_AUTO;
static int cgroup_enable_cpuacct_cpu_throttling = CONFIG_BOOLEAN_YES;
static int cgroup_enable_cpuacct_cpu_shares = CONFIG_BOOLEAN_NO;
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
static int cgroup_enable_pressure_cpu = CONFIG_BOOLEAN_AUTO;
static int cgroup_enable_pressure_io_some = CONFIG_BOOLEAN_AUTO;
static int cgroup_enable_pressure_io_full = CONFIG_BOOLEAN_AUTO;
static int cgroup_enable_pressure_memory_some = CONFIG_BOOLEAN_AUTO;
static int cgroup_enable_pressure_memory_full = CONFIG_BOOLEAN_AUTO;

static int cgroup_enable_systemd_services = CONFIG_BOOLEAN_YES;
static int cgroup_enable_systemd_services_detailed_memory = CONFIG_BOOLEAN_NO;
static int cgroup_used_memory = CONFIG_BOOLEAN_YES;

static int cgroup_use_unified_cgroups = CONFIG_BOOLEAN_NO;
static int cgroup_unified_exist = CONFIG_BOOLEAN_AUTO;

static int cgroup_search_in_devices = 1;

static int cgroup_check_for_new_every = 10;
static int cgroup_update_every = 1;
static int cgroup_containers_chart_priority = NETDATA_CHART_PRIO_CGROUPS_CONTAINERS;

static int cgroup_recheck_zero_blkio_every_iterations = 10;
static int cgroup_recheck_zero_mem_failcnt_every_iterations = 10;
static int cgroup_recheck_zero_mem_detailed_every_iterations = 10;

static char *cgroup_cpuacct_base = NULL;
static char *cgroup_cpuset_base = NULL;
static char *cgroup_blkio_base = NULL;
static char *cgroup_memory_base = NULL;
static char *cgroup_devices_base = NULL;
static char *cgroup_unified_base = NULL;

static int cgroup_root_count = 0;
static int cgroup_root_max = 1000;
static int cgroup_max_depth = 0;

static SIMPLE_PATTERN *enabled_cgroup_paths = NULL;
static SIMPLE_PATTERN *enabled_cgroup_names = NULL;
static SIMPLE_PATTERN *search_cgroup_paths = NULL;
static SIMPLE_PATTERN *enabled_cgroup_renames = NULL;
static SIMPLE_PATTERN *systemd_services_cgroups = NULL;

static SIMPLE_PATTERN *entrypoint_parent_process_comm = NULL;

static char *cgroups_rename_script = NULL;
static char *cgroups_network_interface_script = NULL;

static int cgroups_check = 0;

static uint32_t Read_hash = 0;
static uint32_t Write_hash = 0;
static uint32_t user_hash = 0;
static uint32_t system_hash = 0;
static uint32_t user_usec_hash = 0;
static uint32_t system_usec_hash = 0;
static uint32_t nr_periods_hash = 0;
static uint32_t nr_throttled_hash = 0;
static uint32_t throttled_time_hash = 0;
static uint32_t throttled_usec_hash = 0;

enum cgroups_type { CGROUPS_AUTODETECT_FAIL, CGROUPS_V1, CGROUPS_V2 };

enum cgroups_systemd_setting {
    SYSTEMD_CGROUP_ERR,
    SYSTEMD_CGROUP_LEGACY,
    SYSTEMD_CGROUP_HYBRID,
    SYSTEMD_CGROUP_UNIFIED
};

struct cgroups_systemd_config_setting {
    char *name;
    enum cgroups_systemd_setting setting;
};

static struct cgroups_systemd_config_setting cgroups_systemd_options[] = {
    { .name = "legacy",  .setting = SYSTEMD_CGROUP_LEGACY  },
    { .name = "hybrid",  .setting = SYSTEMD_CGROUP_HYBRID  },
    { .name = "unified", .setting = SYSTEMD_CGROUP_UNIFIED },
    { .name = NULL,      .setting = SYSTEMD_CGROUP_ERR     },
};

// Shared memory with information from detected cgroups
netdata_ebpf_cgroup_shm_t shm_cgroup_ebpf = {NULL, NULL};
static int shm_fd_cgroup_ebpf = -1;
sem_t *shm_mutex_cgroup_ebpf = SEM_FAILED;

/* on Fed systemd is not in PATH for some reason */
#define SYSTEMD_CMD_RHEL "/usr/lib/systemd/systemd --version"
#define SYSTEMD_HIERARCHY_STRING "default-hierarchy="

#define MAXSIZE_PROC_CMDLINE 4096
static enum cgroups_systemd_setting cgroups_detect_systemd(const char *exec)
{
    pid_t command_pid;
    enum cgroups_systemd_setting retval = SYSTEMD_CGROUP_ERR;
    char buf[MAXSIZE_PROC_CMDLINE];
    char *begin, *end;

    FILE *f = mypopen(exec, &command_pid);

    if (!f)
        return retval;

    fd_set rfds;
    struct timeval timeout;
    int fd = fileno(f);
    int ret = -1;

    FD_ZERO(&rfds);
    FD_SET(fd, &rfds);
    timeout.tv_sec = 3;
    timeout.tv_usec = 0;

    if (fd != -1) {
        ret = select(fd + 1, &rfds, NULL, NULL, &timeout);
    }

    if (ret == -1) {
        error("Failed to get the output of \"%s\"", exec);
    } else if (ret == 0) {
        info("Cannot get the output of \"%s\" within %"PRId64" seconds", exec, (int64_t)timeout.tv_sec);
    } else {
        while (fgets(buf, MAXSIZE_PROC_CMDLINE, f) != NULL) {
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

    if (mypclose(f, command_pid))
        return SYSTEMD_CGROUP_ERR;

    return retval;
}

static enum cgroups_type cgroups_try_detect_version()
{
    pid_t command_pid;
    char buf[MAXSIZE_PROC_CMDLINE];
    enum cgroups_systemd_setting systemd_setting;
    int cgroups2_available = 0;

    // 1. check if cgroups2 available on system at all
    FILE *f = mypopen("grep cgroup /proc/filesystems", &command_pid);
    if (!f) {
        error("popen failed");
        return CGROUPS_AUTODETECT_FAIL;
    }
    while (fgets(buf, MAXSIZE_PROC_CMDLINE, f) != NULL) {
        if (strstr(buf, "cgroup2")) {
            cgroups2_available = 1;
            break;
        }
    }
    if(mypclose(f, command_pid))
        return CGROUPS_AUTODETECT_FAIL;

    if(!cgroups2_available)
        return CGROUPS_V1;

#if defined CGROUP2_SUPER_MAGIC
    // 2. check filesystem type for the default mountpoint
    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/sys/fs/cgroup");
    struct statfs fsinfo;
    if (!statfs(filename, &fsinfo)) {
        if (fsinfo.f_type == CGROUP2_SUPER_MAGIC)
            return CGROUPS_V2;
    }
#endif

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
    f = fopen("/proc/cmdline", "r");
    if (!f) {
        error("Error reading kernel boot commandline parameters");
        return CGROUPS_AUTODETECT_FAIL;
    }

    if (!fgets(buf, MAXSIZE_PROC_CMDLINE, f)) {
        error("couldn't read all cmdline params into buffer");
        fclose(f);
        return CGROUPS_AUTODETECT_FAIL;
    }

    fclose(f);

    if (strstr(buf, "systemd.unified_cgroup_hierarchy=0")) {
        info("cgroups v2 (unified cgroups) is available but are disabled on this system.");
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

    cgroup_update_every = (int)config_get_number("plugin:cgroups", "update every", localhost->rrd_update_every);
    if(cgroup_update_every < localhost->rrd_update_every)
        cgroup_update_every = localhost->rrd_update_every;

    cgroup_check_for_new_every = (int)config_get_number("plugin:cgroups", "check for new cgroups every", (long long)cgroup_check_for_new_every * (long long)cgroup_update_every);
    if(cgroup_check_for_new_every < cgroup_update_every)
        cgroup_check_for_new_every = cgroup_update_every;

    cgroup_use_unified_cgroups = config_get_boolean_ondemand("plugin:cgroups", "use unified cgroups", CONFIG_BOOLEAN_AUTO);
    if(cgroup_use_unified_cgroups == CONFIG_BOOLEAN_AUTO)
        cgroup_use_unified_cgroups = (cgroups_try_detect_version() == CGROUPS_V2);

    info("use unified cgroups %s", cgroup_use_unified_cgroups ? "true" : "false");

    cgroup_containers_chart_priority = (int)config_get_number("plugin:cgroups", "containers priority", cgroup_containers_chart_priority);
    if(cgroup_containers_chart_priority < 1)
        cgroup_containers_chart_priority = NETDATA_CHART_PRIO_CGROUPS_CONTAINERS;

    cgroup_enable_cpuacct_stat = config_get_boolean_ondemand("plugin:cgroups", "enable cpuacct stat (total CPU)", cgroup_enable_cpuacct_stat);
    cgroup_enable_cpuacct_usage = config_get_boolean_ondemand("plugin:cgroups", "enable cpuacct usage (per core CPU)", cgroup_enable_cpuacct_usage);
    cgroup_enable_cpuacct_cpu_throttling = config_get_boolean_ondemand("plugin:cgroups", "enable cpuacct cpu throttling", cgroup_enable_cpuacct_cpu_throttling);
    cgroup_enable_cpuacct_cpu_shares = config_get_boolean_ondemand("plugin:cgroups", "enable cpuacct cpu shares", cgroup_enable_cpuacct_cpu_shares);

    cgroup_enable_memory = config_get_boolean_ondemand("plugin:cgroups", "enable memory", cgroup_enable_memory);
    cgroup_enable_detailed_memory = config_get_boolean_ondemand("plugin:cgroups", "enable detailed memory", cgroup_enable_detailed_memory);
    cgroup_enable_memory_failcnt = config_get_boolean_ondemand("plugin:cgroups", "enable memory limits fail count", cgroup_enable_memory_failcnt);
    cgroup_enable_swap = config_get_boolean_ondemand("plugin:cgroups", "enable swap memory", cgroup_enable_swap);

    cgroup_enable_blkio_io = config_get_boolean_ondemand("plugin:cgroups", "enable blkio bandwidth", cgroup_enable_blkio_io);
    cgroup_enable_blkio_ops = config_get_boolean_ondemand("plugin:cgroups", "enable blkio operations", cgroup_enable_blkio_ops);
    cgroup_enable_blkio_throttle_io = config_get_boolean_ondemand("plugin:cgroups", "enable blkio throttle bandwidth", cgroup_enable_blkio_throttle_io);
    cgroup_enable_blkio_throttle_ops = config_get_boolean_ondemand("plugin:cgroups", "enable blkio throttle operations", cgroup_enable_blkio_throttle_ops);
    cgroup_enable_blkio_queued_ops = config_get_boolean_ondemand("plugin:cgroups", "enable blkio queued operations", cgroup_enable_blkio_queued_ops);
    cgroup_enable_blkio_merged_ops = config_get_boolean_ondemand("plugin:cgroups", "enable blkio merged operations", cgroup_enable_blkio_merged_ops);

    cgroup_enable_pressure_cpu = config_get_boolean_ondemand("plugin:cgroups", "enable cpu pressure", cgroup_enable_pressure_cpu);
    cgroup_enable_pressure_io_some = config_get_boolean_ondemand("plugin:cgroups", "enable io some pressure", cgroup_enable_pressure_io_some);
    cgroup_enable_pressure_io_full = config_get_boolean_ondemand("plugin:cgroups", "enable io full pressure", cgroup_enable_pressure_io_full);
    cgroup_enable_pressure_memory_some = config_get_boolean_ondemand("plugin:cgroups", "enable memory some pressure", cgroup_enable_pressure_memory_some);
    cgroup_enable_pressure_memory_full = config_get_boolean_ondemand("plugin:cgroups", "enable memory full pressure", cgroup_enable_pressure_memory_full);

    cgroup_recheck_zero_blkio_every_iterations = (int)config_get_number("plugin:cgroups", "recheck zero blkio every iterations", cgroup_recheck_zero_blkio_every_iterations);
    cgroup_recheck_zero_mem_failcnt_every_iterations = (int)config_get_number("plugin:cgroups", "recheck zero memory failcnt every iterations", cgroup_recheck_zero_mem_failcnt_every_iterations);
    cgroup_recheck_zero_mem_detailed_every_iterations = (int)config_get_number("plugin:cgroups", "recheck zero detailed memory every iterations", cgroup_recheck_zero_mem_detailed_every_iterations);

    cgroup_enable_systemd_services = config_get_boolean("plugin:cgroups", "enable systemd services", cgroup_enable_systemd_services);
    cgroup_enable_systemd_services_detailed_memory = config_get_boolean("plugin:cgroups", "enable systemd services detailed memory", cgroup_enable_systemd_services_detailed_memory);
    cgroup_used_memory = config_get_boolean("plugin:cgroups", "report used memory", cgroup_used_memory);

    char filename[FILENAME_MAX + 1], *s;
    struct mountinfo *mi, *root = mountinfo_read(0);
    if(!cgroup_use_unified_cgroups) {
        // cgroup v1 does not have pressure metrics
        cgroup_enable_pressure_cpu =
        cgroup_enable_pressure_io_some =
        cgroup_enable_pressure_io_full =
        cgroup_enable_pressure_memory_some =
        cgroup_enable_pressure_memory_full = CONFIG_BOOLEAN_NO;

        mi = mountinfo_find_by_filesystem_super_option(root, "cgroup", "cpuacct");
        if(!mi) mi = mountinfo_find_by_filesystem_mount_source(root, "cgroup", "cpuacct");
        if(!mi) {
            error("CGROUP: cannot find cpuacct mountinfo. Assuming default: /sys/fs/cgroup/cpuacct");
            s = "/sys/fs/cgroup/cpuacct";
        }
        else s = mi->mount_point;
        set_cgroup_base_path(filename, s);
        cgroup_cpuacct_base = config_get("plugin:cgroups", "path to /sys/fs/cgroup/cpuacct", filename);

        mi = mountinfo_find_by_filesystem_super_option(root, "cgroup", "cpuset");
        if(!mi) mi = mountinfo_find_by_filesystem_mount_source(root, "cgroup", "cpuset");
        if(!mi) {
            error("CGROUP: cannot find cpuset mountinfo. Assuming default: /sys/fs/cgroup/cpuset");
            s = "/sys/fs/cgroup/cpuset";
        }
        else s = mi->mount_point;
        set_cgroup_base_path(filename, s);
        cgroup_cpuset_base = config_get("plugin:cgroups", "path to /sys/fs/cgroup/cpuset", filename);

        mi = mountinfo_find_by_filesystem_super_option(root, "cgroup", "blkio");
        if(!mi) mi = mountinfo_find_by_filesystem_mount_source(root, "cgroup", "blkio");
        if(!mi) {
            error("CGROUP: cannot find blkio mountinfo. Assuming default: /sys/fs/cgroup/blkio");
            s = "/sys/fs/cgroup/blkio";
        }
        else s = mi->mount_point;
        set_cgroup_base_path(filename, s);
        cgroup_blkio_base = config_get("plugin:cgroups", "path to /sys/fs/cgroup/blkio", filename);

        mi = mountinfo_find_by_filesystem_super_option(root, "cgroup", "memory");
        if(!mi) mi = mountinfo_find_by_filesystem_mount_source(root, "cgroup", "memory");
        if(!mi) {
            error("CGROUP: cannot find memory mountinfo. Assuming default: /sys/fs/cgroup/memory");
            s = "/sys/fs/cgroup/memory";
        }
        else s = mi->mount_point;
        set_cgroup_base_path(filename, s);
        cgroup_memory_base = config_get("plugin:cgroups", "path to /sys/fs/cgroup/memory", filename);

        mi = mountinfo_find_by_filesystem_super_option(root, "cgroup", "devices");
        if(!mi) mi = mountinfo_find_by_filesystem_mount_source(root, "cgroup", "devices");
        if(!mi) {
            error("CGROUP: cannot find devices mountinfo. Assuming default: /sys/fs/cgroup/devices");
            s = "/sys/fs/cgroup/devices";
        }
        else s = mi->mount_point;
        set_cgroup_base_path(filename, s);
        cgroup_devices_base = config_get("plugin:cgroups", "path to /sys/fs/cgroup/devices", filename);
    }
    else {
        //cgroup_enable_cpuacct_stat =
        cgroup_enable_cpuacct_usage =
        //cgroup_enable_memory =
        //cgroup_enable_detailed_memory =
        cgroup_enable_memory_failcnt =
        //cgroup_enable_swap =
        //cgroup_enable_blkio_io =
        //cgroup_enable_blkio_ops =
        cgroup_enable_blkio_throttle_io =
        cgroup_enable_blkio_throttle_ops =
        cgroup_enable_blkio_merged_ops =
        cgroup_enable_blkio_queued_ops = CONFIG_BOOLEAN_NO;
        cgroup_search_in_devices = 0;
        cgroup_enable_systemd_services_detailed_memory = CONFIG_BOOLEAN_NO;
        cgroup_used_memory = CONFIG_BOOLEAN_NO; //unified cgroups use different values

        //TODO: can there be more than 1 cgroup2 mount point?
        mi = mountinfo_find_by_filesystem_super_option(root, "cgroup2", "rw"); //there is no cgroup2 specific super option - for now use 'rw' option
        if(mi) debug(D_CGROUP, "found unified cgroup root using super options, with path: '%s'", mi->mount_point);
        if(!mi) {
            mi = mountinfo_find_by_filesystem_mount_source(root, "cgroup2", "cgroup");
            if(mi) debug(D_CGROUP, "found unified cgroup root using mountsource info, with path: '%s'", mi->mount_point);
        }
        if(!mi) {
            error("CGROUP: cannot find cgroup2 mountinfo. Assuming default: /sys/fs/cgroup");
            s = "/sys/fs/cgroup";
        }
        else s = mi->mount_point;
        set_cgroup_base_path(filename, s);
        cgroup_unified_base = config_get("plugin:cgroups", "path to unified cgroups", filename);
        debug(D_CGROUP, "using cgroup root: '%s'", cgroup_unified_base);
    }

    cgroup_root_max = (int)config_get_number("plugin:cgroups", "max cgroups to allow", cgroup_root_max);
    cgroup_max_depth = (int)config_get_number("plugin:cgroups", "max cgroups depth to monitor", cgroup_max_depth);

    enabled_cgroup_paths = simple_pattern_create(
            config_get("plugin:cgroups", "enable by default cgroups matching",
            // ----------------------------------------------------------------

                    " !*/init.scope "                      // ignore init.scope
                    " !/system.slice/run-*.scope "         // ignore system.slice/run-XXXX.scope
                    " *.scope "                            // we need all other *.scope for sure

            // ----------------------------------------------------------------

                    " /machine.slice/*.service "           // #3367 systemd-nspawn
                    " /kubepods/pod*/* "                   // k8s containers
                    " /kubepods/*/pod*/* "                 // k8s containers

            // ----------------------------------------------------------------

                    " !/kubepods* "                        // all other k8s cgroups
                    " !*/vcpu* "                           // libvirtd adds these sub-cgroups
                    " !*/emulator "                        // libvirtd adds these sub-cgroups
                    " !*.mount "
                    " !*.partition "
                    " !*.service "
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
                    " !/machine "
                    " !/qemu "
                    " !/system "
                    " !/systemd "
                    " !/user "
                    " * "                                  // enable anything else
            ), NULL, SIMPLE_PATTERN_EXACT);

    enabled_cgroup_names = simple_pattern_create(
            config_get("plugin:cgroups", "enable by default cgroups names matching",
                    " * "
            ), NULL, SIMPLE_PATTERN_EXACT);

    search_cgroup_paths = simple_pattern_create(
            config_get("plugin:cgroups", "search for cgroups in subpaths matching",
                    " !*/init.scope "                      // ignore init.scope
                    " !*-qemu "                            //  #345
                    " !*.libvirt-qemu "                    //  #3010
                    " !/init.scope "
                    " !/system "
                    " !/systemd "
                    " !/user "
                    " !/user.slice "
                    " !/lxc/*/* "                          //  #2161 #2649
                    " !/lxc.monitor "
                    " !/lxc.payload/*/* "
                    " !/lxc.payload.* "
                    " * "
            ), NULL, SIMPLE_PATTERN_EXACT);

    snprintfz(filename, FILENAME_MAX, "%s/cgroup-name.sh", netdata_configured_primary_plugins_dir);
    cgroups_rename_script = config_get("plugin:cgroups", "script to get cgroup names", filename);

    snprintfz(filename, FILENAME_MAX, "%s/cgroup-network", netdata_configured_primary_plugins_dir);
    cgroups_network_interface_script = config_get("plugin:cgroups", "script to get cgroup network interfaces", filename);

    enabled_cgroup_renames = simple_pattern_create(
            config_get("plugin:cgroups", "run script to rename cgroups matching",
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
                    " /kubepods/pod*/* "                   // k8s containers
                    " /kubepods/*/pod*/* "                 // k8s containers
                    " !/kubepods* "                        // all other k8s cgroups
                    " *.libvirt-qemu "                    // #3010
                    " * "
            ), NULL, SIMPLE_PATTERN_EXACT);

    if(cgroup_enable_systemd_services) {
        systemd_services_cgroups = simple_pattern_create(
                config_get("plugin:cgroups", "cgroups to match as systemd services",
                        " !/system.slice/*/*.service "
                        " /system.slice/*.service "
                ), NULL, SIMPLE_PATTERN_EXACT);
    }

    mountinfo_free_all(root);
}

void netdata_cgroup_ebpf_set_values(size_t length)
{
    sem_wait(shm_mutex_cgroup_ebpf);

    shm_cgroup_ebpf.header->cgroup_max = cgroup_root_max;
    shm_cgroup_ebpf.header->systemd_enabled = cgroup_enable_systemd_services |
                                              cgroup_enable_systemd_services_detailed_memory |
                                              cgroup_used_memory;
    shm_cgroup_ebpf.header->body_length = length;

    sem_post(shm_mutex_cgroup_ebpf);
}

void netdata_cgroup_ebpf_initialize_shm()
{
    shm_fd_cgroup_ebpf = shm_open(NETDATA_SHARED_MEMORY_EBPF_CGROUP_NAME, O_CREAT | O_RDWR, 0660);
    if (shm_fd_cgroup_ebpf < 0) {
        error("Cannot initialize shared memory used by cgroup and eBPF, integration won't happen.");
        return;
    }

    size_t length = sizeof(netdata_ebpf_cgroup_shm_header_t) + cgroup_root_max * sizeof(netdata_ebpf_cgroup_shm_body_t);
    if (ftruncate(shm_fd_cgroup_ebpf, length)) {
        error("Cannot set size for shared memory.");
        goto end_init_shm;
    }

    shm_cgroup_ebpf.header = (netdata_ebpf_cgroup_shm_header_t *) mmap(NULL, length,
                                                                       PROT_READ | PROT_WRITE, MAP_SHARED,
                                                                       shm_fd_cgroup_ebpf, 0);

    if (!shm_cgroup_ebpf.header) {
        error("Cannot map shared memory used between cgroup and eBPF, integration won't happen");
        goto end_init_shm;
    }
    shm_cgroup_ebpf.body = (netdata_ebpf_cgroup_shm_body_t *) ((char *)shm_cgroup_ebpf.header +
                                                              sizeof(netdata_ebpf_cgroup_shm_header_t));

    shm_mutex_cgroup_ebpf = sem_open(NETDATA_NAMED_SEMAPHORE_EBPF_CGROUP_NAME, O_CREAT,
                                     S_IRUSR | S_IWUSR | S_IRGRP | S_IWGRP | S_IROTH | S_IWOTH, 1);

    if (shm_mutex_cgroup_ebpf != SEM_FAILED) {
        netdata_cgroup_ebpf_set_values(length);
        return;
    }

    error("Cannot create semaphore, integration between eBPF and cgroup won't happen");
    munmap(shm_cgroup_ebpf.header, length);

end_init_shm:
    close(shm_fd_cgroup_ebpf);
    shm_fd_cgroup_ebpf = -1;
    shm_unlink(NETDATA_SHARED_MEMORY_EBPF_CGROUP_NAME);
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
/*
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
    unsigned long long inactive_anon;
    unsigned long long active_anon;
    unsigned long long inactive_file;
    unsigned long long active_file;
    unsigned long long unevictable;
    unsigned long long hierarchical_memory_limit;
*/
    //unified cgroups metrics
    unsigned long long anon;
    unsigned long long kernel_stack;
    unsigned long long slab;
    unsigned long long sock;
    unsigned long long shmem;
    unsigned long long anon_thp;
    //unsigned long long file_writeback;
    //unsigned long long file_dirty;
    //unsigned long long file;

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
/*
    unsigned long long total_inactive_anon;
    unsigned long long total_active_anon;
*/

    unsigned long long total_inactive_file;

/*
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
    int enabled;            // CONFIG_BOOLEAN_YES or CONFIG_BOOLEAN_AUTO

    char *filename;

    unsigned long long user;           // v1, v2(user_usec)
    unsigned long long system;         // v1, v2(system_usec)
};

// https://www.kernel.org/doc/Documentation/cgroup-v1/cpuacct.txt
struct cpuacct_usage {
    int updated;
    int enabled; // CONFIG_BOOLEAN_YES or CONFIG_BOOLEAN_AUTO

    char *filename;

    unsigned int cpus;
    unsigned long long *cpu_percpu;
};

// represents cpuacct/cpu.stat, for v2 'cpuacct_stat' is used for 'user_usec', 'system_usec'
struct cpuacct_cpu_throttling {
    int updated;
    int enabled; // CONFIG_BOOLEAN_YES or CONFIG_BOOLEAN_AUTO

    char *filename;

    unsigned long long nr_periods;
    unsigned long long nr_throttled;
    unsigned long long throttled_time;

    unsigned long long nr_throttled_perc;
};

// https://access.redhat.com/documentation/en-us/red_hat_enterprise_linux/6/html/resource_management_guide/sec-cpu#sect-cfs
// https://access.redhat.com/documentation/en-us/red_hat_enterprise_linux/8/html/managing_monitoring_and_updating_the_kernel/using-cgroups-v2-to-control-distribution-of-cpu-time-for-applications_managing-monitoring-and-updating-the-kernel#proc_controlling-distribution-of-cpu-time-for-applications-by-adjusting-cpu-weight_using-cgroups-v2-to-control-distribution-of-cpu-time-for-applications
struct cpuacct_cpu_shares {
    int updated;
    int enabled; // CONFIG_BOOLEAN_YES or CONFIG_BOOLEAN_AUTO

    char *filename;

    unsigned long long shares;
};

struct cgroup_network_interface {
    const char *host_device;
    const char *container_device;
    struct cgroup_network_interface *next;
};

// *** WARNING *** The fields are not thread safe. Take care of safe usage.
struct cgroup {
    uint32_t options;

    int first_time_seen; // first time seen by the discoverer
    int processed;       // the discoverer is done processing a cgroup (resolved name, set 'enabled' option)

    char available;      // found in the filesystem
    char enabled;        // enabled in the config

    char pending_renames;
    char *intermediate_id; // TODO: remove it when the renaming script is fixed

    char *id;
    uint32_t hash;

    char *chart_id;
    uint32_t hash_chart;

    char *chart_title;

    struct label *chart_labels;

    struct cpuacct_stat cpuacct_stat;
    struct cpuacct_usage cpuacct_usage;
    struct cpuacct_cpu_throttling cpuacct_cpu_throttling;
    struct cpuacct_cpu_shares cpuacct_cpu_shares;

    struct memory memory;

    struct blkio io_service_bytes;              // bytes
    struct blkio io_serviced;                   // operations

    struct blkio throttle_io_service_bytes;     // bytes
    struct blkio throttle_io_serviced;          // operations

    struct blkio io_merged;                     // operations
    struct blkio io_queued;                     // operations

    struct cgroup_network_interface *interfaces;

    struct pressure cpu_pressure;
    struct pressure io_pressure;
    struct pressure memory_pressure;

    // per cgroup charts
    RRDSET *st_cpu;
    RRDSET *st_cpu_limit;
    RRDSET *st_cpu_per_core;
    RRDSET *st_cpu_nr_throttled;
    RRDSET *st_cpu_throttled_time;
    RRDSET *st_cpu_shares;

    RRDSET *st_mem;
    RRDSET *st_mem_utilization;
    RRDSET *st_writeback;
    RRDSET *st_mem_activity;
    RRDSET *st_pgfaults;
    RRDSET *st_mem_usage;
    RRDSET *st_mem_usage_limit;
    RRDSET *st_mem_failcnt;

    RRDSET *st_io;
    RRDSET *st_serviced_ops;
    RRDSET *st_throttle_io;
    RRDSET *st_throttle_serviced_ops;
    RRDSET *st_queued_ops;
    RRDSET *st_merged_ops;

    // per cgroup chart variables
    char *filename_cpuset_cpus;
    unsigned long long cpuset_cpus;

    char *filename_cpu_cfs_period;
    unsigned long long cpu_cfs_period;

    char *filename_cpu_cfs_quota;
    unsigned long long cpu_cfs_quota;

    RRDSETVAR *chart_var_cpu_limit;
    calculated_number prev_cpu_usage;

    char *filename_memory_limit;
    unsigned long long memory_limit;
    RRDSETVAR *chart_var_memory_limit;

    char *filename_memoryswap_limit;
    unsigned long long memoryswap_limit;
    RRDSETVAR *chart_var_memoryswap_limit;

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
    struct cgroup *discovered_next;

} *cgroup_root = NULL;

uv_mutex_t cgroup_root_mutex;

struct cgroup *discovered_cgroup_root = NULL;

struct discovery_thread {
    uv_thread_t thread;
    uv_mutex_t mutex;
    uv_cond_t cond_var;
    int start_discovery;
    int exited;
} discovery_thread;

// ---------------------------------------------------------------------------------------------

static inline int matches_enabled_cgroup_paths(char *id) {
    return simple_pattern_matches(enabled_cgroup_paths, id);
}

static inline int matches_enabled_cgroup_names(char *name) {
    return simple_pattern_matches(enabled_cgroup_names, name);
}

static inline int matches_enabled_cgroup_renames(char *id) {
    return simple_pattern_matches(enabled_cgroup_renames, id);
}

static inline int matches_systemd_services_cgroups(char *id) {
    return simple_pattern_matches(systemd_services_cgroups, id);
}

static inline int matches_search_cgroup_paths(const char *dir) {
    return simple_pattern_matches(search_cgroup_paths, dir);
}

static inline int matches_entrypoint_parent_process_comm(const char *comm) {
    return simple_pattern_matches(entrypoint_parent_process_comm, comm);
}

static inline int is_cgroup_systemd_service(struct cgroup *cg) {
    return (cg->options & CGROUP_OPTIONS_SYSTEM_SLICE_SERVICE);
}

// ---------------------------------------------------------------------------------------------
static int k8s_is_container(const char *id) {
    // examples:
    // https://github.com/netdata/netdata/blob/0fc101679dcd12f1cb8acdd07bb4c85d8e553e53/collectors/cgroups.plugin/cgroup-name.sh#L121-L147
    const char *p = id;
    const char *pp = NULL;
    int i = 0;
    size_t l = 3; // pod
    while ((p = strstr(p, "pod"))) {
        i++;
        p += l;
        pp = p;
    }
    return !(i < 2 || !pp || !(pp = strchr(pp, '/')) || !pp++ || !*pp);
}

#define TASK_COMM_LEN 16

static int k8s_get_container_first_proc_comm(const char *id, char *comm) {
    if (!k8s_is_container(id)) {
        return 1;
    }

    static procfile *ff = NULL;

    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s/%s/cgroup.procs", cgroup_cpuacct_base, id);

    ff = procfile_reopen(ff, filename, NULL, PROCFILE_FLAG_DEFAULT);
    if (unlikely(!ff)) {
        debug(D_CGROUP, "CGROUP: k8s_is_pause_container(): cannot open file '%s'.", filename);
        return 1;
    }

    ff = procfile_readall(ff);
    if (unlikely(!ff)) {
        debug(D_CGROUP, "CGROUP: k8s_is_pause_container(): cannot read file '%s'.", filename);
        return 1;
    }

    unsigned long lines = procfile_lines(ff);
    if (likely(lines < 2)) {
        return 1;
    }

    char *pid = procfile_lineword(ff, 0, 0);
    if (!pid || !*pid) {
        return 1;
    }

    snprintfz(filename, FILENAME_MAX, "%s/proc/%s/comm", netdata_configured_host_prefix, pid);

    ff = procfile_reopen(ff, filename, NULL, PROCFILE_FLAG_DEFAULT);
    if (unlikely(!ff)) {
        debug(D_CGROUP, "CGROUP: k8s_is_pause_container(): cannot open file '%s'.", filename);
        return 1;
    }

    ff = procfile_readall(ff);
    if (unlikely(!ff)) {
        debug(D_CGROUP, "CGROUP: k8s_is_pause_container(): cannot read file '%s'.", filename);
        return 1;
    }

    lines = procfile_lines(ff);
    if (unlikely(lines != 2)) {
        return 1;
    }

    char *proc_comm = procfile_lineword(ff, 0, 0);
    if (!proc_comm || !*proc_comm) {
        return 1;
    }

    strncpyz(comm, proc_comm, TASK_COMM_LEN);
    return 0;
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
    return (calculated_number)value / (calculated_number)total * 100;
}

static int calc_cgroup_depth(const char *id) {
    int depth = 0;
    const char *s;
    for (s = id; *s; s++) {
        depth += unlikely(*s == '/');
    }
    return depth;
}

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

        if(unlikely(cp->enabled == CONFIG_BOOLEAN_AUTO &&
                    (cp->user || cp->system || netdata_zero_metrics_enabled == CONFIG_BOOLEAN_YES)))
            cp->enabled = CONFIG_BOOLEAN_YES;
    }
}

static inline void cgroup_read_cpuacct_cpu_stat(struct cpuacct_cpu_throttling *cp) {
    if (unlikely(!cp->filename)) {
        return;
    }

    static procfile *ff = NULL;
    ff = procfile_reopen(ff, cp->filename, NULL, PROCFILE_FLAG_DEFAULT);
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
        error("CGROUP: file '%s' should have 3 lines.", cp->filename);
        cp->updated = 0;
        return;
    }

    unsigned long long nr_periods_last = cp->nr_periods; 
    unsigned long long nr_throttled_last = cp->nr_throttled; 

    for (unsigned long i = 0; i < lines; i++) {
        char *s = procfile_lineword(ff, i, 0);
        uint32_t hash = simple_hash(s);

        if (unlikely(hash == nr_periods_hash && !strcmp(s, "nr_periods"))) {
            cp->nr_periods = str2ull(procfile_lineword(ff, i, 1));
        } else if (unlikely(hash == nr_throttled_hash && !strcmp(s, "nr_throttled"))) {
            cp->nr_throttled = str2ull(procfile_lineword(ff, i, 1));
        } else if (unlikely(hash == throttled_time_hash && !strcmp(s, "throttled_time"))) {
            cp->throttled_time = str2ull(procfile_lineword(ff, i, 1));
        }
    }
    cp->nr_throttled_perc =
        calc_percentage(calc_delta(cp->nr_throttled, nr_throttled_last), calc_delta(cp->nr_periods, nr_periods_last));

    cp->updated = 1;

    if (unlikely(cp->enabled == CONFIG_BOOLEAN_AUTO)) {
        if (likely(
                cp->nr_periods || cp->nr_throttled || cp->throttled_time ||
                netdata_zero_metrics_enabled == CONFIG_BOOLEAN_YES)) {
            cp->enabled = CONFIG_BOOLEAN_YES;
        }
    }
}

static inline void cgroup2_read_cpuacct_cpu_stat(struct cpuacct_stat *cp, struct cpuacct_cpu_throttling *cpt) {
    static procfile *ff = NULL;
    if (unlikely(!cp->filename)) {
        return;
    }

    ff = procfile_reopen(ff, cp->filename, NULL, PROCFILE_FLAG_DEFAULT);
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
        error("CGROUP: file '%s' should have at least 3 lines.", cp->filename);
        cp->updated = 0;
        return;
    }

    unsigned long long nr_periods_last = cpt->nr_periods; 
    unsigned long long nr_throttled_last = cpt->nr_throttled; 

    for (unsigned long i = 0; i < lines; i++) {
        char *s = procfile_lineword(ff, i, 0);
        uint32_t hash = simple_hash(s);

        if (unlikely(hash == user_usec_hash && !strcmp(s, "user_usec"))) {
            cp->user = str2ull(procfile_lineword(ff, i, 1));
        } else if (unlikely(hash == system_usec_hash && !strcmp(s, "system_usec"))) {
            cp->system = str2ull(procfile_lineword(ff, i, 1));
        } else if (unlikely(hash == nr_periods_hash && !strcmp(s, "nr_periods"))) {
            cpt->nr_periods = str2ull(procfile_lineword(ff, i, 1));
        } else if (unlikely(hash == nr_throttled_hash && !strcmp(s, "nr_throttled"))) {
            cpt->nr_throttled = str2ull(procfile_lineword(ff, i, 1));
        } else if (unlikely(hash == throttled_usec_hash && !strcmp(s, "throttled_usec"))) {
            cpt->throttled_time = str2ull(procfile_lineword(ff, i, 1)) * 1000; // usec -> ns
        }
    }
    cpt->nr_throttled_perc =
        calc_percentage(calc_delta(cpt->nr_throttled, nr_throttled_last), calc_delta(cpt->nr_periods, nr_periods_last));

    cp->updated = 1;
    cpt->updated = 1;

    if (unlikely(cp->enabled == CONFIG_BOOLEAN_AUTO)) {
        if (likely(cp->user || cp->system || netdata_zero_metrics_enabled == CONFIG_BOOLEAN_YES)) {
            cp->enabled = CONFIG_BOOLEAN_YES;
        }
    }
    if (unlikely(cpt->enabled == CONFIG_BOOLEAN_AUTO)) {
        if (likely(
                cpt->nr_periods || cpt->nr_throttled || cpt->throttled_time ||
                netdata_zero_metrics_enabled == CONFIG_BOOLEAN_YES)) {
            cpt->enabled = CONFIG_BOOLEAN_YES;
        }
    }
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
    if (unlikely((cp->enabled == CONFIG_BOOLEAN_AUTO)) &&
        (cp->shares || netdata_zero_metrics_enabled == CONFIG_BOOLEAN_YES)) {
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

        if(unlikely(ca->enabled == CONFIG_BOOLEAN_AUTO &&
                    (total || netdata_zero_metrics_enabled == CONFIG_BOOLEAN_YES)))
            ca->enabled = CONFIG_BOOLEAN_YES;
    }
}

static inline void cgroup_read_blkio(struct blkio *io) {
    if(unlikely(io->enabled == CONFIG_BOOLEAN_AUTO && io->delay_counter > 0)) {
        io->delay_counter--;
        return;
    }

    if(likely(io->filename)) {
        static procfile *ff = NULL;

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
            if(unlikely(io->Read || io->Write || netdata_zero_metrics_enabled == CONFIG_BOOLEAN_YES))
                io->enabled = CONFIG_BOOLEAN_YES;
            else
                io->delay_counter = cgroup_recheck_zero_blkio_every_iterations;
        }
    }
}

static inline void cgroup2_read_blkio(struct blkio *io, unsigned int word_offset) {
    if(unlikely(io->enabled == CONFIG_BOOLEAN_AUTO && io->delay_counter > 0)) {
            io->delay_counter--;
            return;
        }

        if(likely(io->filename)) {
            static procfile *ff = NULL;

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

            if (unlikely(lines < 1)) {
                error("CGROUP: file '%s' should have 1+ lines.", io->filename);
                io->updated = 0;
                return;
            }

            io->Read = 0;
            io->Write = 0;

            for (i = 0; i < lines; i++) {
                io->Read += str2ull(procfile_lineword(ff, i, 2 + word_offset));
                io->Write += str2ull(procfile_lineword(ff, i, 4 + word_offset));
            }

            io->updated = 1;

            if(unlikely(io->enabled == CONFIG_BOOLEAN_AUTO)) {
                if(unlikely(io->Read || io->Write || netdata_zero_metrics_enabled == CONFIG_BOOLEAN_YES))
                    io->enabled = CONFIG_BOOLEAN_YES;
                else
                    io->delay_counter = cgroup_recheck_zero_blkio_every_iterations;
            }
        }
}

static inline void cgroup2_read_pressure(struct pressure *res) {
    static procfile *ff = NULL;

    if (likely(res->filename)) {
        ff = procfile_reopen(ff, res->filename, " =", PROCFILE_FLAG_DEFAULT);
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
            error("CGROUP: file '%s' should have 1+ lines.", res->filename);
            res->updated = 0;
            return;
        }

    
        res->some.share_time.value10 = strtod(procfile_lineword(ff, 0, 2), NULL);
        res->some.share_time.value60 = strtod(procfile_lineword(ff, 0, 4), NULL);
        res->some.share_time.value300 = strtod(procfile_lineword(ff, 0, 6), NULL);
        res->some.total_time.value_total = str2ull(procfile_lineword(ff, 0, 8)) / 1000; // us->ms

        if (lines > 2) {
            res->full.share_time.value10 = strtod(procfile_lineword(ff, 1, 2), NULL);
            res->full.share_time.value60 = strtod(procfile_lineword(ff, 1, 4), NULL);
            res->full.share_time.value300 = strtod(procfile_lineword(ff, 1, 6), NULL);
            res->full.total_time.value_total = str2ull(procfile_lineword(ff, 0, 8)) / 1000; // us->ms
        }

        res->updated = 1;

        if (unlikely(res->some.enabled == CONFIG_BOOLEAN_AUTO)) {
            res->some.enabled = CONFIG_BOOLEAN_YES;
            if (lines > 2) {
                res->full.enabled = CONFIG_BOOLEAN_YES;
            } else {
                res->full.enabled = CONFIG_BOOLEAN_NO;
            }
        }
    }
}

static inline void cgroup_read_memory(struct memory *mem, char parent_cg_is_unified) {
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

        for(i = 0; i < lines ; i++) {
            if(arl_check(mem->arl_base,
                    procfile_lineword(ff, i, 0),
                    procfile_lineword(ff, i, 1))) break;
        }

        if(unlikely(mem->arl_dirty->flags & ARL_ENTRY_FLAG_FOUND))
            mem->detailed_has_dirty = 1;

        if(unlikely(parent_cg_is_unified == 0 && mem->arl_swap->flags & ARL_ENTRY_FLAG_FOUND))
            mem->detailed_has_swap = 1;

        // fprintf(stderr, "READ: '%s', cache: %llu, rss: %llu, rss_huge: %llu, mapped_file: %llu, writeback: %llu, dirty: %llu, swap: %llu, pgpgin: %llu, pgpgout: %llu, pgfault: %llu, pgmajfault: %llu, inactive_anon: %llu, active_anon: %llu, inactive_file: %llu, active_file: %llu, unevictable: %llu, hierarchical_memory_limit: %llu, total_cache: %llu, total_rss: %llu, total_rss_huge: %llu, total_mapped_file: %llu, total_writeback: %llu, total_dirty: %llu, total_swap: %llu, total_pgpgin: %llu, total_pgpgout: %llu, total_pgfault: %llu, total_pgmajfault: %llu, total_inactive_anon: %llu, total_active_anon: %llu, total_inactive_file: %llu, total_active_file: %llu, total_unevictable: %llu\n", mem->filename, mem->cache, mem->rss, mem->rss_huge, mem->mapped_file, mem->writeback, mem->dirty, mem->swap, mem->pgpgin, mem->pgpgout, mem->pgfault, mem->pgmajfault, mem->inactive_anon, mem->active_anon, mem->inactive_file, mem->active_file, mem->unevictable, mem->hierarchical_memory_limit, mem->total_cache, mem->total_rss, mem->total_rss_huge, mem->total_mapped_file, mem->total_writeback, mem->total_dirty, mem->total_swap, mem->total_pgpgin, mem->total_pgpgout, mem->total_pgfault, mem->total_pgmajfault, mem->total_inactive_anon, mem->total_active_anon, mem->total_inactive_file, mem->total_active_file, mem->total_unevictable);

        mem->updated_detailed = 1;

        if(unlikely(mem->enabled_detailed == CONFIG_BOOLEAN_AUTO)) {
            if(( (!parent_cg_is_unified) && ( mem->total_cache || mem->total_dirty || mem->total_rss || mem->total_rss_huge || mem->total_mapped_file || mem->total_writeback
                    || mem->total_swap || mem->total_pgpgin || mem->total_pgpgout || mem->total_pgfault || mem->total_pgmajfault || mem->total_inactive_file))
               || (parent_cg_is_unified && ( mem->anon || mem->total_dirty || mem->kernel_stack || mem->slab || mem->sock || mem->total_writeback
                    || mem->anon_thp || mem->total_pgfault || mem->total_pgmajfault || mem->total_inactive_file))
               || netdata_zero_metrics_enabled == CONFIG_BOOLEAN_YES)
                mem->enabled_detailed = CONFIG_BOOLEAN_YES;
            else
                mem->delay_counter_detailed = cgroup_recheck_zero_mem_detailed_every_iterations;
        }
    }

memory_next:

    // read usage_in_bytes
    if(likely(mem->filename_usage_in_bytes)) {
        mem->updated_usage_in_bytes = !read_single_number_file(mem->filename_usage_in_bytes, &mem->usage_in_bytes);
        if(unlikely(mem->updated_usage_in_bytes && mem->enabled_usage_in_bytes == CONFIG_BOOLEAN_AUTO &&
                    (mem->usage_in_bytes || netdata_zero_metrics_enabled == CONFIG_BOOLEAN_YES)))
            mem->enabled_usage_in_bytes = CONFIG_BOOLEAN_YES;
    }

    if (likely(mem->updated_usage_in_bytes && mem->updated_detailed)) {
        mem->usage_in_bytes =
            (mem->usage_in_bytes > mem->total_inactive_file) ? (mem->usage_in_bytes - mem->total_inactive_file) : 0;
    }

    // read msw_usage_in_bytes
    if(likely(mem->filename_msw_usage_in_bytes)) {
        mem->updated_msw_usage_in_bytes = !read_single_number_file(mem->filename_msw_usage_in_bytes, &mem->msw_usage_in_bytes);
        if(unlikely(mem->updated_msw_usage_in_bytes && mem->enabled_msw_usage_in_bytes == CONFIG_BOOLEAN_AUTO &&
                    (mem->msw_usage_in_bytes || netdata_zero_metrics_enabled == CONFIG_BOOLEAN_YES)))
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
                if(unlikely(mem->failcnt || netdata_zero_metrics_enabled == CONFIG_BOOLEAN_YES))
                    mem->enabled_failcnt = CONFIG_BOOLEAN_YES;
                else
                    mem->delay_counter_failcnt = cgroup_recheck_zero_mem_failcnt_every_iterations;
            }
        }
    }
}

static inline void read_cgroup(struct cgroup *cg) {
    debug(D_CGROUP, "reading metrics for cgroups '%s'", cg->id);
    if(!(cg->options & CGROUP_OPTIONS_IS_UNIFIED)) {
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
    }
    else {
        //TODO: io_service_bytes and io_serviced use same file merge into 1 function
        cgroup2_read_blkio(&cg->io_service_bytes, 0);
        cgroup2_read_blkio(&cg->io_serviced, 4);
        cgroup2_read_cpuacct_cpu_stat(&cg->cpuacct_stat, &cg->cpuacct_cpu_throttling);
        cgroup_read_cpuacct_cpu_shares(&cg->cpuacct_cpu_shares);
        cgroup2_read_pressure(&cg->cpu_pressure);
        cgroup2_read_pressure(&cg->io_pressure);
        cgroup2_read_pressure(&cg->memory_pressure);
        cgroup_read_memory(&cg->memory, 1);
    }
}

static inline void read_all_discovered_cgroups(struct cgroup *root) {
    debug(D_CGROUP, "reading metrics for all cgroups");

    struct cgroup *cg;
    for (cg = root; cg; cg = cg->next) {
        if (cg->enabled && !cg->pending_renames) {
            read_cgroup(cg);
        }
    }
}

// ----------------------------------------------------------------------------
// cgroup network interfaces

#define CGROUP_NETWORK_INTERFACE_MAX_LINE 2048
static inline void read_cgroup_network_interfaces(struct cgroup *cg) {
    debug(D_CGROUP, "looking for the network interfaces of cgroup '%s' with chart id '%s' and title '%s'", cg->id, cg->chart_id, cg->chart_title);

    pid_t cgroup_pid;
    char cgroup_identifier[CGROUP_NETWORK_INTERFACE_MAX_LINE + 1];

    if(!(cg->options & CGROUP_OPTIONS_IS_UNIFIED)) {
        snprintfz(cgroup_identifier, CGROUP_NETWORK_INTERFACE_MAX_LINE, "%s%s", cgroup_cpuacct_base, cg->id);
    }
    else {
        snprintfz(cgroup_identifier, CGROUP_NETWORK_INTERFACE_MAX_LINE, "%s%s", cgroup_unified_base, cg->id);
    }

    debug(D_CGROUP, "executing cgroup_identifier %s --cgroup '%s' for cgroup '%s'", cgroups_network_interface_script, cgroup_identifier, cg->id);
    FILE *fp;
    (void)mypopen_raw_default_flags_and_environment(&cgroup_pid, &fp, cgroups_network_interface_script, "--cgroup", cgroup_identifier);
    if(!fp) {
        error("CGROUP: cannot popen(%s --cgroup \"%s\", \"r\").", cgroups_network_interface_script, cgroup_identifier);
        return;
    }

    char *s;
    char buffer[CGROUP_NETWORK_INTERFACE_MAX_LINE + 1];
    while((s = fgets(buffer, CGROUP_NETWORK_INTERFACE_MAX_LINE, fp))) {
        trim(s);

        if(*s && *s != '\n') {
            char *t = s;
            while(*t && *t != ' ') t++;
            if(*t == ' ') {
                *t = '\0';
                t++;
            }

            if(!*s) {
                error("CGROUP: empty host interface returned by script");
                continue;
            }

            if(!*t) {
                error("CGROUP: empty guest interface returned by script");
                continue;
            }

            struct cgroup_network_interface *i = callocz(1, sizeof(struct cgroup_network_interface));
            i->host_device = strdupz(s);
            i->container_device = strdupz(t);
            i->next = cg->interfaces;
            cg->interfaces = i;

            info("CGROUP: cgroup '%s' has network interface '%s' as '%s'", cg->id, i->host_device, i->container_device);

            // register a device rename to proc_net_dev.c
            netdev_rename_device_add(i->host_device, i->container_device, cg->chart_id, cg->chart_labels);
        }
    }

    mypclose(fp, cgroup_pid);
    // debug(D_CGROUP, "closed cgroup_identifier for cgroup '%s'", cg->id);
}

static inline void free_cgroup_network_interfaces(struct cgroup *cg) {
    while(cg->interfaces) {
        struct cgroup_network_interface *i = cg->interfaces;
        cg->interfaces = i->next;

        // delete the registration of proc_net_dev rename
        netdev_rename_device_del(i->host_device);

        freez((void *)i->host_device);
        freez((void *)i->container_device);
        freez((void *)i);
    }
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

// TODO: move the code to cgroup_chart_id_strdupz() when the renaming script is fixed
static inline void substitute_dots_in_id(char *s) {
    // dots are used to distinguish chart type and id in streaming, so we should replace them
    for (char *d = s; *d; d++) {
        if (*d == '.')
            *d = '-';
    }
}

char *k8s_parse_resolved_name(struct label **labels, char *data) {
    char *name = mystrsep(&data, " ");
    
    if (!data) {
        return name;
    }

    while (data) {
        char *key = mystrsep(&data, "=");

        char *value;
        if (data && *data == ',') {
            value = "";
            *data++ = '\0';
        } else {
            value = mystrsep(&data, ",");
        }
        value = strip_double_quotes(value, 1);

        if (!key || *key == '\0' || !value || *value == '\0')
            continue;

        *labels = add_label_to_list(*labels, key, value, LABEL_SOURCE_KUBERNETES);
    }

    return name;
}

static inline void free_pressure(struct pressure *res) {
    if (res->some.share_time.st)   rrdset_is_obsolete(res->some.share_time.st);
    if (res->some.total_time.st)   rrdset_is_obsolete(res->some.total_time.st);
    if (res->full.share_time.st)   rrdset_is_obsolete(res->full.share_time.st);
    if (res->full.total_time.st)   rrdset_is_obsolete(res->full.total_time.st);
    freez(res->filename);
}

static inline void cgroup_free(struct cgroup *cg) {
    debug(D_CGROUP, "Removing cgroup '%s' with chart id '%s' (was %s and %s)", cg->id, cg->chart_id, (cg->enabled)?"enabled":"disabled", (cg->available)?"available":"not available");

    if(cg->st_cpu)                   rrdset_is_obsolete(cg->st_cpu);
    if(cg->st_cpu_limit)             rrdset_is_obsolete(cg->st_cpu_limit);
    if(cg->st_cpu_per_core)          rrdset_is_obsolete(cg->st_cpu_per_core);
    if(cg->st_cpu_nr_throttled)      rrdset_is_obsolete(cg->st_cpu_nr_throttled);
    if(cg->st_cpu_throttled_time)    rrdset_is_obsolete(cg->st_cpu_throttled_time);
    if(cg->st_cpu_shares)            rrdset_is_obsolete(cg->st_cpu_shares);
    if(cg->st_mem)                   rrdset_is_obsolete(cg->st_mem);
    if(cg->st_writeback)             rrdset_is_obsolete(cg->st_writeback);
    if(cg->st_mem_activity)          rrdset_is_obsolete(cg->st_mem_activity);
    if(cg->st_pgfaults)              rrdset_is_obsolete(cg->st_pgfaults);
    if(cg->st_mem_usage)             rrdset_is_obsolete(cg->st_mem_usage);
    if(cg->st_mem_usage_limit)       rrdset_is_obsolete(cg->st_mem_usage_limit);
    if(cg->st_mem_utilization)       rrdset_is_obsolete(cg->st_mem_utilization);
    if(cg->st_mem_failcnt)           rrdset_is_obsolete(cg->st_mem_failcnt);
    if(cg->st_io)                    rrdset_is_obsolete(cg->st_io);
    if(cg->st_serviced_ops)          rrdset_is_obsolete(cg->st_serviced_ops);
    if(cg->st_throttle_io)           rrdset_is_obsolete(cg->st_throttle_io);
    if(cg->st_throttle_serviced_ops) rrdset_is_obsolete(cg->st_throttle_serviced_ops);
    if(cg->st_queued_ops)            rrdset_is_obsolete(cg->st_queued_ops);
    if(cg->st_merged_ops)            rrdset_is_obsolete(cg->st_merged_ops);

    freez(cg->filename_cpuset_cpus);
    freez(cg->filename_cpu_cfs_period);
    freez(cg->filename_cpu_cfs_quota);
    freez(cg->filename_memory_limit);
    freez(cg->filename_memoryswap_limit);

    free_cgroup_network_interfaces(cg);

    freez(cg->cpuacct_usage.cpu_percpu);

    freez(cg->cpuacct_stat.filename);
    freez(cg->cpuacct_usage.filename);
    freez(cg->cpuacct_cpu_throttling.filename);
    freez(cg->cpuacct_cpu_shares.filename);

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

    free_pressure(&cg->cpu_pressure);
    free_pressure(&cg->io_pressure);
    free_pressure(&cg->memory_pressure);

    freez(cg->id);
    freez(cg->intermediate_id);
    freez(cg->chart_id);
    freez(cg->chart_title);

    free_label_list(cg->chart_labels);

    freez(cg);

    cgroup_root_count--;
}

// ----------------------------------------------------------------------------

static inline void discovery_rename_cgroup(struct cgroup *cg) {
    if (!cg->pending_renames) {
        return;
    }
    cg->pending_renames--;

    debug(D_CGROUP, "looking for the name of cgroup '%s' with chart id '%s' and title '%s'", cg->id, cg->chart_id, cg->chart_title);
    debug(D_CGROUP, "executing command %s \"%s\" for cgroup '%s'", cgroups_rename_script, cg->intermediate_id, cg->chart_id);
    pid_t cgroup_pid;

    FILE *fp;
    (void)mypopen_raw_default_flags_and_environment(&cgroup_pid, &fp, cgroups_rename_script, cg->id, cg->intermediate_id);
    if (!fp) {
        error("CGROUP: cannot popen(%s \"%s\", \"r\").", cgroups_rename_script, cg->intermediate_id);
        cg->pending_renames = 0;
        cg->processed = 1;
        return;
    }

    char buffer[CGROUP_CHARTID_LINE_MAX + 1];
    char *new_name = fgets(buffer, CGROUP_CHARTID_LINE_MAX, fp);
    int exit_code = mypclose(fp, cgroup_pid);

    switch (exit_code) {
        case 0:
            cg->pending_renames = 0;
            break;
        case 3:
            cg->pending_renames = 0;
            cg->processed = 1;
            break;
    }

    if (cg->pending_renames || cg->processed) {
        return;
    }
    if (!(new_name && *new_name && *new_name != '\n')) {
        return;
    }
    new_name = trim(new_name);
    if (!(new_name)) {
        return;
    }
    char *name = new_name;
    if (!strncmp(new_name, "k8s_", 4)) {
        free_label_list(cg->chart_labels);
        name = k8s_parse_resolved_name(&cg->chart_labels, new_name);
    }
    freez(cg->chart_title);
    cg->chart_title = cgroup_title_strdupz(name);
    freez(cg->chart_id);
    cg->chart_id = cgroup_chart_id_strdupz(name);
    substitute_dots_in_id(cg->chart_id);
    cg->hash_chart = simple_hash(cg->chart_id);
}

static void is_cgroup_procs_exist(netdata_ebpf_cgroup_shm_body_t *out, char *id) {
    struct stat buf;

    snprintfz(out->path, FILENAME_MAX, "%s%s/cgroup.procs", cgroup_cpuset_base, id);
    if (likely(stat(out->path, &buf) == 0)) {
        return;
    }

    snprintfz(out->path, FILENAME_MAX, "%s%s/cgroup.procs", cgroup_blkio_base, id);
    if (likely(stat(out->path, &buf) == 0)) {
        return;
    }

    snprintfz(out->path, FILENAME_MAX, "%s%s/cgroup.procs", cgroup_memory_base, id);
    if (likely(stat(out->path, &buf) == 0)) {
        return;
    }

    snprintfz(out->path, FILENAME_MAX, "%s%s/cgroup.procs", cgroup_devices_base, id);
    if (likely(stat(out->path, &buf) == 0)) {
        return;
    }

    out->path[0] = '\0';
    out->enabled = 0;
}

static inline void convert_cgroup_to_systemd_service(struct cgroup *cg) {
    char buffer[CGROUP_CHARTID_LINE_MAX];
    cg->options |= CGROUP_OPTIONS_SYSTEM_SLICE_SERVICE;
    strncpyz(buffer, cg->id, CGROUP_CHARTID_LINE_MAX);
    char *s = buffer;

    // skip to the last slash
    size_t len = strlen(s);
    while (len--) {
        if (unlikely(s[len] == '/')) {
            break;
        }
    }
    if (len) {
        s = &s[len + 1];
    }

    // remove extension
    len = strlen(s);
    while (len--) {
        if (unlikely(s[len] == '.')) {
            break;
        }
    }
    if (len) {
        s[len] = '\0';
    }

    freez(cg->chart_title);
    cg->chart_title = cgroup_title_strdupz(s);
}

static inline struct cgroup *discovery_cgroup_add(const char *id) {
    debug(D_CGROUP, "adding to list, cgroup with id '%s'", id);

    struct cgroup *cg = callocz(1, sizeof(struct cgroup));
    cg->id = strdupz(id);
    cg->hash = simple_hash(cg->id);
    cg->chart_title = cgroup_title_strdupz(id);
    cg->intermediate_id = cgroup_chart_id_strdupz(id);
    cg->chart_id = cgroup_chart_id_strdupz(id);
    substitute_dots_in_id(cg->chart_id);
    cg->hash_chart = simple_hash(cg->chart_id);
    if (cgroup_use_unified_cgroups) {
        cg->options |= CGROUP_OPTIONS_IS_UNIFIED;
    }

    if (!discovered_cgroup_root)
        discovered_cgroup_root = cg;
    else {
        struct cgroup *t;
        for (t = discovered_cgroup_root; t->discovered_next; t = t->discovered_next) {
        }
        t->discovered_next = cg;
    }

    return cg;
}

static inline struct cgroup *discovery_cgroup_find(const char *id) {
    debug(D_CGROUP, "searching for cgroup '%s'", id);

    uint32_t hash = simple_hash(id);

    struct cgroup *cg;
    for(cg = discovered_cgroup_root; cg ; cg = cg->discovered_next) {
        if(hash == cg->hash && strcmp(id, cg->id) == 0)
            break;
    }

    debug(D_CGROUP, "cgroup '%s' %s in memory", id, (cg)?"found":"not found");
    return cg;
}

static inline void discovery_find_cgroup_in_dir_callback(const char *dir) {
    if (!dir || !*dir) {
        dir = "/";
    }
    debug(D_CGROUP, "examining cgroup dir '%s'", dir);

    struct cgroup *cg = discovery_cgroup_find(dir);
    if (cg) {
        cg->available = 1;
        return;
    }

    if (cgroup_root_count >= cgroup_root_max) {
        info("CGROUP: maximum number of cgroups reached (%d). Not adding cgroup '%s'", cgroup_root_count, dir);
        return;
    }

    if (cgroup_max_depth > 0) {
        int depth = calc_cgroup_depth(dir);
        if (depth > cgroup_max_depth) {
            info("CGROUP: '%s' is too deep (%d, while max is %d)", dir, depth, cgroup_max_depth);
            return;
        }
    }

    cg = discovery_cgroup_add(dir);
    cg->available = 1;
    cg->first_time_seen = 1;
    cgroup_root_count++;
}

static inline int discovery_find_dir_in_subdirs(const char *base, const char *this, void (*callback)(const char *)) {
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
                enabled = matches_search_cgroup_paths(r);
            }

            if(enabled) {
                char *s = mallocz(dirlen + strlen(de->d_name) + 2);
                strcpy(s, this);
                strcat(s, "/");
                strcat(s, de->d_name);
                int ret2 = discovery_find_dir_in_subdirs(base, s, callback);
                if(ret2 > 0) ret += ret2;
                freez(s);
            }
        }
    }

    closedir(dir);
    return ret;
}

static inline void discovery_mark_all_cgroups_as_unavailable() {
    debug(D_CGROUP, "marking all cgroups as not available");
    struct cgroup *cg;
    for (cg = discovered_cgroup_root; cg; cg = cg->discovered_next) {
        cg->available = 0;
    }
}

static inline void discovery_update_filenames() {
    struct cgroup *cg;
    struct stat buf;
    for(cg = discovered_cgroup_root; cg ; cg = cg->discovered_next) {
        if(unlikely(!cg->available || !cg->enabled || cg->pending_renames))
            continue;

        debug(D_CGROUP, "checking paths for cgroup '%s'", cg->id);

        // check for newly added cgroups
        // and update the filenames they read
        char filename[FILENAME_MAX + 1];
        if(!cgroup_use_unified_cgroups) {
            if(unlikely(cgroup_enable_cpuacct_stat && !cg->cpuacct_stat.filename)) {
                snprintfz(filename, FILENAME_MAX, "%s%s/cpuacct.stat", cgroup_cpuacct_base, cg->id);
                if(likely(stat(filename, &buf) != -1)) {
                    cg->cpuacct_stat.filename = strdupz(filename);
                    cg->cpuacct_stat.enabled = cgroup_enable_cpuacct_stat;
                    snprintfz(filename, FILENAME_MAX, "%s%s/cpuset.cpus", cgroup_cpuset_base, cg->id);
                    cg->filename_cpuset_cpus = strdupz(filename);
                    snprintfz(filename, FILENAME_MAX, "%s%s/cpu.cfs_period_us", cgroup_cpuacct_base, cg->id);
                    cg->filename_cpu_cfs_period = strdupz(filename);
                    snprintfz(filename, FILENAME_MAX, "%s%s/cpu.cfs_quota_us", cgroup_cpuacct_base, cg->id);
                    cg->filename_cpu_cfs_quota = strdupz(filename);
                    debug(D_CGROUP, "cpuacct.stat filename for cgroup '%s': '%s'", cg->id, cg->cpuacct_stat.filename);
                }
                else
                    debug(D_CGROUP, "cpuacct.stat file for cgroup '%s': '%s' does not exist.", cg->id, filename);
            }

            if(unlikely(cgroup_enable_cpuacct_usage && !cg->cpuacct_usage.filename && !is_cgroup_systemd_service(cg))) {
                snprintfz(filename, FILENAME_MAX, "%s%s/cpuacct.usage_percpu", cgroup_cpuacct_base, cg->id);
                if(likely(stat(filename, &buf) != -1)) {
                    cg->cpuacct_usage.filename = strdupz(filename);
                    cg->cpuacct_usage.enabled = cgroup_enable_cpuacct_usage;
                    debug(D_CGROUP, "cpuacct.usage_percpu filename for cgroup '%s': '%s'", cg->id, cg->cpuacct_usage.filename);
                }
                else
                    debug(D_CGROUP, "cpuacct.usage_percpu file for cgroup '%s': '%s' does not exist.", cg->id, filename);
            }
            if(unlikely(cgroup_enable_cpuacct_cpu_throttling && !cg->cpuacct_cpu_throttling.filename && !is_cgroup_systemd_service(cg))) {
                snprintfz(filename, FILENAME_MAX, "%s%s/cpu.stat", cgroup_cpuacct_base, cg->id);
                if(likely(stat(filename, &buf) != -1)) {
                    cg->cpuacct_cpu_throttling.filename = strdupz(filename);
                    cg->cpuacct_cpu_throttling.enabled = cgroup_enable_cpuacct_cpu_throttling;
                    debug(D_CGROUP, "cpu.stat filename for cgroup '%s': '%s'", cg->id, cg->cpuacct_cpu_throttling.filename);
                }
                else
                    debug(D_CGROUP, "cpu.stat file for cgroup '%s': '%s' does not exist.", cg->id, filename);
            }
            if (unlikely(
                    cgroup_enable_cpuacct_cpu_shares && !cg->cpuacct_cpu_shares.filename &&
                    !is_cgroup_systemd_service(cg))) {
                snprintfz(filename, FILENAME_MAX, "%s%s/cpu.shares", cgroup_cpuacct_base, cg->id);
                if (likely(stat(filename, &buf) != -1)) {
                    cg->cpuacct_cpu_shares.filename = strdupz(filename);
                    cg->cpuacct_cpu_shares.enabled = cgroup_enable_cpuacct_cpu_shares;
                    debug(
                        D_CGROUP, "cpu.shares filename for cgroup '%s': '%s'", cg->id, cg->cpuacct_cpu_shares.filename);
                } else
                    debug(D_CGROUP, "cpu.shares file for cgroup '%s': '%s' does not exist.", cg->id, filename);
            }

            if(unlikely((cgroup_enable_detailed_memory || cgroup_used_memory) && !cg->memory.filename_detailed && (cgroup_used_memory || cgroup_enable_systemd_services_detailed_memory || !is_cgroup_systemd_service(cg)))) {
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
                    snprintfz(filename, FILENAME_MAX, "%s%s/memory.limit_in_bytes", cgroup_memory_base, cg->id);
                    cg->filename_memory_limit = strdupz(filename);
                }
                else
                    debug(D_CGROUP, "memory.usage_in_bytes file for cgroup '%s': '%s' does not exist.", cg->id, filename);
            }

            if(unlikely(cgroup_enable_swap && !cg->memory.filename_msw_usage_in_bytes)) {
                snprintfz(filename, FILENAME_MAX, "%s%s/memory.memsw.usage_in_bytes", cgroup_memory_base, cg->id);
                if(likely(stat(filename, &buf) != -1)) {
                    cg->memory.filename_msw_usage_in_bytes = strdupz(filename);
                    cg->memory.enabled_msw_usage_in_bytes = cgroup_enable_swap;
                    snprintfz(filename, FILENAME_MAX, "%s%s/memory.memsw.limit_in_bytes", cgroup_memory_base, cg->id);
                    cg->filename_memoryswap_limit = strdupz(filename);
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
                snprintfz(filename, FILENAME_MAX, "%s%s/blkio.io_service_bytes_recursive", cgroup_blkio_base, cg->id);
                if (unlikely(stat(filename, &buf) != -1)) {
                    cg->io_service_bytes.filename = strdupz(filename);
                    cg->io_service_bytes.enabled = cgroup_enable_blkio_io;
                    debug(D_CGROUP, "blkio.io_service_bytes_recursive filename for cgroup '%s': '%s'", cg->id, cg->io_service_bytes.filename);
                } else {
                    debug(D_CGROUP, "blkio.io_service_bytes_recursive file for cgroup '%s': '%s' does not exist.", cg->id, filename);
                    snprintfz(filename, FILENAME_MAX, "%s%s/blkio.io_service_bytes", cgroup_blkio_base, cg->id);
                    if (likely(stat(filename, &buf) != -1)) {
                        cg->io_service_bytes.filename = strdupz(filename);
                        cg->io_service_bytes.enabled = cgroup_enable_blkio_io;
                        debug(D_CGROUP, "blkio.io_service_bytes filename for cgroup '%s': '%s'", cg->id, cg->io_service_bytes.filename);
                    } else {
                        debug(D_CGROUP, "blkio.io_service_bytes file for cgroup '%s': '%s' does not exist.", cg->id, filename);
                    }
                }
            }

            if (unlikely(cgroup_enable_blkio_ops && !cg->io_serviced.filename)) {
                snprintfz(filename, FILENAME_MAX, "%s%s/blkio.io_serviced_recursive", cgroup_blkio_base, cg->id);
                if (unlikely(stat(filename, &buf) != -1)) {
                    cg->io_serviced.filename = strdupz(filename);
                    cg->io_serviced.enabled = cgroup_enable_blkio_ops;
                    debug(D_CGROUP, "blkio.io_serviced_recursive filename for cgroup '%s': '%s'", cg->id, cg->io_serviced.filename);
                } else {
                    debug(D_CGROUP, "blkio.io_serviced_recursive file for cgroup '%s': '%s' does not exist.", cg->id, filename);
                    snprintfz(filename, FILENAME_MAX, "%s%s/blkio.io_serviced", cgroup_blkio_base, cg->id);
                    if (likely(stat(filename, &buf) != -1)) {
                        cg->io_serviced.filename = strdupz(filename);
                        cg->io_serviced.enabled = cgroup_enable_blkio_ops;
                        debug(D_CGROUP, "blkio.io_serviced filename for cgroup '%s': '%s'", cg->id, cg->io_serviced.filename);
                    } else {
                        debug(D_CGROUP, "blkio.io_serviced file for cgroup '%s': '%s' does not exist.", cg->id, filename);
                    }
                }
            }

            if (unlikely(cgroup_enable_blkio_throttle_io && !cg->throttle_io_service_bytes.filename)) {
                snprintfz(filename, FILENAME_MAX, "%s%s/blkio.throttle.io_service_bytes_recursive", cgroup_blkio_base, cg->id);
                if (unlikely(stat(filename, &buf) != -1)) {
                    cg->throttle_io_service_bytes.filename = strdupz(filename);
                    cg->throttle_io_service_bytes.enabled = cgroup_enable_blkio_throttle_io;
                    debug(D_CGROUP,"blkio.throttle.io_service_bytes_recursive filename for cgroup '%s': '%s'", cg->id, cg->throttle_io_service_bytes.filename);
                } else {
                    debug(D_CGROUP, "blkio.throttle.io_service_bytes_recursive file for cgroup '%s': '%s' does not exist.", cg->id, filename);
                    snprintfz(
                        filename, FILENAME_MAX, "%s%s/blkio.throttle.io_service_bytes", cgroup_blkio_base, cg->id);
                    if (likely(stat(filename, &buf) != -1)) {
                        cg->throttle_io_service_bytes.filename = strdupz(filename);
                        cg->throttle_io_service_bytes.enabled = cgroup_enable_blkio_throttle_io;
                        debug(D_CGROUP, "blkio.throttle.io_service_bytes filename for cgroup '%s': '%s'", cg->id, cg->throttle_io_service_bytes.filename);
                    } else {
                        debug(D_CGROUP, "blkio.throttle.io_service_bytes file for cgroup '%s': '%s' does not exist.", cg->id, filename);
                    }
                }
            }

            if (unlikely(cgroup_enable_blkio_throttle_ops && !cg->throttle_io_serviced.filename)) {
                snprintfz(filename, FILENAME_MAX, "%s%s/blkio.throttle.io_serviced_recursive", cgroup_blkio_base, cg->id);
                if (unlikely(stat(filename, &buf) != -1)) {
                    cg->throttle_io_serviced.filename = strdupz(filename);
                    cg->throttle_io_serviced.enabled = cgroup_enable_blkio_throttle_ops;
                    debug(D_CGROUP, "blkio.throttle.io_serviced_recursive filename for cgroup '%s': '%s'", cg->id, cg->throttle_io_serviced.filename);
                } else {
                    debug(D_CGROUP, "blkio.throttle.io_serviced_recursive file for cgroup '%s': '%s' does not exist.", cg->id, filename);
                    snprintfz(filename, FILENAME_MAX, "%s%s/blkio.throttle.io_serviced", cgroup_blkio_base, cg->id);
                    if (likely(stat(filename, &buf) != -1)) {
                        cg->throttle_io_serviced.filename = strdupz(filename);
                        cg->throttle_io_serviced.enabled = cgroup_enable_blkio_throttle_ops;
                        debug(D_CGROUP, "blkio.throttle.io_serviced filename for cgroup '%s': '%s'", cg->id, cg->throttle_io_serviced.filename);
                    } else {
                        debug(D_CGROUP, "blkio.throttle.io_serviced file for cgroup '%s': '%s' does not exist.", cg->id, filename);
                    }
                }
            }

            if (unlikely(cgroup_enable_blkio_merged_ops && !cg->io_merged.filename)) {
                snprintfz(filename, FILENAME_MAX, "%s%s/blkio.io_merged_recursive", cgroup_blkio_base, cg->id);
                if (unlikely(stat(filename, &buf) != -1)) {
                    cg->io_merged.filename = strdupz(filename);
                    cg->io_merged.enabled = cgroup_enable_blkio_merged_ops;
                    debug(D_CGROUP, "blkio.io_merged_recursive filename for cgroup '%s': '%s'", cg->id, cg->io_merged.filename);
                } else {
                    debug(D_CGROUP, "blkio.io_merged_recursive file for cgroup '%s': '%s' does not exist.", cg->id, filename);
                    snprintfz(filename, FILENAME_MAX, "%s%s/blkio.io_merged", cgroup_blkio_base, cg->id);
                    if (likely(stat(filename, &buf) != -1)) {
                        cg->io_merged.filename = strdupz(filename);
                        cg->io_merged.enabled = cgroup_enable_blkio_merged_ops;
                        debug(D_CGROUP, "blkio.io_merged filename for cgroup '%s': '%s'", cg->id, cg->io_merged.filename);
                    } else {
                        debug(D_CGROUP, "blkio.io_merged file for cgroup '%s': '%s' does not exist.", cg->id, filename);
                    }
                }
            }

            if (unlikely(cgroup_enable_blkio_queued_ops && !cg->io_queued.filename)) {
                snprintfz(filename, FILENAME_MAX, "%s%s/blkio.io_queued_recursive", cgroup_blkio_base, cg->id);
                if (unlikely(stat(filename, &buf) != -1)) {
                    cg->io_queued.filename = strdupz(filename);
                    cg->io_queued.enabled = cgroup_enable_blkio_queued_ops;
                    debug(D_CGROUP, "blkio.io_queued_recursive filename for cgroup '%s': '%s'", cg->id, cg->io_queued.filename);
                } else {
                    debug(D_CGROUP, "blkio.io_queued_recursive file for cgroup '%s': '%s' does not exist.", cg->id, filename);
                    snprintfz(filename, FILENAME_MAX, "%s%s/blkio.io_queued", cgroup_blkio_base, cg->id);
                    if (likely(stat(filename, &buf) != -1)) {
                        cg->io_queued.filename = strdupz(filename);
                        cg->io_queued.enabled = cgroup_enable_blkio_queued_ops;
                        debug(D_CGROUP, "blkio.io_queued filename for cgroup '%s': '%s'", cg->id, cg->io_queued.filename);
                    } else {
                        debug(D_CGROUP, "blkio.io_queued file for cgroup '%s': '%s' does not exist.", cg->id, filename);
                    }
                }
            }
        }
        else if(likely(cgroup_unified_exist)) {
            if(unlikely(cgroup_enable_blkio_io && !cg->io_service_bytes.filename)) {
                snprintfz(filename, FILENAME_MAX, "%s%s/io.stat", cgroup_unified_base, cg->id);
                if(likely(stat(filename, &buf) != -1)) {
                    cg->io_service_bytes.filename = strdupz(filename);
                    cg->io_service_bytes.enabled = cgroup_enable_blkio_io;
                    debug(D_CGROUP, "io.stat filename for unified cgroup '%s': '%s'", cg->id, cg->io_service_bytes.filename);
                } else
                    debug(D_CGROUP, "io.stat file for unified cgroup '%s': '%s' does not exist.", cg->id, filename);
            }
            if (unlikely(cgroup_enable_blkio_ops && !cg->io_serviced.filename)) {
                snprintfz(filename, FILENAME_MAX, "%s%s/io.stat", cgroup_unified_base, cg->id);
                if (likely(stat(filename, &buf) != -1)) {
                    cg->io_serviced.filename = strdupz(filename);
                    cg->io_serviced.enabled = cgroup_enable_blkio_ops;
                    debug(D_CGROUP, "io.stat filename for unified cgroup '%s': '%s'", cg->id, cg->io_service_bytes.filename);
                } else
                    debug(D_CGROUP, "io.stat file for unified cgroup '%s': '%s' does not exist.", cg->id, filename);
            }
            if (unlikely(
                    (cgroup_enable_cpuacct_stat || cgroup_enable_cpuacct_cpu_throttling) &&
                    !cg->cpuacct_stat.filename)) {
                snprintfz(filename, FILENAME_MAX, "%s%s/cpu.stat", cgroup_unified_base, cg->id);
                if(likely(stat(filename, &buf) != -1)) {
                    cg->cpuacct_stat.filename = strdupz(filename);
                    cg->cpuacct_stat.enabled = cgroup_enable_cpuacct_stat;
                    cg->cpuacct_cpu_throttling.enabled = cgroup_enable_cpuacct_cpu_throttling;
                    cg->filename_cpuset_cpus = NULL;
                    cg->filename_cpu_cfs_period = NULL;
                    snprintfz(filename, FILENAME_MAX, "%s%s/cpu.max", cgroup_unified_base, cg->id);
                    cg->filename_cpu_cfs_quota = strdupz(filename);
                    debug(D_CGROUP, "cpu.stat filename for unified cgroup '%s': '%s'", cg->id, cg->cpuacct_stat.filename);
                }
                else
                    debug(D_CGROUP, "cpu.stat file for unified cgroup '%s': '%s' does not exist.", cg->id, filename);
            }
            if (unlikely(cgroup_enable_cpuacct_cpu_shares && !cg->cpuacct_cpu_shares.filename)) {
                snprintfz(filename, FILENAME_MAX, "%s%s/cpu.weight", cgroup_unified_base, cg->id);
                if (likely(stat(filename, &buf) != -1)) {
                    cg->cpuacct_cpu_shares.filename = strdupz(filename);
                    cg->cpuacct_cpu_shares.enabled = cgroup_enable_cpuacct_cpu_shares;
                    debug(D_CGROUP, "cpu.weight filename for cgroup '%s': '%s'", cg->id, cg->cpuacct_cpu_shares.filename);
                } else
                    debug(D_CGROUP, "cpu.weight file for cgroup '%s': '%s' does not exist.", cg->id, filename);
            }

            if(unlikely((cgroup_enable_detailed_memory || cgroup_used_memory) && !cg->memory.filename_detailed && (cgroup_used_memory || cgroup_enable_systemd_services_detailed_memory || !is_cgroup_systemd_service(cg)))) {
                snprintfz(filename, FILENAME_MAX, "%s%s/memory.stat", cgroup_unified_base, cg->id);
                if(likely(stat(filename, &buf) != -1)) {
                    cg->memory.filename_detailed = strdupz(filename);
                    cg->memory.enabled_detailed = (cgroup_enable_detailed_memory == CONFIG_BOOLEAN_YES)?CONFIG_BOOLEAN_YES:CONFIG_BOOLEAN_AUTO;
                    debug(D_CGROUP, "memory.stat filename for cgroup '%s': '%s'", cg->id, cg->memory.filename_detailed);
                }
                else
                    debug(D_CGROUP, "memory.stat file for cgroup '%s': '%s' does not exist.", cg->id, filename);
            }

            if(unlikely(cgroup_enable_memory && !cg->memory.filename_usage_in_bytes)) {
                snprintfz(filename, FILENAME_MAX, "%s%s/memory.current", cgroup_unified_base, cg->id);
                if(likely(stat(filename, &buf) != -1)) {
                    cg->memory.filename_usage_in_bytes = strdupz(filename);
                    cg->memory.enabled_usage_in_bytes = cgroup_enable_memory;
                    debug(D_CGROUP, "memory.current filename for cgroup '%s': '%s'", cg->id, cg->memory.filename_usage_in_bytes);
                    snprintfz(filename, FILENAME_MAX, "%s%s/memory.max", cgroup_unified_base, cg->id);
                    cg->filename_memory_limit = strdupz(filename);
                }
                else
                    debug(D_CGROUP, "memory.current file for cgroup '%s': '%s' does not exist.", cg->id, filename);
            }

            if(unlikely(cgroup_enable_swap && !cg->memory.filename_msw_usage_in_bytes)) {
                snprintfz(filename, FILENAME_MAX, "%s%s/memory.swap.current", cgroup_unified_base, cg->id);
                if(likely(stat(filename, &buf) != -1)) {
                    cg->memory.filename_msw_usage_in_bytes = strdupz(filename);
                    cg->memory.enabled_msw_usage_in_bytes = cgroup_enable_swap;
                    snprintfz(filename, FILENAME_MAX, "%s%s/memory.swap.max", cgroup_unified_base, cg->id);
                    cg->filename_memoryswap_limit = strdupz(filename);
                    debug(D_CGROUP, "memory.swap.current filename for cgroup '%s': '%s'", cg->id, cg->memory.filename_msw_usage_in_bytes);
                }
                else
                    debug(D_CGROUP, "memory.swap file for cgroup '%s': '%s' does not exist.", cg->id, filename);
            }

            if (unlikely(cgroup_enable_pressure_cpu && !cg->cpu_pressure.filename)) {
                snprintfz(filename, FILENAME_MAX, "%s%s/cpu.pressure", cgroup_unified_base, cg->id);
                if (likely(stat(filename, &buf) != -1)) {
                    cg->cpu_pressure.filename = strdupz(filename);
                    cg->cpu_pressure.some.enabled = cgroup_enable_pressure_cpu;
                    cg->cpu_pressure.full.enabled = CONFIG_BOOLEAN_NO;
                    debug(D_CGROUP, "cpu.pressure filename for cgroup '%s': '%s'", cg->id, cg->cpu_pressure.filename);
                } else {
                    debug(D_CGROUP, "cpu.pressure file for cgroup '%s': '%s' does not exist", cg->id, filename);
                }
            }

            if (unlikely((cgroup_enable_pressure_io_some || cgroup_enable_pressure_io_full) && !cg->io_pressure.filename)) {
                snprintfz(filename, FILENAME_MAX, "%s%s/io.pressure", cgroup_unified_base, cg->id);
                if (likely(stat(filename, &buf) != -1)) {
                    cg->io_pressure.filename = strdupz(filename);
                    cg->io_pressure.some.enabled = cgroup_enable_pressure_io_some;
                    cg->io_pressure.full.enabled = cgroup_enable_pressure_io_full;
                    debug(D_CGROUP, "io.pressure filename for cgroup '%s': '%s'", cg->id, cg->io_pressure.filename);
                } else {
                    debug(D_CGROUP, "io.pressure file for cgroup '%s': '%s' does not exist", cg->id, filename);
                }
            }

            if (unlikely((cgroup_enable_pressure_memory_some || cgroup_enable_pressure_memory_full) && !cg->memory_pressure.filename)) {
                snprintfz(filename, FILENAME_MAX, "%s%s/memory.pressure", cgroup_unified_base, cg->id);
                if (likely(stat(filename, &buf) != -1)) {
                    cg->memory_pressure.filename = strdupz(filename);
                    cg->memory_pressure.some.enabled = cgroup_enable_pressure_memory_some;
                    cg->memory_pressure.full.enabled = cgroup_enable_pressure_memory_full;
                    debug(D_CGROUP, "memory.pressure filename for cgroup '%s': '%s'", cg->id, cg->memory_pressure.filename);
                } else {
                    debug(D_CGROUP, "memory.pressure file for cgroup '%s': '%s' does not exist", cg->id, filename);
                }
            }
        }
    }
}

static inline void discovery_cleanup_all_cgroups() {
    struct cgroup *cg = discovered_cgroup_root, *last = NULL;

    for(; cg ;) {
        if(!cg->available) {
            // enable the first duplicate cgroup
            {
                struct cgroup *t;
                for(t = discovered_cgroup_root; t ; t = t->discovered_next) {
                    if(t != cg && t->available && !t->enabled && t->options & CGROUP_OPTIONS_DISABLED_DUPLICATE && t->hash_chart == cg->hash_chart && !strcmp(t->chart_id, cg->chart_id)) {
                        debug(D_CGROUP, "Enabling duplicate of cgroup '%s' with id '%s', because the original with id '%s' stopped.", t->chart_id, t->id, cg->id);
                        t->enabled = 1;
                        t->options &= ~CGROUP_OPTIONS_DISABLED_DUPLICATE;
                        break;
                    }
                }
            }

            if(!last)
                discovered_cgroup_root = cg->discovered_next;
            else
                last->discovered_next = cg->discovered_next;

            cgroup_free(cg);

            if(!last)
                cg = discovered_cgroup_root;
            else
                cg = last->discovered_next;
        }
        else {
            last = cg;
            cg = cg->discovered_next;
        }
    }
}

static inline void discovery_copy_discovered_cgroups_to_reader() {
    debug(D_CGROUP, "copy discovered cgroups to the main group list");

    struct cgroup *cg;

    for (cg = discovered_cgroup_root; cg; cg = cg->discovered_next) {
        cg->next = cg->discovered_next;
    }

    cgroup_root = discovered_cgroup_root;
}

static inline void discovery_share_cgroups_with_ebpf() {
    struct cgroup *cg;
    int count;
    struct stat buf;

    if (shm_mutex_cgroup_ebpf == SEM_FAILED) {
        return;
    }
    sem_wait(shm_mutex_cgroup_ebpf);

    for (cg = cgroup_root, count = 0; cg; cg = cg->next, count++) {
        netdata_ebpf_cgroup_shm_body_t *ptr = &shm_cgroup_ebpf.body[count];
        char *prefix = (is_cgroup_systemd_service(cg)) ? "" : "cgroup_";
        snprintfz(ptr->name, CGROUP_EBPF_NAME_SHARED_LENGTH - 1, "%s%s", prefix, cg->chart_title);
        ptr->hash = simple_hash(ptr->name);
        ptr->options = cg->options;
        ptr->enabled = cg->enabled;
        if (cgroup_use_unified_cgroups) {
            snprintfz(ptr->path, FILENAME_MAX, "%s%s/cgroup.procs", cgroup_unified_base, cg->id);
            if (likely(stat(ptr->path, &buf) == -1)) {
                ptr->path[0] = '\0';
                ptr->enabled = 0;
            }
        } else {
            is_cgroup_procs_exist(ptr, cg->id);
        }

        debug(D_CGROUP, "cgroup shared: NAME=%s, ENABLED=%d", ptr->name, ptr->enabled);
    }

    shm_cgroup_ebpf.header->cgroup_root_count = count;
    sem_post(shm_mutex_cgroup_ebpf);
}

static inline void discovery_find_all_cgroups_v1() {
    if (cgroup_enable_cpuacct_stat || cgroup_enable_cpuacct_usage) {
        if (discovery_find_dir_in_subdirs(cgroup_cpuacct_base, NULL, discovery_find_cgroup_in_dir_callback) == -1) {
            cgroup_enable_cpuacct_stat = cgroup_enable_cpuacct_usage = CONFIG_BOOLEAN_NO;
            error("CGROUP: disabled cpu statistics.");
        }
    }

    if (cgroup_enable_blkio_io || cgroup_enable_blkio_ops || cgroup_enable_blkio_throttle_io ||
        cgroup_enable_blkio_throttle_ops || cgroup_enable_blkio_merged_ops || cgroup_enable_blkio_queued_ops) {
        if (discovery_find_dir_in_subdirs(cgroup_blkio_base, NULL, discovery_find_cgroup_in_dir_callback) == -1) {
            cgroup_enable_blkio_io = cgroup_enable_blkio_ops = cgroup_enable_blkio_throttle_io =
                cgroup_enable_blkio_throttle_ops = cgroup_enable_blkio_merged_ops = cgroup_enable_blkio_queued_ops =
                    CONFIG_BOOLEAN_NO;
            error("CGROUP: disabled blkio statistics.");
        }
    }

    if (cgroup_enable_memory || cgroup_enable_detailed_memory || cgroup_enable_swap || cgroup_enable_memory_failcnt) {
        if (discovery_find_dir_in_subdirs(cgroup_memory_base, NULL, discovery_find_cgroup_in_dir_callback) == -1) {
            cgroup_enable_memory = cgroup_enable_detailed_memory = cgroup_enable_swap = cgroup_enable_memory_failcnt =
                CONFIG_BOOLEAN_NO;
            error("CGROUP: disabled memory statistics.");
        }
    }

    if (cgroup_search_in_devices) {
        if (discovery_find_dir_in_subdirs(cgroup_devices_base, NULL, discovery_find_cgroup_in_dir_callback) == -1) {
            cgroup_search_in_devices = 0;
            error("CGROUP: disabled devices statistics.");
        }
    }
}

static inline void discovery_find_all_cgroups_v2() {
    if (discovery_find_dir_in_subdirs(cgroup_unified_base, NULL, discovery_find_cgroup_in_dir_callback) == -1) {
        cgroup_unified_exist = CONFIG_BOOLEAN_NO;
        error("CGROUP: disabled unified cgroups statistics.");
    }
}

static int is_digits_only(const char *s) {
  do {
    if (!isdigit(*s++)) {
      return 0;
    }
  } while (*s);

  return 1;
}

static inline void discovery_process_first_time_seen_cgroup(struct cgroup *cg) {
    if (!cg->first_time_seen) {
        return;
    }
    cg->first_time_seen = 0;

    char comm[TASK_COMM_LEN];

    if (is_inside_k8s && !k8s_get_container_first_proc_comm(cg->id, comm)) {
        // container initialization may take some time when CPU % is high
        // seen on GKE: comm is '6' before 'runc:[2:INIT]' (dunno if it could be another number)
        if (is_digits_only(comm) || matches_entrypoint_parent_process_comm(comm)) {
            cg->first_time_seen = 1;
            return;
        }
        if (!strcmp(comm, "pause")) {
            // a container that holds the network namespace for the pod
            // we don't need to collect its metrics
            cg->processed = 1;
            return;
        }
    }

    if (cgroup_enable_systemd_services && matches_systemd_services_cgroups(cg->id)) {
        debug(D_CGROUP, "cgroup '%s' (name '%s') matches 'cgroups to match as systemd services'", cg->id, cg->chart_title);
        convert_cgroup_to_systemd_service(cg);
        return;
    }

    if (matches_enabled_cgroup_renames(cg->id)) {
        debug(D_CGROUP, "cgroup '%s' (name '%s') matches 'run script to rename cgroups matching', will try to rename it", cg->id, cg->chart_title);
        if (is_inside_k8s && k8s_is_container(cg->id)) {
            // it may take up to a minute for the K8s API to return data for the container
            // tested on AWS K8s cluster with 100% CPU utilization
            cg->pending_renames = 9; // 1.5 minute
        } else {
            cg->pending_renames = 2;
        }
    }
}

static int discovery_is_cgroup_duplicate(struct cgroup *cg) {
   // https://github.com/netdata/netdata/issues/797#issuecomment-241248884
   struct cgroup *c;
   for (c = discovered_cgroup_root; c; c = c->discovered_next) {
       if (c != cg && c->enabled && c->hash_chart == cg->hash_chart && !strcmp(c->chart_id, cg->chart_id)) {
           error("CGROUP: chart id '%s' already exists with id '%s' and is enabled and available. Disabling cgroup with id '%s'.", cg->chart_id, c->id, cg->id);
           return 1;
       }
   }
   return 0;
}

static inline void discovery_process_cgroup(struct cgroup *cg) {
    if (!cg) {
        debug(D_CGROUP, "discovery_process_cgroup() received NULL");
        return;
    }
    if (!cg->available || cg->processed) {
        return;
    }

    if (cg->first_time_seen) {
        worker_is_busy(WORKER_DISCOVERY_PROCESS_FIRST_TIME);
        discovery_process_first_time_seen_cgroup(cg);
        if (unlikely(cg->first_time_seen || cg->processed)) {
            return;
        }
    }

    if (cg->pending_renames) {
        worker_is_busy(WORKER_DISCOVERY_PROCESS_RENAME);
        discovery_rename_cgroup(cg);
        if (unlikely(cg->pending_renames || cg->processed)) {
            return;
        }
    }

    cg->processed = 1;

    if (is_cgroup_systemd_service(cg)) {
        cg->enabled = 1;
        return;
    }

    if (!(cg->enabled = matches_enabled_cgroup_names(cg->chart_title))) {
        debug(D_CGROUP, "cgroup '%s' (name '%s') disabled by 'enable by default cgroups names matching'", cg->id, cg->chart_title);
        return;
    }

    if (!(cg->enabled = matches_enabled_cgroup_paths(cg->id))) {
        debug(D_CGROUP, "cgroup '%s' (name '%s') disabled by 'enable by default cgroups matching'", cg->id, cg->chart_title);
        return;
    }

    if (discovery_is_cgroup_duplicate(cg)) {
        cg->enabled = 0;
        cg->options |= CGROUP_OPTIONS_DISABLED_DUPLICATE;
        return;
    }

    worker_is_busy(WORKER_DISCOVERY_PROCESS_NETWORK);
    read_cgroup_network_interfaces(cg);
}

static inline void discovery_find_all_cgroups() {
    debug(D_CGROUP, "searching for cgroups");

    worker_is_busy(WORKER_DISCOVERY_INIT);
    discovery_mark_all_cgroups_as_unavailable();

    worker_is_busy(WORKER_DISCOVERY_FIND);
    if (!cgroup_use_unified_cgroups) {
        discovery_find_all_cgroups_v1();
    } else {
        discovery_find_all_cgroups_v2();
    }

    struct cgroup *cg;
    for (cg = discovered_cgroup_root; cg; cg = cg->discovered_next) {
        worker_is_busy(WORKER_DISCOVERY_PROCESS);
        discovery_process_cgroup(cg);
    }

    worker_is_busy(WORKER_DISCOVERY_UPDATE);
    discovery_update_filenames();

    worker_is_busy(WORKER_DISCOVERY_LOCK);
    uv_mutex_lock(&cgroup_root_mutex);

    worker_is_busy(WORKER_DISCOVERY_CLEANUP);
    discovery_cleanup_all_cgroups();

    worker_is_busy(WORKER_DISCOVERY_COPY);
    discovery_copy_discovered_cgroups_to_reader();

    uv_mutex_unlock(&cgroup_root_mutex);

    worker_is_busy(WORKER_DISCOVERY_SHARE);
    discovery_share_cgroups_with_ebpf();

    debug(D_CGROUP, "done searching for cgroups");
}

void cgroup_discovery_worker(void *ptr)
{
    UNUSED(ptr);

    worker_register("CGROUPSDISC");
    worker_register_job_name(WORKER_DISCOVERY_INIT,               "init");
    worker_register_job_name(WORKER_DISCOVERY_FIND,               "find");
    worker_register_job_name(WORKER_DISCOVERY_PROCESS,            "process");
    worker_register_job_name(WORKER_DISCOVERY_PROCESS_RENAME,     "rename");
    worker_register_job_name(WORKER_DISCOVERY_PROCESS_NETWORK,    "network");
    worker_register_job_name(WORKER_DISCOVERY_PROCESS_FIRST_TIME, "new");
    worker_register_job_name(WORKER_DISCOVERY_UPDATE,             "update");
    worker_register_job_name(WORKER_DISCOVERY_CLEANUP,            "cleanup");
    worker_register_job_name(WORKER_DISCOVERY_COPY,               "copy");
    worker_register_job_name(WORKER_DISCOVERY_SHARE,              "share");
    worker_register_job_name(WORKER_DISCOVERY_LOCK,               "lock");

    entrypoint_parent_process_comm = simple_pattern_create(
        " runc:[* " // http://terenceli.github.io/%E6%8A%80%E6%9C%AF/2021/12/28/runc-internals-3)
        " exe ", // https://github.com/falcosecurity/falco/blob/9d41b0a151b83693929d3a9c84f7c5c85d070d3a/rules/falco_rules.yaml#L1961
        NULL,
        SIMPLE_PATTERN_EXACT);

    while (!netdata_exit) {
        worker_is_idle();

        uv_mutex_lock(&discovery_thread.mutex);
        while (!discovery_thread.start_discovery)
            uv_cond_wait(&discovery_thread.cond_var, &discovery_thread.mutex);
        discovery_thread.start_discovery = 0;
        uv_mutex_unlock(&discovery_thread.mutex);

        if (unlikely(netdata_exit))
            break;

        discovery_find_all_cgroups();
    }

    discovery_thread.exited = 1;
    worker_unregister();
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
            snprintfz(title, CHART_TITLE_MAX, "Systemd Services CPU utilization (100%% = 1 core)");

            st_cpu = rrdset_create_localhost(
                    "services"
                    , "cpu"
                    , NULL
                    , "cpu"
                    , "services.cpu"
                    , title
                    , "percentage"
                    , PLUGIN_CGROUPS_NAME
                    , PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME
                    , NETDATA_CHART_PRIO_CGROUPS_SYSTEMD
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
                    , "Systemd Services Used Memory"
                    , "MiB"
                    , PLUGIN_CGROUPS_NAME
                    , PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME
                    , NETDATA_CHART_PRIO_CGROUPS_SYSTEMD + 10
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
                    , "MiB"
                    , PLUGIN_CGROUPS_NAME
                    , PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME
                    , NETDATA_CHART_PRIO_CGROUPS_SYSTEMD + 20
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
                    , "MiB"
                    , PLUGIN_CGROUPS_NAME
                    , PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME
                    , NETDATA_CHART_PRIO_CGROUPS_SYSTEMD + 30
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
                    , "MiB"
                    , PLUGIN_CGROUPS_NAME
                    , PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME
                    , NETDATA_CHART_PRIO_CGROUPS_SYSTEMD + 40
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
                    , "MiB"
                    , PLUGIN_CGROUPS_NAME
                    , PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME
                    , NETDATA_CHART_PRIO_CGROUPS_SYSTEMD + 50
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
                    , "MiB/s"
                    , PLUGIN_CGROUPS_NAME
                    , PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME
                    , NETDATA_CHART_PRIO_CGROUPS_SYSTEMD + 60
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
                    , "MiB/s"
                    , PLUGIN_CGROUPS_NAME
                    , PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME
                    , NETDATA_CHART_PRIO_CGROUPS_SYSTEMD + 70
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
                    , "MiB/s"
                    , PLUGIN_CGROUPS_NAME
                    , PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME
                    , NETDATA_CHART_PRIO_CGROUPS_SYSTEMD + 80
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
                    , "MiB/s"
                    , PLUGIN_CGROUPS_NAME
                    , PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME
                    , NETDATA_CHART_PRIO_CGROUPS_SYSTEMD + 90
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
                    , "failures"
                    , PLUGIN_CGROUPS_NAME
                    , PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME
                    , NETDATA_CHART_PRIO_CGROUPS_SYSTEMD + 110
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
                    , "MiB"
                    , PLUGIN_CGROUPS_NAME
                    , PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME
                    , NETDATA_CHART_PRIO_CGROUPS_SYSTEMD + 100
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
                    , "KiB/s"
                    , PLUGIN_CGROUPS_NAME
                    , PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME
                    , NETDATA_CHART_PRIO_CGROUPS_SYSTEMD + 120
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
                    , "KiB/s"
                    , PLUGIN_CGROUPS_NAME
                    , PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME
                    , NETDATA_CHART_PRIO_CGROUPS_SYSTEMD + 130
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
                    , PLUGIN_CGROUPS_NAME
                    , PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME
                    , NETDATA_CHART_PRIO_CGROUPS_SYSTEMD + 140
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
                    , PLUGIN_CGROUPS_NAME
                    , PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME
                    , NETDATA_CHART_PRIO_CGROUPS_SYSTEMD + 150
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
                    , "KiB/s"
                    , PLUGIN_CGROUPS_NAME
                    , PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME
                    , NETDATA_CHART_PRIO_CGROUPS_SYSTEMD + 160
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
                    , "KiB/s"
                    , PLUGIN_CGROUPS_NAME
                    , PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME
                    , NETDATA_CHART_PRIO_CGROUPS_SYSTEMD + 170
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
                    , PLUGIN_CGROUPS_NAME
                    , PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME
                    , NETDATA_CHART_PRIO_CGROUPS_SYSTEMD + 180
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
                    , PLUGIN_CGROUPS_NAME
                    , PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME
                    , NETDATA_CHART_PRIO_CGROUPS_SYSTEMD + 190
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
                    , PLUGIN_CGROUPS_NAME
                    , PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME
                    , NETDATA_CHART_PRIO_CGROUPS_SYSTEMD + 200
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
                    , PLUGIN_CGROUPS_NAME
                    , PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME
                    , NETDATA_CHART_PRIO_CGROUPS_SYSTEMD + 210
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
                    , PLUGIN_CGROUPS_NAME
                    , PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME
                    , NETDATA_CHART_PRIO_CGROUPS_SYSTEMD + 220
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
                    , PLUGIN_CGROUPS_NAME
                    , PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME
                    , NETDATA_CHART_PRIO_CGROUPS_SYSTEMD + 230
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
        if(unlikely(!cg->enabled || cg->pending_renames || !is_cgroup_systemd_service(cg)))
            continue;

        if(likely(do_cpu && cg->cpuacct_stat.updated)) {
            if(unlikely(!cg->rd_cpu)){


                if (!(cg->options & CGROUP_OPTIONS_IS_UNIFIED)) {
                    cg->rd_cpu = rrddim_add(st_cpu, cg->chart_id, cg->chart_title, 100, system_hz, RRD_ALGORITHM_INCREMENTAL);
                } else {
                    cg->rd_cpu = rrddim_add(st_cpu, cg->chart_id, cg->chart_title, 100, 1000000, RRD_ALGORITHM_INCREMENTAL);
                }
            }

            rrddim_set_by_pointer(st_cpu, cg->rd_cpu, cg->cpuacct_stat.user + cg->cpuacct_stat.system);
        }

        if(likely(do_mem_usage && cg->memory.updated_usage_in_bytes)) {
            if(unlikely(!cg->rd_mem_usage))
                cg->rd_mem_usage = rrddim_add(st_mem_usage, cg->chart_id, cg->chart_title, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);

            rrddim_set_by_pointer(st_mem_usage, cg->rd_mem_usage, cg->memory.usage_in_bytes);
        }

        if(likely(do_mem_detailed && cg->memory.updated_detailed)) {
            if(unlikely(!cg->rd_mem_detailed_rss))
                cg->rd_mem_detailed_rss = rrddim_add(st_mem_detailed_rss, cg->chart_id, cg->chart_title, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);

            rrddim_set_by_pointer(st_mem_detailed_rss, cg->rd_mem_detailed_rss, cg->memory.total_rss);

            if(unlikely(!cg->rd_mem_detailed_mapped))
                cg->rd_mem_detailed_mapped = rrddim_add(st_mem_detailed_mapped, cg->chart_id, cg->chart_title, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);

            rrddim_set_by_pointer(st_mem_detailed_mapped, cg->rd_mem_detailed_mapped, cg->memory.total_mapped_file);

            if(unlikely(!cg->rd_mem_detailed_cache))
                cg->rd_mem_detailed_cache = rrddim_add(st_mem_detailed_cache, cg->chart_id, cg->chart_title, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);

            rrddim_set_by_pointer(st_mem_detailed_cache, cg->rd_mem_detailed_cache, cg->memory.total_cache);

            if(unlikely(!cg->rd_mem_detailed_writeback))
                cg->rd_mem_detailed_writeback = rrddim_add(st_mem_detailed_writeback, cg->chart_id, cg->chart_title, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);

            rrddim_set_by_pointer(st_mem_detailed_writeback, cg->rd_mem_detailed_writeback, cg->memory.total_writeback);

            if(unlikely(!cg->rd_mem_detailed_pgfault))
                cg->rd_mem_detailed_pgfault = rrddim_add(st_mem_detailed_pgfault, cg->chart_id, cg->chart_title, system_page_size, 1024 * 1024, RRD_ALGORITHM_INCREMENTAL);

            rrddim_set_by_pointer(st_mem_detailed_pgfault, cg->rd_mem_detailed_pgfault, cg->memory.total_pgfault);

            if(unlikely(!cg->rd_mem_detailed_pgmajfault))
                cg->rd_mem_detailed_pgmajfault = rrddim_add(st_mem_detailed_pgmajfault, cg->chart_id, cg->chart_title, system_page_size, 1024 * 1024, RRD_ALGORITHM_INCREMENTAL);

            rrddim_set_by_pointer(st_mem_detailed_pgmajfault, cg->rd_mem_detailed_pgmajfault, cg->memory.total_pgmajfault);

            if(unlikely(!cg->rd_mem_detailed_pgpgin))
                cg->rd_mem_detailed_pgpgin = rrddim_add(st_mem_detailed_pgpgin, cg->chart_id, cg->chart_title, system_page_size, 1024 * 1024, RRD_ALGORITHM_INCREMENTAL);

            rrddim_set_by_pointer(st_mem_detailed_pgpgin, cg->rd_mem_detailed_pgpgin, cg->memory.total_pgpgin);

            if(unlikely(!cg->rd_mem_detailed_pgpgout))
                cg->rd_mem_detailed_pgpgout = rrddim_add(st_mem_detailed_pgpgout, cg->chart_id, cg->chart_title, system_page_size, 1024 * 1024, RRD_ALGORITHM_INCREMENTAL);

            rrddim_set_by_pointer(st_mem_detailed_pgpgout, cg->rd_mem_detailed_pgpgout, cg->memory.total_pgpgout);
        }

        if(likely(do_mem_failcnt && cg->memory.updated_failcnt)) {
            if(unlikely(!cg->rd_mem_failcnt))
                cg->rd_mem_failcnt = rrddim_add(st_mem_failcnt, cg->chart_id, cg->chart_title, 1, 1, RRD_ALGORITHM_INCREMENTAL);

            rrddim_set_by_pointer(st_mem_failcnt, cg->rd_mem_failcnt, cg->memory.failcnt);
        }

        if(likely(do_swap_usage && cg->memory.updated_msw_usage_in_bytes)) {
            if(unlikely(!cg->rd_swap_usage))
                cg->rd_swap_usage = rrddim_add(st_swap_usage, cg->chart_id, cg->chart_title, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);

            if(!(cg->options & CGROUP_OPTIONS_IS_UNIFIED)) {
                rrddim_set_by_pointer(
                    st_swap_usage,
                    cg->rd_swap_usage,
                    cg->memory.msw_usage_in_bytes > (cg->memory.usage_in_bytes + cg->memory.total_inactive_file) ?
                        cg->memory.msw_usage_in_bytes - (cg->memory.usage_in_bytes + cg->memory.total_inactive_file) : 0);
            } else {
                rrddim_set_by_pointer(st_swap_usage, cg->rd_swap_usage, cg->memory.msw_usage_in_bytes);
            }
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

static inline unsigned long long cpuset_str2ull(char **s) {
    unsigned long long n = 0;
    char c;
    for(c = **s; c >= '0' && c <= '9' ; c = *(++*s)) {
        n *= 10;
        n += c - '0';
    }
    return n;
}

static inline void update_cpu_limits(char **filename, unsigned long long *value, struct cgroup *cg) {
    if(*filename) {
        int ret = -1;

        if(value == &cg->cpuset_cpus) {
            static char *buf = NULL;
            static size_t buf_size = 0;

            if(!buf) {
                buf_size = 100U + 6 * get_system_cpus(); // taken from kernel/cgroup/cpuset.c
                buf = mallocz(buf_size + 1);
            }

            ret = read_file(*filename, buf, buf_size);

            if(!ret) {
                char *s = buf;
                unsigned long long ncpus = 0;

                // parse the cpuset string and calculate the number of cpus the cgroup is allowed to use
                while(*s) {
                    unsigned long long n = cpuset_str2ull(&s);
                    ncpus++;
                    if(*s == ',') {
                        s++;
                        continue;
                    }
                    if(*s == '-') {
                        s++;
                        unsigned long long m = cpuset_str2ull(&s);
                        ncpus += m - n; // calculate the number of cpus in the region
                    }
                    s++;
                }

                if(likely(ncpus)) *value = ncpus;
            }
        }
        else if(value == &cg->cpu_cfs_period) {
            ret = read_single_number_file(*filename, value);
        }
        else if(value == &cg->cpu_cfs_quota) {
            ret = read_single_number_file(*filename, value);
        }
        else ret = -1;

        if(ret) {
            error("Cannot refresh cgroup %s cpu limit by reading '%s'. Will not update its limit anymore.", cg->id, *filename);
            freez(*filename);
            *filename = NULL;
        }
    }
}

static inline void update_cpu_limits2(struct cgroup *cg) {
    if(cg->filename_cpu_cfs_quota){
        static procfile *ff = NULL;

        ff = procfile_reopen(ff, cg->filename_cpu_cfs_quota, NULL, PROCFILE_FLAG_DEFAULT);
        if(unlikely(!ff)) {
            goto cpu_limits2_err;
        }

        ff = procfile_readall(ff);
        if(unlikely(!ff)) {
            goto cpu_limits2_err;
        }

        unsigned long lines = procfile_lines(ff);

        if (unlikely(lines < 1)) {
            error("CGROUP: file '%s' should have 1 lines.", cg->filename_cpu_cfs_quota);
            return;
        }

        cg->cpu_cfs_period = str2ull(procfile_lineword(ff, 0, 1));
        cg->cpuset_cpus = get_system_cpus();

        char *s = "max\n\0";
        if(strsame(s, procfile_lineword(ff, 0, 0)) == 0){
            cg->cpu_cfs_quota = cg->cpu_cfs_period * cg->cpuset_cpus;
        } else {
            cg->cpu_cfs_quota = str2ull(procfile_lineword(ff, 0, 0));
        }
        debug(D_CGROUP, "CPU limits values: %llu %llu %llu", cg->cpu_cfs_period, cg->cpuset_cpus, cg->cpu_cfs_quota);
        return;

cpu_limits2_err:
        error("Cannot refresh cgroup %s cpu limit by reading '%s'. Will not update its limit anymore.", cg->id, cg->filename_cpu_cfs_quota);
        freez(cg->filename_cpu_cfs_quota);
        cg->filename_cpu_cfs_quota = NULL;

    }
}

static inline int update_memory_limits(char **filename, RRDSETVAR **chart_var, unsigned long long *value, const char *chart_var_name, struct cgroup *cg) {
    if(*filename) {
        if(unlikely(!*chart_var)) {
            *chart_var = rrdsetvar_custom_chart_variable_create(cg->st_mem_usage, chart_var_name);
            if(!*chart_var) {
                error("Cannot create cgroup %s chart variable '%s'. Will not update its limit anymore.", cg->id, chart_var_name);
                freez(*filename);
                *filename = NULL;
            }
        }

        if(*filename && *chart_var) {
            if(!(cg->options & CGROUP_OPTIONS_IS_UNIFIED)) {
                if(read_single_number_file(*filename, value)) {
                    error("Cannot refresh cgroup %s memory limit by reading '%s'. Will not update its limit anymore.", cg->id, *filename);
                    freez(*filename);
                    *filename = NULL;
                }
                else {
                    rrdsetvar_custom_chart_variable_set(*chart_var, (calculated_number)(*value / (1024 * 1024)));
                    return 1;
                }
            } else {
                char buffer[30 + 1];
                int ret = read_file(*filename, buffer, 30);
                if(ret) {
                    error("Cannot refresh cgroup %s memory limit by reading '%s'. Will not update its limit anymore.", cg->id, *filename);
                    freez(*filename);
                    *filename = NULL;
                    return 0;
                }
                char *s = "max\n\0";
                if(strsame(s, buffer) == 0){
                    *value = UINT64_MAX;
                    rrdsetvar_custom_chart_variable_set(*chart_var, (calculated_number)(*value / (1024 * 1024)));
                    return 1;
                }
                *value = str2ull(buffer);
                rrdsetvar_custom_chart_variable_set(*chart_var, (calculated_number)(*value / (1024 * 1024)));
                return 1;
            }
        }
    }
    return 0;
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
        if(unlikely(!cg->enabled || cg->pending_renames))
            continue;

        if(likely(cgroup_enable_systemd_services && is_cgroup_systemd_service(cg))) {
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
                snprintfz(title, CHART_TITLE_MAX, "CPU Usage (100%% = 1 core)");

                cg->st_cpu = rrdset_create_localhost(
                        cgroup_chart_type(type, cg->chart_id, RRD_ID_LENGTH_MAX)
                        , "cpu"
                        , NULL
                        , "cpu"
                        , "cgroup.cpu"
                        , title
                        , "percentage"
                        , PLUGIN_CGROUPS_NAME
                        , PLUGIN_CGROUPS_MODULE_CGROUPS_NAME
                        , cgroup_containers_chart_priority
                        , update_every
                        , RRDSET_TYPE_STACKED
                );

                rrdset_update_labels(cg->st_cpu, cg->chart_labels);

                if(!(cg->options & CGROUP_OPTIONS_IS_UNIFIED)) {
                    rrddim_add(cg->st_cpu, "user", NULL, 100, system_hz, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(cg->st_cpu, "system", NULL, 100, system_hz, RRD_ALGORITHM_INCREMENTAL);
                }
                else {
                    rrddim_add(cg->st_cpu, "user", NULL, 100, 1000000, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(cg->st_cpu, "system", NULL, 100, 1000000, RRD_ALGORITHM_INCREMENTAL);
                }
            }
            else
                rrdset_next(cg->st_cpu);

            rrddim_set(cg->st_cpu, "user", cg->cpuacct_stat.user);
            rrddim_set(cg->st_cpu, "system", cg->cpuacct_stat.system);
            rrdset_done(cg->st_cpu);

            if(likely(cg->filename_cpuset_cpus || cg->filename_cpu_cfs_period || cg->filename_cpu_cfs_quota)) {
                if(!(cg->options & CGROUP_OPTIONS_IS_UNIFIED)) {
                    update_cpu_limits(&cg->filename_cpuset_cpus, &cg->cpuset_cpus, cg);
                    update_cpu_limits(&cg->filename_cpu_cfs_period, &cg->cpu_cfs_period, cg);
                    update_cpu_limits(&cg->filename_cpu_cfs_quota, &cg->cpu_cfs_quota, cg);
                } else {
                    update_cpu_limits2(cg);
                }

                if(unlikely(!cg->chart_var_cpu_limit)) {
                    cg->chart_var_cpu_limit = rrdsetvar_custom_chart_variable_create(cg->st_cpu, "cpu_limit");
                    if(!cg->chart_var_cpu_limit) {
                        error("Cannot create cgroup %s chart variable 'cpu_limit'. Will not update its limit anymore.", cg->id);
                        if(cg->filename_cpuset_cpus) freez(cg->filename_cpuset_cpus);
                        cg->filename_cpuset_cpus = NULL;
                        if(cg->filename_cpu_cfs_period) freez(cg->filename_cpu_cfs_period);
                        cg->filename_cpu_cfs_period = NULL;
                        if(cg->filename_cpu_cfs_quota) freez(cg->filename_cpu_cfs_quota);
                        cg->filename_cpu_cfs_quota = NULL;
                    }
                }
                else {
                    calculated_number value = 0, quota = 0;

                    if(likely( ((!(cg->options & CGROUP_OPTIONS_IS_UNIFIED)) && (cg->filename_cpuset_cpus || (cg->filename_cpu_cfs_period && cg->filename_cpu_cfs_quota)))
                            || ((cg->options & CGROUP_OPTIONS_IS_UNIFIED) && cg->filename_cpu_cfs_quota))) {
                        if(unlikely(cg->cpu_cfs_quota > 0))
                            quota = (calculated_number)cg->cpu_cfs_quota / (calculated_number)cg->cpu_cfs_period;

                        if(unlikely(quota > 0 && quota < cg->cpuset_cpus))
                            value = quota * 100;
                        else
                            value = (calculated_number)cg->cpuset_cpus * 100;
                    }
                    if(likely(value)) {
                        rrdsetvar_custom_chart_variable_set(cg->chart_var_cpu_limit, value);

                        if(unlikely(!cg->st_cpu_limit)) {
                            snprintfz(title, CHART_TITLE_MAX, "CPU Usage within the limits");

                            cg->st_cpu_limit = rrdset_create_localhost(
                                    cgroup_chart_type(type, cg->chart_id, RRD_ID_LENGTH_MAX)
                                    , "cpu_limit"
                                    , NULL
                                    , "cpu"
                                    , "cgroup.cpu_limit"
                                    , title
                                    , "percentage"
                                    , PLUGIN_CGROUPS_NAME
                                    , PLUGIN_CGROUPS_MODULE_CGROUPS_NAME
                                    , cgroup_containers_chart_priority - 1
                                    , update_every
                                    , RRDSET_TYPE_LINE
                            );

                            rrdset_update_labels(cg->st_cpu_limit, cg->chart_labels);

                            if(!(cg->options & CGROUP_OPTIONS_IS_UNIFIED))
                                rrddim_add(cg->st_cpu_limit, "used", NULL, 1, system_hz, RRD_ALGORITHM_ABSOLUTE);
                            else
                                rrddim_add(cg->st_cpu_limit, "used", NULL, 1, 1000000, RRD_ALGORITHM_ABSOLUTE);
                            cg->prev_cpu_usage = (calculated_number)(cg->cpuacct_stat.user + cg->cpuacct_stat.system) * 100;
                        }
                        else
                            rrdset_next(cg->st_cpu_limit);

                        calculated_number cpu_usage = 0;
                        cpu_usage = (calculated_number)(cg->cpuacct_stat.user + cg->cpuacct_stat.system) * 100;
                        calculated_number cpu_used = 100 * (cpu_usage - cg->prev_cpu_usage) / (value * update_every);

                        rrdset_isnot_obsolete(cg->st_cpu_limit);

                        rrddim_set(cg->st_cpu_limit, "used", (cpu_used > 0)?cpu_used:0);

                        cg->prev_cpu_usage = cpu_usage;

                        rrdset_done(cg->st_cpu_limit);
                    }
                    else {
                        rrdsetvar_custom_chart_variable_set(cg->chart_var_cpu_limit, NAN);
                        if(unlikely(cg->st_cpu_limit)) {
                            rrdset_is_obsolete(cg->st_cpu_limit);
                            cg->st_cpu_limit = NULL;
                        }
                    }
                }
            }
        }

        if (likely(cg->cpuacct_cpu_throttling.updated && cg->cpuacct_cpu_throttling.enabled == CONFIG_BOOLEAN_YES)) {
            if (unlikely(!cg->st_cpu_nr_throttled)) {
                snprintfz(title, CHART_TITLE_MAX, "CPU Throttled Runnable Periods");

                cg->st_cpu_nr_throttled = rrdset_create_localhost(
                        cgroup_chart_type(type, cg->chart_id, RRD_ID_LENGTH_MAX)
                        , "throttled"
                        , NULL
                        , "cpu"
                        , "cgroup.throttled"
                        , title
                        , "percentage"
                        , PLUGIN_CGROUPS_NAME
                        , PLUGIN_CGROUPS_MODULE_CGROUPS_NAME
                        , cgroup_containers_chart_priority + 10
                        , update_every
                        , RRDSET_TYPE_LINE
                );

                rrdset_update_labels(cg->st_cpu_nr_throttled, cg->chart_labels);
                rrddim_add(cg->st_cpu_nr_throttled, "throttled", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            } else {
                rrdset_next(cg->st_cpu_nr_throttled);
                rrddim_set(cg->st_cpu_nr_throttled, "throttled", cg->cpuacct_cpu_throttling.nr_throttled_perc);
                rrdset_done(cg->st_cpu_nr_throttled);
            }

            if (unlikely(!cg->st_cpu_throttled_time)) {
                snprintfz(title, CHART_TITLE_MAX, "CPU Throttled Time Duration");

                cg->st_cpu_throttled_time = rrdset_create_localhost(
                        cgroup_chart_type(type, cg->chart_id, RRD_ID_LENGTH_MAX)
                        , "throttled_duration"
                        , NULL
                        , "cpu"
                        , "cgroup.throttled_duration"
                        , title
                        , "ms"
                        , PLUGIN_CGROUPS_NAME
                        , PLUGIN_CGROUPS_MODULE_CGROUPS_NAME
                        , cgroup_containers_chart_priority + 15
                        , update_every
                        , RRDSET_TYPE_LINE
                );

                rrdset_update_labels(cg->st_cpu_throttled_time, cg->chart_labels);
                rrddim_add(cg->st_cpu_throttled_time, "duration", NULL, 1, 1000000, RRD_ALGORITHM_INCREMENTAL);
            } else {
                rrdset_next(cg->st_cpu_throttled_time);
                rrddim_set(cg->st_cpu_throttled_time, "duration", cg->cpuacct_cpu_throttling.throttled_time);
                rrdset_done(cg->st_cpu_throttled_time);
            }
        }

        if (likely(cg->cpuacct_cpu_shares.updated && cg->cpuacct_cpu_shares.enabled == CONFIG_BOOLEAN_YES)) {
            if (unlikely(!cg->st_cpu_shares)) {
                snprintfz(title, CHART_TITLE_MAX, "CPU Time Relative Share");

                cg->st_cpu_shares = rrdset_create_localhost(
                        cgroup_chart_type(type, cg->chart_id, RRD_ID_LENGTH_MAX)
                        , "cpu_shares"
                        , NULL
                        , "cpu"
                        , "cgroup.cpu_shares"
                        , title
                        , "shares"
                        , PLUGIN_CGROUPS_NAME
                        , PLUGIN_CGROUPS_MODULE_CGROUPS_NAME
                        , cgroup_containers_chart_priority + 20
                        , update_every
                        , RRDSET_TYPE_LINE
                );

                rrdset_update_labels(cg->st_cpu_shares, cg->chart_labels);
                rrddim_add(cg->st_cpu_shares, "shares", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            } else {
                rrdset_next(cg->st_cpu_shares);
                rrddim_set(cg->st_cpu_shares, "shares", cg->cpuacct_cpu_shares.shares);
                rrdset_done(cg->st_cpu_shares);
            }
        }

        if(likely(cg->cpuacct_usage.updated && cg->cpuacct_usage.enabled == CONFIG_BOOLEAN_YES)) {
            char id[RRD_ID_LENGTH_MAX + 1];
            unsigned int i;

            if(unlikely(!cg->st_cpu_per_core)) {
                snprintfz(title, CHART_TITLE_MAX, "CPU Usage (100%% = 1 core) Per Core");

                cg->st_cpu_per_core = rrdset_create_localhost(
                        cgroup_chart_type(type, cg->chart_id, RRD_ID_LENGTH_MAX)
                        , "cpu_per_core"
                        , NULL
                        , "cpu"
                        , "cgroup.cpu_per_core"
                        , title
                        , "percentage"
                        , PLUGIN_CGROUPS_NAME
                        , PLUGIN_CGROUPS_MODULE_CGROUPS_NAME
                        , cgroup_containers_chart_priority + 100
                        , update_every
                        , RRDSET_TYPE_STACKED
                );

                rrdset_update_labels(cg->st_cpu_per_core, cg->chart_labels);

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
                snprintfz(title, CHART_TITLE_MAX, "Memory Usage");

                cg->st_mem = rrdset_create_localhost(
                        cgroup_chart_type(type, cg->chart_id, RRD_ID_LENGTH_MAX)
                        , "mem"
                        , NULL
                        , "mem"
                        , "cgroup.mem"
                        , title
                        , "MiB"
                        , PLUGIN_CGROUPS_NAME
                        , PLUGIN_CGROUPS_MODULE_CGROUPS_NAME
                        , cgroup_containers_chart_priority + 220
                        , update_every
                        , RRDSET_TYPE_STACKED
                );
                
                rrdset_update_labels(cg->st_mem, cg->chart_labels);

                if(!(cg->options & CGROUP_OPTIONS_IS_UNIFIED)) {
                    rrddim_add(cg->st_mem, "cache", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
                    rrddim_add(cg->st_mem, "rss", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);

                    if(cg->memory.detailed_has_swap)
                        rrddim_add(cg->st_mem, "swap", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);

                    rrddim_add(cg->st_mem, "rss_huge", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
                    rrddim_add(cg->st_mem, "mapped_file", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
                } else {
                    rrddim_add(cg->st_mem, "anon", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
                    rrddim_add(cg->st_mem, "kernel_stack", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
                    rrddim_add(cg->st_mem, "slab", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
                    rrddim_add(cg->st_mem, "sock", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
                    rrddim_add(cg->st_mem, "anon_thp", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
                    rrddim_add(cg->st_mem, "file", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
                }
            }
            else
                rrdset_next(cg->st_mem);

            if(!(cg->options & CGROUP_OPTIONS_IS_UNIFIED)) {
                rrddim_set(cg->st_mem, "cache", cg->memory.total_cache);
                rrddim_set(cg->st_mem, "rss", (cg->memory.total_rss > cg->memory.total_rss_huge)?(cg->memory.total_rss - cg->memory.total_rss_huge):0);

                if(cg->memory.detailed_has_swap)
                    rrddim_set(cg->st_mem, "swap", cg->memory.total_swap);

                rrddim_set(cg->st_mem, "rss_huge", cg->memory.total_rss_huge);
                rrddim_set(cg->st_mem, "mapped_file", cg->memory.total_mapped_file);
            } else {
                rrddim_set(cg->st_mem, "anon", cg->memory.anon);
                rrddim_set(cg->st_mem, "kernel_stack", cg->memory.kernel_stack);
                rrddim_set(cg->st_mem, "slab", cg->memory.slab);
                rrddim_set(cg->st_mem, "sock", cg->memory.sock);
                rrddim_set(cg->st_mem, "anon_thp", cg->memory.anon_thp);
                rrddim_set(cg->st_mem, "file", cg->memory.total_mapped_file);
            }
            rrdset_done(cg->st_mem);

            if(unlikely(!cg->st_writeback)) {
                snprintfz(title, CHART_TITLE_MAX, "Writeback Memory");

                cg->st_writeback = rrdset_create_localhost(
                        cgroup_chart_type(type, cg->chart_id, RRD_ID_LENGTH_MAX)
                        , "writeback"
                        , NULL
                        , "mem"
                        , "cgroup.writeback"
                        , title
                        , "MiB"
                        , PLUGIN_CGROUPS_NAME
                        , PLUGIN_CGROUPS_MODULE_CGROUPS_NAME
                        , cgroup_containers_chart_priority + 300
                        , update_every
                        , RRDSET_TYPE_AREA
                );

                rrdset_update_labels(cg->st_writeback, cg->chart_labels);

                if(cg->memory.detailed_has_dirty)
                    rrddim_add(cg->st_writeback, "dirty", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);

                rrddim_add(cg->st_writeback, "writeback", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
            }
            else
                rrdset_next(cg->st_writeback);

            if(cg->memory.detailed_has_dirty)
                rrddim_set(cg->st_writeback, "dirty", cg->memory.total_dirty);

            rrddim_set(cg->st_writeback, "writeback", cg->memory.total_writeback);
            rrdset_done(cg->st_writeback);

            if(!(cg->options & CGROUP_OPTIONS_IS_UNIFIED)) {
                if(unlikely(!cg->st_mem_activity)) {
                    snprintfz(title, CHART_TITLE_MAX, "Memory Activity");

                    cg->st_mem_activity = rrdset_create_localhost(
                            cgroup_chart_type(type, cg->chart_id, RRD_ID_LENGTH_MAX)
                            , "mem_activity"
                            , NULL
                            , "mem"
                            , "cgroup.mem_activity"
                            , title
                            , "MiB/s"
                            , PLUGIN_CGROUPS_NAME
                            , PLUGIN_CGROUPS_MODULE_CGROUPS_NAME
                            , cgroup_containers_chart_priority + 400
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    rrdset_update_labels(cg->st_mem_activity, cg->chart_labels);

                    rrddim_add(cg->st_mem_activity, "pgpgin", "in", system_page_size, 1024 * 1024, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(cg->st_mem_activity, "pgpgout", "out", -system_page_size, 1024 * 1024, RRD_ALGORITHM_INCREMENTAL);
                }
                else
                    rrdset_next(cg->st_mem_activity);

                rrddim_set(cg->st_mem_activity, "pgpgin", cg->memory.total_pgpgin);
                rrddim_set(cg->st_mem_activity, "pgpgout", cg->memory.total_pgpgout);
                rrdset_done(cg->st_mem_activity);
            }

            if(unlikely(!cg->st_pgfaults)) {
                snprintfz(title, CHART_TITLE_MAX, "Memory Page Faults");

                cg->st_pgfaults = rrdset_create_localhost(
                        cgroup_chart_type(type, cg->chart_id, RRD_ID_LENGTH_MAX)
                        , "pgfaults"
                        , NULL
                        , "mem"
                        , "cgroup.pgfaults"
                        , title
                        , "MiB/s"
                        , PLUGIN_CGROUPS_NAME
                        , PLUGIN_CGROUPS_MODULE_CGROUPS_NAME
                        , cgroup_containers_chart_priority + 500
                        , update_every
                        , RRDSET_TYPE_LINE
                );

                rrdset_update_labels(cg->st_pgfaults, cg->chart_labels);

                rrddim_add(cg->st_pgfaults, "pgfault", NULL, system_page_size, 1024 * 1024, RRD_ALGORITHM_INCREMENTAL);
                rrddim_add(cg->st_pgfaults, "pgmajfault", "swap", -system_page_size, 1024 * 1024, RRD_ALGORITHM_INCREMENTAL);
            }
            else
                rrdset_next(cg->st_pgfaults);

            rrddim_set(cg->st_pgfaults, "pgfault", cg->memory.total_pgfault);
            rrddim_set(cg->st_pgfaults, "pgmajfault", cg->memory.total_pgmajfault);
            rrdset_done(cg->st_pgfaults);
        }

        if(likely(cg->memory.updated_usage_in_bytes && cg->memory.enabled_usage_in_bytes == CONFIG_BOOLEAN_YES)) {
            if(unlikely(!cg->st_mem_usage)) {
                snprintfz(title, CHART_TITLE_MAX, "Used Memory");

                cg->st_mem_usage = rrdset_create_localhost(
                        cgroup_chart_type(type, cg->chart_id, RRD_ID_LENGTH_MAX)
                        , "mem_usage"
                        , NULL
                        , "mem"
                        , "cgroup.mem_usage"
                        , title
                        , "MiB"
                        , PLUGIN_CGROUPS_NAME
                        , PLUGIN_CGROUPS_MODULE_CGROUPS_NAME
                        , cgroup_containers_chart_priority + 210
                        , update_every
                        , RRDSET_TYPE_STACKED
                );

                rrdset_update_labels(cg->st_mem_usage, cg->chart_labels);

                rrddim_add(cg->st_mem_usage, "ram", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
                rrddim_add(cg->st_mem_usage, "swap", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
            }
            else
                rrdset_next(cg->st_mem_usage);

            rrddim_set(cg->st_mem_usage, "ram", cg->memory.usage_in_bytes);
            if(!(cg->options & CGROUP_OPTIONS_IS_UNIFIED)) {
                rrddim_set(
                    cg->st_mem_usage,
                    "swap",
                    cg->memory.msw_usage_in_bytes > (cg->memory.usage_in_bytes + cg->memory.total_inactive_file) ?
                        cg->memory.msw_usage_in_bytes - (cg->memory.usage_in_bytes + cg->memory.total_inactive_file) : 0);
            } else {
                rrddim_set(cg->st_mem_usage, "swap", cg->memory.msw_usage_in_bytes);
            }
            rrdset_done(cg->st_mem_usage);

            if (likely(update_memory_limits(&cg->filename_memory_limit, &cg->chart_var_memory_limit, &cg->memory_limit, "memory_limit", cg))) {
                static unsigned long long ram_total = 0;

                if(unlikely(!ram_total)) {
                    procfile *ff = NULL;

                    char filename[FILENAME_MAX + 1];
                    snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/proc/meminfo");
                    ff = procfile_open(config_get("plugin:cgroups", "meminfo filename to monitor", filename), " \t:", PROCFILE_FLAG_DEFAULT);

                    if(likely(ff))
                        ff = procfile_readall(ff);
                    if(likely(ff && procfile_lines(ff) && !strncmp(procfile_word(ff, 0), "MemTotal", 8)))
                        ram_total = str2ull(procfile_word(ff, 1)) * 1024;
                    else {
                        error("Cannot read file %s. Will not update cgroup %s RAM limit anymore.", filename, cg->id);
                        freez(cg->filename_memory_limit);
                        cg->filename_memory_limit = NULL;
                    }

                    procfile_close(ff);
                }

                if(likely(ram_total)) {
                    unsigned long long memory_limit = ram_total;

                    if(unlikely(cg->memory_limit < ram_total))
                        memory_limit = cg->memory_limit;

                    if(unlikely(!cg->st_mem_usage_limit)) {
                        snprintfz(title, CHART_TITLE_MAX, "Used RAM within the limits");

                        cg->st_mem_usage_limit = rrdset_create_localhost(
                                cgroup_chart_type(type, cg->chart_id, RRD_ID_LENGTH_MAX)
                                , "mem_usage_limit"
                                , NULL
                                , "mem"
                                , "cgroup.mem_usage_limit"
                                , title
                                , "MiB"
                                , PLUGIN_CGROUPS_NAME
                                , PLUGIN_CGROUPS_MODULE_CGROUPS_NAME
                                , cgroup_containers_chart_priority + 200
                                , update_every
                                , RRDSET_TYPE_STACKED
                        );

                        rrdset_update_labels(cg->st_mem_usage_limit, cg->chart_labels);

                        rrddim_add(cg->st_mem_usage_limit, "available", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
                        rrddim_add(cg->st_mem_usage_limit, "used", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
                    }
                    else
                        rrdset_next(cg->st_mem_usage_limit);

                    rrdset_isnot_obsolete(cg->st_mem_usage_limit);

                    rrddim_set(cg->st_mem_usage_limit, "available", memory_limit - cg->memory.usage_in_bytes);
                    rrddim_set(cg->st_mem_usage_limit, "used", cg->memory.usage_in_bytes);
                    rrdset_done(cg->st_mem_usage_limit);

                    if (unlikely(!cg->st_mem_utilization)) {
                        snprintfz(title, CHART_TITLE_MAX, "Memory Utilization");

                        cg->st_mem_utilization = rrdset_create_localhost(
                                cgroup_chart_type(type, cg->chart_id, RRD_ID_LENGTH_MAX)
                                , "mem_utilization"
                                , NULL
                                , "mem"
                                , "cgroup.mem_utilization"
                                , title
                                , "percentage"
                                , PLUGIN_CGROUPS_NAME
                                , PLUGIN_CGROUPS_MODULE_CGROUPS_NAME
                                , cgroup_containers_chart_priority + 199
                                , update_every
                                , RRDSET_TYPE_AREA
                        );

                        rrdset_update_labels(cg->st_mem_utilization, cg->chart_labels);

                        rrddim_add(cg->st_mem_utilization, "utilization", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                    } else
                        rrdset_next(cg->st_mem_utilization);

                    if (memory_limit) {
                        rrdset_isnot_obsolete(cg->st_mem_utilization);

                        rrddim_set(
                            cg->st_mem_utilization, "utilization", cg->memory.usage_in_bytes * 100 / memory_limit);
                        rrdset_done(cg->st_mem_utilization);
                    }
                }
            }
            else {
                if(unlikely(cg->st_mem_usage_limit)) {
                    rrdset_is_obsolete(cg->st_mem_usage_limit);
                    cg->st_mem_usage_limit = NULL;
                }

                if(unlikely(cg->st_mem_utilization)) {
                    rrdset_is_obsolete(cg->st_mem_utilization);
                    cg->st_mem_utilization = NULL;
                }
            }

            update_memory_limits(&cg->filename_memoryswap_limit, &cg->chart_var_memoryswap_limit, &cg->memoryswap_limit, "memory_and_swap_limit", cg);
        }

        if(likely(cg->memory.updated_failcnt && cg->memory.enabled_failcnt == CONFIG_BOOLEAN_YES)) {
            if(unlikely(!cg->st_mem_failcnt)) {
                snprintfz(title, CHART_TITLE_MAX, "Memory Limit Failures");

                cg->st_mem_failcnt = rrdset_create_localhost(
                        cgroup_chart_type(type, cg->chart_id, RRD_ID_LENGTH_MAX)
                        , "mem_failcnt"
                        , NULL
                        , "mem"
                        , "cgroup.mem_failcnt"
                        , title
                        , "count"
                        , PLUGIN_CGROUPS_NAME
                        , PLUGIN_CGROUPS_MODULE_CGROUPS_NAME
                        , cgroup_containers_chart_priority + 250
                        , update_every
                        , RRDSET_TYPE_LINE
                );
                
                rrdset_update_labels(cg->st_mem_failcnt, cg->chart_labels);

                rrddim_add(cg->st_mem_failcnt, "failures", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            }
            else
                rrdset_next(cg->st_mem_failcnt);

            rrddim_set(cg->st_mem_failcnt, "failures", cg->memory.failcnt);
            rrdset_done(cg->st_mem_failcnt);
        }

        if(likely(cg->io_service_bytes.updated && cg->io_service_bytes.enabled == CONFIG_BOOLEAN_YES)) {
            if(unlikely(!cg->st_io)) {
                snprintfz(title, CHART_TITLE_MAX, "I/O Bandwidth (all disks)");

                cg->st_io = rrdset_create_localhost(
                        cgroup_chart_type(type, cg->chart_id, RRD_ID_LENGTH_MAX)
                        , "io"
                        , NULL
                        , "disk"
                        , "cgroup.io"
                        , title
                        , "KiB/s"
                        , PLUGIN_CGROUPS_NAME
                        , PLUGIN_CGROUPS_MODULE_CGROUPS_NAME
                        , cgroup_containers_chart_priority + 1200
                        , update_every
                        , RRDSET_TYPE_AREA
                );

                rrdset_update_labels(cg->st_io, cg->chart_labels);

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
                snprintfz(title, CHART_TITLE_MAX, "Serviced I/O Operations (all disks)");

                cg->st_serviced_ops = rrdset_create_localhost(
                        cgroup_chart_type(type, cg->chart_id, RRD_ID_LENGTH_MAX)
                        , "serviced_ops"
                        , NULL
                        , "disk"
                        , "cgroup.serviced_ops"
                        , title
                        , "operations/s"
                        , PLUGIN_CGROUPS_NAME
                        , PLUGIN_CGROUPS_MODULE_CGROUPS_NAME
                        , cgroup_containers_chart_priority + 1200
                        , update_every
                        , RRDSET_TYPE_LINE
                );

                rrdset_update_labels(cg->st_serviced_ops, cg->chart_labels);

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
                snprintfz(title, CHART_TITLE_MAX, "Throttle I/O Bandwidth (all disks)");

                cg->st_throttle_io = rrdset_create_localhost(
                        cgroup_chart_type(type, cg->chart_id, RRD_ID_LENGTH_MAX)
                        , "throttle_io"
                        , NULL
                        , "disk"
                        , "cgroup.throttle_io"
                        , title
                        , "KiB/s"
                        , PLUGIN_CGROUPS_NAME
                        , PLUGIN_CGROUPS_MODULE_CGROUPS_NAME
                        , cgroup_containers_chart_priority + 1200
                        , update_every
                        , RRDSET_TYPE_AREA
                );
                
                rrdset_update_labels(cg->st_throttle_io, cg->chart_labels);

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
                snprintfz(title, CHART_TITLE_MAX, "Throttle Serviced I/O Operations (all disks)");

                cg->st_throttle_serviced_ops = rrdset_create_localhost(
                        cgroup_chart_type(type, cg->chart_id, RRD_ID_LENGTH_MAX)
                        , "throttle_serviced_ops"
                        , NULL
                        , "disk"
                        , "cgroup.throttle_serviced_ops"
                        , title
                        , "operations/s"
                        , PLUGIN_CGROUPS_NAME
                        , PLUGIN_CGROUPS_MODULE_CGROUPS_NAME
                        , cgroup_containers_chart_priority + 1200
                        , update_every
                        , RRDSET_TYPE_LINE
                );
                
                rrdset_update_labels(cg->st_throttle_serviced_ops, cg->chart_labels);

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
                snprintfz(title, CHART_TITLE_MAX, "Queued I/O Operations (all disks)");

                cg->st_queued_ops = rrdset_create_localhost(
                        cgroup_chart_type(type, cg->chart_id, RRD_ID_LENGTH_MAX)
                        , "queued_ops"
                        , NULL
                        , "disk"
                        , "cgroup.queued_ops"
                        , title
                        , "operations"
                        , PLUGIN_CGROUPS_NAME
                        , PLUGIN_CGROUPS_MODULE_CGROUPS_NAME
                        , cgroup_containers_chart_priority + 2000
                        , update_every
                        , RRDSET_TYPE_LINE
                );
                
                rrdset_update_labels(cg->st_queued_ops, cg->chart_labels);

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
                snprintfz(title, CHART_TITLE_MAX, "Merged I/O Operations (all disks)");

                cg->st_merged_ops = rrdset_create_localhost(
                        cgroup_chart_type(type, cg->chart_id, RRD_ID_LENGTH_MAX)
                        , "merged_ops"
                        , NULL
                        , "disk"
                        , "cgroup.merged_ops"
                        , title
                        , "operations/s"
                        , PLUGIN_CGROUPS_NAME
                        , PLUGIN_CGROUPS_MODULE_CGROUPS_NAME
                        , cgroup_containers_chart_priority + 2100
                        , update_every
                        , RRDSET_TYPE_LINE
                );
                
                rrdset_update_labels(cg->st_merged_ops, cg->chart_labels);

                rrddim_add(cg->st_merged_ops, "read", NULL, 1, 1024, RRD_ALGORITHM_INCREMENTAL);
                rrddim_add(cg->st_merged_ops, "write", NULL, -1, 1024, RRD_ALGORITHM_INCREMENTAL);
            }
            else
                rrdset_next(cg->st_merged_ops);

            rrddim_set(cg->st_merged_ops, "read", cg->io_merged.Read);
            rrddim_set(cg->st_merged_ops, "write", cg->io_merged.Write);
            rrdset_done(cg->st_merged_ops);
        }

        if (cg->options & CGROUP_OPTIONS_IS_UNIFIED) {
            struct pressure *res = &cg->cpu_pressure;

            if (likely(res->updated && res->some.enabled)) {
                struct pressure_charts *pcs;
                pcs = &res->some;

                if (unlikely(!pcs->share_time.st)) {
                    RRDSET *chart;
                    snprintfz(title, CHART_TITLE_MAX, "CPU some pressure");
                    chart = pcs->share_time.st = rrdset_create_localhost(
                        cgroup_chart_type(type, cg->chart_id, RRD_ID_LENGTH_MAX)
                        , "cpu_some_pressure"
                        , NULL
                        , "cpu"
                        , "cgroup.cpu_some_pressure"
                        , title
                        , "percentage"
                        , PLUGIN_CGROUPS_NAME
                        , PLUGIN_CGROUPS_MODULE_CGROUPS_NAME
                        , cgroup_containers_chart_priority + 2200
                        , update_every
                        , RRDSET_TYPE_LINE
                    );
                    rrdset_update_labels(chart = pcs->share_time.st, cg->chart_labels);
                    pcs->share_time.rd10 = rrddim_add(chart, "some 10", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
                    pcs->share_time.rd60 = rrddim_add(chart, "some 60", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
                    pcs->share_time.rd300 = rrddim_add(chart, "some 300", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
                } else {
                    rrdset_next(pcs->share_time.st);
                }
                if (unlikely(!pcs->total_time.st)) {
                    RRDSET *chart;
                    snprintfz(title, CHART_TITLE_MAX, "CPU some pressure stall time");
                    chart = pcs->total_time.st = rrdset_create_localhost(
                        cgroup_chart_type(type, cg->chart_id, RRD_ID_LENGTH_MAX)
                        , "cpu_some_pressure_stall_time"
                        , NULL
                        , "cpu"
                        , "cgroup.cpu_some_pressure_stall_time"
                        , title
                        , "ms"
                        , PLUGIN_CGROUPS_NAME
                        , PLUGIN_CGROUPS_MODULE_CGROUPS_NAME
                        , cgroup_containers_chart_priority + 2220
                        , update_every
                        , RRDSET_TYPE_LINE
                    );
                    rrdset_update_labels(chart = pcs->total_time.st, cg->chart_labels);
                    pcs->total_time.rdtotal = rrddim_add(chart, "time", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                } else {
                    rrdset_next(pcs->total_time.st);
                }
                update_pressure_charts(pcs);
            }
            if (likely(res->updated && res->full.enabled)) {
                struct pressure_charts *pcs;
                pcs = &res->full;

                if (unlikely(!pcs->share_time.st)) {
                    RRDSET *chart;
                    snprintfz(title, CHART_TITLE_MAX, "CPU full pressure");
                    chart = pcs->share_time.st = rrdset_create_localhost(
                        cgroup_chart_type(type, cg->chart_id, RRD_ID_LENGTH_MAX)
                        , "cpu_full_pressure"
                        , NULL
                        , "cpu"
                        , "cgroup.cpu_full_pressure"
                        , title
                        , "percentage"
                        , PLUGIN_CGROUPS_NAME
                        , PLUGIN_CGROUPS_MODULE_CGROUPS_NAME
                        , cgroup_containers_chart_priority + 2240
                        , update_every
                        , RRDSET_TYPE_LINE
                    );
                    rrdset_update_labels(chart = pcs->share_time.st, cg->chart_labels);
                    pcs->share_time.rd10 = rrddim_add(chart, "full 10", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
                    pcs->share_time.rd60 = rrddim_add(chart, "full 60", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
                    pcs->share_time.rd300 = rrddim_add(chart, "full 300", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
                } else {
                    rrdset_next(pcs->share_time.st);
                }
                if (unlikely(!pcs->total_time.st)) {
                    RRDSET *chart;
                    snprintfz(title, CHART_TITLE_MAX, "CPU full pressure stall time");
                    chart = pcs->total_time.st = rrdset_create_localhost(
                        cgroup_chart_type(type, cg->chart_id, RRD_ID_LENGTH_MAX)
                        , "cpu_full_pressure_stall_time"
                        , NULL
                        , "cpu"
                        , "cgroup.cpu_full_pressure_stall_time"
                        , title
                        , "ms"
                        , PLUGIN_CGROUPS_NAME
                        , PLUGIN_CGROUPS_MODULE_CGROUPS_NAME
                        , cgroup_containers_chart_priority + 2260
                        , update_every
                        , RRDSET_TYPE_LINE
                    );
                    rrdset_update_labels(chart = pcs->total_time.st, cg->chart_labels);
                    pcs->total_time.rdtotal = rrddim_add(chart, "time", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                } else {
                    rrdset_next(pcs->total_time.st);
                }
                update_pressure_charts(pcs);
            }

            res = &cg->memory_pressure;

            if (likely(res->updated && res->some.enabled)) {
                struct pressure_charts *pcs;
                pcs = &res->some;

                if (unlikely(!pcs->share_time.st)) {
                    RRDSET *chart;
                    snprintfz(title, CHART_TITLE_MAX, "Memory some pressure");
                    chart = pcs->share_time.st = rrdset_create_localhost(
                        cgroup_chart_type(type, cg->chart_id, RRD_ID_LENGTH_MAX)
                        , "mem_some_pressure"
                        , NULL
                        , "mem"
                        , "cgroup.memory_some_pressure"
                        , title
                        , "percentage"
                        , PLUGIN_CGROUPS_NAME
                        , PLUGIN_CGROUPS_MODULE_CGROUPS_NAME
                        , cgroup_containers_chart_priority + 2300
                        , update_every
                        , RRDSET_TYPE_LINE
                        );         
                    rrdset_update_labels(chart = pcs->share_time.st, cg->chart_labels);
                    pcs->share_time.rd10 = rrddim_add(chart, "some 10", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
                    pcs->share_time.rd60 = rrddim_add(chart, "some 60", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
                    pcs->share_time.rd300 = rrddim_add(chart, "some 300", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
                } else {
                    rrdset_next(pcs->share_time.st);
                }
                if (unlikely(!pcs->total_time.st)) {
                    RRDSET *chart;
                    snprintfz(title, CHART_TITLE_MAX, "Memory some pressure stall time");
                    chart = pcs->total_time.st = rrdset_create_localhost(
                        cgroup_chart_type(type, cg->chart_id, RRD_ID_LENGTH_MAX)
                        , "memory_some_pressure_stall_time"
                        , NULL
                        , "mem"
                        , "cgroup.memory_some_pressure_stall_time"
                        , title
                        , "ms"
                        , PLUGIN_CGROUPS_NAME
                        , PLUGIN_CGROUPS_MODULE_CGROUPS_NAME
                        , cgroup_containers_chart_priority + 2320
                        , update_every
                        , RRDSET_TYPE_LINE
                    );
                    rrdset_update_labels(chart = pcs->total_time.st, cg->chart_labels);
                    pcs->total_time.rdtotal = rrddim_add(chart, "time", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                } else {
                    rrdset_next(pcs->total_time.st);
                }
                update_pressure_charts(pcs);
            }

            if (likely(res->updated && res->full.enabled)) {
                struct pressure_charts *pcs;
                pcs = &res->full;

                if (unlikely(!pcs->share_time.st)) {
                    RRDSET *chart;
                    snprintfz(title, CHART_TITLE_MAX, "Memory full pressure");

                    chart = pcs->share_time.st = rrdset_create_localhost(
                        cgroup_chart_type(type, cg->chart_id, RRD_ID_LENGTH_MAX)
                        , "mem_full_pressure"
                        , NULL
                        , "mem"
                        , "cgroup.memory_full_pressure"
                        , title
                        , "percentage"
                        , PLUGIN_CGROUPS_NAME
                        , PLUGIN_CGROUPS_MODULE_CGROUPS_NAME
                        , cgroup_containers_chart_priority + 2340
                        , update_every
                        , RRDSET_TYPE_LINE
                        );
                        
                    rrdset_update_labels(chart = pcs->share_time.st, cg->chart_labels);
                    pcs->share_time.rd10 = rrddim_add(chart, "full 10", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
                    pcs->share_time.rd60 = rrddim_add(chart, "full 60", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
                    pcs->share_time.rd300 = rrddim_add(chart, "full 300", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
                } else {
                    rrdset_next(pcs->share_time.st);
                }
                if (unlikely(!pcs->total_time.st)) {
                    RRDSET *chart;
                    snprintfz(title, CHART_TITLE_MAX, "Memory full pressure stall time");
                    chart = pcs->total_time.st = rrdset_create_localhost(
                        cgroup_chart_type(type, cg->chart_id, RRD_ID_LENGTH_MAX)
                        , "memory_full_pressure_stall_time"
                        , NULL
                        , "mem"
                        , "cgroup.memory_full_pressure_stall_time"
                        , title
                        , "ms"
                        , PLUGIN_CGROUPS_NAME
                        , PLUGIN_CGROUPS_MODULE_CGROUPS_NAME
                        , cgroup_containers_chart_priority + 2360
                        , update_every
                        , RRDSET_TYPE_LINE
                    );
                    rrdset_update_labels(chart = pcs->total_time.st, cg->chart_labels);
                    pcs->total_time.rdtotal = rrddim_add(chart, "time", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                } else {
                    rrdset_next(pcs->total_time.st);
                }
                update_pressure_charts(pcs);
            }

            res = &cg->io_pressure;

            if (likely(res->updated && res->some.enabled)) {
                struct pressure_charts *pcs;
                pcs = &res->some;

                if (unlikely(!pcs->share_time.st)) {
                    RRDSET *chart;
                    snprintfz(title, CHART_TITLE_MAX, "I/O some pressure");
                    chart = pcs->share_time.st = rrdset_create_localhost(
                        cgroup_chart_type(type, cg->chart_id, RRD_ID_LENGTH_MAX)
                        , "io_some_pressure"
                        , NULL
                        , "disk"
                        , "cgroup.io_some_pressure"
                        , title
                        , "percentage"
                        , PLUGIN_CGROUPS_NAME
                        , PLUGIN_CGROUPS_MODULE_CGROUPS_NAME
                        , cgroup_containers_chart_priority + 2400
                        , update_every
                        , RRDSET_TYPE_LINE
                        );
                    rrdset_update_labels(chart = pcs->share_time.st, cg->chart_labels);
                    pcs->share_time.rd10 = rrddim_add(chart, "some 10", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
                    pcs->share_time.rd60 = rrddim_add(chart, "some 60", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
                    pcs->share_time.rd300 = rrddim_add(chart, "some 300", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
                } else {
                    rrdset_next(pcs->share_time.st);
                }
                if (unlikely(!pcs->total_time.st)) {
                    RRDSET *chart;
                    snprintfz(title, CHART_TITLE_MAX, "I/O some pressure stall time");
                    chart = pcs->total_time.st = rrdset_create_localhost(
                        cgroup_chart_type(type, cg->chart_id, RRD_ID_LENGTH_MAX)
                        , "io_some_pressure_stall_time"
                        , NULL
                        , "disk"
                        , "cgroup.io_some_pressure_stall_time"
                        , title
                        , "ms"
                        , PLUGIN_CGROUPS_NAME
                        , PLUGIN_CGROUPS_MODULE_CGROUPS_NAME
                        , cgroup_containers_chart_priority + 2420
                        , update_every
                        , RRDSET_TYPE_LINE
                    );
                    rrdset_update_labels(chart = pcs->total_time.st, cg->chart_labels);
                    pcs->total_time.rdtotal = rrddim_add(chart, "time", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                } else {
                    rrdset_next(pcs->total_time.st);
                }
                update_pressure_charts(pcs);
            }

            if (likely(res->updated && res->full.enabled)) {
                struct pressure_charts *pcs;
                pcs = &res->full;

                if (unlikely(!pcs->share_time.st)) {
                    RRDSET *chart;
                    snprintfz(title, CHART_TITLE_MAX, "I/O full pressure");
                    chart = pcs->share_time.st = rrdset_create_localhost(
                        cgroup_chart_type(type, cg->chart_id, RRD_ID_LENGTH_MAX)
                        , "io_full_pressure"
                        , NULL
                        , "disk"
                        , "cgroup.io_full_pressure"
                        , title
                        , "percentage"
                        , PLUGIN_CGROUPS_NAME
                        , PLUGIN_CGROUPS_MODULE_CGROUPS_NAME
                        , cgroup_containers_chart_priority + 2440
                        , update_every
                        , RRDSET_TYPE_LINE
                        );
                    rrdset_update_labels(chart = pcs->share_time.st, cg->chart_labels);
                    pcs->share_time.rd10 = rrddim_add(chart, "full 10", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
                    pcs->share_time.rd60 = rrddim_add(chart, "full 60", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
                    pcs->share_time.rd300 = rrddim_add(chart, "full 300", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
                } else {
                    rrdset_next(pcs->share_time.st);
                }
                if (unlikely(!pcs->total_time.st)) {
                    RRDSET *chart;
                    snprintfz(title, CHART_TITLE_MAX, "I/O full pressure stall time");
                    chart = pcs->total_time.st = rrdset_create_localhost(
                        cgroup_chart_type(type, cg->chart_id, RRD_ID_LENGTH_MAX)
                        , "io_full_pressure_stall_time"
                        , NULL
                        , "disk"
                        , "cgroup.io_full_pressure_stall_time"
                        , title
                        , "ms"
                        , PLUGIN_CGROUPS_NAME
                        , PLUGIN_CGROUPS_MODULE_CGROUPS_NAME
                        , cgroup_containers_chart_priority + 2460
                        , update_every
                        , RRDSET_TYPE_LINE
                    );
                    rrdset_update_labels(chart = pcs->total_time.st, cg->chart_labels);
                    pcs->total_time.rdtotal = rrddim_add(chart, "time", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                } else {
                    rrdset_next(pcs->total_time.st);
                }
                update_pressure_charts(pcs);
            }
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

static void cgroup_main_cleanup(void *ptr) {
    worker_unregister();

    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)ptr;
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;

    info("cleaning up...");

    usec_t max = 2 * USEC_PER_SEC, step = 50000;

    if (!discovery_thread.exited) {
        info("stopping discovery thread worker");
        uv_mutex_lock(&discovery_thread.mutex);
        discovery_thread.start_discovery = 1;
        uv_cond_signal(&discovery_thread.cond_var);
        uv_mutex_unlock(&discovery_thread.mutex);
    }

    info("waiting for discovery thread to finish...");
    
    while (!discovery_thread.exited && max > 0) {
        max -= step;
        sleep_usec(step);
    }

    if (shm_mutex_cgroup_ebpf != SEM_FAILED) {
        sem_close(shm_mutex_cgroup_ebpf);
    }

    if (shm_cgroup_ebpf.header) {
        munmap(shm_cgroup_ebpf.header, shm_cgroup_ebpf.header->body_length);
    }

    if (shm_fd_cgroup_ebpf > 0) {
        close(shm_fd_cgroup_ebpf);
    }

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
}

void *cgroups_main(void *ptr) {
    worker_register("CGROUPS");
    worker_register_job_name(WORKER_CGROUPS_LOCK, "lock");
    worker_register_job_name(WORKER_CGROUPS_READ, "read");
    worker_register_job_name(WORKER_CGROUPS_CHART, "chart");

    netdata_thread_cleanup_push(cgroup_main_cleanup, ptr);

    if (getenv("KUBERNETES_SERVICE_HOST") != NULL && getenv("KUBERNETES_SERVICE_PORT") != NULL) {
        is_inside_k8s = 1;
        cgroup_enable_cpuacct_cpu_shares = CONFIG_BOOLEAN_YES;
    }

    read_cgroup_plugin_configuration();
    netdata_cgroup_ebpf_initialize_shm();

    if (uv_mutex_init(&cgroup_root_mutex)) {
        error("CGROUP: cannot initialize mutex for the main cgroup list");
        goto exit;
    }

    // dispatch a discovery worker thread
    discovery_thread.start_discovery = 0;
    discovery_thread.exited = 0;

    if (uv_mutex_init(&discovery_thread.mutex)) {
        error("CGROUP: cannot initialize mutex for discovery thread");
        goto exit;
    }
    if (uv_cond_init(&discovery_thread.cond_var)) {
        error("CGROUP: cannot initialize conditional variable for discovery thread");
        goto exit;
    }

    int error = uv_thread_create(&discovery_thread.thread, cgroup_discovery_worker, NULL);
    if (error) {
        error("CGROUP: cannot create thread worker. uv_thread_create(): %s", uv_strerror(error));
        goto exit;
    }
    uv_thread_set_name_np(discovery_thread.thread, "PLUGIN[cgroups]");

    heartbeat_t hb;
    heartbeat_init(&hb);
    usec_t step = cgroup_update_every * USEC_PER_SEC;
    usec_t find_every = cgroup_check_for_new_every * USEC_PER_SEC, find_dt = 0;

    while(!netdata_exit) {
        worker_is_idle();

        usec_t hb_dt = heartbeat_next(&hb, step);
        if(unlikely(netdata_exit)) break;

        find_dt += hb_dt;
        if (unlikely(find_dt >= find_every || (!is_inside_k8s && cgroups_check))) {
            uv_cond_signal(&discovery_thread.cond_var);
            discovery_thread.start_discovery = 1;
            find_dt = 0;
            cgroups_check = 0;
        }

        worker_is_busy(WORKER_CGROUPS_LOCK);
        uv_mutex_lock(&cgroup_root_mutex);

        worker_is_busy(WORKER_CGROUPS_READ);
        read_all_discovered_cgroups(cgroup_root);

        worker_is_busy(WORKER_CGROUPS_CHART);
        update_cgroup_charts(cgroup_update_every);

        worker_is_idle();
        uv_mutex_unlock(&cgroup_root_mutex);
    }

exit:
    worker_unregister();
    netdata_thread_cleanup_pop(1);
    return NULL;
}
