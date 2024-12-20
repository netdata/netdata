// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SYSTEM_PROCESSES_H
#define NETDATA_SYSTEM_PROCESSES_H

#include "common-contexts.h"

#define _system_process_chart() \
    rrdset_create_localhost( \
        "system" \
        , "processes" \
        , NULL  \
        , "processes" \
        , NULL \
        , "System Processes" \
        , "processes" \
        , _COMMON_PLUGIN_NAME \
        , _COMMON_PLUGIN_MODULE_NAME \
        , NETDATA_CHART_PRIO_SYSTEM_PROCESSES \
        , update_every \
        , RRDSET_TYPE_LINE \
        )

#if defined(OS_WINDOWS)
static inline void common_system_processes(uint64_t running, int update_every) {
    static RRDSET *st_processes = NULL;
    static RRDDIM *rd_running = NULL;

    if(unlikely(!st_processes)) {
        st_processes = _system_process_chart();

        rd_running = rrddim_add(st_processes, "running", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(st_processes, rd_running, running);
    rrdset_done(st_processes);
}

// EBPF COUNTER PART
static inline void common_system_threads(uint64_t threads, int update_every) {
    static RRDSET *st_threads = NULL;
    static RRDDIM *rd_threads = NULL;

    if(unlikely(!st_threads)) {
        st_threads = rrdset_create_localhost(
            "system"
            , "threads"
            , NULL
            , "processes"
            , NULL
            , "Threads"
            , "threads"
            , _COMMON_PLUGIN_NAME
            , _COMMON_PLUGIN_MODULE_NAME
            , NETDATA_CHART_PRIO_WINDOWS_THREADS
            , update_every
            , RRDSET_TYPE_LINE
            );

        rd_threads = rrddim_add(st_threads, "threads", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(st_threads, rd_threads, threads);
    rrdset_done(st_threads);
}
#endif

#if defined(OS_LINUX)
static inline void common_system_processes(uint64_t running, uint64_t blocked, int update_every) {
    static RRDSET *st_processes = NULL;
    static RRDDIM *rd_running = NULL;
    static RRDDIM *rd_blocked = NULL;

    if(unlikely(!st_processes)) {
        st_processes = _system_process_chart();

        rd_running = rrddim_add(st_processes, "running", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        rd_blocked = rrddim_add(st_processes, "blocked", NULL, -1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(st_processes, rd_running, (collected_number)running);
    rrddim_set_by_pointer(st_processes, rd_blocked, (collected_number)blocked);
    rrdset_done(st_processes);
}
#endif

static inline void common_system_context_switch(uint64_t value, int update_every) {
    static RRDSET *st_ctxt = NULL;
    static RRDDIM *rd_switches = NULL;

    if(unlikely(!st_ctxt)) {
        st_ctxt = rrdset_create_localhost(
         "system"
        , "ctxt"
        , NULL
        , "processes"
        , NULL
        , "CPU Context Switches"
        , "context switches/s"
        , _COMMON_PLUGIN_NAME
        , _COMMON_PLUGIN_MODULE_NAME
        , NETDATA_CHART_PRIO_SYSTEM_CTXT
        , update_every
        , RRDSET_TYPE_LINE
        );

        rd_switches = rrddim_add(st_ctxt, "switches", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(st_ctxt, rd_switches,  (collected_number)value);
    rrdset_done(st_ctxt);
}


#endif //NETDATA_SYSTEM_PROCESSES_H
