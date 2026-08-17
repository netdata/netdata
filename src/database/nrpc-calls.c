// SPDX-License-Identifier: GPL-3.0-or-later

#include "nrpc-serving-internals.h"
#include "nrpc-internals.h"
#include "nrpc-calls.h"

struct nrpc_call {
    RRDHOST *host;
    nd_uuid_t call_id_uuid;
    const char *call_id;
    const char *cmd;
    const char *sanitized_cmd;
    const char *source;
    size_t sanitized_cmd_length;
    int timeout;
    bool cancelled;
    usec_t stop_monotonic_ut;

    HTTP_ACCESS user_access;

    BUFFER *payload;

    NRPC_METHOD_ACQUIRED *method_acquired;

    // the method being called
    // we acquire this structure at the beginning,
    // and we release it at the end
    struct nrpc_method *method;

    // capture-at-find (see nrpc_method_capture): executors use THIS
    // pair, never method->handler / handler_data re-read at call time - a
    // concurrent re-registration swaps those and frees the displaced
    // transport; the captured pin keeps ours alive and a stale capture
    // degrades to a clean 503 via the transport's acquire-or-fail
    struct nrpc_capture captured;

    struct {
        BUFFER *wb;

        // in async mode,
        // the function to call to send the result back
        nrpc_result_cb_t cb;
        void *data;
    } result;

    struct {
        SPINLOCK spinlock;
    } callbacks;

    struct {
        // to be called in sync mode
        // while the function is running
        // to check if the function has been canceled
        nrpc_is_cancelled_cb_t cb;
        void *data;
    } is_cancelled;

    struct {
        // to be registered by the function itself
        // used to signal the function to cancel
        nrpc_cancel_hook_cb_t cb;
        void *data;
    } cancel_hook;

    struct {
        // callback to receive progress reports from function
        nrpc_progress_cb_t cb;
        void *data;
    } progress;

    struct {
        // to be registered by the function itself
        // used to send progress requests to function
        nrpc_progress_hook_cb_t cb;
        void *data;
    } progress_hook;
};

// The in-flight calls table: every in-flight call, keyed by its compact
// call_id. One global instance - in-flight calls are process-wide, not
// per-host (the record carries its host).
struct nrpc_inflight_calls {
    DICTIONARY *dict;
};

static struct nrpc_inflight_calls nrpc_inflight_calls = { .dict = NULL };

static void nrpc_call_cancel_internal(struct nrpc_call *call);

// ----------------------------------------------------------------------------

static void nrpc_call_cleanup(struct nrpc_call *call) {
    buffer_free(call->payload);
    freez((void *)call->call_id);
    freez((void *)call->cmd);
    freez((void *)call->sanitized_cmd);
    freez((void *)call->source);

    // the execute capture's transport pin (NULL for INTERNAL sources)
    nrpc_capture_release(&call->captured);

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
    call->cmd = NULL;
    call->sanitized_cmd = NULL;
}

static void nrpc_inflight_calls_delete_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    struct nrpc_call *call = value;

    // internal_error(true, "NRPC: call_id '%s' finished", call->call_id);

    nrpc_call_cleanup(call);
    nrpc_method_acquired_release(call->host, call->method_acquired);
}

static void nrpc_inflight_calls_insert_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    struct nrpc_call *call = value;
    spinlock_init(&call->callbacks.spinlock);
}

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
    if(unlikely(!out_stop_monotonic_ut || !call_id || !*call_id || !nrpc_inflight_calls.dict))
        return false;

    const DICTIONARY_ITEM *item = dictionary_get_and_acquire_item(nrpc_inflight_calls.dict, call_id);
    if(!item)
        return false;

    struct nrpc_call *call = dictionary_acquired_item_value(item);
    *out_stop_monotonic_ut = __atomic_load_n(&call->stop_monotonic_ut, __ATOMIC_RELAXED);
    dictionary_acquired_item_release(nrpc_inflight_calls.dict, item);

    return true;
}

