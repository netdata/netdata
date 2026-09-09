// SPDX-License-Identifier: GPL-3.0-or-later

// Method invocations: the in-flight calls table, authorization, the three
// execution modes (sync, async-nowait, async-wait), and cancel/progress
// dispatch.

#include "nrpc-serving-internals.h"
#include "nrpc-internals.h"

// bytes currently held by call-related BUFFERs (charged atomically by the
// buffer code via the pointer handed to buffer_create); the daemon's pulse
// buffers chart reads it, and daemon code allocating buffers on the
// component's behalf (the streaming function executor) charges it too
size_t nrpc_buffers_functions = 0;

struct nrpc_call {
    nd_uuid_t call_id_uuid;
    const char *call_id;
    const char *sanitized_cmd;
    const char *source;
    bool cancelled;
    usec_t stop_monotonic_ut;

    HTTP_ACCESS user_access;

    BUFFER *payload;

    // The method being called: an OWNED reference to the immutable
    // descriptor of the registration this call was authorized against,
    // acquired at call start and released when the record leaves the table.
    // It is the ONLY view executors, cancel and progress ever have - a
    // concurrent re-registration swaps the registry's slot to a new
    // descriptor but can never change or free THIS one, so the handler
    // pair, the sync/async fork and the serving-handle dispatch gate are
    // all decided by the registration whose handler actually runs. The
    // descriptor entry-pins its transport; a call outliving the
    // registration degrades to a clean 503 via the transport's
    // acquire-or-fail, exactly as before.
    struct nrpc_method *method;

    // where the answer lands, and (async modes) who is told it arrived
    struct {
        BUFFER *wb;
        nrpc_result_cb_t cb;
        void *data;
    } result;

    struct {
        SPINLOCK spinlock;              // guards the two hook pairs below
    } callbacks;

    // The CALLER's cancellation predicate. A sync handler polls it
    // directly; in async modes the HANDLER instead sees the record's own
    // cancelled flag - but the wait-mode waiter still polls this one, and
    // it is what turns a caller hang-up into a cancel dispatch.
    struct {
        nrpc_is_cancelled_cb_t cb;
        void *data;
    } is_cancelled;

    // registered by the TRANSPORT while handling an async call: how to tell
    // the remote side to stop working on this call_id
    struct {
        nrpc_cancel_hook_cb_t cb;
        void *data;                     // a transport, entry-pinned for the record's lifetime
    } cancel_hook;

    // the CALLER's sink for progress reports coming back from the function
    struct {
        nrpc_progress_cb_t cb;
        void *data;
    } progress;

    // registered by the TRANSPORT while handling an async call: how to ask
    // the remote side for a progress update
    struct {
        nrpc_progress_hook_cb_t cb;
        void *data;                     // a transport, entry-pinned for the record's lifetime
    } progress_hook;
};

// The in-flight calls table: every in-flight call, keyed by its compact
// call_id. One global instance - in-flight calls are process-wide, not
// per-host (the record's descriptor ref carries everything a completion,
// cancel or progress ever needs; no host lookup is involved).
struct nrpc_inflight_calls {
    DICTIONARY *dict;
};

static struct nrpc_inflight_calls nrpc_inflight_calls = { .dict = NULL };

static void nrpc_call_cancel_internal(struct nrpc_call *call);

// ----------------------------------------------------------------------------
// the in-flight call record: cleanup, table callbacks, hook registration

static void nrpc_call_cleanup(struct nrpc_call *call) {
    buffer_free(call->payload);
    freez((void *)call->call_id);
    freez((void *)call->sanitized_cmd);
    freez((void *)call->source);

    // the cancel_hook/progress_hook registration pins: per the
    // nrpc_cancel_hook_cb_t contract their data is a transport, entry-
    // pinned at registration - BOTH released here, no dedup (an async record
    // can hold two). NULL-safe, so cleanup on a never-inserted record (the
    // early error paths) releases nothing.
    nrpc_transport_entry_release(call->cancel_hook.data);
    call->cancel_hook.data = NULL;
    nrpc_transport_entry_release(call->progress_hook.data);
    call->progress_hook.data = NULL;

    call->payload = NULL;
    call->call_id = NULL;
    call->sanitized_cmd = NULL;
}

