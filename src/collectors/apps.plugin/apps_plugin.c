// SPDX-License-Identifier: GPL-3.0-or-later

#include "apps_plugin.h"
#include "libnetdata/required_dummies.h"
#include "libnetdata/parsers/duration.h"

#define APPS_PLUGIN_FUNCTIONS() do { \
    fprintf(stdout, PLUGINSD_KEYWORD_FUNCTION " \"processes\" %d \"%s\" \"top\" "HTTP_ACCESS_FORMAT" %d\n",         \
            PLUGINS_FUNCTIONS_TIMEOUT_DEFAULT, APPS_PLUGIN_PROCESSES_FUNCTION_DESCRIPTION,                          \
            (HTTP_ACCESS_FORMAT_CAST)(HTTP_ACCESS_SIGNED_ID|HTTP_ACCESS_SAME_SPACE|HTTP_ACCESS_SENSITIVE_DATA),     \
            RRDFUNCTIONS_PRIORITY_DEFAULT / 10);                                                                    \
} while(0)

#define APPS_PLUGIN_GLOBAL_FUNCTIONS() do { \
    fprintf(stdout, PLUGINSD_KEYWORD_FUNCTION " GLOBAL \"processes\" %d \"%s\" \"top\" "HTTP_ACCESS_FORMAT" %d\n",  \
            PLUGINS_FUNCTIONS_TIMEOUT_DEFAULT, APPS_PLUGIN_PROCESSES_FUNCTION_DESCRIPTION,                          \
            (HTTP_ACCESS_FORMAT_CAST)(HTTP_ACCESS_SIGNED_ID|HTTP_ACCESS_SAME_SPACE|HTTP_ACCESS_SENSITIVE_DATA),     \
            RRDFUNCTIONS_PRIORITY_DEFAULT / 10);                                                                    \
} while(0)

// ----------------------------------------------------------------------------
// options

bool debug_enabled = false;

bool enable_detailed_uptime_charts = false;
bool enable_users_charts = true;
bool enable_groups_charts = true;
bool include_exited_childs = true;
bool proc_pid_cmdline_is_needed = true; // true when we need to read /proc/cmdline

#if defined(OS_FREEBSD) || defined(OS_MACOS)
int enable_file_charts = CONFIG_BOOLEAN_NO;
#elif defined(OS_LINUX)
int enable_file_charts = CONFIG_BOOLEAN_AUTO;
#elif defined(OS_WINDOWS)
int enable_file_charts = CONFIG_BOOLEAN_YES;
#endif
bool obsolete_file_charts = false;

// ----------------------------------------------------------------------------
// internal counters

size_t
    global_iterations_counter = 1,
    calls_counter = 0,
    file_counter = 0,
    filenames_allocated_counter = 0,
    inodes_changed_counter = 0,
    links_changed_counter = 0,
    targets_assignment_counter = 0,
    apps_groups_targets_count = 0;       // # of apps_groups.conf targets

#if (PROCESSES_HAVE_CPU_GUEST_TIME == 1)
bool enable_guest_charts = false;
bool show_guest_time = false;            // set when guest values are collected
#endif

uint32_t
    all_files_len = 0,
    all_files_size = 0;

// --------------------------------------------------------------------------------------------------------------------
// Normalization
//
// With normalization we lower the collected metrics by a factor to make them
// match the total utilization of the system.
// The discrepancy exists because apps.plugin needs some time to collect all
// the metrics. This results in utilization that exceeds the total utilization
// of the system.
//
// During normalization, we align the per-process utilization to the global
// utilization of the system. We first consume the exited children utilization
// and it the collected values is above the total, we proportionally scale each
// reported metric.

// the total system time, as reported by /proc/stat
#if (ALL_PIDS_ARE_READ_INSTANTLY == 0)
kernel_uint_t
    global_utime = 0,
    global_stime = 0,
    global_gtime = 0;
#endif

// the normalization ratios, as calculated by normalize_utilization()
NETDATA_DOUBLE
    utime_fix_ratio = 1.0,
    stime_fix_ratio = 1.0,
    gtime_fix_ratio = 1.0,
    minflt_fix_ratio = 1.0,
    majflt_fix_ratio = 1.0,
    cutime_fix_ratio = 1.0,
    cstime_fix_ratio = 1.0,
    cgtime_fix_ratio = 1.0,
    cminflt_fix_ratio = 1.0,
    cmajflt_fix_ratio = 1.0;

