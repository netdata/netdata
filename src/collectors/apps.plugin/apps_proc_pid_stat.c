// SPDX-License-Identifier: GPL-3.0-or-later

#include "apps_plugin.h"

// ----------------------------------------------------------------------------

static inline void assign_app_group_target_to_pid(struct pid_stat *p) {
    targets_assignment_counter++;

    struct target *w;
    for(w = apps_groups_root_target; w ; w = w->next) {
        // find it - 4 cases:
        // 1. the target is not a pattern
        // 2. the target has the prefix
        // 3. the target has the suffix
        // 4. the target is something inside cmdline

        if(unlikely(( (!w->starts_with && !w->ends_with && w->compare == p->comm)
                      || (w->starts_with && !w->ends_with && string_starts_with_string(p->comm, w->compare))
                      || (!w->starts_with && w->ends_with && string_ends_with_string(p->comm, w->compare))
                      || (proc_pid_cmdline_is_needed && w->starts_with && w->ends_with && strstr(pid_stat_cmdline(p), string2str(w->compare)))
                          ))) {

            p->matched_by_config = true;
            if(w->target) p->target = w->target;
            else p->target = w;

            if(debug_enabled || (p->target && p->target->debug_enabled))
                debug_log_int("%s linked to target %s", pid_stat_comm(p), p->target->name);

            break;
        }
    }
}

static inline void update_pid_comm(struct pid_stat *p, const char *comm) {
    if(strcmp(pid_stat_comm(p), comm) != 0) {
        if(unlikely(debug_enabled)) {
            if(string_strlen(p->comm))
                debug_log("\tpid %d (%s) changed name to '%s'", p->pid, pid_stat_comm(p), comm);
            else
                debug_log("\tJust added %d (%s)", p->pid, comm);
        }

        string_freez(p->comm);
        p->comm = string_strdupz(comm);

        // /proc/<pid>/cmdline
        if(likely(proc_pid_cmdline_is_needed))
            managed_log(p, PID_LOG_CMDLINE, read_proc_pid_cmdline(p));

        assign_app_group_target_to_pid(p);
    }
}

static inline void clear_pid_stat(struct pid_stat *p, bool threads) {
    p->minflt           = 0;
    p->cminflt          = 0;
    p->majflt           = 0;
    p->cmajflt          = 0;
    p->utime            = 0;
    p->stime            = 0;
    p->gtime            = 0;
    p->cutime           = 0;
    p->cstime           = 0;
    p->cgtime           = 0;

    if(threads)
        p->num_threads      = 0;

    // p->rss              = 0;
}