static void nrpc_inflight_calls_delete_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    struct nrpc_call *call = value;

    nrpc_call_cleanup(call);
    nrpc_method_release(call->method);
    call->method = NULL;
}

static void nrpc_inflight_calls_insert_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    struct nrpc_call *call = value;
    spinlock_init(&call->callbacks.spinlock);
}

// Detects a call_id collision: on a second insert with the same call_id the
// dictionary calls this instead of storing, we flag the caller through the
// data out-param, and returning false leaves the existing record untouched.
static bool nrpc_inflight_calls_conflict_cb(const DICTIONARY_ITEM *item __maybe_unused,
                                               void *old_value __maybe_unused,
                                               void *new_value __maybe_unused,
                                               void *data) {
    bool *duplicate_call_id = data;
    if(duplicate_call_id)
        *duplicate_call_id = true;

    return false;
}

void nrpc_inflight_calls_create(void) {
    if(nrpc_inflight_calls.dict)
        return;

    nrpc_inflight_calls.dict = dictionary_create_advanced(DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct nrpc_call));

    dictionary_register_insert_callback(nrpc_inflight_calls.dict, nrpc_inflight_calls_insert_cb, NULL);
    dictionary_register_delete_callback(nrpc_inflight_calls.dict, nrpc_inflight_calls_delete_cb, NULL);
    dictionary_register_conflict_callback(nrpc_inflight_calls.dict, nrpc_inflight_calls_conflict_cb, NULL);
}

// Key-format invariant: the in-flight calls table keys call_ids with
// uuid_unparse_lower_compact() (see nrpc_call), and so does the
// pluginsd transport for its own inflight dictionary - a key-format change on
// either side would silently turn every lookup here into a miss.
bool nrpc_call_deadline(const char *call_id, usec_t *out_stop_monotonic_ut) {
    if(unlikely(!out_stop_monotonic_ut || !call_id || !*call_id))
        return false;

    const DICTIONARY_ITEM *item = dictionary_get_and_acquire_item(nrpc_inflight_calls.dict, call_id);
    if(!item)
        return false;

    struct nrpc_call *call = dictionary_acquired_item_value(item);
    *out_stop_monotonic_ut = __atomic_load_n(&call->stop_monotonic_ut, __ATOMIC_RELAXED);
    dictionary_acquired_item_release(nrpc_inflight_calls.dict, item);

    return true;
}

// This has exactly one call site: the leak-checking cleanup at the end of the
// daemon shutdown, which sits behind FSANITIZE_ADDRESS. Sanitizer builds define
// that macro, so there the table is torn down at exit; every other build
// compiles the block out and the table lives for the whole process lifetime.
//
// Either way, no accessor tests the dictionary for NULL, and that is deliberate.
// The teardown is ordered after every thread that could reach the table has been
// joined, so a thread that outlived that join to reach one accessor reaches the
// other five just as easily. Ordering is the guarantee; a NULL test in one of
// six places could only disguise which contract is meant to hold.
void nrpc_inflight_calls_destroy(void) {
    if(!nrpc_inflight_calls.dict)
        return;

    dictionary_destroy(nrpc_inflight_calls.dict);
    nrpc_inflight_calls.dict = NULL;
}

static void nrpc_call_register_cancel_hook_cb(void *register_cancel_hook_cb_data, nrpc_cancel_hook_cb_t cancel_hook_cb, void *cancel_hook_cb_data) {
    struct nrpc_call *call = register_cancel_hook_cb_data;

    // per the nrpc_cancel_hook_cb_t contract, cancel_hook_cb_data is the
    // registrar's transport: entry-pin it for the record's lifetime, so a late
    // cancel after the transport's owner died acquire-fails on a valid (dead)
    // transport instead of chasing a freed pointer. Released in
    // nrpc_call_cleanup(). A re-registration (none exists today)
    // releases the previous pin so it cannot leak. entry-acquire cannot fail
    // on a legally held pointer (it fatals on use-after-release); storing its
    // returned pointer keeps the sites that fill a dedicated transport field
    // shaped the same way (dyncfg_insert_cb, dyncfg_conflict_cb). The registry
    // entry sites store the transport as handler_data itself, so they have
    // no separate field to fill.
    struct nrpc_transport *pinned = nrpc_transport_entry_acquire(cancel_hook_cb_data);

    spinlock_lock(&call->callbacks.spinlock);
    struct nrpc_transport *previous = call->cancel_hook.data;
    call->cancel_hook.cb = cancel_hook_cb;
    call->cancel_hook.data = pinned;
    spinlock_unlock(&call->callbacks.spinlock);

    nrpc_transport_entry_release(previous);
}

