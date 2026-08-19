// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PLUGINSD_FUNCTIONS_H
#define NETDATA_PLUGINSD_FUNCTIONS_H

#include "pluginsd_internals.h"
#include "nrpc/nrpc-transport.h"

struct pluginsd_call {
    nd_uuid_t transaction;

    int code;
    int timeout_s;
    STRING *function;
    BUFFER *payload;
    HTTP_ACCESS access;
    const char *source;

    BUFFER *result_body_wb;

    // NO deadline pointer here: the transport reads deadlines exclusively
    // through the table-keyed accessor nrpc_call_deadline()
    // (the parser dict key IS the compact transaction id)
    usec_t started_monotonic_ut;
    usec_t sent_monotonic_ut;
    PARSER *parser;

    bool sent_successfully;

    // written only under the parser dict write lock: the GC pass that cancels
    // this record and queues its deletion sets it, so a second GC pass entering
    // between that pass's unlock and its del cannot re-send FUNCTION_CANCEL
    bool gc_collected;

    struct {
        nrpc_result_cb_t cb;
        void *data;
    } result;

    struct {
        nrpc_progress_cb_t cb;
        void *data;
    } progress;
};

PARSER_RC pluginsd_function(char **words, size_t num_words, PARSER *parser);
PARSER_RC pluginsd_function_del(char **words, size_t num_words, PARSER *parser);
PARSER_RC pluginsd_function_result_begin(char **words, size_t num_words, PARSER *parser);
PARSER_RC pluginsd_function_progress(char **words, size_t num_words, PARSER *parser);

void pluginsd_calls_init(PARSER *parser);
void pluginsd_calls_cleanup(PARSER *parser);
void pluginsd_calls_release_deferred(PARSER *parser);
void pluginsd_calls_garbage_collect(PARSER  *parser, usec_t now_ut);

int pluginsd_nrpc_handler(struct nrpc_request *req, void *data);

#endif //NETDATA_PLUGINSD_FUNCTIONS_H
