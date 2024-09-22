// SPDX-License-Identifier: GPL-3.0-or-later

#include "apps_plugin.h"

static inline void send_BEGIN(const char *type, const char *name,const char *metric,  usec_t usec) {
    fprintf(stdout, "BEGIN %s.%s_%s %" PRIu64 "\n", type, name, metric, usec);
}

static inline void send_SET(const char *name, kernel_uint_t value) {
    fprintf(stdout, "SET %s = " KERNEL_UINT_FORMAT "\n", name, value);
}

static inline void send_END(void) {
    fprintf(stdout, "END\n\n");
}

void send_resource_usage_to_netdata(usec_t dt) {
    static struct timeval last = { 0, 0 };
    static struct rusage me_last;

    struct timeval now;
    struct rusage me;

    usec_t cpuuser;
    usec_t cpusyst;

    if(!last.tv_sec) {
        now_monotonic_timeval(&last);
        getrusage(RUSAGE_SELF, &me_last);

        cpuuser = 0;
        cpusyst = 0;
    }
    else {
        now_monotonic_timeval(&now);
        getrusage(RUSAGE_SELF, &me);

        cpuuser = me.ru_utime.tv_sec * USEC_PER_SEC + me.ru_utime.tv_usec;
        cpusyst = me.ru_stime.tv_sec * USEC_PER_SEC + me.ru_stime.tv_usec;

        memmove(&last, &now, sizeof(struct timeval));
        memmove(&me_last, &me, sizeof(struct rusage));
    }

    static char created_charts = 0;
    if(unlikely(!created_charts)) {
        created_charts = 1;

        fprintf(stdout,
                "CHART netdata.apps_cpu '' 'Apps Plugin CPU' 'milliseconds/s' apps.plugin netdata.apps_cpu stacked 140000 %1$d\n"
                "DIMENSION user '' incremental 1 1000\n"
                "DIMENSION system '' incremental 1 1000\n"
                "CHART netdata.apps_sizes '' 'Apps Plugin Files' 'files/s' apps.plugin netdata.apps_sizes line 140001 %1$d\n"
                "DIMENSION calls '' incremental 1 1\n"
                "DIMENSION files '' incremental 1 1\n"
                "DIMENSION filenames '' incremental 1 1\n"
                "DIMENSION inode_changes '' incremental 1 1\n"
                "DIMENSION link_changes '' incremental 1 1\n"
                "DIMENSION pids '' absolute 1 1\n"
                "DIMENSION fds '' absolute 1 1\n"
                "DIMENSION targets '' absolute 1 1\n"
                "DIMENSION new_pids 'new pids' incremental 1 1\n"
                , update_every
        );
    }

    fprintf(stdout,
            "BEGIN netdata.apps_cpu %"PRIu64"\n"
            "SET user = %"PRIu64"\n"
            "SET system = %"PRIu64"\n"
            "END\n"
            "BEGIN netdata.apps_sizes %"PRIu64"\n"
            "SET calls = %zu\n"
            "SET files = %zu\n"
            "SET filenames = %zu\n"
            "SET inode_changes = %zu\n"
            "SET link_changes = %zu\n"
            "SET pids = %zu\n"
            "SET fds = %"PRIu32"\n"
            "SET targets = %zu\n"
            "SET new_pids = %zu\n"
            "END\n"
            , dt
            , cpuuser
            , cpusyst
            , dt
            , calls_counter
            , file_counter
            , filenames_allocated_counter
            , inodes_changed_counter
            , links_changed_counter
            , all_pids_count()
            , all_files_len
            , apps_groups_targets_count
            , targets_assignment_counter
    );
}

