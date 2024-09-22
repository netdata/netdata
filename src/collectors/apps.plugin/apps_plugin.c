// SPDX-License-Identifier: GPL-3.0-or-later

/*
 * netdata apps.plugin
 * (C) Copyright 2023 Netdata Inc.
 * Released under GPL v3+
 */

#include "apps_plugin.h"
#include "libnetdata/required_dummies.h"

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
bool enable_guest_charts = false;
bool enable_detailed_uptime_charts = false;
bool enable_users_charts = true;
bool enable_groups_charts = true;
bool include_exited_childs = true;
bool proc_pid_cmdline_is_needed = false; // true when we need to read /proc/cmdline

#if defined(__FreeBSD__) || defined(__APPLE__)
bool enable_file_charts = false;
#else
bool enable_file_charts = true;
#endif

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

int
    show_guest_time = 0,            // 1 when guest values are collected
    show_guest_time_old = 0;

uint32_t
    all_files_len = 0,
    all_files_size = 0;

#if defined(__FreeBSD__) || defined(__APPLE__)
usec_t system_current_time_ut;
#else
kernel_uint_t system_uptime_secs;
#endif

// ----------------------------------------------------------------------------
// Normalization
//
// With normalization we lower the collected metrics by a factor to make them
// match the total utilization of the system.
// The discrepancy exists because apps.plugin needs some time to collect all
// the metrics. This results in utilization that exceeds the total utilization
// of the system.
//
// During normalization, we align the per-process utilization, to the total of
// the system. We first consume the exited children utilization and it the
// collected values is above the total, we proportionally scale each reported
// metric.

// the total system time, as reported by /proc/stat
kernel_uint_t
    global_utime = 0,
    global_stime = 0,
    global_gtime = 0;

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

// ----------------------------------------------------------------------------
// factor for calculating correct CPU time values depending on units of raw data
unsigned int time_factor = 0;

// ----------------------------------------------------------------------------
// command line options

int update_every = 1;

#if defined(__APPLE__)
mach_timebase_info_data_t mach_info;
#endif

#if !defined(__FreeBSD__) && !defined(__APPLE__)
int max_fds_cache_seconds = 60;
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

struct target
        *apps_groups_default_target = NULL, // the default target
        *apps_groups_root_target = NULL,    // apps_groups.conf defined
        *users_root_target = NULL,          // users
        *groups_root_target = NULL,         // user groups
        *tree_root_target = NULL;

size_t pagesize;

// ----------------------------------------------------------------------------

int managed_log(struct pid_stat *p, PID_LOG log, int status) {
    if(unlikely(!status)) {
        // netdata_log_error("command failed log %u, errno %d", log, errno);

        if(unlikely(debug_enabled || errno != ENOENT)) {
            if(unlikely(debug_enabled || !(p->log_thrown & log))) {
                p->log_thrown |= log;
                switch(log) {
                    case PID_LOG_IO:
                        #if defined(__FreeBSD__) || defined(__APPLE__)
                        netdata_log_error("Cannot fetch process %d I/O info (command '%s')", p->pid, pid_stat_comm(p));
                        #else
                        netdata_log_error("Cannot process %s/proc/%d/io (command '%s')", netdata_configured_host_prefix, p->pid, pid_stat_comm(p));
                        #endif
                        break;

                    case PID_LOG_STATUS:
                        #if defined(__FreeBSD__) || defined(__APPLE__)
                        netdata_log_error("Cannot fetch process %d status info (command '%s')", p->pid, pid_stat_comm(p));
                        #else
                        netdata_log_error("Cannot process %s/proc/%d/status (command '%s')", netdata_configured_host_prefix, p->pid, pid_stat_comm(p));
                        #endif
                        break;

                    case PID_LOG_CMDLINE:
                        #if defined(__FreeBSD__) || defined(__APPLE__)
                        netdata_log_error("Cannot fetch process %d command line (command '%s')", p->pid, pid_stat_comm(p));
                        #else
                        netdata_log_error("Cannot process %s/proc/%d/cmdline (command '%s')", netdata_configured_host_prefix, p->pid, pid_stat_comm(p));
                        #endif
                        break;

                    case PID_LOG_FDS:
                        #if defined(__FreeBSD__) || defined(__APPLE__)
                        netdata_log_error("Cannot fetch process %d files (command '%s')", p->pid, pid_stat_comm(p));
                        #else
                        netdata_log_error("Cannot process entries in %s/proc/%d/fd (command '%s')", netdata_configured_host_prefix, p->pid, pid_stat_comm(p));
                        #endif
                        break;

                    case PID_LOG_LIMITS:
                        #if defined(__FreeBSD__) || defined(__APPLE__)
                        ;
                        #else
                        netdata_log_error("Cannot process %s/proc/%d/limits (command '%s')", netdata_configured_host_prefix, p->pid, pid_stat_comm(p));
                        #endif

                    case PID_LOG_STAT:
                        break;

                    default:
                        netdata_log_error("unhandled error for pid %d, command '%s'", p->pid, pid_stat_comm(p));
                        break;
                }
            }
        }
        errno_clear();
    }
    else if(unlikely(p->log_thrown & log)) {
        // netdata_log_error("unsetting log %u on pid %d", log, p->pid);
        p->log_thrown &= ~log;
    }

    return status;
}

