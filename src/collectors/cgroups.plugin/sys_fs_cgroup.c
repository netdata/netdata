// SPDX-License-Identifier: GPL-3.0-or-later

#include "cgroup-internals.h"

// main cgroups thread worker jobs
#define WORKER_CGROUPS_LOCK 0
#define WORKER_CGROUPS_READ 1
#define WORKER_CGROUPS_CHART 2

// ----------------------------------------------------------------------------
// cgroup globals
unsigned long long host_ram_total = 0;
int is_inside_k8s = 0;
long system_page_size = 4096; // system will be queried via sysconf() in configuration()
int cgroup_enable_cpuacct_stat = CONFIG_BOOLEAN_AUTO;
int cgroup_enable_cpuacct_usage = CONFIG_BOOLEAN_NO;
int cgroup_enable_cpuacct_cpu_throttling = CONFIG_BOOLEAN_YES;
int cgroup_enable_cpuacct_cpu_shares = CONFIG_BOOLEAN_NO;
int cgroup_enable_memory = CONFIG_BOOLEAN_AUTO;
int cgroup_enable_detailed_memory = CONFIG_BOOLEAN_AUTO;
int cgroup_enable_memory_failcnt = CONFIG_BOOLEAN_AUTO;
int cgroup_enable_swap = CONFIG_BOOLEAN_AUTO;
int cgroup_enable_blkio_io = CONFIG_BOOLEAN_AUTO;
int cgroup_enable_blkio_ops = CONFIG_BOOLEAN_AUTO;
int cgroup_enable_blkio_throttle_io = CONFIG_BOOLEAN_AUTO;
int cgroup_enable_blkio_throttle_ops = CONFIG_BOOLEAN_AUTO;
int cgroup_enable_blkio_merged_ops = CONFIG_BOOLEAN_AUTO;
int cgroup_enable_blkio_queued_ops = CONFIG_BOOLEAN_AUTO;
int cgroup_enable_pressure_cpu = CONFIG_BOOLEAN_AUTO;
int cgroup_enable_pressure_io_some = CONFIG_BOOLEAN_AUTO;
int cgroup_enable_pressure_io_full = CONFIG_BOOLEAN_AUTO;
int cgroup_enable_pressure_memory_some = CONFIG_BOOLEAN_AUTO;
int cgroup_enable_pressure_memory_full = CONFIG_BOOLEAN_AUTO;
int cgroup_enable_pressure_irq_some = CONFIG_BOOLEAN_NO;
int cgroup_enable_pressure_irq_full = CONFIG_BOOLEAN_AUTO;
int cgroup_enable_systemd_services = CONFIG_BOOLEAN_YES;
int cgroup_enable_systemd_services_detailed_memory = CONFIG_BOOLEAN_NO;
int cgroup_used_memory = CONFIG_BOOLEAN_YES;
int cgroup_use_unified_cgroups = CONFIG_BOOLEAN_NO;
int cgroup_unified_exist = CONFIG_BOOLEAN_AUTO;
int cgroup_search_in_devices = 1;
int cgroup_check_for_new_every = 10;
int cgroup_update_every = 1;
int cgroup_containers_chart_priority = NETDATA_CHART_PRIO_CGROUPS_CONTAINERS;
int cgroup_recheck_zero_blkio_every_iterations = 10;
int cgroup_recheck_zero_mem_failcnt_every_iterations = 10;
int cgroup_recheck_zero_mem_detailed_every_iterations = 10;
char *cgroup_cpuacct_base = NULL;
char *cgroup_cpuset_base = NULL;
char *cgroup_blkio_base = NULL;
char *cgroup_memory_base = NULL;
char *cgroup_devices_base = NULL;
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
char *cgroups_network_interface_script = NULL;
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
uv_mutex_t cgroup_root_mutex;

struct cgroups_systemd_config_setting cgroups_systemd_options[] = {
        { .name = "legacy",  .setting = SYSTEMD_CGROUP_LEGACY  },
        { .name = "hybrid",  .setting = SYSTEMD_CGROUP_HYBRID  },
        { .name = "unified", .setting = SYSTEMD_CGROUP_UNIFIED },
        { .name = NULL,      .setting = SYSTEMD_CGROUP_ERR     },
};

