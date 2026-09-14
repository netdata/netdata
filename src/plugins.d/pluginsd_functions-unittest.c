// SPDX-License-Identifier: GPL-3.0-or-later

#include "pluginsd_functions.h"
#include "nrpc/nrpc-internals.h"
#include "daemon/dyncfg/dyncfg-internals.h"
#include "daemon/dyncfg/dyncfg.h"

// ----------------------------------------------------------------------------
// Pins the transport lifetime model (step 6 of the function-mechanism
// refactor): the refcount accounting of the registry entries and the dyncfg
// node pin, and the deferred-result delivery discipline of the pluginsd
// transport (UAF-A/B/C/D fixes).
//
//   1. RESULT_BEGIN without END + parser destroy => the caller is answered
//      exactly once with 503 (a truncated stream must not report the 2xx the
//      plugin sent in RESULT_BEGIN), and the inflight dictionary is not
//      queued-for-destruction forever behind the defer reference.
//   2. mixed-source re-registration (COLLECTOR-over-INTERNAL and back)
//      neither crashes nor leaks transport refs.
//   3. an equal-pointer conflict (a re-sent function list) nets ZERO refs.
//   4. a GC delete landing mid-defer followed by RESULT_END delivers the
//      result at result_end - through the detach-then-deliver sweep - exactly
//      once.
//   5. a dyncfg node re-CREATE from the same parser with equal cb/data nets
//      zero pins.
//   6. two GC passes racing on the same parser (serialized only by the dict
//      write lock) cancel an expired transaction exactly once - no duplicate
//      FUNCTION_CANCEL on the wire.

struct c6ut_result {
    size_t calls;
    int code;
};

// mimics the in-flight table's registration callbacks: stores the (cb, data) pair and
// entry-pins the transport, per the nrpc_cancel_hook_cb_t contract
struct c6ut_cancel_hook {
    nrpc_cancel_hook_cb_t cb;
    struct nrpc_transport *transport; // entry-pinned
};

static void c6ut_register_cancel_hook_cb(void *register_cancel_hook_cb_data, nrpc_cancel_hook_cb_t cancel_hook_cb, void *cancel_hook_cb_data) {
    struct c6ut_cancel_hook *c = register_cancel_hook_cb_data;
    c->cb = cancel_hook_cb;
    c->transport = nrpc_transport_entry_acquire(cancel_hook_cb_data);
}

// same, for the progress_hook half of the registration contract
struct c6ut_progress_hook {
    nrpc_progress_hook_cb_t cb;
    struct nrpc_transport *transport; // entry-pinned
};

static void c6ut_register_progress_hook_cb(void *register_progress_hook_cb_data, nrpc_progress_hook_cb_t progress_hook_cb, void *progress_hook_cb_data) {
    struct c6ut_progress_hook *p = register_progress_hook_cb_data;
    p->cb = progress_hook_cb;
    p->transport = nrpc_transport_entry_acquire(progress_hook_cb_data);
}

static void c6ut_result_cb(BUFFER *wb __maybe_unused, int code, void *data) {
    struct c6ut_result *r = data;
    r->calls++;
    r->code = code;
}

struct c6ut_sends {
    size_t total;
    size_t cancels;
    size_t progresses;
    bool fail;              // simulate a plugin whose pipe is already gone
};

static ssize_t c6ut_send_cb(const char *txt, void *data, STREAM_TRAFFIC_TYPE type __maybe_unused) {
    struct c6ut_sends *s = data;
    s->total++;
    if(strstr(txt, PLUGINSD_CALL_FUNCTION_CANCEL))
        s->cancels++;
    if(strstr(txt, PLUGINSD_CALL_FUNCTION_PROGRESS))
        s->progresses++;
    if(s->fail)
        return -1;
    return (ssize_t)strlen(txt);
}

static int c6ut_noop_execute_cb(struct nrpc_request *req __maybe_unused, void *data __maybe_unused) {
    return HTTP_RESP_OK;
}

// an async function that never completes on its own: captures the transaction
// and the in-flight table's result callback so the test can finish it explicitly
static struct {
    char tx[UUID_COMPACT_STR_LEN];
    BUFFER *result_wb;
    nrpc_result_cb_t result_cb;
    void *result_cb_data;
} c5b_capture;

static int c5b_async_execute_cb(struct nrpc_request *req, void *data __maybe_unused) {
    uuid_unparse_lower_compact(*req->call_id, c5b_capture.tx);
    c5b_capture.result_wb = req->result.wb;
    c5b_capture.result_cb = req->result.cb;
    c5b_capture.result_cb_data = req->result.data;
    return HTTP_RESP_OK;
}

// an async function that registers a cancel_hook and a progress_hook with
// transports of the test's choosing, so the record's registration PINS can be
// counted (and a re-registration's release verified)
static struct {
    struct nrpc_transport *first;
    struct nrpc_transport *second;
    size_t cancels;
    size_t progresses;
    char tx[UUID_COMPACT_STR_LEN];
    BUFFER *result_wb;
    nrpc_result_cb_t result_cb;
    void *result_cb_data;
} c7_pin;

static void c7_pin_cancel_hook(const char *transaction __maybe_unused, void *data __maybe_unused) {
    c7_pin.cancels++;
}

static void c7_pin_progress_hook(const char *transaction __maybe_unused, void *data __maybe_unused) {
    c7_pin.progresses++;
}

static int c7_pin_execute_cb(struct nrpc_request *req, void *data __maybe_unused) {
    uuid_unparse_lower_compact(*req->call_id, c7_pin.tx);
    c7_pin.result_wb = req->result.wb;
    c7_pin.result_cb = req->result.cb;
    c7_pin.result_cb_data = req->result.data;

    if(req->register_cancel_hook.cb) {
        // registering twice: the second registration must release the pin the
        // first one took, or the first transport leaks a ref forever
        req->register_cancel_hook.cb(req->register_cancel_hook.data, c7_pin_cancel_hook, c7_pin.first);
        req->register_cancel_hook.cb(req->register_cancel_hook.data, c7_pin_cancel_hook, c7_pin.second);
    }

    if(req->register_progress_hook.cb)
        req->register_progress_hook.cb(req->register_progress_hook.data, c7_pin_progress_hook, c7_pin.second);

    return HTTP_RESP_OK;
}

// a SYNC function: it must be executed inline, with no cancel_hook/progress_hook
// registration hooks, and must leave no in-flight call record behind
static struct {
    char tx[UUID_COMPACT_STR_LEN];
    bool has_cancel_hook;
    bool has_progress_hook;
    bool deadline_visible;
} c7_sync;

static int c7_sync_execute_cb(struct nrpc_request *req, void *data __maybe_unused) {
    uuid_unparse_lower_compact(*req->call_id, c7_sync.tx);
    c7_sync.has_cancel_hook = (req->register_cancel_hook.cb != NULL);
    c7_sync.has_progress_hook = (req->register_progress_hook.cb != NULL);

    usec_t d = 0;
    c7_sync.deadline_visible = nrpc_call_deadline(c7_sync.tx, &d);

    if(req->result.cb)
        req->result.cb(req->result.wb, HTTP_RESP_OK, req->result.data);

    return HTTP_RESP_OK;
}

// two of these race on the same parser to exercise the window between a GC
// pass's unlock and its deferred dels (see gc_collected)
struct c6ut_gc_race {
    PARSER *parser;
    usec_t now_ut;
};

static void c6ut_gc_race_thread(void *arg) {
    struct c6ut_gc_race *g = arg;
    pluginsd_calls_garbage_collect(g->parser, g->now_ut);
}

static PARSER *c6ut_parser_create(struct c6ut_sends *sends) {
    struct parser_user_object user = { 0 };
    PARSER *parser = parser_init(&user, -1, -1, PARSER_INPUT_SPLIT, NULL);
    parser->send_to_plugin_cb = c6ut_send_cb;
    parser->send_to_plugin_data = sends;
    pluginsd_keywords_init(parser, PARSER_INIT_PLUGINSD);
    return parser;
}

