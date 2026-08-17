// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdcollector-internals.h"
#include "rrdfunctions-internals.h"
#include "rrdfunctions-inflight.h"

struct rrd_function_inflight {
    RRDHOST *host;
    nd_uuid_t transaction_uuid;
    const char *transaction;
    const char *cmd;
    const char *sanitized_cmd;
    const char *source;
    size_t sanitized_cmd_length;
    int timeout;
    bool cancelled;
    usec_t stop_monotonic_ut;

    HTTP_ACCESS user_access;

    BUFFER *payload;

    RRD_FUNCTION_ACQUIRED *host_function_acquired;

    // the collector
    // we acquire this structure at the beginning,
    // and we release it at the end
    struct rrd_host_function *rdcf;

    // capture-at-find (see rrd_function_acquired_capture): executors use THIS
    // pair, never rdcf->execute_cb / execute_cb_data re-read at call time - a
    // concurrent re-registration swaps those and frees the displaced
    // transport; the captured pin keeps ours alive and a stale capture
    // degrades to a clean 503 via the transport's acquire-or-fail
    struct rrd_function_capture captured;

    struct {
        BUFFER *wb;

        // in async mode,
        // the function to call to send the result back
        rrd_function_result_callback_t cb;
        void *data;
    } result;

    struct {
        SPINLOCK spinlock;
    } callbacks;

    struct {
        // to be called in sync mode
        // while the function is running
        // to check if the function has been canceled
        rrd_function_is_cancelled_cb_t cb;
        void *data;
    } is_cancelled;

    struct {
        // to be registered by the function itself
        // used to signal the function to cancel
        rrd_function_cancel_cb_t cb;
        void *data;
    } canceller;

    struct {
        // callback to receive progress reports from function
        rrd_function_progress_cb_t cb;
        void *data;
    } progress;

    struct {
        // to be registered by the function itself
        // used to send progress requests to function
        rrd_function_progresser_cb_t cb;
        void *data;
    } progresser;
};

// The transaction broker: every in-flight function call, keyed by its compact
// transaction id. One global instance - transactions are process-wide, not
// per-host (the record carries its host).
struct rrd_function_transactions {
    DICTIONARY *dict;
};

static struct rrd_function_transactions rrd_function_transactions = { .dict = NULL };

static void rrd_function_cancel_inflight(struct rrd_function_inflight *r);

// ----------------------------------------------------------------------------

static void rrd_functions_inflight_cleanup(struct rrd_function_inflight *r) {
    buffer_free(r->payload);
    freez((void *)r->transaction);
    freez((void *)r->cmd);
    freez((void *)r->sanitized_cmd);
    freez((void *)r->source);

    // the execute capture's transport pin (NULL for INTERNAL sources)
    rrd_function_capture_release(&r->captured);

    // the canceller/progresser registration pins: per the
    // rrd_function_cancel_cb_t contract their data is a transport, entry-
    // pinned at registration - BOTH released here, no dedup (an async record
    // can hold two). NULL-safe, so cleanup on a never-inserted record (the
    // early error paths) releases nothing.
    rrd_function_transport_entry_release(r->canceller.data);
    r->canceller.data = NULL;
    rrd_function_transport_entry_release(r->progresser.data);
    r->progresser.data = NULL;

    r->payload = NULL;
    r->transaction = NULL;
    r->cmd = NULL;
    r->sanitized_cmd = NULL;
}

static void rrd_functions_inflight_delete_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    struct rrd_function_inflight *r = value;

    // internal_error(true, "FUNCTIONS: transaction '%s' finished", r->transaction);

    rrd_functions_inflight_cleanup(r);
    rrd_function_acquired_release(r->host, r->host_function_acquired);
}

static void rrd_functions_inflight_insert_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    struct rrd_function_inflight *r = value;
    spinlock_init(&r->callbacks.spinlock);
}

