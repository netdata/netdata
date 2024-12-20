// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DISK_UTIL_H
#define NETDATA_DISK_UTIL_H

#include "common-contexts.h"

typedef struct {
    RRDSET *st_util;
    RRDDIM *rd_util;
} ND_DISK_UTIL;

static inline void common_disk_util(ND_DISK_UTIL *d, const char *id, const char *name, uint64_t percent, int update_every, instance_labels_cb_t cb, void *data) {
    if(unlikely(!d->st_util)) {
        d->st_util = rrdset_create_localhost(
                "disk_util"
                , id
                , name
                , "utilization"
                , "disk.util"
                , "Disk Utilization Time"
                , "% of time working"
                , _COMMON_PLUGIN_NAME
                , _COMMON_PLUGIN_MODULE_NAME
                , NETDATA_CHART_PRIO_DISK_UTIL
                , update_every
                , RRDSET_TYPE_AREA
        );

        d->rd_util  = rrddim_add(d->st_util, "utilization",  NULL,  1, 1, RRD_ALGORITHM_ABSOLUTE);

        if(cb)
            cb(d->st_util, data);
    }

    // this always have to be in base units, so that exporting sends base units to other time-series db
    rrddim_set_by_pointer(d->st_util, d->rd_util, (collected_number)percent);
    rrdset_done(d->st_util);
}

#endif //NETDATA_DISK_UTIL_H