// --------------------------------------------------------------------------------------------------------------------

int update_every = 1;

#if defined(OS_LINUX)
proc_state proc_state_count[PROC_STATUS_END];
const char *proc_states[] = {
    [PROC_STATUS_RUNNING] = "running",
    [PROC_STATUS_SLEEPING] = "sleeping_interruptible",
    [PROC_STATUS_SLEEPING_D] = "sleeping_uninterruptible",
    [PROC_STATUS_ZOMBIE] = "zombie",
    [PROC_STATUS_STOPPED] = "stopped",
};
#endif

// will be changed to getenv(NETDATA_USER_CONFIG_DIR) if it exists
static char *user_config_dir = CONFIG_DIR;
static char *stock_config_dir = LIBCONFIG_DIR;

size_t pagesize;

void sanitize_apps_plugin_chart_meta(char *buf) {
    external_plugins_sanitize(buf, buf, strlen(buf) + 1);
}

// ----------------------------------------------------------------------------
// update chart dimensions

// Helper function to count the number of processes in the linked list
int count_processes(struct pid_stat *root) {
    int count = 0;

    for(struct pid_stat *p = root; p ; p = p->next)
        if(p->updated) count++;

    return count;
}

// Comparator function to sort by pid
int compare_by_pid(const void *a, const void *b) {
    struct pid_stat *pa = *(struct pid_stat **)a;
    struct pid_stat *pb = *(struct pid_stat **)b;
    return ((int)pa->pid - (int)pb->pid);
}

// Function to print a process and its children recursively
void print_process_tree(struct pid_stat *root, struct pid_stat *parent, int depth, int total_processes) {
    // Allocate an array of pointers for processes with the given parent
    struct pid_stat **children = (struct pid_stat **)malloc(total_processes * sizeof(struct pid_stat *));
    int children_count = 0;

    // Populate the array with processes that have the given parent
    struct pid_stat *p = root;
    while (p != NULL) {
        if (p->updated && p->parent == parent) {
            children[children_count++] = p;
        }
        p = p->next;
    }

    // Sort the children array by pid
    qsort(children, children_count, sizeof(struct pid_stat *), compare_by_pid);

    // Print each child and recurse
    for (int i = 0; i < children_count; i++) {
        // Print the current process with indentation based on depth
        if (depth > 0) {
            for (int j = 0; j < (depth - 1) * 4; j++) {
                printf(" ");
            }
            printf(" \\_ ");
        }

#if (PROCESSES_HAVE_COMM_AND_NAME == 1)
        printf("[%d] %s (name: %s) [%s]: %s\n", children[i]->pid,
               string2str(children[i]->comm),
               string2str(children[i]->name),
               string2str(children[i]->target->name),
               string2str(children[i]->cmdline));
#else
        printf("[%d] orig: '%s' new: '%s' [target: %s]: cmdline: %s\n", children[i]->pid,
               string2str(children[i]->comm_orig),
               string2str(children[i]->comm),
               string2str(children[i]->target->name),
               string2str(children[i]->cmdline));
#endif

        // Recurse to print this child's children
        print_process_tree(root, children[i], depth + 1, total_processes);
    }

    // Free the allocated array
    free(children);
}

// Function to print the full hierarchy
void print_hierarchy(struct pid_stat *root) {
    // Count the total number of processes
    int total_processes = count_processes(root);

    // Start printing from processes with parent = NULL (i.e., root processes)
    print_process_tree(root, NULL, 0, total_processes);
}

// ----------------------------------------------------------------------------
// update chart dimensions