// drive one function call into the parser's transport with a caller-chosen
// transaction id (so duplicates can be forced)
static int c6ut_execute_tx(PARSER *parser, const char *fn, nd_uuid_t *uuid, usec_t *stop_ut,
                           BUFFER *wb, struct c6ut_result *res,
                           struct c6ut_cancel_hook *cancel_hook,
                           struct c6ut_progress_hook *progress_hook) {
    struct nrpc_request req = {
        .call_id = uuid,
        .function = fn,
        .payload = NULL,
        .source = "unittest",
        .user_access = HTTP_ACCESS_ALL,
        .stop_monotonic_ut = stop_ut,
        .result = {
            .wb = wb,
            .cb = c6ut_result_cb,
            .data = res,
        },
        .register_cancel_hook = {
            .cb = cancel_hook ? c6ut_register_cancel_hook_cb : NULL,
            .data = cancel_hook,
        },
        .register_progress_hook = {
            .cb = progress_hook ? c6ut_register_progress_hook_cb : NULL,
            .data = progress_hook,
        },
    };

    return pluginsd_nrpc_handler(&req, parser->inflight.transport);
}

// drive one function call into the parser's transport, returning the compact
// transaction id in tx
static int c6ut_execute(PARSER *parser, const char *fn, usec_t *stop_ut,
                        BUFFER *wb, struct c6ut_result *res,
                        char tx[UUID_COMPACT_STR_LEN],
                        struct c6ut_cancel_hook *cancel_hook) {
    nd_uuid_t uuid;
    uuid_generate_random(uuid);
    uuid_unparse_lower_compact(uuid, tx);

    return c6ut_execute_tx(parser, fn, &uuid, stop_ut, wb, res, cancel_hook, NULL);
}

static void c6ut_result_begin(PARSER *parser, const char *tx, const char *status) {
    char w0[] = "FUNCTION_RESULT_BEGIN";
    char w2[16], w3[] = "text/plain", w4[] = "1";
    strncpyz(w2, status, sizeof(w2) - 1);
    char txbuf[UUID_COMPACT_STR_LEN];
    strncpyz(txbuf, tx, sizeof(txbuf) - 1);
    char *words[] = { w0, txbuf, w2, w3, w4 };
    pluginsd_function_result_begin(words, 5, parser);
}

// mimic the parser's defer completion (parser.h): invoke the action, then
// reset the defer state exactly like the parser loop does
static void c6ut_defer_complete(PARSER *parser) {
    parser->flags &= ~PARSER_DEFER_UNTIL_KEYWORD;
    parser->defer.action(parser, parser->defer.action_data);
    parser->defer.action = NULL;
    parser->defer.action_data = NULL;
    parser->defer.end_keyword = NULL;
    parser->defer.response = NULL;
}

