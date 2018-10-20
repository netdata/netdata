// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_API_DATA_QUERY_H
#define NETDATA_API_DATA_QUERY_H

#include "rrdr.h"
#include "web/api/queries/average/average.h"
#include "incremental_sum/incremental_sum.h"
#include "max/max.h"
#include "min/min.h"
#include "sum/sum.h"

extern RRDR *rrd2rrdr(RRDSET *st, long points, long long after, long long before, int group_method, long group_time, int aligned);

#endif //NETDATA_API_DATA_QUERY_H
