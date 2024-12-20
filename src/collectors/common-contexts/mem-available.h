// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_MEM_AVAILABLE_H
#define NETDATA_MEM_AVAILABLE_H
#include "common-contexts.h"

static inline void common_mem_available(uint64_t available_bytes, int update_every) {
    static RRDSET *st_mem_available = NULL;
    static RRDDIM *rd_avail = NULL;

    if(unlikely(!st_mem_available)) {
        st_mem_available = rrdset_create_localhost(
            "mem"
            , "available"
            , NULL
            , "overview"
            , NULL
            , "Available RAM for applications"
            , "MiB"
            , _COMMON_PLUGIN_NAME
            , _COMMON_PLUGIN_MODULE_NAME
            , NETDATA_CHART_PRIO_MEM_SYSTEM_AVAILABLE
            , update_every
            , RRDSET_TYPE_AREA
        );

        rd_avail   = rrddim_add(st_mem_available, "avail", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
    }

    // this always have to be in base units, so that exporting sends base units to other time-series db
    rrddim_set_by_pointer(st_mem_available, rd_avail, (collected_number)available_bytes);
    rrdset_done(st_mem_available);
}

#endif //NETDATA_MEM_AVAILABLE_H
