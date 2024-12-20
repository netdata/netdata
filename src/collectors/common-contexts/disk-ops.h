// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DISK_OPS_H
#define NETDATA_DISK_OPS_H

#include "common-contexts.h"

typedef struct {
    RRDSET *st_ops;
    RRDDIM *rd_ops_reads;
    RRDDIM *rd_ops_writes;
} ND_DISK_OPS;

static inline void common_disk_ops(ND_DISK_OPS *d, const char *id, const char *name, uint64_t ops_read, uint64_t ops_write, int update_every, instance_labels_cb_t cb, void *data) {
    if(unlikely(!d->st_ops)) {
        d->st_ops = rrdset_create_localhost(
                "disk_ops"
                , id
                , name
                , "ops"
                , "disk.ops"
                , "Disk Completed I/O Operations"
                , "operations/s"
                , _COMMON_PLUGIN_NAME
                , _COMMON_PLUGIN_MODULE_NAME
                , NETDATA_CHART_PRIO_DISK_OPS
                , update_every
                , RRDSET_TYPE_LINE
        );

        d->rd_ops_reads  = rrddim_add(d->st_ops, "reads",  NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
        d->rd_ops_writes = rrddim_add(d->st_ops, "writes", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);

        if(cb)
            cb(d->st_ops, data);
    }

    // this always have to be in base units, so that exporting sends base units to other time-series db
    rrddim_set_by_pointer(d->st_ops, d->rd_ops_reads, (collected_number)ops_read);
    rrddim_set_by_pointer(d->st_ops, d->rd_ops_writes, (collected_number)ops_write);
    rrdset_done(d->st_ops);
}

#endif //NETDATA_DISK_OPS_H
