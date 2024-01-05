// SPDX-License-Identifier: GPL-3.0-or-later
#ifndef NETDATA_RRDFUNCTIONS_H
#define NETDATA_RRDFUNCTIONS_H 1

// ----------------------------------------------------------------------------

#include "../libnetdata/libnetdata.h"

#define RRDFUNCTIONS_PRIORITY_DEFAULT 100

#define RRDFUNCTIONS_TIMEOUT_EXTENSION_UT (1 * USEC_PER_SEC)

typedef void (*rrd_function_result_callback_t)(BUFFER *wb, int code, void *result_cb_data);
typedef bool (*rrd_function_is_cancelled_cb_t)(void *is_cancelled_cb_data);
typedef void (*rrd_function_cancel_cb_t)(void *data);
typedef void (*rrd_function_register_canceller_cb_t)(void *register_cancel_cb_data, rrd_function_cancel_cb_t cancel_cb, void *cancel_cb_data);
typedef void (*rrd_function_progress_cb_t)(void *data, size_t done, size_t all);
typedef void (*rrd_function_progresser_cb_t)(void *data);
typedef void (*rrd_function_register_progresser_cb_t)(void *register_progresser_cb_data, rrd_function_progresser_cb_t progresser_cb, void *progresser_cb_data);

typedef int (*rrd_function_execute_cb_t)(uuid_t *transaction, BUFFER *wb, BUFFER *payload,
                                         usec_t *stop_monotonic_ut, const char *function, void *collector_data,
                                         rrd_function_result_callback_t result_cb, void *result_cb_data,
                                         rrd_function_progress_cb_t progress_cb, void *progress_cb_data,
                                         rrd_function_is_cancelled_cb_t is_cancelled_cb, void *is_cancelled_cb_data,
                                         rrd_function_register_canceller_cb_t register_canceller_cb, void *register_canceller_cb_data,
                                         rrd_function_register_progresser_cb_t register_progresser_cb, void *register_progresser_cb_data);

// ----------------------------------------------------------------------------

#ifdef RRD_FUNCTIONS_INTERNALS

typedef enum __attribute__((packed)) {
    RRD_FUNCTION_LOCAL  = (1 << 0),
    RRD_FUNCTION_GLOBAL = (1 << 1),
    RRD_FUNCTION_DYNCFG = (1 << 2),

    // this is 8-bit
} RRD_FUNCTION_OPTIONS;

struct rrd_host_function {
    bool sync;                      // when true, the function is called synchronously
    RRD_FUNCTION_OPTIONS options;   // RRD_FUNCTION_OPTIONS
    HTTP_ACCESS access;
    STRING *help;
    STRING *tags;
    int timeout;                    // the default timeout of the function
    int priority;

    rrd_function_execute_cb_t execute_cb;
    void *execute_cb_data;

    struct rrd_collector *collector;
};

size_t rrd_functions_sanitize(char *dst, const char *src, size_t dst_len);
int rrd_functions_find_by_name(RRDHOST *host, BUFFER *wb, const char *name, size_t key_length, const DICTIONARY_ITEM **item);

#endif // RRD_FUNCTIONS_INTERNALS

// ----------------------------------------------------------------------------

#include "rrd.h"

void rrd_functions_host_init(RRDHOST *host);
void rrd_functions_host_destroy(RRDHOST *host);

// add a function, to be run from the collector
void rrd_function_add(RRDHOST *host, RRDSET *st, const char *name, int timeout, int priority, const char *help, const char *tags,
                      HTTP_ACCESS access, bool sync, rrd_function_execute_cb_t execute_cb,
                      void *execute_cb_data);

// call a function, to be run from anywhere
int rrd_function_run(RRDHOST *host, BUFFER *result_wb, int timeout_s, HTTP_ACCESS access, const char *cmd,
                     bool wait, const char *transaction,
                     rrd_function_result_callback_t result_cb, void *result_cb_data,
                     rrd_function_progress_cb_t progress_cb, void *progress_cb_data,
                     rrd_function_is_cancelled_cb_t is_cancelled_cb, void *is_cancelled_cb_data, BUFFER *payload);

int rrd_call_function_error(BUFFER *wb, const char *msg, int code);

#include "rrdfunctions-inline.h"
#include "rrdfunctions-inflight.h"
#include "rrdfunctions-exporters.h"
#include "rrdfunctions-streaming.h"
#include "rrdfunctions-progress.h"

#endif // NETDATA_RRDFUNCTIONS_H