// ----------------------------------------------------------------------------
// update statistics on the targets

// 1. link all childs to their parents
// 2. go from bottom to top, marking as merged all children to their parents,
//    this step links all parents without a target to the child target, if any
// 3. link all top level processes (the ones not merged) to default target
// 4. go from top to bottom, linking all children without a target to their parent target
//    after this step all processes have a target.
// [5. for each killed pid (updated = 0), remove its usage from its target]
// 6. zero all apps_groups_targets
// 7. concentrate all values on the apps_groups_targets
// 8. remove all killed processes
// 9. find the unique file count for each target
// check: update_apps_groups_statistics()

static void apply_apps_groups_targets_inheritance(void) {
    struct pid_stat *p = NULL;

    // children that do not have a target
    // inherit their target from their parent
    int found = 1, loops = 0;
    while(found) {
        if(unlikely(debug_enabled)) loops++;
        found = 0;
        for(p = root_of_pids(); p ; p = p->next) {
            // if this process does not have a target,
            // and it has a parent
            // and its parent has a target
            // then, set the parent's target to this process
            if(unlikely(!p->target && p->parent && p->parent->target)) {
                p->target = p->parent->target;
                found++;

                if(debug_enabled || (p->target && p->target->debug_enabled))
                    debug_log_int("TARGET INHERITANCE: %s is inherited by %d (%s) from its parent %d (%s).",
                                  string2str(p->target->name), p->pid, pid_stat_comm(p), p->parent->pid, pid_stat_comm(p->parent));
            }
        }
    }

    // find all the procs with 0 childs and merge them to their parents
    // repeat, until nothing more can be done.
    found = 1;
    while(found) {
        if(unlikely(debug_enabled)) loops++;
        found = 0;

        for(p = root_of_pids(); p ; p = p->next) {
            if(unlikely(
                    !p->children_count            // if this process does not have any children
                    && !p->merged                 // and is not already merged
                    && p->parent                  // and has a parent
                    && p->parent->children_count  // and its parent has children
                                                  // and the target of this process and its parent is the same,
                                                  // or the parent does not have a target
                    && (p->target == p->parent->target || !p->parent->target)
                    && p->ppid != INIT_PID        // and its parent is not init
                )) {
                // mark it as merged
                p->parent->children_count--;
                p->merged = true;

                // the parent inherits the child's target, if it does not have a target itself
                if(unlikely(p->target && !p->parent->target)) {
                    p->parent->target = p->target;

                    if(debug_enabled || (p->target && p->target->debug_enabled))
                        debug_log_int("TARGET INHERITANCE: %s is inherited by %d (%s) from its child %d (%s).",
                                      string2str(p->target->name), p->parent->pid, pid_stat_comm(p->parent), p->pid, pid_stat_comm(p));
                }

                found++;
            }
        }

        debug_log("TARGET INHERITANCE: merged %d processes", found);
    }

    // init goes always to default target
    struct pid_stat *pi = find_pid_entry(INIT_PID);
    if(pi && !pi->matched_by_config)
        pi->target = apps_groups_default_target;

    // pid 0 goes always to default target
    pi = find_pid_entry(0);
    if(pi && !pi->matched_by_config)
        pi->target = apps_groups_default_target;

    // give a default target on all top level processes
    if(unlikely(debug_enabled)) loops++;

    for(p = root_of_pids(); p ; p = p->next) {
        // if the process is not merged itself
        // then it is a top level process
        if(unlikely(!p->merged && !p->target))
            p->target = apps_groups_default_target;
    }

    // give a target to all merged child processes
    found = 1;
    while(found) {
        if(unlikely(debug_enabled)) loops++;
        found = 0;
        for(p = root_of_pids(); p ; p = p->next) {
            if(unlikely(!p->target && p->merged && p->parent && p->parent->target)) {
                p->target = p->parent->target;
                found++;

                if(debug_enabled || (p->target && p->target->debug_enabled))
                    debug_log_int("TARGET INHERITANCE: %s is inherited by %d (%s) from its parent %d (%s) at phase 2.",
                                  string2str(p->target->name), p->pid, pid_stat_comm(p), p->parent->pid, pid_stat_comm(p->parent));
            }
        }
    }

    debug_log("apply_apps_groups_targets_inheritance() made %d loops on the process tree", loops);
}

