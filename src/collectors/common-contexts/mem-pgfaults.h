// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_MEM_PGFAULTS_H
#define NETDATA_MEM_PGFAULTS_H

#include "common-contexts.h"

static inline void common_mem_pgfaults(uint64_t minor, uint64_t major, int update_every) {
    static RRDSET *st_pgfaults = NULL;
    static RRDDIM *rd_minor = NULL, *rd_major = NULL;

    if(unlikely(!st_pgfaults)) {
        st_pgfaults = rrdset_create_localhost(
            "mem"
            , "pgfaults"
            , NULL
            , "page faults"
            , NULL
            , "Memory Page Faults"
            , "faults/s"
            , _COMMON_PLUGIN_NAME
            , _COMMON_PLUGIN_MODULE_NAME
            , NETDATA_CHART_PRIO_MEM_SYSTEM_PGFAULTS
            , update_every
            , RRDSET_TYPE_LINE
        );

        rd_minor = rrddim_add(st_pgfaults, "minor", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
        rd_major = rrddim_add(st_pgfaults, "major", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    // this always have to be in base units, so that exporting sends base units to other time-series db
    rrddim_set_by_pointer(st_pgfaults, rd_minor, minor);
    rrddim_set_by_pointer(st_pgfaults, rd_major, major);
    rrdset_done(st_pgfaults);
}

#endif //NETDATA_MEM_PGFAULTS_H