void send_collected_data_to_netdata(struct target *root, const char *type, usec_t dt) {
    struct target *w;

    for (w = root; w ; w = w->next) {
        if (unlikely(!w->exposed))
            continue;

        send_BEGIN(type, string2str(w->clean_name), "processes", dt);
        send_SET("processes", w->processes);
        send_END();

        send_BEGIN(type, string2str(w->clean_name), "threads", dt);
        send_SET("threads", w->num_threads);
        send_END();

        if (unlikely(!w->processes && !w->is_other))
            continue;

        send_BEGIN(type, string2str(w->clean_name), "cpu_utilization", dt);
        send_SET("user", (kernel_uint_t)(w->utime * utime_fix_ratio) + (include_exited_childs ? ((kernel_uint_t)(w->cutime * cutime_fix_ratio)) : 0ULL));
        send_SET("system", (kernel_uint_t)(w->stime * stime_fix_ratio) + (include_exited_childs ? ((kernel_uint_t)(w->cstime * cstime_fix_ratio)) : 0ULL));
        send_END();

#if !defined(__FreeBSD__) && !defined(__APPLE__)
        if (enable_guest_charts) {
            send_BEGIN(type, string2str(w->clean_name), "cpu_guest_utilization", dt);
            send_SET("guest", (kernel_uint_t)(w->gtime * gtime_fix_ratio) + (include_exited_childs ? ((kernel_uint_t)(w->cgtime * cgtime_fix_ratio)) : 0ULL));
            send_END();
        }

        send_BEGIN(type, string2str(w->clean_name), "cpu_context_switches", dt);
        send_SET("voluntary", w->status_voluntary_ctxt_switches);
        send_SET("involuntary", w->status_nonvoluntary_ctxt_switches);
        send_END();

        send_BEGIN(type, string2str(w->clean_name), "mem_private_usage", dt);
        send_SET("mem", (w->status_vmrss > w->status_vmshared)?(w->status_vmrss - w->status_vmshared) : 0ULL);
        send_END();
#endif

        send_BEGIN(type, string2str(w->clean_name), "mem_usage", dt);
        send_SET("rss", w->status_vmrss);
        send_END();

#if !defined(__APPLE__)
        send_BEGIN(type, string2str(w->clean_name), "vmem_usage", dt);
        send_SET("vmem", w->status_vmsize);
        send_END();
#endif

        send_BEGIN(type, string2str(w->clean_name), "mem_page_faults", dt);
        send_SET("minor", (kernel_uint_t)(w->minflt * minflt_fix_ratio) + (include_exited_childs ? ((kernel_uint_t)(w->cminflt * cminflt_fix_ratio)) : 0ULL));
        send_SET("major", (kernel_uint_t)(w->majflt * majflt_fix_ratio) + (include_exited_childs ? ((kernel_uint_t)(w->cmajflt * cmajflt_fix_ratio)) : 0ULL));
        send_END();

#if !defined(__FreeBSD__) && !defined(__APPLE__)
        send_BEGIN(type, string2str(w->clean_name), "swap_usage", dt);
        send_SET("swap", w->status_vmswap);
        send_END();
#endif

        if (w->processes == 0) {
            send_BEGIN(type, string2str(w->clean_name), "uptime", dt);
            send_SET("uptime", 0);
            send_END();

            if (enable_detailed_uptime_charts) {
                send_BEGIN(type, string2str(w->clean_name), "uptime_summary", dt);
                send_SET("min", 0);
                send_SET("avg", 0);
                send_SET("max", 0);
                send_END();
            }
        } else {
            send_BEGIN(type, string2str(w->clean_name), "uptime", dt);
            send_SET("uptime", w->uptime_max);
            send_END();

            if (enable_detailed_uptime_charts) {
                send_BEGIN(type, string2str(w->clean_name), "uptime_summary", dt);
                send_SET("min", w->uptime_min);
                send_SET("avg", w->processes > 0 ? w->uptime_sum / w->processes : 0);
                send_SET("max", w->uptime_max);
                send_END();
            }
        }

        send_BEGIN(type, string2str(w->clean_name), "disk_physical_io", dt);
        send_SET("reads", w->io_storage_bytes_read);
        send_SET("writes", w->io_storage_bytes_written);
        send_END();

#if !defined(__FreeBSD__) && !defined(__APPLE__)
        send_BEGIN(type, string2str(w->clean_name), "disk_logical_io", dt);
        send_SET("reads", w->io_logical_bytes_read);
        send_SET("writes", w->io_logical_bytes_written);
        send_END();
#endif
        if (enable_file_charts) {
            send_BEGIN(type, string2str(w->clean_name), "fds_open_limit", dt);
            send_SET("limit", w->max_open_files_percent * 100.0);
            send_END();

            send_BEGIN(type, string2str(w->clean_name), "fds_open", dt);
            send_SET("files", w->openfds.files);
            send_SET("sockets", w->openfds.sockets);
            send_SET("pipes", w->openfds.sockets);
            send_SET("inotifies", w->openfds.inotifies);
            send_SET("event", w->openfds.eventfds);
            send_SET("timer", w->openfds.timerfds);
            send_SET("signal", w->openfds.signalfds);
            send_SET("eventpolls", w->openfds.eventpolls);
            send_SET("other", w->openfds.other);
            send_END();
        }
    }
}


