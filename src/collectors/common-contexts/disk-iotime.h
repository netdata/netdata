// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DISK_IOTIME_H
#define NETDATA_DISK_IOTIME_H

#include "common-contexts.h"

typedef struct {
    RRDSET *st_iotime;
    RRDDIM *rd_reads_ms;
    RRDDIM *rd_writes_ms;
} ND_DISK_IOTIME;

static inline void common_disk_iotime(ND_DISK_IOTIME *d, const char *id, const char *name, uint64_t reads_ms, uint64_t writes_ms, int update_every, instance_labels_cb_t cb, void *data) {
    if(unlikely(!d->st_iotime)) {
        d->st_iotime = rrdset_create_localhost(
                "disk_iotime"
                , id
                , name
                , "utilization"
                , "disk.iotime"
                , "Disk Total I/O Time"
                , "milliseconds/s"
                , _COMMON_PLUGIN_NAME
                , _COMMON_PLUGIN_MODULE_NAME
                , NETDATA_CHART_PRIO_DISK_IOTIME
                , update_every
                , RRDSET_TYPE_AREA
        );

        d->rd_reads_ms  = rrddim_add(d->st_iotime, "reads", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        d->rd_writes_ms = rrddim_add(d->st_iotime, "writes", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);

        if(cb)
            cb(d->st_iotime, data);
    }

    // this always have to be in base units, so that exporting sends base units to other time-series db
    rrddim_set_by_pointer(d->st_iotime, d->rd_reads_ms, (collected_number)reads_ms);
    rrddim_set_by_pointer(d->st_iotime, d->rd_writes_ms, (collected_number)writes_ms);
    rrdset_done(d->st_iotime);
}

#endif //NETDATA_DISK_IOTIME_H
