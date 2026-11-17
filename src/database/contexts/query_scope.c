// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdcontext-internal.h"

typedef enum {
    QUERY_SCOPE_HOST_OUT_OF_SCOPE = 0,
    QUERY_SCOPE_HOST_IN_SCOPE,          // in scope, but not selected by the hosts pattern
    QUERY_SCOPE_HOST_QUERYABLE,
} QUERY_SCOPE_HOST_MATCH;

// A node filter (scope_nodes or nodes) is often a long list of exact node ids: Cloud sends every node of the room
// on every query. Walking that list for every host and identifier costs hosts x entries compares, so a long list of
// exact strings is matched through a hash instead, when the agent has enough hosts to repay building it. Any other
// pattern is matched as before.
#define QUERY_SCOPE_HOSTS_SET_MIN_ENTRIES 12
#define QUERY_SCOPE_HOSTS_SET_MIN_HOSTS 32

typedef struct {
    SIMPLE_PATTERN *sp;
    Pvoid_t set;            // JudyHS of the entries of sp, when sp is a long list of exact strings
} QUERY_SCOPE_HOSTS_FILTER;

// hosts: the number of hosts the walk will match against the filter
static void query_scope_hosts_filter_init(QUERY_SCOPE_HOSTS_FILTER *f, SIMPLE_PATTERN *sp, size_t hosts) {
    f->sp = sp;
    f->set = NULL;

    if(hosts < QUERY_SCOPE_HOSTS_SET_MIN_HOSTS || simple_pattern_exact_entries(sp) < QUERY_SCOPE_HOSTS_SET_MIN_ENTRIES)
        return;

    SIMPLE_PATTERN *it = sp;
    const char *s;
    while(it && (s = simple_pattern_iterate(&it))) {
        Pvoid_t *PValue = JudyHSIns(&f->set, (void *)s, strlen(s), PJE0);
        if(unlikely(!PValue || PValue == PJERR)) {
            // never match through a partial set: fall back to the pattern
            JudyHSFreeArray(&f->set, PJE0);
            f->set = NULL;
            return;
        }

        *PValue = (void *)1;
    }
}

static void query_scope_hosts_filter_cleanup(QUERY_SCOPE_HOSTS_FILTER *f) {
    if(f->set)
        JudyHSFreeArray(&f->set, PJE0);

    f->set = NULL;
    f->sp = NULL;
}

// The entries in the set are not empty, so an empty or missing string never matches, as with the pattern.
static inline SIMPLE_PATTERN_RESULT query_scope_hosts_filter_set_get(QUERY_SCOPE_HOSTS_FILTER *f, const char *s, size_t len) {
    if(!s || !len)
        return SP_NOT_MATCHED;

    return JudyHSGet(f->set, (void *)s, len) ? SP_MATCHED_POSITIVE : SP_NOT_MATCHED;
}

static inline SIMPLE_PATTERN_RESULT query_scope_hosts_filter_match_string(QUERY_SCOPE_HOSTS_FILTER *f, STRING *str) {
    if(!f->set)
        return simple_pattern_matches_string_extract(f->sp, str, NULL, 0);

    return str ? query_scope_hosts_filter_set_get(f, string2str(str), string_strlen(str)) : SP_NOT_MATCHED;
}

static inline SIMPLE_PATTERN_RESULT query_scope_hosts_filter_match(QUERY_SCOPE_HOSTS_FILTER *f, const char *s) {
    if(!f->set)
        return simple_pattern_matches_extract(f->sp, s, NULL, 0);

    return query_scope_hosts_filter_set_get(f, s, s ? strlen(s) : 0);
}

// Tries the hostname, then the machine guid, then the node id (when the host has one); the first that matches,
// positively or negatively, decides.
static SIMPLE_PATTERN_RESULT query_scope_hosts_filter_match_identity(QUERY_SCOPE_HOSTS_FILTER *f, STRING *hostname,
                                                                     const char *machine_guid, const char *node_id_str) {
    SIMPLE_PATTERN_RESULT match = query_scope_hosts_filter_match_string(f, hostname);
    if(match == SP_NOT_MATCHED) {
        match = query_scope_hosts_filter_match(f, machine_guid);
        if(match == SP_NOT_MATCHED && *node_id_str)
            match = query_scope_hosts_filter_match(f, node_id_str);
    }

    return match;
}

