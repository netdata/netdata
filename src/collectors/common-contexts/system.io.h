// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SYSTEM_IO_H
#define NETDATA_SYSTEM_IO_H

#include "common-contexts.h"

static inline void common_system_io(uint64_t read_bytes, uint64_t write_bytes, int update_every) {
    static RRDSET *st_io = NULL;
    static RRDDIM *rd_in = NULL, *rd_out = NULL;

    if(unlikely(!st_io)) {
        st_io = rrdset_create_localhost(
            "system"
            , "io"
            , NULL
            , "disk"
            , NULL
            , "Disk I/O"
            , "KiB/s"
            , _COMMON_PLUGIN_NAME
            , _COMMON_PLUGIN_MODULE_NAME
            , NETDATA_CHART_PRIO_SYSTEM_IO
            , update_every
            , RRDSET_TYPE_AREA
        );

        rd_in  = rrddim_add(st_io, "in",  "reads",  1, 1024, RRD_ALGORITHM_INCREMENTAL);
        rd_out = rrddim_add(st_io, "out", "writes", -1, 1024, RRD_ALGORITHM_INCREMENTAL);
    }

    // this always have to be in base units, so that exporting sends base units to other time-series db
    rrddim_set_by_pointer(st_io, rd_in, (collected_number)read_bytes);
    rrddim_set_by_pointer(st_io, rd_out, (collected_number)write_bytes);
    rrdset_done(st_io);
}

static inline void common_system_uio(uint64_t bytes, int update_every) {
    static RRDSET *st_uio = NULL;
    static RRDDIM *rd_io = NULL;

    if(unlikely(!st_uio)) {
        st_uio = rrdset_create_localhost("system"
                                         , "uio"
                                         , NULL
                                         , "disk"
                                         , NULL
                                         , "Unified disk I/O"
                                         , "KiB/s"
                                         , _COMMON_PLUGIN_NAME
                                         , _COMMON_PLUGIN_MODULE_NAME
                                         , NETDATA_CHART_PRIO_SYSTEM_IO + 1
                                         , update_every
                                         , RRDSET_TYPE_AREA
                                         );

        rd_io  = rrddim_add(st_uio, "io",  "io",  1, 1024, RRD_ALGORITHM_INCREMENTAL);
    }

    // this always have to be in base units, so that exporting sends base units to other time-series db
    rrddim_set_by_pointer(st_uio, rd_io, (collected_number)bytes);
    rrdset_done(st_uio);
}

#endif //NETDATA_SYSTEM_IO_H
