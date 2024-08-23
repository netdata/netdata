// SPDX-License-Identifier: GPL-3.0-or-later

#include "apps_plugin.h"

static struct pid_stat **all_pids = NULL;
size_t all_pids_count = 0; // the number of processes running

struct pid_stat *root_of_pids = NULL;   // global linked list of all processes running

#if (ALL_PIDS_ARE_READ_INSTANTLY == 0)
// Another pre-allocated list of all possible pids.
// We need it to assign them a unique sortlist id, so that we
// read parents before children. This is needed to prevent a situation where
// a child is found running, but until we read its parent, it has exited and
// its parent has accumulated its resources.
pid_t *all_pids_sortlist = NULL;
#endif

void pids_init(void) {
#if (ALL_PIDS_ARE_READ_INSTANTLY == 0)
    all_pids_sortlist = callocz(sizeof(pid_t), (size_t)pid_max + 1);
#endif

    all_pids = callocz(sizeof(struct pid_stat *), (size_t) pid_max + 1);
}

inline struct pid_stat *find_pid_entry(pid_t pid) {
    return all_pids[pid];
}

static inline struct pid_stat *get_or_allocate_pid_entry(pid_t pid) {
    struct pid_stat *p = find_pid_entry(pid);
    if(likely(p))
        return p;

    p = callocz(sizeof(struct pid_stat), 1);
    p->fds = mallocz(sizeof(struct pid_fd) * MAX_SPARE_FDS);
    p->fds_size = MAX_SPARE_FDS;
    init_pid_fds(p, 0, p->fds_size);
    p->pid = pid;

    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(root_of_pids, p, prev, next);
    all_pids[pid] = p;
    all_pids_count++;

    return p;
}

static inline void del_pid_entry(pid_t pid) {
    struct pid_stat *p = find_pid_entry(pid);

    if(unlikely(!p)) {
        netdata_log_error("attempted to free pid %d that is not allocated.", pid);
        return;
    }

    debug_log("process %d %s exited, deleting it.", pid, p->comm);

    DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(root_of_pids, p, prev, next);

#if !defined(__FreeBSD__) && !defined(__APPLE__)
    {
        size_t i;
        for(i = 0; i < p->fds_size; i++)
            if(p->fds[i].filename)
                freez(p->fds[i].filename);
    }
    arl_free(p->status_arl);
#endif

    freez(p->fds);
    freez(p->fds_dirname);
    freez(p->stat_filename);
    freez(p->status_filename);
    freez(p->limits_filename);
    freez(p->io_filename);
    freez(p->cmdline_filename);
    freez(p->cmdline);
    freez(p);

    all_pids[pid] = NULL;
    all_pids_count--;
}

static inline int collect_data_for_pid(pid_t pid, void *ptr) {
    if(unlikely(pid < 0 || pid > pid_max)) {
        netdata_log_error("Invalid pid %d read (expected %d to %d). Ignoring process.", pid, 0, pid_max);
        return 0;
    }

    struct pid_stat *p = get_or_allocate_pid_entry(pid);
    if(unlikely(!p || p->read)) return 0;
    p->read = true;

    // debug_log("Reading process %d (%s), sortlist %d", p->pid, p->comm, p->sortlist);

    // --------------------------------------------------------------------
    // /proc/<pid>/stat

    if(unlikely(!managed_log(p, PID_LOG_STAT, read_proc_pid_stat(p, ptr))))
        // there is no reason to proceed if we cannot get its status
        return 0;

    // check its parent pid
    if(unlikely(p->ppid < 0 || p->ppid > pid_max)) {
        netdata_log_error("Pid %d (command '%s') states invalid parent pid %d. Using 0.", pid, p->comm, p->ppid);
        p->ppid = 0;
    }

    // --------------------------------------------------------------------
    // /proc/<pid>/io

    managed_log(p, PID_LOG_IO, read_proc_pid_io(p, ptr));

    // --------------------------------------------------------------------
    // /proc/<pid>/status

    if(unlikely(!managed_log(p, PID_LOG_STATUS, read_proc_pid_status(p, ptr))))
        // there is no reason to proceed if we cannot get its status
        return 0;

    // --------------------------------------------------------------------
    // /proc/<pid>/fd

    if(enable_file_charts) {
        managed_log(p, PID_LOG_FDS, read_pid_file_descriptors(p, ptr));
        managed_log(p, PID_LOG_LIMITS, read_proc_pid_limits(p, ptr));
    }

    // --------------------------------------------------------------------
    // done!

    if(unlikely(debug_enabled && include_exited_childs && all_pids_count && p->ppid && all_pids[p->ppid] && !all_pids[p->ppid]->read))
        debug_log("Read process %d (%s) sortlisted %d, but its parent %d (%s) sortlisted %d, is not read", p->pid, p->comm, p->sortlist, all_pids[p->ppid]->pid, all_pids[p->ppid]->comm, all_pids[p->ppid]->sortlist);

    // mark it as updated
    p->updated = true;
    p->keep = false;
    p->keeploops = 0;

    return 1;
}