// Matches a host by hostname, machine guid or node id. Fills host_node_id_str with the host's node id.
static QUERY_SCOPE_HOST_MATCH query_scope_host_match(RRDHOST *host, QUERY_SCOPE_HOSTS_FILTER *scope_hosts, QUERY_SCOPE_HOSTS_FILTER *hosts,
                                                     char *host_node_id_str) {
    RRDHOST_IDENTITY identity = { 0 };

    if(scope_hosts->sp || hosts->sp)
        identity = rrdhost_identity_acquire(host);

    if(!UUIDiszero(host->node_id))
        uuid_unparse_lower(host->node_id.uuid, host_node_id_str);
    else
        host_node_id_str[0] = '\0';

    SIMPLE_PATTERN_RESULT match = SP_MATCHED_POSITIVE;
    if(scope_hosts->sp)
        match = query_scope_hosts_filter_match_identity(scope_hosts, identity.hostname, host->machine_guid, host_node_id_str);

    if(match != SP_MATCHED_POSITIVE) {
        rrdhost_identity_release(&identity);
        return QUERY_SCOPE_HOST_OUT_OF_SCOPE;
    }

    if(hosts->sp)
        match = query_scope_hosts_filter_match_identity(hosts, identity.hostname, host->machine_guid, host_node_id_str);

    rrdhost_identity_release(&identity);
    return (match == SP_MATCHED_POSITIVE) ? QUERY_SCOPE_HOST_QUERYABLE : QUERY_SCOPE_HOST_IN_SCOPE;
}

static void query_scope_versions_add(struct query_versions *versions, RRDHOST *host) {
    versions->contexts_hard_hash += dictionary_version(host->rrdctx.contexts);
    versions->contexts_soft_hash += rrdcontext_queue_version(&host->rrdctx.hub_queue);
    versions->alerts_hard_hash += dictionary_version(host->rrdcalc_root_index);
    versions->alerts_soft_hash += __atomic_load_n(&host->health_transitions, __ATOMIC_RELAXED);
}

ssize_t query_scope_foreach_host(SIMPLE_PATTERN *scope_hosts_sp, SIMPLE_PATTERN *hosts_sp,
                                  foreach_host_cb_t cb, void *data,
                                  struct query_versions *versions,
                                  char *host_node_id_str) {
    char uuid[UUID_STR_LEN];
    if(!host_node_id_str) host_node_id_str = uuid;
    host_node_id_str[0] = '\0';

    size_t entries = dictionary_entries(rrdhost_root_index);
    QUERY_SCOPE_HOSTS_FILTER scope_hosts, hosts;
    query_scope_hosts_filter_init(&scope_hosts, scope_hosts_sp, entries);
    query_scope_hosts_filter_init(&hosts, hosts_sp, entries);

    RRDHOST *host;
    ssize_t added = 0;
    struct query_versions v = { 0 };

    dfe_start_read(rrdhost_root_index, host) {
        QUERY_SCOPE_HOST_MATCH match = query_scope_host_match(host, &scope_hosts, &hosts, host_node_id_str);
        if(match == QUERY_SCOPE_HOST_OUT_OF_SCOPE)
            continue;

        dfe_unlock(host);

        query_scope_versions_add(&v, host);
        ssize_t ret = cb(data, host, match == QUERY_SCOPE_HOST_QUERYABLE);
        if(ret < 0) {
            added = ret;
            break;
        }
        added += ret;
    }
    dfe_done(host);

    query_scope_hosts_filter_cleanup(&scope_hosts);
    query_scope_hosts_filter_cleanup(&hosts);

    if(versions)
        *versions = v;

    return added;
}

