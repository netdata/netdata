// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DISK_AVGSZ_H
#define NETDATA_DISK_AVGSZ_H

#include "common-contexts.h"

typedef struct {
    RRDSET *st_avgsz;
    RRDDIM *rd_avgsz_reads;
    RRDDIM *rd_avgsz_writes;
} ND_DISK_AVGSZ;

static inline void common_disk_avgsz(ND_DISK_AVGSZ *d, const char *id, const char *name, uint64_t avg_bytes_read, uint64_t avg_bytes_write, int update_every, instance_labels_cb_t cb, void *data) {
    if(unlikely(!d->st_avgsz)) {
        d->st_avgsz = rrdset_create_localhost(
            "disk_avgsz"
            , id
            , name
            , "io"
            , "disk.avgsz"
            , "Average Completed I/O Operation Bandwidth"
            , "KiB/operation"
            , _COMMON_PLUGIN_NAME
            , _COMMON_PLUGIN_MODULE_NAME
            , NETDATA_CHART_PRIO_DISK_AVGSZ
            , update_every
            , RRDSET_TYPE_AREA
        );

        d->rd_avgsz_reads  = rrddim_add(d->st_avgsz, "reads",  NULL,  1, 1024, RRD_ALGORITHM_ABSOLUTE);
        d->rd_avgsz_writes = rrddim_add(d->st_avgsz, "writes", NULL, -1, 1024, RRD_ALGORITHM_ABSOLUTE);

        if(cb)
            cb(d->st_avgsz, data);
    }

    // this always have to be in base units, so that exporting sends base units to other time-series db
    rrddim_set_by_pointer(d->st_avgsz, d->rd_avgsz_reads, (collected_number)avg_bytes_read);
    rrddim_set_by_pointer(d->st_avgsz, d->rd_avgsz_writes, (collected_number)avg_bytes_write);
    rrdset_done(d->st_avgsz);
}

#endif //NETDATA_DISK_AVGSZ_H
