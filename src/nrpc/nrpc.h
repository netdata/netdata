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
// START HERE: src/nrpc/README.md - the object model, the lifecycles, the
// locking rules and the file map. This header is the public API; the map
// below is its glossary.
//
// Concept map:
// - registry (struct nrpc_registry): per-host table of methods; the
//   registries live in a component-global index keyed by the owning host
//   OBJECT (NRPC_OWNER); the owner needs no component field of its own.
//   Teardown is disarm the owner vtable + unlink, then retire the entry's
//   operation GATE (no new readers; drain current ones), then destroy.
// - method (struct nrpc_method): a named callable endpoint. The dictionary
//   keeps a stable SLOT per sanitized name, pointing at an IMMUTABLE,
//   refcounted DESCRIPTOR (struct nrpc_method) that holds one registration's
//   whole state - attributes (name, help, tags, access, timeout, priority,
//   version; never "metadata", which RPC reserves for per-call headers) plus
//   handler, transport and serving-handle references. A re-registration
//   builds a new descriptor and swaps the slot's pointer as one unit.
//   Registration takes a caller-filled registration struct
//   (struct nrpc_method_desc, struct nrpc_builtin_desc); it is never stored.
// - epoch: the owner's liveness generation. Descriptors are stamped with the
//   epoch current at registration and are available only while it matches
//   the owner's - a host reconnect silently retires the previous
//   connection's registrations until they are re-registered.
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
// - catalog: publishes the registry to users, parents and Cloud (the
//   streaming re-list, the JSON views, the cloud manifest).
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

// Memory attribution owned by the component, aggregated by the daemon's
// pulse charts (daemon/pulse) - the daemon reads these, the component (and
// any daemon code allocating on the component's behalf) charges them. This
// is the allowed dependency direction: pulse depends on nrpc, never the
// reverse (precedent: dictionary_stats_category_other).
extern struct dictionary_stats dictionary_stats_category_functions;
extern size_t nrpc_buffers_functions;
extern size_t nrpc_methods_functions;   // bytes held by method descriptors (heap, outside the dictionary stats)

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
// struct nrpc_transport, and the in-flight calls table entry-pins it for the
// record's lifetime. ("entry-pin": holds an NRPC_LIFETIME entry ref, the
// counter that decides when the object is freed.)
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

// CONTRACT for handlers: req->result.cb MUST be invoked exactly once, on
// success and failure alike - for async methods possibly long after this
// returns. That invocation is what retires the in-flight call record; the
// return value only says whether the call was ACCEPTED, not answered.
typedef int (*nrpc_handler_cb_t)(struct nrpc_request *req, void *data);


// ----------------------------------------------------------------------------

struct nrpc_method;

// Opaque handle to an authorized method: an owned reference to the method's
// immutable descriptor - the registration cannot be freed while the handle
// is held, and release touches no registry state.
typedef struct nrpc_method_acquired NRPC_METHOD_ACQUIRED;

// The identity of a registry and of everyone who talks to it: the owning host
// OBJECT, carried as an opaque token. It is BOTH the key of the component-
// global registries index AND the argument handed back to the owner's
// callbacks. The component never dereferences .ptr - only the owner knows the
// pointee, and only the owner ever builds the token.
//
// Why the object and not the host's machine guid: two live hosts are two
// distinct objects, so two owners can never collide, whatever their guids say.
// A guid is text off the wire that several spellings can parse to, and an
// entry keyed on it can be claimed by a host that merely looks like its owner.
//
// CONTRACT the owner must honour, in exchange:
// - one token, one object, for as long as the entry exists;
// - destroy runs before the object's memory is released, so a later object
//   reusing the address finds no entry to inherit.
//
// init and destroy for one token MAY overlap; the component arbitrates them
// itself (the index serializes create against delete, and the entry's own lock
// serializes init's liveness check against destroy's disarm), so no
// interleaving corrupts anything. What the component cannot repair is the
// OUTCOME of losing that race: an init whose lookup landed just before a
// destroy deleted the entry ends up creating a fresh one, but an init that
// completed just before one leaves its owner with no registry until it
// initializes again. An owner that cannot tolerate that must serialize its own
// init and destroy - RRDHOST currently does not, and records why above
// rrdhost_nrpc_registry_owner().
typedef struct {
    void *ptr;
} NRPC_OWNER;

#define NRPC_OWNER_NONE ((NRPC_OWNER){ .ptr = NULL })

static inline bool nrpc_owner_is_set(NRPC_OWNER o) { return o.ptr != NULL; }
static inline bool nrpc_owner_eq(NRPC_OWNER a, NRPC_OWNER b) { return a.ptr == b.ptr; }