static size_t zero_all_targets(struct target *root) {
    struct target *w;
    size_t count = 0;

    for (w = root; w ; w = w->next) {
        count++;

        w->minflt = 0;
        w->majflt = 0;
        w->utime = 0;
        w->stime = 0;
        w->gtime = 0;
        w->cminflt = 0;
        w->cmajflt = 0;
        w->cutime = 0;
        w->cstime = 0;
        w->cgtime = 0;
        w->num_threads = 0;
        // w->rss = 0;
        w->processes = 0;

        w->status_vmsize = 0;
        w->status_vmrss = 0;
        w->status_vmshared = 0;
        w->status_rssfile = 0;
        w->status_rssshmem = 0;
        w->status_vmswap = 0;
        w->status_voluntary_ctxt_switches = 0;
        w->status_nonvoluntary_ctxt_switches = 0;

        w->io_logical_bytes_read = 0;
        w->io_logical_bytes_written = 0;
        w->io_read_calls = 0;
        w->io_write_calls = 0;
        w->io_storage_bytes_read = 0;
        w->io_storage_bytes_written = 0;
        w->io_cancelled_write_bytes = 0;

        // zero file counters
        if(w->target_fds) {
            memset(w->target_fds, 0, sizeof(int) * w->target_fds_size);
            w->openfds.files = 0;
            w->openfds.pipes = 0;
            w->openfds.sockets = 0;
            w->openfds.inotifies = 0;
            w->openfds.eventfds = 0;
            w->openfds.timerfds = 0;
            w->openfds.signalfds = 0;
            w->openfds.eventpolls = 0;
            w->openfds.other = 0;

            w->max_open_files_percent = 0.0;
        }

        w->uptime_min = 0;
        w->uptime_sum = 0;
        w->uptime_max = 0;

        if(unlikely(w->root_pid)) {
            struct pid_on_target *pid_on_target = w->root_pid;

            while(pid_on_target) {
                struct pid_on_target *pid_on_target_to_free = pid_on_target;
                pid_on_target = pid_on_target->next;
                freez(pid_on_target_to_free);
            }

            w->root_pid = NULL;
        }
    }

    return count;
}

