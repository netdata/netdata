// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrd.h"
#include "nrpc-internals.h"
#include "sqlite/sqlite_aclk_node.h"
#include "aclk/aclk_query_queue.h"

// ----------------------------------------------------------------------------
// Regression test for GHSA-6628-vxm3-4g8g.
//
// The MCP execute_function path used to build parameter-validation / "info"
// help output (log source names, file counts, sizes, coverage windows,
// timestamps) BEFORE enforcing the caller's access level, while the normal
// /api/v3/function path denied the same anonymous caller. The fix routes both
// paths through rrd_function_verify_access(): /api/v3 via rrd_function_run(),
// and MCP by calling it directly before disclosing any metadata.
//
// This test pins the shared gate: for a protected function (like
// systemd-journal, which requires signed-in + same-space + sensitive-data), an
// anonymous caller MUST be denied, and the acquire/release contract of
// out_acquired MUST hold (item set iff authorized, never leaked on denial). If
// the gate ever regresses, every caller that relies on it — including MCP —
// regresses with it.

static int rrdfunctions_unittest_noop_cb(struct rrd_function_execute *rfe __maybe_unused, void *data __maybe_unused) {
    // never reached: verify_access authorizes without executing
    return HTTP_RESP_OK;
}

int rrdfunctions_verify_access_unittest(void) {
    fprintf(stderr, "\n%s() running...\n", __FUNCTION__);

    RRDHOST *host = localhost;
    if(!host) {
        fprintf(stderr, "  FAILED: localhost is NULL (rrd_init not prepared)\n");
        return 1;
    }

    // A protected function mirroring systemd-journal's requirements.
    rrd_function_add(host, NULL, "protected-fn", 10, 0, 1, "protected", "logs",
                     HTTP_ACCESS_SIGNED_ID | HTTP_ACCESS_SAME_SPACE | HTTP_ACCESS_SENSITIVE_DATA,
                     true, RRD_FUNCTION_REG_SOURCE_INTERNAL, rrdfunctions_unittest_noop_cb, NULL);

    // A restricted function: name starting with "__" flags RRD_FUNCTION_RESTRICTED.
    rrd_function_add(host, NULL, "__restricted-fn", 10, 0, 1, "restricted", "top",
                     HTTP_ACCESS_NONE, true, RRD_FUNCTION_REG_SOURCE_INTERNAL, rrdfunctions_unittest_noop_cb, NULL);

    // A public function requiring nothing — baseline that the gate does not over-block.
    rrd_function_add(host, NULL, "public-fn", 10, 0, 1, "public", "top",
                     HTTP_ACCESS_NONE, true, RRD_FUNCTION_REG_SOURCE_INTERNAL, rrdfunctions_unittest_noop_cb, NULL);

    struct {
        RRDHOST *host;
        const char *fn;
        HTTP_ACCESS user_access;
        bool allow_restricted;
        int expect_code;
        bool expect_item;
    } cases[] = {
        // GHSA-6628-vxm3-4g8g: anonymous caller denied on the protected function.
        // (~SIGNED_ID -> 412, exactly what /api/v3/function returned in the report.)
        { host, "protected-fn", HTTP_ACCESS_ANONYMOUS_DATA, false, HTTP_RESP_PRECOND_FAIL, false },

        // Signed-in but still missing same-space + sensitive-data -> denied (403, has SIGNED_ID).
        { host, "protected-fn", HTTP_ACCESS_SIGNED_ID, false, HTTP_RESP_FORBIDDEN, false },

        // Fully authorized caller -> allowed, item acquired.
        { host, "protected-fn", HTTP_ACCESS_SIGNED_ID | HTTP_ACCESS_SAME_SPACE | HTTP_ACCESS_SENSITIVE_DATA,
          false, HTTP_RESP_OK, true },

        // Restricted function is blocked from this API even for a fully authorized caller.
        { host, "__restricted-fn", HTTP_ACCESS_SIGNED_ID | HTTP_ACCESS_SAME_SPACE | HTTP_ACCESS_SENSITIVE_DATA,
          false, HTTP_RESP_FORBIDDEN, false },

        // ... unless the internal caller explicitly allows restricted functions.
        { host, "__restricted-fn", HTTP_ACCESS_ANONYMOUS_DATA, true, HTTP_RESP_OK, true },

        // Public function stays reachable anonymously — the gate must not over-block.
        { host, "public-fn", HTTP_ACCESS_ANONYMOUS_DATA, false, HTTP_RESP_OK, true },

        // Unknown function -> 404, no item.
        { host, "does-not-exist", HTTP_ACCESS_ANONYMOUS_DATA, false, HTTP_RESP_NOT_FOUND, false },

        // No host to route to -> 500, no item, error body populated. Guards the early
        // !host branch that resets out_acquired before any function lookup.
        { NULL, "protected-fn", HTTP_ACCESS_ANONYMOUS_DATA, false, HTTP_RESP_INTERNAL_SERVER_ERROR, false },
    };

    int errors = 0;
    for(size_t i = 0; i < _countof(cases); i++) {
        CLEAN_BUFFER *wb = buffer_create(0, NULL);
        RRD_FUNCTION_ACQUIRED *item = (RRD_FUNCTION_ACQUIRED *)(uintptr_t)0x1; // poison: verify it is reset

        int code = rrd_function_verify_access(cases[i].host, wb, cases[i].fn,
                                              cases[i].user_access, cases[i].allow_restricted, &item);

        bool ok = false, item_ok = false, body_ok = false;

        if(code != cases[i].expect_code) {
            fprintf(stderr, "  FAILED case %zu (%s, access 0x%x, allow_restricted=%d): "
                            "got code %d, expected %d\n",
                    i, cases[i].fn, (unsigned)cases[i].user_access, cases[i].allow_restricted,
                    code, cases[i].expect_code);
            errors++;
        }
        else
            ok = true;

        // Acquire/release contract: item set iff authorized, never left dangling on denial.
        if(cases[i].expect_item && !item) {
            fprintf(stderr, "  FAILED case %zu (%s): expected an acquired item, got NULL\n",
                    i, cases[i].fn);
            errors++;
        }
        else if(!cases[i].expect_item && item != NULL) {
            fprintf(stderr, "  FAILED case %zu (%s): item must be NULL on denial, but it is set\n",
                    i, cases[i].fn);
            errors++;
        }
        else
            item_ok = true;

        // Denial must emit an error body (rrd_call_function_error), not silently return an
        // empty result_wb. A regression that drops the message would still pass the code and
        // item checks above but would leave the caller with nothing — the security fix
        // requires the denial to be explicit. Authorized calls do not touch result_wb here.
        if(cases[i].expect_code != HTTP_RESP_OK && buffer_strlen(wb) == 0) {
            fprintf(stderr, "  FAILED case %zu (%s): denial produced an empty error body\n",
                    i, cases[i].fn);
            errors++;
        }
        else
            body_ok = true;

        if(item)
            rrd_function_acquired_release(cases[i].host, item);

        if(ok && item_ok && body_ok)
            fprintf(stderr, "  OK case %zu: %s (access 0x%x, allow_restricted=%d) -> %d\n",
                    i, cases[i].fn, (unsigned)cases[i].user_access, cases[i].allow_restricted, code);
    }

    rrd_function_del(host, NULL, "protected-fn", RRD_FUNCTION_REG_SOURCE_INTERNAL);
    rrd_function_del(host, NULL, "__restricted-fn", RRD_FUNCTION_REG_SOURCE_INTERNAL);
    rrd_function_del(host, NULL, "public-fn", RRD_FUNCTION_REG_SOURCE_INTERNAL);

    fprintf(stderr, "%s() %s (%d error%s)\n\n",
            __FUNCTION__, errors ? "FAILED" : "passed", errors, errors == 1 ? "" : "s");

    return errors;
}

// ----------------------------------------------------------------------------
// Pins the contents of the ACLK node instance manifest.
//
// host_functions_to_manifest_dict() decides what the agent tells Netdata Cloud
// about this node's functions. Two exclusions are contractual:
//
//   - dyncfg functions. They are an internal configuration transport, not
//     user-facing, and the cloud must never be offered them. The exclusion is
//     by RRD_FUNCTION_DYNCFG, which get_function_options() derives from the
//     SANITIZED KEY, so the classifier is pinned directly below against every
//     name shape the tree actually produces.
//
//   - restricted functions ("__" prefix, or the "hidden" tag).
//
// It also pins that help survives as a byte copy, because the entry has to stay
// valid after the host functions read lock is released.
//
// Deliberately does NOT register the bare "config" name: that is the live dyncfg
// function on localhost (dyncfg-tree.c), and registering plus deleting it here
// would tear down real dyncfg state for the tests that follow. The bare and
// leading-space shapes are covered by the classifier assertions instead.

