// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_API_V2_CONTEXTS_H
#define NETDATA_API_V2_CONTEXTS_H

#include "rrdcontext-internal.h"

typedef enum __attribute__ ((__packed__)) {
    FTS_MATCHED_NONE = 0,
    FTS_MATCHED_CONTEXT,
    FTS_MATCHED_INSTANCE,
    FTS_MATCHED_DIMENSION,
    FTS_MATCHED_LABEL,
    FTS_MATCHED_ALERT,
    FTS_MATCHED_ALERT_INFO,
    FTS_MATCHED_FAMILY,
    FTS_MATCHED_TITLE,
    FTS_MATCHED_UNITS,
} FTS_MATCH;

typedef struct full_text_search_index {
    size_t searches;
    size_t string_searches;
    size_t char_searches;
} FTS_INDEX;

struct contexts_v2_node {
    size_t ni;
    size_t contexts_matched;
    RRDHOST *host;
};

struct rrdcontext_to_json_v2_data {
    time_t now;

    BUFFER *wb;
    struct api_v2_contexts_request *request;

    CONTEXTS_V2_MODE mode;
    CONTEXTS_OPTIONS options;
    struct query_versions versions;

    struct {
        SIMPLE_PATTERN *scope_pattern;
        SIMPLE_PATTERN *pattern;
        size_t ni;
        DICTIONARY *dict; // the result set
    } nodes;

    struct {
        SIMPLE_PATTERN *scope_pattern;
        SIMPLE_PATTERN *pattern;
        size_t ci;
        DICTIONARY *dict; // the result set
    } contexts;

    struct {
        SIMPLE_PATTERN *alert_name_pattern;
        time_t alarm_id_filter;

        size_t ati;

        DICTIONARY *summary;
        DICTIONARY *alert_instances;

        DICTIONARY *by_type;
        DICTIONARY *by_component;
        DICTIONARY *by_classification;
        DICTIONARY *by_recipient;
        DICTIONARY *by_module;
    } alerts;

    struct {
        char host_node_id_str[UUID_STR_LEN];
        SIMPLE_PATTERN *pattern;
        FTS_INDEX fts;
    } q;

    struct {
        DICTIONARY *dict; // the result set
    } functions;

    struct {
        bool enabled;
        bool relative;
        time_t after;
        time_t before;
    } window;

    struct query_timings timings;
};

void agent_capabilities_to_json(BUFFER *wb, RRDHOST *host, const char *key);

#include "api_v2_contexts_alerts.h"

#endif //NETDATA_API_V2_CONTEXTS_H