static inline void aggregate_pid_on_target(struct target *w, struct pid_stat *p, struct target *o) {
    (void)o;

    if(unlikely(!p->updated)) {
        // the process is not running
        return;
    }

    if(unlikely(!w)) {
        netdata_log_error("pid %d %s was left without a target!", p->pid, pid_stat_comm(p));
        return;
    }

    if(p->openfds_limits_percent > w->max_open_files_percent)
        w->max_open_files_percent = p->openfds_limits_percent;

    w->cutime  += p->cutime;
    w->cstime  += p->cstime;
    w->cgtime  += p->cgtime;
    w->cminflt += p->cminflt;
    w->cmajflt += p->cmajflt;

    w->utime  += p->utime;
    w->stime  += p->stime;
    w->gtime  += p->gtime;
    w->minflt += p->minflt;
    w->majflt += p->majflt;

    // w->rss += p->rss;

    w->status_vmsize   += p->status_vmsize;
    w->status_vmrss    += p->status_vmrss;
    w->status_vmshared += p->status_vmshared;
    w->status_rssfile  += p->status_rssfile;
    w->status_rssshmem += p->status_rssshmem;
    w->status_vmswap   += p->status_vmswap;
    w->status_voluntary_ctxt_switches += p->status_voluntary_ctxt_switches;
    w->status_nonvoluntary_ctxt_switches += p->status_nonvoluntary_ctxt_switches;

    w->io_logical_bytes_read    += p->io_logical_bytes_read;
    w->io_logical_bytes_written += p->io_logical_bytes_written;
    w->io_read_calls            += p->io_read_calls;
    w->io_write_calls           += p->io_write_calls;
    w->io_storage_bytes_read    += p->io_storage_bytes_read;
    w->io_storage_bytes_written += p->io_storage_bytes_written;
    w->io_cancelled_write_bytes += p->io_cancelled_write_bytes;

    w->processes++;
    w->num_threads += p->num_threads;

    if(!w->uptime_min || p->uptime < w->uptime_min) w->uptime_min = p->uptime;
    if(!w->uptime_max || w->uptime_max < p->uptime) w->uptime_max = p->uptime;
    w->uptime_sum += p->uptime;

    if(unlikely(debug_enabled || w->debug_enabled)) {
        debug_log_int("aggregating '%s' pid %d on target '%s' utime=" KERNEL_UINT_FORMAT ", stime=" KERNEL_UINT_FORMAT ", gtime=" KERNEL_UINT_FORMAT ", cutime=" KERNEL_UINT_FORMAT ", cstime=" KERNEL_UINT_FORMAT ", cgtime=" KERNEL_UINT_FORMAT ", minflt=" KERNEL_UINT_FORMAT ", majflt=" KERNEL_UINT_FORMAT ", cminflt=" KERNEL_UINT_FORMAT ", cmajflt=" KERNEL_UINT_FORMAT "",
                      pid_stat_comm(p), p->pid, w->name, p->utime, p->stime, p->gtime, p->cutime, p->cstime, p->cgtime, p->minflt, p->majflt, p->cminflt, p->cmajflt);

        struct pid_on_target *pid_on_target = mallocz(sizeof(struct pid_on_target));
        pid_on_target->pid = p->pid;
        pid_on_target->next = w->root_pid;
        w->root_pid = pid_on_target;
    }
}

