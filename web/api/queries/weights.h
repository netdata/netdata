// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_API_WEIGHTS_H
#define NETDATA_API_WEIGHTS_H 1

#include "query.h"

typedef enum {
    WEIGHTS_METHOD_MC_KS2       = 1,
    WEIGHTS_METHOD_MC_VOLUME    = 2,
    WEIGHTS_METHOD_ANOMALY_RATE = 3,
} WEIGHTS_METHOD;

typedef enum {
    WEIGHTS_FORMAT_CHARTS    = 1,
    WEIGHTS_FORMAT_CONTEXTS  = 2,
} WEIGHTS_FORMAT;

extern int enable_metric_correlations;
extern int metric_correlations_version;
extern WEIGHTS_METHOD default_metric_correlations_method;

int web_api_v1_weights (RRDHOST *host, BUFFER *wb, WEIGHTS_METHOD method, WEIGHTS_FORMAT format,
                               RRDR_GROUPING group, const char *group_options,
                               time_t baseline_after, time_t baseline_before,
                               time_t after, time_t before,
                               size_t points, RRDR_OPTIONS options, SIMPLE_PATTERN *contexts, size_t tier, size_t timeout);

WEIGHTS_METHOD weights_string_to_method(const char *method);
const char *weights_method_to_string(WEIGHTS_METHOD method);

typedef long int DIFFS_NUMBERS;

double ks_2samp(DIFFS_NUMBERS baseline_diffs[], int base_size,
                DIFFS_NUMBERS highlight_diffs[], int high_size,
                uint32_t base_shifts);

#endif //NETDATA_API_WEIGHTS_H
