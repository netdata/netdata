// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DISK_IO_H
#define NETDATA_DISK_IO_H

#include "common-contexts.h"

typedef struct {
    RRDSET *st_io;
    RRDDIM *rd_io_reads;
    RRDDIM *rd_io_writes;
} ND_DISK_IO;

static inline void common_disk_io(ND_DISK_IO *d, const char *id, const char *name, uint64_t bytes_read, uint64_t bytes_write, int update_every, instance_labels_cb_t cb, void *data) {
    if(unlikely(!d->st_io)) {
        d->st_io = rrdset_create_localhost(
            "disk"
            , id
            , name
            , "io"
            , "disk.io"
            , "Disk I/O Bandwidth"
            , "KiB/s"
            , _COMMON_PLUGIN_NAME
            , _COMMON_PLUGIN_MODULE_NAME
            , NETDATA_CHART_PRIO_DISK_IO
            , update_every
            , RRDSET_TYPE_AREA
        );

        d->rd_io_reads  = rrddim_add(d->st_io, "reads",  NULL,  1, 1024, RRD_ALGORITHM_INCREMENTAL);
        d->rd_io_writes = rrddim_add(d->st_io, "writes", NULL, -1, 1024, RRD_ALGORITHM_INCREMENTAL);

        if(cb)
            cb(d->st_io, data);
    }

    // this always have to be in base units, so that exporting sends base units to other time-series db
    rrddim_set_by_pointer(d->st_io, d->rd_io_reads, (collected_number)bytes_read);
    rrddim_set_by_pointer(d->st_io, d->rd_io_writes, (collected_number)bytes_write);
    rrdset_done(d->st_io);
}

#endif //NETDATA_DISK_IO_H