static void nrpc_call_register_progress_hook_cb(void *register_progress_hook_cb_data, nrpc_progress_hook_cb_t progress_hook_cb, void *progress_hook_cb_data) {
    struct nrpc_call *call = register_progress_hook_cb_data;

    // same contract, pin and re-registration handling as the cancel_hook above
    struct nrpc_transport *pinned = nrpc_transport_entry_acquire(progress_hook_cb_data);

    spinlock_lock(&call->callbacks.spinlock);
    struct nrpc_transport *previous = call->progress_hook.data;
    call->progress_hook.cb = progress_hook_cb;
    call->progress_hook.data = pinned;
    spinlock_unlock(&call->callbacks.spinlock);

    nrpc_transport_entry_release(previous);
}

// ----------------------------------------------------------------------------
// waiting for async function completion

struct nrpc_call_wait {
    char *call_id;

    bool free_with_signal;
    bool data_are_ready;
    netdata_mutex_t mutex;
    netdata_cond_t cond;
    int code;
};

static void nrpc_inflight_calls_del(const char *call_id) {
    dictionary_del(nrpc_inflight_calls.dict, call_id);
    dictionary_garbage_collect(nrpc_inflight_calls.dict);
}

static void nrpc_call_wait_free(struct nrpc_call_wait *tmp) {
    nrpc_inflight_calls_del(tmp->call_id);
    freez(tmp->call_id);

    netdata_cond_destroy(&tmp->cond);
    netdata_mutex_destroy(&tmp->mutex);
    freez(tmp);
}

// The completion side of the wait-mode ownership handoff (the full protocol
// is on nrpc_call_async_wait): `tmp` and `temp_wb` are freed exactly once -
// by US when the waiter already gave up (free_with_signal), by the WAITER
// otherwise. The flag is written and read only under tmp->mutex; that is the
// handoff.
static void nrpc_call_signal_ready(BUFFER *temp_wb __maybe_unused, int code, void *callback_data) {
    struct nrpc_call_wait *tmp = callback_data;
    bool we_should_free = false;

    netdata_mutex_lock(&tmp->mutex);

    // since we got the mutex,
    // the waiting thread is either in cond_timedwait()
    // or gave up and left.

    tmp->code = code;
    tmp->data_are_ready = true;

    if(tmp->free_with_signal)
        we_should_free = true;

    netdata_cond_signal(&tmp->cond);

    netdata_mutex_unlock(&tmp->mutex);

    if(we_should_free) {
        buffer_free(temp_wb);
        nrpc_call_wait_free(tmp);
    }
}

static void nrpc_call_nowait_finished(BUFFER *wb, int code, void *data) {
    struct nrpc_call *call = data;

    if(call->result.cb)
        call->result.cb(wb, code, call->result.data);

    nrpc_inflight_calls_del(call->call_id);
}

static bool nrpc_call_is_cancelled(void *data) {
    struct nrpc_call *call = data;
    return __atomic_load_n(&call->cancelled, __ATOMIC_RELAXED);
}

// The handler's view of a call. All three execution modes share every field
// but two, so those two are the only parameters:
//
// - where the answer goes and who is told about it: sync passes the caller's
//   own buffer and result callback straight through; nowait substitutes its
//   completion hook (which forwards to the caller and then retires the
//   record); wait substitutes a temporary buffer it can abandon on timeout.
//
// - `async`: whether the record outlives the handler's return. Only then may
//   the handler register cancel/progress hooks, and only then does the
//   handler learn about cancellation through the record's own flag (which
//   nrpc_call_cancel() sets from another thread). A sync handler runs inside
//   the caller's thread, so the caller's own is_cancelled predicate is the
//   live answer and there is nothing to hook into.
static struct nrpc_request nrpc_request_for(struct nrpc_call *call, BUFFER *wb,
                                            nrpc_result_cb_t result_cb, void *result_data,
                                            bool async) {
    struct nrpc_request req = {
        .call_id = &call->call_id_uuid,
        .function = call->sanitized_cmd,
        .payload = call->payload,
        .user_access = call->user_access,
        .source = call->source,
        .stop_monotonic_ut = &call->stop_monotonic_ut,
        .result = { .wb = wb, .cb = result_cb, .data = result_data },
        .progress = { .cb = call->progress.cb, .data = call->progress.data },
        .is_cancelled = { .cb = call->is_cancelled.cb, .data = call->is_cancelled.data },
    };

    if(async) {
        req.is_cancelled.cb = nrpc_call_is_cancelled;
        req.is_cancelled.data = call;
        req.register_cancel_hook.cb = nrpc_call_register_cancel_hook_cb;
        req.register_cancel_hook.data = call;
        req.register_progress_hook.cb = nrpc_call_register_progress_hook_cb;
        req.register_progress_hook.data = call;
    }

    return req;
}

