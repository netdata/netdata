// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_MEM_WRITEBACK_H
#define NETDATA_MEM_WRITEBACK_H

#include "common-contexts.h"

#if defined(OS_WINDOWS)
static inline void common_mem_writeback(uint64_t writeback,
                                        int multiplier,
                                        int update_every) {
    static RRDSET *st_mem_writeback = NULL;
    static RRDDIM *rd_writeback = NULL;

    if(unlikely(!st_mem_writeback)) {
        st_mem_writeback = rrdset_create_localhost("mem"
                                                   , "writeback"
                                                   , NULL
                                                   , "writeback"
                                                   , NULL
                                                   , "Writeback Memory"
                                                   , "MiB"
                                                   , _COMMON_PLUGIN_NAME
                                                   , _COMMON_PLUGIN_MODULE_NAME
                                                   , NETDATA_CHART_PRIO_MEM_KERNEL
                                                   , update_every
                                                   , RRDSET_TYPE_LINE
                                                   );
        rrdset_flag_set(st_mem_writeback, RRDSET_FLAG_DETAIL);

        // Users can configure Windows Page Syze to improve performance, but by default on x86_64
        // the value is 4096 bytes
        rd_writeback     = rrddim_add(st_mem_writeback, "Writeback",     NULL, multiplier, 1024, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(st_mem_writeback, rd_writeback,     (collected_number)writeback);
    rrdset_done(st_mem_writeback);
}
#endif

#if defined(OS_LINUX)
static inline void common_mem_writeback(uint64_t dirty,
                                        uint64_t writeback,
                                        uint64_t fuseWriteback,
                                        uint64_t NFSWriteback,
                                        uint64_t Bounce,
                                        int update_every) {
    static RRDSET *st_mem_writeback = NULL;
    static RRDDIM *rd_dirty = NULL, *rd_writeback = NULL, *rd_fusewriteback = NULL,
                  *rd_nfs_writeback = NULL, *rd_bounce = NULL;

    if(unlikely(!st_mem_writeback)) {
        st_mem_writeback = rrdset_create_localhost("mem"
                                                   , "writeback"
                                                   , NULL
                                                   , "writeback"
                                                   , NULL
                                                   , "Writeback Memory"
                                                   , "MiB"
                                                   , _COMMON_PLUGIN_NAME
                                                   , _COMMON_PLUGIN_MODULE_NAME
                                                   , NETDATA_CHART_PRIO_MEM_KERNEL
                                                   , update_every
                                                   , RRDSET_TYPE_LINE
                                                   );
        rrdset_flag_set(st_mem_writeback, RRDSET_FLAG_DETAIL);

        rd_dirty         = rrddim_add(st_mem_writeback, "Dirty",         NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
        rd_writeback     = rrddim_add(st_mem_writeback, "Writeback",     NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
        rd_fusewriteback = rrddim_add(st_mem_writeback, "FuseWriteback", NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
        rd_nfs_writeback = rrddim_add(st_mem_writeback, "NfsWriteback",  NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
        rd_bounce        = rrddim_add(st_mem_writeback, "Bounce",        NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(st_mem_writeback, rd_dirty,         (collected_number)dirty);
    rrddim_set_by_pointer(st_mem_writeback, rd_writeback,     (collected_number)writeback);
    rrddim_set_by_pointer(st_mem_writeback, rd_fusewriteback, (collected_number)fuseWriteback);
    rrddim_set_by_pointer(st_mem_writeback, rd_nfs_writeback, (collected_number)NFSWriteback);
    rrddim_set_by_pointer(st_mem_writeback, rd_bounce,        (collected_number)Bounce);
    rrdset_done(st_mem_writeback);
}
#endif

#endif //NETDATA_MEM_WRITEBACK_H