typedef enum __attribute__((packed)) {
    NRPC_METHOD_FLAG_NONE = 0,
    NRPC_METHOD_FLAG_DYNCFG = (1 << 0),
    NRPC_METHOD_FLAG_RESTRICTED = (1 << 1), // this method is restricted (hidden from users)

    // the enum is packed to one byte - keep the flags within 8 bits
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

// The owner vtable: everything the component may need from whoever owns a
// registry entry, supplied at entry creation. `id` is the identity - it keys
// the entry and comes back as the callbacks' argument. The component copies
// `name` (it keeps its own STRING, so an owner-side rename cannot free bytes
// under a concurrent log line) and stores the rest verbatim.
//
// `epoch` is the owner's liveness GENERATION: an OBJECT_STATE counter the
// owner bumps whenever the host goes unreachable and comes back - e.g. a
// streaming child reconnect. Every
// method descriptor is stamped with the epoch id current at its
// registration and is available only while the two match, so a reconnect
// silently retires everything the previous connection registered until it
// re-registers under the new id. The pointer is LIVE, valid exactly while
// the entry is armed - entry disarm (inside destroy) precedes owner
// teardown on every path.
//
// `changed` reports that the host-wide visible function set changed
// (`arm_manifest` is true when the change affects the cloud manifest - the
// flag-update and the manifest-arm fire under different per-entry
// conditions, so the component passes the verdict). Two unrelated "arm"
// verbs: the ENTRY is armed while its vtable is populated; arm_manifest
// means scheduling a cloud node-manifest refresh.
//
// `wants_del_journal` answers whether deleted names should be queued for a
// parent (a LIVE question - the owner's sender can appear and disappear).
struct nrpc_registry_owner {
    NRPC_OWNER id;                                    // required: the identity - index key and callback argument
    const char *name;                                 // copied at entry creation
    OBJECT_STATE *epoch;                              // required: live epoch pointer (see above). Its NULL is
                                                      // this component's DISARMED marker, so init refuses one
    void (*changed)(NRPC_OWNER id, bool arm_manifest);// the visible function set changed
    bool (*wants_del_journal)(NRPC_OWNER id);         // queue FUNCTION_DELs towards a parent?
};

// Registry entry lifecycle - called from the owner's own lifecycle with a live
// owner: init creates the entry in the component-global registries index,
// destroy disarms and unlinks it. Both are keyed on the owner itself,
// so neither can reach another owner's entry: init on a live entry is
// idempotent (it can only be a re-init of that same owner, e.g. un-archive),
// and destroy for an owner that never got an entry simply finds nothing.
void nrpc_registry_init(const struct nrpc_registry_owner *owner);
void nrpc_registry_destroy(NRPC_OWNER owner);

// destroy the (by then empty) component-global registries index at shutdown,
// after every owner has destroyed its entry, so leak checking sees a clean
// heap (the same contract as nrpc_inflight_calls_destroy)
void nrpc_registries_destroy(void);

// Release a handle acquired via nrpc_method_authorize(). The handle is an
// owned reference to the method's immutable descriptor - self-contained and
// touching no registry state - so release needs no owner and is safe even
// after the host's registry entry was destroyed by a concurrent teardown.
void nrpc_method_acquired_release(NRPC_METHOD_ACQUIRED *acquired);

// Registration descriptor: the method's attribute bundle plus its owner
// and handler. Stack-filled by the caller; the registry copies what it
// keeps. The registry applies exactly TWO normalizations - tags NULL ->
// "top" and priority 0 -> NRPC_PRIORITY_DEFAULT, both noted on their fields;
// every other field's zero keeps the meaning a zero positional argument had.
// Style rule: policy-valued fields (source, access, sync, timeout_s,
// priority, version) are written explicitly at every site even when zero;
// pointer fields may be omitted when NULL.
struct nrpc_method_desc {
    NRPC_OWNER owner;              // required: the owning host, as its owner token
    const char *name;              // required
    const char *help;              // required
    const char *tags;              // NULL normalizes to "top"; NRPC_TAG_HIDDEN derives RESTRICTED
    int timeout_s;                 // stored raw - a zero is a 0s deadline, not "default"
    int priority;                  // 0 normalizes to NRPC_PRIORITY_DEFAULT on every registration
    uint32_t version;              // 0 == NRPC_VERSION_DEFAULT (the production default)
    HTTP_ACCESS access;            // HTTP_ACCESS_NONE (0) is a real, permissive value
    bool sync;                     // false = async handler
    NRPC_SOURCE source;            // required: NRPC_SOURCE_UNSET (0) is debug-asserted against
    nrpc_handler_cb_t handler;     // required
    void *handler_data;            // legitimately NULL for daemon handlers
};

// register a method, called from its serving thread
void nrpc_method_register(const struct nrpc_method_desc *desc);

bool nrpc_method_unregister(NRPC_OWNER owner, const char *name, NRPC_SOURCE source);

// true if name is a reserved dynamic-configuration method name ("config" or "config <id>")
bool nrpc_method_name_is_dyncfg(const char *name);

// Call specification: the caller's input to one method invocation. Stack-
// filled by the caller; the in-flight call record copies what it keeps, so
// pointers into dying scopes are safe. Every field's zero keeps the meaning a
// zero positional argument had - no defaulting is added. Style rule: policy-
// valued fields (user_access, timeout_s, wait, allow_restricted) are written
// explicitly at every site even when zero; pointer fields may be omitted when
// NULL. Naming: the spec (caller input) feeds struct nrpc_call (the in-flight
// record, private to the component) which the handler sees as
// struct nrpc_request - three views of one invocation.
struct nrpc_call_spec {
    NRPC_OWNER owner;              // the target host; NRPC_OWNER_NONE = "no host given" (answers 500)
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

// Verify the caller may invoke `cmd` on the given host, applying the same
// RESTRICTED and access-level checks nrpc_call() enforces, WITHOUT executing the method.
// This lets non-execution paths (e.g. MCP descriptor/help generation) authorize a caller
// before disclosing anything. NRPC_OWNER_NONE is the "no host given" sentinel and answers 500.
// On success returns HTTP_RESP_OK; if out_acquired != NULL it receives an acquired registry
// handle the caller MUST release with nrpc_method_acquired_release(*out_acquired),
// otherwise the handle is released internally. On failure it writes the error into result_wb,
// releases any acquired handle, sets *out_acquired (if any) to NULL, and returns the HTTP code.
int nrpc_method_authorize(NRPC_OWNER owner, BUFFER *result_wb, const char *cmd,
                              HTTP_ACCESS user_access, bool allow_restricted,
                              NRPC_METHOD_ACQUIRED **out_acquired);

// The contract suites (each requires a prepared RRD / localhost; each
// returns its failure count, 0 = pass). Their suite-header comments double
// as prose documentation of what the component promises:
int nrpc_access_unittest(void);         // authorize() access gating (GHSA-6628-vxm3-4g8g regression)
int nrpc_manifest_unittest(void);       // dyncfg-namespace enforcement + cloud-manifest membership/hash
int nrpc_manifest_pacer_unittest(void); // manifest publish pacing
int nrpc_del_unittest(void);            // the pending FUNCTION_DEL queue protocol
int nrpc_registry_unittest(void);       // registration/deletion contracts + the lifecycle race suites
int nrpc_catalog_unittest(void);        // the renderers: streaming re-list, JSON, export, owner protocol

bool nrpc_method_available(NRPC_OWNER owner, const char *function);

bool nrpc_call_has_result_cb(nd_uuid_t *call_id, nrpc_result_cb_t cb);

// ----------------------------------------------------------------------------
// serving threads

// Every thread that registers methods brackets its registering life with
// these two calls. The handle they manage is part of every method's
// availability: once finished() runs, everything this thread registered
// becomes unavailable in every view at once (the entries stay in the
// registry until something unregisters or replaces them). started() is
// idempotent; finished() also drains in-flight cancel/progress dispatches
// aimed at this thread before it may exit.
void nrpc_serving_started(void);
void nrpc_serving_finished(void);

// ----------------------------------------------------------------------------
// the in-flight calls table

// the in-flight calls table: create at startup; destroy exists only for the
// ASAN-gated shutdown path (see nrpc_inflight_calls_destroy)
void nrpc_inflight_calls_create(void);
void nrpc_inflight_calls_destroy(void);

// Table-keyed deadline accessor: looks the call_id up in the in-flight calls table,
// atomically reads its deadline, releases. Returns false when the call_id
// is not (or no longer) in flight. This is how the transports read deadlines -
// they must NOT hold pointers into in-flight call records.
bool nrpc_call_deadline(const char *call_id, usec_t *out_stop_monotonic_ut);

// cancel a running function, to be run from anywhere
void nrpc_call_cancel(const char *call_id);

// deliver a progress ping to a running function's progress hook (gated on
// the serving thread of the call's own registration); from anywhere
void nrpc_call_progress(const char *call_id);
void nrpc_call_request_progress(nd_uuid_t *call_id);

// ----------------------------------------------------------------------------
// registering builtin (daemon-implemented, synchronous) methods

typedef int (*nrpc_builtin_handler_cb_t)(BUFFER *wb, const char *function, BUFFER *payload, const char *source);

// Registration struct for a builtin (daemon-implemented synchronous)
// method. Deliberately narrower than struct nrpc_method_desc: sync and source
// are forced internally (true, NRPC_SOURCE_DAEMON) where no caller can
// contradict them, and the handler is the typed builtin callback. Zero
// semantics match struct nrpc_method_desc - including the priority-0
// normalization; the same must-write style rule applies (timeout_s,
// priority, version, access).
struct nrpc_builtin_desc {
    NRPC_OWNER owner;              // required: the owning host, as its owner token
    const char *name;              // required
    const char *help;              // required
    const char *tags;              // NULL normalizes to "top"; NRPC_TAG_HIDDEN derives RESTRICTED
    int timeout_s;                 // stored raw - a zero is a 0s deadline, not "default"
    int priority;                  // 0 normalizes to NRPC_PRIORITY_DEFAULT on every registration
    uint32_t version;              // 0 == NRPC_VERSION_DEFAULT
    HTTP_ACCESS access;            // HTTP_ACCESS_NONE (0) is a real, permissive value
    nrpc_builtin_handler_cb_t handler; // required
};

void nrpc_method_register_builtin(const struct nrpc_builtin_desc *desc);

// ----------------------------------------------------------------------------
// reading the registry: the catalog

#define NRPC_VERSION_SEPARATOR "|"

// ----------------------------------------------------------------------------
// the iteration/visibility API: every consumer that renders or exports the
// function registry goes through the catalog's iteration core (the in-module
// renderers via the internal registry_foreach on an entry they already hold;
// external consumers and the unittests via nrpc_catalog_host_foreach()) -
// nobody outside the module touches the registry dictionary or its entries
// directly

// which functions a traversal visits; EVERY filter includes the availability
// check (serving thread running, host state current, not unregistered)
typedef enum {
    NRPC_CATALOG_FILTER_USER,        // + skip DYNCFG and RESTRICTED (user-facing lists, cloud)
    NRPC_CATALOG_FILTER_STREAM_GLOBAL, // + skip DYNCFG; DYNCFG entries are COUNTED
                                            //   (the return value) for dyncfg_add_streaming()
} NRPC_CATALOG_FILTER;

// The view handed to the callback. name, help and tags are ALL borrowed -
// help/tags from the visited entry's pinned immutable descriptor, name from
// the traversal - and are valid only for the duration of the callback, so a
// callback that keeps any of them must copy.
struct nrpc_method_view {
    const char *name;
    const char *help;
    const char *tags;
    int timeout;
    HTTP_ACCESS access;
    int priority;
    uint32_t version;
    NRPC_METHOD_FLAGS options;
};

typedef void (*nrpc_method_view_cb_t)(const struct nrpc_method_view *v, void *data);

// returns the number of DYNCFG entries encountered (meaningful for
// NRPC_CATALOG_FILTER_STREAM_GLOBAL, zero otherwise)
size_t nrpc_catalog_host_foreach(NRPC_OWNER owner, NRPC_CATALOG_FILTER filter, nrpc_method_view_cb_t cb, void *data);

// ----------------------------------------------------------------------------
// the consumers built on the iteration API

// The caller owns the changed-flag clear (the streaming side's half of the
// pending-dels ordering protocol) and must clear it BEFORE calling this.
// Returns the number of dyncfg-backed entries encountered so the caller can
// decide the synthetic "config" line (dyncfg is an application on top of
// this component - the renderer emits no dyncfg output itself).
size_t nrpc_catalog_render_global_functions(NRPC_OWNER owner, BUFFER *wb, bool can_function_del);

void nrpc_catalog_host2json(NRPC_OWNER owner, BUFFER *wb);

// help/tags receive OWNED byte copies (strdupz) - the destination dictionary's
// callbacks own freeing them (conflict losers and deleted entries)
void nrpc_catalog_host_to_dict(NRPC_OWNER owner, DICTIONARY *dst, void *value, size_t value_size,
                            const char **help, const char **tags, HTTP_ACCESS *access, int *priority, uint32_t *version);

// the ACLK node-instance manifest content
struct nrpc_manifest_entry {
    const char *help;   // owned copy
    const char *tags;   // owned copy
    HTTP_ACCESS access;
    int priority;
    uint32_t version;
};

// Returns a new dictionary, keyed by function name, holding one
// struct nrpc_manifest_entry per available, user-visible function.
// The caller owns it and must dictionary_destroy() it; the string copies are
// released by a delete callback registered on the dictionary.
DICTIONARY *nrpc_catalog_manifest_dict(NRPC_OWNER owner);

// Content hash of a manifest dictionary plus the node identity it will be published under.
// Covers exactly the fields generate_update_node_instance_manifest_message() transmits
// (name, help, tags, access, priority, version) plus node_id/claim_id - updated_at is
// excluded because it changes on every build. Used to suppress publishing a manifest
// identical to the one already sent. Change detection only, not security-sensitive.
uint64_t nrpc_catalog_manifest_hash(DICTIONARY *dict, const char *node_id, const char *claim_id);

// ----------------------------------------------------------------------------
// the transport (kept in its own header: it is legitimately included on its
// own, without the rest of this API)

#include "nrpc-transport.h"

#endif // NETDATA_NRPC_H