static bool rrd_functions_inflight_conflict_cb(const DICTIONARY_ITEM *item __maybe_unused,
                                               void *old_value __maybe_unused,
                                               void *new_value __maybe_unused,
                                               void *data) {
    bool *duplicate_transaction = data;
    if(duplicate_transaction)
        *duplicate_transaction = true;

    return false;
}

void rrd_function_transactions_create(void) {
    if(rrd_function_transactions.dict)
        return;

    rrd_function_transactions.dict = dictionary_create_advanced(DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct rrd_function_inflight));

    dictionary_register_insert_callback(rrd_function_transactions.dict, rrd_functions_inflight_insert_cb, NULL);
    dictionary_register_delete_callback(rrd_function_transactions.dict, rrd_functions_inflight_delete_cb, NULL);
    dictionary_register_conflict_callback(rrd_function_transactions.dict, rrd_functions_inflight_conflict_cb, NULL);
}

// called ONLY from the ASAN-gated shutdown path (daemon-shutdown.c): in normal
// operation the broker lives for the process lifetime and is never torn down -
// the destroy exists so leak checking sees a clean heap
void rrd_function_transactions_destroy(void) {
    if(!rrd_function_transactions.dict)
        return;

    dictionary_destroy(rrd_function_transactions.dict);
    rrd_function_transactions.dict = NULL;
}

static void rrd_inflight_async_function_register_canceller_cb(void *register_canceller_cb_data, rrd_function_cancel_cb_t canceller_cb, void *canceller_cb_data) {
    struct rrd_function_inflight *r = register_canceller_cb_data;

    // per the rrd_function_cancel_cb_t contract, canceller_cb_data is the
    // registrar's transport: entry-pin it for the record's lifetime, so a late
    // cancel after the transport's owner died acquire-fails on a valid (dead)
    // transport instead of chasing a freed pointer. Released in
    // rrd_functions_inflight_cleanup(). A re-registration (none exists today)
    // releases the previous pin so it cannot leak.
    rrd_function_transport_entry_acquire(canceller_cb_data);

    spinlock_lock(&r->callbacks.spinlock);
    struct rrd_function_transport *previous = r->canceller.data;
    r->canceller.cb = canceller_cb;
    r->canceller.data = canceller_cb_data;
    spinlock_unlock(&r->callbacks.spinlock);

    rrd_function_transport_entry_release(previous);
}

static void rrd_inflight_async_function_register_progresser_cb(void *register_progresser_cb_data, rrd_function_progresser_cb_t progresser_cb, void *progresser_cb_data) {
    struct rrd_function_inflight *r = register_progresser_cb_data;

    // same contract, pin and re-registration handling as the canceller above
    rrd_function_transport_entry_acquire(progresser_cb_data);

    spinlock_lock(&r->callbacks.spinlock);
    struct rrd_function_transport *previous = r->progresser.data;
    r->progresser.cb = progresser_cb;
    r->progresser.data = progresser_cb_data;
    spinlock_unlock(&r->callbacks.spinlock);

    rrd_function_transport_entry_release(previous);
}

// ----------------------------------------------------------------------------
// waiting for async function completion

struct rrd_function_call_wait {
    RRDHOST *host;
    char *transaction;

    bool free_with_signal;
    bool data_are_ready;
    netdata_mutex_t mutex;
    netdata_cond_t cond;
    int code;
};

static void rrd_inflight_function_cleanup(RRDHOST *host __maybe_unused, const char *transaction) {
    dictionary_del(rrd_function_transactions.dict, transaction);
    dictionary_garbage_collect(rrd_function_transactions.dict);
}

static void rrd_function_call_wait_free(struct rrd_function_call_wait *tmp) {
    rrd_inflight_function_cleanup(tmp->host, tmp->transaction);
    freez(tmp->transaction);

    netdata_cond_destroy(&tmp->cond);
    netdata_mutex_destroy(&tmp->mutex);
    freez(tmp);
}

static void rrd_async_function_signal_when_ready(BUFFER *temp_wb __maybe_unused, int code, void *callback_data) {
    struct rrd_function_call_wait *tmp = callback_data;
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
        rrd_function_call_wait_free(tmp);
    }
}