int rrdfunctions_manifest_unittest(void) {
    fprintf(stderr, "\n%s() running...\n", __FUNCTION__);

    RRDHOST *host = localhost;
    if(!host) {
        fprintf(stderr, "  FAILED: localhost is NULL (rrd_init not prepared)\n");
        return 1;
    }

    int errors = 0;

    // 1. The dyncfg classifier, on the sanitized key. " config" matters because the sanitizer
    //    strips the leading space, so it lands on the "config" key and must classify as dyncfg -
    //    classifying the raw name instead would hand a dyncfg-reserved key to a regular function.
    struct { const char *raw; bool expected_dyncfg; } names[] = {
        { PLUGINSD_FUNCTION_CONFIG, true  },   // dyncfg-tree.c
        { "config test:job",        true  },   // dyncfg_insert_cb() builds "config <id>"
        { " config",                true  },   // sanitizes to "config"
        { "  config  ",             true  },
        { "configuration",          false },   // NOT dyncfg - "config" is only a prefix here
        { "config-something",       false },
        { "manifest-visible-fn",    false },
    };

    for(size_t i = 0; i < _countof(names); i++) {
        char key[PLUGINSD_LINE_MAX];
        rrd_functions_sanitize(key, names[i].raw, sizeof(key));
        bool got = rrd_function_name_is_dyncfg(key);

        if(got != names[i].expected_dyncfg) {
            fprintf(stderr, "  FAILED classifier: '%s' -> key '%s' -> dyncfg=%s, expected %s\n",
                    names[i].raw, key, got ? "true" : "false",
                    names[i].expected_dyncfg ? "true" : "false");
            errors++;
        }
        else
            fprintf(stderr, "  OK classifier: %-20s -> key '%-18s' dyncfg=%s\n",
                    names[i].raw, key, got ? "true" : "false");
    }

    // 1b. Registry-side enforcement of the dyncfg namespace: a COLLECTOR registration of a
    //     reserved name must be rejected by rrd_function_add() itself (the caller-side pluginsd
    //     guard was removed - the registry is now the only gate). The bare "config" name is the
    //     LIVE dyncfg function on localhost, so the assertions also pin that a rejected
    //     registration leaves the live entry untouched (no execute_cb hijack via conflict swap).
    {
        int enforce_errors_before = errors;

        struct rrd_host_function *live = dictionary_get(host->functions->dict, "config");
        rrd_function_execute_cb_t live_cb = live ? live->execute_cb : NULL;

        const char *rejected[] = {
            "config",               // exact reserved name
            " config",              // sanitizes to "config" - must classify on the sanitized key
            "config c2-reject:job", // per-id reserved name
        };

        for(size_t i = 0; i < _countof(rejected); i++)
            rrd_function_add(host, NULL, rejected[i], 10, 0, 1, "hijack attempt", "top",
                             HTTP_ACCESS_ANONYMOUS_DATA, true, RRD_FUNCTION_REG_SOURCE_COLLECTOR,
                             rrdfunctions_unittest_noop_cb, NULL);

        if(dictionary_get(host->functions->dict, "config c2-reject:job")) {
            fprintf(stderr, "  FAILED enforcement: COLLECTOR registered reserved name 'config c2-reject:job'\n");
            errors++;
            rrd_function_del(host, NULL, "config c2-reject:job", RRD_FUNCTION_REG_SOURCE_INTERNAL);
        }

        // fixed-size dictionary slots keep the value pointer stable across conflicts, so a
        // hijack is visible only through the swapped execute_cb - compare against the capture
        struct rrd_host_function *live_after = dictionary_get(host->functions->dict, "config");
        if(live && (!live_after || live_after->execute_cb != live_cb ||
                    live_after->execute_cb == rrdfunctions_unittest_noop_cb)) {
            fprintf(stderr, "  FAILED enforcement: COLLECTOR registration of 'config' touched the live dyncfg entry\n");
            errors++;
        }

        // if there is NO live "config" in this environment, the rejected registrations must
        // not have created one
        if(!live && dictionary_get(host->functions->dict, "config")) {
            fprintf(stderr, "  FAILED enforcement: COLLECTOR registration created reserved name 'config'\n");
            errors++;
            rrd_function_del(host, NULL, "config", RRD_FUNCTION_REG_SOURCE_INTERNAL);
        }

        if(errors == enforce_errors_before)
            fprintf(stderr, "  OK enforcement: COLLECTOR registrations of 'config', ' config' and 'config <id>' rejected\n");
    }

    // 2. The manifest dictionary itself, using names that cannot collide with live functions.
    struct {
        const char *name;
        const char *tags;
        bool expected_in_manifest;
    } fns[] = {
        { "config manifest-test:job",  "config",                false },  // dyncfg -> excluded
        { "__manifest-hidden-fn",      "top",                   false },  // "__" -> restricted
        { "manifest-tagged-fn",        RRDFUNCTIONS_TAG_HIDDEN, false },  // hidden tag -> restricted
        { "manifest-visible-fn",       "top",                   true  },
        { "manifest-logs-fn",          "logs",                  true  },
    };

    for(size_t i = 0; i < _countof(fns); i++)
        rrd_function_add(host, NULL, fns[i].name, 10, 0, 1, "manifest help text", fns[i].tags,
                         HTTP_ACCESS_ANONYMOUS_DATA, true, RRD_FUNCTION_REG_SOURCE_INTERNAL,
                         rrdfunctions_unittest_noop_cb, NULL);

    DICTIONARY *manifest = host_functions_to_manifest_dict(host);

    for(size_t i = 0; i < _countof(fns); i++) {
        struct rrd_function_manifest_entry *e = dictionary_get(manifest, fns[i].name);
        bool present = (e != NULL);

        if(present != fns[i].expected_in_manifest) {
            fprintf(stderr, "  FAILED '%s': %s the manifest, expected %s\n",
                    fns[i].name, present ? "IS in" : "is NOT in",
                    fns[i].expected_in_manifest ? "present" : "absent");
            errors++;
        }
        else if(present && (!e->help || strcmp(e->help, "manifest help text") != 0)) {
            fprintf(stderr, "  FAILED '%s': help not copied (got '%s')\n",
                    fns[i].name, e->help ? e->help : "(null)");
            errors++;
        }
        else
            fprintf(stderr, "  OK %-26s -> %s\n",
                    fns[i].name, present ? "in manifest" : "excluded");
    }

    // 3. No dyncfg function of any kind may appear, including the live ones this test did not
    //    register - a belt-and-braces sweep over whatever the running agent has.
    struct rrd_function_manifest_entry *v;
    dfe_start_read(manifest, v) {
        (void)v;
        char key[PLUGINSD_LINE_MAX];
        rrd_functions_sanitize(key, v_dfe.name, sizeof(key));
        if(rrd_function_name_is_dyncfg(key)) {
            fprintf(stderr, "  FAILED: dyncfg function '%s' present in the manifest\n", v_dfe.name);
            errors++;
        }
    }
    dfe_done(v);

    dictionary_destroy(manifest);

    for(size_t i = 0; i < _countof(fns); i++)
        rrd_function_del(host, NULL, fns[i].name, RRD_FUNCTION_REG_SOURCE_INTERNAL);

    // 4. The suppression hash. build_node_manifest() publishes only when manifest_dict_hash()
    //    differs from the last sent hash, so the hash must be: deterministic for identical
    //    content, independent of dictionary traversal order, and sensitive to every transmitted
    //    field including the node_id and claim_id the manifest is keyed under at the cloud. Local
    //    standalone dictionaries keep this independent of whatever live functions localhost has.
    //    The other half of the suppression - scoping the record to one ACLK session - is folded
    //    into the same value by manifest_publication_key(), covered at the end of this block.
    //    What is NOT covered here is the recording side: build_node_manifest() records the key under a
    //    per-enqueue token as it enqueues, and aclk_node_manifest_publish_result() invalidates that
    //    token if the message was dropped. Invalidation is pure atomics on the host's ACLK config, so
    //    it does NOT need a live ACLK connection - but it does need a host that HAS a config, and this
    //    test's localhost does not: sql_load_node_id() only calls set_host_node_id() when the query
    //    returns a row, so an unregistered host reaches neither it nor create_aclk_config(), the only
    //    place host->aclk_host_config is ever set. A test would have to call create_aclk_config()
    //    itself (a plain callocz plus a publish CAS, no ACLK thread needed) and include
    //    aclk/aclk_query_queue.h for struct aclk_manifest_publication. Untested; tracked in the SOW.
    {
        int hash_errors_before = errors;
        const char *node1 = "11111111-2222-3333-4444-555555555555";
        const char *node2 = "66666666-7777-8888-9999-aaaaaaaaaaaa";
        const char *claim = "bbbbbbbb-cccc-dddd-eeee-ffffffffffff";

        struct rrd_function_manifest_entry e1 = { .help = "help one", .tags = "top",
                                                  .access = HTTP_ACCESS_ANONYMOUS_DATA, .priority = 0, .version = 1 };
        struct rrd_function_manifest_entry e2 = { .help = "help two", .tags = "logs",
                                                  .access = HTTP_ACCESS_SAME_SPACE, .priority = 50, .version = 2 };

        DICTIONARY *a = dictionary_create(DICT_OPTION_SINGLE_THREADED);
        DICTIONARY *b = dictionary_create(DICT_OPTION_SINGLE_THREADED);
        dictionary_set(a, "fn-one", &e1, sizeof(e1));
        dictionary_set(a, "fn-two", &e2, sizeof(e2));
        // same content, opposite insertion order
        dictionary_set(b, "fn-two", &e2, sizeof(e2));
        dictionary_set(b, "fn-one", &e1, sizeof(e1));

        uint64_t ha = manifest_dict_hash(a, node1, claim);
        uint64_t hb = manifest_dict_hash(b, node1, claim);
        if(ha != hb) {
            fprintf(stderr, "  FAILED hash: identical content in different insertion order hashed differently\n");
            errors++;
        }

        // every transmitted field must change the hash - one mutation of e1 per field, applied to
        // b's "fn-one" and reverted, so each case is compared against the unchanged content hash
        struct rrd_function_manifest_entry m_help = e1;     m_help.help = "help one modified";
        struct rrd_function_manifest_entry m_tags = e1;     m_tags.tags = "top other";
        struct rrd_function_manifest_entry m_access = e1;   m_access.access = HTTP_ACCESS_SIGNED_ID;
        struct rrd_function_manifest_entry m_priority = e1; m_priority.priority = 42;
        struct rrd_function_manifest_entry m_version = e1;  m_version.version = 2;

        struct {
            const char *field;
            struct rrd_function_manifest_entry *mutated;
        } mutations[] = {
            { "help",     &m_help },
            { "tags",     &m_tags },
            { "access",   &m_access },
            { "priority", &m_priority },
            { "version",  &m_version },
        };

        for(size_t i = 0; i < _countof(mutations); i++) {
            dictionary_set(b, "fn-one", mutations[i].mutated, sizeof(e1));
            if(manifest_dict_hash(b, node1, claim) == ha) {
                fprintf(stderr, "  FAILED hash: modified %s did not change the hash\n", mutations[i].field);
                errors++;
            }
            dictionary_set(b, "fn-one", &e1, sizeof(e1));
        }

        // removing a function must change the hash. The entry count is not hashed on its own -
        // per-entry hashes are XOR-combined - so this pins that a dropped entry is still visible.
        dictionary_del(b, "fn-two");
        if(manifest_dict_hash(b, node1, claim) == ha) {
            fprintf(stderr, "  FAILED hash: removing a function did not change the hash\n");
            errors++;
        }
        dictionary_set(b, "fn-two", &e2, sizeof(e2));
        if(manifest_dict_hash(b, node1, claim) != ha) {
            fprintf(stderr, "  FAILED hash: restoring a removed function did not restore the hash\n");
            errors++;
        }

        // a non-positive priority is transmitted as the default - hashing the raw value would
        // see a "change" the cloud never sees, so both must hash identically
        struct rrd_function_manifest_entry e1p = e1;
        e1p.priority = -5;
        dictionary_set(b, "fn-one", &e1p, sizeof(e1p));
        if(manifest_dict_hash(b, node1, claim) != ha) {
            fprintf(stderr, "  FAILED hash: negative priority hashed differently from the transmitted default\n");
            errors++;
        }
        dictionary_set(b, "fn-one", &e1, sizeof(e1));

        // same functions under a different node_id is a different manifest at the cloud
        if(manifest_dict_hash(a, node2, claim) == ha) {
            fprintf(stderr, "  FAILED hash: node_id change did not change the hash\n");
            errors++;
        }

        // the claim_id is transmitted too, and a re-claim must re-publish
        if(manifest_dict_hash(a, node1, "cccccccc-dddd-eeee-ffff-000000000000") == ha) {
            fprintf(stderr, "  FAILED hash: claim_id change did not change the hash\n");
            errors++;
        }

        // an empty manifest is still content (means "no functions" to the cloud) and must
        // differ from a non-empty one; an absent dict hashes like an empty one
        DICTIONARY *empty = dictionary_create(DICT_OPTION_SINGLE_THREADED);
        if(manifest_dict_hash(empty, node1, claim) == ha) {
            fprintf(stderr, "  FAILED hash: empty manifest hashed like a non-empty one\n");
            errors++;
        }
        if(manifest_dict_hash(NULL, node1, claim) != manifest_dict_hash(empty, node1, claim)) {
            fprintf(stderr, "  FAILED hash: NULL dict and empty dict hashed differently\n");
            errors++;
        }

        // field concatenation ambiguity: "ab"+"c" must not hash like "a"+"bc"
        DICTIONARY *c1 = dictionary_create(DICT_OPTION_SINGLE_THREADED);
        DICTIONARY *c2 = dictionary_create(DICT_OPTION_SINGLE_THREADED);
        struct rrd_function_manifest_entry x1 = { .help = "ab", .tags = "c", .access = 0, .priority = 1, .version = 1 };
        struct rrd_function_manifest_entry x2 = { .help = "a",  .tags = "bc", .access = 0, .priority = 1, .version = 1 };
        dictionary_set(c1, "fn", &x1, sizeof(x1));
        dictionary_set(c2, "fn", &x2, sizeof(x2));
        if(manifest_dict_hash(c1, node1, claim) == manifest_dict_hash(c2, node1, claim)) {
            fprintf(stderr, "  FAILED hash: field concatenation ambiguity\n");
            errors++;
        }

        // the ACLK session is folded into the same value, so one comparison covers both content and
        // session (see node_manifest_sent_key). Determinism for identical inputs is not asserted: the
        // key is a pure function of them, so such a check cannot fail.
        usec_t s1 = 1700000000000000ULL, s2 = 1700000000000001ULL;
        if(manifest_publication_key(ha, s1) == manifest_publication_key(ha, s2)) {
            fprintf(stderr, "  FAILED key: a new ACLK session did not change the key\n");
            errors++;
        }
        if(manifest_publication_key(ha, s1) == manifest_publication_key(manifest_dict_hash(empty, node1, claim), s1)) {
            fprintf(stderr, "  FAILED key: content change did not survive folding the session in\n");
            errors++;
        }

        if(errors == hash_errors_before)
            fprintf(stderr, "  OK hash: determinism, order independence, field, node_id, claim_id and session sensitivity\n");

        dictionary_destroy(a);
        dictionary_destroy(b);
        dictionary_destroy(empty);
        dictionary_destroy(c1);
        dictionary_destroy(c2);
    }

    // 5. The publication record - what stops a manifest that never reached the mqtt layer from
    //    suppressing every later identical build. build_node_manifest() records (key, token) as it
    //    enqueues; aclk_node_manifest_publish_result() invalidates that record when the message was
    //    dropped, by CASing the TOKEN it carried and never the key. The token is why that is safe:
    //    a manifest enqueued after this one holds a different token, so it keeps its record even
    //    when its content - and therefore its key - is identical, which is exactly the case a
    //    content-keyed clear would get wrong (content published, changed, then changed back while
    //    the first publication is still in flight).
    //
    //    Pinned directly because none of it is observable from the message stream. It needs no ACLK
    //    connection and no ACLK thread: the invalidation is atomics on the host's ACLK config, and
    //    with no sync thread running aclk_sync_on_event_loop_thread() is false, so the write-back
    //    takes an uncontended rrd_rdlock() and resolves localhost from rrdhost_root_index normally.
    {
        int record_errors_before = errors;

        // An unregistered host has no ACLK config - sql_load_node_id() only reaches
        // create_aclk_config() for a host the cloud has registered - so make one here. A NULL
        // node_id keeps create_aclk_config() from also writing host->node_id.
        //
        // Remember whether this test is the one that created it, so the cleanup below can put the
        // host back exactly as it was found: this binary runs several tests in-process against the
        // same localhost, and leaving an ACLK config attached would hand them mutable state (and a
        // leaked allocation) they never asked for.
        bool config_created_here = (__atomic_load_n(&host->aclk_host_config, __ATOMIC_ACQUIRE) == NULL);
        create_aclk_config(host, &host->host_id.uuid, NULL);
        aclk_sync_cfg_t *cfg = __atomic_load_n(&host->aclk_host_config, __ATOMIC_ACQUIRE);

        if(!cfg) {
            fprintf(stderr, "  FAILED record: could not create an ACLK config for localhost\n");
            errors++;
        }
        else {
            const uint64_t K = 0x0123456789abcdefULL; // any content key; the record is content-blind
            const uint64_t T1 = 1111, T2 = 2222, T3 = 3333;

            struct aclk_manifest_publication pub = { .key = K, .token = T1, .published = false };
            strncpyz(pub.machine_guid, host->machine_guid, sizeof(pub.machine_guid) - 1);

            // a dropped manifest invalidates its own record and re-arms, so the content is rebuilt
            __atomic_store_n(&cfg->node_manifest_sent_key, K, __ATOMIC_RELEASE);
            __atomic_store_n(&cfg->node_manifest_sent_token, T1, __ATOMIC_RELEASE);
            aclk_send_timestamp_set(&cfg->node_manifest_send_time, 0);

            aclk_node_manifest_publish_result(&pub);

            if(__atomic_load_n(&cfg->node_manifest_sent_token, __ATOMIC_ACQUIRE) != 0) {
                fprintf(stderr, "  FAILED record: a dropped manifest did not invalidate its own token\n");
                errors++;
            }
            if(aclk_send_timestamp_get(&cfg->node_manifest_send_time) == 0) {
                fprintf(stderr, "  FAILED record: a dropped manifest did not re-arm the request\n");
                errors++;
            }

            // the same drop must NOT touch a later enqueue's record, even though the content, and
            // so the key, is identical - only the token distinguishes them
            __atomic_store_n(&cfg->node_manifest_sent_key, K, __ATOMIC_RELEASE);
            __atomic_store_n(&cfg->node_manifest_sent_token, T2, __ATOMIC_RELEASE);
            aclk_send_timestamp_set(&cfg->node_manifest_send_time, 0);

            aclk_node_manifest_publish_result(&pub); // still carries the stale T1

            if(__atomic_load_n(&cfg->node_manifest_sent_token, __ATOMIC_ACQUIRE) != T2) {
                fprintf(stderr, "  FAILED record: a stale drop cleared a later enqueue's token\n");
                errors++;
            }
            if(aclk_send_timestamp_get(&cfg->node_manifest_send_time) != 0) {
                fprintf(stderr, "  FAILED record: a stale drop re-armed a request it does not own\n");
                errors++;
            }

            // a manifest that reached the mqtt layer was already accounted for at enqueue time, so
            // reporting it must change nothing at all
            __atomic_store_n(&cfg->node_manifest_sent_token, T3, __ATOMIC_RELEASE);
            aclk_send_timestamp_set(&cfg->node_manifest_send_time, 0);
            pub.token = T3;
            pub.published = true;

            aclk_node_manifest_publish_result(&pub);

            if(__atomic_load_n(&cfg->node_manifest_sent_token, __ATOMIC_ACQUIRE) != T3 ||
               aclk_send_timestamp_get(&cfg->node_manifest_send_time) != 0) {
                fprintf(stderr, "  FAILED record: a published manifest changed the record\n");
                errors++;
            }

            // a host that no longer resolves is a no-op, not a crash and not someone else's record
            pub.published = false;
            strncpyz(pub.machine_guid, "00000000-0000-0000-0000-000000000000", sizeof(pub.machine_guid) - 1);

            aclk_node_manifest_publish_result(&pub);

            if(__atomic_load_n(&cfg->node_manifest_sent_token, __ATOMIC_ACQUIRE) != T3) {
                fprintf(stderr, "  FAILED record: a publication for an unknown host touched this config\n");
                errors++;
            }

            // leave nothing recorded or armed behind for the rest of the process. Redundant when the
            // config is destroyed below, but this is also the path taken when the config pre-existed
            // and must survive - and after destroy_aclk_config() cfg is freed, so it happens here.
            __atomic_store_n(&cfg->node_manifest_sent_token, 0, __ATOMIC_RELEASE);
            __atomic_store_n(&cfg->node_manifest_sent_key, 0, __ATOMIC_RELEASE);
            aclk_send_timestamp_set(&cfg->node_manifest_send_time, 0);
        }

        // Detach and free the config if this test created it. With no ACLK sync thread running in the
        // unittest binary, destroy_aclk_config() skips its event-loop round trip (it is gated on
        // aclk_sync_config.initialized, set only inside aclk_synchronization_event_loop()) and reduces
        // to an atomic exchange plus freez - no thread, no timer, no database. cfg is dangling after
        // this point.
        if(config_created_here)
            destroy_aclk_config(host);

        if(errors == record_errors_before)
            fprintf(stderr, "  OK record: a drop invalidates its own token, a stale drop does not, "
                            "a publish is a no-op, an unknown host is a no-op\n");
    }

    fprintf(stderr, "%s() %s (%d error%s)\n\n",
            __FUNCTION__, errors ? "FAILED" : "passed", errors, errors == 1 ? "" : "s");

    return errors;
}

