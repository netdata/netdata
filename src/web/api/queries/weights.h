// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_API_WEIGHTS_H
#define NETDATA_API_WEIGHTS_H 1

#include "query.h"

typedef enum {
    WEIGHTS_METHOD_MC_KS2       = 1,
    WEIGHTS_METHOD_MC_VOLUME    = 2,
    WEIGHTS_METHOD_ANOMALY_RATE = 3,
    WEIGHTS_METHOD_VALUE        = 4,
} WEIGHTS_METHOD;

typedef enum {
    WEIGHTS_FORMAT_CHARTS    = 1,
    WEIGHTS_FORMAT_CONTEXTS  = 2,
    WEIGHTS_FORMAT_MULTINODE = 3,
} WEIGHTS_FORMAT;

extern int metric_correlations_version;

typedef bool (*weights_interrupt_callback_t)(void *data);

typedef struct query_weights_request {
    size_t version;
    RRDHOST *host;
    const char *scope_nodes;
    const char *scope_contexts;
    const char *nodes;
    const char *contexts;
    const char *instances;
    const char *dimensions;
    const char *labels;
    const char *alerts;

    struct {
        RRDR_GROUP_BY group_by;
        char *group_by_label;
        RRDR_GROUP_BY_FUNCTION aggregation;
    } group_by;

    WEIGHTS_METHOD method;
    WEIGHTS_FORMAT format;
    RRDR_TIME_GROUPING time_group_method;
    const char *time_group_options;
    time_t baseline_after;
    time_t baseline_before;
    time_t after;
    time_t before;
    size_t points;
    RRDR_OPTIONS options;
    size_t tier;
    time_t timeout_ms;

    weights_interrupt_callback_t interrupt_callback;
    void *interrupt_callback_data;

    nd_uuid_t *transaction;
} QUERY_WEIGHTS_REQUEST;

int web_api_v12_weights(BUFFER *wb, QUERY_WEIGHTS_REQUEST *qwr);

WEIGHTS_METHOD weights_string_to_method(const char *method);
const char *weights_method_to_string(WEIGHTS_METHOD method);
int mc_unittest(void);

#endif //NETDATA_API_WEIGHTS_H
