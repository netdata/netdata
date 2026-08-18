// SPDX-License-Identifier: GPL-3.0-or-later
#ifndef NETDATA_NRPC_H
#define NETDATA_NRPC_H 1

// ----------------------------------------------------------------------------
// nRPC - Netdata's ad-hoc RPC system
//
// nRPC is the mechanism behind the product feature called "Functions": named
// callable endpoints that plugins, streaming children and the daemon itself
// expose, and that users, parents and Netdata Cloud invoke on demand.
//
// Concept map:
// - registry (struct nrpc_registry): per-host table of methods.
// - method (struct nrpc_method): a named callable endpoint. Its static
//   attribute bundle (name, help, tags, access, timeout, priority, version)
//   is its DESCRIPTOR - never "metadata", which RPC reserves for per-call
//   headers. Registration takes a caller-filled desc struct
//   (struct nrpc_method_desc, struct nrpc_builtin_desc) that extends the
//   attribute bundle with the method's owner (host), scope (st) and handler.
// - call: one invocation of a method, tracked in the in-flight calls table
//   (struct nrpc_inflight_calls) and correlated by a call_id. Calls carry a
//   deadline and support cancel and progress.
// - handler (nrpc_handler_cb_t): the code that executes a method; it receives
//   a struct nrpc_request.
// - transport (struct nrpc_transport): carries calls to remote handlers
//   (pluginsd for local plugins, streaming for children).
// - serving handle (struct nrpc_serving_handle): the liveness token of the
//   thread serving a method - when that thread exits, its methods become
//   unavailable.
// - builtin: a daemon-implemented synchronous method (NRPC_SOURCE_DAEMON).
// - catalog (nrpc-catalog.c): publishes the registry to users, parents and
//   Cloud.
//
// Wire <-> code terminology: the pluginsd/streaming wire and every plugin SDK
// name the correlation id "transaction" - the SAME value this code names
// call_id. The wire keywords (FUNCTION, FUNCTION_DEL, FUNCTION_CANCEL,
// FUNCTION_PROGRESS, FUNCTION_RESULT_BEGIN/END, FUNCTION_PAYLOAD*), the JSON
// fields, the HTTP endpoints and the registered method-name strings are
// contracts and keep the "function" vocabulary; this code says
// method/call/handler for the mechanism, and "function" only at those
// boundaries. dyncfg is an application built on top (the reserved "config"
// method) and is not part of this component.

#include "libnetdata/libnetdata.h"

#define NRPC_PRIORITY_DEFAULT 100
#define NRPC_VERSION_DEFAULT 0
#define NRPC_TAG_HIDDEN "hidden"

#define NRPC_DEADLINE_GRACE_UT (1 * USEC_PER_SEC)

// the in-flight calls table grants every deadline a grace extension before enforcing it;
// apply it through this helper only, so the policy lives in one place
static inline usec_t nrpc_effective_deadline_ut(usec_t stop_monotonic_ut) {
    return stop_monotonic_ut + NRPC_DEADLINE_GRACE_UT;
}

typedef void (*nrpc_result_cb_t)(BUFFER *wb, int code, void *result_cb_data);
typedef bool (*nrpc_is_cancelled_cb_t)(void *is_cancelled_cb_data);

// The cancel hook is keyed by call_id (daemon-internal contract: the sole
// implementer is the pluginsd transport; plugin-side functions_evloop cancels
// via a polled flag and registers no cancel hook). CONTRACT: cancel and
// progress hooks are registered ONLY by transports - their `data` is a
// struct nrpc_transport, and the in-flight calls table entry-pins it for the record's
// lifetime.
typedef void (*nrpc_cancel_hook_cb_t)(const char *call_id, void *data);
typedef void (*nrpc_register_cancel_hook_cb_t)(void *register_cancel_hook_cb_data, nrpc_cancel_hook_cb_t cancel_hook_cb, void *cancel_hook_cb_data);
typedef void (*nrpc_progress_cb_t)(nd_uuid_t *call_id, void *data, size_t done, size_t all);
typedef void (*nrpc_progress_hook_cb_t)(const char *call_id, void *data);
typedef void (*nrpc_register_progress_hook_cb_t)(void *register_progress_hook_cb_data, nrpc_progress_hook_cb_t progress_hook_cb, void *progress_hook_cb_data);

struct nrpc_request {
    nd_uuid_t *call_id;
    const char *function;
    BUFFER *payload;
    const char *source;

    HTTP_ACCESS user_access;