static void aggregate_processes_to_targets(void) {
    apply_apps_groups_targets_inheritance();

    zero_all_targets(tree_root_target);
    zero_all_targets(users_root_target);
    zero_all_targets(groups_root_target);
    apps_groups_targets_count = zero_all_targets(apps_groups_root_target);

    // this has to be done, before the cleanup
    struct pid_stat *p = NULL;
    struct target *w = NULL, *o = NULL;

    // concentrate everything on the targets
    for(p = root_of_pids(); p ; p = p->next) {

        // --------------------------------------------------------------------
        // apps_groups target

        aggregate_pid_on_target(p->target, p, NULL);


        // --------------------------------------------------------------------
        // user target

        o = p->uid_target;
        if(likely(p->uid_target && p->uid_target->uid == p->uid))
            w = p->uid_target;
        else {
            if(unlikely(debug_enabled && p->uid_target))
                debug_log("pid %d (%s) switched user from %u (%s) to %u.", p->pid, pid_stat_comm(p), p->uid_target->uid, p->uid_target->name, p->uid);

            w = p->uid_target = get_uid_target(p->uid);
        }

        aggregate_pid_on_target(w, p, o);


        // --------------------------------------------------------------------
        // user group target

        o = p->gid_target;
        if(likely(p->gid_target && p->gid_target->gid == p->gid))
            w = p->gid_target;
        else {
            if(unlikely(debug_enabled && p->gid_target))
                debug_log("pid %d (%s) switched group from %u (%s) to %u.", p->pid, pid_stat_comm(p), p->gid_target->gid, p->gid_target->name, p->gid);

            w = p->gid_target = get_gid_target(p->gid);
        }

        aggregate_pid_on_target(w, p, o);


        // --------------------------------------------------------------------
        // top target

        o = p->tree_target;
        if(likely(p->tree_target && p->tree_target->pid_comm == p->comm))
            w = p->tree_target;
        else {
            w = p->tree_target = get_tree_target(p);

            if(unlikely(debug_enabled && o))
                debug_log("pid %d (%s) switched top target from '%s' to '%s'.", p->pid, pid_stat_comm(p), string2str(o->pid_comm), string2str(p->gid_target->pid_comm));
        }

        aggregate_pid_on_target(w, p, o);


        // --------------------------------------------------------------------
        // aggregate all file descriptors

        if(enable_file_charts)
            aggregate_pid_fds_on_targets(p);
    }

    cleanup_exited_pids();
}

// ----------------------------------------------------------------------------
// update chart dimensions

