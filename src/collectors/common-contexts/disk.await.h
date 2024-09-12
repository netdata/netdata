// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DISK_AWAIT_H
#define NETDATA_DISK_AWAIT_H

#include "common-contexts.h"

typedef struct {
    RRDSET *st_await;
    RRDDIM *rd_await_reads;
    RRDDIM *rd_await_writes;
} ND_DISK_AWAIT;

static inline void common_system_ioawait(uint64_t bytes_read, uint64_t bytes_write, int update_every) {
    static RRDSET *st_avgsz = NULL;
    static RRDDIM *rd_avgsz_reads = NULL, *rd_avgsz_writes = NULL;

    if(unlikely(!st_avgsz)) {
        st_avgsz = rrdset_create_localhost("system"
                                           , "disk_await"
                                           , NULL
                                           , "disk"
                                           , NULL
                                           , "Average Completed I/O Operation Time"
                                           , "milliseconds/operation"
                                           , _COMMON_PLUGIN_NAME
                                           , _COMMON_PLUGIN_MODULE_NAME
                                           , NETDATA_CHART_PRIO_SYSTEM_IOAWAIT
                                           , update_every
                                           , RRDSET_TYPE_AREA
                                           );


        rd_avgsz_reads  = rrddim_add(st_avgsz, "reads",  NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
        rd_avgsz_writes = rrddim_add(st_avgsz, "writes", NULL, -1, 1024, RRD_ALGORITHM_ABSOLUTE);
    }

    // this always have to be in base units, so that exporting sends base units to other time-series db
    rrddim_set_by_pointer(st_avgsz, rd_avgsz_reads, (collected_number)bytes_read);
    rrddim_set_by_pointer(st_avgsz, rd_avgsz_writes, (collected_number)bytes_write);
    rrdset_done(st_avgsz);
}

static inline void common_disk_await(ND_DISK_AWAIT *d, const char *id, const char *name, uint64_t bytes_read, uint64_t bytes_write, int update_every, instance_labels_cb_t cb, void *data) {
    if(unlikely(!d->st_await)) {
        d->st_await = rrdset_create_localhost("disk_await"
                                              , id
                                              , name
                                              , "iotime"
                                              , "disk.await"
                                              , "Average Completed I/O Operation Time"
                                              , "milliseconds/operation"
                                              , _COMMON_PLUGIN_NAME
                                              , _COMMON_PLUGIN_MODULE_NAME
                                              , NETDATA_CHART_PRIO_DISK_AWAIT
                                              , update_every
                                              , RRDSET_TYPE_LINE
                                              );

        d->rd_await_reads  = rrddim_add(d->st_await, "reads",  NULL,  1, 1024, RRD_ALGORITHM_INCREMENTAL);
        d->rd_await_writes = rrddim_add(d->st_await, "writes", NULL, -1, 1024, RRD_ALGORITHM_INCREMENTAL);

        if(cb)
            cb(d->st_await, data);
    }

    // this always have to be in base units, so that exporting sends base units to other time-series db
    rrddim_set_by_pointer(d->st_await, d->rd_await_reads, (collected_number)bytes_read);
    rrddim_set_by_pointer(d->st_await, d->rd_await_writes, (collected_number)bytes_write);
    rrdset_done(d->st_await);
}

#endif //NETDATA_DISK_AWAIT_H