void cleanup_exited_pids(void) {
    size_t c;
    struct pid_stat *p = NULL;

    for(p = root_of_pids; p ;) {
        if(!p->updated && (!p->keep || p->keeploops > 0)) {
            if(unlikely(debug_enabled && (p->keep || p->keeploops)))
                debug_log(" > CLEANUP cannot keep exited process %d (%s) anymore - removing it.", p->pid, p->comm);

            for(c = 0; c < p->fds_size; c++)
                if(p->fds[c].fd > 0) {
                    file_descriptor_not_used(p->fds[c].fd);
                    clear_pid_fd(&p->fds[c]);
                }

            pid_t r = p->pid;
            p = p->next;
            del_pid_entry(r);
        }
        else {
            if(unlikely(p->keep)) p->keeploops++;
            p->keep = false;
            p = p->next;
        }
    }
}

// ----------------------------------------------------------------------------

static inline void link_all_processes_to_their_parents(void) {
    struct pid_stat *p, *pp;

    // link all children to their parents
    // and update children count on parents
    for(p = root_of_pids; p ; p = p->next) {
        // for each process found

        p->sortlist = 0;
        p->parent = NULL;

        if(unlikely(!p->ppid)) {
            //unnecessary code from apps_plugin.c
            //p->parent = NULL;
            continue;
        }

        pp = all_pids[p->ppid];
        if(likely(pp)) {
            p->parent = pp;
            pp->children_count++;

            if(unlikely(debug_enabled || (p->target && p->target->debug_enabled)))
                debug_log_int("child %d (%s, %s) on target '%s' has parent %d (%s, %s). Parent: utime=" KERNEL_UINT_FORMAT ", stime=" KERNEL_UINT_FORMAT ", gtime=" KERNEL_UINT_FORMAT ", minflt=" KERNEL_UINT_FORMAT ", majflt=" KERNEL_UINT_FORMAT ", cutime=" KERNEL_UINT_FORMAT ", cstime=" KERNEL_UINT_FORMAT ", cgtime=" KERNEL_UINT_FORMAT ", cminflt=" KERNEL_UINT_FORMAT ", cmajflt=" KERNEL_UINT_FORMAT "", p->pid, p->comm, p->updated?"running":"exited", (p->target)?p->target->name:"UNSET", pp->pid, pp->comm, pp->updated?"running":"exited", pp->utime, pp->stime, pp->gtime, pp->minflt, pp->majflt, pp->cutime, pp->cstime, pp->cgtime, pp->cminflt, pp->cmajflt);
        }
        else {
            p->parent = NULL;
            netdata_log_error("pid %d %s states parent %d, but the later does not exist.", p->pid, p->comm, p->ppid);
        }
    }
}

// ----------------------------------------------------------------------------