// called ONLY from the ASAN-gated shutdown path (daemon-shutdown.c): in normal
// operation the table lives for the process lifetime and is never torn down -
// the destroy exists so leak checking sees a clean heap
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
    RRDHOST *host;
    char *call_id;

    bool free_with_signal;
    bool data_are_ready;
    netdata_mutex_t mutex;
    netdata_cond_t cond;
    int code;
};

static void nrpc_inflight_calls_del(RRDHOST *host __maybe_unused, const char *call_id) {
    dictionary_del(nrpc_inflight_calls.dict, call_id);
    dictionary_garbage_collect(nrpc_inflight_calls.dict);
}

static void nrpc_call_wait_free(struct nrpc_call_wait *tmp) {
    nrpc_inflight_calls_del(tmp->host, tmp->call_id);
    freez(tmp->call_id);

    netdata_cond_destroy(&tmp->cond);
    netdata_mutex_destroy(&tmp->mutex);
    freez(tmp);
}

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

    nrpc_inflight_calls_del(call->host, call->call_id);
}

static bool nrpc_call_is_cancelled(void *data) {
    struct nrpc_call *call = data;
    return __atomic_load_n(&call->cancelled, __ATOMIC_RELAXED);
}

static inline int nrpc_call_async_nowait(struct nrpc_call *call) {
    struct nrpc_request req = {
        .call_id = &call->call_id_uuid,
        .function = call->sanitized_cmd,
        .payload = call->payload,
        .user_access = call->user_access,
        .source = call->source,
        .stop_monotonic_ut = &call->stop_monotonic_ut,
        .result = {
            .wb = call->result.wb,
            .cb = nrpc_call_nowait_finished,
            .data = call,
        },
        .progress = {
            .cb = call->progress.cb,
            .data = call->progress.data,
        },
        .is_cancelled = {
            .cb = nrpc_call_is_cancelled,
            .data = call,
        },
        .register_cancel_hook = {
            .cb = nrpc_call_register_cancel_hook_cb,
            .data = call,
        },
        .register_progress_hook = {
            .cb = nrpc_call_register_progress_hook_cb,
            .data = call,
        },
    };
    int code = call->captured.handler(&req, call->captured.handler_data);

    return code;
}

