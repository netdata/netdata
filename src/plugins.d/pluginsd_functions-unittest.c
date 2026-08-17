// SPDX-License-Identifier: GPL-3.0-or-later

#include "pluginsd_functions.h"
#include "database/rrdfunctions-internals.h"
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

// mimics the broker's registration callbacks: stores the (cb, data) pair and
// entry-pins the transport, per the rrd_function_cancel_cb_t contract
struct c6ut_canceller {
    rrd_function_cancel_cb_t cb;
    struct rrd_function_transport *transport; // entry-pinned
};

static void c6ut_register_canceller_cb(void *register_cancel_cb_data, rrd_function_cancel_cb_t cancel_cb, void *cancel_cb_data) {
    struct c6ut_canceller *c = register_cancel_cb_data;
    c->cb = cancel_cb;
    c->transport = rrd_function_transport_entry_acquire(cancel_cb_data);
}

static void c6ut_result_cb(BUFFER *wb __maybe_unused, int code, void *data) {
    struct c6ut_result *r = data;
    r->calls++;
    r->code = code;
}

struct c6ut_sends {
    size_t total;
    size_t cancels;
};

static ssize_t c6ut_send_cb(const char *txt, void *data, STREAM_TRAFFIC_TYPE type __maybe_unused) {
    struct c6ut_sends *s = data;
    s->total++;
    if(strstr(txt, PLUGINSD_CALL_FUNCTION_CANCEL))
        s->cancels++;
    return (ssize_t)strlen(txt);
}

static int c6ut_noop_execute_cb(struct rrd_function_execute *rfe __maybe_unused, void *data __maybe_unused) {
    return HTTP_RESP_OK;
}

// an async function that never completes on its own: captures the transaction
// and the broker's result callback so the test can finish it explicitly
static struct {
    char tx[UUID_COMPACT_STR_LEN];
    BUFFER *result_wb;
    rrd_function_result_callback_t result_cb;
    void *result_cb_data;
} c5b_capture;

static int c5b_async_execute_cb(struct rrd_function_execute *rfe, void *data __maybe_unused) {
    uuid_unparse_lower_compact(*rfe->transaction, c5b_capture.tx);
    c5b_capture.result_wb = rfe->result.wb;
    c5b_capture.result_cb = rfe->result.cb;
    c5b_capture.result_cb_data = rfe->result.data;
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
    pluginsd_inflight_functions_garbage_collect(g->parser, g->now_ut);
}

static PARSER *c6ut_parser_create(struct c6ut_sends *sends) {
    struct parser_user_object user = { 0 };
    PARSER *parser = parser_init(&user, -1, -1, PARSER_INPUT_SPLIT, NULL);
    parser->send_to_plugin_cb = c6ut_send_cb;
    parser->send_to_plugin_data = sends;
    pluginsd_keywords_init(parser, PARSER_INIT_PLUGINSD);
    return parser;
}