static inline int nrpc_call_async_nowait(struct nrpc_call *call) {
    struct nrpc_request req = nrpc_request_for(call, call->result.wb,
                                               nrpc_call_nowait_finished, call, true);

    return call->method->handler(&req, call->method->handler_data);
}

// The "wait" mode: run an async handler, then block this thread until it
// answers, the deadline passes, or the caller cancels.
//
// The hard part is ownership of `tmp` (the wait record) and `temp_wb` (the
// temporary result buffer). Both are shared with nrpc_call_signal_ready(),
// which runs on the HANDLER's thread, whenever that turns out to be.
// Exactly one side frees them, decided under tmp->mutex before we let go
// of it:
//
//   answered in time -> WE own the cleanup (free_with_signal stays false).
//                       We copy temp_wb into the caller's buffer first,
//                       because the caller's buffer is the one that
//                       outlives us.
//   timeout/cancel/  -> the COMPLETION CALLBACK owns the cleanup
//   handler failure     (free_with_signal = true). We walk away - and the
//                       callback always comes, because a handler MUST
//                       invoke result.cb even when it fails. This is
//                       why temp_wb exists at all: the caller's buffer may
//                       be gone by the time a late handler answers - the
//                       temporary buffer is required, not an optimization.
//
// The mutex is taken only AFTER the handler returns: a handler is allowed
// to answer synchronously from inside its own invocation, and it would then
// deadlock against a mutex we already held.
static int nrpc_call_async_wait(struct nrpc_call *call) {
    struct nrpc_call_wait *tmp = mallocz(sizeof(struct nrpc_call_wait));
    tmp->free_with_signal = false;
    tmp->data_are_ready = false;
    tmp->call_id = strdupz(call->call_id);
    netdata_mutex_init(&tmp->mutex);
    netdata_cond_init(&tmp->cond);

    bool we_should_free = false;
    BUFFER *temp_wb  = buffer_create(1024, &nrpc_buffers_functions); // we need it because we may give up on it
    temp_wb->content_type = call->result.wb->content_type;

    // the result callbacks are ours, not the caller's: they signal this thread
    // and own the cleanup of the temporary buffer and the wait record
    struct nrpc_request req = nrpc_request_for(call, temp_wb, nrpc_call_signal_ready, tmp, true);
    int code = call->method->handler(&req, call->method->handler_data);

    netdata_mutex_lock(&tmp->mutex); // only after the handler returned - see above

    if (code == HTTP_RESP_OK || tmp->data_are_ready) {
        bool cancelled = false;
        int rc = 0;
        while (rc == 0 && !cancelled && !tmp->data_are_ready) {
            usec_t now_mono_ut = now_monotonic_usec();
            usec_t stop_mono_ut = nrpc_effective_deadline_ut(__atomic_load_n(&call->stop_monotonic_ut, __ATOMIC_RELAXED));
            if(now_mono_ut > stop_mono_ut) {
                rc = UV_ETIMEDOUT;
                break;
            }

            // wait for 10ms, and loop again...
            // the mutex is unlocked within cond_timedwait()
            rc = netdata_cond_timedwait(&tmp->cond, &tmp->mutex, 10 * NSEC_PER_MSEC);
            // the mutex is again ours

            if(rc == UV_ETIMEDOUT) {
                // 10ms have passed

                rc = 0;
                if (!tmp->data_are_ready && call->is_cancelled.cb &&
                    call->is_cancelled.cb(call->is_cancelled.data)) {
                    cancelled = true;
                    nrpc_call_cancel_internal(call);
                    break;
                }
            }
        }

        if (tmp->data_are_ready) {
            // we have a response

            buffer_contents_replace(call->result.wb, buffer_tostring(temp_wb), buffer_strlen(temp_wb));
            call->result.wb->content_type = temp_wb->content_type;
            call->result.wb->expires = temp_wb->expires;

            if(call->result.wb->expires)
                buffer_cacheable(call->result.wb);
            else
                buffer_no_cacheable(call->result.wb);

            code = tmp->code;

            tmp->free_with_signal = false;
            we_should_free = true;
        }
        else if (rc == UV_ETIMEDOUT || cancelled) {
            // timeout
            // we will go away and let the callback free the structure

            if(cancelled)
                code = nrpc_call_error(call->result.wb,
                                               "Request cancelled",
                                               HTTP_RESP_CLIENT_CLOSED_REQUEST);
            else
                code = nrpc_call_error(call->result.wb,
                                               "Timeout while waiting for a response from the plugin that serves this features",
                                               HTTP_RESP_GATEWAY_TIMEOUT);

            tmp->free_with_signal = true;
            we_should_free = false;
        }
        else {
            code = nrpc_call_error(
                call->result.wb, "Internal error while communicating with the plugin that serves this feature.",
                HTTP_RESP_INTERNAL_SERVER_ERROR);

            tmp->free_with_signal = true;
            we_should_free = false;
        }
    }
    else {
        // the response is not ok, and we don't have the data
        tmp->free_with_signal = true;
        we_should_free = false;
    }

    netdata_mutex_unlock(&tmp->mutex);

    if (we_should_free) {
        nrpc_call_wait_free(tmp);
        buffer_free(temp_wb);
    }

    return code;
}

