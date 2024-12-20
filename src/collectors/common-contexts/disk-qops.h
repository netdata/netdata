// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DISK_QOPS_H
#define NETDATA_DISK_QOPS_H

#include "common-contexts.h"

typedef struct {
    RRDSET *st_qops;
    RRDDIM *rd_qops;
} ND_DISK_QOPS;

static inline void common_disk_qops(ND_DISK_QOPS *d, const char *id, const char *name, uint64_t queued_ops, int update_every, instance_labels_cb_t cb, void *data) {
    if(unlikely(!d->st_qops)) {
        d->st_qops = rrdset_create_localhost(
                "disk_qops"
                , id
                , name
                , "ops"
                , "disk.qops"
                , "Disk Current I/O Operations"
                , "operations"
                , _COMMON_PLUGIN_NAME
                , _COMMON_PLUGIN_MODULE_NAME
                , NETDATA_CHART_PRIO_DISK_QOPS
                , update_every
                , RRDSET_TYPE_LINE
        );

        d->rd_qops  = rrddim_add(d->st_qops, "operations", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        if(cb)
            cb(d->st_qops, data);
    }

    // this always have to be in base units, so that exporting sends base units to other time-series db
    rrddim_set_by_pointer(d->st_qops, d->rd_qops, (collected_number)queued_ops);
    rrdset_done(d->st_qops);
}

#endif //NETDATA_DISK_QOPS_H
