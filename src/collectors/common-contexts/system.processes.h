// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SYSTEM_PROCESSES_H
#define NETDATA_SYSTEM_PROCESSES_H

#include "common-contexts.h"

static inline void common_system_processes(uint64_t running, uint64_t blocked, int update_every) {
    static RRDSET *st_processes = NULL;
    static RRDDIM *rd_running = NULL;
    static RRDDIM *rd_blocked = NULL;

    if(unlikely(!st_processes)) {
        st_processes = rrdset_create_localhost(
            "system"
        , "processes"
        , NULL
        , "processes"
        , NULL
        , "System Processes"
        , "processes"
        , _COMMON_PLUGIN_NAME
        , _COMMON_PLUGIN_MODULE_NAME
        , NETDATA_CHART_PRIO_SYSTEM_PROCESSES
        , update_every
        , RRDSET_TYPE_LINE
        );

        rd_running = rrddim_add(st_processes, "running", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        rd_blocked = rrddim_add(st_processes, "blocked", NULL, -1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(st_processes, rd_running, running);
    rrddim_set_by_pointer(st_processes, rd_blocked, blocked);
    rrdset_done(st_processes);
}

#endif //NETDATA_SYSTEM_PROCESSES_H