// Pins every queryable host in scope (see metadata_lifetime_lock in rrdhost.h), so the caller can keep
// using them after the walk - from other threads too - until query_scope_hosts_unpin(). A host that
// cannot be pinned is left out: a removal holds it, or is trying to take it at that moment (an attempt
// that fails when the host is already pinned elsewhere). The versions cover every host in scope, as
// query_scope_foreach_host() computes them.
//
// It waits for rrd_rdlock(): call it only from a thread that no rrd_wrlock() holder waits on (see
// rrdhost_apply_by_machine_guid()). The pins MUST be released on the same thread.
size_t query_scope_hosts_pin(SIMPLE_PATTERN *scope_hosts_sp, SIMPLE_PATTERN *hosts_sp,
                             RRDHOST ***hosts, struct query_versions *versions) {
    char uuid[UUID_STR_LEN];
    struct query_versions v = { 0 };
    size_t count = 0;

    // built before taking the lock, so a long list does not lengthen the time rrd_rdlock() is held;
    // the host count may change before the lock is taken, which only moves the choice between set and pattern
    size_t entries = dictionary_entries(rrdhost_root_index);
    QUERY_SCOPE_HOSTS_FILTER scope_hosts, hosts_filter;
    query_scope_hosts_filter_init(&scope_hosts, scope_hosts_sp, entries);
    query_scope_hosts_filter_init(&hosts_filter, hosts_sp, entries);

    rrd_rdlock();

    // hosts are added under rrd_wrlock(), so the index cannot grow while we hold the read lock
    entries = dictionary_entries(rrdhost_root_index);
    RRDHOST **array = mallocz((entries ? entries : 1) * sizeof(*array));

    RRDHOST *host;
    dfe_start_read(rrdhost_root_index, host) {
        QUERY_SCOPE_HOST_MATCH match = query_scope_host_match(host, &scope_hosts, &hosts_filter, uuid);
        if(match == QUERY_SCOPE_HOST_OUT_OF_SCOPE)
            continue;

        query_scope_versions_add(&v, host);

        if(match == QUERY_SCOPE_HOST_QUERYABLE && count < entries && rrdhost_pin_try(host))
            array[count++] = host;
    }
    dfe_done(host);

    rrd_rdunlock();

    query_scope_hosts_filter_cleanup(&scope_hosts);
    query_scope_hosts_filter_cleanup(&hosts_filter);

    if(versions)
        *versions = v;

    *hosts = array;
    return count;
}

void query_scope_hosts_unpin(RRDHOST **hosts, size_t count) {
    for(size_t i = 0; i < count; i++)
        rrdhost_pin_release(hosts[i]);

    freez(hosts);
}

// A plain context name (no separators, wildcards, negation or escapes) compiles to a pattern
// that matches only that name, so a dictionary miss for it is final.
static bool scope_contexts_is_plain_name(const char *scope_contexts, SIMPLE_PATTERN *scope_contexts_sp) {
    return scope_contexts && *scope_contexts && scope_contexts_sp && !strchr(scope_contexts, '\\') &&
           !simple_pattern_contains_wildcards(scope_contexts, SIMPLE_PATTERN_DEFAULT_WEB_SEPARATORS);
}

