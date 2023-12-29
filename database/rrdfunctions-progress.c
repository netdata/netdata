// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdfunctions-progress.h"


int rrdhost_function_progress(uuid_t *transaction __maybe_unused, BUFFER *wb,
                              usec_t *stop_monotonic_ut __maybe_unused, const char *function __maybe_unused,
                              void *collector_data __maybe_unused,
                              rrd_function_result_callback_t result_cb, void *result_cb_data,
                              rrd_function_progress_cb_t progress_cb __maybe_unused, void *progress_cb_data __maybe_unused,
                              rrd_function_is_cancelled_cb_t is_cancelled_cb, void *is_cancelled_cb_data,
                              rrd_function_register_canceller_cb_t register_canceller_cb __maybe_unused,
                              void *register_canceller_cb_data __maybe_unused,
                              rrd_function_register_progresser_cb_t register_progresser_cb __maybe_unused,
                              void *register_progresser_cb_data __maybe_unused) {

    int response = progress_function_result(wb, rrdhost_hostname(localhost));

    if(is_cancelled_cb && is_cancelled_cb(is_cancelled_cb_data)) {
        buffer_flush(wb);
        response = HTTP_RESP_CLIENT_CLOSED_REQUEST;
    }

    if(result_cb)
        result_cb(wb, response, result_cb_data);

    return response;
}