#if (ALL_PIDS_ARE_READ_INSTANTLY == 0)
static void normalize_utilization(struct target *root) {
    struct target *w;

    // children processing introduces spikes,
    // here we try to eliminate them by disabling children processing either
    // for specific dimensions or entirely.
    // of course, either way, we disable it just for a single iteration.

    kernel_uint_t max_time = os_get_system_cpus() * NSEC_PER_SEC;
    kernel_uint_t utime = 0, cutime = 0, stime = 0, cstime = 0, gtime = 0, cgtime = 0, minflt = 0, cminflt = 0, majflt = 0, cmajflt = 0;

    if(global_utime > max_time) global_utime = max_time;
    if(global_stime > max_time) global_stime = max_time;
    if(global_gtime > max_time) global_gtime = max_time;

    for(w = root; w ; w = w->next) {
        if(w->target || (!w->values[PDF_PROCESSES] && !w->exposed)) continue;

        utime   += w->values[PDF_UTIME];
        stime   += w->values[PDF_STIME];
        gtime   += w->values[PDF_GTIME];
        cutime  += w->values[PDF_CUTIME];
        cstime  += w->values[PDF_CSTIME];
        cgtime  += w->values[PDF_CGTIME];

        minflt  += w->values[PDF_MINFLT];
        majflt  += w->values[PDF_MAJFLT];
        cminflt += w->values[PDF_CMINFLT];
        cmajflt += w->values[PDF_CMAJFLT];
    }

    if(global_utime || global_stime || global_gtime) {
        if(global_utime + global_stime + global_gtime > utime + cutime + stime + cstime + gtime + cgtime) {
            // everything we collected fits
            utime_fix_ratio  =
            stime_fix_ratio  =
            gtime_fix_ratio  =
            cutime_fix_ratio =
            cstime_fix_ratio =
            cgtime_fix_ratio = 1.0; //(NETDATA_DOUBLE)(global_utime + global_stime) / (NETDATA_DOUBLE)(utime + cutime + stime + cstime);
        }
        else if((global_utime + global_stime > utime + stime) && (cutime || cstime)) {
            // children resources are too high,
            // lower only the children resources
            utime_fix_ratio  =
            stime_fix_ratio  =
            gtime_fix_ratio  = 1.0;
            cutime_fix_ratio =
            cstime_fix_ratio =
            cgtime_fix_ratio = (NETDATA_DOUBLE)((global_utime + global_stime) - (utime + stime)) / (NETDATA_DOUBLE)(cutime + cstime);
        }
        else if(utime || stime) {
            // even running processes are unrealistic
            // zero the children resources
            // lower the running processes resources
            utime_fix_ratio  =
            stime_fix_ratio  =
            gtime_fix_ratio  = (NETDATA_DOUBLE)(global_utime + global_stime) / (NETDATA_DOUBLE)(utime + stime);
            cutime_fix_ratio =
            cstime_fix_ratio =
            cgtime_fix_ratio = 0.0;
        }
        else {
            utime_fix_ratio  =
            stime_fix_ratio  =
            gtime_fix_ratio  =
            cutime_fix_ratio =
            cstime_fix_ratio =
            cgtime_fix_ratio = 0.0;
        }
    }
    else {
        utime_fix_ratio  =
        stime_fix_ratio  =
        gtime_fix_ratio  =
        cutime_fix_ratio =
        cstime_fix_ratio =
        cgtime_fix_ratio = 0.0;
    }

    if(utime_fix_ratio  > 1.0) utime_fix_ratio  = 1.0;
    if(cutime_fix_ratio > 1.0) cutime_fix_ratio = 1.0;
    if(stime_fix_ratio  > 1.0) stime_fix_ratio  = 1.0;
    if(cstime_fix_ratio > 1.0) cstime_fix_ratio = 1.0;
    if(gtime_fix_ratio  > 1.0) gtime_fix_ratio  = 1.0;
    if(cgtime_fix_ratio > 1.0) cgtime_fix_ratio = 1.0;

    // if(utime_fix_ratio  < 0.0) utime_fix_ratio  = 0.0;
    // if(cutime_fix_ratio < 0.0) cutime_fix_ratio = 0.0;
    // if(stime_fix_ratio  < 0.0) stime_fix_ratio  = 0.0;
    // if(cstime_fix_ratio < 0.0) cstime_fix_ratio = 0.0;
    // if(gtime_fix_ratio  < 0.0) gtime_fix_ratio  = 0.0;
    // if(cgtime_fix_ratio < 0.0) cgtime_fix_ratio = 0.0;

    // TODO
    // we use cpu time to normalize page faults
    // the problem is that to find the proper max values
    // for page faults we have to parse /proc/vmstat
    // which is quite big to do it again (netdata does it already)
    //
    // a better solution could be to somehow have netdata
    // do this normalization for us

    if(utime || stime || gtime)
        majflt_fix_ratio =
        minflt_fix_ratio = (NETDATA_DOUBLE)(utime * utime_fix_ratio + stime * stime_fix_ratio + gtime * gtime_fix_ratio) / (NETDATA_DOUBLE)(utime + stime + gtime);
    else
        minflt_fix_ratio =
        majflt_fix_ratio = 1.0;

    if(cutime || cstime || cgtime)
        cmajflt_fix_ratio =
        cminflt_fix_ratio = (NETDATA_DOUBLE)(cutime * cutime_fix_ratio + cstime * cstime_fix_ratio + cgtime * cgtime_fix_ratio) / (NETDATA_DOUBLE)(cutime + cstime + cgtime);
    else
        cminflt_fix_ratio =
        cmajflt_fix_ratio = 1.0;

    // the report

    debug_log(
            "SYSTEM: u=" KERNEL_UINT_FORMAT " s=" KERNEL_UINT_FORMAT " g=" KERNEL_UINT_FORMAT " "
            "COLLECTED: u=" KERNEL_UINT_FORMAT " s=" KERNEL_UINT_FORMAT " g=" KERNEL_UINT_FORMAT " cu=" KERNEL_UINT_FORMAT " cs=" KERNEL_UINT_FORMAT " cg=" KERNEL_UINT_FORMAT " "
            "DELTA: u=" KERNEL_UINT_FORMAT " s=" KERNEL_UINT_FORMAT " g=" KERNEL_UINT_FORMAT " "
            "FIX: u=%0.2f s=%0.2f g=%0.2f cu=%0.2f cs=%0.2f cg=%0.2f "
            "FINALLY: u=" KERNEL_UINT_FORMAT " s=" KERNEL_UINT_FORMAT " g=" KERNEL_UINT_FORMAT " cu=" KERNEL_UINT_FORMAT " cs=" KERNEL_UINT_FORMAT " cg=" KERNEL_UINT_FORMAT " "
            , global_utime
            , global_stime
            , global_gtime
            , utime
            , stime
            , gtime
            , cutime
            , cstime
            , cgtime
            , utime + cutime - global_utime
            , stime + cstime - global_stime
            , gtime + cgtime - global_gtime
            , utime_fix_ratio
            , stime_fix_ratio
            , gtime_fix_ratio
            , cutime_fix_ratio
            , cstime_fix_ratio
            , cgtime_fix_ratio
            , (kernel_uint_t)(utime * utime_fix_ratio)
            , (kernel_uint_t)(stime * stime_fix_ratio)
            , (kernel_uint_t)(gtime * gtime_fix_ratio)
            , (kernel_uint_t)(cutime * cutime_fix_ratio)
            , (kernel_uint_t)(cstime * cstime_fix_ratio)
            , (kernel_uint_t)(cgtime * cgtime_fix_ratio)
    );
}
#endif