static inline int debug_print_process_and_parents(struct pid_stat *p, usec_t time) {
    char *prefix = "\\_ ";
    int indent = 0;

    if(p->parent)
        indent = debug_print_process_and_parents(p->parent, p->stat_collected_usec);
    else
        prefix = " > ";

    char buffer[indent + 1];
    int i;

    for(i = 0; i < indent ;i++) buffer[i] = ' ';
    buffer[i] = '\0';

    fprintf(stderr, "  %s %s%s (%d %s %"PRIu64""
            , buffer
            , prefix
            , p->comm
            , p->pid
            , p->updated?"running":"exited"
            , p->stat_collected_usec - time
    );

    if(p->utime)   fprintf(stderr, " utime=" KERNEL_UINT_FORMAT,   p->utime);
    if(p->stime)   fprintf(stderr, " stime=" KERNEL_UINT_FORMAT,   p->stime);
    if(p->gtime)   fprintf(stderr, " gtime=" KERNEL_UINT_FORMAT,   p->gtime);
    if(p->cutime)  fprintf(stderr, " cutime=" KERNEL_UINT_FORMAT,  p->cutime);
    if(p->cstime)  fprintf(stderr, " cstime=" KERNEL_UINT_FORMAT,  p->cstime);
    if(p->cgtime)  fprintf(stderr, " cgtime=" KERNEL_UINT_FORMAT,  p->cgtime);
    if(p->minflt)  fprintf(stderr, " minflt=" KERNEL_UINT_FORMAT,  p->minflt);
    if(p->cminflt) fprintf(stderr, " cminflt=" KERNEL_UINT_FORMAT, p->cminflt);
    if(p->majflt)  fprintf(stderr, " majflt=" KERNEL_UINT_FORMAT,  p->majflt);
    if(p->cmajflt) fprintf(stderr, " cmajflt=" KERNEL_UINT_FORMAT, p->cmajflt);
    fprintf(stderr, ")\n");

    return indent + 1;
}

static inline void debug_print_process_tree(struct pid_stat *p, char *msg __maybe_unused) {
    debug_log("%s: process %s (%d, %s) with parents:", msg, p->comm, p->pid, p->updated?"running":"exited");
    debug_print_process_and_parents(p, p->stat_collected_usec);
}

static inline void debug_find_lost_child(struct pid_stat *pe, kernel_uint_t lost, int type) {
    int found = 0;
    struct pid_stat *p = NULL;

    for(p = root_of_pids; p ; p = p->next) {
        if(p == pe) continue;

        switch(type) {
            case 1:
                if(p->cminflt > lost) {
                    fprintf(stderr, " > process %d (%s) could use the lost exited child minflt " KERNEL_UINT_FORMAT " of process %d (%s)\n", p->pid, p->comm, lost, pe->pid, pe->comm);
                    found++;
                }
                break;

            case 2:
                if(p->cmajflt > lost) {
                    fprintf(stderr, " > process %d (%s) could use the lost exited child majflt " KERNEL_UINT_FORMAT " of process %d (%s)\n", p->pid, p->comm, lost, pe->pid, pe->comm);
                    found++;
                }
                break;

            case 3:
                if(p->cutime > lost) {
                    fprintf(stderr, " > process %d (%s) could use the lost exited child utime " KERNEL_UINT_FORMAT " of process %d (%s)\n", p->pid, p->comm, lost, pe->pid, pe->comm);
                    found++;
                }
                break;

            case 4:
                if(p->cstime > lost) {
                    fprintf(stderr, " > process %d (%s) could use the lost exited child stime " KERNEL_UINT_FORMAT " of process %d (%s)\n", p->pid, p->comm, lost, pe->pid, pe->comm);
                    found++;
                }
                break;

            case 5:
                if(p->cgtime > lost) {
                    fprintf(stderr, " > process %d (%s) could use the lost exited child gtime " KERNEL_UINT_FORMAT " of process %d (%s)\n", p->pid, p->comm, lost, pe->pid, pe->comm);
                    found++;
                }
                break;
        }
    }

    if(!found) {
        switch(type) {
            case 1:
                fprintf(stderr, " > cannot find any process to use the lost exited child minflt " KERNEL_UINT_FORMAT " of process %d (%s)\n", lost, pe->pid, pe->comm);
                break;

            case 2:
                fprintf(stderr, " > cannot find any process to use the lost exited child majflt " KERNEL_UINT_FORMAT " of process %d (%s)\n", lost, pe->pid, pe->comm);
                break;

            case 3:
                fprintf(stderr, " > cannot find any process to use the lost exited child utime " KERNEL_UINT_FORMAT " of process %d (%s)\n", lost, pe->pid, pe->comm);
                break;

            case 4:
                fprintf(stderr, " > cannot find any process to use the lost exited child stime " KERNEL_UINT_FORMAT " of process %d (%s)\n", lost, pe->pid, pe->comm);
                break;

            case 5:
                fprintf(stderr, " > cannot find any process to use the lost exited child gtime " KERNEL_UINT_FORMAT " of process %d (%s)\n", lost, pe->pid, pe->comm);
                break;
        }
    }
}

