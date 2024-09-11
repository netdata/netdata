// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DISK_SIZE_H
#define NETDATA_DISK_SIZE_H

#include "common-contexts.h"

typedef struct {
    RRDSET *st_avgsz;
    RRDDIM *rd_avgsz_reads;
    RRDDIM *rd_avgsz_writes;
} ND_DISK_AVGSIZE;

typedef struct {
    RRDSET *st_uavgsz;
    RRDDIM *rd_avgsz_bytes;
} ND_DISK_UAVGSIZE;

static inline void common_disk_avgsize(ND_DISK_AVGSIZE *d, const char *id, const char *name, uint64_t bytes_read, uint64_t bytes_write, uint64_t sector_size, int update_every, instance_labels_cb_t cb, void *data) {
    if(unlikely(!d->st_avgsz)) {
        d->st_avgsz = rrdset_create_localhost(
            "disk_avgsz"
            , id
            , name
            , "size"
            , "disk.avgsz"
            , "Average Completed I/O Operation Bandwidth"
            , "operations/s"
            , _COMMON_PLUGIN_NAME
            , _COMMON_PLUGIN_MODULE_NAME
            , NETDATA_CHART_PRIO_DISK_AVGSZ
            , update_every
            , RRDSET_TYPE_AREA
        );

        d->rd_avgsz_reads  = rrddim_add(d->st_avgsz, "reads",  NULL, sector_size, 1024, RRD_ALGORITHM_ABSOLUTE);
        d->rd_avgsz_writes = rrddim_add(d->st_avgsz, "writes", NULL, sector_size * -1, 1024, RRD_ALGORITHM_ABSOLUTE);

        if(cb)
            cb(d->st_avgsz, data);
    }

    // this always have to be in base units, so that exporting sends base units to other time-series db
    rrddim_set_by_pointer(d->st_avgsz, d->rd_avgsz_reads, (collected_number)bytes_read);
    rrddim_set_by_pointer(d->st_avgsz, d->rd_avgsz_writes, (collected_number)bytes_write);
    rrdset_done(d->st_avgsz);
}

static inline void common_unified_disk_avgsize(ND_DISK_UAVGSIZE *d, const char *id, const char *name, uint64_t bytes, int update_every, instance_labels_cb_t cb, void *data) {
    if(unlikely(!d->st_uavgsz)) {
        d->st_uavgsz = rrdset_create_localhost(
            "disk_uavgsz"
            , id
            , name
            , "size"
            , "disk.uavgsz"
            , "Average Completed I/O Operation Bandwidth"
            , "operations/s"
            , _COMMON_PLUGIN_NAME
            , _COMMON_PLUGIN_MODULE_NAME
            , NETDATA_CHART_PRIO_DISK_AVGSZ + 1
            , update_every
            , RRDSET_TYPE_AREA
        );

        d->rd_avgsz_bytes  = rrddim_add(d->st_uavgsz, "io",  NULL,  1, 1024, RRD_ALGORITHM_ABSOLUTE);

        if(cb)
            cb(d->st_uavgsz, data);
    }

    // this always have to be in base units, so that exporting sends base units to other time-series db
    rrddim_set_by_pointer(d->st_uavgsz, d->rd_avgsz_bytes, (collected_number)bytes);
    rrdset_done(d->st_uavgsz);
}

#endif //NETDATA_DISK_SIZE_H