#if defined(__FreeBSD__)
static inline bool read_proc_pid_stat_per_os(struct pid_stat *p, void *ptr) {
    struct kinfo_proc *proc_info = (struct kinfo_proc *)ptr;
    if (unlikely(proc_info->ki_tdflags & TDF_IDLETD))
        goto cleanup;

    char *comm          = proc_info->ki_comm;
    p->ppid             = proc_info->ki_ppid;

    update_pid_comm(p, comm);

    pid_incremental_rate(stat, p->minflt,  (kernel_uint_t)proc_info->ki_rusage.ru_minflt);
    pid_incremental_rate(stat, p->cminflt, (kernel_uint_t)proc_info->ki_rusage_ch.ru_minflt);
    pid_incremental_rate(stat, p->majflt,  (kernel_uint_t)proc_info->ki_rusage.ru_majflt);
    pid_incremental_rate(stat, p->cmajflt, (kernel_uint_t)proc_info->ki_rusage_ch.ru_majflt);
    pid_incremental_rate(stat, p->utime,   (kernel_uint_t)proc_info->ki_rusage.ru_utime.tv_sec * 100 + proc_info->ki_rusage.ru_utime.tv_usec / 10000);
    pid_incremental_rate(stat, p->stime,   (kernel_uint_t)proc_info->ki_rusage.ru_stime.tv_sec * 100 + proc_info->ki_rusage.ru_stime.tv_usec / 10000);
    pid_incremental_rate(stat, p->cutime,  (kernel_uint_t)proc_info->ki_rusage_ch.ru_utime.tv_sec * 100 + proc_info->ki_rusage_ch.ru_utime.tv_usec / 10000);
    pid_incremental_rate(stat, p->cstime,  (kernel_uint_t)proc_info->ki_rusage_ch.ru_stime.tv_sec * 100 + proc_info->ki_rusage_ch.ru_stime.tv_usec / 10000);

    p->num_threads      = proc_info->ki_numthreads;

    usec_t started_ut = timeval_usec(&proc_info->ki_start);
    p->uptime = (system_current_time_ut > started_ut) ? (system_current_time_ut - started_ut) / USEC_PER_SEC : 0;

    if(enable_guest_charts) {
        enable_guest_charts = false;
        netdata_log_info("Guest charts aren't supported by FreeBSD");
    }

    if(unlikely(debug_enabled || (p->target && p->target->debug_enabled)))
        debug_log_int("READ PROC/PID/STAT: %s/proc/%d/stat, process: '%s' on target '%s' (dt=%llu) VALUES: utime=" KERNEL_UINT_FORMAT ", stime=" KERNEL_UINT_FORMAT ", cutime=" KERNEL_UINT_FORMAT ", cstime=" KERNEL_UINT_FORMAT ", minflt=" KERNEL_UINT_FORMAT ", majflt=" KERNEL_UINT_FORMAT ", cminflt=" KERNEL_UINT_FORMAT ", cmajflt=" KERNEL_UINT_FORMAT ", threads=%d", netdata_configured_host_prefix, p->pid, pid_stat_comm(p), (p->target)?p->target->name:"UNSET", p->stat_collected_usec - p->last_stat_collected_usec, p->utime, p->stime, p->cutime, p->cstime, p->minflt, p->majflt, p->cminflt, p->cmajflt, p->num_threads);

    if(unlikely(global_iterations_counter == 1))
        clear_pid_stat(p, false);

    return true;

cleanup:
    clear_pid_stat(p, true);
    return false;
}
#endif // __FreeBSD__

#ifdef __APPLE__
static inline bool read_proc_pid_stat_per_os(struct pid_stat *p, void *ptr) {
    struct pid_info *pi = ptr;

    p->ppid = pi->proc.kp_eproc.e_ppid;

    // Update command name and target if changed
    char comm[PROC_PIDPATHINFO_MAXSIZE];
    int ret = proc_name(p->pid, comm, sizeof(comm));
    if (ret <= 0)
        strncpyz(comm, "unknown", sizeof(comm) - 1);

    update_pid_comm(p, comm);

    kernel_uint_t userCPU = (pi->taskinfo.pti_total_user * mach_info.numer) / mach_info.denom / NSEC_PER_USEC / 10000;
    kernel_uint_t systemCPU = (pi->taskinfo.pti_total_system * mach_info.numer) / mach_info.denom / NSEC_PER_USEC / 10000;

    // Map the values from taskinfo to the pid_stat structure
    pid_incremental_rate(stat, p->minflt, pi->taskinfo.pti_faults);
    pid_incremental_rate(stat, p->majflt, pi->taskinfo.pti_pageins);
    pid_incremental_rate(stat, p->utime, userCPU);
    pid_incremental_rate(stat, p->stime, systemCPU);
    p->num_threads = pi->taskinfo.pti_threadnum;

    usec_t started_ut = timeval_usec(&pi->proc.kp_proc.p_starttime);
    p->uptime = (system_current_time_ut > started_ut) ? (system_current_time_ut - started_ut) / USEC_PER_SEC : 0;

    // Note: Some values such as guest time, cutime, cstime, etc., are not directly available in MacOS.
    // You might need to approximate or leave them unset depending on your needs.

    if(unlikely(debug_enabled || (p->target && p->target->debug_enabled))) {
        debug_log_int("READ PROC/PID/STAT for MacOS: process: '%s' on target '%s' VALUES: utime=" KERNEL_UINT_FORMAT ", stime=" KERNEL_UINT_FORMAT ", minflt=" KERNEL_UINT_FORMAT ", majflt=" KERNEL_UINT_FORMAT ", threads=%d",
                      pid_stat_comm(p), (p->target) ? p->target->name : "UNSET", p->utime, p->stime, p->minflt, p->majflt, p->num_threads);
    }

    if(unlikely(global_iterations_counter == 1))
        clear_pid_stat(p, false);

    // MacOS doesn't have a direct concept of process state like Linux,
    // so updating process state count might need a different approach.

    return true;
}
#endif // __APPLE__

