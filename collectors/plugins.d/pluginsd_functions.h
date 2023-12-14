// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PLUGINSD_FUNCTIONS_H
#define NETDATA_PLUGINSD_FUNCTIONS_H

#include "pluginsd_internals.h"

struct inflight_function {
    uuid_t transaction;

    int code;
    int timeout_s;
    STRING *function;
    BUFFER *result_body_wb;
    usec_t *stop_monotonic_ut; // pointer to caller data
    usec_t started_monotonic_ut;
    usec_t sent_monotonic_ut;
    const char *payload;
    PARSER *parser;
    bool virtual;

    struct {
        rrd_function_result_callback_t cb;
        void *data;
    } result;

    struct {
        rrd_function_progress_cb_t cb;
        void *data;
    } progress;

    struct {
        usec_t stop_monotonic_ut;
    } dyncfg;
};

PARSER_RC pluginsd_function(char **words, size_t num_words, PARSER *parser);
PARSER_RC pluginsd_function_result_begin(char **words, size_t num_words, PARSER *parser);
PARSER_RC pluginsd_function_progress(char **words, size_t num_words, PARSER *parser);

void pluginsd_inflight_functions_init(PARSER *parser);
void pluginsd_inflight_functions_cleanup(PARSER *parser);
void pluginsd_inflight_functions_garbage_collect(PARSER  *parser, usec_t now_ut);

#endif //NETDATA_PLUGINSD_FUNCTIONS_H