static inline int nrpc_call_async(struct nrpc_call *call, bool wait) {
    if(wait)
        return nrpc_call_async_wait(call);
    else
        return nrpc_call_async_nowait(call);
}

// ----------------------------------------------------------------------------
// authorization: resolve a command to a method the caller may invoke

// Internal: authorize and hand out an OWNED descriptor reference for the
// method. On failure everything acquired is released and *out_method is
// NULL. An unset owner answers 500 ("no host given"); an owner with no live
// registry entry answers 404.
//
// `sanitized_cmd` MUST already be sanitized - each entry point sanitizes once,
// on its own, and hands the result down. That is why this takes the sanitized
// form rather than re-deriving it: nrpc_call() needs the sanitized command for
// the in-flight record anyway, so sanitizing here too would do the same work
// twice on every call.
static int nrpc_method_authorize_acquire(NRPC_OWNER owner, BUFFER *result_wb, const char *sanitized_cmd,
                                         HTTP_ACCESS user_access, bool allow_restricted,
                                         struct nrpc_method **out_method) {
    *out_method = NULL;

    if(!nrpc_owner_is_set(owner))
        return nrpc_call_error(result_wb, "No host given for routing this request to.",
                                       HTTP_RESP_INTERNAL_SERVER_ERROR);

    const DICTIONARY_ITEM *registry_item;
    struct nrpc_registry *registry = nrpc_registry_acquire(owner, &registry_item);
    if(!registry)
        return nrpc_call_error(result_wb,
                                       "This feature is not available on this host at this time.",
                                       HTTP_RESP_NOT_FOUND);

    // the registry's operation gate around the find: a registry whose
    // destroy started answers exactly like an absent one
    if(!nrpc_registry_dispatcher_acquire(registry)) {
        nrpc_registry_release(registry_item);
        return nrpc_call_error(result_wb,
                                       "This feature is not available on this host at this time.",
                                       HTTP_RESP_NOT_FOUND);
    }

    struct nrpc_method *method = NULL;
    int code = nrpc_registry_find(registry, result_wb, sanitized_cmd, &method);

    // past the find, neither the gate nor the registry handle is needed: the
    // find returned an owned descriptor reference, independent of the
    // registry and its dictionaries
    nrpc_registry_dispatcher_release(registry);
    nrpc_registry_release(registry_item);

    if(code != HTTP_RESP_OK)
        return code;

    if((method->options & NRPC_METHOD_FLAG_RESTRICTED) && !allow_restricted) {
        code = nrpc_call_error(result_wb,
                                       "This feature is not available via this API.",
                                       HTTP_ACCESS_PERMISSION_DENIED_HTTP_CODE(user_access));
        nrpc_method_release(method);
        return code;
    }

    if(!http_access_user_has_enough_access_level_for_endpoint(user_access, method->access)) {

        if((method->access & HTTP_ACCESS_SIGNED_ID) && !(user_access & HTTP_ACCESS_SIGNED_ID))
            code = nrpc_call_error(result_wb,
                                           "You need to be authenticated via Netdata Cloud Single-Sign-On (SSO) "
                                           "to access this feature. Sign-in on this dashboard, "
                                           "or access your Netdata via https://app.netdata.cloud.",
                                           HTTP_ACCESS_PERMISSION_DENIED_HTTP_CODE(user_access));

        else if((method->access & HTTP_ACCESS_SAME_SPACE) && !(user_access & HTTP_ACCESS_SAME_SPACE))
            code = nrpc_call_error(result_wb,
                                           "You need to login to the Netdata Cloud space this agent is claimed to, "
                                           "to access this feature.",
                                           HTTP_ACCESS_PERMISSION_DENIED_HTTP_CODE(user_access));

        else if((method->access & HTTP_ACCESS_COMMERCIAL_SPACE) && !(user_access & HTTP_ACCESS_COMMERCIAL_SPACE))
            code = nrpc_call_error(result_wb,
                                           "This feature is only available for commercial users and supporters "
                                           "of Netdata. To use it, please upgrade your space. "
                                           "Thank you for supporting Netdata.",
                                           HTTP_ACCESS_PERMISSION_DENIED_HTTP_CODE(user_access));

        else {
            HTTP_ACCESS missing_access = (~user_access) & method->access;
            char perms_str[1024];
            http_access2txt(perms_str, sizeof(perms_str), ", ", missing_access);

            char msg[2048];
            snprintfz(msg, sizeof(msg), "This feature requires additional permissions: %s.", perms_str);

            code = nrpc_call_error(result_wb, msg,
                                           HTTP_ACCESS_PERMISSION_DENIED_HTTP_CODE(user_access));
        }

        nrpc_method_release(method);
        return code;
    }

    *out_method = method;
    return HTTP_RESP_OK;
}