ssize_t query_scope_foreach_context(RRDHOST *host, const char *scope_contexts, SIMPLE_PATTERN *scope_contexts_sp,
                                   SIMPLE_PATTERN *contexts_sp, foreach_context_cb_t cb, bool queryable_host, void *data,
                                   foreach_stop_cb_t stop_cb) {
    if(unlikely(!host->rrdctx.contexts))
        return 0;

    ssize_t added = 0;

    RRDCONTEXT_ACQUIRED *rca = NULL;

    if(scope_contexts)
        rca = (RRDCONTEXT_ACQUIRED *)dictionary_get_and_acquire_item(host->rrdctx.contexts, scope_contexts);

    if(likely(rca)) {
        // we found it!

        bool queryable_context = queryable_host;
        RRDCONTEXT *rc = rrdcontext_acquired_value(rca);
        if(queryable_context && contexts_sp && !simple_pattern_matches_string(contexts_sp, rc->id))
            queryable_context = false;

        added = cb(data, rca, queryable_context);

        rrdcontext_release(rca);
    }
    else if(!scope_contexts_is_plain_name(scope_contexts, scope_contexts_sp)) {
        // Probably it is a pattern, we need to search for it...
        RRDCONTEXT *rc;
        dfe_start_read(host->rrdctx.contexts, rc) {
            // polled before the scope filter: a scan that rejects every context never reaches cb
            if(stop_cb && stop_cb(data)) {
                added = -1;
                break;
            }

            if(scope_contexts_sp && !simple_pattern_matches_string(scope_contexts_sp, rc->id))
                continue;

            dfe_unlock(rc);

            bool queryable_context = queryable_host;
            if(queryable_context && contexts_sp && !simple_pattern_matches_string(contexts_sp, rc->id))
                queryable_context = false;

            ssize_t ret = cb(data, (RRDCONTEXT_ACQUIRED *)rc_dfe.item, queryable_context);

            if(ret < 0) {
                added = ret;
                break;
            }

            added += ret;
        }
        dfe_done(rc);
    }

    return added;
}


// ----------------------------------------------------------------------------
// unit test: the filter must return what the pattern returns, for every identifier

#define QSU_HOSTS 64

struct qsu_host {
    STRING *hostname;
    char machine_guid[UUID_STR_LEN];
    char node_id[UUID_STR_LEN];
};

static uint64_t qsu_seed = 0x9E3779B97F4A7C15ULL;
static uint64_t qsu_rand(void) {
    qsu_seed ^= qsu_seed << 13;
    qsu_seed ^= qsu_seed >> 7;
    qsu_seed ^= qsu_seed << 17;
    return qsu_seed;
}

static void qsu_uuid(char *dst) {
    nd_uuid_t uuid;
    uint64_t a = qsu_rand(), b = qsu_rand();
    memcpy(uuid, &a, sizeof(a));
    memcpy(&uuid[8], &b, sizeof(b));
    uuid_unparse_lower(uuid, dst);
}

// today's matching, straight on the pattern
static SIMPLE_PATTERN_RESULT qsu_reference(SIMPLE_PATTERN *sp, struct qsu_host *h) {
    SIMPLE_PATTERN_RESULT match = simple_pattern_matches_string_extract(sp, h->hostname, NULL, 0);
    if(match == SP_NOT_MATCHED) {
        match = simple_pattern_matches_extract(sp, h->machine_guid, NULL, 0);
        if(match == SP_NOT_MATCHED && *h->node_id)
            match = simple_pattern_matches_extract(sp, h->node_id, NULL, 0);
    }
    return match;
}

// separators inside a hostname are escaped, so the hostname stays one entry
static void qsu_put_escaped(BUFFER *wb, const char *s) {
    for(; *s; s++) {
        if(*s == '|' || *s == ',')
            buffer_putc(wb, '\\');
        buffer_putc(wb, *s);
    }
}

// a token that names one identifier of a host, or nothing known
static void qsu_token(BUFFER *wb, struct qsu_host *hosts, bool plain) {
    struct qsu_host *h = &hosts[qsu_rand() % QSU_HOSTS];
    size_t kind = qsu_rand() % (plain ? 6 : 10);

    switch(kind) {
        case 0:  qsu_put_escaped(wb, h->hostname ? string2str(h->hostname) : "missing-host"); break;
        case 1:  buffer_strcat(wb, h->machine_guid); break;
        case 2:  buffer_strcat(wb, *h->node_id ? h->node_id : h->machine_guid); break;
        case 3: {
            char uuid[UUID_STR_LEN];
            qsu_uuid(uuid);
            buffer_strcat(wb, uuid);
            break;
        }
        case 4:  buffer_strcat(wb, "host\\|with\\,separators"); break;   // escaped separators, unescaped by the parser
        case 5: {
            // the same node id in upper case: must not match a case sensitive pattern, nor the set
            const char *s = *h->node_id ? h->node_id : h->machine_guid;
            for(; *s; s++) buffer_putc(wb, (char)toupper((unsigned char)*s));
            break;
        }
        case 6:  buffer_sprintf(wb, "!%s", h->machine_guid); break;
        case 7:  buffer_strcat(wb, "host-1*"); break;
        case 8:  buffer_strcat(wb, "*"); break;
        default:
            buffer_putc(wb, '!');
            qsu_put_escaped(wb, h->hostname ? string2str(h->hostname) : "missing-host");
            break;
    }
}