// ----------------------------------------------------------------------------
// The node-manifest pacer (MANIFEST_PACER in sqlite_aclk_node.h).
//
// aclk_check_node_info_collectors_and_manifest() publishes at most
// MAX_NODE_MANIFESTS_PER_SCAN manifests per pass, and picks WHICH due hosts get those slots by
// oldest deadline rather than by position in rrdhost_root_index. Both halves matter and neither is
// observable from the message stream: the budget keeps a fleet-wide arm (every ACLK reconnect) from
// publishing an entire parent's hosts in one second, and the earliest-deadline-first cutoff is what
// stops hosts late in the index from being starved forever by hosts ahead of them that keep
// re-arming - with B slots per pass and a W second window, plain index order can only ever reach
// the first B*W hosts.
//
// This pins the pacer directly, because the scan it lives in needs a live fleet and an ACLK
// connection to exercise.
int rrdfunctions_manifest_pacer_unittest(void) {
    fprintf(stderr, "\n%s() running...\n", __FUNCTION__);

    int errors = 0;

    // 1. The deferred set keeps the OLDEST deadlines, and the cutoff is the newest of those - so
    //    "deadline <= cutoff" admits exactly the set that was tracked, and nothing newer.
    {
        int before = errors;
        MANIFEST_PACER p = { 0 };
        manifest_pacer_begin(&p);

        // more entries than the pacer can track, deliberately out of order
        for(size_t i = 0; i < MAX_NODE_MANIFESTS_PER_SCAN * 3; i++) {
            // 0, 59, 1, 58, 2, 57 ... interleaves ascending and descending
            time_t d = (i % 2) ? (time_t)(MAX_NODE_MANIFESTS_PER_SCAN * 3 - i) : (time_t)(i + 1);
            manifest_pacer_defer(&p, d);
        }

        if(p.deferred != MAX_NODE_MANIFESTS_PER_SCAN) {
            fprintf(stderr, "  FAILED pacer: tracked %zu deadlines, expected %d\n",
                    p.deferred, MAX_NODE_MANIFESTS_PER_SCAN);
            errors++;
        }

        for(size_t i = 1; i < p.deferred; i++) {
            if(p.deadlines[i - 1] > p.deadlines[i]) {
                fprintf(stderr, "  FAILED pacer: deferred deadlines are not sorted oldest first\n");
                errors++;
                break;
            }
        }

        manifest_pacer_end(&p);
        if(p.cutoff != p.deadlines[MAX_NODE_MANIFESTS_PER_SCAN - 1]) {
            fprintf(stderr, "  FAILED pacer: cutoff is not the newest of the tracked oldest set\n");
            errors++;
        }

        // the tracked set must be the globally oldest ones seen, not the first ones seen
        for(size_t i = 0; i < p.deferred; i++) {
            if(p.deadlines[i] > (time_t)MAX_NODE_MANIFESTS_PER_SCAN) {
                fprintf(stderr, "  FAILED pacer: a newer deadline displaced an older one\n");
                errors++;
                break;
            }
        }

        if(errors == before)
            fprintf(stderr, "  OK pacer: deferred set keeps the oldest deadlines, cutoff derived from them\n");
    }

    // 2. A pass that leaves nothing waiting must clear the cutoff. Without this an old cutoff would
    //    outlive the backlog that produced it and keep excluding hosts that are legitimately due.
    {
        int before = errors;
        MANIFEST_PACER p = { 0 };

        manifest_pacer_begin(&p);
        manifest_pacer_defer(&p, 100);
        manifest_pacer_end(&p);
        if(p.cutoff != 100) {
            fprintf(stderr, "  FAILED pacer: cutoff not carried from a pass that deferred work\n");
            errors++;
        }

        manifest_pacer_begin(&p);   // nothing deferred this pass
        manifest_pacer_end(&p);
        if(p.cutoff != 0) {
            fprintf(stderr, "  FAILED pacer: cutoff not cleared after a pass with nothing waiting\n");
            errors++;
        }
        if(!manifest_pacer_admit(&p, 999999)) {
            fprintf(stderr, "  FAILED pacer: a cleared cutoff must admit any deadline\n");
            errors++;
        }

        if(errors == before)
            fprintf(stderr, "  OK pacer: cutoff clears when a pass leaves nothing waiting\n");
    }

    // 3. The budget caps a pass, and only actual publishes are charged - a claimed-but-suppressed
    //    manifest must not consume a slot belonging to the hosts behind it.
    {
        int before = errors;
        MANIFEST_PACER p = { 0 };
        manifest_pacer_begin(&p);

        for(int i = 0; i < MAX_NODE_MANIFESTS_PER_SCAN; i++) {
            if(!manifest_pacer_admit(&p, 100)) {
                fprintf(stderr, "  FAILED pacer: budget exhausted after %d of %d publishes\n",
                        i, MAX_NODE_MANIFESTS_PER_SCAN);
                errors++;
                break;
            }
            manifest_pacer_published(&p);
        }

        if(manifest_pacer_admit(&p, 100)) {
            fprintf(stderr, "  FAILED pacer: budget did not cap the pass\n");
            errors++;
        }

        manifest_pacer_begin(&p);
        for(int i = 0; i < MAX_NODE_MANIFESTS_PER_SCAN * 5; i++)
            (void)manifest_pacer_admit(&p, 100);   // admitted and claimed, but every one suppressed
        if(!manifest_pacer_admit(&p, 100)) {
            fprintf(stderr, "  FAILED pacer: suppressed manifests consumed the send budget\n");
            errors++;
        }

        if(errors == before)
            fprintf(stderr, "  OK pacer: budget caps a pass and charges only real publishes\n");
    }

    // 4. Liveness: a host late in the index with an OLD deadline must be served promptly even when
    //    the hosts ahead of it are permanently oversubscribed. This is the case plain index order
    //    cannot serve at all: 1200 hot hosts each needing service once per window is ~40 hosts/pass
    //    of demand against a budget of 20, so an index-ordered scan never reaches index 1200+.
    {
        int before = errors;
        const size_t hot = 1200, cold = 100, hosts = hot + cold;

        time_t *deadline = mallocz(hosts * sizeof(time_t));
        bool *published = callocz(hosts, sizeof(bool));

        // the cold tail is armed strictly earlier, so oldest-first must reach it before any hot host
        for(size_t i = 0; i < hot; i++) deadline[i] = 5;
        for(size_t i = hot; i < hosts; i++) deadline[i] = 1;

        MANIFEST_PACER p = { 0 };
        size_t cold_published = 0, passes = 0;
        const size_t max_passes = 20;

        for(time_t now = 36; cold_published < cold && passes < max_passes; now++, passes++) {
            manifest_pacer_begin(&p);
            size_t published_this_pass = 0;

            for(size_t i = 0; i < hosts; i++) {
                if(!deadline[i] || nd_time_t_add_compare(deadline[i], NODE_MANIFEST_WINDOW_S, now) >= 0)
                    continue;   // unarmed or still inside the coalescing window

                if(manifest_pacer_admit(&p, deadline[i])) {
                    if(!published[i]) {
                        published[i] = true;
                        if(i >= hot) cold_published++;
                    }
                    manifest_pacer_published(&p);
                    published_this_pass++;
                    // a hot host re-arms immediately, refreshing its deadline to now
                    deadline[i] = (i < hot) ? now : 0;
                }
                else
                    manifest_pacer_defer(&p, deadline[i]);
            }

            manifest_pacer_end(&p);

            if(published_this_pass > MAX_NODE_MANIFESTS_PER_SCAN) {
                fprintf(stderr, "  FAILED pacer: %zu manifests in one pass, budget is %d\n",
                        published_this_pass, MAX_NODE_MANIFESTS_PER_SCAN);
                errors++;
                break;
            }
        }

        if(cold_published != cold) {
            fprintf(stderr,
                    "  FAILED pacer: only %zu of %zu starved hosts served in %zu passes"
                    " - the oldest deadline is not winning\n",
                    cold_published, cold, passes);
            errors++;
        }

        if(errors == before)
            fprintf(stderr, "  OK pacer: oldest-deadline hosts served in %zu passes despite %zu"
                            " oversubscribed hosts ahead of them\n", passes, hot);

        freez(deadline);
        freez(published);
    }

    fprintf(stderr, "%s() %s (%d error%s)\n\n",
            __FUNCTION__, errors ? "FAILED" : "passed", errors, errors == 1 ? "" : "s");

    return errors;
}