static inline kernel_uint_t remove_exited_child_from_parent(kernel_uint_t *field, kernel_uint_t *pfield) {
    kernel_uint_t absorbed = 0;

    if(*field > *pfield) {
        absorbed += *pfield;
        *field -= *pfield;
        *pfield = 0;
    }
    else {
        absorbed += *field;
        *pfield -= *field;
        *field = 0;
    }

    return absorbed;
}

static inline void process_exited_pids() {
    struct pid_stat *p;

    for(p = root_of_pids; p ; p = p->next) {
        if(p->updated || !p->stat_collected_usec)
            continue;

        kernel_uint_t utime  = (p->utime_raw + p->cutime_raw)   * (USEC_PER_SEC * RATES_DETAIL) / (p->stat_collected_usec - p->last_stat_collected_usec);
        kernel_uint_t stime  = (p->stime_raw + p->cstime_raw)   * (USEC_PER_SEC * RATES_DETAIL) / (p->stat_collected_usec - p->last_stat_collected_usec);
        kernel_uint_t gtime  = (p->gtime_raw + p->cgtime_raw)   * (USEC_PER_SEC * RATES_DETAIL) / (p->stat_collected_usec - p->last_stat_collected_usec);
        kernel_uint_t minflt = (p->minflt_raw + p->cminflt_raw) * (USEC_PER_SEC * RATES_DETAIL) / (p->stat_collected_usec - p->last_stat_collected_usec);
        kernel_uint_t majflt = (p->majflt_raw + p->cmajflt_raw) * (USEC_PER_SEC * RATES_DETAIL) / (p->stat_collected_usec - p->last_stat_collected_usec);

        if(utime + stime + gtime + minflt + majflt == 0)
            continue;

        if(unlikely(debug_enabled)) {
            debug_log("Absorb %s (%d %s total resources: utime=" KERNEL_UINT_FORMAT " stime=" KERNEL_UINT_FORMAT " gtime=" KERNEL_UINT_FORMAT " minflt=" KERNEL_UINT_FORMAT " majflt=" KERNEL_UINT_FORMAT ")"
                      , p->comm
                      , p->pid
                      , p->updated?"running":"exited"
                      , utime
                      , stime
                      , gtime
                      , minflt
                      , majflt
            );
            debug_print_process_tree(p, "Searching parents");
        }

        struct pid_stat *pp;
        for(pp = p->parent; pp ; pp = pp->parent) {
            if(!pp->updated) continue;

            kernel_uint_t absorbed;
            absorbed = remove_exited_child_from_parent(&utime,  &pp->cutime);
            if(unlikely(debug_enabled && absorbed))
                debug_log(" > process %s (%d %s) absorbed " KERNEL_UINT_FORMAT " utime (remaining: " KERNEL_UINT_FORMAT ")", pp->comm, pp->pid, pp->updated?"running":"exited", absorbed, utime);

            absorbed = remove_exited_child_from_parent(&stime,  &pp->cstime);
            if(unlikely(debug_enabled && absorbed))
                debug_log(" > process %s (%d %s) absorbed " KERNEL_UINT_FORMAT " stime (remaining: " KERNEL_UINT_FORMAT ")", pp->comm, pp->pid, pp->updated?"running":"exited", absorbed, stime);

            absorbed = remove_exited_child_from_parent(&gtime,  &pp->cgtime);
            if(unlikely(debug_enabled && absorbed))
                debug_log(" > process %s (%d %s) absorbed " KERNEL_UINT_FORMAT " gtime (remaining: " KERNEL_UINT_FORMAT ")", pp->comm, pp->pid, pp->updated?"running":"exited", absorbed, gtime);

            absorbed = remove_exited_child_from_parent(&minflt, &pp->cminflt);
            if(unlikely(debug_enabled && absorbed))
                debug_log(" > process %s (%d %s) absorbed " KERNEL_UINT_FORMAT " minflt (remaining: " KERNEL_UINT_FORMAT ")", pp->comm, pp->pid, pp->updated?"running":"exited", absorbed, minflt);

            absorbed = remove_exited_child_from_parent(&majflt, &pp->cmajflt);
            if(unlikely(debug_enabled && absorbed))
                debug_log(" > process %s (%d %s) absorbed " KERNEL_UINT_FORMAT " majflt (remaining: " KERNEL_UINT_FORMAT ")", pp->comm, pp->pid, pp->updated?"running":"exited", absorbed, majflt);
        }

        if(unlikely(utime + stime + gtime + minflt + majflt > 0)) {
            if(unlikely(debug_enabled)) {
                if(utime) debug_find_lost_child(p, utime, 3);
                if(stime) debug_find_lost_child(p, stime, 4);
                if(gtime) debug_find_lost_child(p, gtime, 5);
                if(minflt) debug_find_lost_child(p, minflt, 1);
                if(majflt) debug_find_lost_child(p, majflt, 2);
            }

            p->keep = true;

            debug_log(" > remaining resources - KEEP - for another loop: %s (%d %s total resources: utime=" KERNEL_UINT_FORMAT " stime=" KERNEL_UINT_FORMAT " gtime=" KERNEL_UINT_FORMAT " minflt=" KERNEL_UINT_FORMAT " majflt=" KERNEL_UINT_FORMAT ")"
                      , p->comm
                      , p->pid
                      , p->updated?"running":"exited"
                      , utime
                      , stime
                      , gtime
                      , minflt
                      , majflt
            );

            for(pp = p->parent; pp ; pp = pp->parent) {
                if(pp->updated) break;
                pp->keep = true;

                debug_log(" > - KEEP - parent for another loop: %s (%d %s)"
                          , pp->comm
                          , pp->pid
                          , pp->updated?"running":"exited"
                );
            }

            p->utime_raw   = utime  * (p->stat_collected_usec - p->last_stat_collected_usec) / (USEC_PER_SEC * RATES_DETAIL);
            p->stime_raw   = stime  * (p->stat_collected_usec - p->last_stat_collected_usec) / (USEC_PER_SEC * RATES_DETAIL);
            p->gtime_raw   = gtime  * (p->stat_collected_usec - p->last_stat_collected_usec) / (USEC_PER_SEC * RATES_DETAIL);
            p->minflt_raw  = minflt * (p->stat_collected_usec - p->last_stat_collected_usec) / (USEC_PER_SEC * RATES_DETAIL);
            p->majflt_raw  = majflt * (p->stat_collected_usec - p->last_stat_collected_usec) / (USEC_PER_SEC * RATES_DETAIL);
            p->cutime_raw = p->cstime_raw = p->cgtime_raw = p->cminflt_raw = p->cmajflt_raw = 0;

            debug_log(" ");
        }
        else
            debug_log(" > totally absorbed - DONE - %s (%d %s)"
                      , p->comm
                      , p->pid
                      , p->updated?"running":"exited"
            );
    }
}

