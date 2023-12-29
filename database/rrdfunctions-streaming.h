// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDFUNCTIONS_STREAMING_H
#define NETDATA_RRDFUNCTIONS_STREAMING_H

#include "rrd.h"

int rrdhost_function_streaming(uuid_t *transaction, BUFFER *wb,
                               usec_t *stop_monotonic_ut, const char *function, void *collector_data,
                               rrd_function_result_callback_t result_cb, void *result_cb_data,
                               rrd_function_progress_cb_t progress_cb, void *progress_cb_data,
                               rrd_function_is_cancelled_cb_t is_cancelled_cb, void *is_cancelled_cb_data,
                               rrd_function_register_canceller_cb_t register_canceller_cb, void *register_canceller_cb_data,
                               rrd_function_register_progresser_cb_t register_progresser_cb,
                               void *register_progresser_cb_data);

#endif //NETDATA_RRDFUNCTIONS_STREAMING_H
