// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdfunctions-inline.h"

struct rrd_function_inline {
    rrd_function_execute_inline_cb_t cb;
};

static int rrd_function_run_inline(uuid_t *transaction __maybe_unused, BUFFER *wb, BUFFER *payload __maybe_unused,
                            usec_t *stop_monotonic_ut __maybe_unused, const char *function __maybe_unused,
                            void *execute_cb_data __maybe_unused,
                            rrd_function_result_callback_t result_cb, void *result_cb_data,
                            rrd_function_progress_cb_t progress_cb __maybe_unused, void *progress_cb_data __maybe_unused,
                            rrd_function_is_cancelled_cb_t is_cancelled_cb, void *is_cancelled_cb_data,
                            rrd_function_register_canceller_cb_t register_canceller_cb __maybe_unused,
                            void *register_canceller_cb_data __maybe_unused,
                            rrd_function_register_progresser_cb_t register_progresser_cb __maybe_unused,
                            void *register_progresser_cb_data __maybe_unused) {

    struct rrd_function_inline *fi = execute_cb_data;

    int response = fi->cb(wb, function);

    if(is_cancelled_cb && is_cancelled_cb(is_cancelled_cb_data)) {
        buffer_flush(wb);
        response = HTTP_RESP_CLIENT_CLOSED_REQUEST;
    }

    if(result_cb)
        result_cb(wb, response, result_cb_data);

    return response;
}

void rrd_function_add_inline(RRDHOST *host, RRDSET *st, const char *name, int timeout, int priority, const char *help, const char *tags,
                      HTTP_ACCESS access, rrd_function_execute_inline_cb_t execute_cb) {

    rrd_collector_started(); // this creates a collector that runs for as long as netdata runs

    struct rrd_function_inline *fi = callocz(1, sizeof(struct rrd_function_inline));
    fi->cb = execute_cb;

    rrd_function_add(host, st, name, timeout, priority, help, tags, access, true, rrd_function_run_inline, fi);
}