#if !defined(__FreeBSD__) && !defined(__APPLE__)
static inline void update_proc_state_count(char proc_stt) {
    switch (proc_stt) {
        case 'S':
            proc_state_count[PROC_STATUS_SLEEPING] += 1;
            break;
        case 'R':
            proc_state_count[PROC_STATUS_RUNNING] += 1;
            break;
        case 'D':
            proc_state_count[PROC_STATUS_SLEEPING_D] += 1;
            break;
        case 'Z':
            proc_state_count[PROC_STATUS_ZOMBIE] += 1;
            break;
        case 'T':
            proc_state_count[PROC_STATUS_STOPPED] += 1;
            break;
        default:
            break;
    }
}

static inline bool read_proc_pid_stat_per_os(struct pid_stat *p, void *ptr __maybe_unused) {
    static procfile *ff = NULL;

    if(unlikely(!p->stat_filename)) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s/proc/%d/stat", netdata_configured_host_prefix, p->pid);
        p->stat_filename = strdupz(filename);
    }

    int set_quotes = (!ff)?1:0;

    ff = procfile_reopen(ff, p->stat_filename, NULL, PROCFILE_FLAG_NO_ERROR_ON_FILE_IO);
    if(unlikely(!ff)) goto cleanup;

    // if(set_quotes) procfile_set_quotes(ff, "()");
    if(unlikely(set_quotes))
        procfile_set_open_close(ff, "(", ")");

    ff = procfile_readall(ff);
    if(unlikely(!ff)) goto cleanup;

    // p->pid           = str2pid_t(procfile_lineword(ff, 0, 0));
    char *comm          = procfile_lineword(ff, 0, 1);
    p->state            = *(procfile_lineword(ff, 0, 2));
    p->ppid             = (int32_t)str2pid_t(procfile_lineword(ff, 0, 3));
    // p->pgrp          = (int32_t)str2pid_t(procfile_lineword(ff, 0, 4));
    // p->session       = (int32_t)str2pid_t(procfile_lineword(ff, 0, 5));
    // p->tty_nr        = (int32_t)str2pid_t(procfile_lineword(ff, 0, 6));
    // p->tpgid         = (int32_t)str2pid_t(procfile_lineword(ff, 0, 7));
    // p->flags         = str2uint64_t(procfile_lineword(ff, 0, 8));

    update_pid_comm(p, comm);

    pid_incremental_rate(stat, p->minflt,  str2kernel_uint_t(procfile_lineword(ff, 0,  9)));
    pid_incremental_rate(stat, p->cminflt, str2kernel_uint_t(procfile_lineword(ff, 0, 10)));
    pid_incremental_rate(stat, p->majflt,  str2kernel_uint_t(procfile_lineword(ff, 0, 11)));
    pid_incremental_rate(stat, p->cmajflt, str2kernel_uint_t(procfile_lineword(ff, 0, 12)));
    pid_incremental_rate(stat, p->utime,   str2kernel_uint_t(procfile_lineword(ff, 0, 13)));
    pid_incremental_rate(stat, p->stime,   str2kernel_uint_t(procfile_lineword(ff, 0, 14)));
    pid_incremental_rate(stat, p->cutime,  str2kernel_uint_t(procfile_lineword(ff, 0, 15)));
    pid_incremental_rate(stat, p->cstime,  str2kernel_uint_t(procfile_lineword(ff, 0, 16)));
    // p->priority      = str2kernel_uint_t(procfile_lineword(ff, 0, 17));
    // p->nice          = str2kernel_uint_t(procfile_lineword(ff, 0, 18));
    p->num_threads      = (int32_t) str2uint32_t(procfile_lineword(ff, 0, 19), NULL);
    // p->itrealvalue   = str2kernel_uint_t(procfile_lineword(ff, 0, 20));
    kernel_uint_t collected_starttime = str2kernel_uint_t(procfile_lineword(ff, 0, 21)) / system_hz;
    p->uptime           = (system_uptime_secs > collected_starttime)?(system_uptime_secs - collected_starttime):0;
    // p->vsize         = str2kernel_uint_t(procfile_lineword(ff, 0, 22));
    // p->rss           = str2kernel_uint_t(procfile_lineword(ff, 0, 23));
    // p->rsslim        = str2kernel_uint_t(procfile_lineword(ff, 0, 24));
    // p->starcode      = str2kernel_uint_t(procfile_lineword(ff, 0, 25));
    // p->endcode       = str2kernel_uint_t(procfile_lineword(ff, 0, 26));
    // p->startstack    = str2kernel_uint_t(procfile_lineword(ff, 0, 27));
    // p->kstkesp       = str2kernel_uint_t(procfile_lineword(ff, 0, 28));
    // p->kstkeip       = str2kernel_uint_t(procfile_lineword(ff, 0, 29));
    // p->signal        = str2kernel_uint_t(procfile_lineword(ff, 0, 30));
    // p->blocked       = str2kernel_uint_t(procfile_lineword(ff, 0, 31));
    // p->sigignore     = str2kernel_uint_t(procfile_lineword(ff, 0, 32));
    // p->sigcatch      = str2kernel_uint_t(procfile_lineword(ff, 0, 33));
    // p->wchan         = str2kernel_uint_t(procfile_lineword(ff, 0, 34));
    // p->nswap         = str2kernel_uint_t(procfile_lineword(ff, 0, 35));
    // p->cnswap        = str2kernel_uint_t(procfile_lineword(ff, 0, 36));
    // p->exit_signal   = str2kernel_uint_t(procfile_lineword(ff, 0, 37));
    // p->processor     = str2kernel_uint_t(procfile_lineword(ff, 0, 38));
    // p->rt_priority   = str2kernel_uint_t(procfile_lineword(ff, 0, 39));
    // p->policy        = str2kernel_uint_t(procfile_lineword(ff, 0, 40));
    // p->delayacct_blkio_ticks = str2kernel_uint_t(procfile_lineword(ff, 0, 41));

    if(enable_guest_charts) {
        pid_incremental_rate(stat, p->gtime,  str2kernel_uint_t(procfile_lineword(ff, 0, 42)));
        pid_incremental_rate(stat, p->cgtime, str2kernel_uint_t(procfile_lineword(ff, 0, 43)));

        if (show_guest_time || p->gtime || p->cgtime) {
            p->utime -= (p->utime >= p->gtime) ? p->gtime : p->utime;
            p->cutime -= (p->cutime >= p->cgtime) ? p->cgtime : p->cutime;
            show_guest_time = 1;
        }
    }

    if(unlikely(debug_enabled || (p->target && p->target->debug_enabled)))
        debug_log_int("READ PROC/PID/STAT: %s/proc/%d/stat, process: '%s' on target '%s' (dt=%llu) VALUES: utime=" KERNEL_UINT_FORMAT ", stime=" KERNEL_UINT_FORMAT ", cutime=" KERNEL_UINT_FORMAT ", cstime=" KERNEL_UINT_FORMAT ", minflt=" KERNEL_UINT_FORMAT ", majflt=" KERNEL_UINT_FORMAT ", cminflt=" KERNEL_UINT_FORMAT ", cmajflt=" KERNEL_UINT_FORMAT ", threads=%d",
                      netdata_configured_host_prefix, p->pid, pid_stat_comm(p), (p->target)?string2str(p->target->name):"UNSET", p->stat_collected_usec - p->last_stat_collected_usec, p->utime, p->stime, p->cutime, p->cstime, p->minflt, p->majflt, p->cminflt, p->cmajflt, p->num_threads);

    if(unlikely(global_iterations_counter == 1))
        clear_pid_stat(p, false);

    update_proc_state_count(p->state);
    return true;

cleanup:
    clear_pid_stat(p, true);
    return false;
}
#endif // !__FreeBSD__ !__APPLE__

int read_proc_pid_stat(struct pid_stat *p, void *ptr) {
    p->last_stat_collected_usec = p->stat_collected_usec;
    p->stat_collected_usec = now_monotonic_usec();
    calls_counter++;

    if(!read_proc_pid_stat_per_os(p, ptr))
        return 0;

    return 1;
}