int pluginsd_functions_unittest(void) {
    fprintf(stderr, "\n%s() running...\n", __FUNCTION__);

    RRDHOST *host = localhost;
    if(!host) {
        fprintf(stderr, "  FAILED: localhost (or its functions registry) is NULL\n");
        return 1;
    }

    int errors = 0;

    // 1. RESULT_BEGIN without END + parser destroy => exactly one 503
    {
        int before = errors;
        struct c6ut_sends sends = { 0 };
        struct c6ut_result res = { 0 };
        PARSER *parser = c6ut_parser_create(&sends);

        usec_t stop_ut = now_monotonic_usec() + 60 * USEC_PER_SEC;
        CLEAN_BUFFER *wb = buffer_create(0, NULL);
        char tx[UUID_COMPACT_STR_LEN];

        struct c6ut_cancel_hook cancel_hook = { 0 };
        int code = c6ut_execute(parser, "c6-truncated-fn", &stop_ut, wb, &res, tx, &cancel_hook);
        if(code != HTTP_RESP_OK) {
            fprintf(stderr, "  FAILED truncation: execute returned %d\n", code);
            errors++;
        }

        c6ut_result_begin(parser, tx, "200"); // the plugin claims success...
        if(!parser->defer.item) {
            fprintf(stderr, "  FAILED truncation: RESULT_BEGIN did not stash the acquired item\n");
            errors++;
        }

        // the keyed cancel_hook, while the plugin lives: O(1) lookup + CANCEL send
        if(!cancel_hook.cb || !cancel_hook.transport) {
            fprintf(stderr, "  FAILED truncation: no cancel_hook was registered\n");
            errors++;
        }
        else {
            size_t cancels_before = sends.cancels;
            cancel_hook.cb(tx, cancel_hook.transport);
            if(sends.cancels != cancels_before + 1) {
                fprintf(stderr, "  FAILED truncation: live cancel did not send a CANCEL to the plugin\n");
                errors++;
            }
        }

        parser_destroy(parser); // ...but dies before RESULT_END

        // the keyed cancel_hook AFTER death: the dispatcher acquire fails on the
        // pinned (dead) transport - no send, no crash
        if(cancel_hook.cb) {
            size_t cancels_before = sends.cancels;
            cancel_hook.cb(tx, cancel_hook.transport);
            if(sends.cancels != cancels_before) {
                fprintf(stderr, "  FAILED truncation: cancel after parser death still sent to the plugin\n");
                errors++;
            }
        }
        nrpc_transport_entry_release(cancel_hook.transport);

        if(res.calls != 1) {
            fprintf(stderr, "  FAILED truncation: result delivered %zu times, expected exactly once\n", res.calls);
            errors++;
        }
        else if(res.code != HTTP_RESP_SERVICE_UNAVAILABLE) {
            fprintf(stderr, "  FAILED truncation: got code %d, expected 503 (2xx must not survive a truncated stream)\n", res.code);
            errors++;
        }

        if(errors == before)
            fprintf(stderr, "  OK truncation: RESULT_BEGIN + parser destroy delivered exactly one 503\n");
    }

    // 2 + 3. registry transport refcounts: mixed-source re-registration and
    //        the equal-pointer conflict
    {
        int before = errors;
        struct nrpc_transport *tr = nrpc_transport_create(NULL);
        // base ref == 1

        // COLLECTOR with transport: descriptor construction acquires -> 2
        nrpc_method_register(&(struct nrpc_method_desc) {
            .owner = rrdhost_nrpc_owner(host),
            .name = "c6-mixed-fn",
            .help = "mixed",
            .tags = "top",
            .timeout_s = 10,
            .priority = 0,
            .version = 1,
            .access = HTTP_ACCESS_ANONYMOUS_DATA,
            .sync = true,
            .source = NRPC_SOURCE_PLUGIN,
            .handler = c6ut_noop_execute_cb,
            .handler_data = tr,
        });
        if(refcount_references(&tr->lifetime.entry_refcount) != 2) {
            fprintf(stderr, "  FAILED refs: after COLLECTOR add, entry refs %d != 2\n",
                    refcount_references(&tr->lifetime.entry_refcount));
            errors++;
        }

        // INTERNAL displaces it: the displaced descriptor's destructor releases under its stored tag -> 1
        nrpc_method_register(&(struct nrpc_method_desc) {
            .owner = rrdhost_nrpc_owner(host),
            .name = "c6-mixed-fn",
            .help = "mixed",
            .tags = "top",
            .timeout_s = 10,
            .priority = 0,
            .version = 1,
            .access = HTTP_ACCESS_ANONYMOUS_DATA,
            .sync = true,
            .source = NRPC_SOURCE_DAEMON,
            .handler = c6ut_noop_execute_cb,
        });
        if(refcount_references(&tr->lifetime.entry_refcount) != 1) {
            fprintf(stderr, "  FAILED refs: after INTERNAL re-add, entry refs %d != 1\n",
                    refcount_references(&tr->lifetime.entry_refcount));
            errors++;
        }

        // and back to COLLECTOR: the installed descriptor holds its ref -> 2
        nrpc_method_register(&(struct nrpc_method_desc) {
            .owner = rrdhost_nrpc_owner(host),
            .name = "c6-mixed-fn",
            .help = "mixed",
            .tags = "top",
            .timeout_s = 10,
            .priority = 0,
            .version = 1,
            .access = HTTP_ACCESS_ANONYMOUS_DATA,
            .sync = true,
            .source = NRPC_SOURCE_PLUGIN,
            .handler = c6ut_noop_execute_cb,
            .handler_data = tr,
        });
        if(refcount_references(&tr->lifetime.entry_refcount) != 2) {
            fprintf(stderr, "  FAILED refs: after COLLECTOR re-add, entry refs %d != 2\n",
                    refcount_references(&tr->lifetime.entry_refcount));
            errors++;
        }

        // identical re-registration (a re-sent function list): the incoming
        // descriptor's ref and the displaced one's release cancel out - nets
        // ZERO refs
        nrpc_method_register(&(struct nrpc_method_desc) {
            .owner = rrdhost_nrpc_owner(host),
            .name = "c6-mixed-fn",
            .help = "mixed",
            .tags = "top",
            .timeout_s = 10,
            .priority = 0,
            .version = 1,
            .access = HTTP_ACCESS_ANONYMOUS_DATA,
            .sync = true,
            .source = NRPC_SOURCE_PLUGIN,
            .handler = c6ut_noop_execute_cb,
            .handler_data = tr,
        });
        if(refcount_references(&tr->lifetime.entry_refcount) != 2) {
            fprintf(stderr, "  FAILED refs: equal-pointer conflict changed entry refs to %d (expected 2 - must net zero)\n",
                    refcount_references(&tr->lifetime.entry_refcount));
            errors++;
        }

        // delete: the stored descriptor released under its stored tag -> 1
        nrpc_method_unregister(rrdhost_nrpc_owner(host), "c6-mixed-fn", NRPC_SOURCE_DAEMON);
        if(refcount_references(&tr->lifetime.entry_refcount) != 1) {
            fprintf(stderr, "  FAILED refs: after del, entry refs %d != 1\n",
                    refcount_references(&tr->lifetime.entry_refcount));
            errors++;
        }

        nrpc_transport_mark_dead_and_drain(tr);
        nrpc_transport_owner_release(tr); // frees it; ASAN verifies no leak

        if(errors == before)
            fprintf(stderr, "  OK refs: mixed-source re-registration and equal-pointer conflict net the right refs\n");
    }

    // 4. GC delete mid-defer + RESULT_END delivers exactly once (the
    //    detach-then-deliver sweep). Production-shaped: the call goes through
    //    the in-flight calls table (nrpc_call), because the GC reads deadlines
    //    exclusively through the in-flight calls table-keyed accessor.
    {
        int before = errors;
        struct c6ut_sends sends = { 0 };
        struct c6ut_result res = { 0 };
        PARSER *parser = c6ut_parser_create(&sends);

        nrpc_inflight_calls_create(); // idempotent

        // the function is served by the pluginsd transport, as in production
        nrpc_method_register(&(struct nrpc_method_desc) {
            .owner = rrdhost_nrpc_owner(host),
            .name = "c6-gc-mid-defer-fn",
            .help = "gc",
            .tags = "top",
            .timeout_s = 1,
            .priority = 0,
            .version = 1,
            .access = HTTP_ACCESS_ANONYMOUS_DATA,
            .sync = false,
            .source = NRPC_SOURCE_PLUGIN,
            .handler = pluginsd_nrpc_handler,
            .handler_data = parser->inflight.transport,
        });

        // our own transaction id, so RESULT_BEGIN can reference it
        nd_uuid_t uuid;
        uuid_generate_random(uuid);
        char tx[UUID_COMPACT_STR_LEN];
        uuid_unparse_lower_compact(uuid, tx);

        CLEAN_BUFFER *wb = buffer_create(0, NULL);
        int code = nrpc_call(&(struct nrpc_call_spec) {
            .owner = rrdhost_nrpc_owner(host),
            .result_wb = wb,
            .cmd = "c6-gc-mid-defer-fn",
            .source = "unittest",
            .user_access = HTTP_ACCESS_ALL,
            .timeout_s = 1,
            .wait = false,
            .allow_restricted = false,
            .call_id = tx,
            .result.cb = c6ut_result_cb,
            .result.data = &res,
        });
        if(code != HTTP_RESP_OK) {
            fprintf(stderr, "  FAILED gc-mid-defer: async run returned %d\n", code);
            errors++;
        }

        c6ut_result_begin(parser, tx, "200");

        // let the in-flight calls table deadline (1s) plus the grace extension (1s) lapse,
        // then run the GC: the defer's reference keeps the parser record
        // alive, so the GC's delete only marks it pending - no delivery yet
        sleep_usec(2200 * USEC_PER_MS);
        pluginsd_calls_garbage_collect(parser, now_monotonic_usec());

        if(res.calls != 0) {
            fprintf(stderr, "  FAILED gc-mid-defer: GC delivered %zu times while the defer holds the record\n", res.calls);
            errors++;
        }
        if(sends.cancels != 1) {
            fprintf(stderr, "  FAILED gc-mid-defer: expected 1 CANCEL to the plugin, got %zu\n", sends.cancels);
            errors++;
        }

        // RESULT_END: release -> del (hashtable miss) -> sweep delivers
        c6ut_defer_complete(parser);

        if(res.calls != 1) {
            fprintf(stderr, "  FAILED gc-mid-defer: result delivered %zu times at RESULT_END, expected exactly once\n", res.calls);
            errors++;
        }
        else if(res.code != HTTP_RESP_GATEWAY_TIMEOUT) {
            fprintf(stderr, "  FAILED gc-mid-defer: got code %d, expected 504 (the GC's timeout)\n", res.code);
            errors++;
        }

        parser_destroy(parser);
        if(res.calls != 1) {
            fprintf(stderr, "  FAILED gc-mid-defer: parser destroy re-delivered (calls %zu)\n", res.calls);
            errors++;
        }

        nrpc_method_unregister(rrdhost_nrpc_owner(host), "c6-gc-mid-defer-fn", NRPC_SOURCE_DAEMON);

        if(errors == before)
            fprintf(stderr, "  OK gc-mid-defer: GC delete mid-defer delivered exactly once, at RESULT_END\n");
    }

    // 5. dyncfg node same-parser re-CREATE (equal cb/data) nets zero pins
    {
        int before = errors;

        // the standalone unittest environment does not initialize dyncfg;
        // do it here (guarded internally, same as dyncfg_unittest does)
        if(!dyncfg_globals.nodes)
            dyncfg_init(false);

        if(!dyncfg_globals.nodes) {
            fprintf(stderr, "  FAILED dyncfg-pin: dyncfg could not be initialized\n");
            errors++;
        }
        else {
            struct nrpc_transport *tr = nrpc_transport_create(NULL);
            // base ref == 1

            // mimics the pluginsd CONFIG path: handler_data IS the transport
            dyncfg_add_low_level(&(struct dyncfg_add_spec) {
                .host = host,
                .id = "c6test:node",
                .path = "/c6test",
                .status = DYNCFG_STATUS_RUNNING,
                .type = DYNCFG_TYPE_SINGLE,
                .source_type = DYNCFG_SOURCE_TYPE_INTERNAL,
                .source = "unittest",
                .cmds = DYNCFG_CMD_SCHEMA | DYNCFG_CMD_GET,
                .sync = false,
                .view_access = HTTP_ACCESS_NONE,
                .edit_access = HTTP_ACCESS_NONE,
                .handler = c6ut_noop_execute_cb,
                .handler_data = tr,
                .transport = tr,
            });

            // node pin (+1) - the registry "config c6test:node" entry is
            // INTERNAL with data NULL and takes no ref
            if(refcount_references(&tr->lifetime.entry_refcount) != 2) {
                fprintf(stderr, "  FAILED dyncfg-pin: after CREATE, entry refs %d != 2\n",
                        refcount_references(&tr->lifetime.entry_refcount));
                errors++;
            }

            // same-parser re-CREATE with equal cb/data: the transfer condition
            // is false, so no pin churn
            dyncfg_add_low_level(&(struct dyncfg_add_spec) {
                .host = host,
                .id = "c6test:node",
                .path = "/c6test",
                .status = DYNCFG_STATUS_RUNNING,
                .type = DYNCFG_TYPE_SINGLE,
                .source_type = DYNCFG_SOURCE_TYPE_INTERNAL,
                .source = "unittest",
                .cmds = DYNCFG_CMD_SCHEMA | DYNCFG_CMD_GET,
                .sync = false,
                .view_access = HTTP_ACCESS_NONE,
                .edit_access = HTTP_ACCESS_NONE,
                .handler = c6ut_noop_execute_cb,
                .handler_data = tr,
                .transport = tr,
            });

            if(refcount_references(&tr->lifetime.entry_refcount) != 2) {
                fprintf(stderr, "  FAILED dyncfg-pin: re-CREATE changed entry refs to %d (expected 2 - must net zero)\n",
                        refcount_references(&tr->lifetime.entry_refcount));
                errors++;
            }

            // a re-CREATE from a DIFFERENT connection (the plugin restarted and
            // its parser - and therefore its transport - is a new one): the
            // conflict transfer branch must release the displaced pin and take
            // one on the installed transport, as ONE swap under the node lock
            struct nrpc_transport *tr2 = nrpc_transport_create(NULL);

            dyncfg_add_low_level(&(struct dyncfg_add_spec) {
                .host = host,
                .id = "c6test:node",
                .path = "/c6test",
                .status = DYNCFG_STATUS_RUNNING,
                .type = DYNCFG_TYPE_SINGLE,
                .source_type = DYNCFG_SOURCE_TYPE_INTERNAL,
                .source = "unittest",
                .cmds = DYNCFG_CMD_SCHEMA | DYNCFG_CMD_GET,
                .sync = false,
                .view_access = HTTP_ACCESS_NONE,
                .edit_access = HTTP_ACCESS_NONE,
                .handler = c6ut_noop_execute_cb,
                .handler_data = tr2,
                .transport = tr2,
            });

            if(refcount_references(&tr->lifetime.entry_refcount) != 1) {
                fprintf(stderr, "  FAILED dyncfg-pin: the displaced transport was not released (entry refs %d != 1)\n",
                        refcount_references(&tr->lifetime.entry_refcount));
                errors++;
            }
            if(refcount_references(&tr2->lifetime.entry_refcount) != 2) {
                fprintf(stderr, "  FAILED dyncfg-pin: the installed transport was not pinned (entry refs %d != 2)\n",
                        refcount_references(&tr2->lifetime.entry_refcount));
                errors++;
            }

            dyncfg_del_low_level(host, "c6test:node");

            if(refcount_references(&tr->lifetime.entry_refcount) != 1 ||
               refcount_references(&tr2->lifetime.entry_refcount) != 1) {
                fprintf(stderr, "  FAILED dyncfg-pin: after DELETE, entry refs %d/%d != 1/1\n",
                        refcount_references(&tr->lifetime.entry_refcount), refcount_references(&tr2->lifetime.entry_refcount));
                errors++;
            }

            nrpc_transport_mark_dead_and_drain(tr);
            nrpc_transport_owner_release(tr);
            nrpc_transport_mark_dead_and_drain(tr2);
            nrpc_transport_owner_release(tr2);
        }

        if(errors == before)
            fprintf(stderr, "  OK dyncfg-pin: CREATE pins once, an equal re-CREATE nets zero, "
                            "a re-CREATE from another connection transfers the pin, DELETE releases\n");
    }

    // 6. concurrent GC passes cancel an expired transaction exactly once.
    //    The dels run after the pass unlocks, so a second pass can re-traverse
    //    while the first pass's victims are still in the hashtable; the
    //    gc_collected mark must keep it from re-sending FUNCTION_CANCEL.
    {
        int before = errors;
        struct c6ut_sends sends = { 0 };
        struct c6ut_result res = { 0 };
        PARSER *parser = c6ut_parser_create(&sends);

        nrpc_inflight_calls_create(); // idempotent

        nrpc_method_register(&(struct nrpc_method_desc) {
            .owner = rrdhost_nrpc_owner(host),
            .name = "c6-gc-race-fn",
            .help = "gc",
            .tags = "top",
            .timeout_s = 1,
            .priority = 0,
            .version = 1,
            .access = HTTP_ACCESS_ANONYMOUS_DATA,
            .sync = false,
            .source = NRPC_SOURCE_PLUGIN,
            .handler = pluginsd_nrpc_handler,
            .handler_data = parser->inflight.transport,
        });

        for(size_t i = 0; i < 50 && errors == before; i++) {
            size_t cancels_before = sends.cancels;
            size_t calls_before = res.calls;

            CLEAN_BUFFER *wb = buffer_create(0, NULL);
            int code = nrpc_call(&(struct nrpc_call_spec) {
                .owner = rrdhost_nrpc_owner(host),
                .result_wb = wb,
                .cmd = "c6-gc-race-fn",
                .source = "unittest",
                .user_access = HTTP_ACCESS_ALL,
                .timeout_s = 1,
                .wait = false,
                .allow_restricted = false,
                .result.cb = c6ut_result_cb,
                .result.data = &res,
            });
            if(code != HTTP_RESP_OK) {
                fprintf(stderr, "  FAILED gc-race: async run returned %d\n", code);
                errors++;
                break;
            }

            // GC takes now as a parameter: pretend the deadline (1s) plus the
            // grace extension (1s) have lapsed, instead of sleeping
            struct c6ut_gc_race g = {
                .parser = parser,
                .now_ut = now_monotonic_usec() + 3 * USEC_PER_SEC,
            };

            ND_THREAD *t1 = nd_thread_create("gc-race-1", NETDATA_THREAD_OPTION_DONT_LOG, c6ut_gc_race_thread, &g);
            ND_THREAD *t2 = nd_thread_create("gc-race-2", NETDATA_THREAD_OPTION_DONT_LOG, c6ut_gc_race_thread, &g);
            if(t1) nd_thread_join(t1);
            if(t2) nd_thread_join(t2);

            if(sends.cancels - cancels_before != 1) {
                fprintf(stderr, "  FAILED gc-race: iteration %zu sent %zu CANCELs, expected exactly 1\n",
                        i, sends.cancels - cancels_before);
                errors++;
            }
            if(res.calls - calls_before != 1) {
                fprintf(stderr, "  FAILED gc-race: iteration %zu delivered %zu times, expected exactly once\n",
                        i, res.calls - calls_before);
                errors++;
            }
            else if(res.code != HTTP_RESP_GATEWAY_TIMEOUT) {
                fprintf(stderr, "  FAILED gc-race: iteration %zu delivered code %d, expected 504\n", i, res.code);
                errors++;
            }
        }

        parser_destroy(parser);
        nrpc_method_unregister(rrdhost_nrpc_owner(host), "c6-gc-race-fn", NRPC_SOURCE_DAEMON);

        if(errors == before)
            fprintf(stderr, "  OK gc-race: concurrent GC passes cancel and deliver exactly once\n");
    }

    // 7. the in-flight calls table-keyed deadline accessor (C5b): visible deadline, visible
    //    extension-on-progress, miss after completion
    {
        int before = errors;

        nrpc_inflight_calls_create(); // idempotent; the standalone env may lack it

        // a SHORT timeout, so a PROGRESS actually extends it (the extension
        // fires only when less than FUNCTIONS_EXTENDED_TIME_ON_PROGRESS_UT
        // remains)
        nrpc_method_register(&(struct nrpc_method_desc) {
            .owner = rrdhost_nrpc_owner(host),
            .name = "c5b-deadline-fn",
            .help = "deadline",
            .tags = "top",
            .timeout_s = 5,
            .priority = 0,
            .version = 1,
            .access = HTTP_ACCESS_ANONYMOUS_DATA,
            .sync = false,
            .source = NRPC_SOURCE_DAEMON,
            .handler = c5b_async_execute_cb,
        });

        CLEAN_BUFFER *wb = buffer_create(0, NULL);
        int code = nrpc_call(&(struct nrpc_call_spec) {
            .owner = rrdhost_nrpc_owner(host),
            .result_wb = wb,
            .cmd = "c5b-deadline-fn",
            .source = "unittest",
            .user_access = HTTP_ACCESS_ALL,
            .timeout_s = 5,
            .wait = false,
            .allow_restricted = false,
        });
        if(code != HTTP_RESP_OK) {
            fprintf(stderr, "  FAILED deadline: async run returned %d\n", code);
            errors++;
        }

        usec_t d1 = 0, d2 = 0, d3 = 0;
        if(!nrpc_call_deadline(c5b_capture.tx, &d1) || !d1) {
            fprintf(stderr, "  FAILED deadline: accessor missed a live transaction\n");
            errors++;
        }

        nrpc_call_progress(c5b_capture.tx);

        if(!nrpc_call_deadline(c5b_capture.tx, &d2)) {
            fprintf(stderr, "  FAILED deadline: accessor missed after progress\n");
            errors++;
        }
        else if(d2 <= d1) {
            fprintf(stderr, "  FAILED deadline: progress did not extend the deadline (%"PRIu64" -> %"PRIu64")\n", d1, d2);
            errors++;
        }

        // finish the call; the in-flight call record must disappear
        c5b_capture.result_cb(c5b_capture.result_wb, HTTP_RESP_OK, c5b_capture.result_cb_data);

        if(nrpc_call_deadline(c5b_capture.tx, &d3)) {
            fprintf(stderr, "  FAILED deadline: accessor still finds a completed transaction\n");
            errors++;
        }

        nrpc_method_unregister(rrdhost_nrpc_owner(host), "c5b-deadline-fn", NRPC_SOURCE_DAEMON);

        if(errors == before)
            fprintf(stderr, "  OK deadline: accessor sees the deadline, the progress extension, and the completion\n");
    }

    // 8. a registry entry whose transport is already dead answers a clean 503.
    //    This is the UAF-C gate: the registry keeps entries of plugins that
    //    died (streaming entries survive disconnect by design), so every
    //    execute has to go through the transport's acquire-or-fail instead of
    //    dereferencing the parser.
    {
        int before = errors;
        struct nrpc_transport *tr = nrpc_transport_create(NULL);

        nrpc_inflight_calls_create(); // idempotent

        nrpc_method_register(&(struct nrpc_method_desc) {
            .owner = rrdhost_nrpc_owner(host),
            .name = "c7-dead-transport-fn",
            .help = "dead",
            .tags = "top",
            .timeout_s = 10,
            .priority = 0,
            .version = 1,
            .access = HTTP_ACCESS_ANONYMOUS_DATA,
            .sync = false,
            .source = NRPC_SOURCE_PLUGIN,
            .handler = pluginsd_nrpc_handler,
            .handler_data = tr,
        });

        // the owner tears down: dispatchers drained and `data` invalidated,
        // while the registry entry keeps holding the (dead) transport
        nrpc_transport_mark_dead_and_drain(tr);

        struct c6ut_result res = { 0 };
        CLEAN_BUFFER *wb = buffer_create(0, NULL);
        int code = nrpc_call(&(struct nrpc_call_spec) {
            .owner = rrdhost_nrpc_owner(host),
            .result_wb = wb,
            .cmd = "c7-dead-transport-fn",
            .source = "unittest",
            .user_access = HTTP_ACCESS_ALL,
            .timeout_s = 10,
            .wait = false,
            .allow_restricted = false,
            .result.cb = c6ut_result_cb,
            .result.data = &res,
        });

        if(code != HTTP_RESP_SERVICE_UNAVAILABLE) {
            fprintf(stderr, "  FAILED dead-transport: run returned %d, expected 503\n", code);
            errors++;
        }
        if(res.calls != 1 || res.code != HTTP_RESP_SERVICE_UNAVAILABLE) {
            fprintf(stderr, "  FAILED dead-transport: delivered %zu times with code %d, expected exactly one 503\n",
                    res.calls, res.code);
            errors++;
        }
        if(!buffer_strlen(wb)) {
            fprintf(stderr, "  FAILED dead-transport: the 503 has an empty body\n");
            errors++;
        }

        nrpc_method_unregister(rrdhost_nrpc_owner(host), "c7-dead-transport-fn", NRPC_SOURCE_DAEMON);
        nrpc_transport_owner_release(tr); // frees it; ASAN verifies the accounting

        if(errors == before)
            fprintf(stderr, "  OK dead-transport: an entry of a dead plugin answers exactly one clean 503\n");
    }

    // 9. cancel and progress hooks are KEYED, not pointer-identity matched
    //    (UAF-A): a request carrying one plugin's transport and another
    //    plugin's transaction must reach neither plugin. Before the refactor
    //    this was a scan over recyclable records, where a recycled record
    //    could false-match and cancel the wrong transaction.
    {
        int before = errors;
        struct c6ut_sends sends_a = { 0 }, sends_b = { 0 };
        struct c6ut_result res_a = { 0 }, res_b = { 0 };
        PARSER *pa = c6ut_parser_create(&sends_a);
        PARSER *pb = c6ut_parser_create(&sends_b);

        usec_t stop_ut = now_monotonic_usec() + 60 * USEC_PER_SEC;
        CLEAN_BUFFER *wba = buffer_create(0, NULL);
        CLEAN_BUFFER *wbb = buffer_create(0, NULL);
        char txa[UUID_COMPACT_STR_LEN], txb[UUID_COMPACT_STR_LEN];
        struct c6ut_cancel_hook can_a = { 0 };
        struct c6ut_progress_hook prg_a = { 0 };

        nd_uuid_t ua, ub;
        uuid_generate_random(ua); uuid_unparse_lower_compact(ua, txa);
        uuid_generate_random(ub); uuid_unparse_lower_compact(ub, txb);

        c6ut_execute_tx(pa, "c7-keyed-a", &ua, &stop_ut, wba, &res_a, &can_a, &prg_a);
        c6ut_execute_tx(pb, "c7-keyed-b", &ub, &stop_ut, wbb, &res_b, NULL, NULL);

        if(!can_a.cb || !prg_a.cb) {
            fprintf(stderr, "  FAILED keyed: no cancel_hook/progress_hook was registered\n");
            errors++;
        }
        else {
            // A's transport + B's transaction: A misses in its own dictionary,
            // and B is never reached at all
            can_a.cb(txb, can_a.transport);
            prg_a.cb(txb, prg_a.transport);
            if(sends_a.cancels || sends_a.progresses || sends_b.cancels || sends_b.progresses) {
                fprintf(stderr, "  FAILED keyed: a foreign transaction reached a plugin "
                                "(a: %zu cancels %zu progresses, b: %zu cancels %zu progresses)\n",
                        sends_a.cancels, sends_a.progresses, sends_b.cancels, sends_b.progresses);
                errors++;
            }

            // the matching pair reaches exactly one plugin, exactly once
            can_a.cb(txa, can_a.transport);
            prg_a.cb(txa, prg_a.transport);
            if(sends_a.cancels != 1 || sends_a.progresses != 1) {
                fprintf(stderr, "  FAILED keyed: the owning plugin got %zu cancels and %zu progresses, expected 1 and 1\n",
                        sends_a.cancels, sends_a.progresses);
                errors++;
            }
            if(sends_b.cancels || sends_b.progresses) {
                fprintf(stderr, "  FAILED keyed: the other plugin was disturbed\n");
                errors++;
            }

            // an empty transaction is refused before anything is sent
            can_a.cb("", can_a.transport);
            prg_a.cb(NULL, prg_a.transport);
            if(sends_a.cancels != 1 || sends_a.progresses != 1) {
                fprintf(stderr, "  FAILED keyed: an empty transaction produced a send\n");
                errors++;
            }
        }

        parser_destroy(pa);
        parser_destroy(pb);
        nrpc_transport_entry_release(can_a.transport);
        nrpc_transport_entry_release(prg_a.transport);

        if(errors == before)
            fprintf(stderr, "  OK keyed: cancel/progress reach only the plugin that owns the transaction\n");
    }

    // 10. duplicate transaction ids, on both sides of the transport.
    //     The in-flight calls table rejects the second caller with 400 and must leave the
    //     first (still running) transaction completely untouched; the parser's
    //     own conflict callback does the same for its dictionary, and frees
    //     the loser's payload/source/function (ASAN checks that half).
    {
        int before = errors;

        nrpc_inflight_calls_create(); // idempotent

        nrpc_method_register(&(struct nrpc_method_desc) {
            .owner = rrdhost_nrpc_owner(host),
            .name = "c7-dup-fn",
            .help = "dup",
            .tags = "top",
            .timeout_s = 10,
            .priority = 0,
            .version = 1,
            .access = HTTP_ACCESS_ANONYMOUS_DATA,
            .sync = false,
            .source = NRPC_SOURCE_DAEMON,
            .handler = c5b_async_execute_cb,
        });

        nd_uuid_t uuid;
        uuid_generate_random(uuid);
        char tx[UUID_COMPACT_STR_LEN];
        uuid_unparse_lower_compact(uuid, tx);

        struct c6ut_result r1 = { 0 }, r2 = { 0 };
        CLEAN_BUFFER *wb1 = buffer_create(0, NULL);
        CLEAN_BUFFER *wb2 = buffer_create(0, NULL);

        int c1 = nrpc_call(&(struct nrpc_call_spec) {
            .owner = rrdhost_nrpc_owner(host),
            .result_wb = wb1,
            .cmd = "c7-dup-fn",
            .source = "unittest",
            .user_access = HTTP_ACCESS_ALL,
            .timeout_s = 10,
            .wait = false,
            .allow_restricted = false,
            .call_id = tx,
            .result.cb = c6ut_result_cb,
            .result.data = &r1,
        });
        int c2 = nrpc_call(&(struct nrpc_call_spec) {
            .owner = rrdhost_nrpc_owner(host),
            .result_wb = wb2,
            .cmd = "c7-dup-fn",
            .source = "unittest",
            .user_access = HTTP_ACCESS_ALL,
            .timeout_s = 10,
            .wait = false,
            .allow_restricted = false,
            .call_id = tx,
            .result.cb = c6ut_result_cb,
            .result.data = &r2,
        });

        if(c1 != HTTP_RESP_OK || r1.calls != 0) {
            fprintf(stderr, "  FAILED duplicate: the first call returned %d and was delivered %zu times\n",
                    c1, r1.calls);
            errors++;
        }
        if(c2 != HTTP_RESP_BAD_REQUEST || r2.calls != 1 || r2.code != HTTP_RESP_BAD_REQUEST) {
            fprintf(stderr, "  FAILED duplicate: the second call returned %d and was delivered %zu times with code %d,"
                            " expected one 400\n", c2, r2.calls, r2.code);
            errors++;
        }

        // the rejection must not have touched the transaction already in flight
        usec_t d = 0;
        if(!nrpc_call_deadline(tx, &d)) {
            fprintf(stderr, "  FAILED duplicate: the rejection removed the running transaction from the in-flight calls table\n");
            errors++;
        }

        // finish the first call
        c5b_capture.result_cb(c5b_capture.result_wb, HTTP_RESP_OK, c5b_capture.result_cb_data);

        if(r1.calls != 1 || r1.code != HTTP_RESP_OK) {
            fprintf(stderr, "  FAILED duplicate: the first call finished with %zu deliveries, code %d\n",
                    r1.calls, r1.code);
            errors++;
        }
        if(nrpc_call_deadline(tx, &d)) {
            fprintf(stderr, "  FAILED duplicate: the completed transaction is still in the in-flight calls table\n");
            errors++;
        }

        nrpc_method_unregister(rrdhost_nrpc_owner(host), "c7-dup-fn", NRPC_SOURCE_DAEMON);

        // the same collision inside the parser's own dictionary
        {
            struct c6ut_sends sends = { 0 };
            struct c6ut_result p1 = { 0 }, p2 = { 0 };
            PARSER *parser = c6ut_parser_create(&sends);
            usec_t stop_ut = now_monotonic_usec() + 60 * USEC_PER_SEC;
            CLEAN_BUFFER *pwb1 = buffer_create(0, NULL);
            CLEAN_BUFFER *pwb2 = buffer_create(0, NULL);

            nd_uuid_t pu;
            uuid_generate_random(pu);

            c6ut_execute_tx(parser, "c7-dup-parser-fn", &pu, &stop_ut, pwb1, &p1, NULL, NULL);
            c6ut_execute_tx(parser, "c7-dup-parser-fn", &pu, &stop_ut, pwb2, &p2, NULL, NULL);

            if(p2.calls != 1 || p2.code != HTTP_RESP_BAD_REQUEST) {
                fprintf(stderr, "  FAILED duplicate: the parser's duplicate was delivered %zu times with code %d,"
                                " expected one 400\n", p2.calls, p2.code);
                errors++;
            }
            if(p1.calls != 0) {
                fprintf(stderr, "  FAILED duplicate: the parser's first record was delivered by the collision\n");
                errors++;
            }

            parser_destroy(parser);

            if(p1.calls != 1 || p2.calls != 1) {
                fprintf(stderr, "  FAILED duplicate: after destroy, deliveries are %zu/%zu, expected 1/1\n",
                        p1.calls, p2.calls);
                errors++;
            }
        }

        if(errors == before)
            fprintf(stderr, "  OK duplicate: both the in-flight calls table and the parser reject a duplicate without disturbing the original\n");
    }

    // 11. a request that cannot even be written to the plugin is answered
    //     immediately, exactly once, and leaves no record behind
    {
        int before = errors;
        struct c6ut_sends sends = { .fail = true };
        struct c6ut_result res = { 0 };
        PARSER *parser = c6ut_parser_create(&sends);

        usec_t stop_ut = now_monotonic_usec() + 60 * USEC_PER_SEC;
        CLEAN_BUFFER *wb = buffer_create(0, NULL);
        char tx[UUID_COMPACT_STR_LEN];

        int code = c6ut_execute(parser, "c7-sendfail-fn", &stop_ut, wb, &res, tx, NULL);

        if(code != HTTP_RESP_SERVICE_UNAVAILABLE) {
            fprintf(stderr, "  FAILED send-fail: execute returned %d, expected 503\n", code);
            errors++;
        }
        if(res.calls != 1 || res.code != HTTP_RESP_SERVICE_UNAVAILABLE) {
            fprintf(stderr, "  FAILED send-fail: delivered %zu times with code %d, expected exactly one 503\n",
                    res.calls, res.code);
            errors++;
        }
        if(dictionary_entries(parser->inflight.calls) != 0) {
            fprintf(stderr, "  FAILED send-fail: the record survived a failed send\n");
            errors++;
        }

        parser_destroy(parser);

        if(res.calls != 1) {
            fprintf(stderr, "  FAILED send-fail: destroy re-delivered the failed request (calls %zu)\n", res.calls);
            errors++;
        }

        if(errors == before)
            fprintf(stderr, "  OK send-fail: an unsendable request is answered once and leaves nothing behind\n");
    }

    // 12. GC invariant: a parser record with no in-flight call record is SKIPPED, never
    //     reaped - reaping would run the delete-callback chain into memory
    //     whose invariant already failed. The all-skipped guard must also push
    //     the next GC attempt one extension away, or every later submission
    //     re-runs the GC and re-logs the misses.
    {
        int before = errors;
        struct c6ut_sends sends = { 0 };
        struct c6ut_result res = { 0 };
        PARSER *parser = c6ut_parser_create(&sends);

        nrpc_inflight_calls_create(); // idempotent - the record we make has no entry in it

        // already past its deadline, so the GC would reap it if it could read one
        usec_t stop_ut = now_monotonic_usec() - 10 * USEC_PER_SEC;
        CLEAN_BUFFER *wb = buffer_create(0, NULL);
        char tx[UUID_COMPACT_STR_LEN];

        c6ut_execute(parser, "c7-orphan-fn", &stop_ut, wb, &res, tx, NULL);

        // handler itself runs the GC when the deadline is already behind us
        if(res.calls) {
            fprintf(stderr, "  FAILED gc-skip: a record without a in-flight calls table entry was reaped at submission\n");
            errors++;
        }
        if(parser->inflight.smaller_monotonic_timeout_ut <= nrpc_effective_deadline_ut(stop_ut)) {
            fprintf(stderr, "  FAILED gc-skip: the all-skipped guard did not push the next attempt forward\n");
            errors++;
        }

        pluginsd_calls_garbage_collect(parser, now_monotonic_usec() + 3600 * USEC_PER_SEC);

        if(res.calls) {
            fprintf(stderr, "  FAILED gc-skip: an explicit GC pass reaped the record anyway (calls %zu)\n", res.calls);
            errors++;
        }
        if(sends.cancels) {
            fprintf(stderr, "  FAILED gc-skip: a CANCEL was sent for a record the GC must not touch\n");
            errors++;
        }
        if(dictionary_entries(parser->inflight.calls) != 1) {
            fprintf(stderr, "  FAILED gc-skip: the skipped record left the dictionary\n");
            errors++;
        }

        // the plugin's death is what finally answers it
        parser_destroy(parser);

        if(res.calls != 1 || res.code != HTTP_RESP_SERVICE_UNAVAILABLE) {
            fprintf(stderr, "  FAILED gc-skip: destroy delivered %zu times with code %d, expected one 503\n",
                    res.calls, res.code);
            errors++;
        }

        if(errors == before)
            fprintf(stderr, "  OK gc-skip: a record with no in-flight calls table entry is skipped, never reaped, and answered at destroy\n");
    }

    // 13. the grace extension is real: every deadline gets
    //     NRPC_DEADLINE_GRACE_UT before the GC enforces it, and the
    //     GC applies it through nrpc_effective_deadline_ut() only - so
    //     a transaction inside the grace window is never cancelled early.
    {
        int before = errors;
        struct c6ut_sends sends = { 0 };
        struct c6ut_result res = { 0 };
        PARSER *parser = c6ut_parser_create(&sends);

        nrpc_inflight_calls_create(); // idempotent

        nrpc_method_register(&(struct nrpc_method_desc) {
            .owner = rrdhost_nrpc_owner(host),
            .name = "c7-grace-fn",
            .help = "grace",
            .tags = "top",
            .timeout_s = 10,
            .priority = 0,
            .version = 1,
            .access = HTTP_ACCESS_ANONYMOUS_DATA,
            .sync = false,
            .source = NRPC_SOURCE_PLUGIN,
            .handler = pluginsd_nrpc_handler,
            .handler_data = parser->inflight.transport,
        });

        CLEAN_BUFFER *wb = buffer_create(0, NULL);
        usec_t t0 = now_monotonic_usec();
        int code = nrpc_call(&(struct nrpc_call_spec) {
            .owner = rrdhost_nrpc_owner(host),
            .result_wb = wb,
            .cmd = "c7-grace-fn",
            .source = "unittest",
            .user_access = HTTP_ACCESS_ALL,
            .timeout_s = 10,
            .wait = false,
            .allow_restricted = false,
            .result.cb = c6ut_result_cb,
            .result.data = &res,
        });
        if(code != HTTP_RESP_OK) {
            fprintf(stderr, "  FAILED grace: async run returned %d\n", code);
            errors++;
        }

        // 100ms before the deadline+grace: nothing may happen
        pluginsd_calls_garbage_collect(parser, t0 + 10 * USEC_PER_SEC + 900 * USEC_PER_MS);
        if(sends.cancels || res.calls) {
            fprintf(stderr, "  FAILED grace: a transaction inside the grace window was cancelled"
                            " (%zu cancels, %zu deliveries)\n", sends.cancels, res.calls);
            errors++;
        }

        // 500ms past it: cancelled once and timed out
        pluginsd_calls_garbage_collect(parser, t0 + 11 * USEC_PER_SEC + 500 * USEC_PER_MS);
        if(sends.cancels != 1) {
            fprintf(stderr, "  FAILED grace: %zu CANCELs past the grace window, expected 1\n", sends.cancels);
            errors++;
        }
        if(res.calls != 1 || res.code != HTTP_RESP_GATEWAY_TIMEOUT) {
            fprintf(stderr, "  FAILED grace: delivered %zu times with code %d, expected one 504\n",
                    res.calls, res.code);
            errors++;
        }

        parser_destroy(parser);
        nrpc_method_unregister(rrdhost_nrpc_owner(host), "c7-grace-fn", NRPC_SOURCE_DAEMON);

        if(errors == before)
            fprintf(stderr, "  OK grace: the deadline is enforced one extension late, and exactly once\n");
    }

    // 14. the in-flight calls table's registration pins and the cancel protocol:
    //     a cancel_hook/progress_hook registration entry-pins its transport for the
    //     record's lifetime (so a late cancel finds a valid, dead transport
    //     instead of freed memory), a re-registration releases the previous
    //     pin, completion releases both, and a second CANCEL for the same
    //     transaction is dropped by the `cancelled` flag.
    {
        int before = errors;

        nrpc_inflight_calls_create(); // idempotent

        memset(&c7_pin, 0, sizeof(c7_pin));
        c7_pin.first = nrpc_transport_create(NULL);
        c7_pin.second = nrpc_transport_create(NULL);

        nrpc_method_register(&(struct nrpc_method_desc) {
            .owner = rrdhost_nrpc_owner(host),
            .name = "c7-pin-fn",
            .help = "pin",
            .tags = "top",
            .timeout_s = 10,
            .priority = 0,
            .version = 1,
            .access = HTTP_ACCESS_ANONYMOUS_DATA,
            .sync = false,
            .source = NRPC_SOURCE_DAEMON,
            .handler = c7_pin_execute_cb,
        });

        struct c6ut_result res = { 0 };
        CLEAN_BUFFER *wb = buffer_create(0, NULL);
        int code = nrpc_call(&(struct nrpc_call_spec) {
            .owner = rrdhost_nrpc_owner(host),
            .result_wb = wb,
            .cmd = "c7-pin-fn",
            .source = "unittest",
            .user_access = HTTP_ACCESS_ALL,
            .timeout_s = 10,
            .wait = false,
            .allow_restricted = false,
            .result.cb = c6ut_result_cb,
            .result.data = &res,
        });
        if(code != HTTP_RESP_OK) {
            fprintf(stderr, "  FAILED pins: async run returned %d\n", code);
            errors++;
        }

        // first: base ref only (the cancel_hook re-registration released its pin)
        if(refcount_references(&c7_pin.first->lifetime.entry_refcount) != 1) {
            fprintf(stderr, "  FAILED pins: a cancel_hook re-registration leaked the previous pin (refs %d != 1)\n",
                    refcount_references(&c7_pin.first->lifetime.entry_refcount));
            errors++;
        }
        // second: base + the cancel_hook pin + the progress_hook pin
        if(refcount_references(&c7_pin.second->lifetime.entry_refcount) != 3) {
            fprintf(stderr, "  FAILED pins: the cancel_hook/progress_hook pins are %d, expected 3 (base + 2)\n",
                    refcount_references(&c7_pin.second->lifetime.entry_refcount));
            errors++;
        }

        nrpc_call_cancel(c7_pin.tx);
        if(c7_pin.cancels != 1) {
            fprintf(stderr, "  FAILED pins: CANCEL dispatched %zu times, expected 1\n", c7_pin.cancels);
            errors++;
        }

        // the `cancelled` flag makes a repeat a no-op
        nrpc_call_cancel(c7_pin.tx);
        if(c7_pin.cancels != 1) {
            fprintf(stderr, "  FAILED pins: a repeated CANCEL was dispatched again (%zu)\n", c7_pin.cancels);
            errors++;
        }

        // an unknown transaction is a no-op, not a crash
        nrpc_call_cancel("0123456789abcdef0123456789abcdef");

        nrpc_call_progress(c7_pin.tx);
        if(c7_pin.progresses != 1) {
            fprintf(stderr, "  FAILED pins: PROGRESS dispatched %zu times, expected 1\n", c7_pin.progresses);
            errors++;
        }

        // completion tears the record down and releases both pins
        c7_pin.result_cb(c7_pin.result_wb, HTTP_RESP_OK, c7_pin.result_cb_data);

        if(res.calls != 1) {
            fprintf(stderr, "  FAILED pins: the caller was delivered %zu times, expected once\n", res.calls);
            errors++;
        }
        if(refcount_references(&c7_pin.second->lifetime.entry_refcount) != 1 ||
           refcount_references(&c7_pin.first->lifetime.entry_refcount) != 1) {
            fprintf(stderr, "  FAILED pins: after completion the pins are %d/%d, expected 1/1\n",
                    refcount_references(&c7_pin.first->lifetime.entry_refcount),
                    refcount_references(&c7_pin.second->lifetime.entry_refcount));
            errors++;
        }

        nrpc_method_unregister(rrdhost_nrpc_owner(host), "c7-pin-fn", NRPC_SOURCE_DAEMON);
        nrpc_transport_mark_dead_and_drain(c7_pin.first);
        nrpc_transport_owner_release(c7_pin.first);
        nrpc_transport_mark_dead_and_drain(c7_pin.second);
        nrpc_transport_owner_release(c7_pin.second);

        if(errors == before)
            fprintf(stderr, "  OK pins: registration pins are taken, re-taken and released; CANCEL is idempotent\n");
    }

    // 15. the deadline the in-flight calls table records, and the sync shortcut.
    //     A caller that gives no timeout gets the one the function was
    //     registered with; a sync function is executed inline, is visible in
    //     the in-flight calls table only for the duration of the call, and is offered no
    //     cancel_hook/progress_hook (there is nothing to cancel after it returns).
    {
        int before = errors;

        nrpc_inflight_calls_create(); // idempotent

        nrpc_method_register(&(struct nrpc_method_desc) {
            .owner = rrdhost_nrpc_owner(host),
            .name = "c7-timeout-fn",
            .help = "timeout",
            .tags = "top",
            .timeout_s = 7,
            .priority = 0,
            .version = 1,
            .access = HTTP_ACCESS_ANONYMOUS_DATA,
            .sync = false,
            .source = NRPC_SOURCE_DAEMON,
            .handler = c5b_async_execute_cb,
        });

        struct { int ask; usec_t expect_s; } t[] = {
            { 0,  7 },   // no timeout given -> the registered one
            { -1, 7 },   // a negative timeout is the same "not given"
            { 3,  3 },   // an explicit timeout wins
        };

        for(size_t i = 0; i < _countof(t); i++) {
            CLEAN_BUFFER *wb = buffer_create(0, NULL);
            usec_t t0 = now_monotonic_usec();
            int code = nrpc_call(&(struct nrpc_call_spec) {
                .owner = rrdhost_nrpc_owner(host),
                .result_wb = wb,
                .cmd = "c7-timeout-fn",
                .source = "unittest",
                .user_access = HTTP_ACCESS_ALL,
                .timeout_s = t[i].ask,
                .wait = false,
                .allow_restricted = false,
            });
            if(code != HTTP_RESP_OK) {
                fprintf(stderr, "  FAILED timeout: run returned %d\n", code);
                errors++;
                continue;
            }

            usec_t d = 0;
            if(!nrpc_call_deadline(c5b_capture.tx, &d)) {
                fprintf(stderr, "  FAILED timeout: the transaction is not in the in-flight calls table\n");
                errors++;
            }
            else if(d < t0 + t[i].expect_s * USEC_PER_SEC || d >= t0 + (t[i].expect_s + 1) * USEC_PER_SEC) {
                fprintf(stderr, "  FAILED timeout: asked for %d, expected a ~%"PRIu64"s deadline, got %"PRIu64" usec away\n",
                        t[i].ask, t[i].expect_s, d - t0);
                errors++;
            }

            c5b_capture.result_cb(c5b_capture.result_wb, HTTP_RESP_OK, c5b_capture.result_cb_data);
        }

        nrpc_method_unregister(rrdhost_nrpc_owner(host), "c7-timeout-fn", NRPC_SOURCE_DAEMON);

        memset(&c7_sync, 0, sizeof(c7_sync));
        nrpc_method_register(&(struct nrpc_method_desc) {
            .owner = rrdhost_nrpc_owner(host),
            .name = "c7-sync-fn",
            .help = "sync",
            .tags = "top",
            .timeout_s = 10,
            .priority = 0,
            .version = 1,
            .access = HTTP_ACCESS_ANONYMOUS_DATA,
            .sync = true,
            .source = NRPC_SOURCE_DAEMON,
            .handler = c7_sync_execute_cb,
        });

        struct c6ut_result sres = { 0 };
        {
            CLEAN_BUFFER *wb = buffer_create(0, NULL);
            int code = nrpc_call(&(struct nrpc_call_spec) {
                .owner = rrdhost_nrpc_owner(host),
                .result_wb = wb,
                .cmd = "c7-sync-fn",
                .source = "unittest",
                .user_access = HTTP_ACCESS_ALL,
                .timeout_s = 10,
                .wait = true,
                .allow_restricted = false,
                .result.cb = c6ut_result_cb,
                .result.data = &sres,
            });
            if(code != HTTP_RESP_OK) {
                fprintf(stderr, "  FAILED sync: run returned %d\n", code);
                errors++;
            }
        }

        if(sres.calls != 1) {
            fprintf(stderr, "  FAILED sync: delivered %zu times, expected once\n", sres.calls);
            errors++;
        }
        if(!c7_sync.deadline_visible) {
            fprintf(stderr, "  FAILED sync: the transaction was not in the in-flight calls table during the call\n");
            errors++;
        }
        if(c7_sync.has_cancel_hook || c7_sync.has_progress_hook) {
            fprintf(stderr, "  FAILED sync: a sync call was offered a cancel_hook/progress_hook registration\n");
            errors++;
        }
        usec_t d = 0;
        if(nrpc_call_deadline(c7_sync.tx, &d)) {
            fprintf(stderr, "  FAILED sync: the transaction survived the call\n");
            errors++;
        }

        nrpc_method_unregister(rrdhost_nrpc_owner(host), "c7-sync-fn", NRPC_SOURCE_DAEMON);

        if(errors == before)
            fprintf(stderr, "  OK deadline/sync: the registered timeout is the fallback; a sync call leaves no transaction\n");
    }

    fprintf(stderr, "%s() %s (%d error%s)\n\n",
            __FUNCTION__, errors ? "FAILED" : "passed", errors, errors == 1 ? "" : "s");

    return errors;
}
