// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrd.h"
#include "rrdfunctions-internals.h"
#include "sqlite/sqlite_aclk_node.h"

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
                     true, rrdfunctions_unittest_noop_cb, NULL);

    // A restricted function: name starting with "__" flags RRD_FUNCTION_RESTRICTED.
    rrd_function_add(host, NULL, "__restricted-fn", 10, 0, 1, "restricted", "top",
                     HTTP_ACCESS_NONE, true, rrdfunctions_unittest_noop_cb, NULL);

    // A public function requiring nothing — baseline that the gate does not over-block.
    rrd_function_add(host, NULL, "public-fn", 10, 0, 1, "public", "top",
                     HTTP_ACCESS_NONE, true, rrdfunctions_unittest_noop_cb, NULL);

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
        const DICTIONARY_ITEM *item = (const DICTIONARY_ITEM *)(uintptr_t)0x1; // poison: verify it is reset

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
            dictionary_acquired_item_release(cases[i].host->functions, item);

        if(ok && item_ok && body_ok)
            fprintf(stderr, "  OK case %zu: %s (access 0x%x, allow_restricted=%d) -> %d\n",
                    i, cases[i].fn, (unsigned)cases[i].user_access, cases[i].allow_restricted, code);
    }

    rrd_function_del(host, NULL, "protected-fn", false, true);
    rrd_function_del(host, NULL, "__restricted-fn", false, true);
    rrd_function_del(host, NULL, "public-fn", false, true);

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
                         HTTP_ACCESS_ANONYMOUS_DATA, true, rrdfunctions_unittest_noop_cb, NULL);

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
        rrd_function_del(host, NULL, fns[i].name, false, true);

    // 4. The suppression hash. build_node_manifest() publishes only when manifest_dict_hash()
    //    differs from the last sent hash, so the hash must be: deterministic for identical
    //    content, independent of dictionary traversal order, and sensitive to every transmitted
    //    field including the node_id and claim_id the manifest is keyed under at the cloud. Local
    //    standalone dictionaries keep this independent of whatever live functions localhost has.
    //    The other half of the suppression - scoping the record to one ACLK session - lives in
    //    build_node_manifest(), not in the hash, so it is not covered here.
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

        if(errors == hash_errors_before)
            fprintf(stderr, "  OK hash: determinism, order independence, field, node_id and claim_id sensitivity\n");

        dictionary_destroy(a);
        dictionary_destroy(b);
        dictionary_destroy(empty);
        dictionary_destroy(c1);
        dictionary_destroy(c2);
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
