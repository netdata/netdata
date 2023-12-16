// SPDX-License-Identifier: GPL-3.0-or-later
#ifndef NETDATA_RRDFUNCTIONS_H
#define NETDATA_RRDFUNCTIONS_H 1

// ----------------------------------------------------------------------------

#include "rrd.h"

#define RRDFUNCTIONS_PRIORITY_DEFAULT 100

#define RRDFUNCTIONS_TIMEOUT_EXTENSION_UT (1 * USEC_PER_SEC)

typedef void (*rrd_function_result_callback_t)(BUFFER *wb, int code, void *result_cb_data);
typedef bool (*rrd_function_is_cancelled_cb_t)(void *is_cancelled_cb_data);
typedef void (*rrd_function_cancel_cb_t)(void *data);
typedef void (*rrd_function_register_canceller_cb_t)(void *register_cancel_cb_data, rrd_function_cancel_cb_t cancel_cb, void *cancel_cb_data);
typedef void (*rrd_function_progress_cb_t)(void *data, size_t done, size_t all);
typedef void (*rrd_function_progresser_cb_t)(void *data);
typedef void (*rrd_function_register_progresser_cb_t)(void *register_progresser_cb_data, rrd_function_progresser_cb_t progresser_cb, void *progresser_cb_data);

typedef int (*rrd_function_execute_cb_t)(uuid_t *transaction, BUFFER *wb,
                                         usec_t *stop_monotonic_ut, const char *function, void *collector_data,
                                         rrd_function_result_callback_t result_cb, void *result_cb_data,
                                         rrd_function_progress_cb_t progress_cb, void *progress_cb_data,
                                         rrd_function_is_cancelled_cb_t is_cancelled_cb, void *is_cancelled_cb_data,
                                         rrd_function_register_canceller_cb_t register_canceller_cb, void *register_canceller_cb_data,
                                         rrd_function_register_progresser_cb_t register_progresser_cb, void *register_progresser_cb_data);

void rrd_functions_inflight_init(void);
void rrdfunctions_host_init(RRDHOST *host);
void rrdfunctions_host_destroy(RRDHOST *host);

// add a function, to be run from the collector
void rrd_function_add(RRDHOST *host, RRDSET *st, const char *name, int timeout, int priority, const char *help, const char *tags,
                      HTTP_ACCESS access, bool sync, rrd_function_execute_cb_t execute_cb,
                      void *execute_cb_data);

// call a function, to be run from anywhere
int rrd_function_run(RRDHOST *host, BUFFER *result_wb, int timeout_s, HTTP_ACCESS access, const char *cmd,
                     bool wait, const char *transaction,
                     rrd_function_result_callback_t result_cb, void *result_cb_data,
                     rrd_function_progress_cb_t progress_cb, void *progress_cb_data,
                     rrd_function_is_cancelled_cb_t is_cancelled_cb, void *is_cancelled_cb_data, const char *payload);

// cancel a running function, to be run from anywhere
void rrd_function_cancel(const char *transaction);
void rrd_function_progress(const char *transaction);
void rrd_function_call_progresser(uuid_t *transaction);

void rrd_functions_expose_rrdpush(RRDSET *st, BUFFER *wb);
void rrd_functions_expose_global_rrdpush(RRDHOST *host, BUFFER *wb);

void chart_functions2json(RRDSET *st, BUFFER *wb);
void chart_functions_to_dict(DICTIONARY *rrdset_functions_view, DICTIONARY *dst, void *value, size_t value_size);
void host_functions_to_dict(RRDHOST *host, DICTIONARY *dst, void *value, size_t value_size, STRING **help, STRING **tags, HTTP_ACCESS *access, int *priority);
void host_functions2json(RRDHOST *host, BUFFER *wb);

uint8_t functions_format_to_content_type(const char *format);
const char *functions_content_type_to_format(HTTP_CONTENT_TYPE content_type);
int rrd_call_function_error(BUFFER *wb, const char *msg, int code);

int rrdhost_function_progress(uuid_t *transaction, BUFFER *wb,
                              usec_t *stop_monotonic_ut, const char *function, void *collector_data,
                              rrd_function_result_callback_t result_cb, void *result_cb_data,
                              rrd_function_progress_cb_t progress_cb, void *progress_cb_data,
                              rrd_function_is_cancelled_cb_t is_cancelled_cb, void *is_cancelled_cb_data,
                              rrd_function_register_canceller_cb_t register_canceller_cb, void *register_canceller_cb_data,
                              rrd_function_register_progresser_cb_t register_progresser_cb,
                              void *register_progresser_cb_data);

int rrdhost_function_streaming(uuid_t *transaction, BUFFER *wb,
                               usec_t *stop_monotonic_ut, const char *function, void *collector_data,
                               rrd_function_result_callback_t result_cb, void *result_cb_data,
                               rrd_function_progress_cb_t progress_cb, void *progress_cb_data,
                               rrd_function_is_cancelled_cb_t is_cancelled_cb, void *is_cancelled_cb_data,
                               rrd_function_register_canceller_cb_t register_canceller_cb, void *register_canceller_cb_data,
                               rrd_function_register_progresser_cb_t register_progresser_cb,
                               void *register_progresser_cb_data);

#define RRDFUNCTIONS_STREAMING_HELP "Streaming status for parents and children."

#endif // NETDATA_RRDFUNCTIONS_H
