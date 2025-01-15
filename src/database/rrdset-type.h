// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDSET_TYPE_H
#define NETDATA_RRDSET_TYPE_H

#include "libnetdata/libnetdata.h"

typedef enum __attribute__ ((__packed__)) rrdset_type {
    RRDSET_TYPE_LINE    = 0,
    RRDSET_TYPE_AREA    = 1,
    RRDSET_TYPE_STACKED = 2,
    RRDSET_TYPE_HEATMAP = 3,
} RRDSET_TYPE;

#define RRDSET_TYPE_LINE_NAME "line"
#define RRDSET_TYPE_AREA_NAME "area"
#define RRDSET_TYPE_STACKED_NAME "stacked"
#define RRDSET_TYPE_HEATMAP_NAME "heatmap"

RRDSET_TYPE rrdset_type_id(const char *name);
const char *rrdset_type_name(RRDSET_TYPE chart_type);

#endif //NETDATA_RRDSET_TYPE_H