// ----------------------------------------------------------------------------
// The streaming delete-path queue (C3).
//
// rrd_function_del() no longer sends FUNCTION_DEL synchronously; it queues the
// sanitized name in the registry's pending_dels set (only when the host has a
// stream sender configured) and sets RRDHOST_FLAG_GLOBAL_FUNCTIONS_UPDATED.
// The renderer (stream_sender_send_global_rrdhost_functions) clears the flag,
// snapshots-and-clears the set, emits the FUNCTION_DEL lines and then the full
// re-list - one buffer. This pins:
//
//   1. the add/del truth table: which operations set the flag, queue a
//      FUNCTION_DEL, and arm the cloud manifest - including the dyncfg quirk
//      (dyncfg global deletes set the flag but never emit FUNCTION_DEL) and
//      the no-sender case (never-streaming hosts must not grow the queue);
//   2. drain order and discard: DEL lines precede the re-list; a false
//      can_function_del verdict discards the snapshot without emitting;
//   3. the concurrent del-during-drain race: under the insert-before-flag /
//      clear-flag-before-snapshot protocol no DEL is ever lost or duplicated;
//   4. re-add survival + payload healing: a global re-add does NOT cancel a
//      queued FUNCTION_DEL - the DEL renders, and the SAME payload's re-list
//      re-affirms the function (DEL lines precede the re-list), so the parent
//      nets to the correct state within one commit. Cancelling instead could
//      swallow the only prune signal when a concurrent del outruns the add in
//      the registry (see the comment in rrd_function_add's global branch).

struct fndel_race_ctx {
    RRDHOST *host;
    bool done;
};

#define FNDEL_RACE_N 200

static void fndel_race_worker(void *arg) {
    struct fndel_race_ctx *ctx = arg;
    char name[64];
    for(size_t i = 0; i < FNDEL_RACE_N; i++) {
        snprintfz(name, sizeof(name), "tt-race-fn-%zu", i);
        rrd_function_add(ctx->host, NULL, name, 10, 0, 1, "race", "top",
                         HTTP_ACCESS_ANONYMOUS_DATA, true, RRD_FUNCTION_REG_SOURCE_INTERNAL,
                         rrdfunctions_unittest_noop_cb, NULL);
        rrd_function_del(ctx->host, NULL, name, RRD_FUNCTION_REG_SOURCE_INTERNAL);
    }
    __atomic_store_n(&ctx->done, true, __ATOMIC_RELEASE);
}

static bool fndel_queued(RRDHOST *host, const char *name) {
    // set entries carry no value, so dictionary_get() would return NULL for
    // present entries too - test membership through the item instead
    spinlock_lock(&host->functions->pending_dels.spinlock);
    const DICTIONARY_ITEM *item = dictionary_get_and_acquire_item(host->functions->pending_dels.dict, name);
    bool ret = (item != NULL);
    if(item)
        dictionary_acquired_item_release(host->functions->pending_dels.dict, item);
    spinlock_unlock(&host->functions->pending_dels.spinlock);
    return ret;
}

static void fndel_flush_queue(RRDHOST *host) {
    spinlock_lock(&host->functions->pending_dels.spinlock);
    dictionary_flush(host->functions->pending_dels.dict);
    spinlock_unlock(&host->functions->pending_dels.spinlock);
}

int rrdfunctions_del_unittest(void) {
    fprintf(stderr, "\n%s() running...\n", __FUNCTION__);

    RRDHOST *host = localhost;
    if(!host || !host->functions) {
        fprintf(stderr, "  FAILED: localhost (or its functions registry) is NULL\n");
        return 1;
    }

    int errors = 0;

    // fake a configured stream sender: the pending-del gate only checks the
    // option bit and pointer non-NULLness (rrdhost_has_stream_sender_enabled),
    // and no streaming thread runs in the unittest binary, so the poison
    // pointer is never dereferenced; both are restored before returning
    struct sender_state *saved_sender = host->sender;
    bool saved_option = rrdhost_option_check(host, RRDHOST_OPTION_SENDER_ENABLED);

    // manifest arming is observed exactly like rrdfunctions_manifest_unittest()
    // does: through the host's ACLK config send-time timestamp
    bool config_created_here = (__atomic_load_n(&host->aclk_host_config, __ATOMIC_ACQUIRE) == NULL);
    create_aclk_config(host, &host->host_id.uuid, NULL);
    aclk_sync_cfg_t *cfg = __atomic_load_n(&host->aclk_host_config, __ATOMIC_ACQUIRE);
    if(!cfg) {
        fprintf(stderr, "  FAILED: could not create an ACLK config for localhost\n");
        return 1;
    }

    // 1. the add/del truth table
    {
        struct {
            const char *fn;
            bool sender_enabled;
            bool add_expect_arm;    // add always sets the flag for global functions
            bool del_expect_queued;
            bool del_expect_arm;    // del always sets the flag for global functions
        } cases[] = {
            // plain global function, sender configured: queued + armed both ways
            { "tt-plain-fn",     true,  true,  true,  true  },
            // plain global function, no sender: NOT queued (never-streaming host)
            { "tt-nosender-fn",  false, true,  false, true  },
            // dyncfg-shaped name (INTERNAL registration): flag quirk - no
            // FUNCTION_DEL, and dyncfg is never in the manifest
            { "config tt:job",   true,  false, false, false },
            // restricted function: streams (queued) but is not in the manifest.
            // NOTE: add still arms - a re-registration ADDING the restricted
            // flag drops the function OUT of the manifest, and that transition
            // must be reported (see the arm-site comment in rrd_function_add);
            // only the del skips arming (the entry was never in the manifest)
            { "__tt-hidden-fn",  true,  true,  true,  false },
        };

        int tt_errors_before = errors;
        for(size_t i = 0; i < _countof(cases); i++) {
            if(cases[i].sender_enabled) {
                rrdhost_option_set(host, RRDHOST_OPTION_SENDER_ENABLED);
                host->sender = (struct sender_state *)(uintptr_t)0x1;
            }
            else {
                rrdhost_option_clear(host, RRDHOST_OPTION_SENDER_ENABLED);
                host->sender = NULL;
            }

            // --- add ---
            rrdhost_flag_clear(host, RRDHOST_FLAG_GLOBAL_FUNCTIONS_UPDATED);
            aclk_send_timestamp_set(&cfg->node_manifest_send_time, 0);

            rrd_function_add(host, NULL, cases[i].fn, 10, 0, 1, "truth table", "top",
                             HTTP_ACCESS_ANONYMOUS_DATA, true, RRD_FUNCTION_REG_SOURCE_INTERNAL,
                             rrdfunctions_unittest_noop_cb, NULL);

            if(!rrdhost_flag_check(host, RRDHOST_FLAG_GLOBAL_FUNCTIONS_UPDATED)) {
                fprintf(stderr, "  FAILED tt '%s': add did not set the flag\n", cases[i].fn);
                errors++;
            }
            bool add_armed = aclk_send_timestamp_get(&cfg->node_manifest_send_time) != 0;
            if(add_armed != cases[i].add_expect_arm) {
                fprintf(stderr, "  FAILED tt '%s': add armed=%d, expected %d\n",
                        cases[i].fn, add_armed, cases[i].add_expect_arm);
                errors++;
            }

            // --- del ---
            rrdhost_flag_clear(host, RRDHOST_FLAG_GLOBAL_FUNCTIONS_UPDATED);
            aclk_send_timestamp_set(&cfg->node_manifest_send_time, 0);
            fndel_flush_queue(host);

            rrd_function_del(host, NULL, cases[i].fn, RRD_FUNCTION_REG_SOURCE_INTERNAL);

            if(!rrdhost_flag_check(host, RRDHOST_FLAG_GLOBAL_FUNCTIONS_UPDATED)) {
                fprintf(stderr, "  FAILED tt '%s': del did not set the flag\n", cases[i].fn);
                errors++;
            }
            bool queued = fndel_queued(host, cases[i].fn);
            if(queued != cases[i].del_expect_queued) {
                fprintf(stderr, "  FAILED tt '%s': del queued=%d, expected %d\n",
                        cases[i].fn, queued, cases[i].del_expect_queued);
                errors++;
            }
            bool del_armed = aclk_send_timestamp_get(&cfg->node_manifest_send_time) != 0;
            if(del_armed != cases[i].del_expect_arm) {
                fprintf(stderr, "  FAILED tt '%s': del armed=%d, expected %d\n",
                        cases[i].fn, del_armed, cases[i].del_expect_arm);
                errors++;
            }

            fndel_flush_queue(host);
        }

        if(errors == tt_errors_before)
            fprintf(stderr, "  OK truth table: flag/queue/arm behavior for plain, no-sender, dyncfg and restricted deletes\n");
    }

    // sender stays enabled (fake) for the drain and race sections
    rrdhost_option_set(host, RRDHOST_OPTION_SENDER_ENABLED);
    host->sender = (struct sender_state *)(uintptr_t)0x1;

    // 2. drain order and the discard path
    {
        int drain_errors_before = errors;

        rrd_function_add(host, NULL, "tt-keep-fn", 10, 0, 1, "keep", "top",
                         HTTP_ACCESS_ANONYMOUS_DATA, true, RRD_FUNCTION_REG_SOURCE_INTERNAL,
                         rrdfunctions_unittest_noop_cb, NULL);
        rrd_function_add(host, NULL, "tt-del-fn", 10, 0, 1, "del", "top",
                         HTTP_ACCESS_ANONYMOUS_DATA, true, RRD_FUNCTION_REG_SOURCE_INTERNAL,
                         rrdfunctions_unittest_noop_cb, NULL);
        rrd_function_del(host, NULL, "tt-del-fn", RRD_FUNCTION_REG_SOURCE_INTERNAL);

        {
            CLEAN_BUFFER *wb = buffer_create(0, NULL);
            stream_sender_send_global_rrdhost_functions(host, wb, false, true);
            const char *out = buffer_tostring(wb);

            const char *del_line = strstr(out, PLUGINSD_KEYWORD_FUNCTION_DEL " GLOBAL \"tt-del-fn\"");
            const char *keep_line = strstr(out, PLUGINSD_KEYWORD_FUNCTION " GLOBAL \"tt-keep-fn\"");
            // "FUNCTION GLOBAL" cannot match a FUNCTION_DEL line (the next byte
            // there is '_', not ' '), so this finds the first re-list line
            const char *first_fn_line = strstr(out, PLUGINSD_KEYWORD_FUNCTION " GLOBAL");

            if(!del_line) {
                fprintf(stderr, "  FAILED drain: FUNCTION_DEL line missing from the rendered output\n");
                errors++;
            }
            if(!keep_line) {
                fprintf(stderr, "  FAILED drain: surviving function missing from the re-list\n");
                errors++;
            }
            if(del_line && first_fn_line && del_line > first_fn_line) {
                fprintf(stderr, "  FAILED drain: FUNCTION_DEL emitted after the re-list started\n");
                errors++;
            }
            if(rrdhost_flag_check(host, RRDHOST_FLAG_GLOBAL_FUNCTIONS_UPDATED)) {
                fprintf(stderr, "  FAILED drain: flag not cleared by the renderer\n");
                errors++;
            }
            if(fndel_queued(host, "tt-del-fn")) {
                fprintf(stderr, "  FAILED drain: queue not cleared after the drain\n");
                errors++;
            }
        }

        // discard: a parent without FUNCDEL support gets no DEL lines, and the
        // queue is still emptied (matches the old silent drop)
        rrd_function_del(host, NULL, "tt-keep-fn", RRD_FUNCTION_REG_SOURCE_INTERNAL);
        {
            CLEAN_BUFFER *wb = buffer_create(0, NULL);
            stream_sender_send_global_rrdhost_functions(host, wb, false, false);
            const char *out = buffer_tostring(wb);

            if(strstr(out, PLUGINSD_KEYWORD_FUNCTION_DEL)) {
                fprintf(stderr, "  FAILED discard: FUNCTION_DEL emitted despite can_function_del=false\n");
                errors++;
            }
            if(fndel_queued(host, "tt-keep-fn")) {
                fprintf(stderr, "  FAILED discard: queue not emptied on the discard path\n");
                errors++;
            }
        }

        if(errors == drain_errors_before)
            fprintf(stderr, "  OK drain: DEL-before-re-list ordering, flag/queue clearing, and the discard path\n");
    }

    // 3. concurrent del-during-drain: no DEL lost, none duplicated
    {
        int race_errors_before = errors;

        struct fndel_race_ctx ctx = { .host = host, .done = false };
        uint8_t *seen = callocz(FNDEL_RACE_N, sizeof(uint8_t));

        ND_THREAD *worker = nd_thread_create("fndel-race", NETDATA_THREAD_OPTION_DONT_LOG,
                                             fndel_race_worker, &ctx);
        if(!worker) {
            fprintf(stderr, "  FAILED race: could not create the worker thread\n");
            errors++;
        }
        else {
            bool worker_done = false;
            char needle[128];
            do {
                // one final drain AFTER the worker is done catches anything the
                // last mid-flight drain missed
                worker_done = __atomic_load_n(&ctx.done, __ATOMIC_ACQUIRE);

                CLEAN_BUFFER *wb = buffer_create(0, NULL);
                stream_sender_send_global_rrdhost_functions(host, wb, false, true);
                const char *out = buffer_tostring(wb);

                for(size_t i = 0; i < FNDEL_RACE_N; i++) {
                    snprintfz(needle, sizeof(needle),
                              PLUGINSD_KEYWORD_FUNCTION_DEL " GLOBAL \"tt-race-fn-%zu\"", i);
                    if(strstr(out, needle))
                        seen[i]++;
                }

                if(!worker_done)
                    sleep_usec(1 * USEC_PER_MS);
            } while(!worker_done);

            nd_thread_join(worker);

            size_t lost = 0, dup = 0;
            for(size_t i = 0; i < FNDEL_RACE_N; i++) {
                if(seen[i] == 0) lost++;
                else if(seen[i] > 1) dup++;
            }
            if(lost) {
                fprintf(stderr, "  FAILED race: %zu of %d FUNCTION_DELs were LOST\n", lost, FNDEL_RACE_N);
                errors++;
            }
            if(dup) {
                fprintf(stderr, "  FAILED race: %zu FUNCTION_DELs were emitted more than once\n", dup);
                errors++;
            }

            if(errors == race_errors_before)
                fprintf(stderr, "  OK race: %d concurrent deletes all drained exactly once\n", FNDEL_RACE_N);
        }

        freez(seen);
    }

    // 4. a queued FUNCTION_DEL survives a re-add, and one payload heals it:
    // the DEL renders first and the SAME payload's re-list re-affirms the
    // function, so the parent deletes-then-re-adds within one commit. The
    // re-add must NOT cancel the queued DEL - a cancellation can swallow the
    // only prune signal when a concurrent del outruns the add in the registry
    // (see the comment in rrd_function_add's global branch).
    {
        int readd_errors_before = errors;

        rrd_function_add(host, NULL, "tt-readd-fn", 10, 0, 1, "readd", "top",
                         HTTP_ACCESS_ANONYMOUS_DATA, true, RRD_FUNCTION_REG_SOURCE_INTERNAL,
                         rrdfunctions_unittest_noop_cb, NULL);
        rrd_function_del(host, NULL, "tt-readd-fn", RRD_FUNCTION_REG_SOURCE_INTERNAL);

        if(!fndel_queued(host, "tt-readd-fn")) {
            fprintf(stderr, "  FAILED re-add: del did not queue the FUNCTION_DEL\n");
            errors++;
        }

        rrd_function_add(host, NULL, "tt-readd-fn", 10, 0, 1, "readd", "top",
                         HTTP_ACCESS_ANONYMOUS_DATA, true, RRD_FUNCTION_REG_SOURCE_INTERNAL,
                         rrdfunctions_unittest_noop_cb, NULL);

        if(!fndel_queued(host, "tt-readd-fn")) {
            fprintf(stderr, "  FAILED re-add: the re-add cancelled the queued FUNCTION_DEL\n");
            errors++;
        }

        {
            CLEAN_BUFFER *wb = buffer_create(0, NULL);
            stream_sender_send_global_rrdhost_functions(host, wb, false, true);
            const char *out = buffer_tostring(wb);

            const char *del_line = strstr(out, PLUGINSD_KEYWORD_FUNCTION_DEL " GLOBAL \"tt-readd-fn\"");
            const char *fn_line = strstr(out, PLUGINSD_KEYWORD_FUNCTION " GLOBAL \"tt-readd-fn\"");

            if(!del_line) {
                fprintf(stderr, "  FAILED re-add: queued FUNCTION_DEL missing from the payload\n");
                errors++;
            }
            if(!fn_line) {
                fprintf(stderr, "  FAILED re-add: re-added function missing from the re-list\n");
                errors++;
            }
            if(del_line && fn_line && del_line > fn_line) {
                fprintf(stderr, "  FAILED re-add: FUNCTION_DEL rendered after the re-list line - the payload would not heal\n");
                errors++;
            }
            if(fndel_queued(host, "tt-readd-fn")) {
                fprintf(stderr, "  FAILED re-add: queue not drained by the renderer\n");
                errors++;
            }
        }

        rrd_function_del(host, NULL, "tt-readd-fn", RRD_FUNCTION_REG_SOURCE_INTERNAL);

        if(errors == readd_errors_before)
            fprintf(stderr, "  OK re-add: the queued FUNCTION_DEL survives and the payload heals (DEL before the re-affirming re-list)\n");
    }

    // restore localhost exactly as found
    host->sender = saved_sender;
    if(saved_option)
        rrdhost_option_set(host, RRDHOST_OPTION_SENDER_ENABLED);
    else
        rrdhost_option_clear(host, RRDHOST_OPTION_SENDER_ENABLED);
    rrdhost_flag_clear(host, RRDHOST_FLAG_GLOBAL_FUNCTIONS_UPDATED);
    fndel_flush_queue(host);
    aclk_send_timestamp_set(&cfg->node_manifest_send_time, 0);
    if(config_created_here)
        destroy_aclk_config(host);

    fprintf(stderr, "%s() %s (%d error%s)\n\n",
            __FUNCTION__, errors ? "FAILED" : "passed", errors, errors == 1 ? "" : "s");

    return errors;
}

