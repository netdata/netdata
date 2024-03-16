// SPDX-License-Identifier: GPL-3.0-or-later

#include "apps_plugin.h"

// ----------------------------------------------------------------------------

static inline void assign_target_to_pid(struct pid_stat *p) {
    targets_assignment_counter++;

    uint32_t hash = simple_hash(p->comm);
    size_t pclen  = strlen(p->comm);

    struct target *w;
    for(w = apps_groups_root_target; w ; w = w->next) {
        // if(debug_enabled || (p->target && p->target->debug_enabled)) debug_log_int("\t\tcomparing '%s' with '%s'", w->compare, p->comm);

        // find it - 4 cases:
        // 1. the target is not a pattern
        // 2. the target has the prefix
        // 3. the target has the suffix
        // 4. the target is something inside cmdline

        if(unlikely(( (!w->starts_with && !w->ends_with && w->comparehash == hash && !strcmp(w->compare, p->comm))
                      || (w->starts_with && !w->ends_with && !strncmp(w->compare, p->comm, w->comparelen))
                      || (!w->starts_with && w->ends_with && pclen >= w->comparelen && !strcmp(w->compare, &p->comm[pclen - w->comparelen]))
                      || (proc_pid_cmdline_is_needed && w->starts_with && w->ends_with && p->cmdline && strstr(p->cmdline, w->compare))
                          ))) {

            p->matched_by_config = true;
            if(w->target) p->target = w->target;
            else p->target = w;

            if(debug_enabled || (p->target && p->target->debug_enabled))
                debug_log_int("%s linked to target %s", p->comm, p->target->name);

            break;
        }
    }
}

int read_proc_pid_stat(struct pid_stat *p, void *ptr) {
    (void)ptr;

#ifdef __FreeBSD__
    struct kinfo_proc *proc_info = (struct kinfo_proc *)ptr;
    if (unlikely(proc_info->ki_tdflags & TDF_IDLETD))
        goto cleanup;
#else
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
#endif

    p->last_stat_collected_usec = p->stat_collected_usec;
    p->stat_collected_usec = now_monotonic_usec();
    calls_counter++;

#ifdef __FreeBSD__
    char *comm          = proc_info->ki_comm;
    p->ppid             = proc_info->ki_ppid;
#else
    // p->pid           = str2pid_t(procfile_lineword(ff, 0, 0));
    char *comm          = procfile_lineword(ff, 0, 1);
    p->state            = *(procfile_lineword(ff, 0, 2));
    p->ppid             = (int32_t)str2pid_t(procfile_lineword(ff, 0, 3));
    // p->pgrp          = (int32_t)str2pid_t(procfile_lineword(ff, 0, 4));
    // p->session       = (int32_t)str2pid_t(procfile_lineword(ff, 0, 5));
    // p->tty_nr        = (int32_t)str2pid_t(procfile_lineword(ff, 0, 6));
    // p->tpgid         = (int32_t)str2pid_t(procfile_lineword(ff, 0, 7));
    // p->flags         = str2uint64_t(procfile_lineword(ff, 0, 8));
#endif
    if(strcmp(p->comm, comm) != 0) {
        if(unlikely(debug_enabled)) {
            if(p->comm[0])
                debug_log("\tpid %d (%s) changed name to '%s'", p->pid, p->comm, comm);
            else
                debug_log("\tJust added %d (%s)", p->pid, comm);
        }

        strncpyz(p->comm, comm, MAX_COMPARE_NAME);

        // /proc/<pid>/cmdline
        if(likely(proc_pid_cmdline_is_needed))
            managed_log(p, PID_LOG_CMDLINE, read_proc_pid_cmdline(p));

        assign_target_to_pid(p);
    }

#ifdef __FreeBSD__
    pid_incremental_rate(stat, p->minflt,  (kernel_uint_t)proc_info->ki_rusage.ru_minflt);
    pid_incremental_rate(stat, p->cminflt, (kernel_uint_t)proc_info->ki_rusage_ch.ru_minflt);
    pid_incremental_rate(stat, p->majflt,  (kernel_uint_t)proc_info->ki_rusage.ru_majflt);
    pid_incremental_rate(stat, p->cmajflt, (kernel_uint_t)proc_info->ki_rusage_ch.ru_majflt);
    pid_incremental_rate(stat, p->utime,   (kernel_uint_t)proc_info->ki_rusage.ru_utime.tv_sec * 100 + proc_info->ki_rusage.ru_utime.tv_usec / 10000);
    pid_incremental_rate(stat, p->stime,   (kernel_uint_t)proc_info->ki_rusage.ru_stime.tv_sec * 100 + proc_info->ki_rusage.ru_stime.tv_usec / 10000);
    pid_incremental_rate(stat, p->cutime,  (kernel_uint_t)proc_info->ki_rusage_ch.ru_utime.tv_sec * 100 + proc_info->ki_rusage_ch.ru_utime.tv_usec / 10000);
    pid_incremental_rate(stat, p->cstime,  (kernel_uint_t)proc_info->ki_rusage_ch.ru_stime.tv_sec * 100 + proc_info->ki_rusage_ch.ru_stime.tv_usec / 10000);

    p->num_threads      = proc_info->ki_numthreads;

    if(enable_guest_charts) {
        enable_guest_charts = false;
        netdata_log_info("Guest charts aren't supported by FreeBSD");
    }
#else
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
    p->collected_starttime        = str2kernel_uint_t(procfile_lineword(ff, 0, 21)) / system_hz;
    p->uptime           = (global_uptime > p->collected_starttime)?(global_uptime - p->collected_starttime):0;
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
#endif

    if(unlikely(debug_enabled || (p->target && p->target->debug_enabled)))
        debug_log_int("READ PROC/PID/STAT: %s/proc/%d/stat, process: '%s' on target '%s' (dt=%llu) VALUES: utime=" KERNEL_UINT_FORMAT ", stime=" KERNEL_UINT_FORMAT ", cutime=" KERNEL_UINT_FORMAT ", cstime=" KERNEL_UINT_FORMAT ", minflt=" KERNEL_UINT_FORMAT ", majflt=" KERNEL_UINT_FORMAT ", cminflt=" KERNEL_UINT_FORMAT ", cmajflt=" KERNEL_UINT_FORMAT ", threads=%d", netdata_configured_host_prefix, p->pid, p->comm, (p->target)?p->target->name:"UNSET", p->stat_collected_usec - p->last_stat_collected_usec, p->utime, p->stime, p->cutime, p->cstime, p->minflt, p->majflt, p->cminflt, p->cmajflt, p->num_threads);

    if(unlikely(global_iterations_counter == 1)) {
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
    }
    update_proc_state_count(p->state);
    return 1;

cleanup:
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
    p->num_threads      = 0;
    // p->rss              = 0;
    return 0;
}