static int nrpc_call_async_wait(struct nrpc_call *call) {
    struct nrpc_call_wait *tmp = mallocz(sizeof(struct nrpc_call_wait));
    tmp->free_with_signal = false;
    tmp->data_are_ready = false;
    tmp->host = call->host;
    tmp->call_id = strdupz(call->call_id);
    netdata_mutex_init(&tmp->mutex);
    netdata_cond_init(&tmp->cond);

    // we need a temporary BUFFER, because we may time out and the caller supplied one may vanish,
    // so we create a new one we guarantee will survive until the handler finishes...

    bool we_should_free = false;
    BUFFER *temp_wb  = buffer_create(1024, &netdata_buffers_statistics.buffers_functions); // we need it because we may give up on it
    temp_wb->content_type = call->result.wb->content_type;

    struct nrpc_request req = {
        .call_id = &call->call_id_uuid,
        .function = call->sanitized_cmd,
        .payload = call->payload,
        .user_access = call->user_access,
        .source = call->source,
        .stop_monotonic_ut = &call->stop_monotonic_ut,
        .result = {
            .wb = temp_wb,

            // we overwrite the result callbacks,
            // so that we can clean up the allocations made
            .cb = nrpc_call_signal_ready,
            .data = tmp,
        },
        .progress = {
            .cb = call->progress.cb,
            .data = call->progress.data,
        },
        .is_cancelled = {
            .cb = nrpc_call_is_cancelled,
            .data = call,
        },
        .register_cancel_hook = {
            .cb = nrpc_call_register_cancel_hook_cb,
            .data = call,
        },
        .register_progress_hook = {
            .cb = nrpc_call_register_progress_hook_cb,
            .data = call,
        },
    };
    int code = call->captured.handler(&req, call->captured.handler_data);

    // this has to happen after we execute the callback
    // because if an async call is responded in sync mode, there will be a deadlock.
    netdata_mutex_lock(&tmp->mutex);

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
                    //                    internal_error(true, "NRPC: call_id '%s' is cancelled while waiting for response",
                    //                                   call->call_id);
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

int nrpc_method_authorize(RRDHOST *host, BUFFER *result_wb, const char *cmd,
                              HTTP_ACCESS user_access, bool allow_restricted,
                              NRPC_METHOD_ACQUIRED **out_acquired) {

    if(out_acquired)
        *out_acquired = NULL;

    if(!host)
        return nrpc_call_error(result_wb, "No host given for routing this request to.",
                                       HTTP_RESP_INTERNAL_SERVER_ERROR);

    char sanitized_cmd[PLUGINSD_LINE_MAX + 1];
    size_t sanitized_cmd_length = nrpc_sanitize_name(sanitized_cmd, cmd, sizeof(sanitized_cmd));

    NRPC_METHOD_ACQUIRED *method_acquired = NULL;
    int code = nrpc_registry_find(host, result_wb, sanitized_cmd, sanitized_cmd_length, &method_acquired);
    if(code != HTTP_RESP_OK)
        return code;

    struct nrpc_method *method = nrpc_method_acquired_value(method_acquired);

    if((method->options & NRPC_METHOD_FLAG_RESTRICTED) && !allow_restricted) {
        code = nrpc_call_error(result_wb,
                                       "This feature is not available via this API.",
                                       HTTP_ACCESS_PERMISSION_DENIED_HTTP_CODE(user_access));
        nrpc_method_acquired_release(host, method_acquired);
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

        nrpc_method_acquired_release(host, method_acquired);
        return code;
    }

    if(out_acquired)
        *out_acquired = method_acquired;
    else
        nrpc_method_acquired_release(host, method_acquired);

    return HTTP_RESP_OK;
}

int nrpc_call(RRDHOST *host, BUFFER *result_wb, int timeout_s,
                     HTTP_ACCESS user_access, const char *cmd,
                     bool wait, const char *call_id,
                     nrpc_result_cb_t result_cb, void *result_cb_data,
                     nrpc_progress_cb_t progress_cb, void *progress_cb_data,
                     nrpc_is_cancelled_cb_t is_cancelled_cb, void *is_cancelled_cb_data,
                     BUFFER *payload, const char *source, bool allow_restricted) {

    int code;
    char sanitized_cmd[PLUGINSD_LINE_MAX + 1];
    NRPC_METHOD_ACQUIRED *method_acquired = NULL;

    const char *source_to_sanitize = source ? source : "";
    size_t sanitized_source_size = nrpc_strlen_bounded(source_to_sanitize, PLUGINSD_LINE_MAX) + 1;
    CLEAN_CHAR_P *sanitized_source = mallocz(sanitized_source_size);
    nrpc_sanitize_name(sanitized_source, source_to_sanitize, sanitized_source_size);

    // ------------------------------------------------------------------------
    // check for the host
    if(!host) {
        code = HTTP_RESP_INTERNAL_SERVER_ERROR;

        nrpc_call_error(result_wb, "No host given for routing this request to.", code);

        if(result_cb)
            result_cb(result_wb, code, result_cb_data);

        return code;
    }

    // ------------------------------------------------------------------------
    // find the function and verify the caller's access

    size_t sanitized_cmd_length = nrpc_sanitize_name(sanitized_cmd, cmd, sizeof(sanitized_cmd));

    code = nrpc_method_authorize(host, result_wb, cmd, user_access, allow_restricted, &method_acquired);
    if(code != HTTP_RESP_OK) {

        if(result_cb)
            result_cb(result_wb, code, result_cb_data);

        return code;
    }

    struct nrpc_method *method = nrpc_method_acquired_value(method_acquired);

    if(timeout_s <= 0)
        timeout_s = method->timeout;

    // ------------------------------------------------------------------------
    // validate and parse the call_id, or generate a new call_id id

    char uuid_str[UUID_COMPACT_STR_LEN];
    nd_uuid_t uuid;

    if(!call_id || !*call_id || uuid_parse_flexi(call_id, uuid) != 0)
        uuid_generate_random(uuid);

    uuid_unparse_lower_compact(uuid, uuid_str);
    call_id = uuid_str;

    // ------------------------------------------------------------------------
    // the function can only be executed in async mode
    // put the function into the inflight requests

    struct nrpc_call t = {
        .host = host,
        .cmd = strdupz(cmd),
        .sanitized_cmd = strdupz(sanitized_cmd),
        .sanitized_cmd_length = sanitized_cmd_length,
        .call_id = strdupz(call_id),
        .user_access = user_access,
        .source = sanitized_source,
        .payload = buffer_dup(payload),
        .timeout = timeout_s,
        .cancelled = false,
        .stop_monotonic_ut = now_monotonic_usec() + timeout_s * USEC_PER_SEC,
        .method_acquired = method_acquired,
        .method = method,
        .result = {
            .wb = result_wb,
            .cb = result_cb,
            .data = result_cb_data,
        },
        .is_cancelled = {
            .cb = is_cancelled_cb,
            .data = is_cancelled_cb_data,
        },
        .progress = {
            .cb = progress_cb,
            .data = progress_cb_data,
        },
    };
    sanitized_source = NULL;
    uuid_copy(t.call_id_uuid, uuid);

    // capture the execute pair NOW, under the entry's leaf lock, pinning the
    // transport - executors must never re-read method's pair at call time (see
    // struct nrpc_call.captured). The early error paths below
    // release the pin via nrpc_call_cleanup(&t).
    nrpc_method_capture(method_acquired, &t.captured);

    bool duplicate_call_id = false;
    struct nrpc_call *call = dictionary_set_advanced(
        nrpc_inflight_calls.dict, call_id, -1, &t, sizeof(t), &duplicate_call_id);
    if(!call) {
        // dictionary_set() returns NULL when the dictionary is destroyed (shutdown in progress)
        code = nrpc_call_error(result_wb, "Service is shutting down.", HTTP_RESP_SERVICE_UNAVAILABLE);

        nrpc_call_cleanup(&t);
        nrpc_method_acquired_release(host, t.method_acquired);

        if(result_cb)
            result_cb(result_wb, code, result_cb_data);

        return code;
    }

    if(duplicate_call_id) {
        nd_log(NDLS_DAEMON, NDLP_NOTICE,
               "NRPC: duplicate call_id '%s', method: '%s'",
               t.call_id, t.cmd);

        code = nrpc_call_error(result_wb, "Duplicate transaction.", HTTP_RESP_BAD_REQUEST);

        nrpc_call_cleanup(&t);
        nrpc_method_acquired_release(host, t.method_acquired);

        if(result_cb)
            result_cb(result_wb, code, result_cb_data);

        return code;
    }
    // internal_error(true, "NRPC: call_id '%s' started", call->call_id);

    if(call->method->sync) {
        // the caller has to wait

        struct nrpc_request req = {
            .call_id = &call->call_id_uuid,
            .function = call->sanitized_cmd,
            .payload = call->payload,
            .user_access = call->user_access,
            .source = call->source,
            .stop_monotonic_ut = &call->stop_monotonic_ut,
            .result = {
                .wb = call->result.wb,

                // we overwrite the result callbacks,
                // so that we can clean up the allocations made
                .cb = call->result.cb,
                .data = call->result.data,
            },
            .progress = {
                .cb = call->progress.cb,
                .data = call->progress.data,
            },
            .is_cancelled = {
                .cb = call->is_cancelled.cb,
                .data = call->is_cancelled.data,
            },
            .register_cancel_hook = {
                .cb = NULL,
                .data = NULL,
            },
            .register_progress_hook = {
                .cb = NULL,
                .data = NULL,
            },
        };
        code = call->captured.handler(&req, call->captured.handler_data);

        nrpc_inflight_calls_del(host, call->call_id);
        return code;
    }

    return nrpc_call_async(call, wait);
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

    bool cancelled = __atomic_load_n(&call->cancelled, __ATOMIC_RELAXED);
    if(cancelled) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "NRPC: received a CANCEL request for call_id '%s', but it is already cancelled.",
               call->call_id);
        return;
    }

    __atomic_store_n(&call->cancelled, true, __ATOMIC_RELAXED);

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
    // internal_error(true, "NRPC: request to cancel call_id '%s'", call_id);

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
