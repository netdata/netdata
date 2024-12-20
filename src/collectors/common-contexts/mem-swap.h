// SPDX-License-Identifier: GPL-3.0-or-later

#include "common-contexts.h"

static inline void common_mem_swap(uint64_t free_bytes, uint64_t used_bytes, int update_every) {
    static RRDSET *st_system_swap = NULL;
    static RRDDIM *rd_free = NULL, *rd_used = NULL;

    if (free_bytes == 0 && used_bytes == 0 && st_system_swap) {
        rrdset_is_obsolete___safe_from_collector_thread(st_system_swap);
        st_system_swap = NULL;
        rd_free = NULL;
        rd_used = NULL;
        return;
    }

    if(unlikely(!st_system_swap)) {
        st_system_swap = rrdset_create_localhost(
            "mem"
            , "swap"
            , NULL
            , "swap"
            , NULL
            , "System Swap"
            , "MiB"
            , _COMMON_PLUGIN_NAME
            , _COMMON_PLUGIN_MODULE_NAME
            , NETDATA_CHART_PRIO_MEM_SWAP
            , update_every
            , RRDSET_TYPE_STACKED
        );

        rd_free = rrddim_add(st_system_swap, "free",    NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
        rd_used = rrddim_add(st_system_swap, "used",    NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
    }

    // this always have to be in base units, so that exporting sends base units to other time-series db
    rrddim_set_by_pointer(st_system_swap, rd_used, (collected_number)used_bytes);
    rrddim_set_by_pointer(st_system_swap, rd_free, (collected_number)free_bytes);
    rrdset_done(st_system_swap);
}