// Shared memory with information from detected cgroups
netdata_ebpf_cgroup_shm_t shm_cgroup_ebpf = {NULL, NULL};
int shm_fd_cgroup_ebpf = -1;
sem_t *shm_mutex_cgroup_ebpf = SEM_FAILED;

struct discovery_thread discovery_thread;


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

    FILE *fp_child_input;
    FILE *fp_child_output = netdata_popen(exec, &command_pid, &fp_child_input);

    if (!fp_child_output)
        return retval;

    fd_set rfds;
    struct timeval timeout;
    int fd = fileno(fp_child_output);
    int ret = -1;

    FD_ZERO(&rfds);
    FD_SET(fd, &rfds);
    timeout.tv_sec = 3;
    timeout.tv_usec = 0;

    if (fd != -1) {
        ret = select(fd + 1, &rfds, NULL, NULL, &timeout);
    }

    if (ret == -1) {
        collector_error("Failed to get the output of \"%s\"", exec);
    } else if (ret == 0) {
        collector_info("Cannot get the output of \"%s\" within %"PRId64" seconds", exec, (int64_t)timeout.tv_sec);
    } else {
        while (fgets(buf, MAXSIZE_PROC_CMDLINE, fp_child_output) != NULL) {
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

    if (netdata_pclose(fp_child_input, fp_child_output, command_pid))
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
    FILE *fp_child_input;
    FILE *fp_child_output = netdata_popen("grep cgroup /proc/filesystems", &command_pid, &fp_child_input);
    if (!fp_child_output) {
        collector_error("popen failed");
        return CGROUPS_AUTODETECT_FAIL;
    }
    while (fgets(buf, MAXSIZE_PROC_CMDLINE, fp_child_output) != NULL) {
        if (strstr(buf, "cgroup2")) {
            cgroups2_available = 1;
            break;
        }
    }
    if(netdata_pclose(fp_child_input, fp_child_output, command_pid))
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

    cgroup_update_every = (int)config_get_number("plugin:cgroups", "update every", localhost->rrd_update_every);
    if(cgroup_update_every < localhost->rrd_update_every)
        cgroup_update_every = localhost->rrd_update_every;

    cgroup_check_for_new_every = (int)config_get_number("plugin:cgroups", "check for new cgroups every", cgroup_check_for_new_every);
    if(cgroup_check_for_new_every < cgroup_update_every)
        cgroup_check_for_new_every = cgroup_update_every;

    cgroup_use_unified_cgroups = config_get_boolean_ondemand("plugin:cgroups", "use unified cgroups", CONFIG_BOOLEAN_AUTO);
    if(cgroup_use_unified_cgroups == CONFIG_BOOLEAN_AUTO)
        cgroup_use_unified_cgroups = (cgroups_try_detect_version() == CGROUPS_V2);

    collector_info("use unified cgroups %s", cgroup_use_unified_cgroups ? "true" : "false");

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
        if (!mi)
            mi = mountinfo_find_by_filesystem_mount_source(root, "cgroup", "cpuacct");
        if (!mi) {
            collector_error("CGROUP: cannot find cpuacct mountinfo. Assuming default: /sys/fs/cgroup/cpuacct");
            s = "/sys/fs/cgroup/cpuacct";
        } else
            s = mi->mount_point;
        set_cgroup_base_path(filename, s);
        cgroup_cpuacct_base = config_get("plugin:cgroups", "path to /sys/fs/cgroup/cpuacct", filename);

        mi = mountinfo_find_by_filesystem_super_option(root, "cgroup", "cpuset");
        if (!mi)
            mi = mountinfo_find_by_filesystem_mount_source(root, "cgroup", "cpuset");
        if (!mi) {
            collector_error("CGROUP: cannot find cpuset mountinfo. Assuming default: /sys/fs/cgroup/cpuset");
            s = "/sys/fs/cgroup/cpuset";
        } else
            s = mi->mount_point;
        set_cgroup_base_path(filename, s);
        cgroup_cpuset_base = config_get("plugin:cgroups", "path to /sys/fs/cgroup/cpuset", filename);

        mi = mountinfo_find_by_filesystem_super_option(root, "cgroup", "blkio");
        if (!mi)
            mi = mountinfo_find_by_filesystem_mount_source(root, "cgroup", "blkio");
        if (!mi) {
            collector_error("CGROUP: cannot find blkio mountinfo. Assuming default: /sys/fs/cgroup/blkio");
            s = "/sys/fs/cgroup/blkio";
        } else
            s = mi->mount_point;
        set_cgroup_base_path(filename, s);
        cgroup_blkio_base = config_get("plugin:cgroups", "path to /sys/fs/cgroup/blkio", filename);

        mi = mountinfo_find_by_filesystem_super_option(root, "cgroup", "memory");
        if (!mi)
            mi = mountinfo_find_by_filesystem_mount_source(root, "cgroup", "memory");
        if (!mi) {
            collector_error("CGROUP: cannot find memory mountinfo. Assuming default: /sys/fs/cgroup/memory");
            s = "/sys/fs/cgroup/memory";
        } else
            s = mi->mount_point;
        set_cgroup_base_path(filename, s);
        cgroup_memory_base = config_get("plugin:cgroups", "path to /sys/fs/cgroup/memory", filename);

        mi = mountinfo_find_by_filesystem_super_option(root, "cgroup", "devices");
        if (!mi)
            mi = mountinfo_find_by_filesystem_mount_source(root, "cgroup", "devices");
        if (!mi) {
            collector_error("CGROUP: cannot find devices mountinfo. Assuming default: /sys/fs/cgroup/devices");
            s = "/sys/fs/cgroup/devices";
        } else
            s = mi->mount_point;
        set_cgroup_base_path(filename, s);
        cgroup_devices_base = config_get("plugin:cgroups", "path to /sys/fs/cgroup/devices", filename);

        mi = mountinfo_find_by_filesystem_super_option(root, "cgroup", "pids");
        if (!mi)
            mi = mountinfo_find_by_filesystem_mount_source(root, "cgroup", "pids");
        if (!mi) {
            collector_error("CGROUP: cannot find pids mountinfo. Assuming default: /sys/fs/cgroup/pids");
            s = "/sys/fs/cgroup/pids";
        } else
            s = mi->mount_point;
        set_cgroup_base_path(filename, s);
        cgroup_pids_base = config_get("plugin:cgroups", "path to /sys/fs/cgroup/pids", filename);
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
        cgroup_unified_base = config_get("plugin:cgroups", "path to unified cgroups", filename);
    }

    cgroup_root_max = (int)config_get_number("plugin:cgroups", "max cgroups to allow", cgroup_root_max);
    cgroup_max_depth = (int)config_get_number("plugin:cgroups", "max cgroups depth to monitor", cgroup_max_depth);

    enabled_cgroup_paths = simple_pattern_create(
            config_get("plugin:cgroups", "enable by default cgroups matching",
            // ----------------------------------------------------------------

                       " !*/init.scope "                      // ignore init.scope
                       " !/system.slice/run-*.scope "         // ignore system.slice/run-XXXX.scope
                       " *user.slice/docker-*"                // allow docker rootless containers
                       " !*user.slice*"                       // ignore the rest stuff in user.slice 
                       " *.scope "                            // we need all other *.scope for sure

                       // ----------------------------------------------------------------

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
            config_get("plugin:cgroups", "enable by default cgroups names matching",
                       " * "
            ), NULL, SIMPLE_PATTERN_EXACT, true);

    search_cgroup_paths = simple_pattern_create(
            config_get("plugin:cgroups", "search for cgroups in subpaths matching",
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
                       " */kubepods/pod*/* "                   // k8s containers
                       " */kubepods/*/pod*/* "                 // k8s containers
                       " */*-kubepods-pod*/* "                 // k8s containers
                       " */*-kubepods-*-pod*/* "               // k8s containers
                       " !*kubepods* !*kubelet* "              // all other k8s cgroups
                       " *.libvirt-qemu "                    // #3010
                       " * "
            ), NULL, SIMPLE_PATTERN_EXACT, true);

    if(cgroup_enable_systemd_services) {
        systemd_services_cgroups = simple_pattern_create(
                config_get("plugin:cgroups", "cgroups to match as systemd services",
                           " !/system.slice/*/*.service "
                           " /system.slice/*.service "
                ), NULL, SIMPLE_PATTERN_EXACT, true);
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
        collector_error("Cannot initialize shared memory used by cgroup and eBPF, integration won't happen.");
        return;
    }

    size_t length = sizeof(netdata_ebpf_cgroup_shm_header_t) + cgroup_root_max * sizeof(netdata_ebpf_cgroup_shm_body_t);
    if (ftruncate(shm_fd_cgroup_ebpf, length)) {
        collector_error("Cannot set size for shared memory.");
        goto end_init_shm;
    }

    shm_cgroup_ebpf.header = (netdata_ebpf_cgroup_shm_header_t *) mmap(NULL, length,
                                                                       PROT_READ | PROT_WRITE, MAP_SHARED,
                                                                       shm_fd_cgroup_ebpf, 0);

    if (unlikely(MAP_FAILED == shm_cgroup_ebpf.header)) {
        shm_cgroup_ebpf.header = NULL;
        collector_error("Cannot map shared memory used between cgroup and eBPF, integration won't happen");
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

    collector_error("Cannot create semaphore, integration between eBPF and cgroup won't happen");
    munmap(shm_cgroup_ebpf.header, length);
    shm_cgroup_ebpf.header = NULL;

end_init_shm:
    close(shm_fd_cgroup_ebpf);
    shm_fd_cgroup_ebpf = -1;
    shm_unlink(NETDATA_SHARED_MEMORY_EBPF_CGROUP_NAME);
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

        ff = procfile_reopen(ff, io->filename, NULL, CGROUP_PROCFILE_FLAG);
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
            collector_error("CGROUP: file '%s' should have 1+ lines.", io->filename);
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
                io->Read += str2ull(procfile_lineword(ff, i, 2), NULL);

            else if(unlikely(hash == Write_hash && !strcmp(s, "Write")))
                io->Write += str2ull(procfile_lineword(ff, i, 2), NULL);

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

            ff = procfile_reopen(ff, io->filename, NULL, CGROUP_PROCFILE_FLAG);
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

        if(unlikely(res->some.enabled == CONFIG_BOOLEAN_AUTO))
            res->some.enabled = (did_some) ? CONFIG_BOOLEAN_YES : CONFIG_BOOLEAN_NO;

        if(unlikely(res->full.enabled == CONFIG_BOOLEAN_AUTO))
            res->full.enabled = (did_full) ? CONFIG_BOOLEAN_YES : CONFIG_BOOLEAN_NO;
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

static void cgroup_read_pids_current(struct pids *pids) {
    pids->pids_current_updated = 0;

    if (unlikely(!pids->pids_current_filename))
        return;

    pids->pids_current_updated = !read_single_number_file(pids->pids_current_filename, &pids->pids_current);
}

static inline void read_cgroup(struct cgroup *cg) {
    netdata_log_debug(D_CGROUP, "reading metrics for cgroups '%s'", cg->id);
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
        cgroup_read_pids_current(&cg->pids);
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
        cgroup2_read_pressure(&cg->irq_pressure);
        cgroup_read_memory(&cg->memory, 1);
        cgroup_read_pids_current(&cg->pids);
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
            unsigned long ncpus = read_cpuset_cpus(*filename, get_system_cpus());
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
        cg->cpuset_cpus = get_system_cpus();

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
    const RRDVAR_ACQUIRED **chart_var = &cg->chart_var_memory_limit;
    unsigned long long *value = &cg->memory_limit;

    if(*filename) {
        if(unlikely(!*chart_var)) {
            *chart_var = rrdvar_chart_variable_add_and_acquire(cg->st_mem_usage, "memory_limit");
            if(!*chart_var) {
                collector_error("Cannot create cgroup %s chart variable '%s'. Will not update its limit anymore.", cg->id, "memory_limit");
                freez(*filename);
                *filename = NULL;
            }
        }

        if(*filename && *chart_var) {
            if(!(cg->options & CGROUP_OPTIONS_IS_UNIFIED)) {
                if(read_single_number_file(*filename, value)) {
                    collector_error("Cannot refresh cgroup %s memory limit by reading '%s'. Will not update its limit anymore.", cg->id, *filename);
                    freez(*filename);
                    *filename = NULL;
                }
                else {
                    rrdvar_chart_variable_set(
                        cg->st_mem_usage, *chart_var, (NETDATA_DOUBLE)(*value) / (1024.0 * 1024.0));
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
                    rrdvar_chart_variable_set(
                        cg->st_mem_usage, *chart_var, (NETDATA_DOUBLE)(*value) / (1024.0 * 1024.0));
                    return 1;
                }
                *value = str2ull(buffer, NULL);
                rrdvar_chart_variable_set(cg->st_mem_usage, *chart_var, (NETDATA_DOUBLE)(*value) / (1024.0 * 1024.0));
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

        if (likely(cg->pids.pids_current_updated)) {
            update_pids_current_chart(cg);
        }

        cg->function_ready = true;
    }
}

void update_cgroup_charts() {
    for (struct cgroup *cg = cgroup_root; cg; cg = cg->next) {
        if(unlikely(!cg->enabled || cg->pending_renames || is_cgroup_systemd_service(cg)))
            continue;

        if (likely(cg->cpuacct_stat.updated && cg->cpuacct_stat.enabled == CONFIG_BOOLEAN_YES)) {
            update_cpu_utilization_chart(cg);

            if(likely(cg->filename_cpuset_cpus || cg->filename_cpu_cfs_period || cg->filename_cpu_cfs_quota)) {
                if(!(cg->options & CGROUP_OPTIONS_IS_UNIFIED)) {
                    update_cpu_limits(&cg->filename_cpuset_cpus, &cg->cpuset_cpus, cg);
                    update_cpu_limits(&cg->filename_cpu_cfs_period, &cg->cpu_cfs_period, cg);
                    update_cpu_limits(&cg->filename_cpu_cfs_quota, &cg->cpu_cfs_quota, cg);
                } else {
                    update_cpu_limits2(cg);
                }

                if(unlikely(!cg->chart_var_cpu_limit)) {
                    cg->chart_var_cpu_limit = rrdvar_chart_variable_add_and_acquire(cg->st_cpu, "cpu_limit");
                    if(!cg->chart_var_cpu_limit) {
                        collector_error("Cannot create cgroup %s chart variable 'cpu_limit'. Will not update its limit anymore.", cg->id);
                        if(cg->filename_cpuset_cpus) freez(cg->filename_cpuset_cpus);
                        cg->filename_cpuset_cpus = NULL;
                        if(cg->filename_cpu_cfs_period) freez(cg->filename_cpu_cfs_period);
                        cg->filename_cpu_cfs_period = NULL;
                        if(cg->filename_cpu_cfs_quota) freez(cg->filename_cpu_cfs_quota);
                        cg->filename_cpu_cfs_quota = NULL;
                    }
                } else {
                    NETDATA_DOUBLE value = 0, quota = 0;

                    if(likely( ((!(cg->options & CGROUP_OPTIONS_IS_UNIFIED)) && (cg->filename_cpuset_cpus || (cg->filename_cpu_cfs_period && cg->filename_cpu_cfs_quota)))
                            || ((cg->options & CGROUP_OPTIONS_IS_UNIFIED) && cg->filename_cpu_cfs_quota))) {
                        if(unlikely(cg->cpu_cfs_quota > 0))
                            quota = (NETDATA_DOUBLE)cg->cpu_cfs_quota / (NETDATA_DOUBLE)cg->cpu_cfs_period;

                        if(unlikely(quota > 0 && quota < cg->cpuset_cpus))
                            value = quota * 100;
                        else
                            value = (NETDATA_DOUBLE)cg->cpuset_cpus * 100;
                    }
                    if(likely(value)) {
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

        if (likely(cg->cpuacct_cpu_throttling.updated && cg->cpuacct_cpu_throttling.enabled == CONFIG_BOOLEAN_YES)) {
            update_cpu_throttled_chart(cg);
            update_cpu_throttled_duration_chart(cg);
        }

        if (likely(cg->cpuacct_cpu_shares.updated && cg->cpuacct_cpu_shares.enabled == CONFIG_BOOLEAN_YES)) {
            update_cpu_shares_chart(cg);
        }

        if (likely(cg->cpuacct_usage.updated && cg->cpuacct_usage.enabled == CONFIG_BOOLEAN_YES)) {
            update_cpu_per_core_usage_chart(cg);
        }

        if (likely(cg->memory.updated_detailed && cg->memory.enabled_detailed == CONFIG_BOOLEAN_YES)) {
            update_mem_usage_detailed_chart(cg);
            update_mem_writeback_chart(cg);

            if(!(cg->options & CGROUP_OPTIONS_IS_UNIFIED)) {
                update_mem_activity_chart(cg);
            }

            update_mem_pgfaults_chart(cg);
        }

        if (likely(cg->memory.updated_usage_in_bytes && cg->memory.enabled_usage_in_bytes == CONFIG_BOOLEAN_YES)) {
            update_mem_usage_chart(cg);

            // FIXME: this if should be only for unlimited charts
            if(likely(host_ram_total)) {
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

        if (likely(cg->memory.updated_failcnt && cg->memory.enabled_failcnt == CONFIG_BOOLEAN_YES)) {
            update_mem_failcnt_chart(cg);
        }

        if (likely(cg->io_service_bytes.updated && cg->io_service_bytes.enabled == CONFIG_BOOLEAN_YES)) {
            update_io_serviced_bytes_chart(cg);
        }

        if (likely(cg->io_serviced.updated && cg->io_serviced.enabled == CONFIG_BOOLEAN_YES)) {
            update_io_serviced_ops_chart(cg);
        }

        if (likely(cg->throttle_io_service_bytes.updated && cg->throttle_io_service_bytes.enabled == CONFIG_BOOLEAN_YES)) {
                update_throttle_io_serviced_bytes_chart(cg);
        }

        if (likely(cg->throttle_io_serviced.updated && cg->throttle_io_serviced.enabled == CONFIG_BOOLEAN_YES)) {
                update_throttle_io_serviced_ops_chart(cg);
        }

        if (likely(cg->io_queued.updated && cg->io_queued.enabled == CONFIG_BOOLEAN_YES)) {
                update_io_queued_ops_chart(cg);
        }

        if (likely(cg->io_merged.updated && cg->io_merged.enabled == CONFIG_BOOLEAN_YES)) {
                update_io_merged_ops_chart(cg);
        }

        if (likely(cg->pids.pids_current_updated)) {
                update_pids_current_chart(cg);
        }

        if (cg->options & CGROUP_OPTIONS_IS_UNIFIED) {
            if (likely(cg->cpu_pressure.updated)) {
                    if (cg->cpu_pressure.some.enabled) {
                        update_cpu_some_pressure_chart(cg);
                        update_cpu_some_pressure_stall_time_chart(cg);
                    }
                    if (cg->cpu_pressure.full.enabled) {
                        update_cpu_full_pressure_chart(cg);
                        update_cpu_full_pressure_stall_time_chart(cg);
                    }
            }

            if (likely(cg->memory_pressure.updated)) {
                if (cg->memory_pressure.some.enabled) {
                        update_mem_some_pressure_chart(cg);
                        update_mem_some_pressure_stall_time_chart(cg);
                }
                if (cg->memory_pressure.full.enabled) {
                        update_mem_full_pressure_chart(cg);
                        update_mem_full_pressure_stall_time_chart(cg);
                }
            }

            if (likely(cg->irq_pressure.updated)) {
                if (cg->irq_pressure.some.enabled) {
                        update_irq_some_pressure_chart(cg);
                        update_irq_some_pressure_stall_time_chart(cg);
                }
                if (cg->irq_pressure.full.enabled) {
                        update_irq_full_pressure_chart(cg);
                        update_irq_full_pressure_stall_time_chart(cg);
                }
            }

            if (likely(cg->io_pressure.updated)) {
                if (cg->io_pressure.some.enabled) {
                        update_io_some_pressure_chart(cg);
                        update_io_some_pressure_stall_time_chart(cg);
                }
                if (cg->io_pressure.full.enabled) {
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

static void cgroup_main_cleanup(void *ptr) {
    worker_unregister();

    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)ptr;
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;

    collector_info("cleaning up...");

    usec_t max = 2 * USEC_PER_SEC, step = 50000;

    if (!__atomic_load_n(&discovery_thread.exited, __ATOMIC_RELAXED)) {
        collector_info("waiting for discovery thread to finish...");
        while (!__atomic_load_n(&discovery_thread.exited, __ATOMIC_RELAXED) && max > 0) {
            uv_mutex_lock(&discovery_thread.mutex);
            uv_cond_signal(&discovery_thread.cond_var);
            uv_mutex_unlock(&discovery_thread.mutex);
            max -= step;
            sleep_usec(step);
        }
    }

    if (shm_mutex_cgroup_ebpf != SEM_FAILED) {
        sem_close(shm_mutex_cgroup_ebpf);
    }

    if (shm_cgroup_ebpf.header) {
        shm_cgroup_ebpf.header->cgroup_root_count = 0;
        munmap(shm_cgroup_ebpf.header, shm_cgroup_ebpf.header->body_length);
    }

    if (shm_fd_cgroup_ebpf > 0) {
        close(shm_fd_cgroup_ebpf);
    }

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
}

void cgroup_read_host_total_ram() {
    procfile *ff = NULL;
    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/proc/meminfo");

    ff = procfile_open(
        config_get("plugin:cgroups", "meminfo filename to monitor", filename), " \t:", PROCFILE_FLAG_DEFAULT);

    if (likely((ff = procfile_readall(ff)) && procfile_lines(ff) && !strncmp(procfile_word(ff, 0), "MemTotal", 8)))
        host_ram_total = str2ull(procfile_word(ff, 1), NULL) * 1024;
    else
        collector_error("Cannot read file %s. Will not create RAM limit charts.", filename);

    procfile_close(ff);
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

    cgroup_read_host_total_ram();

    netdata_cgroup_ebpf_initialize_shm();

    if (uv_mutex_init(&cgroup_root_mutex)) {
        collector_error("CGROUP: cannot initialize mutex for the main cgroup list");
        goto exit;
    }

    discovery_thread.exited = 0;

    if (uv_mutex_init(&discovery_thread.mutex)) {
        collector_error("CGROUP: cannot initialize mutex for discovery thread");
        goto exit;
    }
    if (uv_cond_init(&discovery_thread.cond_var)) {
        collector_error("CGROUP: cannot initialize conditional variable for discovery thread");
        goto exit;
    }

    int error = uv_thread_create(&discovery_thread.thread, cgroup_discovery_worker, NULL);
    if (error) {
        collector_error("CGROUP: cannot create thread worker. uv_thread_create(): %s", uv_strerror(error));
        goto exit;
    }

    uv_thread_set_name_np(discovery_thread.thread, "P[cgroups]");

    // we register this only on localhost
    // for the other nodes, the origin server should register it
    cgroup_netdev_link_init();

    rrd_function_add_inline(localhost, NULL, "containers-vms", 10,
                            RRDFUNCTIONS_PRIORITY_DEFAULT / 2, RRDFUNCTIONS_CGTOP_HELP,
                            "top", HTTP_ACCESS_ANONYMOUS_DATA,
                            cgroup_function_cgroup_top);

    rrd_function_add_inline(localhost, NULL, "systemd-services", 10,
                            RRDFUNCTIONS_PRIORITY_DEFAULT / 3, RRDFUNCTIONS_SYSTEMD_SERVICES_HELP,
                            "top", HTTP_ACCESS_ANONYMOUS_DATA,
                            cgroup_function_systemd_top);

    heartbeat_t hb;
    heartbeat_init(&hb);
    usec_t step = cgroup_update_every * USEC_PER_SEC;
    usec_t find_every = cgroup_check_for_new_every * USEC_PER_SEC, find_dt = 0;

    netdata_thread_disable_cancelability();

    while(service_running(SERVICE_COLLECTORS)) {
        worker_is_idle();

        usec_t hb_dt = heartbeat_next(&hb, step);
        if (unlikely(!service_running(SERVICE_COLLECTORS)))
            break;

        find_dt += hb_dt;
        if (unlikely(find_dt >= find_every || (!is_inside_k8s && cgroups_check))) {
            uv_mutex_lock(&discovery_thread.mutex);
            uv_cond_signal(&discovery_thread.cond_var);
            uv_mutex_unlock(&discovery_thread.mutex);
            find_dt = 0;
            cgroups_check = 0;
        }

        worker_is_busy(WORKER_CGROUPS_LOCK);
        uv_mutex_lock(&cgroup_root_mutex);

        worker_is_busy(WORKER_CGROUPS_READ);
        read_all_discovered_cgroups(cgroup_root);

        if (unlikely(!service_running(SERVICE_COLLECTORS))) {
            uv_mutex_unlock(&cgroup_root_mutex);
            break;
        }

        worker_is_busy(WORKER_CGROUPS_CHART);

        update_cgroup_charts();
        if (cgroup_enable_systemd_services)
            update_cgroup_systemd_services_charts();

        if (unlikely(!service_running(SERVICE_COLLECTORS))) {
           uv_mutex_unlock(&cgroup_root_mutex);
           break;
        }

        worker_is_idle();
        uv_mutex_unlock(&cgroup_root_mutex);
    }

exit:
    netdata_thread_cleanup_pop(1);
    return NULL;
}
