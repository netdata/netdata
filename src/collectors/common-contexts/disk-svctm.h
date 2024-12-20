// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DISK_SVCTM_H
#define NETDATA_DISK_SVCTM_H

#include "common-contexts.h"

typedef struct {
    RRDSET *st_svctm;
    RRDDIM *rd_svctm;
} ND_DISK_SVCTM;

static inline void common_disk_svctm(ND_DISK_SVCTM *d, const char *id, const char *name, double svctm_ms, int update_every, instance_labels_cb_t cb, void *data) {
    if(unlikely(!d->st_svctm)) {
        d->st_svctm = rrdset_create_localhost(
            "disk_svctm"
            , id
            , name
            , "latency"
            , "disk.svctm"
            , "Average Service Time"
            , "milliseconds/operation"
            , _COMMON_PLUGIN_NAME
            , _COMMON_PLUGIN_MODULE_NAME
            , NETDATA_CHART_PRIO_DISK_SVCTM
            , update_every
            , RRDSET_TYPE_LINE
        );

        d->rd_svctm  = rrddim_add(d->st_svctm, "svctm", NULL, 1, 1000, RRD_ALGORITHM_ABSOLUTE);

        if(cb)
            cb(d->st_svctm, data);
    }

    // this always have to be in base units, so that exporting sends base units to other time-series db
    rrddim_set_by_pointer(d->st_svctm, d->rd_svctm, (collected_number)(svctm_ms * 1000.0));
    rrdset_done(d->st_svctm);
}

#endif //NETDATA_DISK_SVCTM_H