    // points into the in-flight call record: valid ONLY for the duration of the
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

// opaque handle to an acquired method entry - the entry cannot be
// freed while the handle is held (idiom: RRDSET_ACQUIRED, RRDHOST_ACQUIRED)
typedef struct nrpc_method_acquired NRPC_METHOD_ACQUIRED;

typedef enum __attribute__((packed)) {
    NRPC_METHOD_FLAG_LOCAL  = (1 << 0),
    NRPC_METHOD_FLAG_GLOBAL = (1 << 1),
    NRPC_METHOD_FLAG_DYNCFG = (1 << 2),
    NRPC_METHOD_FLAG_RESTRICTED = (1 << 3), // this method is restricted (hidden from users)

    // this is 8-bit
} NRPC_METHOD_FLAGS;

// who is registering (or unregistering) a method - the registry enforces
// per-source rules (e.g. only the dyncfg subsystem and streaming children may
// register dyncfg-reserved names) and, on delete, who may remove what
typedef enum __attribute__((packed)) {
    NRPC_SOURCE_UNSET = 0,   // a forgotten desc field, never a valid source - the register and
                             // unregister paths assert against it in debug builds
    NRPC_SOURCE_PLUGIN,      // a local plugin (pluginsd FUNCTION)
    NRPC_SOURCE_STREAM,      // a streaming child's advertisement on this parent
    NRPC_SOURCE_DAEMON,      // daemon-internal registration (builtin methods, dyncfg)
} NRPC_SOURCE;

void nrpc_registry_init(RRDHOST *host);
void nrpc_registry_destroy(RRDHOST *host);

// release a handle acquired via nrpc_method_authorize()
void nrpc_method_acquired_release(RRDHOST *host, NRPC_METHOD_ACQUIRED *acquired);

// Registration descriptor: the method's attribute bundle plus its owner,
// scope and handler. Stack-filled by the caller; the registry copies what it
// keeps. Every field's zero keeps the meaning a zero positional argument had -
// the registry adds NO defaulting. Style rule: policy-valued fields (source,
// access, sync, timeout_s, priority, version) are written explicitly at every
// site even when zero; pointer fields may be omitted when NULL.
struct nrpc_method_desc {
    RRDHOST *host;                 // required
    RRDSET *st;                    // NULL = GLOBAL; non-NULL derives LOCAL + the chart functions_view
    const char *name;              // required
    const char *help;              // required
    const char *tags;              // NULL normalizes to "top"; NRPC_TAG_HIDDEN derives RESTRICTED
    int timeout_s;                 // stored raw - a zero is a 0s deadline, not "default"
    int priority;                  // stored raw; 0 is a real value (not NRPC_PRIORITY_DEFAULT)
    uint32_t version;              // 0 == NRPC_VERSION_DEFAULT (the production default)
    HTTP_ACCESS access;            // HTTP_ACCESS_NONE (0) is a real, permissive value
    bool sync;                     // false = async handler
    NRPC_SOURCE source;            // required: NRPC_SOURCE_UNSET (0) is debug-asserted against
    nrpc_handler_cb_t handler;     // required
    void *handler_data;            // legitimately NULL for daemon handlers
};

// register a method, called from its serving thread
void nrpc_method_register(const struct nrpc_method_desc *desc);

bool nrpc_method_unregister(RRDHOST *host, RRDSET *st, const char *name, NRPC_SOURCE source);

// true if name is a reserved dynamic-configuration method name ("config" or "config <id>")
bool nrpc_method_name_is_dyncfg(const char *name);

// Call specification: the caller's input to one method invocation. Stack-
// filled by the caller; the in-flight call record copies what it keeps, so
// pointers into dying scopes are safe. Every field's zero keeps the meaning a
// zero positional argument had - no defaulting is added. Style rule: policy-
// valued fields (user_access, timeout_s, wait, allow_restricted) are written
// explicitly at every site even when zero; pointer fields may be omitted when
// NULL. Naming: the spec (caller input) feeds struct nrpc_call (the in-flight
// record, private to nrpc-calls.c) which the handler sees as
// struct nrpc_request - three views of one invocation.
struct nrpc_call_spec {
    RRDHOST *host;                 // required by every current caller (a NULL host answers 500)
    BUFFER *result_wb;             // required; also the error sink
    const char *cmd;               // "name [args]"; NULL-tolerant: sanitizes to "" and fails the lookup (404)
    const char *source;            // provenance STRING (who is calling) - not NRPC_SOURCE
    HTTP_ACCESS user_access;       // HTTP_ACCESS_NONE (0) is a real, anonymous caller
    int timeout_s;                 // <= 0 = inherit the method's registered timeout
    bool wait;                     // async only: block until the result arrives
    bool allow_restricted;         // permit calling RESTRICTED (hidden) methods
    const char *call_id;           // NULL = generate one
    BUFFER *payload;               // NULL = none
    struct { nrpc_result_cb_t cb;       void *data; } result;
    struct { nrpc_progress_cb_t cb;     void *data; } progress;      // data may be NULL with cb set
    struct { nrpc_is_cancelled_cb_t cb; void *data; } is_cancelled;
};

// invoke a method - callable from anywhere
int nrpc_call(const struct nrpc_call_spec *spec);

// Verify the caller may invoke `cmd` on `host`, applying the same RESTRICTED and access-level
// checks nrpc_call() enforces, WITHOUT executing the method. This lets non-execution
// paths (e.g. MCP descriptor/help generation) authorize a caller before disclosing anything.
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
