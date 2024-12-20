// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DISK_BUSY_H
#define NETDATA_DISK_BUSY_H

#include "common-contexts.h"

typedef struct {
    RRDSET *st_busy;
    RRDDIM *rd_busy;
} ND_DISK_BUSY;

static inline void common_disk_busy(ND_DISK_BUSY *d, const char *id, const char *name, uint64_t busy_ms, int update_every, instance_labels_cb_t cb, void *data) {
    if(unlikely(!d->st_busy)) {
        d->st_busy = rrdset_create_localhost(
                "disk_busy"
                , id
                , name
                , "utilization"
                , "disk.busy"
                , "Disk Busy Time"
                , "milliseconds"
                , _COMMON_PLUGIN_NAME
                , _COMMON_PLUGIN_MODULE_NAME
                , NETDATA_CHART_PRIO_DISK_BUSY
                , update_every
                , RRDSET_TYPE_AREA
        );

        d->rd_busy  = rrddim_add(d->st_busy, "busy",  NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);

        if(cb)
            cb(d->st_busy, data);
    }

    // this always have to be in base units, so that exporting sends base units to other time-series db
    rrddim_set_by_pointer(d->st_busy, d->rd_busy, (collected_number)busy_ms);
    rrdset_done(d->st_busy);
}

#endif //NETDATA_DISK_BUSY_H
