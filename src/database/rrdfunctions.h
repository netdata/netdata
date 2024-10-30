// SPDX-License-Identifier: GPL-3.0-or-later
#ifndef NETDATA_RRDFUNCTIONS_H
#define NETDATA_RRDFUNCTIONS_H 1

// ----------------------------------------------------------------------------

#include "libnetdata/libnetdata.h"

#define RRDFUNCTIONS_PRIORITY_DEFAULT 100
#define RRDFUNCTIONS_VERSION_DEFAULT 0
#define RRDFUNCTIONS_TAG_HIDDEN "hidden"

#define RRDFUNCTIONS_TIMEOUT_EXTENSION_UT (1 * USEC_PER_SEC)

typedef void (*rrd_function_result_callback_t)(BUFFER *wb, int code, void *result_cb_data);
typedef bool (*rrd_function_is_cancelled_cb_t)(void *is_cancelled_cb_data);
typedef void (*rrd_function_cancel_cb_t)(void *data);
typedef void (*rrd_function_register_canceller_cb_t)(void *register_cancel_cb_data, rrd_function_cancel_cb_t cancel_cb, void *cancel_cb_data);
typedef void (*rrd_function_progress_cb_t)(void *data, size_t done, size_t all);
typedef void (*rrd_function_progresser_cb_t)(void *data);
typedef void (*rrd_function_register_progresser_cb_t)(void *register_progresser_cb_data, rrd_function_progresser_cb_t progresser_cb, void *progresser_cb_data);

struct rrd_function_execute {
    nd_uuid_t *transaction;
    const char *function;
    BUFFER *payload;
    const char *source;

    HTTP_ACCESS user_access;

    usec_t *stop_monotonic_ut;

    struct {
        BUFFER *wb; // the response should be written here
        rrd_function_result_callback_t cb;
        void *data;
    } result;

    struct {
        rrd_function_progress_cb_t cb;
        void *data;
    } progress;

    struct {
        rrd_function_is_cancelled_cb_t cb;
        void *data;
    } is_cancelled;

    struct {
        rrd_function_register_canceller_cb_t cb;
        void *data;
    } register_canceller;

    struct {
        rrd_function_register_progresser_cb_t cb;
        void *data;
    } register_progresser;
};

typedef int (*rrd_function_execute_cb_t)(struct rrd_function_execute *rfe, void *data);


// ----------------------------------------------------------------------------

#include "rrd.h"

void rrd_functions_host_init(RRDHOST *host);
void rrd_functions_host_destroy(RRDHOST *host);

// add a function, to be run from the collector
void rrd_function_add(RRDHOST *host, RRDSET *st, const char *name, int timeout, int priority, uint32_t version, const char *help, const char *tags,
                      HTTP_ACCESS access, bool sync, rrd_function_execute_cb_t execute_cb,
                      void *execute_cb_data);

void rrd_function_del(RRDHOST *host, RRDSET *st, const char *name);

// call a function, to be run from anywhere
int rrd_function_run(RRDHOST *host, BUFFER *result_wb, int timeout_s,
                     HTTP_ACCESS user_access, const char *cmd,
                     bool wait, const char *transaction,
                     rrd_function_result_callback_t result_cb, void *result_cb_data,
                     rrd_function_progress_cb_t progress_cb, void *progress_cb_data,
                     rrd_function_is_cancelled_cb_t is_cancelled_cb, void *is_cancelled_cb_data,
                     BUFFER *payload, const char *source, bool allow_restricted);

bool rrd_function_available(RRDHOST *host, const char *function);

bool rrd_function_has_this_original_result_callback(nd_uuid_t *transaction, rrd_function_result_callback_t cb);

#include "rrdfunctions-inline.h"
#include "rrdfunctions-inflight.h"
#include "rrdfunctions-exporters.h"

#endif // NETDATA_RRDFUNCTIONS_H