// agent_hosts: the host count the filter is built for; the check itself always matches the QSU_HOSTS hosts
static int qsu_check(const char *list, struct qsu_host *hosts, size_t agent_hosts, bool expect_set) {
    int errors = 0;

    SIMPLE_PATTERN *sp = string_to_simple_pattern(list);
    QUERY_SCOPE_HOSTS_FILTER f;
    query_scope_hosts_filter_init(&f, sp, agent_hosts);

    if(sp && (f.set != NULL) != expect_set) {
        fprintf(stderr, "QUERY SCOPE UNITTEST: pattern of %zu exact entries for %zu hosts %s the set, expected %s: %s\n",
                simple_pattern_exact_entries(sp), agent_hosts, f.set ? "uses" : "does not use",
                expect_set ? "set" : "pattern", list);
        errors++;
    }

    for(size_t i = 0; i < QSU_HOSTS; i++) {
        struct qsu_host *h = &hosts[i];
        SIMPLE_PATTERN_RESULT want, got;

        // each identifier on its own, then the three in order
        want = simple_pattern_matches_string_extract(sp, h->hostname, NULL, 0);
        got = query_scope_hosts_filter_match_string(&f, h->hostname);
        if(want != got) { errors++; fprintf(stderr, "QUERY SCOPE UNITTEST: hostname of host %zu: want %d got %d, pattern: %s\n", i, want, got, list); }

        want = simple_pattern_matches_extract(sp, h->machine_guid, NULL, 0);
        got = query_scope_hosts_filter_match(&f, h->machine_guid);
        if(want != got) { errors++; fprintf(stderr, "QUERY SCOPE UNITTEST: machine guid of host %zu: want %d got %d, pattern: %s\n", i, want, got, list); }

        want = simple_pattern_matches_extract(sp, h->node_id, NULL, 0);
        got = query_scope_hosts_filter_match(&f, h->node_id);
        if(want != got) { errors++; fprintf(stderr, "QUERY SCOPE UNITTEST: node id of host %zu: want %d got %d, pattern: %s\n", i, want, got, list); }

        if(sp) {
            want = qsu_reference(sp, h);
            got = query_scope_hosts_filter_match_identity(&f, h->hostname, h->machine_guid, h->node_id);
            if(want != got) { errors++; fprintf(stderr, "QUERY SCOPE UNITTEST: host %zu: want %d got %d, pattern: %s\n", i, want, got, list); }
        }
    }

    query_scope_hosts_filter_cleanup(&f);
    simple_pattern_free(sp);
    return errors;
}