int nrpc_method_authorize(NRPC_OWNER owner, BUFFER *result_wb, const char *cmd,
                              HTTP_ACCESS user_access, bool allow_restricted,
                              NRPC_METHOD_ACQUIRED **out_acquired) {
    if(out_acquired)
        *out_acquired = NULL;

    // this entry point owns the sanitize; _acquire() takes the sanitized form
    CLEAN_CHAR_P *sanitized_cmd = nrpc_sanitize_name_dupz(cmd);

    struct nrpc_method *method = NULL;
    int code = nrpc_method_authorize_acquire(owner, result_wb, sanitized_cmd, user_access, allow_restricted, &method);
    if(code != HTTP_RESP_OK)
        return code;

    if(out_acquired) {
        *out_acquired = mallocz(sizeof(**out_acquired));
        (*out_acquired)->method = method;
    }
    else
        nrpc_method_release(method);

    return HTTP_RESP_OK;
}

int nrpc_call(const struct nrpc_call_spec *spec) {
    internal_fatal(!spec->result_wb, "NRPC: call without a result buffer");

    BUFFER *result_wb = spec->result_wb;

    int code;

    const char *source_to_sanitize = spec->source ? spec->source : "";
    size_t sanitized_source_size = nrpc_strlen_bounded(source_to_sanitize, PLUGINSD_LINE_MAX) + 1;
    CLEAN_CHAR_P *sanitized_source = mallocz(sanitized_source_size);
    nrpc_sanitize_name(sanitized_source, source_to_sanitize, sanitized_source_size);

    // Sanitized ONCE, here, and moved into the in-flight record below; the
    // authorize path is handed the result instead of re-deriving it. The local
    // is disarmed after the move, so the early returns in between free it and
    // nothing double-frees.
    CLEAN_CHAR_P *sanitized_cmd = nrpc_sanitize_name_dupz(spec->cmd);

    // ------------------------------------------------------------------------
    // find the function and verify the caller's access
    // (an unset owner answers 500 - the "no host given" sentinel)

    struct nrpc_method *method = NULL;
    code = nrpc_method_authorize_acquire(spec->owner, result_wb, sanitized_cmd, spec->user_access,
                                         spec->allow_restricted, &method);
    if(code != HTTP_RESP_OK) {

        if(spec->result.cb)
            spec->result.cb(result_wb, code, spec->result.data);

        return code;
    }

    int timeout_s = spec->timeout_s;
    if(timeout_s <= 0)
        timeout_s = method->timeout;

    // ------------------------------------------------------------------------
    // validate and parse the call_id, or generate a new call_id id

    char uuid_str[UUID_COMPACT_STR_LEN];
    nd_uuid_t uuid;

    if(!spec->call_id || !*spec->call_id || uuid_parse_flexi(spec->call_id, uuid) != 0)
        uuid_generate_random(uuid);

    uuid_unparse_lower_compact(uuid, uuid_str);
    const char *call_id = uuid_str;

    // ------------------------------------------------------------------------
    // put the call into the in-flight table (every mode - sync included -
    // runs through a record; the mode decides who retires it, see dispatch
    // below)

    struct nrpc_call t = {
        .sanitized_cmd = sanitized_cmd,
        .call_id = strdupz(call_id),
        .user_access = spec->user_access,
        .source = sanitized_source,
        .payload = buffer_dup(spec->payload),
        .cancelled = false,
        .stop_monotonic_ut = now_monotonic_usec() + timeout_s * USEC_PER_SEC,
        .method = method,
        .result = {
            .wb = result_wb,
            .cb = spec->result.cb,
            .data = spec->result.data,
        },
        .is_cancelled = {
            .cb = spec->is_cancelled.cb,
            .data = spec->is_cancelled.data,
        },
        .progress = {
            .cb = spec->progress.cb,
            .data = spec->progress.data,
        },
    };
    sanitized_source = NULL;
    sanitized_cmd = NULL;
    uuid_copy(t.call_id_uuid, uuid);

    // `call` is the dictionary's STORED copy (or, on a collision, the
    // pre-existing record); `t` is our stack buffer. That is why the error
    // paths below clean up `t` - including releasing t.method - and never
    // touch `call`.
    bool duplicate_call_id = false;
    struct nrpc_call *call = dictionary_set_advanced(
        nrpc_inflight_calls.dict, call_id, -1, &t, sizeof(t), &duplicate_call_id);
    if(!call) {
        // dictionary_set() returns NULL when the dictionary is destroyed (shutdown in progress)
        code = nrpc_call_error(result_wb, "Service is shutting down.", HTTP_RESP_SERVICE_UNAVAILABLE);

        nrpc_call_cleanup(&t);
        nrpc_method_release(t.method);

        if(spec->result.cb)
            spec->result.cb(result_wb, code, spec->result.data);

        return code;
    }

    if(duplicate_call_id) {
        nd_log(NDLS_DAEMON, NDLP_NOTICE,
               // the SANITIZED command: a raw one can carry control characters
               // or newlines and inject lines into the journal
               "NRPC: duplicate call_id '%s', method: '%s'",
               t.call_id, t.sanitized_cmd);

        code = nrpc_call_error(result_wb, "Duplicate transaction.", HTTP_RESP_BAD_REQUEST);

        nrpc_call_cleanup(&t);
        nrpc_method_release(t.method);

        if(spec->result.cb)
            spec->result.cb(result_wb, code, spec->result.data);

        return code;
    }
    // Dispatch. Who deletes the in-flight record differs per mode:
    // sync deletes it right here after the handler returns; async-nowait
    // deletes it when the result arrives (nrpc_call_nowait_finished);
    // async-wait deletes it through nrpc_call_wait_free(), on whichever
    // side won the ownership handoff.
    if(call->method->sync) {
        // the handler answers inline, on this thread

        struct nrpc_request req = nrpc_request_for(call, call->result.wb,
                                                   call->result.cb, call->result.data, false);
        code = call->method->handler(&req, call->method->handler_data);

        nrpc_inflight_calls_del(call->call_id);
        return code;
    }

    return nrpc_call_async(call, spec->wait);
}

