// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_API_DATA_QUERY_H
#define NETDATA_API_DATA_QUERY_H

#include "../rrd2json.h"

#define GROUP_UNDEFINED         0
#define GROUP_AVERAGE           1
#define GROUP_MIN               2
#define GROUP_MAX               3
#define GROUP_SUM               4
#define GROUP_INCREMENTAL_SUM   5
#define GROUP_MEDIAN            6

#include "rrdr.h"
#include "web/api/queries/average/average.h"
#include "incremental_sum/incremental_sum.h"
#include "max/max.h"
#include "median/median.h"
#include "min/min.h"
#include "sum/sum.h"

extern RRDR *rrd2rrdr(RRDSET *st, long points, long long after, long long before, int group_method, long group_time, int aligned);

#endif //NETDATA_API_DATA_QUERY_H