static void rrd_inflight_async_function_nowait_finished(BUFFER *wb, int code, void *data) {
    struct rrd_function_inflight *r = data;

    if(r->result.cb)
        r->result.cb(wb, code, r->result.data);

    rrd_inflight_function_cleanup(r->host, r->transaction);
}

static bool rrd_inflight_async_function_is_cancelled(void *data) {
    struct rrd_function_inflight *r = data;
    return __atomic_load_n(&r->cancelled, __ATOMIC_RELAXED);
}

static inline int rrd_call_function_async_and_dont_wait(struct rrd_function_inflight *r) {
    struct rrd_function_execute rfe = {
        .transaction = &r->transaction_uuid,
        .function = r->sanitized_cmd,
        .payload = r->payload,
        .user_access = r->user_access,
        .source = r->source,
        .stop_monotonic_ut = &r->stop_monotonic_ut,
        .result = {
            .wb = r->result.wb,
            .cb = rrd_inflight_async_function_nowait_finished,
            .data = r,
        },
        .progress = {
            .cb = r->progress.cb,
            .data = r->progress.data,
        },
        .is_cancelled = {
            .cb = rrd_inflight_async_function_is_cancelled,
            .data = r,
        },
        .register_canceller = {
            .cb = rrd_inflight_async_function_register_canceller_cb,
            .data = r,
        },
        .register_progresser = {
            .cb = rrd_inflight_async_function_register_progresser_cb,
            .data = r,
        },
    };
    int code = r->captured.execute_cb(&rfe, r->captured.execute_cb_data);

    return code;
}

