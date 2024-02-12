// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_API_FORMATTER_VALUE_H
#define NETDATA_API_FORMATTER_VALUE_H

#include "../rrd2json.h"

typedef struct storage_value {
    NETDATA_DOUBLE value;
    NETDATA_DOUBLE anomaly_rate;
    time_t after;
    time_t before;
    size_t points_read;
    size_t storage_points_per_tier[RRD_STORAGE_TIERS];
    size_t result_points;
    STORAGE_POINT sp;
    usec_t duration_ut;
} QUERY_VALUE;

struct rrdmetric_acquired;
struct rrdinstance_acquired;
struct rrdcontext_acquired;

QUERY_VALUE rrdmetric2value(RRDHOST *host,
                            struct rrdcontext_acquired *rca, struct rrdinstance_acquired *ria, struct rrdmetric_acquired *rma,
                            time_t after, time_t before,
                            RRDR_OPTIONS options, RRDR_TIME_GROUPING time_group_method, const char *time_group_options,
                            size_t tier, time_t timeout, QUERY_SOURCE query_source, STORAGE_PRIORITY priority
);

NETDATA_DOUBLE rrdr2value(RRDR *r, long i, RRDR_OPTIONS options, int *all_values_are_null, NETDATA_DOUBLE *anomaly_rate);

#endif //NETDATA_API_FORMATTER_VALUE_H