// ----------------------------------------------------------------------------
// parse command line arguments

int check_proc_1_io() {
    int ret = 0;

#if defined(OS_LINUX)
    procfile *ff = procfile_open("/proc/1/io", NULL, PROCFILE_FLAG_NO_ERROR_ON_FILE_IO);
    if(!ff) goto cleanup;

    ff = procfile_readall(ff);
    if(!ff) goto cleanup;

    ret = 1;

cleanup:
    procfile_close(ff);
#endif

    return ret;
}

static bool profile_speed = false;
static bool print_tree_and_exit = false;
#if (PROCESSES_HAVE_SMAPS_ROLLUP == 1)
int pss_refresh_period = 0; // disabled by default
#endif

static void parse_args(int argc, char **argv)
{
    int i, freq = 0;

    for(i = 1; i < argc; i++) {
        if(!freq) {
            int n = (int)str2l(argv[i]);
            if(n > 0) {
                freq = n;
                continue;
            }
        }

        if(strcmp("version", argv[i]) == 0 || strcmp("-version", argv[i]) == 0 || strcmp("--version", argv[i]) == 0 || strcmp("-v", argv[i]) == 0 || strcmp("-V", argv[i]) == 0) {
            printf("apps.plugin %s\n", NETDATA_VERSION);
            exit(0);
        }

        if(strcmp("print", argv[i]) == 0 || strcmp("-print", argv[i]) == 0 || strcmp("--print", argv[i]) == 0) {
            print_tree_and_exit = true;
            continue;
        }

#if defined(OS_LINUX)
        if(strcmp("test-permissions", argv[i]) == 0 || strcmp("-t", argv[i]) == 0) {
            if(!check_proc_1_io()) {
                perror("Tried to read /proc/1/io and it failed");
                exit(1);
            }
            printf("OK\n");
            exit(0);
        }
#endif

        if(strcmp("debug", argv[i]) == 0) {
            debug_enabled = true;
#ifndef NETDATA_INTERNAL_CHECKS
            fprintf(stderr, "apps.plugin has been compiled without debugging\n");
#endif
            continue;
        }

        if(strcmp("profile-speed", argv[i]) == 0) {
            profile_speed = true;
            continue;
        }

#if defined(OS_LINUX)
        if(strcmp("fds-cache-secs", argv[i]) == 0) {
            if(argc <= i + 1) {
                fprintf(stderr, "Parameter 'fds-cache-secs' requires a number as argument.\n");
                exit(1);
            }
            i++;
            max_fds_cache_seconds = str2i(argv[i]);
            if(max_fds_cache_seconds < 0) max_fds_cache_seconds = 0;
            continue;
        }

#if (PROCESSES_HAVE_SMAPS_ROLLUP == 1)
        if(strcmp("--pss", argv[i]) == 0) {
            if(argc <= i + 1) {
                fprintf(stderr, "Parameter '--pss' requires a duration (e.g. 5m, 300s) or 'off'.\n");
                exit(1);
            }
            i++;
            int64_t seconds = 0;
            if(!duration_parse(argv[i], &seconds, "s", "s")) {
                fprintf(stderr, "Cannot parse '--pss' value '%s'.\n", argv[i]);
                exit(1);
            }
            if(seconds <= 0) {
                pss_refresh_period = 0; // disabled
            }
            else {
                pss_refresh_period = (int)seconds;
                if(pss_refresh_period < 1)
                    pss_refresh_period = 1;
            }
            continue;
        }
#endif
#endif

#if (PROCESSES_HAVE_CPU_CHILDREN_TIME == 1) || (PROCESSES_HAVE_CHILDREN_FLTS == 1)
        if(strcmp("no-childs", argv[i]) == 0 || strcmp("without-childs", argv[i]) == 0) {
            include_exited_childs = 0;
            continue;
        }

        if(strcmp("with-childs", argv[i]) == 0) {
            include_exited_childs = 1;
            continue;
        }
#endif

#if (PROCESSES_HAVE_CPU_GUEST_TIME == 1)
        if(strcmp("with-guest", argv[i]) == 0) {
            enable_guest_charts = true;
            continue;
        }

        if(strcmp("no-guest", argv[i]) == 0 || strcmp("without-guest", argv[i]) == 0) {
            enable_guest_charts = false;
            continue;
        }
#endif

#if (PROCESSES_HAVE_FDS == 1)
        if(strcmp("with-files", argv[i]) == 0) {
            enable_file_charts = CONFIG_BOOLEAN_YES;
            continue;
        }

        if(strcmp("no-files", argv[i]) == 0 || strcmp("without-files", argv[i]) == 0) {
            enable_file_charts = CONFIG_BOOLEAN_NO;
            continue;
        }
#endif

#if (PROCESSES_HAVE_UID == 1) || (PROCESSES_HAVE_SID == 1)
        if(strcmp("no-users", argv[i]) == 0 || strcmp("without-users", argv[i]) == 0) {
            enable_users_charts = 0;
            continue;
        }
#endif

#if (PROCESSES_HAVE_GID == 1)
        if(strcmp("no-groups", argv[i]) == 0 || strcmp("without-groups", argv[i]) == 0) {
            enable_groups_charts = 0;
            continue;
        }
#endif

        if(strcmp("with-detailed-uptime", argv[i]) == 0) {
            enable_detailed_uptime_charts = 1;
            continue;
        }
        if(strcmp("with-function-cmdline", argv[i]) == 0) {
            enable_function_cmdline = true;
            continue;
        }

        if(strcmp("-h", argv[i]) == 0 || strcmp("--help", argv[i]) == 0) {
            fprintf(stderr,
                    "\n"
                    " netdata apps.plugin %s\n"
                    " Copyright 2018-2025 Netdata Inc.\n"
                    " Released under GNU General Public License v3 or later.\n"
                    " All rights reserved.\n"
                    "\n"
                    " This program is a data collector plugin for netdata.\n"
                    "\n"
                    " Available command line options:\n"
                    "\n"
                    " SECONDS                set the data collection frequency\n"
                    "\n"
                    " debug                  enable debugging (lot of output)\n"
                    "\n"
                    " with-function-cmdline  enable reporting the complete command line for processes\n"
                    "                        it includes the command and passed arguments\n"
                    "                        it may include sensitive data such as passwords and tokens\n"
                    "                        enabling this could be a security risk\n"
                    "\n"
#if (PROCESSES_HAVE_CPU_CHILDREN_TIME == 1) || (PROCESSES_HAVE_CHILDREN_FLTS == 1)
                    " with-childs\n"
                    " without-childs         enable / disable aggregating exited\n"
                    "                        children resources into parents\n"
                    "                        (default is enabled)\n"
                    "\n"
#endif
#if (PROCESSES_HAVE_CPU_GUEST_TIME == 1)
                    " with-guest\n"
                    " without-guest          enable / disable reporting guest charts\n"
                    "                        (default is disabled)\n"
                    "\n"
#endif
#if (PROCESSES_HAVE_FDS == 1)
                    " with-files\n"
                    " without-files          enable / disable reporting files, sockets, pipes\n"
                    "                        (default is enabled)\n"
                    "\n"
#endif
#if (PROCESSES_HAVE_UID == 1) || (PROCESSES_HAVE_SID == 1)
                    " without-users          disable reporting per user charts\n"
                    "\n"
#endif
#if (PROCESSES_HAVE_GID == 1)
                    " without-groups         disable reporting per user group charts\n"
                    "\n"
#endif
                    " with-detailed-uptime   enable reporting min/avg/max uptime charts\n"
                    "\n"
#if defined(OS_LINUX)
                    " fds-cache-secs N       cache the files of processed for N seconds\n"
                    "                        caching is adaptive per file (when a file\n"
                    "                        is found, it starts at 0 and while the file\n"
                    "                        remains open, it is incremented up to the\n"
                    "                        max given)\n"
                    "                        (default is %d seconds)\n"
                    "\n"
#if (PROCESSES_HAVE_SMAPS_ROLLUP == 1)
                    " --pss TIME            enable estimated memory using PSS sampling at the given interval\n"
                    "                        (e.g. 5m, 300s). Use 'off' or '0' to disable.\n"
                    "                        (default is off)\n"
                    "\n"
#endif
#endif
                    " version or -v or -V print program version and exit\n"
                    "\n"
                    , NETDATA_VERSION
#if defined(OS_LINUX)
                    , max_fds_cache_seconds
#endif
            );
            exit(0);
        }

#if !defined(OS_WINDOWS) || !defined(RUN_UNDER_CLION)
        netdata_log_error("Cannot understand option %s", argv[i]);
        exit(1);
#endif
    }

    if(freq > 0) update_every = freq;

    if(read_apps_groups_conf(user_config_dir, "groups")) {
        netdata_log_info("Cannot read process groups configuration file '%s/apps_groups.conf'. Will try '%s/apps_groups.conf'", user_config_dir, stock_config_dir);

        if(read_apps_groups_conf(stock_config_dir, "groups")) {
            netdata_log_error("Cannot read process groups '%s/apps_groups.conf'. There are no internal defaults. Failing.", stock_config_dir);
            exit(1);
        }
        else
            netdata_log_info("Loaded config file '%s/apps_groups.conf'", stock_config_dir);
    }
    else
        netdata_log_info("Loaded config file '%s/apps_groups.conf'", user_config_dir);
}