int query_scope_unittest(void) {
    fprintf(stderr, "\n%s() running...\n", __FUNCTION__);

    int errors = 0;
    struct qsu_host hosts[QSU_HOSTS] = { 0 };

    for(size_t i = 0; i < QSU_HOSTS; i++) {
        char name[64];
        if(i == 0)
            hosts[i].hostname = NULL;                                   // no hostname
        else if(i == 1)
            hosts[i].hostname = string_strdupz("host|with,separators"); // matched only through escapes
        else {
            snprintfz(name, sizeof(name), "host-%zu", i);
            hosts[i].hostname = string_strdupz(name);
        }

        qsu_uuid(hosts[i].machine_guid);
        if(i % 5)
            qsu_uuid(hosts[i].node_id);
        else
            hosts[i].node_id[0] = '\0';                                 // no node id
    }

    // the helper itself
    {
        struct { const char *list; size_t expected; } cases[] = {
            { "a",              1 },
            { "a|b,c",          3 },
            { "a|a|a",          3 },        // repeated entries are counted
            { "a\\|b|c",        2 },        // an escaped separator is part of the entry
            { "a|!b",           0 },
            { "a|b*",           0 },
            { "a|*b",           0 },
            { "a|*b*",          0 },
            { "a|b*c",          0 },
            { "*",              0 },        // string_to_simple_pattern() gives NULL for "*"
            { NULL,             0 },
        };
        for(size_t i = 0; i < _countof(cases); i++) {
            SIMPLE_PATTERN *sp = string_to_simple_pattern(cases[i].list);
            size_t got = simple_pattern_exact_entries(sp);
            if(got != cases[i].expected) {
                fprintf(stderr, "QUERY SCOPE UNITTEST: simple_pattern_exact_entries(\"%s\") = %zu, expected %zu\n",
                        cases[i].list ? cases[i].list : "(null)", got, cases[i].expected);
                errors++;
            }
            simple_pattern_free(sp);
        }

        SIMPLE_PATTERN *sp = string_to_simple_pattern_nocase("a|b");
        if(simple_pattern_exact_entries(sp) != 0) {
            fprintf(stderr, "QUERY SCOPE UNITTEST: a case insensitive pattern must not count as exact\n");
            errors++;
        }
        simple_pattern_free(sp);
    }

    // random lists on both sides of the threshold, plain ones (set) and mixed ones (pattern)
    size_t sizes[] = { 1, 2, QUERY_SCOPE_HOSTS_SET_MIN_ENTRIES - 1, QUERY_SCOPE_HOSTS_SET_MIN_ENTRIES,
                       QUERY_SCOPE_HOSTS_SET_MIN_ENTRIES + 1, 40, 200, 1000 };
    BUFFER *wb = buffer_create(0, NULL);
    for(size_t round = 0; round < 50; round++) {
        for(size_t s = 0; s < _countof(sizes); s++) {
            for(int plain = 0; plain <= 1; plain++) {
                buffer_flush(wb);
                bool has_non_exact = false;
                for(size_t t = 0; t < sizes[s]; t++) {
                    if(t)
                        buffer_putc(wb, (qsu_rand() & 1) ? '|' : ',');

                    size_t before = buffer_strlen(wb);
                    qsu_token(wb, hosts, plain);
                    const char *tok = &buffer_tostring(wb)[before];
                    if(*tok == '!' || strchr(tok, '*'))
                        has_non_exact = true;
                }

                // one disqualifying entry at the end of a long plain list keeps the pattern
                if(!plain && !has_non_exact && sizes[s] >= QUERY_SCOPE_HOSTS_SET_MIN_ENTRIES) {
                    buffer_strcat(wb, "|host-1*");
                    has_non_exact = true;
                }

                bool expect_set = !has_non_exact && sizes[s] >= QUERY_SCOPE_HOSTS_SET_MIN_ENTRIES;
                errors += qsu_check(buffer_tostring(wb), hosts, QSU_HOSTS, expect_set);

                // too few hosts to repay the set: the same list stays on the pattern
                if(expect_set) {
                    errors += qsu_check(buffer_tostring(wb), hosts, QUERY_SCOPE_HOSTS_SET_MIN_HOSTS - 1, false);
                    errors += qsu_check(buffer_tostring(wb), hosts, QUERY_SCOPE_HOSTS_SET_MIN_HOSTS, true);
                }
            }
        }
    }
    buffer_free(wb);

    // no pattern at all
    errors += qsu_check(NULL, hosts, QSU_HOSTS, false);

    for(size_t i = 0; i < QSU_HOSTS; i++)
        string_freez(hosts[i].hostname);

    fprintf(stderr, "%s() %s with %d errors\n", __FUNCTION__, errors ? "FAILED" : "passed", errors);
    return errors ? 1 : 0;
}