// ----------------------------------------------------------------------------
// Golden-output test for the two streaming emitters (step 8 prerequisite).
//
// Pins the BYTES the post-C3 emitters produce for a controlled fixture set, so
// the C4 iteration-API rewrite can be verified to reproduce them identically.
// The expected lines are built here with their own copies of the format
// strings - not by calling the emitters - so any rewrite that changes the
// bytes fails this test. Covered: the FUNCTION_DEL-before-re-list order and
// the dyncfg FUNCDEL quirk (dyncfg deletes never emit FUNCTION_DEL), the
// dyncfg-count => dyncfg_add_streaming() synthetic "config" line (only when
// the parent supports DYNCFG), RESTRICTED functions DO stream, DYNCFG and
// LOCAL functions do not appear in the global list, and the chart emitter's
// exact full output.
//
// It also covers the OTHER consumers of the iteration API - the two JSON
// renderers and the two dictionary exporters - because they share the same
// visibility filter and the same "the view is a byte copy" contract, and
// because the api/v2 contexts exporter additionally owns a key format
// ("<version>|<name>") and an ownership rule (the destination dictionary frees
// the copies) that nothing else pins.

// mirrors struct function_v2_entry in api_v2_contexts.c: the destination
// dictionary owns the help/tags copies host_functions_to_dict() writes
struct e4_fn_entry {
    const char *help;
    const char *tags;
    HTTP_ACCESS access;
    int priority;
    uint32_t version;
};

static bool e4_fn_conflict_cb(const DICTIONARY_ITEM *item __maybe_unused, void *old_value __maybe_unused,
                              void *new_value, void *data __maybe_unused) {
    struct e4_fn_entry *n = new_value;
    freez((void *)n->help);
    freez((void *)n->tags);
    return false;
}

static void e4_fn_delete_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    struct e4_fn_entry *t = value;
    freez((void *)t->help);
    freez((void *)t->tags);
}

