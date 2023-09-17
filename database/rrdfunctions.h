// SPDX-License-Identifier: GPL-3.0-or-later
#ifndef NETDATA_RRDFUNCTIONS_H
#define NETDATA_RRDFUNCTIONS_H 1

// ----------------------------------------------------------------------------

#include "rrd.h"

typedef void (*rrd_function_result_callback_t)(BUFFER *wb, int code, void *result_cb_data);
typedef bool (*rrd_function_is_cancelled_cb_t)(void *is_cancelled_cb_data);
typedef void (*rrd_function_canceller_cb_t)(void *data);
typedef void (*rrd_function_register_canceller_cb_t)(void *register_cancel_cb_data, rrd_function_canceller_cb_t cancel_cb, void *cancel_cb_data);
typedef int (*rrd_function_execute_cb_t)(BUFFER *wb, int timeout, const char *function, void *collector_data,
                                         rrd_function_result_callback_t result_cb, void *result_cb_data,
                                         rrd_function_is_cancelled_cb_t is_cancelled_cb, void *is_cancelled_cb_data,
                                         rrd_function_register_canceller_cb_t register_cancel_cb, void *register_cancel_db_data);

void rrd_functions_inflight_init(void);
void rrdfunctions_host_init(RRDHOST *host);
void rrdfunctions_host_destroy(RRDHOST *host);

void rrd_collector_started(void);
void rrd_collector_finished(void);

// add a function, to be run from the collector
void rrd_function_add(RRDHOST *host, RRDSET *st, const char *name, int timeout, const char *help,
                      bool sync, rrd_function_execute_cb_t execute_cb, void *execute_cb_data);

// call a function, to be run from anywhere
int rrd_function_run(RRDHOST *host, BUFFER *result_wb, int timeout, const char *cmd,
                     bool wait, const char *transaction,
                     rrd_function_result_callback_t result_cb, void *result_cb_data,
                     rrd_function_is_cancelled_cb_t is_cancelled_cb, void *is_cancelled_cb_data);

// cancel a running function, to be run from anywhere
void rrd_function_cancel(const char *transaction);

void rrd_functions_expose_rrdpush(RRDSET *st, BUFFER *wb);
void rrd_functions_expose_global_rrdpush(RRDHOST *host, BUFFER *wb);

void chart_functions2json(RRDSET *st, BUFFER *wb, int tabs, const char *kq, const char *sq);
void chart_functions_to_dict(DICTIONARY *rrdset_functions_view, DICTIONARY *dst, void *value, size_t value_size);
void host_functions_to_dict(RRDHOST *host, DICTIONARY *dst, void *value, size_t value_size, STRING **help);
void host_functions2json(RRDHOST *host, BUFFER *wb);

uint8_t functions_format_to_content_type(const char *format);
const char *functions_content_type_to_format(HTTP_CONTENT_TYPE content_type);
int rrd_call_function_error(BUFFER *wb, const char *msg, int code);

int rrdhost_function_streaming(BUFFER *wb, int timeout, const char *function, void *collector_data,
                               rrd_function_result_callback_t result_cb, void *result_cb_data,
                               rrd_function_is_cancelled_cb_t is_cancelled_cb, void *is_cancelled_cb_data,
                               rrd_function_register_canceller_cb_t register_cancel_cb, void *register_cancel_db_data);

#define RRDFUNCTIONS_STREAMING_HELP "Streaming status for parents and children."

#endif // NETDATA_RRDFUNCTIONS_H