// ----------------------------------------------------------------------------

// 1. read all files in /proc
// 2. for each numeric directory:
//    i.   read /proc/pid/stat
//    ii.  read /proc/pid/status
//    iii. read /proc/pid/io (requires root access)
//    iii. read the entries in directory /proc/pid/fd (requires root access)
//         for each entry:
//         a. find or create a struct file_descriptor
//         b. cleanup any old/unused file_descriptors

// after all these, some pids may be linked to targets, while others may not

// in case of errors, only 1 every 1000 errors is printed
// to avoid filling up all disk space
// if debug is enabled, all errors are printed

static inline void mark_pid_as_unread(struct pid_stat *p) {
    p->read = false; // mark it as not read, so that collect_data_for_pid() will read it
    p->updated = false;
    p->merged = false;
    p->children_count = 0;
    p->parent = NULL;
}

#if defined(__FreeBSD__) || defined(__APPLE__)
static inline void get_current_time(void) {
    struct timeval current_time;
    gettimeofday(&current_time, NULL);
    system_current_time_ut = timeval_usec(&current_time);
}
#endif

#if defined(__FreeBSD__)
static inline bool collect_data_for_all_pids_per_os(void) {
    // Mark all processes as unread before collecting new data
    struct pid_stat *p = NULL;
    if(all_pids_count) {
        for(p = root_of_pids; p ; p = p->next)
            mark_pid_as_unread(p);
    }

    int i, procnum;

    static size_t procbase_size = 0;
    static struct kinfo_proc *procbase = NULL;

    size_t new_procbase_size;

    int mib[3] = { CTL_KERN, KERN_PROC, KERN_PROC_PROC };
    if (unlikely(sysctl(mib, 3, NULL, &new_procbase_size, NULL, 0))) {
        netdata_log_error("sysctl error: Can't get processes data size");
        return false;
    }

    // give it some air for processes that may be started
    // during this little time.
    new_procbase_size += 100 * sizeof(struct kinfo_proc);

    // increase the buffer if needed
    if(new_procbase_size > procbase_size) {
        procbase_size = new_procbase_size;
        procbase = reallocz(procbase, procbase_size);
    }

    // sysctl() gets from new_procbase_size the buffer size
    // and also returns to it the amount of data filled in
    new_procbase_size = procbase_size;

    // get the processes from the system
    if (unlikely(sysctl(mib, 3, procbase, &new_procbase_size, NULL, 0))) {
        netdata_log_error("sysctl error: Can't get processes data");
        return false;
    }

    // based on the amount of data filled in
    // calculate the number of processes we got
    procnum = new_procbase_size / sizeof(struct kinfo_proc);

    get_current_time();

    for (i = 0 ; i < procnum ; ++i) {
        pid_t pid = procbase[i].ki_pid;
        if (pid <= 0) continue;
        collect_data_for_pid(pid, &procbase[i]);
    }

    return true;
}
#endif // __FreeBSD__