bool nrpc_call_has_result_cb(nd_uuid_t *call_id, nrpc_result_cb_t cb) {
    bool ret = false;
    char str[UUID_COMPACT_STR_LEN];
    uuid_unparse_lower_compact(*call_id, str);
    const DICTIONARY_ITEM *item = dictionary_get_and_acquire_item(nrpc_inflight_calls.dict, str);
    if(item) {
        struct nrpc_call *call = dictionary_acquired_item_value(item);
        if(call->result.cb == cb)
            ret = true;

        dictionary_acquired_item_release(nrpc_inflight_calls.dict, item);
    }
    return ret;
}

static void nrpc_call_cancel_internal(struct nrpc_call *call) {
    if(!call)
        return;

    // One read-modify-write, so exactly one caller wins the transition: two
    // threads cancelling the same call_id would otherwise both read false and
    // both dispatch the hook below, sending a duplicate cancel downstream.
    if(__atomic_exchange_n(&call->cancelled, true, __ATOMIC_RELAXED)) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "NRPC: received a CANCEL request for call_id '%s', but it is already cancelled.",
               call->call_id);
        return;
    }

    // Dispatch gates on the serving handle of the CALL's descriptor - the
    // registration whose handler actually ran - not on whatever the registry
    // currently holds for this name. The descriptor is immutable and the
    // call owns a ref, so this acquire and the release below always address
    // the same handle, and the handle cannot be freed in between.
    if(!nrpc_serving_dispatcher_acquire(call->method->serving)) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "NRPC: received a CANCEL request for call_id '%s', but the serving thread is not running.",
               call->call_id);
        return;
    }

    nrpc_cancel_hook_cb_t cancel_hook_cb;
    void *cancel_hook_cb_data;

    spinlock_lock(&call->callbacks.spinlock);
    cancel_hook_cb = call->cancel_hook.cb;
    cancel_hook_cb_data = call->cancel_hook.data;
    spinlock_unlock(&call->callbacks.spinlock);

    if(cancel_hook_cb)
        cancel_hook_cb(call->call_id, cancel_hook_cb_data);

    nrpc_serving_dispatcher_release(call->method->serving);
}