int rrdfunctions_emitters_unittest(void) {
    fprintf(stderr, "\n%s() running...\n", __FUNCTION__);

    RRDHOST *host = localhost;
    if(!host || !host->functions) {
        fprintf(stderr, "  FAILED: localhost (or its functions registry) is NULL\n");
        return 1;
    }

    int errors = 0;

    // fake a configured sender so deletes queue (see rrdfunctions_del_unittest)
    struct sender_state *saved_sender = host->sender;
    bool saved_option = rrdhost_option_check(host, RRDHOST_OPTION_SENDER_ENABLED);
    rrdhost_option_set(host, RRDHOST_OPTION_SENDER_ENABLED);
    host->sender = (struct sender_state *)(uintptr_t)0x1;
    fndel_flush_queue(host);

    // ------------------------------------------------------------------ fixtures
    // registered in FIXED order; the registry dict preserves insertion order,
    // so the fixture lines appear in this relative order in the output
    rrd_function_add(host, NULL, "c4-global-fn", 11, 42, 3, "c4 global help", "top",
                     HTTP_ACCESS_ANONYMOUS_DATA, true, RRD_FUNCTION_REG_SOURCE_INTERNAL,
                     rrdfunctions_unittest_noop_cb, NULL);
    rrd_function_add(host, NULL, "__c4-restricted-fn", 12, 43, 4, "c4 restricted", "top",
                     HTTP_ACCESS_NONE, true, RRD_FUNCTION_REG_SOURCE_INTERNAL,
                     rrdfunctions_unittest_noop_cb, NULL);
    rrd_function_add(host, NULL, "config c4test:job", 120, 1000, 1, "Dynamic configuration", "config",
                     HTTP_ACCESS_ANONYMOUS_DATA, true, RRD_FUNCTION_REG_SOURCE_INTERNAL,
                     rrdfunctions_unittest_noop_cb, NULL);

    // one plain global delete (queued) and one dyncfg delete (flag only - the quirk)
    rrd_function_add(host, NULL, "c4-deleted-fn", 14, 45, 6, "c4 deleted", "top",
                     HTTP_ACCESS_ANONYMOUS_DATA, true, RRD_FUNCTION_REG_SOURCE_INTERNAL,
                     rrdfunctions_unittest_noop_cb, NULL);
    rrd_function_add(host, NULL, "config c4del:job", 120, 1000, 1, "Dynamic configuration", "config",
                     HTTP_ACCESS_ANONYMOUS_DATA, true, RRD_FUNCTION_REG_SOURCE_INTERNAL,
                     rrdfunctions_unittest_noop_cb, NULL);
    rrd_function_del(host, NULL, "c4-deleted-fn", RRD_FUNCTION_REG_SOURCE_INTERNAL);
    rrd_function_del(host, NULL, "config c4del:job", RRD_FUNCTION_REG_SOURCE_INTERNAL);

    // a chart with one LOCAL function - the chart emitter's whole output
    RRDSET *st = rrdset_create_localhost("c4test", "c4chart", NULL, "c4fam", "c4test.ctx",
                                         "c4 title", "units", "c4plugin", "c4module",
                                         999999, 1, RRDSET_TYPE_LINE);
    if(!st) {
        fprintf(stderr, "  FAILED: could not create the fixture chart\n");
        errors++;
    }
    else
        rrd_function_add(host, st, "c4-chart-fn", 13, 44, 5, "c4 chart help", "top",
                         HTTP_ACCESS_ANONYMOUS_DATA, true, RRD_FUNCTION_REG_SOURCE_INTERNAL,
                         rrdfunctions_unittest_noop_cb, NULL);

    // ------------------------------------------------------- expected line bytes
    CLEAN_BUFFER *exp_global = buffer_create(0, NULL);
    buffer_sprintf(exp_global,
                   PLUGINSD_KEYWORD_FUNCTION " GLOBAL \"%s\" %d \"%s\" \"%s\" "HTTP_ACCESS_FORMAT" %d %"PRIu32"\n",
                   "c4-global-fn", 11, "c4 global help", "top",
                   (HTTP_ACCESS_FORMAT_CAST)HTTP_ACCESS_ANONYMOUS_DATA, 42, (uint32_t)3);

    CLEAN_BUFFER *exp_restricted = buffer_create(0, NULL);
    buffer_sprintf(exp_restricted,
                   PLUGINSD_KEYWORD_FUNCTION " GLOBAL \"%s\" %d \"%s\" \"%s\" "HTTP_ACCESS_FORMAT" %d %"PRIu32"\n",
                   "__c4-restricted-fn", 12, "c4 restricted", "top",
                   (HTTP_ACCESS_FORMAT_CAST)HTTP_ACCESS_NONE, 43, (uint32_t)4);

    CLEAN_BUFFER *exp_del = buffer_create(0, NULL);
    buffer_sprintf(exp_del, PLUGINSD_KEYWORD_FUNCTION_DEL " GLOBAL \"%s\"\n", "c4-deleted-fn");

    CLEAN_BUFFER *exp_dyncfg = buffer_create(0, NULL);
    buffer_sprintf(exp_dyncfg,
                   PLUGINSD_KEYWORD_FUNCTION " GLOBAL " PLUGINSD_FUNCTION_CONFIG " %d \"%s\" \"%s\" "HTTP_ACCESS_FORMAT" %d\n",
                   120, "Dynamic configuration", "config", (unsigned)HTTP_ACCESS_ANONYMOUS_DATA, 1000);

    CLEAN_BUFFER *exp_chart = buffer_create(0, NULL);
    buffer_sprintf(exp_chart,
                   PLUGINSD_KEYWORD_FUNCTION " \"%s\" %d \"%s\" \"%s\" "HTTP_ACCESS_FORMAT" %d %"PRIu32"\n",
                   "c4-chart-fn", 13, "c4 chart help", "top",
                   (HTTP_ACCESS_FORMAT_CAST)HTTP_ACCESS_ANONYMOUS_DATA, 44, (uint32_t)5);

    // ------------------------------------------------------------ global emitter
    {
        int before = errors;
        CLEAN_BUFFER *wb = buffer_create(0, NULL);
        stream_sender_send_global_rrdhost_functions(host, wb, true /* dyncfg */, true /* can_function_del */);
        const char *out = buffer_tostring(wb);

        const char *p_del        = strstr(out, buffer_tostring(exp_del));
        const char *p_global     = strstr(out, buffer_tostring(exp_global));
        const char *p_restricted = strstr(out, buffer_tostring(exp_restricted));
        const char *p_dyncfg     = strstr(out, buffer_tostring(exp_dyncfg));
        const char *p_first_fn   = strstr(out, PLUGINSD_KEYWORD_FUNCTION " ");

        if(!p_del)        { fprintf(stderr, "  FAILED global: FUNCTION_DEL line bytes not found\n"); errors++; }
        if(!p_global)     { fprintf(stderr, "  FAILED global: c4-global-fn line bytes not found\n"); errors++; }
        if(!p_restricted) { fprintf(stderr, "  FAILED global: restricted function did not stream\n"); errors++; }
        if(!p_dyncfg)     { fprintf(stderr, "  FAILED global: dyncfg_add_streaming synthetic line not found\n"); errors++; }
        if(strstr(out, "\"config c4test:job\"")) {
            fprintf(stderr, "  FAILED global: dyncfg function leaked into the global list\n"); errors++;
        }
        if(strstr(out, PLUGINSD_KEYWORD_FUNCTION_DEL " GLOBAL \"config c4del:job\"")) {
            fprintf(stderr, "  FAILED global: dyncfg delete emitted FUNCTION_DEL (quirk broken)\n"); errors++;
        }
        if(strstr(out, "\"c4-chart-fn\"")) {
            fprintf(stderr, "  FAILED global: LOCAL function leaked into the global list\n"); errors++;
        }
        if(p_del && p_first_fn && p_del > p_first_fn) {
            fprintf(stderr, "  FAILED global: FUNCTION_DEL did not precede the re-list\n"); errors++;
        }
        if(p_global && p_restricted && p_global > p_restricted) {
            fprintf(stderr, "  FAILED global: fixture lines out of registration order\n"); errors++;
        }

        if(errors == before)
            fprintf(stderr, "  OK global emitter: DEL-first order, exact line bytes, quirk, dyncfg count, filters\n");
    }

    // no-dyncfg-capability variant: the synthetic config line must be absent
    {
        int before = errors;
        CLEAN_BUFFER *wb = buffer_create(0, NULL);
        stream_sender_send_global_rrdhost_functions(host, wb, false /* dyncfg */, true);
        if(strstr(buffer_tostring(wb), buffer_tostring(exp_dyncfg))) {
            fprintf(stderr, "  FAILED global: synthetic config line emitted without DYNCFG capability\n");
            errors++;
        }
        if(errors == before)
            fprintf(stderr, "  OK global emitter: no synthetic config line without the capability\n");
    }

    // ------------------------------------------------------------- chart emitter
    if(st) {
        int before = errors;
        CLEAN_BUFFER *wb = buffer_create(0, NULL);
        stream_sender_send_rrdset_functions(st, wb);
        if(strcmp(buffer_tostring(wb), buffer_tostring(exp_chart)) != 0) {
            fprintf(stderr, "  FAILED chart: output is not byte-identical to the golden\n    expected: %s    got:      %s",
                    buffer_tostring(exp_chart), buffer_tostring(wb));
            errors++;
        }
        if(errors == before)
            fprintf(stderr, "  OK chart emitter: byte-identical output\n");
    }

    // --------------------------------------------------------------- JSON renderers
    // same EXPORTABLE filter as the manifest: dyncfg and restricted functions
    // must never reach a user-facing list. The host list is the whole registry,
    // so it DOES include the chart (LOCAL) functions, tagged as such.
    {
        int before = errors;
        CLEAN_BUFFER *wb = buffer_create(0, NULL);
        buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_DEFAULT);
        host_functions2json(host, wb);
        buffer_json_finalize(wb);
        const char *out = buffer_tostring(wb);

        if(!strstr(out, "\"c4-global-fn\"") || !strstr(out, "\"c4 global help\"")) {
            fprintf(stderr, "  FAILED host json: the global function or its help is missing\n"); errors++;
        }
        if(strstr(out, "\"__c4-restricted-fn\"")) {
            fprintf(stderr, "  FAILED host json: a restricted function was exported\n"); errors++;
        }
        if(strstr(out, "\"config c4test:job\"")) {
            fprintf(stderr, "  FAILED host json: a dyncfg function was exported\n"); errors++;
        }
        if(st && !strstr(out, "\"c4-chart-fn\"")) {
            fprintf(stderr, "  FAILED host json: chart functions must be part of the host list\n"); errors++;
        }
        if(!strstr(out, "\"LOCAL\"") || !strstr(out, "\"GLOBAL\"")) {
            fprintf(stderr, "  FAILED host json: the LOCAL/GLOBAL options array is not rendered\n"); errors++;
        }

        if(st) {
            CLEAN_BUFFER *cwb = buffer_create(0, NULL);
            buffer_json_initialize(cwb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_DEFAULT);
            buffer_json_member_add_object(cwb, "functions");
            chart_functions2json(st, cwb);
            buffer_json_object_close(cwb);
            buffer_json_finalize(cwb);
            const char *cout = buffer_tostring(cwb);

            if(!strstr(cout, "\"c4-chart-fn\"") || !strstr(cout, "\"c4 chart help\"")) {
                fprintf(stderr, "  FAILED chart json: the chart function or its help is missing\n"); errors++;
            }
            if(strstr(cout, "\"c4-global-fn\"")) {
                fprintf(stderr, "  FAILED chart json: a host-global function leaked into the chart list\n"); errors++;
            }
        }

        if(errors == before)
            fprintf(stderr, "  OK json renderers: host list includes LOCAL, excludes dyncfg/restricted; chart list is scoped\n");
    }

    // ----------------------------------------------------- dictionary exporters
    // host_functions_to_dict() is what api/v2 contexts aggregates by: the key
    // carries the version ("<version>|<name>") so two nodes offering different
    // versions of the same function do not merge, and help/tags are OWNED
    // copies the destination dictionary frees.
    {
        int before = errors;

        // exactly how api_v2_contexts.c creates ctl.functions.dict
        DICTIONARY *dst = dictionary_create_advanced(
            DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
            NULL, sizeof(struct e4_fn_entry));
        dictionary_register_conflict_callback(dst, e4_fn_conflict_cb, NULL);
        dictionary_register_delete_callback(dst, e4_fn_delete_cb, NULL);

        struct e4_fn_entry tmp = { 0 };
        host_functions_to_dict(host, dst, &tmp, sizeof(tmp),
                               &tmp.help, &tmp.tags, &tmp.access, &tmp.priority, &tmp.version);

        struct e4_fn_entry *e = dictionary_get(dst, "3|c4-global-fn");
        if(!e) {
            fprintf(stderr, "  FAILED to_dict: '3|c4-global-fn' is missing - the version|name key changed\n");
            errors++;
        }
        else {
            if(!e->help || strcmp(e->help, "c4 global help") != 0 || !e->tags || strcmp(e->tags, "top") != 0) {
                fprintf(stderr, "  FAILED to_dict: help/tags were not copied into the destination entry\n");
                errors++;
            }
            if(e->priority != 42 || e->version != 3 || e->access != HTTP_ACCESS_ANONYMOUS_DATA) {
                fprintf(stderr, "  FAILED to_dict: priority/version/access were not exported\n");
                errors++;
            }
        }
        if(dictionary_get(dst, "c4-global-fn")) {
            fprintf(stderr, "  FAILED to_dict: an unversioned key was produced\n");
            errors++;
        }
        if(dictionary_get(dst, "4|__c4-restricted-fn") || dictionary_get(dst, "1|config c4test:job")) {
            fprintf(stderr, "  FAILED to_dict: a restricted or dyncfg function was exported\n");
            errors++;
        }

        dictionary_destroy(dst);  // frees every copy; ASAN verifies the ownership rule

        if(st) {
            DICTIONARY *cdst = dictionary_create(DICT_OPTION_SINGLE_THREADED);
            chart_functions_to_dict(st, cdst, NULL, 0);

            // the entries carry no value, so membership is tested through the item
            const DICTIONARY_ITEM *ci = dictionary_get_and_acquire_item(cdst, "c4-chart-fn");
            if(!ci) {
                fprintf(stderr, "  FAILED to_dict: the chart function is missing from the chart dictionary\n");
                errors++;
            }
            else
                dictionary_acquired_item_release(cdst, ci);

            ci = dictionary_get_and_acquire_item(cdst, "c4-global-fn");
            if(ci) {
                fprintf(stderr, "  FAILED to_dict: a host-global function leaked into the chart dictionary\n");
                errors++;
                dictionary_acquired_item_release(cdst, ci);
            }
            dictionary_destroy(cdst);
        }

        if(errors == before)
            fprintf(stderr, "  OK dictionary exporters: version|name keys, owned copies, filters\n");
    }

    // --------------------------------------------------- the NULL-registry path
    // An archived host racing the sender's flag poll has no registry left. The
    // renderer must behave exactly like the old NULL-tolerant traversal: the
    // flag is cleared (so the poll does not spin) and nothing is emitted.
    {
        int before = errors;
        struct rrd_functions *saved_functions = host->functions;

        host->functions = NULL;
        rrdhost_flag_set(host, RRDHOST_FLAG_GLOBAL_FUNCTIONS_UPDATED);

        CLEAN_BUFFER *wb = buffer_create(0, NULL);
        stream_sender_send_global_rrdhost_functions(host, wb, true, true);

        if(rrdhost_flag_check(host, RRDHOST_FLAG_GLOBAL_FUNCTIONS_UPDATED)) {
            fprintf(stderr, "  FAILED null-registry: the flag was not cleared, the poll would spin forever\n");
            errors++;
        }
        if(buffer_strlen(wb)) {
            fprintf(stderr, "  FAILED null-registry: something was emitted for a host with no registry\n");
            errors++;
        }

        host->functions = saved_functions;

        if(errors == before)
            fprintf(stderr, "  OK null-registry: the flag is cleared and nothing is emitted\n");
    }

    // -------------------------------------------------------------------- cleanup
    if(st)
        rrd_function_del(host, st, "c4-chart-fn", RRD_FUNCTION_REG_SOURCE_INTERNAL);
    rrd_function_del(host, NULL, "c4-global-fn", RRD_FUNCTION_REG_SOURCE_INTERNAL);
    rrd_function_del(host, NULL, "__c4-restricted-fn", RRD_FUNCTION_REG_SOURCE_INTERNAL);
    rrd_function_del(host, NULL, "config c4test:job", RRD_FUNCTION_REG_SOURCE_INTERNAL);
    fndel_flush_queue(host);
    rrdhost_flag_clear(host, RRDHOST_FLAG_GLOBAL_FUNCTIONS_UPDATED);

    host->sender = saved_sender;
    if(saved_option)
        rrdhost_option_set(host, RRDHOST_OPTION_SENDER_ENABLED);
    else
        rrdhost_option_clear(host, RRDHOST_OPTION_SENDER_ENABLED);

    fprintf(stderr, "%s() %s (%d error%s)\n\n",
            __FUNCTION__, errors ? "FAILED" : "passed", errors, errors == 1 ? "" : "s");

    return errors;
}