#if defined(__APPLE__)
static inline bool collect_data_for_all_pids_per_os(void) {
    // Mark all processes as unread before collecting new data
    struct pid_stat *p;
    if(all_pids_count) {
        for(p = root_of_pids; p; p = p->next)
            mark_pid_as_unread(p);
    }

    static pid_t *pids = NULL;
    static int allocatedProcessCount = 0;

    // Get the number of processes
    int numberOfProcesses = proc_listpids(PROC_ALL_PIDS, 0, NULL, 0);
    if (numberOfProcesses <= 0) {
        netdata_log_error("Failed to retrieve the process count");
        return false;
    }

    // Allocate or reallocate space to hold all the process IDs if necessary
    if (numberOfProcesses > allocatedProcessCount) {
        // Allocate additional space to avoid frequent reallocations
        allocatedProcessCount = numberOfProcesses + 100;
        pids = reallocz(pids, allocatedProcessCount * sizeof(pid_t));
    }

    // this is required, otherwise the PIDs become totally random
    memset(pids, 0, allocatedProcessCount * sizeof(pid_t));

    // get the list of PIDs
    numberOfProcesses = proc_listpids(PROC_ALL_PIDS, 0, pids, allocatedProcessCount * sizeof(pid_t));
    if (numberOfProcesses <= 0) {
        netdata_log_error("Failed to retrieve the process IDs");
        return false;
    }

    get_current_time();

    // Collect data for each process
    for (int i = 0; i < numberOfProcesses; ++i) {
        pid_t pid = pids[i];
        if (pid <= 0) continue;

        struct pid_info pi = { 0 };

        int mib[4] = {CTL_KERN, KERN_PROC, KERN_PROC_PID, pid};

        size_t procSize = sizeof(pi.proc);
        if(sysctl(mib, 4, &pi.proc, &procSize, NULL, 0) == -1) {
            netdata_log_error("Failed to get proc for PID %d", pid);
            continue;
        }
        if(procSize == 0) // no such process
            continue;

        int st = proc_pidinfo(pid, PROC_PIDTASKINFO, 0, &pi.taskinfo, sizeof(pi.taskinfo));
        if (st <= 0) {
            netdata_log_error("Failed to get task info for PID %d", pid);
            continue;
        }

        st = proc_pidinfo(pid, PROC_PIDTBSDINFO, 0, &pi.bsdinfo, sizeof(pi.bsdinfo));
        if (st <= 0) {
            netdata_log_error("Failed to get BSD info for PID %d", pid);
            continue;
        }

        st = proc_pid_rusage(pid, RUSAGE_INFO_V4, (rusage_info_t *)&pi.rusageinfo);
        if (st < 0) {
            netdata_log_error("Failed to get resource usage info for PID %d", pid);
            continue;
        }

        collect_data_for_pid(pid, &pi);
    }

    return true;
}
#endif // __APPLE__

