// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_METRIC_CORRELATIONS_H
#define NETDATA_METRIC_CORRELATIONS_H 1

#include "web/api/queries/query.h"

typedef enum {
    METRIC_CORRELATIONS_KS2    = 1,
    METRIC_CORRELATIONS_VOLUME = 2,
} METRIC_CORRELATIONS_METHOD;

extern int enable_metric_correlations;
extern int metric_correlations_version;
extern METRIC_CORRELATIONS_METHOD default_metric_correlations_method;

extern int metric_correlations (RRDHOST *host, BUFFER *wb, METRIC_CORRELATIONS_METHOD method, RRDR_GROUPING group,
                               long long baseline_after, long long baseline_before,
                               long long after, long long before,
                               long long points, RRDR_OPTIONS options, int timeout);

extern METRIC_CORRELATIONS_METHOD mc_string_to_method(const char *method);
extern const char *mc_method_to_string(METRIC_CORRELATIONS_METHOD method);
extern int mc_unittest(void);

#endif //NETDATA_METRIC_CORRELATIONS_H