// ----------------------------------------------------------------------------
// The registry itself (C1/C2): what may be registered, what may be deleted and
// by whom, what the conflict callback swaps, and how a provider (collector)
// that goes away takes its functions out of every view.
//
// These are the contracts every other consumer sits on top of, and none of
// them is observable from the streaming or manifest output the other suites
// pin:
//
//   1. the registry is indexed by the SANITIZED key - a name that sanitizes to
//      an empty string is refused outright (it could never be looked up, and
//      it would defeat the "__" restricted-prefix check), and everything else
//      lands on its collapsed key; lookups for execution strip trailing words,
//      lookups for availability do not;
//   2. the dyncfg namespace is per-SOURCE: STREAMING (a child's synthetic
//      "config" proxy) and INTERNAL (the dyncfg subsystem) may register
//      reserved names, a COLLECTOR may not - and a rejected COLLECTOR
//      registration must not disturb the entry that is already there;
//   3. deletion is gated the same way: FUNCTION_DEL (COLLECTOR/STREAMING) can
//      never remove a dyncfg entry, and a COLLECTOR may only remove what its
//      OWN provider registered;
//   4. the conflict callback swaps every changed field of an existing entry -
//      including options, which are derived from the tags, so a
//      re-registration that adds the "hidden" tag actually restricts the
//      function (and removing it un-restricts it);
//   5. a function whose provider stopped stays in the registry but disappears
//      from every view and answers 503;
//   6. help/tags are handed to consumers as byte copies taken under the
//      entry's leaf lock, so a concurrent re-registration can never hand out a
//      mixed or freed pair.

struct reg_probe {
    const char *want;
    bool found;
    size_t seen;
    char help[256];
    char tags[256];
    int timeout;
    int priority;
    uint32_t version;
    HTTP_ACCESS access;
    RRD_FUNCTION_OPTIONS options;
};

static void reg_probe_cb(const struct rrd_function_view *v, void *data) {
    struct reg_probe *p = data;
    if(!v->name || strcmp(v->name, p->want) != 0) return;

    p->found = true;
    p->seen++;
    strncpyz(p->help, v->help ? v->help : "", sizeof(p->help) - 1);
    strncpyz(p->tags, v->tags ? v->tags : "", sizeof(p->tags) - 1);
    p->timeout = v->timeout;
    p->priority = v->priority;
    p->version = v->version;
    p->access = v->access;
    p->options = v->options;
}

static bool reg_probe_run(RRDHOST *host, RRD_FUNCTIONS_FILTER filter, const char *name, struct reg_probe *out) {
    memset(out, 0, sizeof(*out));
    out->want = name;
    rrd_functions_host_foreach(host, filter, reg_probe_cb, out);
    return out->found;
}

static size_t reg_cb_a_calls = 0, reg_cb_b_calls = 0;

static int reg_execute_cb_a(struct rrd_function_execute *rfe, void *data __maybe_unused) {
    reg_cb_a_calls++;
    if(rfe->result.cb)
        rfe->result.cb(rfe->result.wb, HTTP_RESP_OK, rfe->result.data);
    return HTTP_RESP_OK;
}

static int reg_execute_cb_b(struct rrd_function_execute *rfe, void *data __maybe_unused) {
    reg_cb_b_calls++;
    if(rfe->result.cb)
        rfe->result.cb(rfe->result.wb, HTTP_RESP_OK, rfe->result.data);
    return HTTP_RESP_OK;
}

// a delete attempted from a thread that never registered anything, so its
// nrpc_thread_serving is NULL - the COLLECTOR ownership check must
// refuse it
struct reg_del_ctx {
    RRDHOST *host;
    const char *name;
    bool collector_del;
    bool internal_del;
};

static void reg_del_worker(void *arg) {
    struct reg_del_ctx *c = arg;
    c->collector_del = rrd_function_del(c->host, NULL, c->name, RRD_FUNCTION_REG_SOURCE_COLLECTOR);
}

// registers a function and then ends its provider, exactly like a plugin
// thread that exits while its functions are still in the registry
static void reg_provider_worker(void *arg) {
    RRDHOST *host = arg;
    nrpc_serving_started();
    rrd_function_add(host, NULL, "reg-provider-fn", 10, 0, 1, "provider", "top",
                     HTTP_ACCESS_ANONYMOUS_DATA, true, RRD_FUNCTION_REG_SOURCE_INTERNAL,
                     rrdfunctions_unittest_noop_cb, NULL);
    nrpc_serving_finished();
}

#define REG_RACE_N 20000
#define REG_RACE_HELP_A "reg race help A"
#define REG_RACE_TAGS_A "top"
#define REG_RACE_HELP_B "reg race help B, deliberately much longer than A"
#define REG_RACE_TAGS_B "logs troubleshooting"

struct reg_race_ctx {
    RRDHOST *host;
    bool done;
};

static void reg_race_worker(void *arg) {
    struct reg_race_ctx *c = arg;
    for(size_t i = 0; i < REG_RACE_N; i++)
        rrd_function_add(c->host, NULL, "reg-race-fn", 10, 0, 1,
                         (i & 1) ? REG_RACE_HELP_A : REG_RACE_HELP_B,
                         (i & 1) ? REG_RACE_TAGS_A : REG_RACE_TAGS_B,
                         HTTP_ACCESS_ANONYMOUS_DATA, true, RRD_FUNCTION_REG_SOURCE_INTERNAL,
                         rrdfunctions_unittest_noop_cb, NULL);

    __atomic_store_n(&c->done, true, __ATOMIC_RELEASE);
}

struct reg_race_check {
    size_t observations;
    size_t mismatches;
};

static void reg_race_check_cb(const struct rrd_function_view *v, void *data) {
    struct reg_race_check *k = data;
    if(!v->name || strcmp(v->name, "reg-race-fn") != 0) return;

    k->observations++;

    // help and tags are swapped as a pair under the entry's leaf lock and
    // copied under that same lock, so a consumer must never see one field of
    // one registration next to the other field of the other one (and must
    // never see freed bytes - that half is what ASAN builds catch here)
    bool a = (strcmp(v->help, REG_RACE_HELP_A) == 0 && strcmp(v->tags, REG_RACE_TAGS_A) == 0);
    bool b = (strcmp(v->help, REG_RACE_HELP_B) == 0 && strcmp(v->tags, REG_RACE_TAGS_B) == 0);
    if(!a && !b)
        k->mismatches++;
}

