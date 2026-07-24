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
typedef void (*rrd_function_progress_cb_t)(nd_uuid_t *transaction, void *data, size_t done, size_t all);
typedef void (*rrd_function_progresser_cb_t)(const char *transaction, void *data);
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

struct rrd_host_function;

void rrd_functions_host_init(RRDHOST *host);
void rrd_functions_host_destroy(RRDHOST *host);

// add a function, to be run from the collector
void rrd_function_add(RRDHOST *host, RRDSET *st, const char *name, int timeout, int priority, uint32_t version, const char *help, const char *tags,
                      HTTP_ACCESS access, bool sync, rrd_function_execute_cb_t execute_cb,
                      void *execute_cb_data);

bool rrd_function_del(RRDHOST *host, RRDSET *st, const char *name, bool from_streaming, bool internal);

// true if name is a reserved dynamic-configuration function name ("config" or "config <id>")
bool rrd_function_name_is_dyncfg(const char *name);

// call a function, to be run from anywhere
int rrd_function_run(RRDHOST *host, BUFFER *result_wb, int timeout_s,
                     HTTP_ACCESS user_access, const char *cmd,
                     bool wait, const char *transaction,
                     rrd_function_result_callback_t result_cb, void *result_cb_data,
                     rrd_function_progress_cb_t progress_cb, void *progress_cb_data,
                     rrd_function_is_cancelled_cb_t is_cancelled_cb, void *is_cancelled_cb_data,
                     BUFFER *payload, const char *source, bool allow_restricted);

// Verify the caller may invoke `cmd` on `host`, applying the same RESTRICTED and access-level
// checks rrd_function_run() enforces, WITHOUT executing the function. This lets non-execution
// paths (e.g. MCP metadata/help generation) authorize a caller before disclosing anything.
// On success returns HTTP_RESP_OK; if out_acquired != NULL it receives an acquired dictionary
// item the caller MUST release with dictionary_acquired_item_release(host->functions, *out_acquired),
// otherwise the item is released internally. On failure it writes the error into result_wb,
// releases any acquired item, sets *out_acquired (if any) to NULL, and returns the HTTP code.
int rrd_function_verify_access(RRDHOST *host, BUFFER *result_wb, const char *cmd,
                              HTTP_ACCESS user_access, bool allow_restricted,
                              const DICTIONARY_ITEM **out_acquired);

// Regression test for rrd_function_verify_access() access gating (GHSA-6628-vxm3-4g8g).
// Requires a prepared RRD (localhost). Returns the number of failures (0 = pass).
int rrdfunctions_verify_access_unittest(void);

bool rrd_function_available(RRDHOST *host, const char *function);
bool rrd_function_is_available(struct rrd_host_function *rdcf, RRDHOST *host);

bool rrd_function_has_this_original_result_callback(nd_uuid_t *transaction, rrd_function_result_callback_t cb);

#include "rrdfunctions-inline.h"
#include "rrdfunctions-inflight.h"
#include "rrdfunctions-exporters.h"

#endif // NETDATA_RRDFUNCTIONS_H