#if !defined(__FreeBSD__) && !defined(__APPLE__)
static int compar_pid(const void *pid1, const void *pid2) {

    struct pid_stat *p1 = all_pids[*((pid_t *)pid1)];
    struct pid_stat *p2 = all_pids[*((pid_t *)pid2)];

    if(p1->sortlist > p2->sortlist)
        return -1;
    else
        return 1;
}

static inline bool collect_data_for_all_pids_per_os(void) {
    struct pid_stat *p = NULL;

    // clear process state counter
    memset(proc_state_count, 0, sizeof proc_state_count);

    if(all_pids_count) {
        size_t slc = 0;
        for(p = root_of_pids; p ; p = p->next) {
            mark_pid_as_unread(p);
            all_pids_sortlist[slc++] = p->pid;
        }

        if(unlikely(slc != all_pids_count)) {
            netdata_log_error("Internal error: I was thinking I had %zu processes in my arrays, but it seems there are %zu.", all_pids_count, slc);
            all_pids_count = slc;
        }

        if(include_exited_childs) {
            // Read parents before childs
            // This is needed to prevent a situation where
            // a child is found running, but until we read
            // its parent, it has exited and its parent
            // has accumulated its resources.

            qsort((void *)all_pids_sortlist, (size_t)all_pids_count, sizeof(pid_t), compar_pid);

            // we forward read all running processes
            // collect_data_for_pid() is smart enough,
            // not to read the same pid twice per iteration
            for(slc = 0; slc < all_pids_count; slc++) {
                collect_data_for_pid(all_pids_sortlist[slc], NULL);
            }
        }
    }

    static char uptime_filename[FILENAME_MAX + 1] = "";
    if(*uptime_filename == '\0')
        snprintfz(uptime_filename, FILENAME_MAX, "%s/proc/uptime", netdata_configured_host_prefix);

    system_uptime_secs = (kernel_uint_t)(uptime_msec(uptime_filename) / MSEC_PER_SEC);

    char dirname[FILENAME_MAX + 1];

    snprintfz(dirname, FILENAME_MAX, "%s/proc", netdata_configured_host_prefix);
    DIR *dir = opendir(dirname);
    if(!dir) return false;

    struct dirent *de = NULL;

    while((de = readdir(dir))) {
        char *endptr = de->d_name;

        if(unlikely(de->d_type != DT_DIR || de->d_name[0] < '0' || de->d_name[0] > '9'))
            continue;

        pid_t pid = (pid_t) strtoul(de->d_name, &endptr, 10);

        // make sure we read a valid number
        if(unlikely(endptr == de->d_name || *endptr != '\0'))
            continue;

        collect_data_for_pid(pid, NULL);
    }
    closedir(dir);

    return true;
}
#endif // !__FreeBSD__ && !__APPLE__

bool collect_data_for_all_pids(void) {
    if(!collect_data_for_all_pids_per_os())
        return false;

    if(!all_pids_count)
        return false;

    // we need /proc/stat to normalize the cpu consumption of the exited childs
    read_global_time();

    // build the process tree
    link_all_processes_to_their_parents();

    // normally this is done
    // however we may have processes exited while we collected values
    // so let's find the exited ones
    // we do this by collecting the ownership of process
    // if we manage to get the ownership, the process still runs
    process_exited_pids();

    return true;
}
