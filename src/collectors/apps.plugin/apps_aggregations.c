// SPDX-License-Identifier: GPL-3.0-or-later

#include "apps_plugin.h"

// ----------------------------------------------------------------------------
// update statistics on the targets

static size_t zero_all_targets(struct target *root) {
    struct target *w;
    size_t count = 0;

    for (w = root; w ; w = w->next) {
        count++;

        for(size_t f = 0; f < PDF_MAX ;f++)
            w->values[f] = 0;

        w->uptime_min = 0;
        w->uptime_max = 0;

#if (PROCESSES_HAVE_FDS == 1)
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
#endif

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

static inline void aggregate_pid_on_target(struct target *w, struct pid_stat *p, struct target *o __maybe_unused) {
    if(unlikely(!p->updated)) {
        // the process is not running
        return;
    }

    if(unlikely(!w)) {
        netdata_log_error("pid %d %s was left without a target!", p->pid, pid_stat_comm(p));
        return;
    }

#if (PROCESSES_HAVE_FDS == 1) && (PROCESSES_HAVE_PID_LIMITS == 1)
    if(p->openfds_limits_percent > w->max_open_files_percent)
        w->max_open_files_percent = p->openfds_limits_percent;
#endif

    for(size_t f = 0; f < PDF_MAX ;f++)
        w->values[f] += p->values[f];

    if(!w->uptime_min || p->values[PDF_UPTIME] < w->uptime_min) w->uptime_min = p->values[PDF_UPTIME];
    if(!w->uptime_max || w->uptime_max < p->values[PDF_UPTIME]) w->uptime_max = p->values[PDF_UPTIME];

    if(unlikely(debug_enabled)) {
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

#if (PROCESSES_HAVE_FDS == 1)
            for(size_t c = 0; c < p->fds_size; c++)
                if(p->fds[c].fd > 0) {
                    file_descriptor_not_used(p->fds[c].fd);
                    clear_pid_fd(&p->fds[c]);
                }
#endif

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

static struct target *get_apps_groups_target_for_pid(struct pid_stat *p) {
    targets_assignment_counter++;

    for(struct target *w = apps_groups_root_target; w ; w = w->next) {
        if(w->type != TARGET_TYPE_APP_GROUP) continue;

        if(pid_match_check(p, &w->match)) {
            if(p->is_manager)
                return NULL;

            p->matched_by_config = true;
            return w->target ? w->target : w;
        }
    }

    return NULL;
}

static void assign_a_target_to_all_processes(void) {
    // assign targets from app_groups.conf
    for(struct pid_stat *p = root_of_pids(); p ; p = p->next) {
        if(!p->target)
            p->target = get_apps_groups_target_for_pid(p);
    }

    // assign targets from their parents, if they have
    for(struct pid_stat *p = root_of_pids(); p ; p = p->next) {
        if(!p->target) {
            if(!p->is_manager) {
                for (struct pid_stat *pp = p->parent; pp; pp = pp->parent) {
                    if(pp->is_manager) break;

                    if (pp->target) {
                        p->target = pp->target;
                        break;
                    }
                }
            }

            if(!p->target) {
                // there is no target, get it from the tree
                p->target = get_tree_target(p);
            }
        }

        fatal_assert(p->target != NULL);
    }
}

void aggregate_processes_to_targets(void) {
    assign_a_target_to_all_processes();
    apps_groups_targets_count = zero_all_targets(apps_groups_root_target);

#if (PROCESSES_HAVE_UID == 1)
    zero_all_targets(users_root_target);
#endif
#if (PROCESSES_HAVE_GID == 1)
    zero_all_targets(groups_root_target);
#endif
#if (PROCESSES_HAVE_SID == 1)
    zero_all_targets(sids_root_target);
#endif

    // this has to be done, before the cleanup
    struct target *w = NULL, *o = NULL;
    (void)w; (void)o;

    // concentrate everything on the targets
    for(struct pid_stat *p = root_of_pids(); p ; p = p->next) {

        // --------------------------------------------------------------------
        // apps_groups and tree target

        aggregate_pid_on_target(p->target, p, NULL);


        // --------------------------------------------------------------------
        // user target

#if (PROCESSES_HAVE_UID == 1)
        update_cached_host_users();

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
        update_cached_host_users();

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
        // sid target

#if (PROCESSES_HAVE_SID == 1)
        o = p->sid_target;
        if(likely(p->sid_target && p->sid_target->sid_name == p->sid_name))
            w = p->sid_target;
        else
            w = p->sid_target = get_sid_target(p->sid_name);

        aggregate_pid_on_target(w, p, o);
#endif

        // --------------------------------------------------------------------
        // aggregate all file descriptors

#if (PROCESSES_HAVE_FDS == 1)
        if(enable_file_charts)
            aggregate_pid_fds_on_targets(p);
#endif
    }

    cleanup_exited_pids();
}
