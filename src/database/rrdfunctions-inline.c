// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdfunctions-inline.h"

static int rrd_function_run_inline(struct rrd_function_execute *rfe, void *data) {

    // IMPORTANT: this function MUST call the result_cb even on failures

    rrd_function_execute_inline_cb_t execute_cb = data;

    int code;

    if(rfe->is_cancelled.cb && rfe->is_cancelled.cb(rfe->is_cancelled.data))
        code = HTTP_RESP_CLIENT_CLOSED_REQUEST;
    else
        code = execute_cb(rfe->result.wb, rfe->function, rfe->payload, rfe->source);

    if(code == HTTP_RESP_CLIENT_CLOSED_REQUEST || (rfe->is_cancelled.cb && rfe->is_cancelled.cb(rfe->is_cancelled.data))) {
        buffer_flush(rfe->result.wb);
        code = HTTP_RESP_CLIENT_CLOSED_REQUEST;
    }

    if(rfe->result.cb)
        rfe->result.cb(rfe->result.wb, code, rfe->result.data);

    return code;
}

void rrd_function_add_inline(RRDHOST *host, RRDSET *st, const char *name, int timeout, int priority, uint32_t version,
                             const char *help, const char *tags,
                             HTTP_ACCESS access, rrd_function_execute_inline_cb_t execute_cb) {

    rrd_collector_started(); // this creates a collector that runs for as long as netdata runs

    rrd_function_add(host, st, name, timeout, priority, version,
                     help, tags, access, true,
                     rrd_function_run_inline, execute_cb);
}