void nrpc_call_cancel(const char *call_id) {
    const DICTIONARY_ITEM *item = dictionary_get_and_acquire_item(nrpc_inflight_calls.dict, call_id);
    if(!item) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "NRPC: received a CANCEL request for call_id '%s', but the call_id is not running.",
               call_id);
        return;
    }

    struct nrpc_call *call = dictionary_acquired_item_value(item);
    nrpc_call_cancel_internal(call);
    dictionary_acquired_item_release(nrpc_inflight_calls.dict, item);
}

void nrpc_call_progress(const char *call_id) {
    const DICTIONARY_ITEM *item = dictionary_get_and_acquire_item(nrpc_inflight_calls.dict, call_id);
    if(!item) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "NRPC: received a PROGRESS request for call_id '%s', but the call_id is not running.",
               call_id);
        return;
    }

    struct nrpc_call *call = dictionary_acquired_item_value(item);

    // same gate as cancel: the CALL's descriptor decides, and acquire and
    // release always address the same, pinned serving handle
    if(!nrpc_serving_dispatcher_acquire(call->method->serving)) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "NRPC: received a PROGRESS request for call_id '%s', but the serving thread is not running.",
               call_id);
        goto cleanup;
    }

    functions_stop_monotonic_update_on_progress(&call->stop_monotonic_ut);

    nrpc_progress_hook_cb_t progress_hook_cb;
    void *progress_hook_cb_data;

    spinlock_lock(&call->callbacks.spinlock);
    progress_hook_cb = call->progress_hook.cb;
    progress_hook_cb_data = call->progress_hook.data;
    spinlock_unlock(&call->callbacks.spinlock);

    if(progress_hook_cb)
        progress_hook_cb(call_id, progress_hook_cb_data);

    nrpc_serving_dispatcher_release(call->method->serving);

cleanup:
    dictionary_acquired_item_release(nrpc_inflight_calls.dict, item);
}

void nrpc_call_request_progress(nd_uuid_t *call_id) {
    if(uuid_is_null(*call_id))
        return;

    char str[UUID_COMPACT_STR_LEN];
    uuid_unparse_lower_compact(*call_id, str);
    nrpc_call_progress(str);
}
