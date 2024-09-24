// SPDX-License-Identifier: GPL-3.0-or-later

#include "apps_plugin.h"

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
        w->cminflt = 0;
        w->cmajflt = 0;

        w->utime = 0;
        w->stime = 0;

#if (PROCESSES_HAVE_CPU_GUEST_TIME == 1)
        w->gtime = 0;
#endif

#if (PROCESSES_HAVE_CPU_CHILDREN_TIME == 1)
        w->cutime = 0;
        w->cstime = 0;
#if (PROCESSES_HAVE_CPU_GUEST_TIME == 1)
        w->cgtime = 0;
#endif
#endif

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

#if (PROCESSES_HAVE_LOGICAL_IO == 1)
        w->io_logical_bytes_read = 0;
        w->io_logical_bytes_written = 0;
#endif
#if (PROCESSES_HAVE_PHYSICAL_IO == 1)
        w->io_storage_bytes_read = 0;
        w->io_storage_bytes_written = 0;
#endif
#if (PROCESSES_HAVE_IO_CALLS == 1)
        w->io_read_calls = 0;
        w->io_write_calls = 0;
#endif

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

    w->utime  += p->utime;
    w->stime  += p->stime;

#if (PROCESSES_HAVE_CPU_GUEST_TIME == 1)
    w->gtime  += p->gtime;
#endif

#if (PROCESSES_HAVE_CPU_CHILDREN_TIME == 1)
    w->cutime  += p->cutime;
    w->cstime  += p->cstime;
#if (PROCESSES_HAVE_CPU_GUEST_TIME == 1)
    w->cgtime  += p->cgtime;
#endif
#endif

    w->minflt += p->minflt;
    w->majflt += p->majflt;
    w->cminflt += p->cminflt;
    w->cmajflt += p->cmajflt;

    // w->rss += p->rss;

    w->status_vmsize   += p->status_vmsize;
    w->status_vmrss    += p->status_vmrss;
    w->status_vmshared += p->status_vmshared;
    w->status_rssfile  += p->status_rssfile;
    w->status_rssshmem += p->status_rssshmem;
    w->status_vmswap   += p->status_vmswap;
#if (PROCESSES_HAVE_CONTEXT_SWITCHES == 1)
    w->status_voluntary_ctxt_switches += p->status_voluntary_ctxt_switches;
    w->status_nonvoluntary_ctxt_switches += p->status_nonvoluntary_ctxt_switches;
#endif

#if (PROCESSES_HAVE_LOGICAL_IO == 1)
    w->io_logical_bytes_read    += p->io_logical_bytes_read;
    w->io_logical_bytes_written += p->io_logical_bytes_written;
#endif
#if (PROCESSES_HAVE_PHYSICAL_IO == 1)
    w->io_storage_bytes_read    += p->io_storage_bytes_read;
    w->io_storage_bytes_written += p->io_storage_bytes_written;
#endif
#if (PROCESSES_HAVE_IO_CALLS == 1)
    w->io_read_calls            += p->io_read_calls;
    w->io_write_calls           += p->io_write_calls;
#endif

    w->processes++;
    w->num_threads += p->num_threads;

    if(!w->uptime_min || p->uptime < w->uptime_min) w->uptime_min = p->uptime;
    if(!w->uptime_max || w->uptime_max < p->uptime) w->uptime_max = p->uptime;
    w->uptime_sum += p->uptime;

    if(unlikely(debug_enabled || w->debug_enabled)) {
        struct pid_on_target *pid_on_target = mallocz(sizeof(struct pid_on_target));
        pid_on_target->pid = p->pid;
        pid_on_target->next = w->root_pid;
        w->root_pid = pid_on_target;
    }
}

static inline void cleanup_exited_pids(void) {
    struct pid_stat *p = NULL;

    for(p = root_of_pids(); p ;) {
        if(!p->updated && (!p->keep || p->keeploops > 0)) {
            if(unlikely(debug_enabled && (p->keep || p->keeploops)))
                debug_log(" > CLEANUP cannot keep exited process %d (%s) anymore - removing it.", p->pid, pid_stat_comm(p));

            for(size_t c = 0; c < p->fds_size; c++)
                if(p->fds[c].fd > 0) {
                    file_descriptor_not_used(p->fds[c].fd);
                    clear_pid_fd(&p->fds[c]);
                }

            const pid_t r = p->pid;
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

void aggregate_processes_to_targets(void) {
    apply_apps_groups_targets_inheritance();

    zero_all_targets(tree_root_target);
#if (PROCESSES_HAVE_UID == 1)
    zero_all_targets(users_root_target);
#endif
#if (PROCESSES_HAVE_GID == 1)
    zero_all_targets(groups_root_target);
#endif
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

#if (PROCESSES_HAVE_UID == 1)
        o = p->uid_target;
        if(likely(p->uid_target && p->uid_target->uid == p->uid))
            w = p->uid_target;
        else {
            if(unlikely(debug_enabled && p->uid_target))
                debug_log("pid %d (%s) switched user from %u (%s) to %u.", p->pid, pid_stat_comm(p), p->uid_target->uid, p->uid_target->name, p->uid);

            w = p->uid_target = get_uid_target(p->uid);
        }

        aggregate_pid_on_target(w, p, o);
#endif

        // --------------------------------------------------------------------
        // user group target

#if (PROCESSES_HAVE_GID == 1)
        o = p->gid_target;
        if(likely(p->gid_target && p->gid_target->gid == p->gid))
            w = p->gid_target;
        else {
            if(unlikely(debug_enabled && p->gid_target))
                debug_log("pid %d (%s) switched group from %u (%s) to %u.", p->pid, pid_stat_comm(p), p->gid_target->gid, p->gid_target->name, p->gid);

            w = p->gid_target = get_gid_target(p->gid);
        }

        aggregate_pid_on_target(w, p, o);
#endif


        // --------------------------------------------------------------------
        // tree target

        o = p->tree_target;
        if(likely(p->tree_target && p->tree_target->pid_comm == p->comm))
            w = p->tree_target;
        else {
            w = p->tree_target = get_tree_target(p);

            if(unlikely(debug_enabled && o))
                debug_log("pid %d (%s) switched top target from '%s' to '%s'.", p->pid, pid_stat_comm(p), string2str(o->pid_comm), string2str(w->pid_comm));
        }

        aggregate_pid_on_target(w, p, o);


        // --------------------------------------------------------------------
        // aggregate all file descriptors

        if(enable_file_charts)
            aggregate_pid_fds_on_targets(p);
    }

    cleanup_exited_pids();
}