static void normalize_utilization(struct target *root) {
    struct target *w;

    // childs processing introduces spikes
    // here we try to eliminate them by disabling childs processing either for specific dimensions
    // or entirely. Of course, either way, we disable it just a single iteration.

    kernel_uint_t max_time = os_get_system_cpus() * time_factor * RATES_DETAIL;
    kernel_uint_t utime = 0, cutime = 0, stime = 0, cstime = 0, gtime = 0, cgtime = 0, minflt = 0, cminflt = 0, majflt = 0, cmajflt = 0;

    if(global_utime > max_time) global_utime = max_time;
    if(global_stime > max_time) global_stime = max_time;
    if(global_gtime > max_time) global_gtime = max_time;

    for(w = root; w ; w = w->next) {
        if(w->target || (!w->processes && !w->exposed)) continue;

        utime   += w->utime;
        stime   += w->stime;
        gtime   += w->gtime;
        cutime  += w->cutime;
        cstime  += w->cstime;
        cgtime  += w->cgtime;

        minflt  += w->minflt;
        majflt  += w->majflt;
        cminflt += w->cminflt;
        cmajflt += w->cmajflt;
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
            // children resources are too high
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

// ----------------------------------------------------------------------------
// parse command line arguments

int check_proc_1_io() {
    int ret = 0;

    procfile *ff = procfile_open("/proc/1/io", NULL, PROCFILE_FLAG_NO_ERROR_ON_FILE_IO);
    if(!ff) goto cleanup;

    ff = procfile_readall(ff);
    if(!ff) goto cleanup;

    ret = 1;

cleanup:
    procfile_close(ff);
    return ret;
}

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

        if(strcmp("test-permissions", argv[i]) == 0 || strcmp("-t", argv[i]) == 0) {
            if(!check_proc_1_io()) {
                perror("Tried to read /proc/1/io and it failed");
                exit(1);
            }
            printf("OK\n");
            exit(0);
        }

        if(strcmp("debug", argv[i]) == 0) {
            debug_enabled = true;
#ifndef NETDATA_INTERNAL_CHECKS
            fprintf(stderr, "apps.plugin has been compiled without debugging\n");
#endif
            continue;
        }

#if !defined(__FreeBSD__) && !defined(__APPLE__)
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
#endif

        if(strcmp("no-childs", argv[i]) == 0 || strcmp("without-childs", argv[i]) == 0) {
            include_exited_childs = 0;
            continue;
        }

        if(strcmp("with-childs", argv[i]) == 0) {
            include_exited_childs = 1;
            continue;
        }

        if(strcmp("with-guest", argv[i]) == 0) {
            enable_guest_charts = true;
            continue;
        }

        if(strcmp("no-guest", argv[i]) == 0 || strcmp("without-guest", argv[i]) == 0) {
            enable_guest_charts = false;
            continue;
        }

        if(strcmp("with-files", argv[i]) == 0) {
            enable_file_charts = 1;
            continue;
        }

        if(strcmp("no-files", argv[i]) == 0 || strcmp("without-files", argv[i]) == 0) {
            enable_file_charts = 0;
            continue;
        }

        if(strcmp("no-users", argv[i]) == 0 || strcmp("without-users", argv[i]) == 0) {
            enable_users_charts = 0;
            continue;
        }

        if(strcmp("no-groups", argv[i]) == 0 || strcmp("without-groups", argv[i]) == 0) {
            enable_groups_charts = 0;
            continue;
        }

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
                    " Copyright (C) 2016-2017 Costa Tsaousis <costa@tsaousis.gr>\n"
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
                    " with-childs\n"
                    " without-childs         enable / disable aggregating exited\n"
                    "                        children resources into parents\n"
                    "                        (default is enabled)\n"
                    "\n"
                    " with-guest\n"
                    " without-guest          enable / disable reporting guest charts\n"
                    "                        (default is disabled)\n"
                    "\n"
                    " with-files\n"
                    " without-files          enable / disable reporting files, sockets, pipes\n"
                    "                        (default is enabled)\n"
                    "\n"
                    " without-users          disable reporting per user charts\n"
                    "\n"
                    " without-groups         disable reporting per user group charts\n"
                    "\n"
                    " with-detailed-uptime   enable reporting min/avg/max uptime charts\n"
                    "\n"
#if !defined(__FreeBSD__) && !defined(__APPLE__)
                    " fds-cache-secs N       cache the files of processed for N seconds\n"
                    "                        caching is adaptive per file (when a file\n"
                    "                        is found, it starts at 0 and while the file\n"
                    "                        remains open, it is incremented up to the\n"
                    "                        max given)\n"
                    "                        (default is %d seconds)\n"
                    "\n"
#endif
                    " version or -v or -V print program version and exit\n"
                    "\n"
                    , NETDATA_VERSION
#if !defined(__FreeBSD__) && !defined(__APPLE__)
                    , max_fds_cache_seconds
#endif
            );
            exit(1);
        }

        netdata_log_error("Cannot understand option %s", argv[i]);
        exit(1);
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

static int am_i_running_as_root() {
    uid_t uid = getuid(), euid = geteuid();

    if(uid == 0 || euid == 0) {
        if(debug_enabled) netdata_log_info("I am running with escalated privileges, uid = %u, euid = %u.", uid, euid);
        return 1;
    }

    if(debug_enabled) netdata_log_info("I am not running with escalated privileges, uid = %u, euid = %u.", uid, euid);
    return 0;
}

