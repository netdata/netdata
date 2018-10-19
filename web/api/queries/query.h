//
// Created by costa on 19/10/18.
//

#ifndef NETDATA_API_DATA_QUERY_H
#define NETDATA_API_DATA_QUERY_H

#include "rrdr.h"

extern RRDR *rrd2rrdr(RRDSET *st, long points, long long after, long long before, int group_method, long group_time, int aligned);

#endif //NETDATA_API_DATA_QUERY_H
