// SPDX-License-Identifier: GPL-3.0-or-later
#ifndef NETDATA_NRPC_H
#define NETDATA_NRPC_H 1

// ----------------------------------------------------------------------------

#include "libnetdata/libnetdata.h"

#define NRPC_PRIORITY_DEFAULT 100
#define NRPC_VERSION_DEFAULT 0
#define NRPC_TAG_HIDDEN "hidden"

#define NRPC_DEADLINE_GRACE_UT (1 * USEC_PER_SEC)

// the broker grants every deadline a grace extension before enforcing it;
// apply it through this helper only, so the policy lives in one place
static inline usec_t nrpc_effective_deadline_ut(usec_t stop_monotonic_ut) {
    return stop_monotonic_ut + NRPC_DEADLINE_GRACE_UT;
}

typedef void (*nrpc_result_cb_t)(BUFFER *wb, int code, void *result_cb_data);
typedef bool (*nrpc_is_cancelled_cb_t)(void *is_cancelled_cb_data);

// The canceller is keyed by call_id (daemon-internal contract: the sole
// implementer is the pluginsd transport; plugin-side functions_evloop cancels
// via a polled flag and registers no canceller). CONTRACT: cancellers and
// progressers are registered ONLY by function transports - their `data` is a
// struct nrpc_transport, and the broker entry-pins it for the record's
// lifetime.
typedef void (*nrpc_cancel_hook_cb_t)(const char *call_id, void *data);
typedef void (*nrpc_register_cancel_hook_cb_t)(void *register_cancel_cb_data, nrpc_cancel_hook_cb_t cancel_cb, void *cancel_cb_data);
typedef void (*nrpc_progress_cb_t)(nd_uuid_t *call_id, void *data, size_t done, size_t all);
typedef void (*nrpc_progress_hook_cb_t)(const char *call_id, void *data);
typedef void (*nrpc_register_progress_hook_cb_t)(void *register_progresser_cb_data, nrpc_progress_hook_cb_t progresser_cb, void *progresser_cb_data);

struct nrpc_request {
    nd_uuid_t *call_id;
    const char *function;
    BUFFER *payload;
    const char *source;

    HTTP_ACCESS user_access;

    // points into the broker record: valid ONLY for the duration of the
    // handler invocation - executors must not stash it; later deadline
    // reads go through nrpc_call_deadline()
    usec_t *stop_monotonic_ut;

    struct {
        BUFFER *wb; // the response should be written here
        nrpc_result_cb_t cb;
        void *data;
    } result;

    struct {
        nrpc_progress_cb_t cb;
        void *data;
    } progress;

    struct {
        nrpc_is_cancelled_cb_t cb;
        void *data;
    } is_cancelled;

    struct {
        nrpc_register_cancel_hook_cb_t cb;
        void *data;
    } register_cancel_hook;

    struct {
        nrpc_register_progress_hook_cb_t cb;
        void *data;
    } register_progress_hook;
};

typedef int (*nrpc_handler_cb_t)(struct nrpc_request *req, void *data);


// ----------------------------------------------------------------------------

#include "rrd.h"

struct nrpc_method;

// opaque handle to an acquired function registry entry - the entry cannot be
// freed while the handle is held (idiom: RRDSET_ACQUIRED, RRDHOST_ACQUIRED)
typedef struct nrpc_method_acquired NRPC_METHOD_ACQUIRED;

typedef enum __attribute__((packed)) {
    NRPC_METHOD_FLAG_LOCAL  = (1 << 0),
    NRPC_METHOD_FLAG_GLOBAL = (1 << 1),
    NRPC_METHOD_FLAG_DYNCFG = (1 << 2),
    NRPC_METHOD_FLAG_RESTRICTED = (1 << 3), // this function is restricted (hidden from user)

    // this is 8-bit
} NRPC_METHOD_FLAGS;

// who is registering (or unregistering) a function - the registry enforces
// per-source rules (e.g. only the dyncfg subsystem and streaming children may
// register dyncfg-reserved names) and, on delete, who may remove what
typedef enum __attribute__((packed)) {
    NRPC_SOURCE_PLUGIN = 0,  // a local plugin / collector thread (pluginsd FUNCTION)
    NRPC_SOURCE_STREAM,      // a streaming child's advertisement on this parent
    NRPC_SOURCE_DAEMON,       // daemon-internal registration (inline functions, dyncfg)
} NRPC_SOURCE;

void nrpc_registry_init(RRDHOST *host);
void nrpc_registry_destroy(RRDHOST *host);

// release a handle acquired via nrpc_method_authorize()
void nrpc_method_acquired_release(RRDHOST *host, NRPC_METHOD_ACQUIRED *acquired);

// add a function, to be run from the collector
void nrpc_method_register(RRDHOST *host, RRDSET *st, const char *name, int timeout, int priority, uint32_t version, const char *help, const char *tags,
                      HTTP_ACCESS access, bool sync, NRPC_SOURCE source,
                      nrpc_handler_cb_t handler, void *handler_data);

bool nrpc_method_unregister(RRDHOST *host, RRDSET *st, const char *name, NRPC_SOURCE source);

// true if name is a reserved dynamic-configuration function name ("config" or "config <id>")
bool nrpc_method_name_is_dyncfg(const char *name);

// call a function, to be run from anywhere
int nrpc_call(RRDHOST *host, BUFFER *result_wb, int timeout_s,
                     HTTP_ACCESS user_access, const char *cmd,
                     bool wait, const char *call_id,
                     nrpc_result_cb_t result_cb, void *result_cb_data,
                     nrpc_progress_cb_t progress_cb, void *progress_cb_data,
                     nrpc_is_cancelled_cb_t is_cancelled_cb, void *is_cancelled_cb_data,
                     BUFFER *payload, const char *source, bool allow_restricted);

// Verify the caller may invoke `cmd` on `host`, applying the same RESTRICTED and access-level
// checks nrpc_call() enforces, WITHOUT executing the function. This lets non-execution
// paths (e.g. MCP metadata/help generation) authorize a caller before disclosing anything.
// On success returns HTTP_RESP_OK; if out_acquired != NULL it receives an acquired registry
// handle the caller MUST release with nrpc_method_acquired_release(host, *out_acquired),
// otherwise the handle is released internally. On failure it writes the error into result_wb,
// releases any acquired handle, sets *out_acquired (if any) to NULL, and returns the HTTP code.
int nrpc_method_authorize(RRDHOST *host, BUFFER *result_wb, const char *cmd,
                              HTTP_ACCESS user_access, bool allow_restricted,
                              NRPC_METHOD_ACQUIRED **out_acquired);

// Regression test for nrpc_method_authorize() access gating (GHSA-6628-vxm3-4g8g).
// Requires a prepared RRD (localhost). Returns the number of failures (0 = pass).
int nrpc_access_unittest(void);
int nrpc_manifest_unittest(void);
int nrpc_manifest_pacer_unittest(void);
int nrpc_del_unittest(void);
int nrpc_registry_unittest(void);
int nrpc_catalog_unittest(void);

bool nrpc_method_available(RRDHOST *host, const char *function);
bool nrpc_method_is_available(RRDHOST *host, struct nrpc_method *method);

bool nrpc_call_has_result_cb(nd_uuid_t *call_id, nrpc_result_cb_t cb);

#include "nrpc-builtin.h"
#include "nrpc-calls.h"
#include "nrpc-catalog.h"

#endif // NETDATA_NRPC_H
