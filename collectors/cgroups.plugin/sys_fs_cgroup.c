// SPDX-License-Identifier: GPL-3.0-or-later

#include "cgroup-internals.h"

#define PLUGIN_CGROUPS_NAME "cgroups.plugin"
#define PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME "systemd"
#define PLUGIN_CGROUPS_MODULE_CGROUPS_NAME "/sys/fs/cgroup"

// main cgroups thread worker jobs
#define WORKER_CGROUPS_LOCK 0
#define WORKER_CGROUPS_READ 1
#define WORKER_CGROUPS_CHART 2

// ----------------------------------------------------------------------------
// cgroup globals

int is_inside_k8s = 0;
long system_page_size = 4096; // system will be queried via sysconf() in configuration()
int cgroup_enable_cpuacct_stat = CONFIG_BOOLEAN_AUTO;
int cgroup_enable_cpuacct_usage = CONFIG_BOOLEAN_AUTO;
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

    cgroup_check_for_new_every = (int)config_get_number("plugin:cgroups", "check for new cgroups every", (long long)cgroup_check_for_new_every * (long long)cgroup_update_every);
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
        if(!mi) mi = mountinfo_find_by_filesystem_mount_source(root, "cgroup", "cpuacct");
        if(!mi) {
            collector_error("CGROUP: cannot find cpuacct mountinfo. Assuming default: /sys/fs/cgroup/cpuacct");
            s = "/sys/fs/cgroup/cpuacct";
        }
        else s = mi->mount_point;
        set_cgroup_base_path(filename, s);
        cgroup_cpuacct_base = config_get("plugin:cgroups", "path to /sys/fs/cgroup/cpuacct", filename);

        mi = mountinfo_find_by_filesystem_super_option(root, "cgroup", "cpuset");
        if(!mi) mi = mountinfo_find_by_filesystem_mount_source(root, "cgroup", "cpuset");
        if(!mi) {
            collector_error("CGROUP: cannot find cpuset mountinfo. Assuming default: /sys/fs/cgroup/cpuset");
            s = "/sys/fs/cgroup/cpuset";
        }
        else s = mi->mount_point;
        set_cgroup_base_path(filename, s);
        cgroup_cpuset_base = config_get("plugin:cgroups", "path to /sys/fs/cgroup/cpuset", filename);

        mi = mountinfo_find_by_filesystem_super_option(root, "cgroup", "blkio");
        if(!mi) mi = mountinfo_find_by_filesystem_mount_source(root, "cgroup", "blkio");
        if(!mi) {
            collector_error("CGROUP: cannot find blkio mountinfo. Assuming default: /sys/fs/cgroup/blkio");
            s = "/sys/fs/cgroup/blkio";
        }
        else s = mi->mount_point;
        set_cgroup_base_path(filename, s);
        cgroup_blkio_base = config_get("plugin:cgroups", "path to /sys/fs/cgroup/blkio", filename);

        mi = mountinfo_find_by_filesystem_super_option(root, "cgroup", "memory");
        if(!mi) mi = mountinfo_find_by_filesystem_mount_source(root, "cgroup", "memory");
        if(!mi) {
            collector_error("CGROUP: cannot find memory mountinfo. Assuming default: /sys/fs/cgroup/memory");
            s = "/sys/fs/cgroup/memory";
        }
        else s = mi->mount_point;
        set_cgroup_base_path(filename, s);
        cgroup_memory_base = config_get("plugin:cgroups", "path to /sys/fs/cgroup/memory", filename);

        mi = mountinfo_find_by_filesystem_super_option(root, "cgroup", "devices");
        if(!mi) mi = mountinfo_find_by_filesystem_mount_source(root, "cgroup", "devices");
        if(!mi) {
            collector_error("CGROUP: cannot find devices mountinfo. Assuming default: /sys/fs/cgroup/devices");
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
        if(mi)
            netdata_log_debug(D_CGROUP, "found unified cgroup root using super options, with path: '%s'", mi->mount_point);
        if(!mi) {
            mi = mountinfo_find_by_filesystem_mount_source(root, "cgroup2", "cgroup");
            if(mi)
                netdata_log_debug(D_CGROUP, "found unified cgroup root using mountsource info, with path: '%s'", mi->mount_point);
        }
        if(!mi) {
            collector_error("CGROUP: cannot find cgroup2 mountinfo. Assuming default: /sys/fs/cgroup");
            s = "/sys/fs/cgroup";
        }
        else s = mi->mount_point;
        set_cgroup_base_path(filename, s);
        cgroup_unified_base = config_get("plugin:cgroups", "path to unified cgroups", filename);
        netdata_log_debug(D_CGROUP, "using cgroup root: '%s'", cgroup_unified_base);
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
                       " !/user.slice "
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

// ----------------------------------------------------------------------------

static inline char *cgroup_chart_type(char *buffer, struct cgroup *cg) {
    if(buffer[0]) return buffer;

    if (cg->chart_id[0] == '\0' || (cg->chart_id[0] == '/' && cg->chart_id[1] == '\0'))
        strncpy(buffer, "cgroup_root", RRD_ID_LENGTH_MAX);
    else if (is_cgroup_systemd_service(cg))
        snprintfz(buffer, RRD_ID_LENGTH_MAX, "%s%s", services_chart_id_prefix, cg->chart_id);
    else
        snprintfz(buffer, RRD_ID_LENGTH_MAX, "%s%s", cgroup_chart_id_prefix, cg->chart_id);

    return buffer;
}

// ----------------------------------------------------------------------------
// generate charts

static void update_mem_usage_chart(
        struct cgroup *cg,
        char *type,
        char *title,
        char *context,
        char *module,
        int priority,
        int update_every,
        bool do_swap_usage) {
    if (unlikely(!cg->st_mem_usage)) {
        cg->st_mem_usage = rrdset_create_localhost(
                cgroup_chart_type(type, cg),
                "mem_usage",
                NULL,
                "mem",
                context,
                title,
                "MiB",
                PLUGIN_CGROUPS_NAME,
                module,
                priority,
                update_every,
                RRDSET_TYPE_STACKED);

        rrdset_update_rrdlabels(cg->st_mem_usage, cg->chart_labels);

        cg->st_mem_rd_ram = rrddim_add(cg->st_mem_usage, "ram", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
        if (likely(do_swap_usage))
            cg->st_mem_rd_swap = rrddim_add(cg->st_mem_usage, "swap", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(cg->st_mem_usage, cg->st_mem_rd_ram, (collected_number)cg->memory.usage_in_bytes);

    if (likely(do_swap_usage)) {
        if (!(cg->options & CGROUP_OPTIONS_IS_UNIFIED))
            rrddim_set_by_pointer(
                    cg->st_mem_usage,
                    cg->st_mem_rd_swap,
                    cg->memory.msw_usage_in_bytes > (cg->memory.usage_in_bytes + cg->memory.total_inactive_file) ?
                    (collected_number)(cg->memory.msw_usage_in_bytes -
                                       (cg->memory.usage_in_bytes + cg->memory.total_inactive_file)) : 0);
        else
            rrddim_set_by_pointer(cg->st_mem_usage, cg->st_mem_rd_swap, (collected_number)cg->memory.msw_usage_in_bytes);
    }

    rrdset_done(cg->st_mem_usage);
}


// ----------------------------------------------------------------------------
// generate charts

#define CHART_TITLE_MAX 300

void update_systemd_services_charts(
    int update_every,
    int do_cpu,
    int do_mem_usage,
    int do_mem_detailed,
    int do_mem_failcnt,
    int do_swap_usage,
    int do_io,
    int do_io_ops,
    int do_throttle_io,
    int do_throttle_ops,
    int do_queued_ops,
    int do_merged_ops)
{
    // update the values
    struct cgroup *cg;
    int systemd_cgroup_chart_priority = NETDATA_CHART_PRIO_CGROUPS_SYSTEMD;
    char type[RRD_ID_LENGTH_MAX + 1];

    for (cg = cgroup_root; cg; cg = cg->next) {
        if (unlikely(!cg->enabled || cg->pending_renames || !is_cgroup_systemd_service(cg)))
            continue;

        type[0] = '\0';
        if (likely(do_cpu && cg->cpuacct_stat.updated)) {
            if (unlikely(!cg->st_cpu)) {
                cg->st_cpu = rrdset_create_localhost(
                    cgroup_chart_type(type, cg),
                    "cpu_utilization",
                    NULL,
                    "cpu",
                    "systemd.service.cpu.utilization",
                    "Systemd Services CPU utilization (100%% = 1 core)",
                    "percentage",
                    PLUGIN_CGROUPS_NAME,
                    PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME,
                    systemd_cgroup_chart_priority,
                    update_every,
                    RRDSET_TYPE_STACKED);

                rrdset_update_rrdlabels(cg->st_cpu, cg->chart_labels);
                if (!(cg->options & CGROUP_OPTIONS_IS_UNIFIED)) {
                    cg->st_cpu_rd_user = rrddim_add(cg->st_cpu, "user", NULL, 100, system_hz, RRD_ALGORITHM_INCREMENTAL);
                    cg->st_cpu_rd_system = rrddim_add(cg->st_cpu, "system", NULL, 100, system_hz, RRD_ALGORITHM_INCREMENTAL);
                } else {
                    cg->st_cpu_rd_user = rrddim_add(cg->st_cpu, "user", NULL, 100, 1000000, RRD_ALGORITHM_INCREMENTAL);
                    cg->st_cpu_rd_system = rrddim_add(cg->st_cpu, "system", NULL, 100, 1000000, RRD_ALGORITHM_INCREMENTAL);
                }
            }

            // complete the iteration
            rrddim_set_by_pointer(cg->st_cpu, cg->st_cpu_rd_user, (collected_number)cg->cpuacct_stat.user);
            rrddim_set_by_pointer(cg->st_cpu, cg->st_cpu_rd_system, (collected_number)cg->cpuacct_stat.system);
            rrdset_done(cg->st_cpu);
        }

        if (unlikely(do_mem_usage && cg->memory.updated_usage_in_bytes))
            update_mem_usage_chart(cg, type, "Systemd Services Used Memory", "systemd.service.memory.usage"
                                   , PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME, systemd_cgroup_chart_priority + 5, update_every
                                   , do_swap_usage
                                  );

        if (likely(do_mem_failcnt && cg->memory.updated_failcnt)) {
            if (unlikely(do_mem_failcnt && !cg->st_mem_failcnt)) {
                cg->st_mem_failcnt = rrdset_create_localhost(
                    cgroup_chart_type(type, cg),
                    "mem_failcnt",
                    NULL,
                    "mem",
                    "systemd.service.memory.failcnt",
                    "Systemd Services Memory Limit Failures",
                    "failures/s",
                    PLUGIN_CGROUPS_NAME,
                    PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME,
                    systemd_cgroup_chart_priority + 10,
                    update_every,
                    RRDSET_TYPE_LINE);

                rrdset_update_rrdlabels(cg->st_mem_failcnt, cg->chart_labels);
                rrddim_add(cg->st_mem_failcnt, "fail", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            }

            rrddim_set(cg->st_mem_failcnt, "fail", (collected_number)cg->memory.failcnt);
            rrdset_done(cg->st_mem_failcnt);
        }

        if (likely(do_mem_detailed && cg->memory.updated_detailed)) {
            if (unlikely(!cg->st_mem)) {
                cg->st_mem = rrdset_create_localhost(
                    cgroup_chart_type(type, cg),
                    "mem_ram_usage",
                    NULL,
                    "mem",
                    "systemd.service.memory.ram.usage",
                    "Systemd Services Memory",
                    "MiB",
                    PLUGIN_CGROUPS_NAME,
                    PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME,
                    systemd_cgroup_chart_priority + 15,
                    update_every,
                    RRDSET_TYPE_STACKED);

                rrdset_update_rrdlabels(cg->st_mem, cg->chart_labels);
                rrddim_add(cg->st_mem, "rss", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
                rrddim_add(cg->st_mem, "cache", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
                rrddim_add(cg->st_mem, "mapped_file", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
                rrddim_add(cg->st_mem, "rss_huge", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
            }

            rrddim_set(cg->st_mem, "rss", (collected_number)cg->memory.total_rss);
            rrddim_set(cg->st_mem, "cache", (collected_number)cg->memory.total_cache);
            rrddim_set(cg->st_mem, "mapped_file", (collected_number)cg->memory.total_mapped_file);
            rrddim_set(cg->st_mem, "rss_huge", (collected_number)cg->memory.total_rss_huge);
            rrdset_done(cg->st_mem);

            if (unlikely(!cg->st_writeback)) {
                cg->st_writeback = rrdset_create_localhost(
                    cgroup_chart_type(type, cg),
                    "mem_writeback",
                    NULL,
                    "mem",
                    "systemd.service.memory.writeback",
                    "Systemd Services Writeback Memory",
                    "MiB",
                    PLUGIN_CGROUPS_NAME,
                    PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME,
                    systemd_cgroup_chart_priority + 20,
                    update_every,
                    RRDSET_TYPE_STACKED);

                rrdset_update_rrdlabels(cg->st_writeback, cg->chart_labels);
                rrddim_add(cg->st_writeback, "writeback", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
                rrddim_add(cg->st_writeback, "dirty", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
            }

            rrddim_set(cg->st_writeback, "writeback", (collected_number)cg->memory.total_writeback);
            rrddim_set(cg->st_writeback, "dirty", (collected_number)cg->memory.total_dirty);
            rrdset_done(cg->st_writeback);

            if (unlikely(!cg->st_pgfaults)) {
                cg->st_pgfaults = rrdset_create_localhost(
                    cgroup_chart_type(type, cg),
                    "mem_pgfault",
                    NULL,
                    "mem",
                    "systemd.service.memory.paging.faults",
                    "Systemd Services Memory Minor and Major Page Faults",
                    "MiB/s",
                    PLUGIN_CGROUPS_NAME,
                    PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME,
                    systemd_cgroup_chart_priority + 25,
                    update_every,
                    RRDSET_TYPE_AREA);

                rrdset_update_rrdlabels(cg->st_pgfaults, cg->chart_labels);
                rrddim_add(cg->st_pgfaults, "minor", NULL, system_page_size, 1024 * 1024, RRD_ALGORITHM_INCREMENTAL);
                rrddim_add(cg->st_pgfaults, "major", NULL, system_page_size, 1024 * 1024, RRD_ALGORITHM_INCREMENTAL);
            }

            rrddim_set(cg->st_pgfaults, "minor", (collected_number)cg->memory.total_pgfault);
            rrddim_set(cg->st_pgfaults, "major", (collected_number)cg->memory.total_pgmajfault);
            rrdset_done(cg->st_pgfaults);

            if (unlikely(!cg->st_mem_activity)) {
                cg->st_mem_activity = rrdset_create_localhost(
                    cgroup_chart_type(type, cg),
                    "mem_paging_io",
                    NULL,
                    "mem",
                    "systemd.service.memory.paging.io",
                    "Systemd Services Memory Paging IO",
                    "MiB/s",
                    PLUGIN_CGROUPS_NAME,
                    PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME,
                    systemd_cgroup_chart_priority + 30,
                    update_every,
                    RRDSET_TYPE_AREA);

                rrdset_update_rrdlabels(cg->st_mem_activity, cg->chart_labels);
                rrddim_add(cg->st_mem_activity, "in", NULL, system_page_size, 1024 * 1024, RRD_ALGORITHM_INCREMENTAL);
                rrddim_add(cg->st_mem_activity, "out", NULL, -system_page_size, 1024 * 1024, RRD_ALGORITHM_INCREMENTAL);
            }

            rrddim_set(cg->st_mem_activity, "in", (collected_number)cg->memory.total_pgpgin);
            rrddim_set(cg->st_mem_activity, "out", (collected_number)cg->memory.total_pgpgout);
            rrdset_done(cg->st_mem_activity);
        }

        if (likely(do_io && cg->io_service_bytes.updated)) {
            if (unlikely(!cg->st_io)) {
                cg->st_io = rrdset_create_localhost(
                    cgroup_chart_type(type, cg),
                    "disk_io",
                    NULL,
                    "disk",
                    "systemd.service.disk.io",
                    "Systemd Services Disk Read/Write Bandwidth",
                    "KiB/s",
                    PLUGIN_CGROUPS_NAME,
                    PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME,
                    systemd_cgroup_chart_priority + 35,
                    update_every,
                    RRDSET_TYPE_AREA);

                rrdset_update_rrdlabels(cg->st_io, cg->chart_labels);
                rrddim_add(cg->st_io, "read", NULL, 1, 1024, RRD_ALGORITHM_INCREMENTAL);
                rrddim_add(cg->st_io, "write", NULL, -1, 1024, RRD_ALGORITHM_INCREMENTAL);
            }
            rrddim_set(cg->st_io, "read", (collected_number)cg->io_service_bytes.Read);
            rrddim_set(cg->st_io, "write", (collected_number)cg->io_service_bytes.Write);
            rrdset_done(cg->st_io);
        }

        if (likely(do_io_ops && cg->io_serviced.updated)) {
            if (unlikely(!cg->st_serviced_ops)) {
                cg->st_serviced_ops = rrdset_create_localhost(
                    cgroup_chart_type(type, cg),
                    "disk_iops",
                    NULL,
                    "disk",
                    "systemd.service.disk.iops",
                    "Systemd Services Disk Read/Write Operations",
                    "operations/s",
                    PLUGIN_CGROUPS_NAME,
                    PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME,
                    systemd_cgroup_chart_priority + 40,
                    update_every,
                    RRDSET_TYPE_LINE);

                rrdset_update_rrdlabels(cg->st_serviced_ops, cg->chart_labels);
                rrddim_add(cg->st_serviced_ops, "read", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                rrddim_add(cg->st_serviced_ops, "write", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            }
            rrddim_set(cg->st_serviced_ops, "read", (collected_number)cg->io_serviced.Read);
            rrddim_set(cg->st_serviced_ops, "write", (collected_number)cg->io_serviced.Write);
            rrdset_done(cg->st_serviced_ops);
        }

        if (likely(do_throttle_io && cg->throttle_io_service_bytes.updated)) {
            if (unlikely(!cg->st_throttle_io)) {
                cg->st_throttle_io = rrdset_create_localhost(
                    cgroup_chart_type(type, cg),
                    "disk_throttle_io",
                    NULL,
                    "disk",
                    "systemd.service.disk.throttle.io",
                    "Systemd Services Throttle Disk Read/Write Bandwidth",
                    "KiB/s",
                    PLUGIN_CGROUPS_NAME,
                    PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME,
                    systemd_cgroup_chart_priority + 45,
                    update_every,
                    RRDSET_TYPE_AREA);

                rrdset_update_rrdlabels(cg->st_throttle_io, cg->chart_labels);
                rrddim_add(cg->st_throttle_io, "read", NULL, 1, 1024, RRD_ALGORITHM_INCREMENTAL);
                rrddim_add(cg->st_throttle_io, "write", NULL, -1, 1024, RRD_ALGORITHM_INCREMENTAL);
            }
            rrddim_set(cg->st_throttle_io, "read", (collected_number)cg->throttle_io_service_bytes.Read);
            rrddim_set(cg->st_throttle_io, "write", (collected_number)cg->throttle_io_service_bytes.Write);
            rrdset_done(cg->st_throttle_io);
        }

        if (likely(do_throttle_ops && cg->throttle_io_serviced.updated)) {
            if (unlikely(!cg->st_throttle_serviced_ops)) {
                cg->st_throttle_serviced_ops = rrdset_create_localhost(
                    cgroup_chart_type(type, cg),
                    "disk_throttle_iops",
                    NULL,
                    "disk",
                    "systemd.service.disk.throttle.iops",
                    "Systemd Services Throttle Disk Read/Write Operations",
                    "operations/s",
                    PLUGIN_CGROUPS_NAME,
                    PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME,
                    systemd_cgroup_chart_priority + 50,
                    update_every,
                    RRDSET_TYPE_LINE);

                rrdset_update_rrdlabels(cg->st_throttle_serviced_ops, cg->chart_labels);
                rrddim_add(cg->st_throttle_serviced_ops, "read", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                rrddim_add(cg->st_throttle_serviced_ops, "write", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            }
            rrddim_set(cg->st_throttle_serviced_ops, "read", (collected_number)cg->throttle_io_serviced.Read);
            rrddim_set(cg->st_throttle_serviced_ops, "write", (collected_number)cg->throttle_io_serviced.Write);
            rrdset_done(cg->st_throttle_serviced_ops);
        }

        if (likely(do_queued_ops && cg->io_queued.updated)) {
            if (unlikely(!cg->st_queued_ops)) {
                cg->st_queued_ops = rrdset_create_localhost(
                    cgroup_chart_type(type, cg),
                    "disk_queued_iops",
                    NULL,
                    "disk",
                    "systemd.service.disk.queued_iops",
                    "Systemd Services Queued Disk Read/Write Operations",
                    "operations/s",
                    PLUGIN_CGROUPS_NAME,
                    PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME,
                    systemd_cgroup_chart_priority + 55,
                    update_every,
                    RRDSET_TYPE_LINE);

                rrdset_update_rrdlabels(cg->st_queued_ops, cg->chart_labels);
                rrddim_add(cg->st_queued_ops, "read", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                rrddim_add(cg->st_queued_ops, "write", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            }
            rrddim_set(cg->st_queued_ops, "read", (collected_number)cg->io_queued.Read);
            rrddim_set(cg->st_queued_ops, "write", (collected_number)cg->io_queued.Write);
            rrdset_done(cg->st_queued_ops);
        }

        if (likely(do_merged_ops && cg->io_merged.updated)) {
            if (unlikely(!cg->st_merged_ops)) {
                cg->st_merged_ops = rrdset_create_localhost(
                    cgroup_chart_type(type, cg),
                    "disk_merged_iops",
                    NULL,
                    "disk",
                    "systemd.service.disk.merged_iops",
                    "Systemd Services Merged Disk Read/Write Operations",
                    "operations/s",
                    PLUGIN_CGROUPS_NAME,
                    PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME,
                    systemd_cgroup_chart_priority + 60,
                    update_every,
                    RRDSET_TYPE_LINE);

                rrdset_update_rrdlabels(cg->st_merged_ops, cg->chart_labels);
                rrddim_add(cg->st_merged_ops, "read", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                rrddim_add(cg->st_merged_ops, "write", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            }
            rrddim_set(cg->st_merged_ops, "read", (collected_number)cg->io_merged.Read);
            rrddim_set(cg->st_merged_ops, "write", (collected_number)cg->io_merged.Write);
            rrdset_done(cg->st_merged_ops);
        }
    }
}

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

static inline int update_memory_limits(char **filename, const RRDSETVAR_ACQUIRED **chart_var, unsigned long long *value, const char *chart_var_name, struct cgroup *cg) {
    if(*filename) {
        if(unlikely(!*chart_var)) {
            *chart_var = rrdsetvar_custom_chart_variable_add_and_acquire(cg->st_mem_usage, chart_var_name);
            if(!*chart_var) {
                collector_error("Cannot create cgroup %s chart variable '%s'. Will not update its limit anymore.", cg->id, chart_var_name);
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
                    rrdsetvar_custom_chart_variable_set(cg->st_mem_usage, *chart_var, (NETDATA_DOUBLE)(*value) / (1024.0 * 1024.0));
                    return 1;
                }
            } else {
                char buffer[30 + 1];
                int ret = read_file(*filename, buffer, 30);
                if(ret) {
                    collector_error("Cannot refresh cgroup %s memory limit by reading '%s'. Will not update its limit anymore.", cg->id, *filename);
                    freez(*filename);
                    *filename = NULL;
                    return 0;
                }
                char *s = "max\n\0";
                if(strcmp(s, buffer) == 0){
                    *value = UINT64_MAX;
                    rrdsetvar_custom_chart_variable_set(cg->st_mem_usage, *chart_var, (NETDATA_DOUBLE)(*value) / (1024.0 * 1024.0));
                    return 1;
                }
                *value = str2ull(buffer, NULL);
                rrdsetvar_custom_chart_variable_set(cg->st_mem_usage, *chart_var, (NETDATA_DOUBLE)(*value) / (1024.0 * 1024.0));
                return 1;
            }
        }
    }
    return 0;
}

void update_cgroup_charts(int update_every) {
    netdata_log_debug(D_CGROUP, "updating cgroups charts");

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
                snprintfz(
                    title,
                    CHART_TITLE_MAX,
                    k8s_is_kubepod(cg) ? "CPU Usage (100%% = 1000 mCPU)" : "CPU Usage (100%% = 1 core)");

                cg->st_cpu = rrdset_create_localhost(
                          cgroup_chart_type(type, cg)
                        , "cpu"
                        , NULL
                        , "cpu"
                        , k8s_is_kubepod(cg) ? "k8s.cgroup.cpu" : "cgroup.cpu"
                        , title
                        , "percentage"
                        , PLUGIN_CGROUPS_NAME
                        , PLUGIN_CGROUPS_MODULE_CGROUPS_NAME
                        , cgroup_containers_chart_priority
                        , update_every
                        , RRDSET_TYPE_STACKED
                );

                rrdset_update_rrdlabels(cg->st_cpu, cg->chart_labels);

                if(!(cg->options & CGROUP_OPTIONS_IS_UNIFIED)) {
                    cg->st_cpu_rd_user = rrddim_add(cg->st_cpu, "user", NULL, 100, system_hz, RRD_ALGORITHM_INCREMENTAL);
                    cg->st_cpu_rd_system = rrddim_add(cg->st_cpu, "system", NULL, 100, system_hz, RRD_ALGORITHM_INCREMENTAL);
                }
                else {
                    cg->st_cpu_rd_user = rrddim_add(cg->st_cpu, "user", NULL, 100, 1000000, RRD_ALGORITHM_INCREMENTAL);
                    cg->st_cpu_rd_system = rrddim_add(cg->st_cpu, "system", NULL, 100, 1000000, RRD_ALGORITHM_INCREMENTAL);
                }
            }

            rrddim_set_by_pointer(cg->st_cpu, cg->st_cpu_rd_user, (collected_number)cg->cpuacct_stat.user);
            rrddim_set_by_pointer(cg->st_cpu, cg->st_cpu_rd_system, (collected_number)cg->cpuacct_stat.system);
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
                    cg->chart_var_cpu_limit = rrdsetvar_custom_chart_variable_add_and_acquire(cg->st_cpu, "cpu_limit");
                    if(!cg->chart_var_cpu_limit) {
                        collector_error("Cannot create cgroup %s chart variable 'cpu_limit'. Will not update its limit anymore.", cg->id);
                        if(cg->filename_cpuset_cpus) freez(cg->filename_cpuset_cpus);
                        cg->filename_cpuset_cpus = NULL;
                        if(cg->filename_cpu_cfs_period) freez(cg->filename_cpu_cfs_period);
                        cg->filename_cpu_cfs_period = NULL;
                        if(cg->filename_cpu_cfs_quota) freez(cg->filename_cpu_cfs_quota);
                        cg->filename_cpu_cfs_quota = NULL;
                    }
                }
                else {
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
                        if(unlikely(!cg->st_cpu_limit)) {
                            snprintfz(title, CHART_TITLE_MAX, "CPU Usage within the limits");

                            cg->st_cpu_limit = rrdset_create_localhost(
                                      cgroup_chart_type(type, cg)
                                    , "cpu_limit"
                                    , NULL
                                    , "cpu"
                                    , k8s_is_kubepod(cg) ? "k8s.cgroup.cpu_limit" : "cgroup.cpu_limit"
                                    , title
                                    , "percentage"
                                    , PLUGIN_CGROUPS_NAME
                                    , PLUGIN_CGROUPS_MODULE_CGROUPS_NAME
                                    , cgroup_containers_chart_priority - 1
                                    , update_every
                                    , RRDSET_TYPE_LINE
                            );

                            rrdset_update_rrdlabels(cg->st_cpu_limit, cg->chart_labels);

                            if(!(cg->options & CGROUP_OPTIONS_IS_UNIFIED))
                                rrddim_add(cg->st_cpu_limit, "used", NULL, 1, system_hz, RRD_ALGORITHM_ABSOLUTE);
                            else
                                rrddim_add(cg->st_cpu_limit, "used", NULL, 1, 1000000, RRD_ALGORITHM_ABSOLUTE);
                            cg->prev_cpu_usage = (NETDATA_DOUBLE)(cg->cpuacct_stat.user + cg->cpuacct_stat.system) * 100;
                        }

                        NETDATA_DOUBLE cpu_usage = 0;
                        cpu_usage = (NETDATA_DOUBLE)(cg->cpuacct_stat.user + cg->cpuacct_stat.system) * 100;
                        NETDATA_DOUBLE cpu_used = 100 * (cpu_usage - cg->prev_cpu_usage) / (value * update_every);

                        rrdset_isnot_obsolete___safe_from_collector_thread(cg->st_cpu_limit);

                        rrddim_set(cg->st_cpu_limit, "used", (cpu_used > 0)?(collected_number)cpu_used:0);

                        cg->prev_cpu_usage = cpu_usage;

                        rrdsetvar_custom_chart_variable_set(cg->st_cpu, cg->chart_var_cpu_limit, value);
                        rrdset_done(cg->st_cpu_limit);
                    }
                    else {
                        if(unlikely(cg->st_cpu_limit)) {
                            rrdset_is_obsolete___safe_from_collector_thread(cg->st_cpu_limit);
                            cg->st_cpu_limit = NULL;
                        }
                        rrdsetvar_custom_chart_variable_set(cg->st_cpu, cg->chart_var_cpu_limit, NAN);
                    }
                }
            }
        }

        if (likely(cg->cpuacct_cpu_throttling.updated && cg->cpuacct_cpu_throttling.enabled == CONFIG_BOOLEAN_YES)) {
            if (unlikely(!cg->st_cpu_nr_throttled)) {
                snprintfz(title, CHART_TITLE_MAX, "CPU Throttled Runnable Periods");

                cg->st_cpu_nr_throttled = rrdset_create_localhost(
                          cgroup_chart_type(type, cg)
                        , "throttled"
                        , NULL
                        , "cpu"
                        , k8s_is_kubepod(cg) ? "k8s.cgroup.throttled" : "cgroup.throttled"
                        , title
                        , "percentage"
                        , PLUGIN_CGROUPS_NAME
                        , PLUGIN_CGROUPS_MODULE_CGROUPS_NAME
                        , cgroup_containers_chart_priority + 10
                        , update_every
                        , RRDSET_TYPE_LINE
                );

                rrdset_update_rrdlabels(cg->st_cpu_nr_throttled, cg->chart_labels);
                rrddim_add(cg->st_cpu_nr_throttled, "throttled", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            } else {
                rrddim_set(cg->st_cpu_nr_throttled, "throttled", (collected_number)cg->cpuacct_cpu_throttling.nr_throttled_perc);
                rrdset_done(cg->st_cpu_nr_throttled);
            }

            if (unlikely(!cg->st_cpu_throttled_time)) {
                snprintfz(title, CHART_TITLE_MAX, "CPU Throttled Time Duration");

                cg->st_cpu_throttled_time = rrdset_create_localhost(
                          cgroup_chart_type(type, cg)
                        , "throttled_duration"
                        , NULL
                        , "cpu"
                        , k8s_is_kubepod(cg) ? "k8s.cgroup.throttled_duration" : "cgroup.throttled_duration"
                        , title
                        , "ms"
                        , PLUGIN_CGROUPS_NAME
                        , PLUGIN_CGROUPS_MODULE_CGROUPS_NAME
                        , cgroup_containers_chart_priority + 15
                        , update_every
                        , RRDSET_TYPE_LINE
                );

                rrdset_update_rrdlabels(cg->st_cpu_throttled_time, cg->chart_labels);
                rrddim_add(cg->st_cpu_throttled_time, "duration", NULL, 1, 1000000, RRD_ALGORITHM_INCREMENTAL);
            } else {
                rrddim_set(cg->st_cpu_throttled_time, "duration", (collected_number)cg->cpuacct_cpu_throttling.throttled_time);
                rrdset_done(cg->st_cpu_throttled_time);
            }
        }

        if (likely(cg->cpuacct_cpu_shares.updated && cg->cpuacct_cpu_shares.enabled == CONFIG_BOOLEAN_YES)) {
            if (unlikely(!cg->st_cpu_shares)) {
                snprintfz(title, CHART_TITLE_MAX, "CPU Time Relative Share");

                cg->st_cpu_shares = rrdset_create_localhost(
                          cgroup_chart_type(type, cg)
                        , "cpu_shares"
                        , NULL
                        , "cpu"
                        , k8s_is_kubepod(cg) ? "k8s.cgroup.cpu_shares" : "cgroup.cpu_shares"
                        , title
                        , "shares"
                        , PLUGIN_CGROUPS_NAME
                        , PLUGIN_CGROUPS_MODULE_CGROUPS_NAME
                        , cgroup_containers_chart_priority + 20
                        , update_every
                        , RRDSET_TYPE_LINE
                );

                rrdset_update_rrdlabels(cg->st_cpu_shares, cg->chart_labels);
                rrddim_add(cg->st_cpu_shares, "shares", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            } else {
                rrddim_set(cg->st_cpu_shares, "shares", (collected_number)cg->cpuacct_cpu_shares.shares);
                rrdset_done(cg->st_cpu_shares);
            }
        }

        if(likely(cg->cpuacct_usage.updated && cg->cpuacct_usage.enabled == CONFIG_BOOLEAN_YES)) {
            char id[RRD_ID_LENGTH_MAX + 1];
            unsigned int i;

            if(unlikely(!cg->st_cpu_per_core)) {
                snprintfz(
                    title,
                    CHART_TITLE_MAX,
                    k8s_is_kubepod(cg) ? "CPU Usage (100%% = 1000 mCPU) Per Core" :
                                         "CPU Usage (100%% = 1 core) Per Core");

                cg->st_cpu_per_core = rrdset_create_localhost(
                          cgroup_chart_type(type, cg)
                        , "cpu_per_core"
                        , NULL
                        , "cpu"
                        , k8s_is_kubepod(cg) ? "k8s.cgroup.cpu_per_core" : "cgroup.cpu_per_core"
                        , title
                        , "percentage"
                        , PLUGIN_CGROUPS_NAME
                        , PLUGIN_CGROUPS_MODULE_CGROUPS_NAME
                        , cgroup_containers_chart_priority + 100
                        , update_every
                        , RRDSET_TYPE_STACKED
                );

                rrdset_update_rrdlabels(cg->st_cpu_per_core, cg->chart_labels);

                for(i = 0; i < cg->cpuacct_usage.cpus; i++) {
                    snprintfz(id, RRD_ID_LENGTH_MAX, "cpu%u", i);
                    rrddim_add(cg->st_cpu_per_core, id, NULL, 100, 1000000000, RRD_ALGORITHM_INCREMENTAL);
                }
            }

            for(i = 0; i < cg->cpuacct_usage.cpus ;i++) {
                snprintfz(id, RRD_ID_LENGTH_MAX, "cpu%u", i);
                rrddim_set(cg->st_cpu_per_core, id, (collected_number)cg->cpuacct_usage.cpu_percpu[i]);
            }
            rrdset_done(cg->st_cpu_per_core);
        }

        if(likely(cg->memory.updated_detailed && cg->memory.enabled_detailed == CONFIG_BOOLEAN_YES)) {
            if(unlikely(!cg->st_mem)) {
                snprintfz(title, CHART_TITLE_MAX, "Memory Usage");

                cg->st_mem = rrdset_create_localhost(
                          cgroup_chart_type(type, cg)
                        , "mem"
                        , NULL
                        , "mem"
                        , k8s_is_kubepod(cg) ? "k8s.cgroup.mem" : "cgroup.mem"
                        , title
                        , "MiB"
                        , PLUGIN_CGROUPS_NAME
                        , PLUGIN_CGROUPS_MODULE_CGROUPS_NAME
                        , cgroup_containers_chart_priority + 220
                        , update_every
                        , RRDSET_TYPE_STACKED
                );

                rrdset_update_rrdlabels(cg->st_mem, cg->chart_labels);

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

            if(!(cg->options & CGROUP_OPTIONS_IS_UNIFIED)) {
                rrddim_set(cg->st_mem, "cache", (collected_number)cg->memory.total_cache);
                rrddim_set(cg->st_mem, "rss", (cg->memory.total_rss > cg->memory.total_rss_huge)?(collected_number)(cg->memory.total_rss - cg->memory.total_rss_huge):0);

                if(cg->memory.detailed_has_swap)
                    rrddim_set(cg->st_mem, "swap", (collected_number)cg->memory.total_swap);

                rrddim_set(cg->st_mem, "rss_huge", (collected_number)cg->memory.total_rss_huge);
                rrddim_set(cg->st_mem, "mapped_file", (collected_number)cg->memory.total_mapped_file);
            } else {
                rrddim_set(cg->st_mem, "anon", (collected_number)cg->memory.anon);
                rrddim_set(cg->st_mem, "kernel_stack", (collected_number)cg->memory.kernel_stack);
                rrddim_set(cg->st_mem, "slab", (collected_number)cg->memory.slab);
                rrddim_set(cg->st_mem, "sock", (collected_number)cg->memory.sock);
                rrddim_set(cg->st_mem, "anon_thp", (collected_number)cg->memory.anon_thp);
                rrddim_set(cg->st_mem, "file", (collected_number)cg->memory.total_mapped_file);
            }
            rrdset_done(cg->st_mem);

            if(unlikely(!cg->st_writeback)) {
                snprintfz(title, CHART_TITLE_MAX, "Writeback Memory");

                cg->st_writeback = rrdset_create_localhost(
                          cgroup_chart_type(type, cg)
                        , "writeback"
                        , NULL
                        , "mem"
                        , k8s_is_kubepod(cg) ? "k8s.cgroup.writeback" : "cgroup.writeback"
                        , title
                        , "MiB"
                        , PLUGIN_CGROUPS_NAME
                        , PLUGIN_CGROUPS_MODULE_CGROUPS_NAME
                        , cgroup_containers_chart_priority + 300
                        , update_every
                        , RRDSET_TYPE_AREA
                );

                rrdset_update_rrdlabels(cg->st_writeback, cg->chart_labels);

                if(cg->memory.detailed_has_dirty)
                    rrddim_add(cg->st_writeback, "dirty", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);

                rrddim_add(cg->st_writeback, "writeback", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
            }

            if(cg->memory.detailed_has_dirty)
                rrddim_set(cg->st_writeback, "dirty", (collected_number)cg->memory.total_dirty);

            rrddim_set(cg->st_writeback, "writeback", (collected_number)cg->memory.total_writeback);
            rrdset_done(cg->st_writeback);

            if(!(cg->options & CGROUP_OPTIONS_IS_UNIFIED)) {
                if(unlikely(!cg->st_mem_activity)) {
                    snprintfz(title, CHART_TITLE_MAX, "Memory Activity");

                    cg->st_mem_activity = rrdset_create_localhost(
                              cgroup_chart_type(type, cg)
                            , "mem_activity"
                            , NULL
                            , "mem"
                            , k8s_is_kubepod(cg) ? "k8s.cgroup.mem_activity" : "cgroup.mem_activity"
                            , title
                            , "MiB/s"
                            , PLUGIN_CGROUPS_NAME
                            , PLUGIN_CGROUPS_MODULE_CGROUPS_NAME
                            , cgroup_containers_chart_priority + 400
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    rrdset_update_rrdlabels(cg->st_mem_activity, cg->chart_labels);

                    rrddim_add(cg->st_mem_activity, "pgpgin", "in", system_page_size, 1024 * 1024, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(cg->st_mem_activity, "pgpgout", "out", -system_page_size, 1024 * 1024, RRD_ALGORITHM_INCREMENTAL);
                }

                rrddim_set(cg->st_mem_activity, "pgpgin", (collected_number)cg->memory.total_pgpgin);
                rrddim_set(cg->st_mem_activity, "pgpgout", (collected_number)cg->memory.total_pgpgout);
                rrdset_done(cg->st_mem_activity);
            }

            if(unlikely(!cg->st_pgfaults)) {
                snprintfz(title, CHART_TITLE_MAX, "Memory Page Faults");

                cg->st_pgfaults = rrdset_create_localhost(
                          cgroup_chart_type(type, cg)
                        , "pgfaults"
                        , NULL
                        , "mem"
                        , k8s_is_kubepod(cg) ? "k8s.cgroup.pgfaults" : "cgroup.pgfaults"
                        , title
                        , "MiB/s"
                        , PLUGIN_CGROUPS_NAME
                        , PLUGIN_CGROUPS_MODULE_CGROUPS_NAME
                        , cgroup_containers_chart_priority + 500
                        , update_every
                        , RRDSET_TYPE_LINE
                );

                rrdset_update_rrdlabels(cg->st_pgfaults, cg->chart_labels);

                rrddim_add(cg->st_pgfaults, "pgfault", NULL, system_page_size, 1024 * 1024, RRD_ALGORITHM_INCREMENTAL);
                rrddim_add(cg->st_pgfaults, "pgmajfault", "swap", -system_page_size, 1024 * 1024, RRD_ALGORITHM_INCREMENTAL);
            }

            rrddim_set(cg->st_pgfaults, "pgfault", (collected_number)cg->memory.total_pgfault);
            rrddim_set(cg->st_pgfaults, "pgmajfault", (collected_number)cg->memory.total_pgmajfault);
            rrdset_done(cg->st_pgfaults);
        }

        if(likely(cg->memory.updated_usage_in_bytes && cg->memory.enabled_usage_in_bytes == CONFIG_BOOLEAN_YES)) {
            update_mem_usage_chart(cg, type, "Used Memory"
                                   , k8s_is_kubepod(cg) ? "k8s.cgroup.mem_usage" : "cgroup.mem_usage"
                                   , PLUGIN_CGROUPS_MODULE_CGROUPS_NAME, cgroup_containers_chart_priority + 210
                                   , update_every, true
                                  );

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
                        ram_total = str2ull(procfile_word(ff, 1), NULL) * 1024;
                    else {
                        collector_error("Cannot read file %s. Will not update cgroup %s RAM limit anymore.", filename, cg->id);
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
                                cgroup_chart_type(type, cg)
                                , "mem_usage_limit"
                                , NULL
                                , "mem"
                                , k8s_is_kubepod(cg) ? "k8s.cgroup.mem_usage_limit": "cgroup.mem_usage_limit"
                                , title
                                , "MiB"
                                , PLUGIN_CGROUPS_NAME
                                , PLUGIN_CGROUPS_MODULE_CGROUPS_NAME
                                , cgroup_containers_chart_priority + 200
                                , update_every
                                , RRDSET_TYPE_STACKED
                        );

                        rrdset_update_rrdlabels(cg->st_mem_usage_limit, cg->chart_labels);

                        rrddim_add(cg->st_mem_usage_limit, "available", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
                        rrddim_add(cg->st_mem_usage_limit, "used", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
                    }

                    rrdset_isnot_obsolete___safe_from_collector_thread(cg->st_mem_usage_limit);

                    rrddim_set(cg->st_mem_usage_limit, "available", (collected_number)(memory_limit - cg->memory.usage_in_bytes));
                    rrddim_set(cg->st_mem_usage_limit, "used", (collected_number)cg->memory.usage_in_bytes);
                    rrdset_done(cg->st_mem_usage_limit);

                    if (unlikely(!cg->st_mem_utilization)) {
                        snprintfz(title, CHART_TITLE_MAX, "Memory Utilization");

                        cg->st_mem_utilization = rrdset_create_localhost(
                                  cgroup_chart_type(type, cg)
                                , "mem_utilization"
                                , NULL
                                , "mem"
                                , k8s_is_kubepod(cg) ? "k8s.cgroup.mem_utilization" : "cgroup.mem_utilization"
                                , title
                                , "percentage"
                                , PLUGIN_CGROUPS_NAME
                                , PLUGIN_CGROUPS_MODULE_CGROUPS_NAME
                                , cgroup_containers_chart_priority + 199
                                , update_every
                                , RRDSET_TYPE_AREA
                        );

                        rrdset_update_rrdlabels(cg->st_mem_utilization, cg->chart_labels);

                        rrddim_add(cg->st_mem_utilization, "utilization", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                    }

                    if (memory_limit) {
                        rrdset_isnot_obsolete___safe_from_collector_thread(cg->st_mem_utilization);

                        rrddim_set(
                            cg->st_mem_utilization, "utilization", (collected_number)(cg->memory.usage_in_bytes * 100 / memory_limit));
                        rrdset_done(cg->st_mem_utilization);
                    }
                }
            }
            else {
                if(unlikely(cg->st_mem_usage_limit)) {
                    rrdset_is_obsolete___safe_from_collector_thread(cg->st_mem_usage_limit);
                    cg->st_mem_usage_limit = NULL;
                }

                if(unlikely(cg->st_mem_utilization)) {
                    rrdset_is_obsolete___safe_from_collector_thread(cg->st_mem_utilization);
                    cg->st_mem_utilization = NULL;
                }
            }

            update_memory_limits(&cg->filename_memoryswap_limit, &cg->chart_var_memoryswap_limit, &cg->memoryswap_limit, "memory_and_swap_limit", cg);
        }

        if(likely(cg->memory.updated_failcnt && cg->memory.enabled_failcnt == CONFIG_BOOLEAN_YES)) {
            if(unlikely(!cg->st_mem_failcnt)) {
                snprintfz(title, CHART_TITLE_MAX, "Memory Limit Failures");

                cg->st_mem_failcnt = rrdset_create_localhost(
                          cgroup_chart_type(type, cg)
                        , "mem_failcnt"
                        , NULL
                        , "mem"
                        , k8s_is_kubepod(cg) ? "k8s.cgroup.mem_failcnt" : "cgroup.mem_failcnt"
                        , title
                        , "count"
                        , PLUGIN_CGROUPS_NAME
                        , PLUGIN_CGROUPS_MODULE_CGROUPS_NAME
                        , cgroup_containers_chart_priority + 250
                        , update_every
                        , RRDSET_TYPE_LINE
                );

                rrdset_update_rrdlabels(cg->st_mem_failcnt, cg->chart_labels);

                rrddim_add(cg->st_mem_failcnt, "failures", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            }

            rrddim_set(cg->st_mem_failcnt, "failures", (collected_number)cg->memory.failcnt);
            rrdset_done(cg->st_mem_failcnt);
        }

        if(likely(cg->io_service_bytes.updated && cg->io_service_bytes.enabled == CONFIG_BOOLEAN_YES)) {
            if(unlikely(!cg->st_io)) {
                snprintfz(title, CHART_TITLE_MAX, "I/O Bandwidth (all disks)");

                cg->st_io = rrdset_create_localhost(
                          cgroup_chart_type(type, cg)
                        , "io"
                        , NULL
                        , "disk"
                        , k8s_is_kubepod(cg) ? "k8s.cgroup.io" : "cgroup.io"
                        , title
                        , "KiB/s"
                        , PLUGIN_CGROUPS_NAME
                        , PLUGIN_CGROUPS_MODULE_CGROUPS_NAME
                        , cgroup_containers_chart_priority + 1200
                        , update_every
                        , RRDSET_TYPE_AREA
                );

                rrdset_update_rrdlabels(cg->st_io, cg->chart_labels);

                rrddim_add(cg->st_io, "read", NULL, 1, 1024, RRD_ALGORITHM_INCREMENTAL);
                rrddim_add(cg->st_io, "write", NULL, -1, 1024, RRD_ALGORITHM_INCREMENTAL);
            }

            rrddim_set(cg->st_io, "read", (collected_number)cg->io_service_bytes.Read);
            rrddim_set(cg->st_io, "write", (collected_number)cg->io_service_bytes.Write);
            rrdset_done(cg->st_io);
        }

        if(likely(cg->io_serviced.updated && cg->io_serviced.enabled == CONFIG_BOOLEAN_YES)) {
            if(unlikely(!cg->st_serviced_ops)) {
                snprintfz(title, CHART_TITLE_MAX, "Serviced I/O Operations (all disks)");

                cg->st_serviced_ops = rrdset_create_localhost(
                          cgroup_chart_type(type, cg)
                        , "serviced_ops"
                        , NULL
                        , "disk"
                        , k8s_is_kubepod(cg) ? "k8s.cgroup.serviced_ops" : "cgroup.serviced_ops"
                        , title
                        , "operations/s"
                        , PLUGIN_CGROUPS_NAME
                        , PLUGIN_CGROUPS_MODULE_CGROUPS_NAME
                        , cgroup_containers_chart_priority + 1200
                        , update_every
                        , RRDSET_TYPE_LINE
                );

                rrdset_update_rrdlabels(cg->st_serviced_ops, cg->chart_labels);

                rrddim_add(cg->st_serviced_ops, "read", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                rrddim_add(cg->st_serviced_ops, "write", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            }

            rrddim_set(cg->st_serviced_ops, "read", (collected_number)cg->io_serviced.Read);
            rrddim_set(cg->st_serviced_ops, "write", (collected_number)cg->io_serviced.Write);
            rrdset_done(cg->st_serviced_ops);
        }

        if(likely(cg->throttle_io_service_bytes.updated && cg->throttle_io_service_bytes.enabled == CONFIG_BOOLEAN_YES)) {
            if(unlikely(!cg->st_throttle_io)) {
                snprintfz(title, CHART_TITLE_MAX, "Throttle I/O Bandwidth (all disks)");

                cg->st_throttle_io = rrdset_create_localhost(
                          cgroup_chart_type(type, cg)
                        , "throttle_io"
                        , NULL
                        , "disk"
                        , k8s_is_kubepod(cg) ? "k8s.cgroup.throttle_io" : "cgroup.throttle_io"
                        , title
                        , "KiB/s"
                        , PLUGIN_CGROUPS_NAME
                        , PLUGIN_CGROUPS_MODULE_CGROUPS_NAME
                        , cgroup_containers_chart_priority + 1200
                        , update_every
                        , RRDSET_TYPE_AREA
                );

                rrdset_update_rrdlabels(cg->st_throttle_io, cg->chart_labels);

                rrddim_add(cg->st_throttle_io, "read", NULL, 1, 1024, RRD_ALGORITHM_INCREMENTAL);
                rrddim_add(cg->st_throttle_io, "write", NULL, -1, 1024, RRD_ALGORITHM_INCREMENTAL);
            }

            rrddim_set(cg->st_throttle_io, "read", (collected_number)cg->throttle_io_service_bytes.Read);
            rrddim_set(cg->st_throttle_io, "write", (collected_number)cg->throttle_io_service_bytes.Write);
            rrdset_done(cg->st_throttle_io);
        }

        if(likely(cg->throttle_io_serviced.updated && cg->throttle_io_serviced.enabled == CONFIG_BOOLEAN_YES)) {
            if(unlikely(!cg->st_throttle_serviced_ops)) {
                snprintfz(title, CHART_TITLE_MAX, "Throttle Serviced I/O Operations (all disks)");

                cg->st_throttle_serviced_ops = rrdset_create_localhost(
                          cgroup_chart_type(type, cg)
                        , "throttle_serviced_ops"
                        , NULL
                        , "disk"
                        , k8s_is_kubepod(cg) ? "k8s.cgroup.throttle_serviced_ops" : "cgroup.throttle_serviced_ops"
                        , title
                        , "operations/s"
                        , PLUGIN_CGROUPS_NAME
                        , PLUGIN_CGROUPS_MODULE_CGROUPS_NAME
                        , cgroup_containers_chart_priority + 1200
                        , update_every
                        , RRDSET_TYPE_LINE
                );

                rrdset_update_rrdlabels(cg->st_throttle_serviced_ops, cg->chart_labels);

                rrddim_add(cg->st_throttle_serviced_ops, "read", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                rrddim_add(cg->st_throttle_serviced_ops, "write", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            }

            rrddim_set(cg->st_throttle_serviced_ops, "read", (collected_number)cg->throttle_io_serviced.Read);
            rrddim_set(cg->st_throttle_serviced_ops, "write", (collected_number)cg->throttle_io_serviced.Write);
            rrdset_done(cg->st_throttle_serviced_ops);
        }

        if(likely(cg->io_queued.updated && cg->io_queued.enabled == CONFIG_BOOLEAN_YES)) {
            if(unlikely(!cg->st_queued_ops)) {
                snprintfz(title, CHART_TITLE_MAX, "Queued I/O Operations (all disks)");

                cg->st_queued_ops = rrdset_create_localhost(
                          cgroup_chart_type(type, cg)
                        , "queued_ops"
                        , NULL
                        , "disk"
                        , k8s_is_kubepod(cg) ? "k8s.cgroup.queued_ops" : "cgroup.queued_ops"
                        , title
                        , "operations"
                        , PLUGIN_CGROUPS_NAME
                        , PLUGIN_CGROUPS_MODULE_CGROUPS_NAME
                        , cgroup_containers_chart_priority + 2000
                        , update_every
                        , RRDSET_TYPE_LINE
                );

                rrdset_update_rrdlabels(cg->st_queued_ops, cg->chart_labels);

                rrddim_add(cg->st_queued_ops, "read", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                rrddim_add(cg->st_queued_ops, "write", NULL, -1, 1, RRD_ALGORITHM_ABSOLUTE);
            }

            rrddim_set(cg->st_queued_ops, "read", (collected_number)cg->io_queued.Read);
            rrddim_set(cg->st_queued_ops, "write", (collected_number)cg->io_queued.Write);
            rrdset_done(cg->st_queued_ops);
        }

        if(likely(cg->io_merged.updated && cg->io_merged.enabled == CONFIG_BOOLEAN_YES)) {
            if(unlikely(!cg->st_merged_ops)) {
                snprintfz(title, CHART_TITLE_MAX, "Merged I/O Operations (all disks)");

                cg->st_merged_ops = rrdset_create_localhost(
                          cgroup_chart_type(type, cg)
                        , "merged_ops"
                        , NULL
                        , "disk"
                        , k8s_is_kubepod(cg) ? "k8s.cgroup.merged_ops" : "cgroup.merged_ops"
                        , title
                        , "operations/s"
                        , PLUGIN_CGROUPS_NAME
                        , PLUGIN_CGROUPS_MODULE_CGROUPS_NAME
                        , cgroup_containers_chart_priority + 2100
                        , update_every
                        , RRDSET_TYPE_LINE
                );

                rrdset_update_rrdlabels(cg->st_merged_ops, cg->chart_labels);

                rrddim_add(cg->st_merged_ops, "read", NULL, 1, 1024, RRD_ALGORITHM_INCREMENTAL);
                rrddim_add(cg->st_merged_ops, "write", NULL, -1, 1024, RRD_ALGORITHM_INCREMENTAL);
            }

            rrddim_set(cg->st_merged_ops, "read", (collected_number)cg->io_merged.Read);
            rrddim_set(cg->st_merged_ops, "write", (collected_number)cg->io_merged.Write);
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
                          cgroup_chart_type(type, cg)
                        , "cpu_some_pressure"
                        , NULL
                        , "cpu"
                        , k8s_is_kubepod(cg) ? "k8s.cgroup.cpu_some_pressure" : "cgroup.cpu_some_pressure"
                        , title
                        , "percentage"
                        , PLUGIN_CGROUPS_NAME
                        , PLUGIN_CGROUPS_MODULE_CGROUPS_NAME
                        , cgroup_containers_chart_priority + 2200
                        , update_every
                        , RRDSET_TYPE_LINE
                    );
                    rrdset_update_rrdlabels(chart = pcs->share_time.st, cg->chart_labels);
                    pcs->share_time.rd10 = rrddim_add(chart, "some 10", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
                    pcs->share_time.rd60 = rrddim_add(chart, "some 60", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
                    pcs->share_time.rd300 = rrddim_add(chart, "some 300", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
                }

                if (unlikely(!pcs->total_time.st)) {
                    RRDSET *chart;
                    snprintfz(title, CHART_TITLE_MAX, "CPU some pressure stall time");
                    chart = pcs->total_time.st = rrdset_create_localhost(
                          cgroup_chart_type(type, cg)
                        , "cpu_some_pressure_stall_time"
                        , NULL
                        , "cpu"
                        , k8s_is_kubepod(cg) ? "k8s.cgroup.cpu_some_pressure_stall_time" : "cgroup.cpu_some_pressure_stall_time"
                        , title
                        , "ms"
                        , PLUGIN_CGROUPS_NAME
                        , PLUGIN_CGROUPS_MODULE_CGROUPS_NAME
                        , cgroup_containers_chart_priority + 2220
                        , update_every
                        , RRDSET_TYPE_LINE
                    );
                    rrdset_update_rrdlabels(chart = pcs->total_time.st, cg->chart_labels);
                    pcs->total_time.rdtotal = rrddim_add(chart, "time", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
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
                          cgroup_chart_type(type, cg)
                        , "cpu_full_pressure"
                        , NULL
                        , "cpu"
                        , k8s_is_kubepod(cg) ? "k8s.cgroup.cpu_full_pressure" : "cgroup.cpu_full_pressure"
                        , title
                        , "percentage"
                        , PLUGIN_CGROUPS_NAME
                        , PLUGIN_CGROUPS_MODULE_CGROUPS_NAME
                        , cgroup_containers_chart_priority + 2240
                        , update_every
                        , RRDSET_TYPE_LINE
                    );
                    rrdset_update_rrdlabels(chart = pcs->share_time.st, cg->chart_labels);
                    pcs->share_time.rd10 = rrddim_add(chart, "full 10", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
                    pcs->share_time.rd60 = rrddim_add(chart, "full 60", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
                    pcs->share_time.rd300 = rrddim_add(chart, "full 300", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
                }

                if (unlikely(!pcs->total_time.st)) {
                    RRDSET *chart;
                    snprintfz(title, CHART_TITLE_MAX, "CPU full pressure stall time");
                    chart = pcs->total_time.st = rrdset_create_localhost(
                          cgroup_chart_type(type, cg)
                        , "cpu_full_pressure_stall_time"
                        , NULL
                        , "cpu"
                        , k8s_is_kubepod(cg) ? "k8s.cgroup.cpu_full_pressure_stall_time" : "cgroup.cpu_full_pressure_stall_time"
                        , title
                        , "ms"
                        , PLUGIN_CGROUPS_NAME
                        , PLUGIN_CGROUPS_MODULE_CGROUPS_NAME
                        , cgroup_containers_chart_priority + 2260
                        , update_every
                        , RRDSET_TYPE_LINE
                    );
                    rrdset_update_rrdlabels(chart = pcs->total_time.st, cg->chart_labels);
                    pcs->total_time.rdtotal = rrddim_add(chart, "time", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
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
                          cgroup_chart_type(type, cg)
                        , "mem_some_pressure"
                        , NULL
                        , "mem"
                        , k8s_is_kubepod(cg) ? "k8s.cgroup.memory_some_pressure" : "cgroup.memory_some_pressure"
                        , title
                        , "percentage"
                        , PLUGIN_CGROUPS_NAME
                        , PLUGIN_CGROUPS_MODULE_CGROUPS_NAME
                        , cgroup_containers_chart_priority + 2300
                        , update_every
                        , RRDSET_TYPE_LINE
                        );
                    rrdset_update_rrdlabels(chart = pcs->share_time.st, cg->chart_labels);
                    pcs->share_time.rd10 = rrddim_add(chart, "some 10", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
                    pcs->share_time.rd60 = rrddim_add(chart, "some 60", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
                    pcs->share_time.rd300 = rrddim_add(chart, "some 300", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
                }

                if (unlikely(!pcs->total_time.st)) {
                    RRDSET *chart;
                    snprintfz(title, CHART_TITLE_MAX, "Memory some pressure stall time");
                    chart = pcs->total_time.st = rrdset_create_localhost(
                          cgroup_chart_type(type, cg)
                        , "memory_some_pressure_stall_time"
                        , NULL
                        , "mem"
                        , k8s_is_kubepod(cg) ? "k8s.cgroup.memory_some_pressure_stall_time" : "cgroup.memory_some_pressure_stall_time"
                        , title
                        , "ms"
                        , PLUGIN_CGROUPS_NAME
                        , PLUGIN_CGROUPS_MODULE_CGROUPS_NAME
                        , cgroup_containers_chart_priority + 2320
                        , update_every
                        , RRDSET_TYPE_LINE
                    );
                    rrdset_update_rrdlabels(chart = pcs->total_time.st, cg->chart_labels);
                    pcs->total_time.rdtotal = rrddim_add(chart, "time", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
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
                          cgroup_chart_type(type, cg)
                        , "mem_full_pressure"
                        , NULL
                        , "mem"
                        , k8s_is_kubepod(cg) ? "k8s.cgroup.memory_full_pressure" : "cgroup.memory_full_pressure"
                        , title
                        , "percentage"
                        , PLUGIN_CGROUPS_NAME
                        , PLUGIN_CGROUPS_MODULE_CGROUPS_NAME
                        , cgroup_containers_chart_priority + 2340
                        , update_every
                        , RRDSET_TYPE_LINE
                        );

                    rrdset_update_rrdlabels(chart = pcs->share_time.st, cg->chart_labels);
                    pcs->share_time.rd10 = rrddim_add(chart, "full 10", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
                    pcs->share_time.rd60 = rrddim_add(chart, "full 60", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
                    pcs->share_time.rd300 = rrddim_add(chart, "full 300", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
                }

                if (unlikely(!pcs->total_time.st)) {
                    RRDSET *chart;
                    snprintfz(title, CHART_TITLE_MAX, "Memory full pressure stall time");
                    chart = pcs->total_time.st = rrdset_create_localhost(
                          cgroup_chart_type(type, cg)
                        , "memory_full_pressure_stall_time"
                        , NULL
                        , "mem"
                        , k8s_is_kubepod(cg) ? "k8s.cgroup.memory_full_pressure_stall_time" : "cgroup.memory_full_pressure_stall_time"
                        , title
                        , "ms"
                        , PLUGIN_CGROUPS_NAME
                        , PLUGIN_CGROUPS_MODULE_CGROUPS_NAME
                        , cgroup_containers_chart_priority + 2360
                        , update_every
                        , RRDSET_TYPE_LINE
                    );
                    rrdset_update_rrdlabels(chart = pcs->total_time.st, cg->chart_labels);
                    pcs->total_time.rdtotal = rrddim_add(chart, "time", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                }

                update_pressure_charts(pcs);
            }

            res = &cg->irq_pressure;

            if (likely(res->updated && res->some.enabled)) {
                struct pressure_charts *pcs;
                pcs = &res->some;

                if (unlikely(!pcs->share_time.st)) {
                    RRDSET *chart;
                    snprintfz(title, CHART_TITLE_MAX, "IRQ some pressure");
                    chart = pcs->share_time.st = rrdset_create_localhost(
                       cgroup_chart_type(type, cg)
                    , "irq_some_pressure"
                    , NULL
                    , "interrupts"
                    , k8s_is_kubepod(cg) ? "k8s.cgroup.irq_some_pressure" : "cgroup.irq_some_pressure"
                    , title
                    , "percentage"
                    , PLUGIN_CGROUPS_NAME
                    , PLUGIN_CGROUPS_MODULE_CGROUPS_NAME
                    , cgroup_containers_chart_priority + 2310
                    , update_every
                    , RRDSET_TYPE_LINE
                    );
                    rrdset_update_rrdlabels(chart = pcs->share_time.st, cg->chart_labels);
                    pcs->share_time.rd10 = rrddim_add(chart, "some 10", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
                    pcs->share_time.rd60 = rrddim_add(chart, "some 60", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
                    pcs->share_time.rd300 = rrddim_add(chart, "some 300", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
                }

                if (unlikely(!pcs->total_time.st)) {
                    RRDSET *chart;
                    snprintfz(title, CHART_TITLE_MAX, "IRQ some pressure stall time");
                    chart = pcs->total_time.st = rrdset_create_localhost(
                      cgroup_chart_type(type, cg)
                    , "irq_some_pressure_stall_time"
                    , NULL
                    , "interrupts"
                    , k8s_is_kubepod(cg) ? "k8s.cgroup.irq_some_pressure_stall_time" : "cgroup.irq_some_pressure_stall_time"
                    , title
                    , "ms"
                    , PLUGIN_CGROUPS_NAME
                    , PLUGIN_CGROUPS_MODULE_CGROUPS_NAME
                    , cgroup_containers_chart_priority + 2330
                    , update_every
                    , RRDSET_TYPE_LINE
                    );
                    rrdset_update_rrdlabels(chart = pcs->total_time.st, cg->chart_labels);
                    pcs->total_time.rdtotal = rrddim_add(chart, "time", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                }

                update_pressure_charts(pcs);
            }

            if (likely(res->updated && res->full.enabled)) {
                struct pressure_charts *pcs;
                pcs = &res->full;

                if (unlikely(!pcs->share_time.st)) {
                    RRDSET *chart;
                    snprintfz(title, CHART_TITLE_MAX, "IRQ full pressure");

                    chart = pcs->share_time.st = rrdset_create_localhost(
                      cgroup_chart_type(type, cg)
                    , "irq_full_pressure"
                    , NULL
                    , "interrupts"
                    , k8s_is_kubepod(cg) ? "k8s.cgroup.irq_full_pressure" : "cgroup.irq_full_pressure"
                    , title
                    , "percentage"
                    , PLUGIN_CGROUPS_NAME
                    , PLUGIN_CGROUPS_MODULE_CGROUPS_NAME
                    , cgroup_containers_chart_priority + 2350
                    , update_every
                    , RRDSET_TYPE_LINE
                    );

                    rrdset_update_rrdlabels(chart = pcs->share_time.st, cg->chart_labels);
                    pcs->share_time.rd10 = rrddim_add(chart, "full 10", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
                    pcs->share_time.rd60 = rrddim_add(chart, "full 60", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
                    pcs->share_time.rd300 = rrddim_add(chart, "full 300", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
                }

                if (unlikely(!pcs->total_time.st)) {
                    RRDSET *chart;
                    snprintfz(title, CHART_TITLE_MAX, "IRQ full pressure stall time");
                    chart = pcs->total_time.st = rrdset_create_localhost(
                      cgroup_chart_type(type, cg)
                    , "irq_full_pressure_stall_time"
                    , NULL
                    , "interrupts"
                    , k8s_is_kubepod(cg) ? "k8s.cgroup.irq_full_pressure_stall_time" : "cgroup.irq_full_pressure_stall_time"
                    , title
                    , "ms"
                    , PLUGIN_CGROUPS_NAME
                    , PLUGIN_CGROUPS_MODULE_CGROUPS_NAME
                    , cgroup_containers_chart_priority + 2370
                    , update_every
                    , RRDSET_TYPE_LINE
                    );
                    rrdset_update_rrdlabels(chart = pcs->total_time.st, cg->chart_labels);
                    pcs->total_time.rdtotal = rrddim_add(chart, "time", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
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
                          cgroup_chart_type(type, cg)
                        , "io_some_pressure"
                        , NULL
                        , "disk"
                        , k8s_is_kubepod(cg) ? "k8s.cgroup.io_some_pressure" : "cgroup.io_some_pressure"
                        , title
                        , "percentage"
                        , PLUGIN_CGROUPS_NAME
                        , PLUGIN_CGROUPS_MODULE_CGROUPS_NAME
                        , cgroup_containers_chart_priority + 2400
                        , update_every
                        , RRDSET_TYPE_LINE
                        );
                    rrdset_update_rrdlabels(chart = pcs->share_time.st, cg->chart_labels);
                    pcs->share_time.rd10 = rrddim_add(chart, "some 10", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
                    pcs->share_time.rd60 = rrddim_add(chart, "some 60", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
                    pcs->share_time.rd300 = rrddim_add(chart, "some 300", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
                }

                if (unlikely(!pcs->total_time.st)) {
                    RRDSET *chart;
                    snprintfz(title, CHART_TITLE_MAX, "I/O some pressure stall time");
                    chart = pcs->total_time.st = rrdset_create_localhost(
                          cgroup_chart_type(type, cg)
                        , "io_some_pressure_stall_time"
                        , NULL
                        , "disk"
                        , k8s_is_kubepod(cg) ? "k8s.cgroup.io_some_pressure_stall_time" : "cgroup.io_some_pressure_stall_time"
                        , title
                        , "ms"
                        , PLUGIN_CGROUPS_NAME
                        , PLUGIN_CGROUPS_MODULE_CGROUPS_NAME
                        , cgroup_containers_chart_priority + 2420
                        , update_every
                        , RRDSET_TYPE_LINE
                    );
                    rrdset_update_rrdlabels(chart = pcs->total_time.st, cg->chart_labels);
                    pcs->total_time.rdtotal = rrddim_add(chart, "time", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
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
                          cgroup_chart_type(type, cg)
                        , "io_full_pressure"
                        , NULL
                        , "disk"
                        , k8s_is_kubepod(cg) ? "k8s.cgroup.io_full_pressure" : "cgroup.io_full_pressure"
                        , title
                        , "percentage"
                        , PLUGIN_CGROUPS_NAME
                        , PLUGIN_CGROUPS_MODULE_CGROUPS_NAME
                        , cgroup_containers_chart_priority + 2440
                        , update_every
                        , RRDSET_TYPE_LINE
                        );
                    rrdset_update_rrdlabels(chart = pcs->share_time.st, cg->chart_labels);
                    pcs->share_time.rd10 = rrddim_add(chart, "full 10", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
                    pcs->share_time.rd60 = rrddim_add(chart, "full 60", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
                    pcs->share_time.rd300 = rrddim_add(chart, "full 300", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
                }

                if (unlikely(!pcs->total_time.st)) {
                    RRDSET *chart;
                    snprintfz(title, CHART_TITLE_MAX, "I/O full pressure stall time");
                    chart = pcs->total_time.st = rrdset_create_localhost(
                          cgroup_chart_type(type, cg)
                        , "io_full_pressure_stall_time"
                        , NULL
                        , "disk"
                        , k8s_is_kubepod(cg) ? "k8s.cgroup.io_full_pressure_stall_time" : "cgroup.io_full_pressure_stall_time"
                        , title
                        , "ms"
                        , PLUGIN_CGROUPS_NAME
                        , PLUGIN_CGROUPS_MODULE_CGROUPS_NAME
                        , cgroup_containers_chart_priority + 2460
                        , update_every
                        , RRDSET_TYPE_LINE
                    );
                    rrdset_update_rrdlabels(chart = pcs->total_time.st, cg->chart_labels);
                    pcs->total_time.rdtotal = rrddim_add(chart, "time", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
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

    netdata_log_debug(D_CGROUP, "done updating cgroups charts");
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
    rrd_collector_started(); // this creates a collector that runs for as long as netdata runs
    cgroup_netdev_link_init();
    rrd_function_add(localhost, NULL, "cgroups", 10,
            RRDFUNCTIONS_CGTOP_HELP, true,
            cgroup_function_cgroup_top, NULL);

    heartbeat_t hb;
    heartbeat_init(&hb);
    usec_t step = cgroup_update_every * USEC_PER_SEC;
    usec_t find_every = cgroup_check_for_new_every * USEC_PER_SEC, find_dt = 0;

    netdata_thread_disable_cancelability();
    while(service_running(SERVICE_COLLECTORS)) {
        worker_is_idle();

        usec_t hb_dt = heartbeat_next(&hb, step);
        if(unlikely(!service_running(SERVICE_COLLECTORS))) break;

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
        update_cgroup_charts(cgroup_update_every);
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