#ifdef HAVE_SYS_CAPABILITY_H
static int check_capabilities() {
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
static int check_capabilities() {
    return 0;
}
#endif

static netdata_mutex_t apps_and_stdout_mutex = NETDATA_MUTEX_INITIALIZER;

static bool apps_plugin_exit = false;

int main(int argc, char **argv) {
    clocks_init();
    nd_log_initialize_for_external_plugins("apps.plugin");

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

    procfile_adaptive_initial_allocation = 1;

    os_get_system_HZ();
#if defined(__FreeBSD__)
    time_factor = 1000000ULL / RATES_DETAIL; // FreeBSD uses usecs
#endif
#if defined(__APPLE__)
    mach_timebase_info(&mach_info);
    time_factor = 1000000ULL / RATES_DETAIL;
#endif
#if !defined(__FreeBSD__) && !defined(__APPLE__)
    time_factor = system_hz; // Linux uses clock ticks
#endif

    os_get_system_pid_max();
    os_get_system_cpus_uncached();

    parse_args(argc, argv);

    if(!check_capabilities() && !am_i_running_as_root() && !check_proc_1_io()) {
        uid_t uid = getuid(), euid = geteuid();
#ifdef HAVE_SYS_CAPABILITY_H
        netdata_log_error("apps.plugin should either run as root (now running with uid %u, euid %u) or have special capabilities. "
                      "Without these, apps.plugin cannot report disk I/O utilization of other processes. "
                      "To enable capabilities run: sudo setcap cap_dac_read_search,cap_sys_ptrace+ep %s; "
                      "To enable setuid to root run: sudo chown root:netdata %s; sudo chmod 4750 %s; "
              , uid, euid, argv[0], argv[0], argv[0]
        );
#else
        netdata_log_error("apps.plugin should either run as root (now running with uid %u, euid %u) or have special capabilities. "
                      "Without these, apps.plugin cannot report disk I/O utilization of other processes. "
                      "Your system does not support capabilities. "
                      "To enable setuid to root run: sudo chown root:netdata %s; sudo chmod 4750 %s; "
              , uid, euid, argv[0], argv[0]
        );
#endif
    }

    netdata_log_info("started on pid %d", getpid());

    users_and_groups_init();
    pids_init();

    // ------------------------------------------------------------------------
    // the event loop for functions

    struct functions_evloop_globals *wg =
            functions_evloop_init(1, "APPS", &apps_and_stdout_mutex, &apps_plugin_exit);

    functions_evloop_add_function(wg, "processes", function_processes, PLUGINS_FUNCTIONS_TIMEOUT_DEFAULT, NULL);

    // ------------------------------------------------------------------------

    netdata_mutex_lock(&apps_and_stdout_mutex);
    APPS_PLUGIN_GLOBAL_FUNCTIONS();

    usec_t step = update_every * USEC_PER_SEC;
    global_iterations_counter = 1;
    heartbeat_t hb;
    heartbeat_init(&hb);
    for(; !apps_plugin_exit ; global_iterations_counter++) {
        netdata_mutex_unlock(&apps_and_stdout_mutex);

#ifdef NETDATA_PROFILING
#warning "compiling for profiling"
        static int profiling_count=0;
        profiling_count++;
        if(unlikely(profiling_count > 2000)) exit(0);
        usec_t dt = update_every * USEC_PER_SEC;
#else
        usec_t dt = heartbeat_next(&hb, step);
#endif
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

        if(global_iterations_counter % 10 == 0)
            get_MemTotal();

        if(!collect_data_for_all_pids()) {
            netdata_log_error("Cannot collect /proc data for running processes. Disabling apps.plugin...");
            printf("DISABLE\n");
            netdata_mutex_unlock(&apps_and_stdout_mutex);
            exit(1);
        }

        aggregate_processes_to_targets();
        normalize_utilization(apps_groups_root_target);

        if(send_resource_usage)
            send_resource_usage_to_netdata(dt);

        send_proc_states_count(dt);
        send_charts_updates_to_netdata(apps_groups_root_target, "app", "app_group", "Processes Custom Groups");
        send_collected_data_to_netdata(apps_groups_root_target, "app", dt);

        send_charts_updates_to_netdata(tree_root_target, "process_tree", "top_process", "Processes Tree");
        send_collected_data_to_netdata(tree_root_target, "process_tree", dt);

        if (enable_users_charts) {
            send_charts_updates_to_netdata(users_root_target, "user", "user", "User Processes");
            send_collected_data_to_netdata(users_root_target, "user", dt);
        }

        if (enable_groups_charts) {
            send_charts_updates_to_netdata(groups_root_target, "usergroup", "user_group", "User Group Processes");
            send_collected_data_to_netdata(groups_root_target, "usergroup", dt);
        }

        fflush(stdout);

        show_guest_time_old = show_guest_time;

        debug_log("done Loop No %zu", global_iterations_counter);
    }
    netdata_mutex_unlock(&apps_and_stdout_mutex);
}