int rrdfunctions_registry_unittest(void) {
    fprintf(stderr, "\n%s() running...\n", __FUNCTION__);

    RRDHOST *host = localhost;
    if(!host || !host->functions) {
        fprintf(stderr, "  FAILED: localhost (or its functions registry) is NULL\n");
        return 1;
    }

    int errors = 0;

    // the standalone -W environment does not start the daemon, so the
    // transaction broker the run paths below need is not there yet (create is
    // idempotent)
    rrd_function_transactions_create();

    // 1. the key is the sanitized name; a name that sanitizes to empty is refused
    {
        int before = errors;

        // text_sanitize() wipes an all-underscore result and strips leading
        // whitespace/control characters, so all of these sanitize to ""
        const char *empty_names[] = { "__", "____", "   ", "\t\n\r", "\x01\x02" };
        size_t entries_before = dictionary_entries(host->functions->dict);

        for(size_t i = 0; i < _countof(empty_names); i++) {
            rrdhost_flag_clear(host, RRDHOST_FLAG_GLOBAL_FUNCTIONS_UPDATED);

            rrd_function_add(host, NULL, empty_names[i], 10, 0, 1, "empty", "top",
                             HTTP_ACCESS_ANONYMOUS_DATA, true, RRD_FUNCTION_REG_SOURCE_INTERNAL,
                             rrdfunctions_unittest_noop_cb, NULL);

            // the refusal happens before anything is announced
            if(rrdhost_flag_check(host, RRDHOST_FLAG_GLOBAL_FUNCTIONS_UPDATED)) {
                fprintf(stderr, "  FAILED key: a refused registration set the global-functions flag\n");
                errors++;
            }
        }

        if(dictionary_entries(host->functions->dict) != entries_before) {
            fprintf(stderr, "  FAILED key: a name that sanitizes to an empty string created an entry\n");
            errors++;
        }
        if(dictionary_get(host->functions->dict, "")) {
            fprintf(stderr, "  FAILED key: an empty key is present in the registry\n");
            errors++;
        }

        // everything else lands on its collapsed key: leading/trailing spaces
        // dropped, runs of whitespace folded to one, '"' mapped to '\''
        rrd_function_add(host, NULL, "  reg-key   edge\"case  ", 10, 0, 1, "key", "top",
                         HTTP_ACCESS_ANONYMOUS_DATA, true, RRD_FUNCTION_REG_SOURCE_INTERNAL,
                         rrdfunctions_unittest_noop_cb, NULL);

        if(!dictionary_get(host->functions->dict, "reg-key edge'case")) {
            fprintf(stderr, "  FAILED key: a name was not indexed by its sanitized form\n");
            errors++;
        }

        // execution lookups strip words from the end until they hit a
        // registered function, so arguments resolve to their function...
        rrd_function_add(host, NULL, "reg-args-fn", 10, 0, 1, "args", "top",
                         HTTP_ACCESS_ANONYMOUS_DATA, true, RRD_FUNCTION_REG_SOURCE_INTERNAL,
                         rrdfunctions_unittest_noop_cb, NULL);
        {
            CLEAN_BUFFER *wb = buffer_create(0, NULL);
            RRD_FUNCTION_ACQUIRED *item = NULL;
            int code = rrd_function_verify_access(host, wb, "reg-args-fn arg1 arg2",
                                                  HTTP_ACCESS_ANONYMOUS_DATA, false, &item);
            if(code != HTTP_RESP_OK || !item) {
                fprintf(stderr, "  FAILED key: '<function> <args>' did not resolve to the function (code %d)\n", code);
                errors++;
            }
            if(item)
                rrd_function_acquired_release(host, item);
        }

        // ... while the availability probe is an exact-name lookup
        if(!rrd_function_available(host, "reg-args-fn")) {
            fprintf(stderr, "  FAILED key: rrd_function_available() missed a live function\n");
            errors++;
        }
        if(rrd_function_available(host, "reg-args-fn arg1")) {
            fprintf(stderr, "  FAILED key: rrd_function_available() must not strip arguments\n");
            errors++;
        }

        rrd_function_del(host, NULL, "  reg-key   edge\"case  ", RRD_FUNCTION_REG_SOURCE_INTERNAL);
        rrd_function_del(host, NULL, "reg-args-fn", RRD_FUNCTION_REG_SOURCE_INTERNAL);

        if(errors == before)
            fprintf(stderr, "  OK key: empty-sanitizing names refused, sanitized indexing, argument stripping\n");
    }

    // 2. the dyncfg namespace is per registration source
    {
        int before = errors;
        const char *fn = "config reg-stream:job";

        // a streaming child's synthetic config proxy: allowed, and classified
        // as DYNCFG from the sanitized key
        rrd_function_add(host, NULL, fn, 10, 0, 1, "streamed config", "config",
                         HTTP_ACCESS_ANONYMOUS_DATA, true, RRD_FUNCTION_REG_SOURCE_STREAMING,
                         reg_execute_cb_a, NULL);

        struct rrd_host_function *entry = dictionary_get(host->functions->dict, fn);
        if(!entry) {
            fprintf(stderr, "  FAILED namespace: a STREAMING registration of a reserved name was rejected\n");
            errors++;
        }
        else {
            if(!(entry->options & RRD_FUNCTION_DYNCFG)) {
                fprintf(stderr, "  FAILED namespace: a reserved name was not classified as DYNCFG\n");
                errors++;
            }

            // a collector cannot take it over - the rejection must not reach
            // the conflict callback, so the installed execute callback stays
            rrd_function_add(host, NULL, fn, 10, 0, 1, "hijack", "config",
                             HTTP_ACCESS_ANONYMOUS_DATA, true, RRD_FUNCTION_REG_SOURCE_COLLECTOR,
                             reg_execute_cb_b, NULL);

            struct rrd_host_function *after = dictionary_get(host->functions->dict, fn);
            if(!after || after->execute_cb != reg_execute_cb_a) {
                fprintf(stderr, "  FAILED namespace: a COLLECTOR registration hijacked a reserved name\n");
                errors++;
            }
        }

        // 3. FUNCTION_DEL can never remove a dyncfg entry
        if(rrd_function_del(host, NULL, fn, RRD_FUNCTION_REG_SOURCE_STREAMING)) {
            fprintf(stderr, "  FAILED namespace: STREAMING FUNCTION_DEL removed a dyncfg function\n");
            errors++;
        }
        if(rrd_function_del(host, NULL, fn, RRD_FUNCTION_REG_SOURCE_COLLECTOR)) {
            fprintf(stderr, "  FAILED namespace: COLLECTOR FUNCTION_DEL removed a dyncfg function\n");
            errors++;
        }
        if(!dictionary_get(host->functions->dict, fn)) {
            fprintf(stderr, "  FAILED namespace: a refused FUNCTION_DEL removed the entry anyway\n");
            errors++;
        }
        if(!rrd_function_del(host, NULL, fn, RRD_FUNCTION_REG_SOURCE_INTERNAL)) {
            fprintf(stderr, "  FAILED namespace: the dyncfg subsystem could not remove its own function\n");
            errors++;
        }

        if(errors == before)
            fprintf(stderr, "  OK namespace: STREAMING/INTERNAL own the dyncfg namespace, COLLECTOR is locked out\n");
    }

    // 3b. deleting what is not there, and deleting what another collector owns
    {
        int before = errors;

        if(rrd_function_del(host, NULL, "reg-does-not-exist-fn", RRD_FUNCTION_REG_SOURCE_INTERNAL)) {
            fprintf(stderr, "  FAILED del: deleting an unknown function reported success\n");
            errors++;
        }
        if(rrd_function_del(host, NULL, "", RRD_FUNCTION_REG_SOURCE_INTERNAL) ||
           rrd_function_del(host, NULL, NULL, RRD_FUNCTION_REG_SOURCE_INTERNAL)) {
            fprintf(stderr, "  FAILED del: deleting an empty name reported success\n");
            errors++;
        }

        rrd_function_add(host, NULL, "reg-owned-fn", 10, 0, 1, "owned", "top",
                         HTTP_ACCESS_ANONYMOUS_DATA, true, RRD_FUNCTION_REG_SOURCE_INTERNAL,
                         rrdfunctions_unittest_noop_cb, NULL);

        struct reg_del_ctx ctx = { .host = host, .name = "reg-owned-fn" };
        ND_THREAD *t = nd_thread_create("reg-del", NETDATA_THREAD_OPTION_DONT_LOG, reg_del_worker, &ctx);
        if(!t) {
            fprintf(stderr, "  FAILED del: could not create the worker thread\n");
            errors++;
        }
        else {
            nd_thread_join(t);

            if(ctx.collector_del) {
                fprintf(stderr, "  FAILED del: a foreign collector removed a function it did not register\n");
                errors++;
            }
            if(!dictionary_get(host->functions->dict, "reg-owned-fn")) {
                fprintf(stderr, "  FAILED del: a refused COLLECTOR delete removed the entry anyway\n");
                errors++;
            }
        }

        // the registering thread (this one) may remove it as a COLLECTOR
        if(!rrd_function_del(host, NULL, "reg-owned-fn", RRD_FUNCTION_REG_SOURCE_COLLECTOR)) {
            fprintf(stderr, "  FAILED del: the registering collector could not remove its own function\n");
            errors++;
        }

        if(errors == before)
            fprintf(stderr, "  OK del: unknown/empty names, and COLLECTOR deletes are scoped to the registering provider\n");
    }

    // 4. the conflict callback swaps every changed field - including the
    //    options derived from the tags
    {
        int before = errors;
        const char *fn = "reg-swap-fn";
        struct reg_probe p;

        rrd_function_add(host, NULL, fn, 11, 42, 3, "swap help one", "top",
                         HTTP_ACCESS_ANONYMOUS_DATA, true, RRD_FUNCTION_REG_SOURCE_INTERNAL,
                         reg_execute_cb_a, NULL);

        if(!reg_probe_run(host, RRD_FUNCTIONS_FILTER_EXPORTABLE, fn, &p)) {
            fprintf(stderr, "  FAILED swap: the function is not visible after registration\n");
            errors++;
        }
        else if(strcmp(p.help, "swap help one") != 0 || strcmp(p.tags, "top") != 0 ||
                p.timeout != 11 || p.priority != 42 || p.version != 3 ||
                p.access != HTTP_ACCESS_ANONYMOUS_DATA ||
                (p.options & RRD_FUNCTION_RESTRICTED) || !(p.options & RRD_FUNCTION_GLOBAL)) {
            fprintf(stderr, "  FAILED swap: the initial registration is not reported as registered\n");
            errors++;
        }

        size_t a_before = reg_cb_a_calls, b_before = reg_cb_b_calls;
        {
            CLEAN_BUFFER *wb = buffer_create(0, NULL);
            rrd_function_run(host, wb, 10, HTTP_ACCESS_ALL, fn, true, NULL,
                             NULL, NULL, NULL, NULL, NULL, NULL, NULL, "unittest", false);
        }
        if(reg_cb_a_calls != a_before + 1 || reg_cb_b_calls != b_before) {
            fprintf(stderr, "  FAILED swap: the registered execute callback did not run\n");
            errors++;
        }

        // re-registration: every field changes, and the "hidden" tag must
        // actually restrict the function (options are derived from the tags
        // and swapped with them)
        rrd_function_add(host, NULL, fn, 22, 43, 4, "swap help two", "logs " RRDFUNCTIONS_TAG_HIDDEN,
                         HTTP_ACCESS_SIGNED_ID, true, RRD_FUNCTION_REG_SOURCE_INTERNAL,
                         reg_execute_cb_b, NULL);

        if(reg_probe_run(host, RRD_FUNCTIONS_FILTER_EXPORTABLE, fn, &p)) {
            fprintf(stderr, "  FAILED swap: a re-registration with the hidden tag stayed user-visible\n");
            errors++;
        }
        // restricted functions still stream to a parent
        if(!reg_probe_run(host, RRD_FUNCTIONS_FILTER_STREAMABLE_GLOBAL, fn, &p)) {
            fprintf(stderr, "  FAILED swap: a restricted function stopped streaming\n");
            errors++;
        }
        else if(strcmp(p.help, "swap help two") != 0 ||
                strcmp(p.tags, "logs " RRDFUNCTIONS_TAG_HIDDEN) != 0 ||
                p.timeout != 22 || p.priority != 43 || p.version != 4 ||
                p.access != HTTP_ACCESS_SIGNED_ID ||
                !(p.options & RRD_FUNCTION_RESTRICTED)) {
            fprintf(stderr, "  FAILED swap: help=%s tags=%s timeout=%d priority=%d version=%u access=0x%x options=0x%x"
                            " - a field was not swapped\n",
                    p.help, p.tags, p.timeout, p.priority, p.version, (unsigned)p.access, (unsigned)p.options);
            errors++;
        }

        a_before = reg_cb_a_calls; b_before = reg_cb_b_calls;
        {
            CLEAN_BUFFER *wb = buffer_create(0, NULL);
            rrd_function_run(host, wb, 10, HTTP_ACCESS_ALL, fn, true, NULL,
                             NULL, NULL, NULL, NULL, NULL, NULL, NULL, "unittest", true /* allow restricted */);
        }
        if(reg_cb_b_calls != b_before + 1 || reg_cb_a_calls != a_before) {
            fprintf(stderr, "  FAILED swap: the swapped execute callback did not take over\n");
            errors++;
        }

        // and back: dropping the tag un-restricts it
        rrd_function_add(host, NULL, fn, 11, 42, 3, "swap help one", "top",
                         HTTP_ACCESS_ANONYMOUS_DATA, true, RRD_FUNCTION_REG_SOURCE_INTERNAL,
                         reg_execute_cb_a, NULL);

        if(!reg_probe_run(host, RRD_FUNCTIONS_FILTER_EXPORTABLE, fn, &p)) {
            fprintf(stderr, "  FAILED swap: dropping the hidden tag did not un-restrict the function\n");
            errors++;
        }
        else if(p.options & RRD_FUNCTION_RESTRICTED) {
            fprintf(stderr, "  FAILED swap: the RESTRICTED option survived the tag removal\n");
            errors++;
        }

        // a registration without tags gets the default "top"
        rrd_function_add(host, NULL, "reg-notags-fn", 10, 0, 1, "no tags", NULL,
                         HTTP_ACCESS_ANONYMOUS_DATA, true, RRD_FUNCTION_REG_SOURCE_INTERNAL,
                         rrdfunctions_unittest_noop_cb, NULL);
        if(!reg_probe_run(host, RRD_FUNCTIONS_FILTER_EXPORTABLE, "reg-notags-fn", &p) ||
           strcmp(p.tags, "top") != 0) {
            fprintf(stderr, "  FAILED swap: a registration without tags did not default to 'top'\n");
            errors++;
        }

        rrd_function_del(host, NULL, "reg-notags-fn", RRD_FUNCTION_REG_SOURCE_INTERNAL);
        rrd_function_del(host, NULL, fn, RRD_FUNCTION_REG_SOURCE_INTERNAL);

        if(errors == before)
            fprintf(stderr, "  OK swap: every field swaps, tags drive the RESTRICTED option both ways, cb takes over\n");
    }

    // 5. a provider that stopped takes its functions out of every view, but
    //    the entries stay in the registry until they are deleted
    {
        int before = errors;

        ND_THREAD *t = nd_thread_create("reg-provider", NETDATA_THREAD_OPTION_DONT_LOG, reg_provider_worker, host);
        if(!t) {
            fprintf(stderr, "  FAILED provider: could not create the worker thread\n");
            errors++;
        }
        else {
            nd_thread_join(t);

            struct reg_probe p;

            if(!dictionary_get(host->functions->dict, "reg-provider-fn")) {
                fprintf(stderr, "  FAILED provider: the entry was removed when its provider stopped\n");
                errors++;
            }
            if(rrd_function_available(host, "reg-provider-fn")) {
                fprintf(stderr, "  FAILED provider: a function of a stopped provider is still available\n");
                errors++;
            }
            if(reg_probe_run(host, RRD_FUNCTIONS_FILTER_EXPORTABLE, "reg-provider-fn", &p) ||
               reg_probe_run(host, RRD_FUNCTIONS_FILTER_STREAMABLE_GLOBAL, "reg-provider-fn", &p)) {
                fprintf(stderr, "  FAILED provider: a function of a stopped provider is still exported\n");
                errors++;
            }

            CLEAN_BUFFER *wb = buffer_create(0, NULL);
            int code = rrd_function_run(host, wb, 10, HTTP_ACCESS_ALL, "reg-provider-fn", true, NULL,
                                        NULL, NULL, NULL, NULL, NULL, NULL, NULL, "unittest", false);
            if(code != HTTP_RESP_SERVICE_UNAVAILABLE) {
                fprintf(stderr, "  FAILED provider: running it returned %d, expected 503\n", code);
                errors++;
            }
            if(!strstr(buffer_tostring(wb), "not currently running")) {
                fprintf(stderr, "  FAILED provider: the 503 does not say the plugin is not running\n");
                errors++;
            }

            // the provider structure itself is released only when the last
            // entry referencing it goes away (ASAN verifies there is no leak
            // and no use-after-free here)
            if(!rrd_function_del(host, NULL, "reg-provider-fn", RRD_FUNCTION_REG_SOURCE_INTERNAL)) {
                fprintf(stderr, "  FAILED provider: could not delete the orphaned function\n");
                errors++;
            }
        }

        if(errors == before)
            fprintf(stderr, "  OK provider: a stopped provider hides its functions everywhere and answers 503\n");
    }

    // 6. help/tags are byte copies taken under the entry's leaf lock: a
    //    consumer iterating while another thread re-registers must never see a
    //    mixed pair (nor freed bytes)
    {
        int before = errors;
        struct reg_race_ctx ctx = { .host = host, .done = false };
        struct reg_race_check check = { 0 };

        rrd_function_add(host, NULL, "reg-race-fn", 10, 0, 1, REG_RACE_HELP_A, REG_RACE_TAGS_A,
                         HTTP_ACCESS_ANONYMOUS_DATA, true, RRD_FUNCTION_REG_SOURCE_INTERNAL,
                         rrdfunctions_unittest_noop_cb, NULL);

        ND_THREAD *t = nd_thread_create("reg-race", NETDATA_THREAD_OPTION_DONT_LOG, reg_race_worker, &ctx);
        if(!t) {
            fprintf(stderr, "  FAILED race: could not create the worker thread\n");
            errors++;
        }
        else {
            while(!__atomic_load_n(&ctx.done, __ATOMIC_ACQUIRE))
                rrd_functions_host_foreach(host, RRD_FUNCTIONS_FILTER_EXPORTABLE, reg_race_check_cb, &check);

            nd_thread_join(t);

            if(!check.observations) {
                fprintf(stderr, "  FAILED race: the function was never observed during the race\n");
                errors++;
            }
            if(check.mismatches) {
                fprintf(stderr, "  FAILED race: %zu of %zu observations saw a mixed help/tags pair\n",
                        check.mismatches, check.observations);
                errors++;
            }
        }

        rrd_function_del(host, NULL, "reg-race-fn", RRD_FUNCTION_REG_SOURCE_INTERNAL);

        if(errors == before)
            fprintf(stderr, "  OK race: %zu consistent help/tags observations against %d re-registrations\n",
                    check.observations, REG_RACE_N);
    }

    // leave localhost as found: every fixture above was deleted in its own
    // section, so only the flag and (if this binary ever runs with a sender
    // configured) the pending-del queue our deletes may have filled are left
    rrdhost_flag_clear(host, RRDHOST_FLAG_GLOBAL_FUNCTIONS_UPDATED);
    fndel_flush_queue(host);

    fprintf(stderr, "%s() %s (%d error%s)\n\n",
            __FUNCTION__, errors ? "FAILED" : "passed", errors, errors == 1 ? "" : "s");

    return errors;
}