// drive one function call into the parser's transport, returning the compact
// transaction id in tx
static int c6ut_execute(PARSER *parser, const char *fn, usec_t *stop_ut,
                        BUFFER *wb, struct c6ut_result *res,
                        char tx[UUID_COMPACT_STR_LEN],
                        struct c6ut_canceller *canceller) {
    nd_uuid_t uuid;
    uuid_generate_random(uuid);
    uuid_unparse_lower_compact(uuid, tx);

    struct rrd_function_execute rfe = {
        .transaction = &uuid,
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
        .register_canceller = {
            .cb = canceller ? c6ut_register_canceller_cb : NULL,
            .data = canceller,
        },
    };

    return pluginsd_function_execute_cb(&rfe, parser->inflight.transport);
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
    if(!host || !host->functions) {
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

        struct c6ut_canceller canceller = { 0 };
        int code = c6ut_execute(parser, "c6-truncated-fn", &stop_ut, wb, &res, tx, &canceller);
        if(code != HTTP_RESP_OK) {
            fprintf(stderr, "  FAILED truncation: execute returned %d\n", code);
            errors++;
        }

        c6ut_result_begin(parser, tx, "200"); // the plugin claims success...
        if(!parser->defer.item) {
            fprintf(stderr, "  FAILED truncation: RESULT_BEGIN did not stash the acquired item\n");
            errors++;
        }

        // the keyed canceller, while the plugin lives: O(1) lookup + CANCEL send
        if(!canceller.cb || !canceller.transport) {
            fprintf(stderr, "  FAILED truncation: no canceller was registered\n");
            errors++;
        }
        else {
            size_t cancels_before = sends.cancels;
            canceller.cb(tx, canceller.transport);
            if(sends.cancels != cancels_before + 1) {
                fprintf(stderr, "  FAILED truncation: live cancel did not send a CANCEL to the plugin\n");
                errors++;
            }
        }

        parser_destroy(parser); // ...but dies before RESULT_END

        // the keyed canceller AFTER death: the dispatcher acquire fails on the
        // pinned (dead) transport - no send, no crash
        if(canceller.cb) {
            size_t cancels_before = sends.cancels;
            canceller.cb(tx, canceller.transport);
            if(sends.cancels != cancels_before) {
                fprintf(stderr, "  FAILED truncation: cancel after parser death still sent to the plugin\n");
                errors++;
            }
        }
        rrd_function_transport_entry_release(canceller.transport);

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
        struct rrd_function_transport *tr = rrd_function_transport_create(NULL);
        // base ref == 1

        // COLLECTOR with transport: insert acquires -> 2
        rrd_function_add(host, NULL, "c6-mixed-fn", 10, 0, 1, "mixed", "top", HTTP_ACCESS_ANONYMOUS_DATA,
                         true, RRD_FUNCTION_REG_SOURCE_COLLECTOR, c6ut_noop_execute_cb, tr);
        if(refcount_references(&tr->entry_refcount) != 2) {
            fprintf(stderr, "  FAILED refs: after COLLECTOR add, entry refs %d != 2\n",
                    refcount_references(&tr->entry_refcount));
            errors++;
        }

        // INTERNAL displaces it: displaced pair released under COLLECTOR tag -> 1
        rrd_function_add(host, NULL, "c6-mixed-fn", 10, 0, 1, "mixed", "top", HTTP_ACCESS_ANONYMOUS_DATA,
                         true, RRD_FUNCTION_REG_SOURCE_INTERNAL, c6ut_noop_execute_cb, NULL);
        if(refcount_references(&tr->entry_refcount) != 1) {
            fprintf(stderr, "  FAILED refs: after INTERNAL re-add, entry refs %d != 1\n",
                    refcount_references(&tr->entry_refcount));
            errors++;
        }

        // and back to COLLECTOR: installed pair acquired -> 2
        rrd_function_add(host, NULL, "c6-mixed-fn", 10, 0, 1, "mixed", "top", HTTP_ACCESS_ANONYMOUS_DATA,
                         true, RRD_FUNCTION_REG_SOURCE_COLLECTOR, c6ut_noop_execute_cb, tr);
        if(refcount_references(&tr->entry_refcount) != 2) {
            fprintf(stderr, "  FAILED refs: after COLLECTOR re-add, entry refs %d != 2\n",
                    refcount_references(&tr->entry_refcount));
            errors++;
        }

        // equal-pointer conflict (a re-sent function list): nets ZERO refs
        rrd_function_add(host, NULL, "c6-mixed-fn", 10, 0, 1, "mixed", "top", HTTP_ACCESS_ANONYMOUS_DATA,
                         true, RRD_FUNCTION_REG_SOURCE_COLLECTOR, c6ut_noop_execute_cb, tr);
        if(refcount_references(&tr->entry_refcount) != 2) {
            fprintf(stderr, "  FAILED refs: equal-pointer conflict changed entry refs to %d (expected 2 - must net zero)\n",
                    refcount_references(&tr->entry_refcount));
            errors++;
        }

        // delete: the stored pair released under its stored tag -> 1
        rrd_function_del(host, NULL, "c6-mixed-fn", RRD_FUNCTION_REG_SOURCE_INTERNAL);
        if(refcount_references(&tr->entry_refcount) != 1) {
            fprintf(stderr, "  FAILED refs: after del, entry refs %d != 1\n",
                    refcount_references(&tr->entry_refcount));
            errors++;
        }

        rrd_function_transport_mark_dead_and_drain(tr);
        rrd_function_transport_owner_release(tr); // frees it; ASAN verifies no leak

        if(errors == before)
            fprintf(stderr, "  OK refs: mixed-source re-registration and equal-pointer conflict net the right refs\n");
    }

    // 4. GC delete mid-defer + RESULT_END delivers exactly once (the
    //    detach-then-deliver sweep). Production-shaped: the call goes through
    //    the broker (rrd_function_run), because the GC reads deadlines
    //    exclusively through the broker-keyed accessor.
    {
        int before = errors;
        struct c6ut_sends sends = { 0 };
        struct c6ut_result res = { 0 };
        PARSER *parser = c6ut_parser_create(&sends);

        rrd_function_transactions_create(); // idempotent

        // the function is served by the pluginsd transport, as in production
        rrd_function_add(host, NULL, "c6-gc-mid-defer-fn", 1, 0, 1, "gc", "top",
                         HTTP_ACCESS_ANONYMOUS_DATA, false /* async */, RRD_FUNCTION_REG_SOURCE_COLLECTOR,
                         pluginsd_function_execute_cb, parser->inflight.transport);

        // our own transaction id, so RESULT_BEGIN can reference it
        nd_uuid_t uuid;
        uuid_generate_random(uuid);
        char tx[UUID_COMPACT_STR_LEN];
        uuid_unparse_lower_compact(uuid, tx);

        CLEAN_BUFFER *wb = buffer_create(0, NULL);
        int code = rrd_function_run(host, wb, 1 /* second */, HTTP_ACCESS_ALL, "c6-gc-mid-defer-fn",
                                    false /* don't wait */, tx,
                                    c6ut_result_cb, &res, NULL, NULL, NULL, NULL,
                                    NULL, "unittest", false);
        if(code != HTTP_RESP_OK) {
            fprintf(stderr, "  FAILED gc-mid-defer: async run returned %d\n", code);
            errors++;
        }

        c6ut_result_begin(parser, tx, "200");

        // let the broker deadline (1s) plus the grace extension (1s) lapse,
        // then run the GC: the defer's reference keeps the parser record
        // alive, so the GC's delete only marks it pending - no delivery yet
        sleep_usec(2200 * USEC_PER_MS);
        pluginsd_inflight_functions_garbage_collect(parser, now_monotonic_usec());

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

        rrd_function_del(host, NULL, "c6-gc-mid-defer-fn", RRD_FUNCTION_REG_SOURCE_INTERNAL);

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
            struct rrd_function_transport *tr = rrd_function_transport_create(NULL);
            // base ref == 1

            // mimics the pluginsd CONFIG path: execute_cb_data IS the transport
            dyncfg_add_low_level(host, "c6test:node", "/c6test",
                                 DYNCFG_STATUS_RUNNING, DYNCFG_TYPE_SINGLE, DYNCFG_SOURCE_TYPE_INTERNAL, "unittest",
                                 DYNCFG_CMD_SCHEMA | DYNCFG_CMD_GET, 0, 0, false,
                                 HTTP_ACCESS_NONE, HTTP_ACCESS_NONE,
                                 c6ut_noop_execute_cb, tr, tr);

            // node pin (+1) - the registry "config c6test:node" entry is
            // INTERNAL with data NULL and takes no ref
            if(refcount_references(&tr->entry_refcount) != 2) {
                fprintf(stderr, "  FAILED dyncfg-pin: after CREATE, entry refs %d != 2\n",
                        refcount_references(&tr->entry_refcount));
                errors++;
            }

            // same-parser re-CREATE with equal cb/data: the transfer condition
            // is false, so no pin churn
            dyncfg_add_low_level(host, "c6test:node", "/c6test",
                                 DYNCFG_STATUS_RUNNING, DYNCFG_TYPE_SINGLE, DYNCFG_SOURCE_TYPE_INTERNAL, "unittest",
                                 DYNCFG_CMD_SCHEMA | DYNCFG_CMD_GET, 0, 0, false,
                                 HTTP_ACCESS_NONE, HTTP_ACCESS_NONE,
                                 c6ut_noop_execute_cb, tr, tr);

            if(refcount_references(&tr->entry_refcount) != 2) {
                fprintf(stderr, "  FAILED dyncfg-pin: re-CREATE changed entry refs to %d (expected 2 - must net zero)\n",
                        refcount_references(&tr->entry_refcount));
                errors++;
            }

            dyncfg_del_low_level(host, "c6test:node");

            if(refcount_references(&tr->entry_refcount) != 1) {
                fprintf(stderr, "  FAILED dyncfg-pin: after DELETE, entry refs %d != 1\n",
                        refcount_references(&tr->entry_refcount));
                errors++;
            }

            rrd_function_transport_mark_dead_and_drain(tr);
            rrd_function_transport_owner_release(tr);
        }

        if(errors == before)
            fprintf(stderr, "  OK dyncfg-pin: CREATE pins once, re-CREATE nets zero, DELETE releases\n");
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

        rrd_function_transactions_create(); // idempotent

        rrd_function_add(host, NULL, "c6-gc-race-fn", 1, 0, 1, "gc", "top",
                         HTTP_ACCESS_ANONYMOUS_DATA, false /* async */, RRD_FUNCTION_REG_SOURCE_COLLECTOR,
                         pluginsd_function_execute_cb, parser->inflight.transport);

        for(size_t i = 0; i < 50 && errors == before; i++) {
            size_t cancels_before = sends.cancels;
            size_t calls_before = res.calls;

            CLEAN_BUFFER *wb = buffer_create(0, NULL);
            int code = rrd_function_run(host, wb, 1 /* second */, HTTP_ACCESS_ALL, "c6-gc-race-fn",
                                        false /* don't wait */, NULL,
                                        c6ut_result_cb, &res, NULL, NULL, NULL, NULL,
                                        NULL, "unittest", false);
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
        rrd_function_del(host, NULL, "c6-gc-race-fn", RRD_FUNCTION_REG_SOURCE_INTERNAL);

        if(errors == before)
            fprintf(stderr, "  OK gc-race: concurrent GC passes cancel and deliver exactly once\n");
    }

    // 7. the broker-keyed deadline accessor (C5b): visible deadline, visible
    //    extension-on-progress, miss after completion
    {
        int before = errors;

        rrd_function_transactions_create(); // idempotent; the standalone env may lack it

        // a SHORT timeout, so a PROGRESS actually extends it (the extension
        // fires only when less than FUNCTIONS_EXTENDED_TIME_ON_PROGRESS_UT
        // remains)
        rrd_function_add(host, NULL, "c5b-deadline-fn", 5, 0, 1, "deadline", "top",
                         HTTP_ACCESS_ANONYMOUS_DATA, false /* async */, RRD_FUNCTION_REG_SOURCE_INTERNAL,
                         c5b_async_execute_cb, NULL);

        CLEAN_BUFFER *wb = buffer_create(0, NULL);
        int code = rrd_function_run(host, wb, 5, HTTP_ACCESS_ALL, "c5b-deadline-fn",
                                    false /* don't wait */, NULL,
                                    NULL, NULL, NULL, NULL, NULL, NULL,
                                    NULL, "unittest", false);
        if(code != HTTP_RESP_OK) {
            fprintf(stderr, "  FAILED deadline: async run returned %d\n", code);
            errors++;
        }

        usec_t d1 = 0, d2 = 0, d3 = 0;
        if(!rrd_function_transaction_deadline(c5b_capture.tx, &d1) || !d1) {
            fprintf(stderr, "  FAILED deadline: accessor missed a live transaction\n");
            errors++;
        }

        rrd_function_progress(c5b_capture.tx);

        if(!rrd_function_transaction_deadline(c5b_capture.tx, &d2)) {
            fprintf(stderr, "  FAILED deadline: accessor missed after progress\n");
            errors++;
        }
        else if(d2 <= d1) {
            fprintf(stderr, "  FAILED deadline: progress did not extend the deadline (%"PRIu64" -> %"PRIu64")\n", d1, d2);
            errors++;
        }

        // finish the call; the broker record must disappear
        c5b_capture.result_cb(c5b_capture.result_wb, HTTP_RESP_OK, c5b_capture.result_cb_data);

        if(rrd_function_transaction_deadline(c5b_capture.tx, &d3)) {
            fprintf(stderr, "  FAILED deadline: accessor still finds a completed transaction\n");
            errors++;
        }

        rrd_function_del(host, NULL, "c5b-deadline-fn", RRD_FUNCTION_REG_SOURCE_INTERNAL);

        if(errors == before)
            fprintf(stderr, "  OK deadline: accessor sees the deadline, the progress extension, and the completion\n");
    }

    fprintf(stderr, "%s() %s (%d error%s)\n\n",
            __FUNCTION__, errors ? "FAILED" : "passed", errors, errors == 1 ? "" : "s");

    return errors;
}
