// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PLUGINSD_FUNCTIONS_H
#define NETDATA_PLUGINSD_FUNCTIONS_H

#include "pluginsd_internals.h"

struct inflight_function {
    int code;
    int timeout;
    STRING *function;
    BUFFER *result_body_wb;
    rrd_function_result_callback_t result_cb;
    void *result_cb_data;
    usec_t timeout_ut;
    usec_t started_ut;
    usec_t sent_ut;
    const char *payload;
    PARSER *parser;
    bool virtual;
};

PARSER_RC pluginsd_function(char **words, size_t num_words, PARSER *parser);
PARSER_RC pluginsd_function_result_begin(char **words, size_t num_words, PARSER *parser);

void inflight_functions_garbage_collect(PARSER  *parser, usec_t now);

#endif //NETDATA_PLUGINSD_FUNCTIONS_H
