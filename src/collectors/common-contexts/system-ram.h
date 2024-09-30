// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SYSTEM_RAM_H
#define NETDATA_SYSTEM_RAM_H

#include "common-contexts.h"

#define _system_ram_chart() \
    rrdset_create_localhost(                    \
        "system"                                \
        , "ram"                                 \
        , NULL                                  \
        , "ram"                                 \
        , NULL                                  \
        , "System RAM"                          \
        , "MiB"                                 \
        , _COMMON_PLUGIN_NAME                   \
        , _COMMON_PLUGIN_MODULE_NAME            \
        , NETDATA_CHART_PRIO_SYSTEM_RAM         \
        , update_every                          \
        , RRDSET_TYPE_STACKED                   \
        )

#ifdef OS_WINDOWS
static inline void common_system_ram(uint64_t free_bytes, uint64_t used_bytes, int update_every) {
    static RRDSET *st_system_ram = NULL;
    static RRDDIM *rd_free = NULL;
    static RRDDIM *rd_used = NULL;

    if(unlikely(!st_system_ram)) {
        st_system_ram = _system_ram_chart();
        rd_free    = rrddim_add(st_system_ram, "free",    NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
        rd_used    = rrddim_add(st_system_ram, "used",    NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
    }

    // this always have to be in base units, so that exporting sends base units to other time-series db
    rrddim_set_by_pointer(st_system_ram, rd_free,    (collected_number)free_bytes);
    rrddim_set_by_pointer(st_system_ram, rd_used,    (collected_number)used_bytes);
    rrdset_done(st_system_ram);
}
#endif

#ifdef OS_LINUX
static inline void common_system_ram(uint64_t free_bytes, uint64_t used_bytes, uint64_t cached_bytes, uint64_t buffers_bytes, int update_every) {
    static RRDSET *st_system_ram = NULL;
    static RRDDIM *rd_free = NULL;
    static RRDDIM *rd_used = NULL;
    static RRDDIM *rd_cached = NULL;
    static RRDDIM *rd_buffers = NULL;

    if(unlikely(!st_system_ram)) {
        st_system_ram = _system_ram_chart();
        rd_free    = rrddim_add(st_system_ram, "free",    NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
        rd_used    = rrddim_add(st_system_ram, "used",    NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
        rd_cached  = rrddim_add(st_system_ram, "cached",  NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
        rd_buffers = rrddim_add(st_system_ram, "buffers", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
    }

    // this always have to be in base units, so that exporting sends base units to other time-series db
    rrddim_set_by_pointer(st_system_ram, rd_free,    (collected_number)free_bytes);
    rrddim_set_by_pointer(st_system_ram, rd_used,    (collected_number)used_bytes);
    rrddim_set_by_pointer(st_system_ram, rd_cached, (collected_number)cached_bytes);
    rrddim_set_by_pointer(st_system_ram, rd_buffers, (collected_number)buffers_bytes);
    rrdset_done(st_system_ram);
}
#endif

#endif //NETDATA_SYSTEM_RAM_H
