// SPDX-License-Identifier: GPL-3.0-or-later

#include "apps_plugin.h"

#if (INCREMENTAL_DATA_COLLECTION == 1)
bool managed_log(struct pid_stat *p, PID_LOG log, bool status) {
    if(unlikely(!status)) {
        // netdata_log_error("command failed log %u, errno %d", log, errno);

        if(unlikely(debug_enabled || errno != ENOENT)) {
            if(unlikely(debug_enabled || !(p->log_thrown & log))) {
                p->log_thrown |= log;
                switch(log) {
                    case PID_LOG_IO:
#if !defined(OS_LINUX)
                        netdata_log_error("Cannot fetch process %d I/O info (command '%s')", p->pid, pid_stat_comm(p));
#else
                        netdata_log_error("Cannot process %s/proc/%d/io (command '%s')", netdata_configured_host_prefix, p->pid, pid_stat_comm(p));
#endif
                        break;

                    case PID_LOG_STATUS:
#if !defined(OS_LINUX)
                        netdata_log_error("Cannot fetch process %d status info (command '%s')", p->pid, pid_stat_comm(p));
#else
                        netdata_log_error("Cannot process %s/proc/%d/status (command '%s')", netdata_configured_host_prefix, p->pid, pid_stat_comm(p));
#endif
                        break;

                    case PID_LOG_CMDLINE:
#if !defined(OS_LINUX)
                        netdata_log_error("Cannot fetch process %d command line (command '%s')", p->pid, pid_stat_comm(p));
#else
                        netdata_log_error("Cannot process %s/proc/%d/cmdline (command '%s')", netdata_configured_host_prefix, p->pid, pid_stat_comm(p));
#endif
                        break;

                    case PID_LOG_FDS:
#if !defined(OS_LINUX)
                        netdata_log_error("Cannot fetch process %d files (command '%s')", p->pid, pid_stat_comm(p));
#else
                        netdata_log_error("Cannot process entries in %s/proc/%d/fd (command '%s')", netdata_configured_host_prefix, p->pid, pid_stat_comm(p));
#endif
                        break;

                    case PID_LOG_LIMITS:
#if !defined(OS_LINUX)
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

static inline bool incrementally_read_pid_stat(struct pid_stat *p, void *ptr) {
    p->last_stat_collected_usec = p->stat_collected_usec;
    p->stat_collected_usec = now_monotonic_usec();
    calls_counter++;

    if(!OS_FUNCTION(apps_os_read_pid_stat)(p, ptr))
        return 0;

    return 1;
}

static inline int incrementally_read_pid_io(struct pid_stat *p, void *ptr) {
    p->last_io_collected_usec = p->io_collected_usec;
    p->io_collected_usec = now_monotonic_usec();
    calls_counter++;

    bool ret = OS_FUNCTION(apps_os_read_pid_io)(p, ptr);

    return ret ? 1 : 0;
}

// --------------------------------------------------------------------------------------------------------------------

int incrementally_collect_data_for_pid_stat(struct pid_stat *p, void *ptr) {
    if(unlikely(p->read)) return 0;

    pid_collection_started(p);

    // --------------------------------------------------------------------
    // /proc/<pid>/stat

    if(unlikely(!managed_log(p, PID_LOG_STAT, incrementally_read_pid_stat(p, ptr)))) {
        // there is no reason to proceed if we cannot get its status
        pid_collection_failed(p);
        return 0;
    }

    // check its parent pid
    if(unlikely(p->ppid < INIT_PID))
        p->ppid = 0;

    // --------------------------------------------------------------------
    // /proc/<pid>/io

    managed_log(p, PID_LOG_IO, incrementally_read_pid_io(p, ptr));

    // --------------------------------------------------------------------
    // /proc/<pid>/status

    if(unlikely(!managed_log(p, PID_LOG_STATUS, OS_FUNCTION(apps_os_read_pid_status)(p, ptr)))) {
        // there is no reason to proceed if we cannot get its status
        pid_collection_failed(p);
        return 0;
    }

    // --------------------------------------------------------------------
    // /proc/<pid>/fd

#if (PROCESSES_HAVE_FDS == 1)
    if(enable_file_charts) {
        managed_log(p, PID_LOG_FDS, read_pid_file_descriptors(p, ptr));
#if (PROCESSES_HAVE_PID_LIMITS == 1)
        managed_log(p, PID_LOG_LIMITS, OS_FUNCTION(apps_os_read_pid_limits)(p, ptr));
#endif
    }
#endif

    // --------------------------------------------------------------------
    // done!

#if defined(NETDATA_INTERNAL_CHECKS) && (ALL_PIDS_ARE_READ_INSTANTLY == 0)
    struct pid_stat *pp = p->parent;
    if(unlikely(include_exited_childs && pp && !pp->read))
        nd_log(NDLS_COLLECTORS, NDLP_WARNING,
               "Read process %d (%s) sortlisted %"PRIu32", but its parent %d (%s) sortlisted %"PRIu32", is not read",
               p->pid, pid_stat_comm(p), p->sortlist, pp->pid, pid_stat_comm(pp), pp->sortlist);
#endif

    pid_collection_completed(p);

    return 1;
}

int incrementally_collect_data_for_pid(pid_t pid, void *ptr) {
    if(unlikely(pid < INIT_PID)) {
        netdata_log_error("Invalid pid %d read (expected >= %d). Ignoring process.", pid, INIT_PID);
        return 0;
    }

    struct pid_stat *p = get_or_allocate_pid_entry(pid);
    if(unlikely(!p)) return 0;

    return incrementally_collect_data_for_pid_stat(p, ptr);
}
#endif

// --------------------------------------------------------------------------------------------------------------------

#if (PROCESSES_HAVE_CMDLINE == 1)
int read_proc_pid_cmdline(struct pid_stat *p) {
    static char cmdline[MAX_CMDLINE];

    if(unlikely(!OS_FUNCTION(apps_os_get_pid_cmdline)(p, cmdline, sizeof(cmdline))))
        goto cleanup;

    update_pid_cmdline(p, cmdline);

    return 1;

cleanup:
    // copy the command to the command line
    string_freez(p->cmdline);
    p->cmdline = NULL;
    return 0;
}
#endif