#if !defined(OS_WINDOWS)
static inline int am_i_running_as_root() {
    uid_t uid = getuid(), euid = geteuid();

    if(uid == 0 || euid == 0) {
        if(debug_enabled) netdata_log_info("I am running with escalated privileges, uid = %u, euid = %u.", uid, euid);
        return 1;
    }

    if(debug_enabled) netdata_log_info("I am not running with escalated privileges, uid = %u, euid = %u.", uid, euid);
    return 0;
}

#ifdef HAVE_CAPABILITY
static inline int check_capabilities() {
    cap_t caps = cap_get_proc();
    if(!caps) {
        netdata_log_error("Cannot get current capabilities.");
        return 0;
    }
    else if(debug_enabled)
        netdata_log_info("Received my capabilities from the system.");

    int ret = 1;

    cap_flag_value_t cfv = CAP_CLEAR;
    if(cap_get_flag(caps, CAP_DAC_READ_SEARCH, CAP_EFFECTIVE, &cfv) == -1) {
        netdata_log_error("Cannot find if CAP_DAC_READ_SEARCH is effective.");
        ret = 0;
    }
    else {
        if(cfv != CAP_SET) {
            netdata_log_error("apps.plugin should run with CAP_DAC_READ_SEARCH.");
            ret = 0;
        }
        else if(debug_enabled)
            netdata_log_info("apps.plugin runs with CAP_DAC_READ_SEARCH.");
    }

    cfv = CAP_CLEAR;
    if(cap_get_flag(caps, CAP_SYS_PTRACE, CAP_EFFECTIVE, &cfv) == -1) {
        netdata_log_error("Cannot find if CAP_SYS_PTRACE is effective.");
        ret = 0;
    }
    else {
        if(cfv != CAP_SET) {
            netdata_log_error("apps.plugin should run with CAP_SYS_PTRACE.");
            ret = 0;
        }
        else if(debug_enabled)
            netdata_log_info("apps.plugin runs with CAP_SYS_PTRACE.");
    }

    cap_free(caps);

    return ret;
}
#else
static inline int check_capabilities() {
    return 0;
}
#endif
#endif