static int rrd_call_function_async_and_wait(struct rrd_function_inflight *r) {
    struct rrd_function_call_wait *tmp = mallocz(sizeof(struct rrd_function_call_wait));
    tmp->free_with_signal = false;
    tmp->data_are_ready = false;
    tmp->host = r->host;
    tmp->transaction = strdupz(r->transaction);
    netdata_mutex_init(&tmp->mutex);
    netdata_cond_init(&tmp->cond);

    // we need a temporary BUFFER, because we may time out and the caller supplied one may vanish,
    // so we create a new one we guarantee will survive until the collector finishes...

    bool we_should_free = false;
    BUFFER *temp_wb  = buffer_create(1024, &netdata_buffers_statistics.buffers_functions); // we need it because we may give up on it
    temp_wb->content_type = r->result.wb->content_type;

    struct rrd_function_execute rfe = {
        .transaction = &r->transaction_uuid,
        .function = r->sanitized_cmd,
        .payload = r->payload,
        .user_access = r->user_access,
        .source = r->source,
        .stop_monotonic_ut = &r->stop_monotonic_ut,
        .result = {
            .wb = temp_wb,

            // we overwrite the result callbacks,
            // so that we can clean up the allocations made
            .cb = rrd_async_function_signal_when_ready,
            .data = tmp,
        },
        .progress = {
            .cb = r->progress.cb,
            .data = r->progress.data,
        },
        .is_cancelled = {
            .cb = rrd_inflight_async_function_is_cancelled,
            .data = r,
        },
        .register_canceller = {
            .cb = rrd_inflight_async_function_register_canceller_cb,
            .data = r,
        },
        .register_progresser = {
            .cb = rrd_inflight_async_function_register_progresser_cb,
            .data = r,
        },
    };
    int code = r->captured.execute_cb(&rfe, r->captured.execute_cb_data);

    // this has to happen after we execute the callback
    // because if an async call is responded in sync mode, there will be a deadlock.
    netdata_mutex_lock(&tmp->mutex);

    if (code == HTTP_RESP_OK || tmp->data_are_ready) {
        bool cancelled = false;
        int rc = 0;
        while (rc == 0 && !cancelled && !tmp->data_are_ready) {
            usec_t now_mono_ut = now_monotonic_usec();
            usec_t stop_mono_ut = __atomic_load_n(&r->stop_monotonic_ut, __ATOMIC_RELAXED) + RRDFUNCTIONS_TIMEOUT_EXTENSION_UT;
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
                if (!tmp->data_are_ready && r->is_cancelled.cb &&
                    r->is_cancelled.cb(r->is_cancelled.data)) {
                    //                    internal_error(true, "FUNCTIONS: transaction '%s' is cancelled while waiting for response",
                    //                                   r->transaction);
                    cancelled = true;
                    rrd_function_cancel_inflight(r);
                    break;
                }
            }
        }

        if (tmp->data_are_ready) {
            // we have a response

            buffer_contents_replace(r->result.wb, buffer_tostring(temp_wb), buffer_strlen(temp_wb));
            r->result.wb->content_type = temp_wb->content_type;
            r->result.wb->expires = temp_wb->expires;

            if(r->result.wb->expires)
                buffer_cacheable(r->result.wb);
            else
                buffer_no_cacheable(r->result.wb);

            code = tmp->code;

            tmp->free_with_signal = false;
            we_should_free = true;
        }
        else if (rc == UV_ETIMEDOUT || cancelled) {
            // timeout
            // we will go away and let the callback free the structure

            if(cancelled)
                code = rrd_call_function_error(r->result.wb,
                                               "Request cancelled",
                                               HTTP_RESP_CLIENT_CLOSED_REQUEST);
            else
                code = rrd_call_function_error(r->result.wb,
                                               "Timeout while waiting for a response from the plugin that serves this features",
                                               HTTP_RESP_GATEWAY_TIMEOUT);

            tmp->free_with_signal = true;
            we_should_free = false;
        }
        else {
            code = rrd_call_function_error(
                r->result.wb, "Internal error while communicating with the plugin that serves this feature.",
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
        rrd_function_call_wait_free(tmp);
        buffer_free(temp_wb);
    }

    return code;
}

static inline int rrd_call_function_async(struct rrd_function_inflight *r, bool wait) {
    if(wait)
        return rrd_call_function_async_and_wait(r);
    else
        return rrd_call_function_async_and_dont_wait(r);
}


// ----------------------------------------------------------------------------

int rrd_function_verify_access(RRDHOST *host, BUFFER *result_wb, const char *cmd,
                              HTTP_ACCESS user_access, bool allow_restricted,
                              RRD_FUNCTION_ACQUIRED **out_acquired) {

    if(out_acquired)
        *out_acquired = NULL;

    if(!host)
        return rrd_call_function_error(result_wb, "No host given for routing this request to.",
                                       HTTP_RESP_INTERNAL_SERVER_ERROR);

    char sanitized_cmd[PLUGINSD_LINE_MAX + 1];
    size_t sanitized_cmd_length = rrd_functions_sanitize(sanitized_cmd, cmd, sizeof(sanitized_cmd));

    RRD_FUNCTION_ACQUIRED *host_function_acquired = NULL;
    int code = rrd_functions_find_by_name(host, result_wb, sanitized_cmd, sanitized_cmd_length, &host_function_acquired);
    if(code != HTTP_RESP_OK)
        return code;

    struct rrd_host_function *rdcf = rrd_function_acquired_value(host_function_acquired);

    if((rdcf->options & RRD_FUNCTION_RESTRICTED) && !allow_restricted) {
        code = rrd_call_function_error(result_wb,
                                       "This feature is not available via this API.",
                                       HTTP_ACCESS_PERMISSION_DENIED_HTTP_CODE(user_access));
        rrd_function_acquired_release(host, host_function_acquired);
        return code;
    }

    if(!http_access_user_has_enough_access_level_for_endpoint(user_access, rdcf->access)) {

        if((rdcf->access & HTTP_ACCESS_SIGNED_ID) && !(user_access & HTTP_ACCESS_SIGNED_ID))
            code = rrd_call_function_error(result_wb,
                                           "You need to be authenticated via Netdata Cloud Single-Sign-On (SSO) "
                                           "to access this feature. Sign-in on this dashboard, "
                                           "or access your Netdata via https://app.netdata.cloud.",
                                           HTTP_ACCESS_PERMISSION_DENIED_HTTP_CODE(user_access));

        else if((rdcf->access & HTTP_ACCESS_SAME_SPACE) && !(user_access & HTTP_ACCESS_SAME_SPACE))
            code = rrd_call_function_error(result_wb,
                                           "You need to login to the Netdata Cloud space this agent is claimed to, "
                                           "to access this feature.",
                                           HTTP_ACCESS_PERMISSION_DENIED_HTTP_CODE(user_access));

        else if((rdcf->access & HTTP_ACCESS_COMMERCIAL_SPACE) && !(user_access & HTTP_ACCESS_COMMERCIAL_SPACE))
            code = rrd_call_function_error(result_wb,
                                           "This feature is only available for commercial users and supporters "
                                           "of Netdata. To use it, please upgrade your space. "
                                           "Thank you for supporting Netdata.",
                                           HTTP_ACCESS_PERMISSION_DENIED_HTTP_CODE(user_access));

        else {
            HTTP_ACCESS missing_access = (~user_access) & rdcf->access;
            char perms_str[1024];
            http_access2txt(perms_str, sizeof(perms_str), ", ", missing_access);

            char msg[2048];
            snprintfz(msg, sizeof(msg), "This feature requires additional permissions: %s.", perms_str);

            code = rrd_call_function_error(result_wb, msg,
                                           HTTP_ACCESS_PERMISSION_DENIED_HTTP_CODE(user_access));
        }

        rrd_function_acquired_release(host, host_function_acquired);
        return code;
    }

    if(out_acquired)
        *out_acquired = host_function_acquired;
    else
        rrd_function_acquired_release(host, host_function_acquired);

    return HTTP_RESP_OK;
}

int rrd_function_run(RRDHOST *host, BUFFER *result_wb, int timeout_s,
                     HTTP_ACCESS user_access, const char *cmd,
                     bool wait, const char *transaction,
                     rrd_function_result_callback_t result_cb, void *result_cb_data,
                     rrd_function_progress_cb_t progress_cb, void *progress_cb_data,
                     rrd_function_is_cancelled_cb_t is_cancelled_cb, void *is_cancelled_cb_data,
                     BUFFER *payload, const char *source, bool allow_restricted) {

    int code;
    char sanitized_cmd[PLUGINSD_LINE_MAX + 1];
    RRD_FUNCTION_ACQUIRED *host_function_acquired = NULL;

    const char *source_to_sanitize = source ? source : "";
    size_t sanitized_source_size = rrd_functions_strlen_bounded(source_to_sanitize, PLUGINSD_LINE_MAX) + 1;
    CLEAN_CHAR_P *sanitized_source = mallocz(sanitized_source_size);
    rrd_functions_sanitize(sanitized_source, source_to_sanitize, sanitized_source_size);

    // ------------------------------------------------------------------------
    // check for the host
    if(!host) {
        code = HTTP_RESP_INTERNAL_SERVER_ERROR;

        rrd_call_function_error(result_wb, "No host given for routing this request to.", code);

        if(result_cb)
            result_cb(result_wb, code, result_cb_data);

        return code;
    }

    // ------------------------------------------------------------------------
    // find the function and verify the caller's access

    size_t sanitized_cmd_length = rrd_functions_sanitize(sanitized_cmd, cmd, sizeof(sanitized_cmd));

    code = rrd_function_verify_access(host, result_wb, cmd, user_access, allow_restricted, &host_function_acquired);
    if(code != HTTP_RESP_OK) {

        if(result_cb)
            result_cb(result_wb, code, result_cb_data);

        return code;
    }

    struct rrd_host_function *rdcf = rrd_function_acquired_value(host_function_acquired);

    if(timeout_s <= 0)
        timeout_s = rdcf->timeout;

    // ------------------------------------------------------------------------
    // validate and parse the transaction, or generate a new transaction id

    char uuid_str[UUID_COMPACT_STR_LEN];
    nd_uuid_t uuid;

    if(!transaction || !*transaction || uuid_parse_flexi(transaction, uuid) != 0)
        uuid_generate_random(uuid);

    uuid_unparse_lower_compact(uuid, uuid_str);
    transaction = uuid_str;

    // ------------------------------------------------------------------------
    // the function can only be executed in async mode
    // put the function into the inflight requests

    struct rrd_function_inflight t = {
        .host = host,
        .cmd = strdupz(cmd),
        .sanitized_cmd = strdupz(sanitized_cmd),
        .sanitized_cmd_length = sanitized_cmd_length,
        .transaction = strdupz(transaction),
        .user_access = user_access,
        .source = sanitized_source,
        .payload = buffer_dup(payload),
        .timeout = timeout_s,
        .cancelled = false,
        .stop_monotonic_ut = now_monotonic_usec() + timeout_s * USEC_PER_SEC,
        .host_function_acquired = host_function_acquired,
        .rdcf = rdcf,
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
    uuid_copy(t.transaction_uuid, uuid);

    // capture the execute pair NOW, under the entry's leaf lock, pinning the
    // transport - executors must never re-read rdcf's pair at call time (see
    // struct rrd_function_inflight.captured). The early error paths below
    // release the pin via rrd_functions_inflight_cleanup(&t).
    rrd_function_acquired_capture(host_function_acquired, &t.captured);

    bool duplicate_transaction = false;
    struct rrd_function_inflight *r = dictionary_set_advanced(
        rrd_function_transactions.dict, transaction, -1, &t, sizeof(t), &duplicate_transaction);
    if(!r) {
        // dictionary_set() returns NULL when the dictionary is destroyed (shutdown in progress)
        code = rrd_call_function_error(result_wb, "Service is shutting down.", HTTP_RESP_SERVICE_UNAVAILABLE);

        rrd_functions_inflight_cleanup(&t);
        rrd_function_acquired_release(host, t.host_function_acquired);

        if(result_cb)
            result_cb(result_wb, code, result_cb_data);

        return code;
    }

    if(duplicate_transaction) {
        nd_log(NDLS_DAEMON, NDLP_NOTICE,
               "FUNCTIONS: duplicate transaction '%s', function: '%s'",
               t.transaction, t.cmd);

        code = rrd_call_function_error(result_wb, "Duplicate transaction.", HTTP_RESP_BAD_REQUEST);

        rrd_functions_inflight_cleanup(&t);
        rrd_function_acquired_release(host, t.host_function_acquired);

        if(result_cb)
            result_cb(result_wb, code, result_cb_data);

        return code;
    }
    // internal_error(true, "FUNCTIONS: transaction '%s' started", r->transaction);

    if(r->rdcf->sync) {
        // the caller has to wait

        struct rrd_function_execute rfe = {
            .transaction = &r->transaction_uuid,
            .function = r->sanitized_cmd,
            .payload = r->payload,
            .user_access = r->user_access,
            .source = r->source,
            .stop_monotonic_ut = &r->stop_monotonic_ut,
            .result = {
                .wb = r->result.wb,

                // we overwrite the result callbacks,
                // so that we can clean up the allocations made
                .cb = r->result.cb,
                .data = r->result.data,
            },
            .progress = {
                .cb = r->progress.cb,
                .data = r->progress.data,
            },
            .is_cancelled = {
                .cb = r->is_cancelled.cb,
                .data = r->is_cancelled.data,
            },
            .register_canceller = {
                .cb = NULL,
                .data = NULL,
            },
            .register_progresser = {
                .cb = NULL,
                .data = NULL,
            },
        };
        code = r->captured.execute_cb(&rfe, r->captured.execute_cb_data);

        rrd_inflight_function_cleanup(host, r->transaction);
        return code;
    }

    return rrd_call_function_async(r, wait);
}

bool rrd_function_has_this_original_result_callback(nd_uuid_t *transaction, rrd_function_result_callback_t cb) {
    bool ret = false;
    char str[UUID_COMPACT_STR_LEN];
    uuid_unparse_lower_compact(*transaction, str);
    const DICTIONARY_ITEM *item = dictionary_get_and_acquire_item(rrd_function_transactions.dict, str);
    if(item) {
        struct rrd_function_inflight *r = dictionary_acquired_item_value(item);
        if(r->result.cb == cb)
            ret = true;

        dictionary_acquired_item_release(rrd_function_transactions.dict, item);
    }
    return ret;
}

static void rrd_function_cancel_inflight(struct rrd_function_inflight *r) {
    if(!r)
        return;

    bool cancelled = __atomic_load_n(&r->cancelled, __ATOMIC_RELAXED);
    if(cancelled) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "FUNCTIONS: received a CANCEL request for transaction '%s', but it is already cancelled.",
               r->transaction);
        return;
    }

    __atomic_store_n(&r->cancelled, true, __ATOMIC_RELAXED);

    if(!rrd_collector_dispatcher_acquire(r->rdcf->collector)) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "FUNCTIONS: received a CANCEL request for transaction '%s', but the collector is not running.",
               r->transaction);
        return;
    }

    rrd_function_cancel_cb_t canceller_cb;
    void *canceller_cb_data;

    spinlock_lock(&r->callbacks.spinlock);
    canceller_cb = r->canceller.cb;
    canceller_cb_data = r->canceller.data;
    spinlock_unlock(&r->callbacks.spinlock);

    if(canceller_cb)
        canceller_cb(r->transaction, canceller_cb_data);

    rrd_collector_dispatcher_release(r->rdcf->collector);
}