// ----------------------------------------------------------------------------
// generate the charts

void send_charts_updates_to_netdata(struct target *root, const char *type, const char *lbl_name, const char *title) {
    struct target *w;

    if (debug_enabled) {
        for (w = root; w; w = w->next) {
            if (unlikely(!w->target && w->processes)) {
                struct pid_on_target *pid_on_target;
                fprintf(stderr, "apps.plugin: target '%s' has aggregated %"PRIu32" process(es):", string2str(w->name), w->processes);
                for (pid_on_target = w->root_pid; pid_on_target; pid_on_target = pid_on_target->next) {
                    fprintf(stderr, " %d", pid_on_target->pid);
                }
                fputc('\n', stderr);
            }
        }
    }

    for (w = root; w; w = w->next) {
        if (likely(w->exposed || (!w->processes && !w->is_other)))
            continue;

        w->exposed = true;

        fprintf(stdout, "CHART %s.%s_cpu_utilization '' '%s CPU utilization (100%% = 1 core)' 'percentage' cpu %s.cpu_utilization stacked 20001 %d\n",
                type, string2str(w->clean_name), title, type, update_every);
        fprintf(stdout, "CLABEL '%s' '%s' 1\n", lbl_name, string2str(w->name));
        fprintf(stdout, "CLABEL_COMMIT\n");
        fprintf(stdout, "DIMENSION user '' absolute 1 %llu\n", time_factor * RATES_DETAIL / 100LLU);
        fprintf(stdout, "DIMENSION system '' absolute 1 %llu\n", time_factor * RATES_DETAIL / 100LLU);

#if !defined(__FreeBSD__) && !defined(__APPLE__)
        if (enable_guest_charts) {
            fprintf(stdout, "CHART %s.%s_cpu_guest_utilization '' '%s CPU guest utlization (100%% = 1 core)' 'percentage' cpu %s.cpu_guest_utilization line 20005 %d\n",
                    type, string2str(w->clean_name), title, type, update_every);
            fprintf(stdout, "CLABEL '%s' '%s' 1\n", lbl_name, string2str(w->name));
            fprintf(stdout, "CLABEL_COMMIT\n");
            fprintf(stdout, "DIMENSION guest '' absolute 1 %llu\n", time_factor * RATES_DETAIL / 100LLU);
        }

        fprintf(stdout, "CHART %s.%s_cpu_context_switches '' '%s CPU context switches' 'switches/s' cpu %s.cpu_context_switches stacked 20010 %d\n",
                type, string2str(w->clean_name), title, type, update_every);
        fprintf(stdout, "CLABEL '%s' '%s' 1\n", lbl_name, string2str(w->name));
        fprintf(stdout, "CLABEL_COMMIT\n");
        fprintf(stdout, "DIMENSION voluntary '' absolute 1 %llu\n", RATES_DETAIL);
        fprintf(stdout, "DIMENSION involuntary '' absolute 1 %llu\n", RATES_DETAIL);

        fprintf(stdout, "CHART %s.%s_mem_private_usage '' '%s memory usage without shared' 'MiB' mem %s.mem_private_usage area 20050 %d\n",
                type, string2str(w->clean_name), title, type, update_every);
        fprintf(stdout, "CLABEL '%s' '%s' 1\n", lbl_name, string2str(w->name));
        fprintf(stdout, "CLABEL_COMMIT\n");
        fprintf(stdout, "DIMENSION mem '' absolute %ld %ld\n", 1L, 1024L);
#endif

        fprintf(stdout, "CHART %s.%s_mem_usage '' '%s memory RSS usage' 'MiB' mem %s.mem_usage area 20055 %d\n",
                type, string2str(w->clean_name), title, type, update_every);
        fprintf(stdout, "CLABEL '%s' '%s' 1\n", lbl_name, string2str(w->name));
        fprintf(stdout, "CLABEL_COMMIT\n");
        fprintf(stdout, "DIMENSION rss '' absolute %ld %ld\n", 1L, 1024L);

#if !defined(__APPLE__)
        fprintf(stdout, "CHART %s.%s_vmem_usage '' '%s virtual memory size' 'MiB' mem %s.vmem_usage line 20065 %d\n",
                type, string2str(w->clean_name), title, type, update_every);
        fprintf(stdout, "CLABEL '%s' '%s' 1\n", lbl_name, string2str(w->name));
        fprintf(stdout, "CLABEL_COMMIT\n");
        fprintf(stdout, "DIMENSION vmem '' absolute %ld %ld\n", 1L, 1024L);
#endif

        fprintf(stdout, "CHART %s.%s_mem_page_faults '' '%s memory page faults' 'pgfaults/s' mem %s.mem_page_faults stacked 20060 %d\n",
                type, string2str(w->clean_name), title, type, update_every);
        fprintf(stdout, "CLABEL '%s' '%s' 1\n", lbl_name, string2str(w->name));
        fprintf(stdout, "CLABEL_COMMIT\n");
        fprintf(stdout, "DIMENSION major '' absolute 1 %llu\n", RATES_DETAIL);
        fprintf(stdout, "DIMENSION minor '' absolute 1 %llu\n", RATES_DETAIL);

#if !defined(__FreeBSD__) && !defined(__APPLE__)
        fprintf(stdout, "CHART %s.%s_swap_usage '' '%s swap usage' 'MiB' mem %s.swap_usage area 20065 %d\n",
                type, string2str(w->clean_name), title, type, update_every);
        fprintf(stdout, "CLABEL '%s' '%s' 1\n", lbl_name, string2str(w->name));
        fprintf(stdout, "CLABEL_COMMIT\n");
        fprintf(stdout, "DIMENSION swap '' absolute %ld %ld\n", 1L, 1024L);
#endif

#if !defined(__FreeBSD__) && !defined(__APPLE__)
        fprintf(stdout, "CHART %s.%s_disk_physical_io '' '%s disk physical IO' 'KiB/s' disk %s.disk_physical_io area 20100 %d\n",
                type, string2str(w->clean_name), title, type, update_every);
        fprintf(stdout, "CLABEL '%s' '%s' 1\n", lbl_name, string2str(w->name));
        fprintf(stdout, "CLABEL_COMMIT\n");
        fprintf(stdout, "DIMENSION reads '' absolute 1 %llu\n", 1024LLU * RATES_DETAIL);
        fprintf(stdout, "DIMENSION writes '' absolute -1 %llu\n", 1024LLU * RATES_DETAIL);

        fprintf(stdout, "CHART %s.%s_disk_logical_io '' '%s disk logical IO' 'KiB/s' disk %s.disk_logical_io area 20105 %d\n",
                type, string2str(w->clean_name), title, type, update_every);
        fprintf(stdout, "CLABEL '%s' '%s' 1\n", lbl_name, string2str(w->name));
        fprintf(stdout, "CLABEL_COMMIT\n");
        fprintf(stdout, "DIMENSION reads '' absolute 1 %llu\n", 1024LLU * RATES_DETAIL);
        fprintf(stdout, "DIMENSION writes '' absolute -1 %llu\n", 1024LLU * RATES_DETAIL);
#else
        fprintf(stdout, "CHART %s.%s_disk_physical_io '' '%s disk physical IO' 'blocks/s' disk %s.disk_physical_block_io area 20100 %d\n",
                type, string2str(w->clean_name), title, type, update_every);
        fprintf(stdout, "CLABEL '%s' '%s' 1\n", lbl_name, string2str(w->name));
        fprintf(stdout, "CLABEL_COMMIT\n");
        fprintf(stdout, "DIMENSION reads '' absolute 1 %llu\n", RATES_DETAIL);
        fprintf(stdout, "DIMENSION writes '' absolute -1 %llu\n", RATES_DETAIL);
#endif

        fprintf(stdout, "CHART %s.%s_processes '' '%s processes' 'processes' processes %s.processes line 20150 %d\n",
                type, string2str(w->clean_name), title, type, update_every);
        fprintf(stdout, "CLABEL '%s' '%s' 1\n", lbl_name, string2str(w->name));
        fprintf(stdout, "CLABEL_COMMIT\n");
        fprintf(stdout, "DIMENSION processes '' absolute 1 1\n");

        fprintf(stdout, "CHART %s.%s_threads '' '%s threads' 'threads' processes %s.threads line 20155 %d\n",
                type, string2str(w->clean_name), title, type, update_every);
        fprintf(stdout, "CLABEL '%s' '%s' 1\n", lbl_name, string2str(w->name));
        fprintf(stdout, "CLABEL_COMMIT\n");
        fprintf(stdout, "DIMENSION threads '' absolute 1 1\n");

        if (enable_file_charts) {
            fprintf(stdout, "CHART %s.%s_fds_open_limit '' '%s open file descriptors limit' '%%' fds %s.fds_open_limit line 20200 %d\n",
                    type, string2str(w->clean_name), title, type, update_every);
            fprintf(stdout, "CLABEL '%s' '%s' 1\n", lbl_name, string2str(w->name));
            fprintf(stdout, "CLABEL_COMMIT\n");
            fprintf(stdout, "DIMENSION limit '' absolute 1 100\n");

            fprintf(stdout, "CHART %s.%s_fds_open '' '%s open files descriptors' 'fds' fds %s.fds_open stacked 20210 %d\n",
                    type, string2str(w->clean_name), title, type, update_every);
            fprintf(stdout, "CLABEL '%s' '%s' 1\n", lbl_name, string2str(w->name));
            fprintf(stdout, "CLABEL_COMMIT\n");
            fprintf(stdout, "DIMENSION files '' absolute 1 1\n");
            fprintf(stdout, "DIMENSION sockets '' absolute 1 1\n");
            fprintf(stdout, "DIMENSION pipes '' absolute 1 1\n");
            fprintf(stdout, "DIMENSION inotifies '' absolute 1 1\n");
            fprintf(stdout, "DIMENSION event '' absolute 1 1\n");
            fprintf(stdout, "DIMENSION timer '' absolute 1 1\n");
            fprintf(stdout, "DIMENSION signal '' absolute 1 1\n");
            fprintf(stdout, "DIMENSION eventpolls '' absolute 1 1\n");
            fprintf(stdout, "DIMENSION other '' absolute 1 1\n");
        }

        fprintf(stdout, "CHART %s.%s_uptime '' '%s uptime' 'seconds' uptime %s.uptime line 20250 %d\n",
                type, string2str(w->clean_name), title, type, update_every);
        fprintf(stdout, "CLABEL '%s' '%s' 1\n", lbl_name, string2str(w->name));
        fprintf(stdout, "CLABEL_COMMIT\n");
        fprintf(stdout, "DIMENSION uptime '' absolute 1 1\n");

        if (enable_detailed_uptime_charts) {
            fprintf(stdout, "CHART %s.%s_uptime_summary '' '%s uptime summary' 'seconds' uptime %s.uptime_summary area 20255 %d\n",
                    type, string2str(w->clean_name), title, type, update_every);
            fprintf(stdout, "CLABEL '%s' '%s' 1\n", lbl_name, string2str(w->name));
            fprintf(stdout, "CLABEL_COMMIT\n");
            fprintf(stdout, "DIMENSION min '' absolute 1 1\n");
            fprintf(stdout, "DIMENSION avg '' absolute 1 1\n");
            fprintf(stdout, "DIMENSION max '' absolute 1 1\n");
        }
    }
}

void send_proc_states_count(usec_t dt __maybe_unused) {
#if !defined(__FreeBSD__) && !defined(__APPLE__)
    static bool chart_added = false;
    // create chart for count of processes in different states
    if (!chart_added) {
        fprintf(
            stdout,
            "CHART system.processes_state '' 'System Processes State' 'processes' processes system.processes_state line %d %d\n",
            NETDATA_CHART_PRIO_SYSTEM_PROCESS_STATES,
            update_every);
        for (proc_state i = PROC_STATUS_RUNNING; i < PROC_STATUS_END; i++) {
            fprintf(stdout, "DIMENSION %s '' absolute 1 1\n", proc_states[i]);
        }
        chart_added = true;
    }

    // send process state count
    fprintf(stdout, "BEGIN system.processes_state %" PRIu64 "\n", dt);
    for (proc_state i = PROC_STATUS_RUNNING; i < PROC_STATUS_END; i++) {
        send_SET(proc_states[i], proc_state_count[i]);
    }
    send_END();
#endif
}