netdata_mutex_t apps_and_stdout_mutex;

static void __attribute__((constructor)) init_mutex(void) {
    netdata_mutex_init(&apps_and_stdout_mutex);
}

static void __attribute__((destructor)) destroy_mutex(void) {
    netdata_mutex_destroy(&apps_and_stdout_mutex);
}

static bool apps_plugin_exit = false;

int main(int argc, char **argv) {
    nd_log_initialize_for_external_plugins("apps.plugin");
    netdata_threads_init_for_external_plugins(0);

    pagesize = (size_t)sysconf(_SC_PAGESIZE);

    bool send_resource_usage = true;
    {
        const char *s = getenv("NETDATA_INTERNALS_MONITORING");
        if(s && *s && strcmp(s, "NO") == 0)
            send_resource_usage = false;
    }

    // since apps.plugin runs as root, prevent it from opening symbolic links
    procfile_open_flags = O_RDONLY|O_NOFOLLOW;

    netdata_configured_host_prefix = getenv("NETDATA_HOST_PREFIX");
    if(verify_netdata_host_prefix(true) == -1) exit(1);

    user_config_dir = getenv("NETDATA_USER_CONFIG_DIR");
    if(user_config_dir == NULL) {
        // netdata_log_info("NETDATA_CONFIG_DIR is not passed from netdata");
        user_config_dir = CONFIG_DIR;
    }
    // else netdata_log_info("Found NETDATA_USER_CONFIG_DIR='%s'", user_config_dir);

    stock_config_dir = getenv("NETDATA_STOCK_CONFIG_DIR");
    if(stock_config_dir == NULL) {
        // netdata_log_info("NETDATA_CONFIG_DIR is not passed from netdata");
        stock_config_dir = LIBCONFIG_DIR;
    }
    // else netdata_log_info("Found NETDATA_USER_CONFIG_DIR='%s'", user_config_dir);

#ifdef NETDATA_INTERNAL_CHECKS
    if(debug_flags != 0) {
        struct rlimit rl = { RLIM_INFINITY, RLIM_INFINITY };
        if(setrlimit(RLIMIT_CORE, &rl) != 0)
            netdata_log_info("Cannot request unlimited core dumps for debugging... Proceeding anyway...");
#ifdef HAVE_SYS_PRCTL_H
        prctl(PR_SET_DUMPABLE, 1, 0, 0, 0);
#endif
    }
#endif /* NETDATA_INTERNAL_CHECKS */

    procfile_set_adaptive_allocation(true, 0, 0, 0);
    os_get_system_cpus_uncached();
    apps_managers_and_aggregators_init(); // before parsing args!
    parse_args(argc, argv);

#if !defined(OS_WINDOWS)
    if(!check_capabilities() && !am_i_running_as_root() && !check_proc_1_io()) {
        uid_t uid = getuid(), euid = geteuid();
#ifdef HAVE_CAPABILITY
        netdata_log_error("apps.plugin should either run as root (now running with uid %u, euid %u) or have special capabilities. "
                          "Without these, apps.plugin cannot report disk I/O utilization of other processes. "
                          "To enable capabilities run: sudo setcap cap_dac_read_search,cap_sys_ptrace+ep %s; "
                          "To enable setuid to root run: sudo chown root:netdata %s; sudo chmod 4750 %s; "
                          , uid, euid, argv[0], argv[0], argv[0]);
#else
        netdata_log_error("apps.plugin should either run as root (now running with uid %u, euid %u) or have special capabilities. "
                          "Without these, apps.plugin cannot report disk I/O utilization of other processes. "
                          "Your system does not support capabilities. "
                          "To enable setuid to root run: sudo chown root:netdata %s; sudo chmod 4750 %s; "
                          , uid, euid, argv[0], argv[0]);
#endif
    }
#endif

    netdata_log_info("started on pid %d", getpid());

#if (PROCESSES_HAVE_UID == 1)
    cached_usernames_init();
#endif

#if (PROCESSES_HAVE_GID == 1)
    cached_groupnames_init();
#endif

#if (PROCESSES_HAVE_SID == 1)
    cached_sid_username_init();
#endif

    apps_pids_init();
    OS_FUNCTION(apps_os_init)();

    // ------------------------------------------------------------------------
    // the event loop for functions

    struct functions_evloop_globals *wg =
            functions_evloop_init(1, "APPS", &apps_and_stdout_mutex, &apps_plugin_exit);

    functions_evloop_add_function(wg, "processes", function_processes, PLUGINS_FUNCTIONS_TIMEOUT_DEFAULT, NULL);

    // ------------------------------------------------------------------------

    netdata_mutex_lock(&apps_and_stdout_mutex);
    APPS_PLUGIN_GLOBAL_FUNCTIONS();

    global_iterations_counter = 1;
    heartbeat_t hb;
    heartbeat_init(&hb, update_every * USEC_PER_SEC);
    for(; !apps_plugin_exit ; global_iterations_counter++) {
        netdata_mutex_unlock(&apps_and_stdout_mutex);

        usec_t dt;
        if(profile_speed) {
            static int profiling_count=0;
            profiling_count++;
            if(unlikely(profiling_count > 500)) exit(0);
            dt = update_every * USEC_PER_SEC;
        }
        else
            dt = heartbeat_next(&hb);

        netdata_mutex_lock(&apps_and_stdout_mutex);

        struct pollfd pollfd = { .fd = fileno(stdout), .events = POLLERR };
        if (unlikely(poll(&pollfd, 1, 0) < 0)) {
            netdata_mutex_unlock(&apps_and_stdout_mutex);
            fatal("Cannot check if a pipe is available");
        }
        if (unlikely(pollfd.revents & POLLERR)) {
            netdata_mutex_unlock(&apps_and_stdout_mutex);
            fatal("Received error on read pipe.");
        }

        if(!collect_data_for_all_pids()) {
            netdata_log_error("Cannot collect /proc data for running processes. Disabling apps.plugin...");
            printf("DISABLE\n");
            netdata_mutex_unlock(&apps_and_stdout_mutex);
            exit(1);
        }

        aggregate_processes_to_targets();

#if (ALL_PIDS_ARE_READ_INSTANTLY == 0)
        OS_FUNCTION(apps_os_read_global_cpu_utilization)();
        normalize_utilization(apps_groups_root_target);
#endif

        if(unlikely(print_tree_and_exit)) {
            print_hierarchy(root_of_pids());
            exit(0);
        }

        if(send_resource_usage)
            send_resource_usage_to_netdata(dt);

#if (PROCESSES_HAVE_STATE == 1)
        send_proc_states_count(dt);
#endif

        send_charts_updates_to_netdata(apps_groups_root_target, "app", "app_group", "Applications Groups");
        send_collected_data_to_netdata(apps_groups_root_target, "app", dt);

#if (PROCESSES_HAVE_UID == 1)
        if (enable_users_charts) {
            send_charts_updates_to_netdata(users_root_target, "user", "user", "User");
            send_collected_data_to_netdata(users_root_target, "user", dt);
        }
#endif

#if (PROCESSES_HAVE_GID == 1)
        if (enable_groups_charts) {
            send_charts_updates_to_netdata(groups_root_target, "usergroup", "user_group", "User Group");
            send_collected_data_to_netdata(groups_root_target, "usergroup", dt);
        }
#endif

#if (PROCESSES_HAVE_SID == 1)
        if (enable_users_charts) {
            send_charts_updates_to_netdata(sids_root_target, "user", "user", "User Processes");
            send_collected_data_to_netdata(sids_root_target, "user", dt);
        }
#endif

        fflush(stdout);

        debug_log("done Loop No %zu", global_iterations_counter);
    }
    netdata_mutex_unlock(&apps_and_stdout_mutex);
}