void rrd_function_cancel(const char *transaction) {
    // internal_error(true, "FUNCTIONS: request to cancel transaction '%s'", transaction);

    const DICTIONARY_ITEM *item = dictionary_get_and_acquire_item(rrd_function_transactions.dict, transaction);
    if(!item) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "FUNCTIONS: received a CANCEL request for transaction '%s', but the transaction is not running.",
               transaction);
        return;
    }

    struct rrd_function_inflight *r = dictionary_acquired_item_value(item);
    rrd_function_cancel_inflight(r);
    dictionary_acquired_item_release(rrd_function_transactions.dict, item);
}

void rrd_function_progress(const char *transaction) {
    const DICTIONARY_ITEM *item = dictionary_get_and_acquire_item(rrd_function_transactions.dict, transaction);
    if(!item) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "FUNCTIONS: received a PROGRESS request for transaction '%s', but the transaction is not running.",
               transaction);
        return;
    }

    struct rrd_function_inflight *r = dictionary_acquired_item_value(item);

    if(!rrd_collector_dispatcher_acquire(r->rdcf->collector)) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "FUNCTIONS: received a PROGRESS request for transaction '%s', but the collector is not running.",
               transaction);
        goto cleanup;
    }

    functions_stop_monotonic_update_on_progress(&r->stop_monotonic_ut);

    rrd_function_progresser_cb_t progresser_cb;
    void *progresser_cb_data;

    spinlock_lock(&r->callbacks.spinlock);
    progresser_cb = r->progresser.cb;
    progresser_cb_data = r->progresser.data;
    spinlock_unlock(&r->callbacks.spinlock);

    if(progresser_cb)
        progresser_cb(transaction, progresser_cb_data);

    rrd_collector_dispatcher_release(r->rdcf->collector);

cleanup:
    dictionary_acquired_item_release(rrd_function_transactions.dict, item);
}

void rrd_function_call_progresser(nd_uuid_t *transaction) {
    if(uuid_is_null(*transaction))
        return;

    char str[UUID_COMPACT_STR_LEN];
    uuid_unparse_lower_compact(*transaction, str);
    rrd_function_progress(str);
}
