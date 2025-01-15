// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRD_ALGORITHM_H
#define NETDATA_RRD_ALGORITHM_H

#include "libnetdata/libnetdata.h"

typedef enum __attribute__ ((__packed__)) rrd_algorithm {
    RRD_ALGORITHM_ABSOLUTE              = 0,
    RRD_ALGORITHM_INCREMENTAL           = 1,
    RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL = 2,
    RRD_ALGORITHM_PCENT_OVER_ROW_TOTAL  = 3,

    // this is 8-bit
} RRD_ALGORITHM;

#define RRD_ALGORITHM_ABSOLUTE_NAME                "absolute"
#define RRD_ALGORITHM_INCREMENTAL_NAME             "incremental"
#define RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL_NAME   "percentage-of-incremental-row"
#define RRD_ALGORITHM_PCENT_OVER_ROW_TOTAL_NAME    "percentage-of-absolute-row"

RRD_ALGORITHM rrd_algorithm_id(const char *name);
const char *rrd_algorithm_name(RRD_ALGORITHM algorithm);


#endif //NETDATA_RRD_ALGORITHM_H
